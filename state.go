package memberlist

import (
	"bytes"
	"net"
	"sync/atomic"
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
	Tags []byte
}

func (n *Node) UDPAddress() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   n.IP,
		Port: int(n.Port),
	}
}

type nodeState struct {
	Node        *Node
	Lives       uint32
	State       StateType
	StateChange time.Time
}

func (n *nodeState) DeadOrLeft() bool {
	return n.State == StateDead || n.State == StateLeft
}

func (n *nodeState) Clone() *nodeState {
	return &nodeState{
		Node: &Node{
			ID:   n.Node.ID,
			IP:   copyBytes(n.Node.IP),
			Port: n.Node.Port,
		},
		Lives:       n.Lives,
		State:       n.State,
		StateChange: n.StateChange,
	}
}

type alive struct {
	Lives uint32
	Node  string
	IP    net.IP
	Port  uint16
	Tags  []byte
}

type suspect struct {
	Lives uint32
	Node  string
	From  string
}

type dead struct {
	Lives uint32
	Node  string
}

type leave struct {
	Lives uint32
	Node  string
}

func (m *Memberlist) aliveNode(a *alive, notify chan struct{}) {
	isLocalNode := a.Node == m.config.ID
	if m.hasLeft() && isLocalNode { // seems no need, because Lives in increased larger that alive
		return
	}
	if err := m.IsIPAllowed(a.IP); err != nil {
		// m.logger.Printf("[WARN] memberlist: Rejected node %s (%v): %s", a.ID, net.IP(a.Addr), errCon)
		return
	}

	var rebroadcast bool
	var notifyJoin bool
	var notifyUpdate bool
	defer func() {
		if rebroadcast {
			// m.encodeBroadcastNotify(state.Node, aliveMsg, a, notify) // change later to fit new broadcast signature
		}
		// if m.Events == nil {
		// 	return
		// }
		if notifyJoin {
			// m.config.Events.NotifJoin(state.Node) // change later to fit new design. maybe joinCh to receive join events
		}
		if notifyUpdate {
			// 	m.config.Events.NotifyUpdate(state.Node) // change later, maybe using a channel. maybe dropping it. tags is managed in memberlist
		}
	}()

	m.nodeL.Lock()
	defer m.nodeL.Unlock()
	node, ok := m.nodeMap[a.Node]
	if !ok {
		node = &nodeState{
			Node: &Node{
				ID:   a.Node,
				IP:   a.IP,
				Port: a.Port,
				Tags: a.Tags,
			},
			Lives:       a.Lives,
			State:       StateAlive,
			StateChange: time.Now(),
		}
		// Add to node list
		m.nodeMap[a.Node] = node
		m.nodes = append(m.nodes, node)
		n := len(m.nodes)
		idx := randIntN(n)
		m.nodes[idx], m.nodes[n] = m.nodes[n], m.nodes[idx]
		atomic.AddInt32(&m.numNodes, 1)

		rebroadcast = true
		notifyJoin = true
		return
	}

	if a.Lives <= node.Lives {
		return
	}

	delete(m.suspicions, a.Node) // clear suspicion ==> what happen when the node fire? we got stateChange difference. don't worry
	rebroadcast = true
	if node.DeadOrLeft() {
		notifyJoin = true
	}
	if !bytes.Equal(node.Node.Tags, a.Tags) {
		notifyUpdate = true
	}
	node.Lives = a.Lives // ===== use Lives in suspicion timeoutFn, not StateChange. this work if the node is alive and suspect again or just becomes alive
	node.Node.Tags = copyBytes(a.Tags)
	if node.State != StateAlive {
		node.State = StateAlive
		node.StateChange = time.Now()
	}
}

func (m *Memberlist) suspectNode(s *suspect) {}

func (m *Memberlist) deadNode(d *dead) {}

func (m *Memberlist) leaveNode(l *leave) {}

// broadcast state alive or dead
func (m *Memberlist) refute(lives int) {}

type stateToMerge struct{}
