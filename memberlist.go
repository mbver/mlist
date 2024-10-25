package memberlist

import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-sockaddr"
)

type Memberlist struct {
	lives          uint32
	awr            *awareness
	config         *Config
	keyring        *Keyring
	transport      *NetTransport
	logger         *log.Logger
	shutdownL      sync.Mutex // guard shutdown, shutdownCh
	shutdownCh     chan struct{}
	shutdown       bool
	leaveL         sync.Mutex
	left           int32
	mbroadcasts    *TransmitCapQueue
	ubroadcasts    UserBroadcasts
	longRunMng     *longRunMsgManager
	usrMsgCh       chan<- []byte
	pingMng        *pingManager
	eventMng       *EventManager
	numPushPull    uint32
	nodeL          sync.RWMutex // guard nodes, nodeMap
	nodes          []*nodeState
	nodeMap        map[string]*nodeState
	numNodes       int32 // allow concurrent access
	suspicions     map[string]*suspicion
	stopScheduleCh chan struct{}
	probeIdx       int
}

type MemberlistBuilder struct {
	config       *Config
	keyring      *Keyring
	secretKey    []byte
	logger       *log.Logger
	eventCh      chan<- *NodeEvent
	pingDelegate PingDelegate
	ubroadcasts  UserBroadcasts
	usrMsgCh     chan<- []byte
}

func (b *MemberlistBuilder) WithConfig(c *Config) {
	b.config = c
}

func (b *MemberlistBuilder) WithKeyRing(r *Keyring) {
	b.keyring = r
}

func (b *MemberlistBuilder) WithSecretKey(k []byte) {
	b.secretKey = k
}

func (b *MemberlistBuilder) WithLogger(l *log.Logger) {
	b.logger = l
}

func (b *MemberlistBuilder) WithEventCh(ch chan<- *NodeEvent) {
	b.eventCh = ch
}

func (b *MemberlistBuilder) WithUserMessageCh(ch chan<- []byte) {
	b.usrMsgCh = ch
}

func (b *MemberlistBuilder) WithUserBroadcasts(u UserBroadcasts) {
	b.ubroadcasts = u
}

func (b *MemberlistBuilder) WithPingDelegate(p PingDelegate) {
	b.pingDelegate = p
}

// state delegate?

func (b *MemberlistBuilder) Build() (*Memberlist, error) {
	if b.secretKey != nil {
		if b.keyring != nil {
			if err := b.keyring.AddKey(b.secretKey); err != nil {
				return nil, err
			}
			if err := b.keyring.UseKey(b.secretKey); err != nil {
				return nil, err
			}
		} else {
			r, err := NewKeyring(nil, b.secretKey)
			if err != nil {
				return nil, err
			}
			b.keyring = r
		}
	}

	if b.logger == nil {
		b.logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	var t *NetTransport
	var err error
	if b.config.BindPort == 0 {
		t, err = ZeroBindPortTransport([]string{b.config.BindAddr}, b.logger)
		if err != nil {
			return nil, err
		}
		b.config.BindPort = t.BindPort()
	} else {
		t, err = NewNetTransport([]string{b.config.BindAddr}, b.config.BindPort, b.logger)
		if err != nil {
			return nil, err
		}
		err = t.Start()
		if err != nil {
			return nil, err
		}
	}

	m := &Memberlist{
		awr:            newAwareness(b.config.MaxAwarenessHealth),
		config:         b.config,
		keyring:        b.keyring,
		transport:      t,
		logger:         b.logger,
		shutdownCh:     make(chan struct{}),
		ubroadcasts:    b.ubroadcasts,
		usrMsgCh:       b.usrMsgCh,
		longRunMng:     newLongRunMsgManager(b.config.MaxLongRunQueueDepth),
		pingMng:        newPingManager(b.pingDelegate),
		eventMng:       &EventManager{b.eventCh},
		nodeMap:        map[string]*nodeState{},
		suspicions:     map[string]*suspicion{},
		stopScheduleCh: make(chan struct{}),
	}
	m.mbroadcasts = NewBroadcastQueue(m.getNumNodes, m.config.RetransmitMult)

	if err = m.start(); err != nil {
		m.Shutdown()
		return nil, err
	}

	return m, nil
}

func (m *Memberlist) start() error {
	if err := m.finalizeAdvertiseAddr(); err != nil {
		return err
	}
	hasPublic, err := m.hasPublicIface()
	if err != nil {
		return err
	}
	if hasPublic && !m.EncryptionEnabled() {
		return fmt.Errorf("encryption not enabled for public address")
	}

	if err := m.setAlive(); err != nil {
		return err
	}

	m.schedule()

	go m.receivePacket()
	go m.receiveTcpConn()
	go m.runLongRunMsgHandler()

	return nil
}

func (m *Memberlist) hasPublicIface() (bool, error) {
	addr, _, err := m.GetAdvertiseAddr()
	if err != nil {
		return false, err
	}
	ipAddr, err := sockaddr.NewIPAddr(addr.String())
	if err != nil {
		return false, fmt.Errorf("failed to parse interface addresses: %v", err)
	}
	ifAddrs := []sockaddr.IfAddr{
		{
			SockAddr: ipAddr,
		},
	}
	_, publicIfs, _ := sockaddr.IfByRFC("6890", ifAddrs)
	return len(publicIfs) > 0, nil
}

func (m *Memberlist) setAlive() error {
	addr, port, err := m.GetAdvertiseAddr()
	if err != nil {
		return err
	}
	a := alive{
		Lives: m.nextLiveNo(),
		ID:    m.config.ID,
		IP:    addr,
		Port:  port,
		Tags:  m.config.Tags,
	}
	m.aliveNode(&a, nil)
	return nil
}

func (m *Memberlist) Join(existing []string) (int, error) {
	numSuccess := 0
	var errs []error
	for _, exist := range existing {
		addrs, port, err := resolveAddr(exist, m.config.DNSConfigPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to resolve %s: %v", exist, err))
			m.logger.Printf("[WARN] memberlist: %v", err)
			continue
		}
		if port == 0 {
			port = uint16(m.config.BindPort)
		}
		for _, addr := range addrs {
			nodeAddr := joinHostPort(addr.String(), port)
			if err := m.pushPullWithNode(nodeAddr); err != nil {
				errs = append(errs, fmt.Errorf("failed to join %s: %v", addr, err))
				m.logger.Printf("[DEBUG] memberlist: %v", err)
				continue
			}
			numSuccess++
		}
	}
	if numSuccess > 0 {
		errs = nil
	}
	return numSuccess, combineErrors(errs)
}

func (m *Memberlist) Leave(timeout time.Duration) error {
	m.leaveL.Lock()
	defer m.leaveL.Unlock()

	if m.hasShutdown() {
		panic("leave after shutdown")
	}

	if m.hasLeft() {
		return nil
	}

	atomic.StoreInt32(&m.left, 1)

	m.nodeL.Lock()
	node, ok := m.nodeMap[m.config.ID]
	m.nodeL.Unlock()
	if !ok {
		m.logger.Printf("[WARN] memberlist: Leave but we're not in the node map.")
		return nil
	}
	d := dead{
		Lives: node.Lives,
		ID:    node.Node.ID,
		Left:  true,
	}
	notifyCh := make(chan struct{})
	m.deadNode(&d, notifyCh)
	if m.NumActive() == 0 || m.config.BroadcastWaitTimeout == 0 {
		return nil
	}
	// Block until the broadcast goes out
	select {
	case <-notifyCh:
	case <-time.After(m.config.BroadcastWaitTimeout):
		return fmt.Errorf("timeout waiting for leave broadcast")
	}
	return nil
}

func (m *Memberlist) UpdateNode(tags []byte) error {
	// Get the existing node
	m.nodeL.RLock()
	node := m.nodeMap[m.config.ID]
	m.nodeL.RUnlock()

	a := alive{
		Lives: m.nextLiveNo(),
		ID:    m.config.ID,
		IP:    node.Node.IP,
		Port:  node.Node.Port,
		Tags:  tags,
	}
	notifyCh := make(chan struct{})
	m.aliveNode(&a, notifyCh)

	if m.NumActive() == 0 || m.config.BroadcastWaitTimeout == 0 {
		return nil
	}
	// Wait for the broadcast or a timeout
	select {
	case <-notifyCh:
	case <-time.After(m.config.BroadcastWaitTimeout):
		return fmt.Errorf("timeout waiting for update broadcast")
	}
	return nil
}

func (m *Memberlist) NumActive() int {
	m.nodeL.RLock()
	defer m.nodeL.RUnlock()
	active := 0
	for _, n := range m.nodes {
		if !n.DeadOrLeft() {
			active++
		}
	}
	return active
}

func (m *Memberlist) getNumNodes() int {
	return int(atomic.LoadInt32(&m.numNodes))
}

func (m *Memberlist) Shutdown() {
	m.shutdownL.Lock()
	defer m.shutdownL.Unlock()
	if m.shutdown {
		return
	}
	m.deschedule()
	m.shutdown = true
	m.transport.Shutdown()
	close(m.shutdownCh)
}

func (m *Memberlist) hasShutdown() bool {
	m.shutdownL.Lock()
	defer m.shutdownL.Unlock()
	return m.shutdown
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
