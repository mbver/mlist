package memberlist

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockUserStateDelegate struct {
	local string
}

func (u *mockUserStateDelegate) LocalState() []byte {
	return []byte(u.local)
}

func (u *mockUserStateDelegate) Merge(s []byte) {
	u.local += string(s)
}

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

	require.Equal(t, 5, m.GetNumNodes())
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
	m, cleanup1, err := newTestMemberlistNoSchedule()
	defer cleanup1()
	require.Nil(t, err)

	m.usrState = &mockUserStateDelegate{
		local: m.config.ID,
	}

	addr := m.LocalNodeState().Node.UDPAddress().String()

	// fake local state to send to m1
	localNodes2 := make([]stateToMerge, 3)
	for i := 0; i < 3; i++ {
		localNodes2[i].ID = fmt.Sprintf("Test %d", i)
		localNodes2[i].IP = net.ParseIP(m.config.BindAddr)
		localNodes2[i].Port = uint16(m.config.BindPort)
		localNodes2[i].Lives = 1
		localNodes2[i].State = StateAlive
	}

	userState := []byte("user state")
	encoded, err := encodePushPullMsg(localNodes2, userState)
	require.Nil(t, err)

	timeout := 2 * time.Second
	conn, err := m.transport.DialTimeout(addr, timeout)
	require.Nil(t, err)
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))
	m.sendTcp(conn, encoded, m.config.Label)

	msgType, bufConn, dec, err := m.unpackStream(conn, m.config.Label)
	require.Nil(t, err)
	require.Equal(t, pushPullMsg, msgType)

	remoteNodes, remoteUserState, err := m.readRemoteState(bufConn, dec)
	require.Nil(t, err)
	require.Equal(t, 1, len(remoteNodes))

	node := remoteNodes[0]
	require.Equal(t, node.ID, m.ID())
	if node.IP.String() != m.config.BindAddr {
		t.Fatalf("unmatched ip: expect: %s, got: %s", m.config.BindAddr, node.IP)
	}
	require.Equal(t, node.Port, uint16(m.config.BindPort))

	// user-states is exchanged
	require.Equal(t, m.config.ID, string(remoteUserState))
	require.Contains(t, string(m.usrState.LocalState()), string(userState))

	time.Sleep(10 * time.Millisecond) // wait for m1 merge state
	require.Equal(t, 4, m.NumActive())
	nodes := m.ActiveNodes()
	found := make([]bool, 3)
	for i := range found {
		for _, n := range nodes {
			if n.ID == fmt.Sprintf("Test %d", i) {
				if n.IP.String() != m.config.BindAddr {
					continue
				}
				if n.Port != uint16(m.config.BindPort) {
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
