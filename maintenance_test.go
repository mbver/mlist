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
	m1, cleanup1, err := newTestMemberlistNoSchedule()
	defer cleanup1()
	require.Nil(t, err)

	m2, cleanup2, err := newTestMemberlistNoSchedule()
	defer cleanup2()
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
	success, msg := retry(func() (bool, string) {
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

func TestProbeNode(t *testing.T) {}
