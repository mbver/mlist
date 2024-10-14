package memberlist

import (
	"sync"
	"sync/atomic"
	"time"
)

type Memberlist struct {
	config      *Config
	keyring     *Keyring
	transport   *NetTransport
	shutdownCh  chan struct{}
	shutdown    int32
	mbroadcasts *TransmitCapQueue
	ubroadcasts UserBroadcasts
	longRunMng  *longRunMsgManager
	pingMng     *pingManager
	numPushPull uint32
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

func (m *Memberlist) Shutdown() {
	if !atomic.CompareAndSwapInt32(&m.shutdown, 0, 1) {
		return
	}
	m.transport.Shutdown()
	close(m.shutdownCh)
	// TODO: deschedule
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
	return n.Clone()
}

func (m *Memberlist) LocalNodeState() *nodeState {
	return m.GetNodeState(m.config.ID)
}
