package memberlist

import (
	"net"
	"time"
)

type StateType int

const (
	StateAlive StateType = iota
	StateSuspect
	StateDead
	StateLeft
)

type Node struct {
	ID   string
	IP   net.IP
	Port uint16
}

func (n *Node) UDPAddress() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   n.IP,
		Port: int(n.Port),
	}
}

func (n *Node) TCPAddress() *net.TCPAddr {
	return &net.TCPAddr{
		IP:   n.IP,
		Port: int(n.Port),
	}
}

type nodeState struct {
	Node  *Node
	Lives uint32
	State StateType
}

func (n *nodeState) DeadOrLeft() bool {
	return n.State == StateDead || n.State == StateLeft
}

func (n *nodeState) Clone() *nodeState {
	return &nodeState{
		Node: &Node{
			ID:   n.Node.ID,
			IP:   n.Node.IP,
			Port: n.Node.Port,
		},
		Lives: n.Lives,
		State: n.State,
	}
}

type alive struct{}

type suspect struct {
	Lives uint32
	Node  string
	From  string
}

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
