package memberlist

import (
	"bytes"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/mbver/mlist/testaddr"
	"github.com/stretchr/testify/require"
)

func setUpPackTest() (*Memberlist, error) {
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
	m, err := setUpPackTest()
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
	m, err := setUpPackTest()
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

func setupTransportTest(t *testing.T) (t1 *NetTransport, t2 *NetTransport, cleanup1 func(), cleanup2 func()) {
	addr1, cleanup1 := testaddr.BindAddrs.NextAvailAddr()
	addrs1 := []string{addr1.String()}

	addr2, cleanup2 := testaddr.BindAddrs.NextAvailAddr()
	addrs2 := []string{addr2.String()}

	logger := log.New(os.Stderr, "testtransport", log.LstdFlags)

	var err error
	defer func() {
		if err != nil {
			cleanup1()
			cleanup2()
		}
	}()
	t1, err = NewNetTransport(addrs1, 0, logger)
	require.Nil(t, err)
	err = t1.Start()
	require.Nil(t, err)

	t2, err = NewNetTransport(addrs2, 0, logger)
	require.Nil(t, err)
	err = t2.Start()
	require.Nil(t, err)
	return t1, t2, cleanup1, cleanup2
}

func TestNetTransport_SendReceive(t *testing.T) {
	t1, t2, cleanup1, cleanup2 := setupTransportTest(t)
	defer func() {
		t1.Shutdown()
		cleanup1()
		t2.Shutdown()
		cleanup2()
	}()

	addr1, port1, err := t1.GetFirstAddr()
	require.Nil(t, err)
	udpAddr1 := net.UDPAddr{IP: addr1, Port: port1}

	addr2, port2, err := t2.GetFirstAddr()
	require.Nil(t, err)
	udpAddr2 := net.UDPAddr{IP: addr2, Port: port2}

	msg := ping{SeqNo: 42, Node: "Node1"}
	encoded, err := encode(pingMsg, msg)
	require.Nil(t, err, "expect no error")

	t1.SendUdp(encoded, udpAddr2.String())

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
}
