package memberlist

import (
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPickRandomNodes(t *testing.T) {
	nodes := []*nodeState{}
	for i := 0; i < 90; i++ {
		state := StateAlive
		switch i % 3 {
		case 1:
			state = StateSuspect
		case 2:
			state = StateDead
		}
		nodes = append(nodes, &nodeState{
			Node: &Node{
				ID: UniqueID(),
			},
			State: state,
		})
	}
	m := &Memberlist{
		nodes: nodes,
	}
	acceptFn := func(n *nodeState) bool {
		return n.State == StateAlive
	}

	s1 := m.pickRandomNodes(3, acceptFn)
	s2 := m.pickRandomNodes(3, acceptFn)
	s3 := m.pickRandomNodes(3, acceptFn)

	if reflect.DeepEqual(s1, s2) ||
		reflect.DeepEqual(s2, s3) ||
		reflect.DeepEqual(s3, s1) {
		t.Fatalf("unexpected equal %v, %v, %v", s1, s2, s3)
	}

	for _, s := range [][]*nodeState{s1, s2, s3} {
		if len(s) != 3 {
			t.Fatalf("bad length, %d; %v", len(s), s)
		}
		for _, n := range s {
			if n.State != StateAlive {
				t.Fatalf("bad state: %v, %v", n, s)
			}
		}
	}
}

func TestMemberlist_PushPull(t *testing.T) {
	m1, m2, cleanup, err := twoNodesNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	joinAndTest(t, m1, m2)

	for i := 0; i < 3; i++ {
		a := alive{
			Lives: 1,
			ID:    fmt.Sprintf("Test %d", i),
			IP:    net.ParseIP(m1.config.BindAddr),
			Port:  uint16(m1.config.BindPort),
		}
		m1.aliveNode(&a, nil)
	}
	success, msg := retry(15, func() (bool, string) {
		m1.pushPull()
		time.Sleep(10 * time.Millisecond)
		if m2.NumActive() != 5 {
			return false, "expect 5 nodes"
		}
		nodes := m2.ActiveNodes()
		sortNodes(nodes)
		for i := 0; i < 3; i++ {
			if nodes[i+2].ID != fmt.Sprintf("Test %d", i) {
				return false, "wrong node id"
			}
			if nodes[i+2].IP.String() != m1.config.BindAddr {
				return false, "wrong node ip"
			}
			if nodes[i+2].Port != uint16(m1.config.BindPort) {
				return false, "wrong node port"
			}
		}
		return true, ""
	})
	require.True(t, success, msg)
}

func TestMemberlist_Gossip(t *testing.T) {
	m1, m2, m3, cleanup, err := threeNodesNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	node1 := m1.LocalNodeState()
	node2 := m2.LocalNodeState()
	node3 := m3.LocalNodeState()

	for _, n := range []*nodeState{node2, node3} {
		a := alive{
			Lives: 1,
			ID:    n.Node.ID,
			IP:    n.Node.IP,
			Port:  n.Node.Port,
			Tags:  n.Node.Tags,
		}
		m1.aliveNode(&a, nil)
	}

	success, msg := retry(5, func() (bool, string) {
		m1.gossip()
		m1.gossip()
		time.Sleep(10 * time.Millisecond)
		for _, m := range []*Memberlist{m2, m3} {
			if m.NumActive() != 3 {
				return false, "expect 3 active nodes"
			}
			nodes := m.ActiveNodes()
			found := []bool{false, false, false}
			for _, n1 := range nodes {
				for i, n2 := range []*Node{node1.Node, node2.Node, node3.Node} {
					if reflect.DeepEqual(n1, n2) {
						found[i] = true
					}
				}
			}
			for _, ok := range found {
				if !ok {
					return false, "missing node"
				}
			}
		}
		return true, ""
	})

	require.True(t, success, msg)
}

func TestMemberlist_GossipToDead(t *testing.T) {
	m1, m2, cleanup, err := twoNodesNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	node := m2.LocalNodeState()
	node.State = StateDead
	node.StateChange = time.Now().Add(-m1.config.DeadNodeExpiredTimeout - 10*time.Millisecond)

	m1.nodeL.Lock()
	m1.nodes = append(m1.nodes, node)
	m1.nodeMap[node.Node.ID] = node
	m1.nodeL.Unlock()

	m1.gossip()

	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, m2.NumActive())

	m1.nodeL.Lock()
	m1.nodes[1].StateChange = time.Now().Add(-10 * time.Millisecond)
	m1.nodeL.Unlock()

	success, msg := retry(5, func() (bool, string) {
		m1.gossip()
		time.Sleep(50 * time.Millisecond)
		if m2.NumActive() != 2 {
			return false, "expect 2 active nodes"
		}
		nodes := m2.ActiveNodes()
		for _, n := range nodes {
			if n.ID == m2.config.ID && n.IP.String() == m2.config.BindAddr {
				return true, ""
			}
		}
		return false, "node 2 not found"
	})
	require.True(t, success, msg)
}

func TestProbeNode(t *testing.T) {}

func TestMemberlist_Reap(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	for i := 0; i < 3; i++ {
		a := alive{Lives: 1, ID: fmt.Sprintf("test%d", i), IP: []byte{127, 0, 0, byte(i)}, Port: 7946}
		m.aliveNode(&a, nil)
	}

	d := dead{Lives: 1, ID: "test2"}
	m.deadNode(&d, nil)

	m.reap()

	require.Equal(t, 4, m.getNumNodes())

	m.nodeL.Lock()
	m.nodeMap["test2"].StateChange = time.Now().Add(-2 * m.config.DeadNodeExpiredTimeout)
	m.nodeL.Unlock()

	m.reap()
	require.Equal(t, 3, m.getNumNodes())
	require.Nil(t, m.GetNodeState("test2"))
}
