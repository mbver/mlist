package memberlist

import (
	"container/list"
	"fmt"
	"io"
	"log"
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

func (m *Memberlist) handlePacket(msg []byte, from *net.UDPAddr, timestamp time.Time) {
	packetLabel := m.config.Label
	var err error
	msg, err = m.unpackPacket(msg, packetLabel)
	if err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to unpack payload: %v from %s", err, from)
		return
	}
	m.handleUdpMsg(msg, from, timestamp) // another goroutine?
}

func (m *Memberlist) handleUdpMsg(msg []byte, from *net.UDPAddr, timestamp time.Time) {
	if len(msg) < 1 {
		m.logger.Printf("[ERR] memberlist: missing message type byte from%s", from)
		return
	}
	// Decode the message type
	mType := msgType(msg[0])
	msg = msg[1:]
	// queue long running message and exit
	if isLongRunMsg(mType) {
		if err := m.longRunMng.queueLongRunMsg(mType, msg, from); err != nil {
			m.logger.Printf("[WARN] memberlist: fail to queue message %s (%d) from %s", err, mType, from)

		}
		return
	}
	// msg that can be handled quickly
	switch mType {
	case compoundMsg:
		m.handleCompound(msg, from, timestamp)
	case pingMsg:
		m.handlePing(msg, from)
	case ackMsg:
		m.handleAck(msg, from, timestamp)
	case indirectPingMsg:
		m.handleIndirectPing(msg, from)
	case indirectAckMsg:
		m.handleIndirectAck(msg, from)
	default:
		m.logger.Printf("[ERR] memberlist: msg type (%d) not supported from %s", mType, from)
	}
}

func (m *Memberlist) handleCompound(msg []byte, from *net.UDPAddr, timestamp time.Time) {
	// Decode the parts
	trunc, msgs, err := unpackCompoundMsg(msg)
	if err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to decode compound request: %s from %s", err, from)
		return
	}

	// Log any truncation
	if trunc > 0 {
		m.logger.Printf("[WARN] memberlist: Compound request had %d truncated messages %s", trunc, from)
	}

	// Handle each message
	for _, msg := range msgs {
		m.handleUdpMsg(msg, from, timestamp)
	}
}

func (m *Memberlist) handlePing(buf []byte, from *net.UDPAddr) {
	var p ping
	if err := decode(buf, &p); err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to decode ping request: %s from: %s", err, from)
		return
	}

	// If node is provided, verify that it is for us
	if p.ID != "" && p.ID != m.config.ID {
		m.logger.Printf("[WARN] memberlist: Got ping for unexpected node '%s' from %s", p.ID, from)
		return
	}
	var a ack
	a.SeqNo = p.SeqNo
	if m.pingMng.usrPing != nil {
		a.Payload = m.pingMng.usrPing.Payload()
	}
	addr := from
	if len(p.SourceIP) > 0 && p.SourcePort > 0 {
		addr = &net.UDPAddr{IP: p.SourceIP, Port: int(p.SourcePort)}
	}

	if err := m.encodeAndSendUdp(addr, ackMsg, &a); err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to send ack: %s", err)
	}
}

func (m *Memberlist) handleAck(buf []byte, from *net.UDPAddr, timestamp time.Time) {
	var a ack
	if err := decode(buf, &a); err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to decode ack response: %s from %s", err, from)
		return
	}
	m.pingMng.invokeAckHandler(a, timestamp)
}

func (m *Memberlist) handleIndirectPing(msg []byte, from *net.UDPAddr) {
	var ind indirectPing
	if err := decode(msg, &ind); err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to decode indirect ping request: %s from %s", err, from)
		return
	}

	// get address of requestor
	addr := from
	if len(ind.SourceIP) > 0 && ind.SourcePort > 0 {
		addr = &net.UDPAddr{IP: ind.SourceIP, Port: int(ind.SourcePort)}
	}
	node := &nodeState{
		Node: &Node{
			ID:   ind.Node,
			IP:   ind.IP,
			Port: ind.Port,
		},
		State: StateAlive,
	}
	go func() {
		ok := m.Ping(node, m.config.PingTimeout) // don't scale timeout for indirect ping
		indAck := indirectAck{ind.SeqNo, true}
		if !ok {
			indAck.Success = false
		}
		if err := m.encodeAndSendUdp(addr, indirectAckMsg, indAck); err != nil {
			// log error
		}
	}()
}

func (m *Memberlist) handleIndirectAck(msg []byte, from *net.UDPAddr) {
	var in indirectAck
	if err := decode(msg, &in); err != nil {
		// log error with from
		return
	}
	m.pingMng.invokeIndirectAckHandler(in)
}

type longRunMsg struct {
	mType msgType
	msg   []byte
	from  *net.UDPAddr
}

// TODO: passing userMsg to a channel for user to handle?
func isLongRunMsg(t msgType) bool {
	return t == aliveMsg || t == suspectMsg || t == deadMsg || t == userMsg
}

type longRunMsgManager struct {
	highPriorQueue *list.List
	lowPriorQueue  *list.List
	maxQueueDepth  int // taken from memberlist config
	l              sync.Mutex
	gotMsgCh       chan struct{}
	logger         *log.Logger
}

func newLongRunMsgManager(qDepth int) *longRunMsgManager {
	return &longRunMsgManager{
		highPriorQueue: list.New(),
		lowPriorQueue:  list.New(),
		maxQueueDepth:  qDepth,
		gotMsgCh:       make(chan struct{}),
	}
}

func (mng *longRunMsgManager) queueLongRunMsg(t msgType, msg []byte, from *net.UDPAddr) error {
	queue := mng.lowPriorQueue
	if t == aliveMsg { // alive msg is prioritized
		queue = mng.highPriorQueue
	}

	// Check for overflow and append if not full
	mng.l.Lock()
	if queue.Len() >= mng.maxQueueDepth { // drop msg
		return fmt.Errorf("queue is full")
	} else {
		queue.PushBack(&longRunMsg{t, msg, from})
	}
	mng.l.Unlock()

	select {
	case mng.gotMsgCh <- struct{}{}: // notify the handler
	default:
	}
	return nil
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
					m.handleSuspect(msg.msg, msg.from)
				case aliveMsg:
					m.handleAlive(msg.msg, msg.from)
				case deadMsg:
					m.handleDead(msg.msg, msg.from)
				case userMsg:
					m.handleUser(msg.msg, msg.from)
				default:
					m.logger.Printf("[ERR] memberlist: Message type (%d) not supported from %s (packet handler)", msg.mType, msg.from)
				}
			}
		case <-m.shutdownCh:
			return
		}
	}
}

func (m *Memberlist) handleAlive(msg []byte, from *net.UDPAddr) {
	if !m.IPAllowed(from.IP) {
		m.logger.Printf("[DEBUG] memberlist: Blocked alive message: from %s", from)
		return
	}
	var a alive
	if err := decode(msg, &a); err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to decode alive message: %s from %s", err, from)
		return
	}
	m.aliveNode(&a, nil)
}

func (m *Memberlist) handleSuspect(msg []byte, from *net.UDPAddr) {
	var sus suspect
	if err := decode(msg, &sus); err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to decode suspect message: %s from %s", err, from)
		return
	}
	m.suspectNode(&sus)
}

func (m *Memberlist) handleUser(msg []byte, from *net.UDPAddr) {
	if m.usrMsgCh != nil {
		m.usrMsgCh <- msg
	}
}

func (m *Memberlist) handleDead(msg []byte, from *net.UDPAddr) {
	var d dead
	if err := decode(msg, &d); err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to decode dead message: %s from %s", err, from)
		return
	}
	m.deadNode(&d, nil)
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
	m.logger.Printf("[DEBUG] memberlist: TCP connection from %s", conn.RemoteAddr())
	conn.SetDeadline(time.Now().Add(m.config.TcpTimeout))

	streamLabel := m.config.Label
	mType, connReader, dec, err := m.unpackStream(conn, streamLabel)

	if err != nil {
		if err != io.EOF {
			m.logger.Printf("[ERR] memberlist: failed to unpack: %s from %s", err, conn.RemoteAddr())
			resp := errResp{err.Error()}
			respMsg, err := encode(errMsg, &resp)
			if err != nil {
				m.logger.Printf("[ERR] memberlist: Failed to encode error response: %s", err)
				return
			}

			err = m.sendTcp(conn, respMsg, streamLabel)
			if err != nil {
				m.logger.Printf("[ERR] memberlist: Failed to send error: %s from %s", err, conn.RemoteAddr())
				return
			}
		}
		return
	}

	switch mType {
	case pushPullMsg:
		m.handlePushPull(connReader, dec, conn, streamLabel)
	case pingMsg:
		m.handlePingTcp(dec, conn, streamLabel)
	default:
		m.logger.Printf("[ERR] memberlist: Received invalid msgType (%d) from %s", mType, conn.RemoteAddr())
	}
}

func (m *Memberlist) handlePingTcp(dec *codec.Decoder, conn net.Conn, streamLabel string) {
	var p ping
	if err := dec.Decode(&p); err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to decode ping: %s from %s", err, conn.RemoteAddr())
		return
	}

	if p.ID != "" && p.ID != m.config.ID {
		m.logger.Printf("[WARN] memberlist: Got ping for unexpected node %s from %s", p.ID, conn.RemoteAddr())
		return
	}

	ack := ack{p.SeqNo, nil}
	out, err := encode(ackMsg, &ack)
	if err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to encode ack: %s", err)
		return
	}

	err = m.sendTcp(conn, out, streamLabel)
	if err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to send ack: %s from %s", err, conn.RemoteAddr())
		return
	}
}

func (m *Memberlist) handlePushPull(connReader io.Reader, dec *codec.Decoder, conn net.Conn, streamLabel string) {
	// Increment counter of pending push/pulls
	nPP := atomic.AddUint32(&m.numPushPull, 1)
	defer atomic.AddUint32(&m.numPushPull, ^uint32(0)) // decrease the counter
	// Check if we have too many open push/pull requests
	if nPP >= uint32(m.config.MaxPushPulls) {
		m.logger.Printf("[ERR] memberlist: Too many pending push/pull requests")
		return
	}

	// don't care about userState
	remoteNodes, err := m.readRemoteState(connReader, dec)
	if err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to read remote state: %s from %s", err, conn.RemoteAddr())
		return
	}

	if err := m.sendLocalState(conn, streamLabel); err != nil {
		m.logger.Printf("[ERR] memberlist: Failed to push local state: %s from %s", err, conn.RemoteAddr())
		return
	}

	m.mergeState(remoteNodes)
}
