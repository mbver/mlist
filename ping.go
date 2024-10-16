package memberlist

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type ping struct {
	SeqNo      uint32
	Node       string
	SourceAddr []byte `codec:",omitempty"`
	SourcePort uint16 `codec:",omitempty"`
}

type ack struct {
	SeqNo   uint32
	Payload []byte
}

type timedAck struct {
	payload   []byte
	timestamp time.Time
}

type ackHandler struct {
	timer *time.Timer
	ackCh chan timedAck
}

type pingManager struct {
	seqNo               uint32
	l                   sync.Mutex
	ackHandlers         map[uint32]*ackHandler
	indirectAckHandlers map[uint32]*indirectAckHandler
	usrPing             PingDelegate
}

func newPingManager() *pingManager {
	return &pingManager{
		ackHandlers:         make(map[uint32]*ackHandler),
		indirectAckHandlers: make(map[uint32]*indirectAckHandler),
	}
}

func (mng *pingManager) nextSeqNo() uint32 { // will wrap arround
	return atomic.AddUint32(&mng.seqNo, 1)
}

func (mng *pingManager) setAckHandler(seqNo uint32, ch chan timedAck, timeout time.Duration) {
	// delete handler after timeout
	t := time.AfterFunc(timeout, func() {
		mng.l.Lock()
		delete(mng.ackHandlers, seqNo)
		mng.l.Unlock()
	})
	mng.l.Lock()
	defer mng.l.Unlock()
	mng.ackHandlers[seqNo] = &ackHandler{t, ch}
}

func (mng *pingManager) invokeAckHandler(a ack, timestamp time.Time) {
	mng.l.Lock()
	h, ok := mng.ackHandlers[a.SeqNo]
	delete(mng.ackHandlers, a.SeqNo)
	mng.l.Unlock()
	if !ok {
		return
	}
	h.timer.Stop() // cancel timer
	select {
	case h.ackCh <- timedAck{a.Payload, timestamp}:
	default:
	}
}

func (m *Memberlist) handleAck(buf []byte, from net.Addr, timestamp time.Time) {
	var a ack
	if err := decode(buf, &a); err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to decode ack response: %s %s", err, LogAddress(from))
		return
	}
	m.pingMng.invokeAckHandler(a, timestamp)
}

type PingDelegate interface {
	Payload() []byte
	FinishPing(from *Node, rtt time.Duration, payload []byte)
}

// run the handler for seqNo when its ack arrives. delete handler from the map.
// include buddy mechanism. they are almost the same
func (m *Memberlist) Ping(node *nodeState) bool {
	localAddr, localPort, err := m.GetAdvertiseAddr()
	if err != nil {
		// TODO; log error
		return false
	}

	p := ping{
		SeqNo:      m.pingMng.nextSeqNo(),
		Node:       node.Node.ID,
		SourceAddr: localAddr,
		SourcePort: localPort,
	}

	ackCh := make(chan timedAck)
	m.pingMng.setAckHandler(p.SeqNo, ackCh, m.config.PingTimeout)
	sent := time.Now()

	if node.State == StateAlive {
		if err := m.encodeAndSendUdp(node.Node.Address(), pingMsg, &p); err != nil {
			// TODO: log error
			return false
		}
	} else { // state suspect, apply buddy mechanism so it can refute asap
		s := suspect{Lives: node.Lives, Node: node.Node.ID, From: m.config.ID}
		msg, err := buddyPingMsg(p, s)
		if err != nil {
			// log err
			return false
		}
		if err = m.sendUdp(node.Node.Address(), msg); err != nil {
			// log err
			return false
		}
	}

	select {
	case a := <-ackCh:
		if m.pingMng.usrPing != nil {
			rtt := a.timestamp.Sub(sent)
			m.pingMng.usrPing.FinishPing(node.Node, rtt, a.payload)
		}
		return true
	case <-time.After(m.config.PingTimeout):
		// m.logger.Printf("[DEBUG] memberlist: Failed UDP ping: %s (timeout reached)", node.Name)
		return false
	}
}

func buddyPingMsg(p ping, s suspect) ([]byte, error) {
	var msgs [][]byte
	if buf, err := encode(pingMsg, p); err != nil {
		return nil, err
	} else {
		msgs = append(msgs, buf)
	}
	if buf, err := encode(suspectMsg, &s); err != nil {
		return nil, err
	} else {
		msgs = append(msgs, buf)
	}
	return packCompoundMsg(msgs), nil
}

func (m *Memberlist) handlePing(buf []byte, from net.Addr) {
	var p ping
	if err := decode(buf, &p); err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to decode ping request: %s %s", err, LogAddress(from))
		return
	}

	// If node is provided, verify that it is for us
	if p.Node != "" && p.Node != m.config.ID {
		// m.logger.Printf("[WARN] memberlist: Got ping for unexpected node '%s' %s", p.Node, LogAddress(from))
		return
	}
	var a ack
	a.SeqNo = p.SeqNo
	if m.pingMng.usrPing != nil {
		a.Payload = m.pingMng.usrPing.Payload()
	}

	var addr string
	if len(p.SourceAddr) > 0 && p.SourcePort > 0 {
		addr = joinHostPort(net.IP(p.SourceAddr).String(), p.SourcePort)
	} else {
		addr = from.String()
	}

	if err := m.encodeAndSendUdp(addr, ackMsg, &a); err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to send ack: %s", err)
	}
}

type indirectPing struct {
	SeqNo      uint32
	Addr       string
	Port       uint16
	Node       string
	SourceAddr string `codec:",omitempty"`
	SourcePort uint16 `codec:",omitempty"`
	SourceNode string `codec:",omitempty"`
}

type indirectAck struct {
	SeqNo   uint32
	Success bool
}

// set ackHandler missing?
type indirectAckHandler struct {
	ackCh  chan struct{}
	nNacks *int32
	timer  *time.Timer
}

func (mng *pingManager) setIndirectAckHandler(seqNo uint32, ackCh chan struct{}, nNacks *int32, timeout time.Duration) {
	t := time.AfterFunc(timeout, func() {
		mng.l.Lock()
		delete(mng.indirectAckHandlers, seqNo)
		mng.l.Unlock()
	})
	mng.l.Lock()
	defer mng.l.Unlock()
	mng.indirectAckHandlers[seqNo] = &indirectAckHandler{ackCh, nNacks, t}
}

type indirectPingResult struct {
	success bool
	nNacks  int
}

func (m *Memberlist) IndirectPing(node *nodeState, timeout time.Duration) chan indirectPingResult {
	resultCh := make(chan indirectPingResult, 1)
	go func() {
		localAddr, localPort, err := m.GetAdvertiseAddr()
		if err != nil {
			// log error
			resultCh <- indirectPingResult{false, 0}
			return
		}

		// pickRandomNodes should hold the lock!
		peers := m.pickRandomNodes(m.config.NumIndirectChecks, func(n *nodeState) bool {
			return n.Node.ID != m.config.ID &&
				n.Node.ID != node.Node.ID &&
				n.State == StateAlive
		})

		ind := indirectPing{
			SeqNo:      m.pingMng.nextSeqNo(),
			Addr:       node.Node.Addr.String(),
			Port:       node.Node.Port,
			Node:       node.Node.ID,
			SourceAddr: localAddr.String(),
			SourcePort: localPort,
			SourceNode: m.config.ID,
		}

		ackCh := make(chan struct{})
		nNacks := int32(0)
		m.pingMng.setIndirectAckHandler(ind.SeqNo, ackCh, &nNacks, timeout)
		for _, peer := range peers {
			if err := m.encodeAndSendUdp(peer.Node.Address(), indirectPingMsg, &ind); err != nil {
				resultCh <- indirectPingResult{false, 0}
				return
			}
		}

		select {
		case <-ackCh:
			resultCh <- indirectPingResult{true, 0}
		case <-time.After(timeout):
			resultCh <- indirectPingResult{false, int(atomic.LoadInt32(&nNacks))}
		}

	}()
	return resultCh
}

func (m *Memberlist) handleIndirectPing(msg []byte, from net.Addr) {
	var ind indirectPing
	if err := decode(msg, &ind); err != nil {
		// m.logger.Printf("[ERR] memberlist: Failed to decode indirect ping request: %s %s", err, LogAddress(from))
		return
	}
	// get address of requestor
	var addr string
	if len(ind.SourceAddr) > 0 && ind.SourcePort > 0 {
		addr = joinHostPort(ind.SourceAddr, ind.SourcePort)
	} else {
		addr = from.String()
	}
	var ok bool
	defer func() {
		indAck := indirectAck{ind.SeqNo, true}
		if !ok {
			indAck.Success = false
		}
		if err := m.encodeAndSendUdp(addr, indirectAckMsg, indAck); err != nil {
			// log error
		}
	}()
	node := &nodeState{
		Node: &Node{
			Addr: net.ParseIP(ind.Addr),
			Port: ind.Port,
			ID:   ind.Node,
		},
		State: StateAlive,
	}
	ok = m.Ping(node)
}

func (m *Memberlist) handleIndirectAck(msg []byte, from net.Addr) {
	var in indirectAck
	if err := decode(msg, &in); err != nil {
		// log error with from
		return
	}
	m.pingMng.invokeIndirectAckHandler(in)
}

func (mng *pingManager) invokeIndirectAckHandler(in indirectAck) {
	mng.l.Lock()
	defer mng.l.Unlock()
	h, ok := mng.indirectAckHandlers[in.SeqNo]
	if !ok {
		return
	}
	if !in.Success {
		atomic.AddInt32(h.nNacks, 1)
		return
	}
	h.timer.Stop()
	delete(mng.indirectAckHandlers, in.SeqNo)
	select {
	case h.ackCh <- struct{}{}:
	default:
	}
}

func (m *Memberlist) TcpPing(node *nodeState, timeout time.Duration) chan bool {
	result := make(chan bool, 1)
	go func() {
		start := time.Now()
		deadline := start.Add(timeout)
		localAddr, localPort, err := m.GetAdvertiseAddr()
		if err != nil {
			// log error
			result <- false
			return
		}
		ping := ping{
			SeqNo:      m.pingMng.nextSeqNo(),
			Node:       node.Node.ID,
			SourceAddr: localAddr,
			SourcePort: localPort,
		}
		conn, err := m.transport.DialTimeout(node.Node.Address(), timeout)
		if err != nil {
			result <- false
			return
		}
		defer conn.Close()

		conn.SetDeadline(deadline)

		encoded, err := encode(pingMsg, &ping)
		if err != nil {
			result <- false
			return
		}
		if err = m.sendTcp(conn, encoded, m.config.Label); err != nil {
			result <- false
			return
		}
		msgType, _, dec, err := m.unpackStream(conn, m.config.Label)
		if err != nil {
			result <- false
			return
		}
		if msgType != ackMsg { // log error
			// m.logger.Printf("unexpected msgType (%d) from ping %s", msgType, LogConn(conn))
			result <- false
			return
		}
		var a ack
		if err = dec.Decode(&a); err != nil {
			result <- false
			return
		}
		if a.SeqNo != ping.SeqNo {
			result <- false
			return
		}
		result <- true
	}()
	return result
}
