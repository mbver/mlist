package memberlist

import (
	"sync"
	"sync/atomic"
	"time"
)

type ackHandler struct {
	timer *time.Timer
	ackCh chan timedAck
}

type PingDelegate interface {
	Payload() []byte
	Finish(from *Node, rtt time.Duration, payload []byte)
}

type pingManager struct {
	seqNo               uint32
	l                   sync.Mutex
	ackHandlers         map[uint32]*ackHandler
	indirectAckHandlers map[uint32]*indirectAckHandler
	usrPing             PingDelegate
}

func newPingManager(d PingDelegate) *pingManager {
	return &pingManager{
		ackHandlers:         make(map[uint32]*ackHandler),
		indirectAckHandlers: make(map[uint32]*indirectAckHandler),
		usrPing:             d,
	}
}

func (mng *pingManager) nextSeqNo() uint32 { // will wrap arround
	return atomic.AddUint32(&mng.seqNo, 1)
}

type ping struct {
	SeqNo      uint32
	ID         string
	SourceIP   []byte `codec:",omitempty"`
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

type indirectPing struct {
	SeqNo      uint32
	IP         []byte
	Port       uint16
	Node       string
	SourceIP   []byte `codec:",omitempty"`
	SourcePort uint16 `codec:",omitempty"`
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

// run the handler for seqNo when its ack arrives. delete handler from the map.
// include buddy mechanism. they are almost the same
func (m *Memberlist) Ping(node *nodeState, timeout time.Duration) bool {
	localIp, localPort, err := m.GetAdvertiseAddr()
	if err != nil {
		// TODO; log error
		return false
	}

	p := ping{
		SeqNo:      m.pingMng.nextSeqNo(),
		ID:         node.Node.ID,
		SourceIP:   localIp,
		SourcePort: localPort,
	}

	ackCh := make(chan timedAck)
	m.pingMng.setAckHandler(p.SeqNo, ackCh, timeout)
	sent := time.Now()

	if node.State == StateAlive {
		if err := m.encodeAndSendUdp(node.Node.UDPAddress(), pingMsg, &p); err != nil {
			// TODO: log error
			return false
		}
	} else { // state suspect, apply buddy mechanism so it can refute asap
		s := suspect{Lives: node.Lives, ID: node.Node.ID, From: m.ID()}
		msg, err := buddyPingMsg(p, s)
		if err != nil {
			// log err
			return false
		}
		if err = m.sendUdp(node.Node.UDPAddress(), msg); err != nil {
			// log err
			return false
		}
	}

	select {
	case a := <-ackCh:
		if m.pingMng.usrPing != nil {
			rtt := a.timestamp.Sub(sent)
			m.pingMng.usrPing.Finish(node.Node, rtt, a.payload)
		}
		return true
	case <-time.After(timeout):
		// m.logger.Printf("[DEBUG] memberlist: Failed UDP ping: %s (timeout reached)", node.Name)
		return false
	}
}

type indirectPingResult struct {
	success bool
	nNacks  int
	numNode int
}

func (m *Memberlist) IndirectPing(node *nodeState, timeout time.Duration) chan indirectPingResult {
	resultCh := make(chan indirectPingResult, 1)
	go func() {
		localIp, localPort, err := m.GetAdvertiseAddr()
		if err != nil {
			// log error
			resultCh <- indirectPingResult{false, 0, 0}
			return
		}

		peers := m.pickRandomNodes(m.config.NumIndirectChecks, func(n *nodeState) bool {
			return n.Node.ID != m.ID() &&
				n.Node.ID != node.Node.ID &&
				n.State == StateAlive
		})

		if len(peers) == 0 {
			resultCh <- indirectPingResult{false, 0, 0}
			return
		}

		ind := indirectPing{
			SeqNo:      m.pingMng.nextSeqNo(),
			IP:         node.Node.IP,
			Port:       node.Node.Port,
			Node:       node.Node.ID,
			SourceIP:   localIp,
			SourcePort: localPort,
		}

		ackCh := make(chan struct{})
		nNacks := int32(0)
		m.pingMng.setIndirectAckHandler(ind.SeqNo, ackCh, &nNacks, timeout)
		for _, peer := range peers {
			if err := m.encodeAndSendUdp(peer.Node.UDPAddress(), indirectPingMsg, &ind); err != nil {
				resultCh <- indirectPingResult{false, 0, 0}
				return
			}
		}

		select {
		case <-ackCh:
			resultCh <- indirectPingResult{true, 0, 0}
		case <-time.After(timeout):
			resultCh <- indirectPingResult{false, int(atomic.LoadInt32(&nNacks)), len(peers)}
		}

	}()
	return resultCh
}

func (m *Memberlist) TcpPing(node *nodeState, timeout time.Duration) chan bool {
	result := make(chan bool, 1)
	go func() {
		start := time.Now()
		deadline := start.Add(timeout)
		localIp, localPort, err := m.GetAdvertiseAddr()
		if err != nil {
			// log error
			result <- false
			return
		}
		ping := ping{
			SeqNo:      m.pingMng.nextSeqNo(),
			ID:         node.Node.ID,
			SourceIP:   localIp,
			SourcePort: localPort,
		}
		addr := node.Node.UDPAddress().String() // same address for udp and tcp
		conn, err := m.transport.DialTimeout(addr, timeout)
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
