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
		found := make([]bool, 3)
		for i := range found {
			for _, n := range nodes {
				if n.ID == fmt.Sprintf("Test %d", i) {
					if n.IP.String() != m1.config.BindAddr {
						continue
					}
					if n.Port != uint16(m1.config.BindPort) {
						continue
					}
					found[i] = true
				}
			}
		}
		for i, v := range found {
			if !v {
				return false, fmt.Sprintf("not found %d", i)
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
			if n.ID == m2.ID() && n.IP.String() == m2.config.BindAddr {
				return true, ""
			}
		}
		return false, "node 2 not found"
	})
	require.True(t, success, msg)
}

func TestMemberlist_NextProbeNode(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	node, err := m.nextProbeNode() // probeIdx == 1
	require.NotNil(t, err)
	require.Nil(t, node)

	m.nodeL.Lock()
	for i := 0; i < 4; i++ {
		node := nodeState{
			Node: &Node{
				ID:   fmt.Sprintf("test %d", i),
				IP:   []byte{127, 0, 0, byte(i)},
				Port: 7946,
			},
			Lives: 1,
			State: StateAlive,
		}
		m.nodes = append(m.nodes, &node)
		m.nodeMap[node.Node.ID] = &node
	}
	m.nodeMap["test 1"].State = StateDead
	m.nodeMap["test 2"].State = StateSuspect
	m.nodeMap["test 3"].State = StateDead
	m.nodeL.Unlock()

	node, err = m.nextProbeNode() // probeIdx == 3
	require.Nil(t, err)
	require.Equal(t, "test 2", node.Node.ID)

	// wrap around
	node, err = m.nextProbeNode() // probeIdx == 1
	require.Nil(t, err)
	require.Equal(t, "test 0", node.Node.ID)

	node, err = m.nextProbeNode() // probeIdx == 3
	require.Nil(t, err)
	require.Equal(t, "test 2", node.Node.ID)

	m.nodeL.Lock()
	m.nodeMap["test 0"].State = StateLeft
	m.nodeMap["test 2"].State = StateDead
	m.nodeL.Unlock()

	node, err = m.nextProbeNode() // probeIdx == 5
	require.NotNil(t, err)
	require.Nil(t, node)
}

func TestMemberlist_ProbeNode(t *testing.T) {
	m1, m2, cleanup, err := twoNodesNoSchedule()
	m1.config.ProbeInterval = 200 * time.Millisecond // for suspect timeout
	probeTimeMax := m1.config.ProbeInterval + m1.config.MaxRTT + 10*time.Millisecond
	defer cleanup()
	require.Nil(t, err)

	joinAndTest(t, m1, m2)

	node := m1.GetNodeState(m2.ID())
	m1.probeNode(node)

	node = m1.GetNodeState(m2.ID())
	require.Equal(t, StateAlive, node.State)

	m2.Shutdown()

	start := time.Now()
	m1.probeNode(node)
	took := time.Since(start)
	node = m1.GetNodeState(m2.ID())
	require.Equal(t, StateSuspect, node.State)
	require.True(t, took < probeTimeMax)
}

func TestMemberlist_ProbeNode_MissedNacks(t *testing.T) {
	m1, m2, cleanup, err := twoNodesNoSchedule()
	m1.config.ProbeInterval = 200 * time.Millisecond // for suspect timeout
	probeTimeMax := m1.awr.ScaleTimeout(m1.config.ProbeInterval) + m1.config.MaxRTT + 10*time.Millisecond
	defer cleanup()
	require.Nil(t, err)

	joinAndTest(t, m1, m2)

	require.Zero(t, m1.Health())

	m1.nodeL.Lock()
	for i := 0; i < 2; i++ {
		node := nodeState{
			Node: &Node{
				ID:   fmt.Sprintf("test %d", i),
				IP:   []byte{127, 0, 0, byte(i)},
				Port: 7946,
			},
			Lives: 1,
			State: StateAlive,
		}
		m1.nodes = append(m1.nodes, &node)
		m1.nodeMap[node.Node.ID] = &node
	}
	m1.nodeL.Unlock()

	node := m1.GetNodeState("test 0")

	start := time.Now()
	m1.probeNode(node)
	took := time.Since(start)
	require.True(t, took < probeTimeMax)
	require.Equal(t, 1, m1.Health())
}

func TestMemberlist_ProbeNode_HealthImproved(t *testing.T) {
	m1, m2, cleanup, err := twoNodesNoSchedule()
	m1.config.ProbeInterval = 200 * time.Millisecond // for suspect timeout
	defer cleanup()
	require.Nil(t, err)

	joinAndTest(t, m1, m2)

	require.Zero(t, m1.Health())
	m1.awr.Punish(1)
	require.Equal(t, 1, m1.Health())

	node := m1.GetNodeState(m2.ID())
	m1.probeNode(node)

	require.Zero(t, m1.Health())
}

func TestMemberlist_ProbeNode_HealthAlreadyDegraded(t *testing.T) {
	m1, m2, m3, cleanup, err := threeNodesNoSchedule()
	m1.config.ProbeInterval = 200 * time.Millisecond
	defer cleanup()
	require.Nil(t, err)

	addr2 := m2.LocalNodeState().Node.UDPAddress().String()
	addr3 := m3.LocalNodeState().Node.UDPAddress().String()
	n, err := m1.Join([]string{addr2, addr3})
	require.Nil(t, err)
	require.Equal(t, 2, n)

	require.Zero(t, m1.Health())
	m1.awr.Punish(1)
	require.Equal(t, 1, m1.Health())

	probeTimeMin := 2*m1.config.ProbeInterval + m1.config.MaxRTT
	a := alive{
		ID:   "test",
		IP:   []byte{127, 0, 0, 4},
		Port: 7495,
	}
	m1.aliveNode(&a, nil)
	node := m1.GetNodeState("test")
	start := time.Now()
	m1.probeNode(node)
	took := time.Since(start)
	require.True(t, took > probeTimeMin, "probe too quickly")
	node = m1.GetNodeState("test")
	require.Equal(t, StateSuspect, node.State)
	require.Equal(t, 1, m1.Health())
	require.Equal(t, 1, int(m2.pingMng.seqNo)) // indirect ping
	require.Equal(t, 1, int(m3.pingMng.seqNo)) // indirect ping
}

func TestMemberlist_ProbeNode_Buddy(t *testing.T) {
	m1, m2, cleanup, err := twoNodesNoSchedule()
	m1.config.ProbeInterval = 200 * time.Millisecond // for suspect timeout
	defer cleanup()
	require.Nil(t, err)

	joinAndTest(t, m1, m2)

	// fake a suspect
	m1.nodeL.Lock()
	m1.nodeMap[m2.ID()].State = StateSuspect
	m1.nodeL.Unlock()

	node := m1.GetNodeState(m2.ID())
	m1.probeNode(node)

	require.Equal(t, 1, m2.Health()) // should be punished
	node = m2.LocalNodeState()
	require.Equal(t, 2, int(node.Lives))

	success, msg := retry(5, func() (bool, string) {
		m2.gossip()
		time.Sleep(10 * time.Millisecond)
		node = m1.GetNodeState(m2.ID())
		if node.State != StateAlive {
			return false, "wrong state"
		}
		if node.Lives != 2 {
			return false, "wrong lives"
		}
		return true, ""
	})
	require.True(t, success, msg)
}

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
