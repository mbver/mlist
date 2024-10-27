package memberlist

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPushPull(t *testing.T) {
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
	require.Equal(t, node.ID, m1.config.ID)
	if node.IP.String() != m1.config.BindAddr {
		t.Fatalf("unmatched ip: expect: %s, got: %s", m1.config.BindAddr, node.IP)
	}
	require.Equal(t, node.Port, uint16(m1.config.BindPort))

	require.Equal(t, 4, m1.NumActive())
	nodes := m1.ActiveNodes()
	sortNodes(nodes)
	for i := 0; i < 3; i++ {
		require.Equal(t, fmt.Sprintf("Test %d", i), nodes[i+1].ID)
		require.Equal(t, m1.config.BindAddr, nodes[i+1].IP.String())
		require.Equal(t, m1.config.BindPort, int(nodes[i+1].Port))
	}
}
