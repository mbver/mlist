package memberlist

import (
	"time"

	"math/rand"
)

func (m *Memberlist) schedule() {}

func (m *Memberlist) deschedule() {}

func (m *Memberlist) scheduleFunc(interval time.Duration, stopCh chan struct{}, f func()) {
	t := time.NewTicker(interval)
	jitter := time.Duration(uint64(rand.Int63()) % uint64(interval))
	time.Sleep(jitter) // wait random fraction of interval to avoid thundering herd
	for {
		select {
		case <-t.C:
			f()
		case <-stopCh:
			t.Stop()
			return
		}
	}
}

func (m *Memberlist) scheduleFuncDynamic(interval time.Duration, stopCh chan struct{}, scaleFunc func(time.Duration, int) time.Duration, f func()) {
	jitter := time.Duration(uint64(rand.Int63()) % uint64(interval))
	time.Sleep(jitter) // wait random fraction of interval to avoid thundering herd
	for {
		scaledInterval := scaleFunc(interval, m.getNumNodes())
		select {
		case <-time.After(scaledInterval):
			f()
		case <-stopCh:
			return
		}
	}
}

func (m *Memberlist) gossip() {}

func (m *Memberlist) pickRandomNodes(numNodes int, acceptFn func(*nodeState) bool) []*nodeState {
	m.nodeL.RLock()
	defer m.nodeL.RUnlock()
	n := len(m.nodes)
	picked := make([]*nodeState, 0, numNodes)

PICKNODE:
	for i := 0; i < 3*n && len(picked) < numNodes; i++ {
		// Get random node
		idx := randIntN(n)
		node := m.nodes[idx]
		// check if it is ok to pick
		if acceptFn != nil && !acceptFn(node) {
			continue

		}
		// check if it is already picked
		for j := 0; j < len(picked); j++ {
			if node.Node.ID == picked[j].Node.ID {
				continue PICKNODE
			}
		}
		picked = append(picked, node.Clone())
	}
	return picked
}

func (m *Memberlist) probe() {}

func (m *Memberlist) nextProbeNode() (*nodeState, error) {
	return nil, nil
}

func (m *Memberlist) probeNode(node *nodeState) {
	pingTimeout := m.awr.ScaleTimeout(m.config.PingTimeout)
	probeTimeout := m.awr.ScaleTimeout(m.config.ProbeTimeout)
	success := m.Ping(node, pingTimeout)
	if success {
		m.awr.Punish(-1) // improve health
		return
	}

	timeout := probeTimeout - pingTimeout
	indirectCh := m.IndirectPing(node, timeout)
	tcpCh := m.TcpPing(node, timeout)
	var indirectRes indirectPingResult

WAIT:
	for {
		select {
		case indirectRes = <-indirectCh:
			if indirectRes.success {
				success = true
				break WAIT
			}
		case res := <-tcpCh:
			if res {
				success = true
				break WAIT
			}
		case <-time.After(timeout + 10*time.Millisecond):
			break WAIT
		}
	}

	if success {
		m.awr.Punish(-1) // improve health
		return
	}

	missedNacks := indirectRes.numNode - indirectRes.nNacks
	m.awr.Punish(missedNacks)
	// m.logger.Printf("[INFO] memberlist: Suspect %s has failed, no acks received", node.Name)
	s := suspect{Lives: node.Lives, ID: node.Node.ID, From: m.config.ID}
	m.suspectNode(&s)
}

// reap
func (m *Memberlist) resetNodes() {}

func (m *Memberlist) swapExpiredToEnd(nodes []*nodeState, timeout time.Duration) int {
	return 0
}

// reconnect
