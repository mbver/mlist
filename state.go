package memberlist

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
