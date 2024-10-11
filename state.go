package memberlist

import "time"

type StateType int

const (
	StateAlive StateType = iota
	StateSuspect
	StateDead
	StateLeft
)

type Node struct{}

type nodeState struct{}

type alive struct{}

type suspect struct{}

type dead struct{}

type leave struct{}

func (m *Memberlist) aliveNode(a *alive, notify chan struct{}) {}

func (m *Memberlist) suspectNode(s *suspect) {}

func (m *Memberlist) deadNode(d *dead) {}

func (m *Memberlist) leaveNode(l *leave) {}

// broadcast state alive or dead
func (m *Memberlist) refute(lives int) {}

type stateToMerge struct{}

type suspicion struct{}

func newSuspicion(
	minTimeout time.Duration,
	maxTimeout time.Duration,
	confirmCap int, from string,
	timeoutFn func(uint32),
) *suspicion {
	return nil
}

func (s *suspicion) remainingTime() time.Duration {
	return 0
}

func (s *suspicion) Confirm(from string) bool {
	return false
}
