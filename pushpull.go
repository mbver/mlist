package memberlist

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net"
	"time"

	"github.com/hashicorp/go-msgpack/v2/codec"
)

const pushPullScaleThreshold = 32

type UserStateDelegate interface {
	LocalState() []byte
	MergeState([]byte)
}

type stateToMerge struct {
	Lives uint32
	ID    string
	IP    net.IP
	Port  uint16
	Tags  []byte
	State StateType
}

type pushPullHeader struct {
	Nodes        int
	UserStateLen int
}

func pushPullScale(interval time.Duration, n int) time.Duration {
	// Don't scale until we cross the threshold
	if n <= pushPullScaleThreshold {
		return interval
	}

	mult := math.Ceil(math.Log2(float64(n))-math.Log2(pushPullScaleThreshold)) + 1.0
	return time.Duration(mult) * interval
}

func (m *Memberlist) pushPull() {
	// choose a node to
	nodes := m.pickRandomNodes(1, func(n *nodeState) bool {
		return n.Node.ID != m.ID() && n.State == StateAlive
	})

	if len(nodes) == 0 {
		return
	}
	node := nodes[0]
	addr := node.Node.UDPAddress().String()
	if err := m.pushPullWithNode(addr); err != nil {
		m.logger.Printf("[ERR] memberlist: Push/Pull with %s failed: %s", node.Node.ID, err)
	}
}

func (m *Memberlist) pushPullWithNode(addr string) error {
	// Attempt to send and receive with the node
	remoteNodes, remoteUserState, err := m.sendAndReceiveState(addr)
	if err != nil {
		return err
	}

	m.mergeState(remoteNodes)
	if m.usrState != nil {
		m.usrState.MergeState(remoteUserState)
	}
	return nil
}

func (m *Memberlist) sendAndReceiveState(addr string) ([]stateToMerge, []byte, error) {
	// Attempt to connect
	conn, err := m.transport.DialTimeout(addr, m.config.TcpTimeout)
	if err != nil { // timeout error if unable to connect
		return nil, nil, err
	}
	defer conn.Close()
	m.logger.Printf("[DEBUG] memberlist: Initiating push/pull sync with: %s %s", addr, conn.RemoteAddr())

	// Send our state
	if err := m.sendLocalState(conn, m.config.Label); err != nil {
		return nil, nil, err
	}

	conn.SetDeadline(time.Now().Add(m.config.TcpTimeout))
	msgType, bufConn, dec, err := m.unpackStream(conn, m.config.Label)

	if err != nil {
		return nil, nil, err
	}

	if msgType == errMsg {
		var resp errResp
		if err := dec.Decode(&resp); err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("remote error: %v", resp.Error)
	}
	// Quit if not push/pull
	if msgType != pushPullMsg {
		err := fmt.Errorf("received invalid msgType (%d), expected pushPullMsg (%d) from %s", msgType, pushPullMsg, conn.RemoteAddr())
		return nil, nil, err
	}

	// Read remote state
	return m.readRemoteState(bufConn, dec)
}

func (m *Memberlist) sendLocalState(conn net.Conn, streamLabel string) error {
	conn.SetDeadline(time.Now().Add(m.config.TcpTimeout))
	localState := m.localState()
	var usrState []byte
	if m.usrState != nil {
		usrState = m.usrState.LocalState()
	}
	// Create a buffered writer
	msg, err := encodePushPullMsg(localState, usrState)
	if err != nil {
		return err
	}

	return m.sendTcp(conn, msg, streamLabel)
}

func (m *Memberlist) localState() []stateToMerge {
	// Prepare the local node state
	m.nodeL.RLock()
	defer m.nodeL.RUnlock()
	res := make([]stateToMerge, len(m.nodes))
	for i, n := range m.nodes {
		res[i].Lives = n.Lives
		res[i].ID = n.Node.ID
		res[i].IP = n.Node.IP
		res[i].Port = n.Node.Port
		res[i].State = n.State
		res[i].Tags = n.Node.Tags
	}
	return res
}

func encodePushPullMsg(localNodes []stateToMerge, usrState []byte) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	header := pushPullHeader{
		Nodes:        len(localNodes),
		UserStateLen: len(usrState),
	}
	hd := codec.MsgpackHandle{}
	enc := codec.NewEncoder(buf, &hd)

	if _, err := buf.Write([]byte{byte(pushPullMsg)}); err != nil {
		return nil, err
	}

	if err := enc.Encode(&header); err != nil {
		return nil, err
	}

	for i := 0; i < header.Nodes; i++ {
		if err := enc.Encode(&localNodes[i]); err != nil {
			return nil, err
		}
	}

	if len(usrState) != 0 {
		if _, err := buf.Write(usrState); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// r is used to read user state. skip for now
func (m *Memberlist) readRemoteState(r io.Reader, dec *codec.Decoder) ([]stateToMerge, []byte, error) {
	var header pushPullHeader
	if err := dec.Decode(&header); err != nil {
		return nil, nil, err
	}

	remoteNodes := make([]stateToMerge, header.Nodes)

	// Try to decode all the states
	for i := 0; i < header.Nodes; i++ {
		if err := dec.Decode(&remoteNodes[i]); err != nil {
			return nil, nil, err
		}
	}
	var usrState []byte
	if header.UserStateLen != 0 {
		usrState = make([]byte, header.UserStateLen)
		_, err := io.ReadFull(r, usrState)
		if err != nil {
			return nil, nil, err
		}
	}
	return remoteNodes, usrState, nil
}

func (m *Memberlist) mergeState(remote []stateToMerge) {
	for _, r := range remote {
		switch r.State {
		case StateAlive:
			a := alive{
				Lives: r.Lives,
				ID:    r.ID,
				IP:    r.IP,
				Port:  r.Port,
				Tags:  r.Tags,
			}
			m.aliveNode(&a, nil)
		case StateLeft:
			d := dead{Lives: r.Lives, ID: r.ID, Left: true}
			m.deadNode(&d, nil)
		case StateDead: // prefer suspect
			fallthrough
		case StateSuspect:
			s := suspect{Lives: r.Lives, ID: r.ID, From: m.ID()}
			m.suspectNode(&s)
		}
	}
}
