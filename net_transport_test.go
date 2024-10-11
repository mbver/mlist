package memberlist

import (
	"bytes"
	"net"
	"testing"
	"time"

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

func TestNetTransport_SendReceive(t *testing.T) {

}
