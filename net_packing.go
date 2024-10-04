package memberlist

import (
	"io"
	"net"

	"github.com/hashicorp/go-msgpack/v2/codec"
)

func (m *Memberlist) packMsgTcp(msg []byte, streamLabel string) ([]byte, error) {
	return nil, nil
}

func (m *Memberlist) unpackStream(conn net.Conn, streamLabel string) (msgType, io.Reader, *codec.Decoder, error) {
	return 0, nil, nil, nil
}

func (m *Memberlist) packMsgUdp(msg []byte, packetLabel string) ([]byte, error) {
	return nil, nil
}

func (m *Memberlist) unpackPacket(msg []byte, packetLabel string) ([]byte, error) {
	return nil, nil
}
