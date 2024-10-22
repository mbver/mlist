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

type stateToMerge struct {
	Lives uint32
	ID    string
	IP    net.IP
	Port  uint16
	Tags  []byte
	State StateType
}

type pushPullHeader struct {
	Nodes int
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
		return n.Node.ID != m.config.ID && n.State == StateAlive
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
	remoteNodes, err := m.sendAndReceiveState(addr)
	if err != nil {
		return err
	}

	m.mergeState(remoteNodes)
	return nil
}

func (m *Memberlist) sendAndReceiveState(addr string) ([]stateToMerge, error) {
	// Attempt to connect
	conn, err := m.transport.DialTimeout(addr, m.config.TcpTimeout)
	if err != nil { // timeout error if unable to connect
		return nil, err
	}
	defer conn.Close()
	m.logger.Printf("[DEBUG] memberlist: Initiating push/pull sync with: %s %s", addr, conn.RemoteAddr())

	// Send our state
	if err := m.sendLocalState(conn, m.config.Label); err != nil {
		return nil, err
	}

	conn.SetDeadline(time.Now().Add(m.config.TcpTimeout))
	msgType, bufConn, dec, err := m.unpackStream(conn, m.config.Label)

	if err != nil {
		return nil, err
	}

	if msgType == errMsg {
		var resp errResp
		if err := dec.Decode(&resp); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("remote error: %v", resp.Error)
	}
	// Quit if not push/pull
	if msgType != pushPullMsg {
		err := fmt.Errorf("received invalid msgType (%d), expected pushPullMsg (%d) %s", msgType, pushPullMsg, LogConn(conn))
		return nil, err
	}

	// Read remote state
	remoteNodes, err := m.readRemoteState(bufConn, dec)
	return remoteNodes, err
}

func (m *Memberlist) sendLocalState(conn net.Conn, streamLabel string) error {
	conn.SetDeadline(time.Now().Add(m.config.TcpTimeout))
	localState := m.localState()

	// Create a buffered writer
	msg, err := encodePushPullMsg(localState)
	if err != nil {
		return err
	}

	return m.sendTcp(conn, msg, streamLabel)
}

func (m *Memberlist) localState() []stateToMerge {
	return nil
}

func encodePushPullMsg(localNodes []stateToMerge) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	header := pushPullHeader{Nodes: len(localNodes)}
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
	return buf.Bytes(), nil
}

// r is used to read user state. skip for now
func (m *Memberlist) readRemoteState(r io.Reader, dec *codec.Decoder) ([]stateToMerge, error) {
	var header pushPullHeader
	if err := dec.Decode(&header); err != nil {
		return nil, err
	}

	remoteNodes := make([]stateToMerge, header.Nodes)

	// Try to decode all the states
	for i := 0; i < header.Nodes; i++ {
		if err := dec.Decode(&remoteNodes[i]); err != nil {
			return nil, err
		}
	}

	return remoteNodes, nil
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
			d := dead{Lives: r.Lives, ID: r.ID}
			m.deadNode(&d, nil)
		case StateDead: // prefer suspect
			fallthrough
		case StateSuspect:
			s := suspect{Lives: r.Lives, ID: r.ID, From: m.config.ID}
			m.suspectNode(&s)
		}
	}
}
