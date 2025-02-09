// Copyright (c) HashiCorp, Inc.
// Copyright (c) 2024 Phuoc Phi
// SPDX-License-Identifier: MPL-2.0
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

func (t StateType) String() string {
	switch t {
	case StateAlive:
		return "alive"
	case StateSuspect:
		return "suspect"
	case StateDead:
		return "dead"
	case StateLeft:
		return "left"
	}
	return "unknown state"
}

type Node struct {
	ID   string
	IP   net.IP
	Port uint16
	Tags []byte
}

func (n *Node) Clone() *Node {
	return &Node{
		ID:   n.ID,
		IP:   copyBytes(n.IP),
		Port: n.Port,
		Tags: copyBytes(n.Tags),
	}
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
		Node:        n.Node.Clone(),
		Lives:       n.Lives,
		State:       n.State,
		StateChange: n.StateChange,
	}
}

type alive struct {
	Lives uint32
	ID    string
	IP    net.IP
	Port  uint16
	Tags  []byte
}

type suspect struct {
	Lives uint32
	ID    string
	From  string
}

type dead struct {
	Lives uint32
	ID    string
	Left  bool
}

func (m *Memberlist) nextLiveNo() uint32 {
	return atomic.AddUint32(&m.lives, 1)
}

func (m *Memberlist) aliveNode(a *alive, notify chan struct{}) {
	isLocalNode := a.ID == m.ID()
	if m.hasLeft() && isLocalNode {
		return
	}
	if !m.IPAllowed(a.IP) {
		m.logger.Printf("[WARN] memberlist: Rejected node %s: %s", a.ID, a.IP)
		return
	}

	var rebroadcast bool
	var notifyJoin bool
	var notifyUpdate bool
	var newNode *Node

	defer func() {
		if rebroadcast {
			m.broadcast(newNode.ID, aliveMsg, a, notify)
		}
		if notifyJoin {
			m.eventMng.NotifyJoin(newNode)
		}
		if notifyUpdate {
			m.eventMng.NotifyUpdate(newNode)
		}
	}()

	m.nodeL.Lock()
	defer m.nodeL.Unlock()
	node, ok := m.nodeMap[a.ID]
	if !ok {
		node = &nodeState{
			Node: &Node{
				ID:   a.ID,
				IP:   a.IP,
				Port: a.Port,
				Tags: a.Tags,
			},
			Lives:       a.Lives,
			State:       StateAlive,
			StateChange: time.Now(),
		}
		// Add to node list
		m.nodeMap[a.ID] = node
		m.nodes = append(m.nodes, node)
		n := len(m.nodes) - 1
		idx := randIntN(n)
		m.nodes[idx], m.nodes[n] = m.nodes[n], m.nodes[idx]
		atomic.AddInt32(&m.numNodes, 1)

		rebroadcast = true
		notifyJoin = true
		newNode = node.Node.Clone()
		return
	}

	// make sure to rejoin with higher Lives than when we stopped
	// so don't have to handle the case a.Lives == node.Lives specially
	if a.Lives <= node.Lives {
		return
	}

	delete(m.suspicions, a.ID)

	node.Lives = a.Lives

	rebroadcast = true
	if node.DeadOrLeft() {
		notifyJoin = true
	}
	if !bytes.Equal(node.Node.Tags, a.Tags) {
		node.Node.Tags = copyBytes(a.Tags)
		notifyUpdate = true
	}
	if node.State != StateAlive {
		node.State = StateAlive
		node.StateChange = time.Now()
	}
	newNode = node.Node.Clone()
}

func (m *Memberlist) suspectNode(s *suspect) {
	m.nodeL.Lock()
	defer m.nodeL.Unlock()
	node, ok := m.nodeMap[s.ID]

	// ignore if the node is gone | message is too old
	if !ok || s.Lives < node.Lives {
		return
	}

	// already suspected. confirm
	if sc, ok := m.suspicions[s.ID]; ok {
		if sc.Confirm(s.From) {
			m.broadcast(s.ID, suspectMsg, s, nil)
		}
		return
	}

	// ignore non-alive nodes
	if node.State != StateAlive {
		return
	}

	// If this is us we need to refute
	if s.ID == m.ID() {
		m.refute(node, s.Lives)
		// m.logger.Printf("[WARN] memberlist: Refuting a suspect message")
		return
	}

	// first time suspect
	m.broadcast(s.ID, suspectMsg, s, nil)
	node.Lives = s.Lives
	node.State = StateSuspect
	node.StateChange = time.Now()

	// create a suspicion timer
	confirmCap := m.config.SuspicionMult - 1
	n := m.GetNumNodes()
	if n-1 < confirmCap {
		confirmCap = 1
	}

	minTimeout := minSuspicionTimeout(m.config.SuspicionMult, n, m.config.ProbeInterval)
	maxTimeout := time.Duration(m.config.SuspicionMaxTimeoutMult) * minTimeout
	lives := s.Lives
	timeoutFn := func(confirms int) {
		var d *dead
		m.nodeL.RLock()
		node, ok := m.nodeMap[s.ID]
		proceed := ok && node.State == StateSuspect && node.Lives == lives
		if proceed {
			d = &dead{Lives: node.Lives, ID: node.Node.ID}
		}
		m.nodeL.RUnlock()

		if proceed {
			if confirms < confirmCap { // timeout before reaching enough confirms
				m.logger.Printf("degraded: timeout before reaching enough confirms %d/%d", confirms, confirmCap)
			}

			m.logger.Printf("[INFO] memberlist: Marking %s as failed, suspect timeout reached (%d peer confirmations)",
				node.Node.ID, confirms)
			m.deadNode(d, nil)
		}
	}
	m.suspicions[s.ID] = newSuspicion(minTimeout, maxTimeout, confirmCap, s.From, timeoutFn)
}

// must hold lock
func (m *Memberlist) refute(node *nodeState, lives uint32) {
	l := m.nextLiveNo()
	for l <= lives {
		l = m.nextLiveNo()
	}
	node.Lives = l
	m.awr.Punish(1) // punish health if we have to refute
	a := alive{
		Lives: l,
		ID:    node.Node.ID,
		IP:    node.Node.IP,
		Port:  node.Node.Port,
		Tags:  copyBytes(node.Node.Tags),
	}
	m.broadcast(a.ID, aliveMsg, a, nil)
}

func (m *Memberlist) deadNode(d *dead, notify chan struct{}) {
	m.nodeL.Lock()
	defer m.nodeL.Unlock()
	node, ok := m.nodeMap[d.ID]

	// ignore if the node is gone | message is too old | the node already dead
	if !ok || d.Lives < node.Lives || node.DeadOrLeft() {
		return
	}

	isLocalNode := d.ID == m.ID()
	// if this is us & still active, refute
	if isLocalNode && !m.hasLeft() {
		m.refute(node, d.Lives)
		m.logger.Printf("[WARN] memberlist: Refuting a dead message")
		return
	}

	// Clear suspicion timers
	delete(m.suspicions, d.ID)

	m.broadcast(d.ID, deadMsg, d, notify)

	// Update the state
	node.Lives = d.Lives
	node.State = StateDead
	if d.Left {
		node.State = StateLeft
	}
	node.StateChange = time.Now()
	if m.eventMng != nil {
		m.eventMng.NotifyLeave(node.Node.Clone())
	}
}
