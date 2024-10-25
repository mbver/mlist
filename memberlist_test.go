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

func getCleanup(cleanups ...func()) func() {
	return func() {
		for _, f := range cleanups {
			if f != nil {
				f()
			}
		}
	}
}

func twoTestNodes() (*Memberlist, *Memberlist, func(), error) {
	m1, cleanup1, err := newTestMemberlist()
	if err != nil {
		return nil, nil, getCleanup(cleanup1), err
	}
	m2, cleanup2, err := newTestMemberlist()
	cleanup := getCleanup(cleanup1, cleanup2)
	if err != nil {
		return nil, nil, cleanup, err
	}
	return m1, m2, cleanup, nil
}

func threeTestNodes() (*Memberlist, *Memberlist, *Memberlist, func(), error) {
	m1, cleanup1, err := newTestMemberlist()
	if err != nil {
		return nil, nil, nil, getCleanup(cleanup1), err
	}
	m2, cleanup2, err := newTestMemberlist()
	if err != nil {
		return nil, nil, nil, getCleanup(cleanup1, cleanup2), err
	}
	m3, cleanup3, err := newTestMemberlist()
	cleanup := getCleanup(cleanup1, cleanup2, cleanup3)
	if err != nil {
		return nil, nil, nil, cleanup, err
	}
	return m1, m2, m3, cleanup, nil
}

func TestMemberlist_Create(t *testing.T) {
	_, cleanup, err := newTestMemberlist()
	if cleanup != nil {
		defer cleanup()
	}
	require.Nil(t, err)
}

func TestMemberlist_Join(t *testing.T) {
	m1, m2, cleanup, err := twoTestNodes()
	defer cleanup()
	require.Nil(t, err)
	addr := m2.LocalNodeState().Node.UDPAddress().String()
	nsuccess, err := m1.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	require.Equal(t, m1.NumActive(), 2)
	require.Equal(t, m2.NumActive(), 2)
}
