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
		scaledInterval := scaleFunc(interval, m.numNodes())
		select {
		case <-time.After(scaledInterval):
			f()
		case <-stopCh:
			return
		}
	}
}

func (m *Memberlist) gossip() {}

func (m *Memberlist) pickRandomNodes(num int, accept func(*nodeState) bool) []Node {
	return nil
}

func (m *Memberlist) probe() {}

func (m *Memberlist) nextProbeNode() (*nodeState, error) {
	return nil, nil
}

func (m *Memberlist) probeNode(node *nodeState) {}

// reap
func (m *Memberlist) resetNodes() {}

func (m *Memberlist) swapExpiredToEnd(nodes []*nodeState, timeout time.Duration) int {
	return 0
}

// reconnect
