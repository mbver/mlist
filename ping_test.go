package memberlist

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ackHandlerExists(m *pingManager, seqNo uint32) bool {
	m.l.Lock()
	defer m.l.Unlock()
	_, ok := m.ackHandlers[seqNo]
	return ok
}

func indirectAckHandlerExists(m *pingManager, seqNo uint32) bool {
	m.l.Lock()
	defer m.l.Unlock()
	_, ok := m.indirectAckHandlers[seqNo]
	return ok
}
func TestPing_SetAckHandler(t *testing.T) {
	m := newPingManager(nil)
	m.setAckHandler(1, nil, 10*time.Millisecond)
	require.True(t, ackHandlerExists(m, 1))
	time.Sleep(15 * time.Millisecond)
	require.False(t, ackHandlerExists(m, 1))
}

func TestPing_SetIndirectAckHandler(t *testing.T) {
	m := newPingManager(nil)
	m.setIndirectAckHandler(1, nil, nil, 10*time.Millisecond)
	require.True(t, indirectAckHandlerExists(m, 1))
	time.Sleep(15 * time.Millisecond)
	require.False(t, indirectAckHandlerExists(m, 1))
}

func TestPing_InvokeAckHandler(t *testing.T) {
	m := newPingManager(nil)

	m.invokeAckHandler(ack{}, time.Now()) // does nothing

	ch := make(chan timedAck, 1)
	m.setAckHandler(1, ch, 10*time.Millisecond)

	ts := time.Now()
	p := []byte("payload")
	m.invokeAckHandler(ack{1, p}, ts)
	require.Equal(t, 1, len(ch))
	a := <-ch
	require.Equal(t, ts, a.timestamp)
	require.True(t, bytes.Equal(a.payload, p))
}

func TestPing_InvokeIndirectAckHandler(t *testing.T) {
	m := newPingManager(nil)
	ch := make(chan struct{}, 1)
	var nack int32

	m.setIndirectAckHandler(1, ch, &nack, 10*time.Millisecond)
	m.invokeIndirectAckHandler(indirectAck{1, false}) // nack
	m.invokeIndirectAckHandler(indirectAck{1, true})  // success

	require.Equal(t, 1, int(nack))
	require.Equal(t, 1, len(ch))
}

func newTestMemberlistNoSchedule() (*Memberlist, func(), error) {
	conf := defaultTestConfig()
	// deactive gossip, probe and pushpull scheduler
	// so they don't interfere with the ping test
	conf.GossipInterval = 0
	conf.ProbeInterval = 0
	conf.PushPullInterval = 0
	conf.ReapInterval = 0
	return newTestMemberlist(nil, 0, conf)
}

func twoNodesNoSchedule() (*Memberlist, *Memberlist, func(), error) {
	m1, cleanup1, err := newTestMemberlistNoSchedule()
	if err != nil {
		return nil, nil, cleanup1, err
	}
	m2, cleanup2, err := newTestMemberlistNoSchedule()
	cleanup := getCleanup(cleanup1, cleanup2)
	if err != nil {
		return nil, nil, cleanup, err
	}
	return m1, m2, cleanup, nil
}

func threeNodesNoSchedule() (*Memberlist, *Memberlist, *Memberlist, func(), error) {
	m1, m2, cleanup1, err := twoNodesNoSchedule()
	if err != nil {
		return nil, nil, nil, cleanup1, err
	}
	m3, cleanup3, err := newTestMemberlistNoSchedule()
	cleanup := getCleanup(cleanup1, cleanup3)
	if err != nil {
		return nil, nil, nil, cleanup, err
	}
	return m1, m2, m3, cleanup, nil
}

func retry(times int, fn func() (bool, string)) (success bool, msg string) {
	for i := 0; i < times; i++ {
		success, msg = fn()
		if success {
			return
		}
	}
	return
}

func TestPing(t *testing.T) {
	m1, m2, m3, cleanup, err := threeNodesNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	node2 := m2.LocalNodeState()
	node3 := m3.LocalNodeState()

	m1.nodeL.Lock()
	m1.nodes = append(m1.nodes, node2)
	m1.nodeMap[node2.Node.ID] = node2
	m1.nodes = append(m1.nodes, node3)
	m1.nodeMap[node3.Node.ID] = node3
	m1.nodeL.Unlock()
	pingTimeout := 10 * time.Millisecond
	probeTimeout := 50 * time.Millisecond

	if !m1.Ping(node2, pingTimeout) {
		t.Fatalf("failed ping")
	}

	timeout := probeTimeout - pingTimeout

	// pick random node is very likely to fail if
	// we only have 3 nodes in the list. retry once again.
	success, msg := retry(5, func() (bool, string) {
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

	success, msg = retry(5, func() (bool, string) {
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

	success, msg = retry(5, func() (bool, string) {
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
