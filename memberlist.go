package memberlist

import (
	"sync"
	"time"
)

type Memberlist struct {
	config      *Config
	keyring     *Keyring
	transport   *NetTransport
	mbroadcasts *TransmitCapQueue
	ubroadcasts UserBroadcasts
	shutdownCh  chan struct{}
	longRunMng  *longRunMsgManager
	numPushPull uint32
	pingMng     *pingManager
	nodeLock    sync.RWMutex
	nodes       []*nodeState
	nodeMap     map[string]*nodeState
}

type MemberlistBuilder struct{}

func (b *MemberlistBuilder) Build() *Memberlist {
	return nil
}

func (m *Memberlist) Start() error {
	return nil
}

func (m *Memberlist) Join(existing []string) (int, error) {
	return 0, nil
}

func (m *Memberlist) Leave(timeout time.Duration) error {
	return nil
}

func (m *Memberlist) LocalNode() *Node {
	return nil
}

func (m *Memberlist) UpdateNode(timeout time.Duration) error {
	return nil
}

func (m *Memberlist) NumActive() int {
	return 0
}

func (m *Memberlist) numNodes() int {
	return 0
}

// consider dropping
func (m *Memberlist) hasActivePeers() bool {
	return false
}

func (m *Memberlist) Shutdown() error {
	return nil
}

func (m *Memberlist) hasShutdown() bool {
	return false
}

func (m *Memberlist) hasLeft() bool {
	return false
}

// return a clone of node state
func (m *Memberlist) GetNodeState(id string) *nodeState {
	m.nodeLock.RLock()
	defer m.nodeLock.RUnlock()
	n, ok := m.nodeMap[id]
	if !ok {
		return nil
	}
	return &nodeState{
		Node: &Node{
			ID:   n.Node.ID,
			Addr: n.Node.Addr,
			Port: n.Node.Port,
		},
		Lives: n.Lives,
		State: n.State,
	}
}
