package memberlist

import (
	"net"
	"time"
)

type ping struct {
	SeqNo      uint32
	Node       string
	SourceAddr []byte `codec:",omitempty"`
	SourcePort uint16 `codec:",omitempty"`
}

// include buddy mechanism. they are almost the same
func (m *Memberlist) Ping(node *nodeState) bool {
	return false
}

// set ackHandler missing?

type indirectPingResult struct{}

func (m *Memberlist) IndirectPing(node *nodeState, timeout time.Duration) chan indirectPingResult {
	return nil
}

type indirectAckMsg struct{}

func (m *Memberlist) setIndirectAckHandler(seqNo, ackCh chan indirectAckMsg, nackCh chan struct{}, timeout time.Duration) {

}

func (m *Memberlist) handleIndirectPing(buf []byte, from net.Addr) {}

func (m *Memberlist) TcpPing(node *nodeState, timeout time.Duration) chan bool {
	return nil
}
