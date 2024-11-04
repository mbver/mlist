package memberlist

import (
	"fmt"
	"net"
	"reflect"
	"sync/atomic"
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

	m1.usrState = &mockUserStateDelegate{m1.config.ID}
	m2.usrState = &mockUserStateDelegate{m2.config.ID}
	joinAndTest(t, m1, m2)

	// user state exchanged
	require.Contains(t, string(m1.usrState.LocalState()), m2.config.ID)
	require.Contains(t, string(m2.usrState.LocalState()), m1.config.ID)

	for i := 0; i < 3; i++ {
		a := alive{
			Lives: 1,
			ID:    fmt.Sprintf("Test %d", i),
			IP:    net.ParseIP(m1.config.BindAddr),
			Port:  uint16(m1.config.BindPort),
		}
		m1.aliveNode(&a, nil)
	}
	// the chance of node2 is not chosen for pushpull is 0.75
	// after 15 times, the the chance of still missing is 1.3%
	// after 20 times -> 0.3%
	// after 25 times -> 0.075%
	success, msg := retry(25, func() (bool, string) {
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
		if len(m1.usrState.LocalState()) < 4*len(m1.config.ID) {
			return false, "user state is not exchanged"
		}
		if len(m2.usrState.LocalState()) < 4*len(m1.config.ID) {
			return false, "user state is not exchanged"
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
			if n.ID == m1.ID() && n.IP.String() == m1.config.BindAddr {
				return true, ""
			}
		}
		return false, "node 1 not found"
	})
	require.True(t, success, msg)
}

func TestMemberlist_Probe(t *testing.T) {
	m1, m2, cleanup, err := twoNodesNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	joinAndTest(t, m1, m2)

	m1.probe()

	node := m1.GetNodeState(m2.config.ID)
	require.Equal(t, StateAlive, node.State)

	seq := atomic.LoadUint32(&m1.pingMng.seqNo)
	require.Equal(t, 1, int(seq))
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

func TestMemberlist_ProbeNode_Suspect(t *testing.T) {
	m1, m2, m3, cleanup, err := threeNodesNoSchedule()
	m1.config.ProbeInterval = 200 * time.Millisecond
	defer cleanup()
	require.Nil(t, err)

	addr2 := m2.LocalNodeState().Node.UDPAddress().String()
	addr3 := m3.LocalNodeState().Node.UDPAddress().String()
	n, err := m1.Join([]string{addr2, addr3})
	require.Nil(t, err)
	require.Equal(t, 2, n)

	a := alive{
		ID:   "test",
		IP:   []byte{127, 0, 0, 4},
		Port: 7495,
	}
	m1.aliveNode(&a, nil)
	node := m1.GetNodeState("test")

	m1.probeNode(node)

	node = m1.GetNodeState("test")
	require.Equal(t, StateSuspect, node.State)

	seq2 := atomic.LoadUint32(&m2.pingMng.seqNo)
	seq3 := atomic.LoadUint32(&m3.pingMng.seqNo)
	require.True(t, seq2 == 1 || seq3 == 1, "at least one indirect ping")
}

func TestMemberlist_ProbeNode_Suspect_Dogpile(t *testing.T) {
	cases := []struct {
		name      string
		npeers    int
		nconfirms int
		expected  time.Duration
	}{
		{"1 peer, no confirms", 1, 0, 500 * time.Millisecond},
		{"2 peers, no confirms", 2, 0, 500 * time.Millisecond},
		{"3 peers, no confirms", 3, 0, 500 * time.Millisecond},
		{"4 peers, no confirms", 4, 0, 1000 * time.Millisecond},
		{"5 peers, no confirms", 5, 0, 1000 * time.Millisecond},
		{"5 peers, 1 confirm", 5, 1, 750 * time.Millisecond},
		{"5 peers, 2 confirms", 5, 2, 604 * time.Millisecond},
		{"5 peers, 3 confirms", 5, 3, 500 * time.Millisecond},
		{"5 peers, 4 confirms", 5, 4, 500 * time.Millisecond},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, cleanup, err := newTestMemberlistNoSchedule()
			defer cleanup()
			require.Nil(t, err)

			m.config.ProbeInterval = 100 * time.Millisecond
			m.config.SuspicionMult = 5
			m.config.SuspicionMaxTimeoutMult = 2

			for i := 0; i < c.npeers-1; i++ {
				a := alive{
					Lives: 1,
					ID:    fmt.Sprintf("test %d", i),
					IP:    []byte{127, 0, 0, byte(i)},
					Port:  1234,
				}
				m.aliveNode(&a, nil)
			}

			// add the bad peer
			a := alive{1, "bad", []byte{127, 0, 0, 5}, 7891, nil}
			m.aliveNode(&a, nil)
			require.Equal(t, c.npeers+1, m.NumActive())

			node := m.GetNodeState("bad")
			m.probeNode(node)

			node = m.GetNodeState("bad")
			require.Equal(t, StateSuspect, node.State)

			for i := 0; i < c.nconfirms; i++ {
				s := suspect{Lives: 1, ID: "bad", From: fmt.Sprintf("test %d", i)}
				m.suspectNode(&s)
			}

			epsilon := 25 * time.Millisecond
			time.Sleep(c.expected - epsilon)

			node = m.GetNodeState("bad")
			require.Equal(t, StateSuspect, node.State)

			time.Sleep(2 * epsilon)
			node = m.GetNodeState("bad")
			require.Equal(t, StateDead, node.State)
		})
	}
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

	success, msg := retry(5, func() (bool, string) {
		if m1.Health() == 1 {
			return true, ""
		}
		// the bad node may not be included for indirectping. do it again.
		m1.probeNode(node)
		return false, "no missed nack"
	})
	require.True(t, success, msg)
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

	seq2 := atomic.LoadUint32(&m2.pingMng.seqNo)
	seq3 := atomic.LoadUint32(&m3.pingMng.seqNo)
	require.True(t, seq2 == 1 || seq3 == 1, "at least one indirect ping")
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

	require.Equal(t, 4, m.GetNumNodes())

	m.nodeL.Lock()
	m.nodeMap["test2"].StateChange = time.Now().Add(-2 * m.config.DeadNodeExpiredTimeout)
	m.nodeL.Unlock()

	m.reap()
	require.Equal(t, 3, m.GetNumNodes())
	require.Nil(t, m.GetNodeState("test2"))
}
