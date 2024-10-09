package memberlist

import "time"

type Memberlist struct {
	config  *Config
	keyring *Keyring
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
