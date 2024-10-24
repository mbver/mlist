package memberlist

import (
	"fmt"
	"sync/atomic"
	"time"

	"math/rand"
)

// call just once
func (m *Memberlist) schedule() {
	if m.config.ProbeInterval > 0 {
		go m.scheduleFunc(m.config.ProbeInterval, m.stopScheduleCh, m.probe)
	}
	if m.config.GossipInterval > 0 && m.config.GossipNodes > 0 {
		go m.scheduleFunc(m.config.GossipInterval, m.stopScheduleCh, m.gossip)
	}
	if m.config.ReapInterval > 0 {
		go m.scheduleFunc(m.config.ReapInterval, m.stopScheduleCh, m.reap)
	}
	if m.config.PushPullInterval > 0 {
		go m.scheduleFuncWithScale(m.config.PushPullInterval, m.stopScheduleCh, pushPullScale, m.pushPull)
	}
}

func (m *Memberlist) deschedule() {
	close(m.stopScheduleCh)
}

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

func (m *Memberlist) scheduleFuncWithScale(interval time.Duration, stopCh chan struct{}, scaleFunc func(time.Duration, int) time.Duration, f func()) {
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

func (m *Memberlist) gossip() {
	// Get some random live, suspect, or unexpired nodes
	nodes := m.pickRandomNodes(m.config.GossipNodes, func(n *nodeState) bool {
		return n.Node.ID != m.config.ID &&
			(n.State != StateDead ||
				(n.State == StateDead && time.Since(n.StateChange) <= m.config.DeadNodeExpiredTimeout))
	})

	// Compute the bytes available
	remainingBytes := m.config.UDPBufferSize - compoundHeaderOverhead
	if m.EncryptionEnabled() {
		remainingBytes -= encryptOverhead(m.config.EncryptionVersion)
	}
	for _, node := range nodes {
		// Get any pending broadcasts
		msgs := m.getBroadcasts(lenMsgOverhead, remainingBytes)
		if len(msgs) == 0 {
			return
		}
		addr := node.Node.UDPAddress()
		if len(msgs) == 1 {
			if err := m.sendUdp(addr, msgs[0]); err != nil {
				m.logger.Printf("[ERR] memberlist: Failed to send gossip to %s: %s", addr, err)
			}
		} else {
			compounds := splitToCompoundMsgs(msgs)
			for _, compound := range compounds {
				if err := m.sendUdp(addr, compound); err != nil {
					m.logger.Printf("[ERR] memberlist: Failed to send gossip to %s: %s", addr, err)
				}
			}
		}
	}
}

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

func (m *Memberlist) probe() {
	node, err := m.nextProbeNode()
	if err != nil {
		// log error
		return
	}
	m.probeNode(node)
}

func (m *Memberlist) nextProbeNode() (*nodeState, error) {
	m.nodeL.RLock()
	defer m.nodeL.Unlock()
	m.probeIdx++
	if m.probeIdx >= len(m.nodes) {
		m.probeIdx = 0
	}
	for m.probeIdx < len(m.nodes) {
		node := m.nodes[m.probeIdx]
		if node.Node.ID == m.config.ID || node.DeadOrLeft() {
			m.probeIdx++
			continue
		}
		return node.Clone(), nil
	}
	return nil, fmt.Errorf("no node to probe")
}

func (m *Memberlist) probeNode(node *nodeState) {
	pingTimeout := m.awr.ScaleTimeout(m.config.PingTimeout)
	probeTimeout := m.awr.ScaleTimeout(m.config.ProbeInterval)
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

func (m *Memberlist) reap() {
	m.nodeL.Lock()
	defer m.nodeL.Unlock()

	expiredIdx := swapExpiredToEnd(m.nodes, m.config.DeadNodeExpiredTimeout)

	// Deregister the dead nodes
	for i := expiredIdx; i < len(m.nodes); i++ {
		delete(m.nodeMap, m.nodes[i].Node.ID)
		m.nodes[i] = nil
	}

	// Trim the nodes to exclude the dead nodes
	m.nodes = m.nodes[:expiredIdx]

	atomic.StoreInt32(&m.numNodes, int32(expiredIdx))
	// Shuffle live nodes
	shuffleNodes(m.nodes)
}

func swapExpiredToEnd(nodes []*nodeState, timeout time.Duration) int {
	expiredIdx := len(nodes)
	for i := 0; i < expiredIdx; i++ {
		if !nodes[i].DeadOrLeft() {
			continue
		}

		// check for expired
		if time.Since(nodes[i].StateChange) <= timeout {
			continue
		}

		// swap to end
		expiredIdx--
		nodes[i], nodes[expiredIdx] = nodes[expiredIdx], nodes[i]
		i-- // backtrack
	}
	return expiredIdx
}
