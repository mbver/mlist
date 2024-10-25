package memberlist

import (
	"log"
	"os"
	"testing"

	"github.com/mbver/mlist/testaddr"
	"github.com/stretchr/testify/require"
)

func newTestMemberlist() (*Memberlist, func(), error) {
	b := &MemberlistBuilder{}
	key := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	keyRing, err := NewKeyring(nil, key)
	if err != nil {
		return nil, nil, err
	}
	b.WithKeyRing(keyRing)

	config := DefaultLANConfig()
	config.Label = "label"
	addr, cleanup := testaddr.BindAddrs.NextAvailAddr()
	config.ID = addr.String()
	config.BindAddr = addr.String()
	logger := log.New(os.Stderr, "mtest-"+config.ID+": ", log.LstdFlags)
	b.WithLogger(logger)
	b.WithConfig(config)
	m, err := b.Build()
	if err != nil {
		return nil, cleanup, err
	}
	cleanup1 := func() {
		m.Shutdown()
		cleanup()
	}
	return m, cleanup1, nil
}
func TestMemberlist_Create(t *testing.T) {
	_, cleanup, err := newTestMemberlist()
	if cleanup != nil {
		defer cleanup()
	}
	require.Nil(t, err)

}
