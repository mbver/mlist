package memberlist

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestPingMemberlist() (*Memberlist, func(), error) {
	m, err := newPackTestMemberlist()
	if err != nil {
		return m, nil, err
	}

	id := UniqueID()
	m.config = DefaultLANConfig() // change config
	m.config.ID = id
	m.config.Label = "label"
	tr, cleanup, err := newTestTransport()
	if err != nil {
		return nil, cleanup, err
	}

	m.transport = tr
	m.finalizeAdvertiseAddr()

	m.pingMng = newPingManager(nil)

	m.shutdownCh = make(chan struct{})
	m.stopScheduleCh = make(chan struct{})

	ip, port, err := m.GetAdvertiseAddr()
	if err != nil {
		return m, cleanup, err
	}

	node := &nodeState{
		Node: &Node{
			ID:   id,
			IP:   ip,
			Port: port,
		},
		Lives: 1,
		State: StateAlive,
	}
	m.nodes = append(m.nodes, node)
	m.nodeMap = map[string]*nodeState{
		id: node,
	}
	m.awr = newAwareness(8)
	m.mbroadcasts = NewBroadcastQueue(m.getNumNodes, 4)
	m.logger = log.New(os.Stderr, "", log.LstdFlags)
	go m.receivePacket()
	go m.receiveTcpConn()

	cleanup1 := func() {
		m.Shutdown()
		cleanup()
	}
	return m, cleanup1, nil
}

func TestPing_DirectIndirectTcp(t *testing.T) {
	m1, cleanup1, err := newTestPingMemberlist()
	defer func() {
		if cleanup1 != nil {
			cleanup1()
		}
	}()
	require.Nil(t, err)

	pingTimeout := m1.config.PingTimeout
	probeTimeout := m1.config.ProbeInterval

	m2, cleanup2, err := newTestPingMemberlist()
	defer func() {
		if cleanup2 != nil {
			cleanup2()
		}
	}()
	require.Nil(t, err)

	m3, cleanup3, err := newTestPingMemberlist()
	defer func() {
		if cleanup3 != nil {
			cleanup3()
		}
	}()
	require.Nil(t, err)

	node2 := m2.LocalNodeState()
	if !m1.Ping(node2, pingTimeout) {
		t.Fatalf("failed ping")
	}

	node3 := m3.LocalNodeState()
	m1.nodes = append(m1.nodes, node3)
	m1.nodeMap[node3.Node.ID] = node3

	timeout := probeTimeout - pingTimeout
	resCh := m1.IndirectPing(node2, timeout)
	select {
	case res := <-resCh:
		if !res.success {
			t.Fatalf("indirect ping failed %v", res)
		}
		require.Zero(t, res.nNacks)
		require.Zero(t, res.numNode)
	case <-time.After(timeout + 10*time.Millisecond):
		t.Fatalf("expect no timeout in indirect ping")
	}

	tcpCh := m1.TcpPing(node2, timeout)
	select {
	case res := <-tcpCh:
		if !res {
			t.Fatalf("tcp ping failed")
		}
	case <-time.After(timeout + 10*time.Millisecond):
		t.Fatalf("expect no timeout")
	}

	conn, _ := m2.transport.getFirstConn()
	conn.Close()

	resCh = m1.IndirectPing(node2, timeout)
	select {
	case res := <-resCh:
		require.False(t, res.success, "expect fail indirect ping")
		require.Equal(t, res.nNacks, 1, "expect 1 nack")
		require.Equal(t, res.numNode, 1, "expect 1 node")
	case <-time.After(timeout + 10*time.Millisecond):
		t.Fatalf("expect no timeout")
	}

	conn, _ = m3.transport.getFirstConn()
	conn.Close()
	resCh = m1.IndirectPing(node2, timeout)
	select {
	case res := <-resCh:
		require.False(t, res.success, "expect fail indirect ping")
		require.Equal(t, res.nNacks, 0, "expect missed nack")
		require.Equal(t, res.numNode, 1, "expect 1 node")
	case <-time.After(timeout + 10*time.Millisecond):
		t.Fatalf("expect no timeout")
	}
}
