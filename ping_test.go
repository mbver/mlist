package memberlist

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestPingMemberlist() (*Memberlist, func(), error) {
	conf := defaultTestConfig()
	// deactive gossip, probe and pushpull scheduler
	// deactive
	// so they don't interfere with the ping test
	conf.GossipInterval = 0
	conf.ProbeInterval = 0
	conf.PushPullInterval = 0
	conf.RetransmitMult = 0
	return newTestMemberlist(nil, 0, conf)
}

func threePingTestNodes() (*Memberlist, *Memberlist, *Memberlist, func(), error) {
	m1, cleanup1, err := newTestPingMemberlist()
	if err != nil {
		return nil, nil, nil, cleanup1, err
	}
	m2, cleanup2, err := newTestPingMemberlist()
	if err != nil {
		return nil, nil, nil, getCleanup(cleanup1, cleanup2), err
	}
	m3, cleanup3, err := newTestPingMemberlist()
	cleanup := getCleanup(cleanup1, cleanup2, cleanup3)
	if err != nil {
		return nil, nil, nil, cleanup, err
	}
	return m1, m2, m3, cleanup, nil
}

func retry(fn func() (bool, string)) (success bool, msg string) {
	for i := 0; i < 5; i++ {
		success, msg = fn()
		if success {
			return
		}
	}
	return
}

func TestPing(t *testing.T) {
	m1, m2, m3, cleanup, err := threePingTestNodes()
	defer cleanup()
	require.Nil(t, err)

	pingTimeout := 10 * time.Millisecond
	probeTimeout := 50 * time.Millisecond

	node2 := m2.LocalNodeState()
	if !m1.Ping(node2, pingTimeout) {
		t.Fatalf("failed ping")
	}
	node3 := m3.LocalNodeState()

	m1.nodeL.Lock()
	m1.nodes = append(m1.nodes, node2)
	m1.nodeMap[node2.Node.ID] = node2
	m1.nodes = append(m1.nodes, node3)
	m1.nodeMap[node3.Node.ID] = node3
	m1.nodeL.Unlock()

	timeout := probeTimeout - pingTimeout

	// pick random node is very likely to fail if
	// we only have 3 nodes in the list. retry once again.
	success, msg := retry(func() (bool, string) {
		resCh := m1.IndirectPing(node2, timeout)
		select {
		case res := <-resCh:
			if !res.success {
				return false, fmt.Sprintf("indirect ping failed %+v", res)
			}
			if res.nNacks != 0 || res.numNode != 0 {
				return false, fmt.Sprintf("expect zero numNodes and nacks, got: %d, %d", res.numNode, res.nNacks)
			}
			return true, ""
		case <-time.After(timeout + 10*time.Millisecond):
			return false, "indirect ping timeout"
		}
	})
	require.True(t, success, msg)

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

	success, msg = retry(func() (bool, string) {
		resCh := m1.IndirectPing(node2, timeout)
		select {
		case res := <-resCh:
			if res.success {
				return false, "expect indirect ping failed"
			}
			if res.nNacks != 1 || res.numNode != 1 {
				return false, fmt.Sprintf("expect 1 node and 1 nack, got: %d, %d", res.numNode, res.nNacks)
			}
			return true, ""
		case <-time.After(timeout + 10*time.Millisecond):
			return false, "indirect ping timeout"
		}
	})
	require.True(t, success, msg)

	conn, _ = m3.transport.getFirstConn()
	conn.Close()

	success, msg = retry(func() (bool, string) {
		resCh := m1.IndirectPing(node2, timeout)
		select {
		case res := <-resCh:
			if res.success {
				return false, "expect indirect ping failed"
			}
			if res.nNacks != 0 || res.numNode != 1 {
				return false, fmt.Sprintf("expect 1 node and 0 nack, got: %d, %d", res.numNode, res.nNacks)
			}
			return true, ""
		case <-time.After(timeout + 10*time.Millisecond):
			return false, "indirect ping timeout"
		}
	})
	require.True(t, success, msg)
}
