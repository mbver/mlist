package memberlist

import (
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
	m.config = DefaultLANConfig(id) // change config
	m.config.Label = "label"
	tr, cleanup, err := newTestTransport()
	if err != nil {
		return nil, cleanup, err
	}

	m.transport = tr
	m.finalizeAdvertiseAddr()

	m.pingMng = newPingManager()

	m.shutdownCh = make(chan struct{})

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
	if !m1.Ping(node2) {
		t.Fatalf("failed ping")
	}

	node3 := m3.LocalNodeState()
	m1.nodes = append(m1.nodes, node3)
	m1.nodeMap[node3.Node.ID] = node3

	resCh := m1.IndirectPing(node2, m1.config.PingTimeout)
	select {
	case res := <-resCh:
		if !res.success {
			t.Fatalf("indirect ping failed %v", res)
		}
	case <-time.After(m1.config.PingTimeout):
		t.Fatalf("expect no timeout in indirect ping")
	}

	tcpCh := m1.TcpPing(node2, m1.config.PingTimeout)
	select {
	case res := <-tcpCh:
		if !res {
			t.Fatalf("tcp ping failed")
		}
	case <-time.After(m1.config.PingTimeout):
		t.Fatalf("expect no timeout")
	}

	conn, _ := m2.transport.GetFirstConn()
	conn.Close()

	timeout := m1.config.ProbeInterval - m1.config.PingTimeout // 2 * PingTimeout
	resCh = m1.IndirectPing(node2, timeout)
	select {
	case res := <-resCh:
		require.False(t, res.success, "expect fail indirect ping")
		require.Equal(t, res.nNacks, 1, "expect nack")
	case <-time.After(timeout + 10*time.Millisecond): // allow some extra time
		t.Fatalf("expect no timeout in indirect ping")
	}
}
