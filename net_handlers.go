package memberlist

import (
	"io"
	"net"
	"time"

	"github.com/hashicorp/go-msgpack/v2/codec"
)

type msgType int

const (
	pingMsg msgType = iota
	indirectPingMsg
	ackRespMsg
	suspectMsg
	aliveMsg
	deadMsg
	pushPullMsg
	compoundMsg
	userMsg // User mesg, not handled by us
	compressMsg
	encryptMsg
	nackRespMsg
	hasCrcMsg
	errMsg
)

func (m *Memberlist) receiveTcpConn() {}

func (m *Memberlist) handleTcpConn(conn net.Conn) {}

func (m *Memberlist) handlePingTcp(dec *codec.Decoder, conn *net.Conn, streamLabel string) {}

func (m *Memberlist) handlePushPull(connReader io.Reader, dec *codec.Decoder, conn *net.Conn, streamLabel string) {
}

func (m *Memberlist) receivePacket() {}

func (m *Memberlist) handlePacket(msg []byte, from net.Addr, timestamp time.Time) {}

func (m *Memberlist) handleUdpMsg(msg []byte, from net.Addr, timestamp time.Time) {}

type longRunMsg struct{}

func (m *Memberlist) isLongRunMsg(t msgType) bool {
	return false
}

func (m *Memberlist) queueLongRunMsg(t msgType, msg []byte, from net.Addr) {}

func (m *Memberlist) getNextLongRunMsg() (longRunMsg, bool) {
	return longRunMsg{}, false
}

func (m *Memberlist) handleLongRunMsg() {}
