package memberlist

import (
	"bytes"
	"io"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/go-msgpack/v2/codec"
	"github.com/mbver/mlist/testaddr"
	"github.com/stretchr/testify/require"
)

func newPackTestMemberlist() (*Memberlist, error) {
	key := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	keyRing, err := NewKeyring(nil, key)
	if err != nil {
		return nil, err
	}
	conf := &Config{
		Label:             "label",
		EnableCompression: true,
	}
	return &Memberlist{
		keyring: keyRing,
		config:  conf,
	}, nil
}

func TestNetPacking_PackUnpackUDP(t *testing.T) {
	m, err := newPackTestMemberlist()
	require.Nil(t, err, "setup memberlist failed")
	msg := []byte("this is a message")
	label := "label"

	packed, err := m.packUdp(msg)
	require.Nil(t, err, "pack msg failed")

	unpacked, err := m.unpackPacket(packed, label)
	require.Nil(t, err, "unpack msg failed")

	if !bytes.Equal(unpacked, msg) {
		t.Fatalf("unmatched pack-unpack result. expect: %s, got: %s", msg, unpacked)
	}
}

// MockConn implements net.Conn interface
type MockConn struct {
	buf bytes.Buffer
}

func NewMockConn() *MockConn {
	return &MockConn{
		buf: *bytes.NewBuffer(nil),
	}
}
func (c *MockConn) Read(b []byte) (int, error) {
	return c.buf.Read(b)
}
func (c *MockConn) Write(b []byte) (int, error) {
	return c.buf.Write(b)
}

func (c *MockConn) Close() error                       { return nil }
func (c *MockConn) LocalAddr() net.Addr                { return nil }
func (c *MockConn) RemoteAddr() net.Addr               { return nil }
func (c *MockConn) SetDeadline(t time.Time) error      { return nil }
func (c *MockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *MockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestNetPacking_PackUnpackTCP(t *testing.T) {
	m, err := newPackTestMemberlist()
	require.Nil(t, err, "setup memberlist failed")

	msg := []byte("this is a message")
	encoded, err := encode(pushPullMsg, msg)
	require.Nil(t, err, "fail to encode message")

	label := "label"

	packed, err := m.packTcp(encoded, label)
	require.Nil(t, err, "pack msg tcp failed")

	conn := NewMockConn()
	conn.Write(packed)

	msgType, _, dec, err := m.unpackStream(conn, label)

	require.Nil(t, err, "fail to unpack stream")
	require.Equal(t, msgType, pushPullMsg, "unmatched message type")

	out := make([]byte, len(msg))
	err = dec.Decode(out)
	require.Nil(t, err, "fail to decode msg")
	if !bytes.Equal(out, msg) {
		t.Fatalf("unmatched unpack msg stream, expect: %s, got: %s", msg, out)
	}
}

func newTestTransport() (*NetTransport, func(), error) {
	addr, cleanup := testaddr.BindAddrs.NextAvailAddr()
	addrs := []string{addr.String()}

	logger := log.New(os.Stderr, "testtransport", log.LstdFlags)
	var tr *NetTransport
	tr, err := NewNetTransport(addrs, 0, logger)
	if err != nil {
		return tr, cleanup, err
	}
	err = tr.Start()
	if err != nil {
		return tr, cleanup, err
	}
	cleanup1 := func() {
		tr.Shutdown()
		cleanup()
	}
	return tr, cleanup1, nil
}

func TestNetTransport_SendReceiveUDP(t *testing.T) {
	t1, cleanup1, err := newTestTransport()
	defer func() {
		if cleanup1 != nil {
			cleanup1()
		}
	}()
	require.Nil(t, err)

	t2, cleanup2, err := newTestTransport()
	defer func() {
		if cleanup2 != nil {
			cleanup2()
		}
	}()
	require.Nil(t, err)

	addr1, port1, err := t1.GetFirstAddr()
	require.Nil(t, err)
	udpAddr1 := net.UDPAddr{IP: addr1, Port: port1}

	addr2, port2, err := t2.GetFirstAddr()
	require.Nil(t, err)
	udpAddr2 := net.UDPAddr{IP: addr2, Port: port2}

	msg := ping{SeqNo: 42, Node: "Node1", SourceIP: addr1, SourcePort: uint16(port1)}
	encoded, err := encode(pingMsg, msg)
	require.Nil(t, err)

	t1.SendUdp(encoded, &udpAddr2)

	var packet *Packet
	select {
	case packet = <-t2.PacketCh():
	case <-time.After(5 * time.Millisecond):
		t.Fatalf("expect no timeout")
	}

	require.Equal(t, packet.From.String(), udpAddr1.String(), "expect matching FROM address")

	received := packet.Buf
	require.Equal(t, msgType(received[0]), pingMsg, "expect ping msg")

	var decoded ping
	err = decode(received[1:], &decoded)
	require.Nil(t, err)
	require.Equal(t, decoded.SeqNo, msg.SeqNo)
	require.Equal(t, decoded.Node, msg.Node)
	if !bytes.Equal(decoded.SourceIP, addr1) {
		t.Errorf("source addr not matched. expect %s, got %s", addr1, decoded.SourceIP)
	}
	require.Equal(t, decoded.SourcePort, uint16(port1))
}

func TestNetTransport_SendReceiveTCP(t *testing.T) {
	t1, cleanup1, err := newTestTransport()
	defer func() {
		if cleanup1 != nil {
			cleanup1()
		}
	}()
	require.Nil(t, err)

	t2, cleanup2, err := newTestTransport()
	defer func() {
		if cleanup2 != nil {
			cleanup2()
		}
	}()
	require.Nil(t, err)

	addr2, port2, err := t2.GetFirstAddr()
	require.Nil(t, err)
	tcpAddr2 := net.TCPAddr{IP: addr2, Port: port2}

	timeout := 5 * time.Millisecond
	conn1, err := t1.DialTimeout(tcpAddr2.String(), timeout)
	require.Nil(t, err)
	defer conn1.Close()

	var conn2 net.Conn
	select {
	case conn2 = <-t2.TcpConnCh():
	case <-time.After(timeout):
		t.Fatalf("expect no timeout!")
	}
	defer conn2.Close()

	conn1.SetDeadline(time.Now().Add(timeout))
	conn2.SetDeadline(time.Now().Add(timeout))

	msg := ping{SeqNo: 42, Node: "Node1"}
	encoded, err := encode(pingMsg, msg)
	require.Nil(t, err)

	conn1.Write(encoded)

	buf := [1]byte{0}
	_, err = io.ReadFull(conn2, buf[:])
	require.Nil(t, err)
	require.Equal(t, msgType(buf[0]), pingMsg, "expect ping msg")

	hd := codec.MsgpackHandle{}
	dec := codec.NewDecoder(conn2, &hd)
	var decoded ping
	err = dec.Decode(&decoded)
	require.Nil(t, err)
	require.Equal(t, decoded.SeqNo, msg.SeqNo)
	require.Equal(t, decoded.Node, msg.Node)

	res := ack{msg.SeqNo, []byte("abc")}
	encoded, err = encode(ackMsg, res)
	require.Nil(t, err)
	conn2.Write(encoded)

	_, err = io.ReadFull(conn1, buf[:])
	require.Nil(t, err)
	require.Equal(t, msgType(buf[0]), ackMsg, "expect ack msg")

	var decRes ack
	dec = codec.NewDecoder(conn1, &hd)
	err = dec.Decode(&decRes)
	require.Nil(t, err)
	require.Equal(t, decRes.SeqNo, res.SeqNo)
	require.Equal(t, decRes.Payload, res.Payload)
}
