package memberlist

import (
	"io"
	"net"
	"time"

	"github.com/hashicorp/go-msgpack/v2/codec"
)

type msgType int

type NetTransportConfig struct{}

type Packet struct{}

type NetTransport struct{}

func NewNetTransport(config *NetTransportConfig) (*NetTransport, error) {
	return nil, nil
}

func setUDPRecvBuf(c *net.UDPConn) error {
	return nil
}

func (t *NetTransport) tcpListen(l *net.TCPListener) {}

func (t *NetTransport) TcpConnCh() <-chan net.Conn {
	return nil
}

func (t *NetTransport) udpListen(c *net.UDPConn) {}

func (t *NetTransport) PacketCh() <-chan *Packet {
	return nil
}

func (t *NetTransport) SendUdp(msg []byte, a net.Addr) (time.Time, error) {
	return time.Time{}, nil
}

func (t *NetTransport) GetAdvertiseAddr() (net.IP, int, error) {
	return nil, 0, nil
}

func (t *NetTransport) Shutdown() {}

func (m *Memberlist) sendMsgTcp(conn net.Conn, msg []byte, streamLabel string) error {
	return nil
}

func (m *Memberlist) readStream(conn net.Conn, streamLabel string) (msgType, io.Reader, *codec.Decoder, error) {
	return 0, nil, nil, nil
}

func (m *Memberlist) sendMsgUdp(a net.Addr, msg []byte) error {
	return nil
}

func (m *Memberlist) sendMsgPiggyback(a net.Addr, msg []byte) error {
	return nil
}

func makeCompoundMsg(msgs [][]byte) []byte {
	return nil
}
