package memberlist

import (
	"sync"
	"sync/atomic"
	"time"
)

type Memberlist struct {
	lives       uint32
	awr         *awareness
	config      *Config
	keyring     *Keyring
	transport   *NetTransport
	shutdownL   sync.Mutex // guard shutdown, shutdownCh
	shutdownCh  chan struct{}
	shutdown    int32
	left        int32
	mbroadcasts *TransmitCapQueue
	ubroadcasts UserBroadcasts
	longRunMng  *longRunMsgManager
	pingMng     *pingManager
	eventMng    *EventManager
	numPushPull uint32
	nodeL       sync.RWMutex // guard nodes, nodeMap
	nodes       []*nodeState
	nodeMap     map[string]*nodeState
	numNodes    int32 // allow concurrent access
	suspicions  map[string]*suspicion
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

func (m *Memberlist) getNumNodes() int {
	return int(atomic.LoadInt32(&m.numNodes))
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
	return atomic.LoadInt32(&m.left) == 1
}

// return a clone of node state
func (m *Memberlist) GetNodeState(id string) *nodeState {
	m.nodeL.RLock()
	defer m.nodeL.RUnlock()
	n, ok := m.nodeMap[id]
	if !ok {
		return nil
	}
	return n.Clone()
}

func (m *Memberlist) LocalNodeState() *nodeState {
	return m.GetNodeState(m.config.ID)
}
