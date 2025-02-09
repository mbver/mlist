// Copyright (c) HashiCorp, Inc.
// Copyright (c) 2024 Phuoc Phi
// SPDX-License-Identifier: MPL-2.0
package memberlist

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHandleUDP_ZeroLengthMsg(t *testing.T) {
	var buf bytes.Buffer
	m := Memberlist{
		logger: log.New(&buf, "", 0),
	}
	m.handleUdpMsg(nil, nil, time.Now())
	require.Contains(t, buf.String(), "missing message type byte")
}

func listenUDP(t *testing.T) *net.UDPConn {
	var udp *net.UDPConn
	for port := 60000; port < 61000; port++ {
		udpAddr := fmt.Sprintf("127.0.0.1:%d", port)
		udpLn, err := net.ListenPacket("udp", udpAddr)
		if err == nil {
			udp = udpLn.(*net.UDPConn)
			break
		}
	}
	if udp == nil {
		t.Fatalf("no udp listener")
	}
	return udp
}

func cleanQueue(q *TransmitCapQueue) {
	for q.Len() != 0 {
		q.Pop()
	}
}

func TestHandlePing(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	cleanQueue(m.mbroadcasts) // don't let the piggyback mess with ping response

	udp := listenUDP(t)
	defer udp.Close()
	udpAddr := udp.LocalAddr().(*net.UDPAddr)

	p := ping{
		SeqNo:      42,
		ID:         m.config.ID,
		SourceIP:   udpAddr.IP,
		SourcePort: uint16(udpAddr.Port),
	}

	encoded, err := encode(pingMsg, p)
	require.Nil(t, err)
	packed, err := m.packUdp(encoded)
	require.Nil(t, err)

	addr := m.LocalNodeState().Node.UDPAddress()
	_, err = udp.WriteTo(packed, addr)
	require.Nil(t, err)

	in := make([]byte, 1400)
	udp.SetDeadline(time.Now().Add(50 * time.Millisecond))
	n, _, err := udp.ReadFrom(in)
	require.Nil(t, err)

	unpacked, err := m.unpackPacket(in[:n], m.config.Label)
	require.Nil(t, err)
	require.Equal(t, ackMsg, msgType(unpacked[0]))

	var a ack
	err = decode(unpacked[1:], &a)
	require.Nil(t, err)
	require.Equal(t, 42, int(a.SeqNo))
}

func TestHandleCompoundPing(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	cleanQueue(m.mbroadcasts) // don't let the piggyback mess with ping response

	udp := listenUDP(t)
	defer udp.Close()
	udpAddr := udp.LocalAddr().(*net.UDPAddr)

	p := ping{
		SeqNo:      42,
		ID:         m.config.ID,
		SourceIP:   udpAddr.IP,
		SourcePort: uint16(udpAddr.Port),
	}

	encoded, err := encode(pingMsg, p)
	require.Nil(t, err)
	compound := packCompoundMsg([][]byte{encoded, encoded, encoded})

	packed, err := m.packUdp(compound)
	require.Nil(t, err)

	addr := m.LocalNodeState().Node.UDPAddress()
	_, err = udp.WriteTo(packed, addr)
	require.Nil(t, err)

	in := make([]byte, 1400)
	for i := 0; i < 3; i++ {
		udp.SetDeadline(time.Now().Add(50 * time.Millisecond))
		n, _, err := udp.ReadFrom(in)
		require.Nil(t, err)
		unpacked, err := m.unpackPacket(in[:n], m.config.Label)
		require.Nil(t, err)
		require.Equal(t, ackMsg, msgType(unpacked[0]))

		var a ack
		err = decode(unpacked[1:], &a)
		require.Nil(t, err)
		require.Equal(t, 42, int(a.SeqNo))
	}
}

func TestHandlePing_Piggyback(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	udp := listenUDP(t)
	defer udp.Close()
	udpAddr := udp.LocalAddr().(*net.UDPAddr)

	p := ping{
		SeqNo:      42,
		ID:         m.config.ID,
		SourceIP:   udpAddr.IP,
		SourcePort: uint16(udpAddr.Port),
	}

	encoded, err := encode(pingMsg, p)
	require.Nil(t, err)
	packed, err := m.packUdp(encoded)
	require.Nil(t, err)

	addr := m.LocalNodeState().Node.UDPAddress()
	_, err = udp.WriteTo(packed, addr)
	require.Nil(t, err)

	in := make([]byte, 1400)
	udp.SetDeadline(time.Now().Add(50 * time.Millisecond))
	n, _, err := udp.ReadFrom(in)
	require.Nil(t, err)

	unpacked, err := m.unpackPacket(in[:n], m.config.Label)
	require.Nil(t, err)
	require.Equal(t, compoundMsg, msgType(unpacked[0]))

	trunc, msgs, err := unpackCompoundMsg(unpacked[1:])
	require.Nil(t, err)
	require.Zero(t, trunc)
	require.Equal(t, 2, len(msgs))

	require.Equal(t, ackMsg, msgType(msgs[0][0]))
	var a ack
	err = decode(msgs[0][1:], &a)
	require.Nil(t, err)
	require.Equal(t, 42, int(a.SeqNo))

	require.Equal(t, aliveMsg, msgType(msgs[1][0]))
	var al alive
	err = decode(msgs[1][1:], &al)
	require.Nil(t, err)
	require.Equal(t, m.config.ID, al.ID)
}

func TestHandlePing_WrongNode(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	cleanQueue(m.mbroadcasts) // don't let the piggyback mess with ping response

	udp := listenUDP(t)
	defer udp.Close()
	udpAddr := udp.LocalAddr().(*net.UDPAddr)

	p := ping{
		SeqNo:      42,
		ID:         "bad",
		SourceIP:   udpAddr.IP,
		SourcePort: uint16(udpAddr.Port),
	}

	encoded, err := encode(pingMsg, p)
	require.Nil(t, err)
	packed, err := m.packUdp(encoded)
	require.Nil(t, err)

	addr := m.LocalNodeState().Node.UDPAddress()
	_, err = udp.WriteTo(packed, addr)
	require.Nil(t, err)

	in := make([]byte, 1400)
	udp.SetDeadline(time.Now().Add(50 * time.Millisecond))
	_, _, err = udp.ReadFrom(in)
	require.NotNil(t, err, "should not have response")
}

func TestHandleIndirectPing(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	cleanQueue(m.mbroadcasts) // don't let the piggyback mess with ping response

	udp := listenUDP(t)
	defer udp.Close()
	udpAddr := udp.LocalAddr().(*net.UDPAddr)

	addr := m.LocalNodeState().Node.UDPAddress()

	ind := indirectPing{
		SeqNo:      42,
		ID:         m.config.ID,
		IP:         addr.IP,
		Port:       uint16(addr.Port),
		SourceIP:   udpAddr.IP,
		SourcePort: uint16(udpAddr.Port),
	}

	encoded, err := encode(indirectPingMsg, ind)
	require.Nil(t, err)
	packed, err := m.packUdp(encoded)
	require.Nil(t, err)

	_, err = udp.WriteTo(packed, addr)
	require.Nil(t, err)

	in := make([]byte, 1400)
	udp.SetDeadline(time.Now().Add(1 * time.Second))
	n, _, err := udp.ReadFrom(in)
	require.Nil(t, err)

	unpacked, err := m.unpackPacket(in[:n], m.config.Label)
	require.Nil(t, err)
	require.Equal(t, indirectAckMsg, msgType(unpacked[0]))

	var a indirectAck
	err = decode(unpacked[1:], &a)
	require.Nil(t, err)
	require.Equal(t, 42, int(a.SeqNo))
}

func TestHandlePingTCP(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	addr := m.LocalNodeState().Node.UDPAddress().String()
	timeout := 2 * time.Second
	conn, err := m.transport.DialTimeout(addr, timeout)
	require.Nil(t, err)
	defer conn.Close()

	p := ping{
		SeqNo: 42,
		ID:    m.config.ID,
	}
	encoded, err := encode(pingMsg, p)
	require.Nil(t, err)

	conn.SetDeadline(time.Now().Add(timeout))
	m.sendTcp(conn, encoded, m.config.Label)

	mType, _, dec, err := m.unpackStream(conn, m.config.Label)
	require.Nil(t, err)
	require.Equal(t, ackMsg, mType)

	var a ack
	err = dec.Decode(&a)
	require.Nil(t, err)
	require.Equal(t, 42, int(a.SeqNo))
}
