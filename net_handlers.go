package memberlist

import (
	"container/list"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-msgpack/v2/codec"
)

type msgType int

const (
	pingMsg msgType = iota
	indirectPingMsg
	ackMsg
	indirectAckMsg
	aliveMsg
	suspectMsg
	deadMsg
	leaveMsg
	pushPullMsg
	compoundMsg
	userMsg // User mesg, not handled by us
	compressMsg
	encryptMsg
	hasCrcMsg
	errMsg
)

func (m *Memberlist) receivePacket() {
	for {
		select {
		case packet := <-m.transport.PacketCh():
			m.handlePacket(packet.Buf, packet.From, packet.Timestamp)

		case <-m.shutdownCh:
			return
		}
	}
}

func (m *Memberlist) handlePacket(msg []byte, from net.Addr, timestamp time.Time) {
	packetLabel := m.config.Label
	var err error
	msg, err = m.unpackPacket(msg, packetLabel)
	if err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to unpack payload: %v %s", err, LogAddress(from))
		return
	}
	m.handleUdpMsg(msg, from, timestamp) // another goroutine?
}

func (m *Memberlist) handleUdpMsg(msg []byte, from net.Addr, timestamp time.Time) {
	if len(msg) < 1 {
		// m.logger.Printf("[ERR] memberlist: missing message type byte %s", LogAddress(from))
		return
	}
	// Decode the message type
	mType := msgType(msg[0])
	msg = msg[1:]
	// queue long running message and exit
	if isLongRunMsg(mType) {
		m.longRunMng.queueLongRunMsg(mType, msg, from)
		return
	}
	// msg that can be handled quickly
	switch mType {
	case compoundMsg:
		// m.handleCompound(buf, from, timestamp)
	case pingMsg:
		m.handlePing(msg, from)
	case indirectPingMsg:
		m.handleIndirectPing(msg, from)
	case ackMsg:
		m.handleAck(msg, from, timestamp)
	case indirectAckMsg:
		m.handleIndirectAck(msg, from)
	default:
		// m.logger.Printf("[ERR] memberlist: msg type (%d) not supported %s", msgType, LogAddress(from))
	}
}

func (m *Memberlist) handleCompound(msg []byte, from net.Addr, timestamp time.Time) {
	// Decode the parts
	trunc, msgs, err := unpackCompoundMsg(msg)
	if err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to decode compound request: %s %s", err, LogAddress(from))
		return
	}

	// Log any truncation
	if trunc > 0 {
		// m.logger.Printf("[WARN] memberlist: Compound request had %d truncated messages %s", trunc, LogAddress(from))
	}

	// Handle each message
	for _, msg := range msgs {
		m.handleUdpMsg(msg, from, timestamp)
	}
}

type longRunMsgManager struct {
	highPriorQueue *list.List
	lowPriorQueue  *list.List
	maxQueueDepth  int // taken from memberlist config
	l              sync.Mutex
	gotMsgCh       chan struct{}
}

func newLongRunMsgManager(qDepth int) *longRunMsgManager {
	return &longRunMsgManager{
		highPriorQueue: list.New(),
		lowPriorQueue:  list.New(),
		maxQueueDepth:  qDepth,
		gotMsgCh:       make(chan struct{}),
	}
}

type longRunMsg struct {
	mType msgType
	msg   []byte
	from  net.Addr
}

// TODO: passing userMsg to a channel for user to handle?
func isLongRunMsg(t msgType) bool {
	return t == aliveMsg || t == suspectMsg || t == deadMsg || t == leaveMsg || t == userMsg
}

func (mng *longRunMsgManager) queueLongRunMsg(t msgType, msg []byte, from net.Addr) {
	queue := mng.lowPriorQueue
	if t == aliveMsg || t == leaveMsg { // alive msg is prioritized
		queue = mng.highPriorQueue
	}

	// Check for overflow and append if not full
	mng.l.Lock()
	if queue.Len() >= mng.maxQueueDepth { // drop msg
		// m.logger.Printf("[WARN] memberlist: handler queue full, dropping message (%d) %s", msgType, LogAddress(from))
	} else {
		queue.PushBack(&longRunMsg{t, msg, from})
	}
	mng.l.Unlock()

	select {
	case mng.gotMsgCh <- struct{}{}: // notify the handler
	default:
	}
}

func (mng *longRunMsgManager) getNextMsg() (*longRunMsg, error) {
	mng.l.Lock()
	defer mng.l.Unlock()

	if el := mng.highPriorQueue.Back(); el != nil {
		mng.highPriorQueue.Remove(el)
		return el.Value.(*longRunMsg), nil
	} else if el := mng.lowPriorQueue.Back(); el != nil {
		mng.lowPriorQueue.Remove(el)
		return el.Value.(*longRunMsg), nil
	}
	return nil, fmt.Errorf("no long-run msg")
}

func (m *Memberlist) runLongRunMsgHandler() {
	for {
		select {
		case <-m.longRunMng.gotMsgCh:
			for {
				msg, err := m.longRunMng.getNextMsg()
				if err != nil {
					break
				}

				switch msg.mType {
				case suspectMsg:
					// m.handleSuspect(msg.msg, msg.from)
				case aliveMsg:
					// m.handleAlive(msg.msg, msg.from)
				case deadMsg:
					// m.handleDead(msg.msg, msg.from)
				case userMsg:
					// m.handleUser(msg.msg, msg.from)
				default:
					// m.logger.Printf("[ERR] memberlist: Message type (%d) not supported %s (packet handler)", msgType, LogAddress(from))
				}
			}
		case <-m.shutdownCh:
			return
		}
	}
}

func (m *Memberlist) receiveTcpConn() {
	for {
		select {
		case conn := <-m.transport.TcpConnCh():
			go m.handleTcpConn(conn)
		case <-m.shutdownCh:
			return
		}
	}
}

type errResp struct {
	Error string
}

func (m *Memberlist) handleTcpConn(conn net.Conn) {
	defer conn.Close()
	// m.logger.Printf("[DEBUG] memberlist: TCP connection %s", LogConn(conn))

	conn.SetDeadline(time.Now().Add(m.config.TcpTimeout))

	streamLabel := m.config.Label
	mType, connReader, dec, err := m.unpackStream(conn, streamLabel)

	if err != nil {
		if err != io.EOF {
			// m.logger.Printf("[ERR] memberlist: failed to unpack: %s %s", err, LogConn(conn))
			resp := errResp{err.Error()}
			respMsg, err := encode(errMsg, &resp)
			if err != nil {
				// m.logger.Printf("[ERR] memberlist: Failed to encode error response: %s", err)
				return
			}

			err = m.sendTcp(conn, respMsg, streamLabel)
			if err != nil {
				// m.logger.Printf("[ERR] memberlist: Failed to send error: %s %s", err, LogConn(conn))
				return
			}
		}
		return
	}

	switch mType {
	// don't send user msg via tcp anymore. drop it
	case pushPullMsg:
		m.handlePushPull(connReader, dec, conn, streamLabel)
	case pingMsg:
		m.handlePingTcp(dec, conn, streamLabel)
	default:
		// m.logger.Printf("[ERR] memberlist: Received invalid msgType (%d) %s", msgType, LogConn(conn))
	}
}

func (m *Memberlist) handlePingTcp(dec *codec.Decoder, conn net.Conn, streamLabel string) {
	var p ping
	if err := dec.Decode(&p); err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to decode ping: %s %s", err, LogConn(conn))
		return
	}

	if p.Node != "" && p.Node != m.config.ID {
		// m.logger.Printf("[WARN] memberlist: Got ping for unexpected node %s %s", p.Node, LogConn(conn))
		return
	}

	ack := ack{p.SeqNo, nil}
	out, err := encode(ackMsg, &ack)
	if err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to encode ack: %s", err)
		return
	}

	err = m.sendTcp(conn, out, streamLabel)
	if err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to send ack: %s %s", err, LogConn(conn))
		return
	}
}

func (m *Memberlist) handlePushPull(connReader io.Reader, dec *codec.Decoder, conn net.Conn, streamLabel string) {
	// Increment counter of pending push/pulls
	nPP := atomic.AddUint32(&m.numPushPull, 1)
	defer atomic.AddUint32(&m.numPushPull, ^uint32(0)) // decrease the counter
	// Check if we have too many open push/pull requests
	if nPP >= uint32(m.config.MaxConcurentPushPull) {
		// m.logger.Printf("[ERR] memberlist: Too many pending push/pull requests")
		return
	}

	// don't care about userState
	remoteNodes, err := m.readRemoteState(connReader, dec)
	if err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to read remote state: %s %s", err, LogConn(conn))
		return
	}

	if err := m.sendLocalState(conn, streamLabel); err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to push local state: %s %s", err, LogConn(conn))
		return
	}

	m.mergeState(remoteNodes)
}
