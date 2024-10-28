package memberlist

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPushPull_MergeState(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)
	m.config.ProbeInterval = 2 * time.Second // used to compute suspicion timeout, not for scheduling
	for i := 0; i < 3; i++ {
		a := alive{
			Lives: 1,
			ID:    fmt.Sprintf("test%d", i),
			IP:    []byte{127, 0, 0, byte(i)},
			Port:  7946,
		}
		m.aliveNode(&a, nil)
	}

	s := suspect{Lives: 1, ID: "test0"}
	m.suspectNode(&s)

	remote := make([]stateToMerge, 4)
	for i := 0; i < 4; i++ {
		remote[i] = stateToMerge{
			Lives: 1,
			ID:    fmt.Sprintf("test%d", i),
			IP:    []byte{127, 0, 0, byte(i)},
			Port:  7946,
			State: StateAlive,
		}
	}
	remote[0].Lives = 2
	remote[1].State = StateSuspect
	remote[2].State = StateDead
	remote[3].Lives = 2

	m.mergeState(remote)

	require.Equal(t, 5, m.getNumNodes())
	node := m.GetNodeState("test0")
	if node.State != StateAlive || node.Lives != 2 {
		t.Fatalf("bad node %+v", node)
	}

	node = m.GetNodeState("test1")
	if node.State != StateSuspect || node.Lives != 1 {
		t.Fatalf("bad node %+v", node)
	}

	node = m.GetNodeState("test2")
	if node.State != StateSuspect || node.Lives != 1 {
		t.Fatalf("bad node %+v", node)
	}

	node = m.GetNodeState("test3")
	if node.State != StateAlive || node.Lives != 2 {
		t.Fatalf("bad node %+v", node)
	}
}

func TestPushPull_SendReceive(t *testing.T) {
	m1, cleanup1, err := newTestMemberlistNoSchedule()
	defer cleanup1()
	require.Nil(t, err)
	addr := m1.LocalNodeState().Node.UDPAddress().String()

	m2, err := newPackTestMemberlist()
	require.Nil(t, err)

	// fake local state to send to m1
	localNodes2 := make([]stateToMerge, 3)
	for i := 0; i < 3; i++ {
		localNodes2[i].ID = fmt.Sprintf("Test %d", i)
		localNodes2[i].IP = net.ParseIP(m1.config.BindAddr)
		localNodes2[i].Port = uint16(m1.config.BindPort)
		localNodes2[i].Lives = 1
		localNodes2[i].State = StateAlive
	}

	encoded, err := encodePushPullMsg(localNodes2)
	require.Nil(t, err)

	timeout := 2 * time.Second
	conn, err := m2.transport.DialTimeout(addr, timeout)
	require.Nil(t, err)
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))
	m2.sendTcp(conn, encoded, m2.config.Label)

	msgType, bufConn, dec, err := m2.unpackStream(conn, m2.config.Label)
	require.Nil(t, err)
	require.Equal(t, pushPullMsg, msgType)

	remoteNodes, err := m2.readRemoteState(bufConn, dec)
	require.Nil(t, err)
	require.Equal(t, 1, len(remoteNodes))

	node := remoteNodes[0]
	require.Equal(t, node.ID, m1.ID())
	if node.IP.String() != m1.config.BindAddr {
		t.Fatalf("unmatched ip: expect: %s, got: %s", m1.config.BindAddr, node.IP)
	}
	require.Equal(t, node.Port, uint16(m1.config.BindPort))

	require.Equal(t, 4, m1.NumActive())
	nodes := m1.ActiveNodes()
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
		require.True(t, v, fmt.Sprintf("not found %d", i))
	}
}
