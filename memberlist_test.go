package memberlist

import (
	"log"
	"net"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/mbver/mlist/testaddr"
	"github.com/stretchr/testify/require"
)

func newTestMemberlist(ip net.IP) (*Memberlist, func(), error) {
	b := &MemberlistBuilder{}
	key := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	keyRing, err := NewKeyring(nil, key)
	if err != nil {
		return nil, nil, err
	}
	b.WithKeyRing(keyRing)

	config := DefaultLANConfig()
	config.Label = "label"
	cleanup := func() {}
	if ip == nil {
		ip, cleanup = testaddr.BindAddrs.NextAvailAddr()
	}
	config.BindAddr = ip.String()
	config.ID = config.BindAddr
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
	m1, cleanup1, err := newTestMemberlist(nil)
	if err != nil {
		return nil, nil, getCleanup(cleanup1), err
	}
	m2, cleanup2, err := newTestMemberlist(nil)
	cleanup := getCleanup(cleanup1, cleanup2)
	if err != nil {
		return nil, nil, cleanup, err
	}
	return m1, m2, cleanup, nil
}

func threeTestNodes() (*Memberlist, *Memberlist, *Memberlist, func(), error) {
	m1, cleanup1, err := newTestMemberlist(nil)
	if err != nil {
		return nil, nil, nil, getCleanup(cleanup1), err
	}
	m2, cleanup2, err := newTestMemberlist(nil)
	if err != nil {
		return nil, nil, nil, getCleanup(cleanup1, cleanup2), err
	}
	m3, cleanup3, err := newTestMemberlist(nil)
	cleanup := getCleanup(cleanup1, cleanup2, cleanup3)
	if err != nil {
		return nil, nil, nil, cleanup, err
	}
	return m1, m2, m3, cleanup, nil
}

func TestMemberlist_ActiveNodes(t *testing.T) {
	n1 := &Node{ID: "test"}
	n2 := &Node{ID: "test2"}
	n3 := &Node{ID: "test3"}

	m := &Memberlist{}
	nodes := []*nodeState{
		{Node: n1, State: StateAlive},
		{Node: n2, State: StateDead},
		{Node: n3, State: StateSuspect},
	}
	m.nodes = nodes

	members := m.ActiveNodes()
	if !reflect.DeepEqual(members, []*Node{n1, n3}) {
		t.Fatalf("bad members")
	}
}

func TestMemberlist_Create(t *testing.T) {
	m, cleanup, err := newTestMemberlist(nil)
	if cleanup != nil {
		defer cleanup()
	}
	require.Nil(t, err)
	require.Equal(t, m.NumActive(), 1)
	require.Equal(t, m.ActiveNodes()[0].ID, m.config.ID)
}

func sortNodes(nodes []*Node) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
}

func testJoinState(t *testing.T, mlists ...*Memberlist) {
	nodes0 := mlists[0].ActiveNodes()
	sortNodes(nodes0)
	for _, m := range mlists {
		require.Equal(t, m.NumActive(), len(mlists))
		nodes := m.ActiveNodes()
		sortNodes(nodes)
		if !reflect.DeepEqual(nodes0, nodes) {
			t.Fatalf("nodes not matching")
		}
	}
}
func TestMemberlist_Join(t *testing.T) {
	m1, m2, cleanup, err := twoTestNodes()
	defer cleanup()
	require.Nil(t, err)
	addr := m2.LocalNodeState().Node.UDPAddress().String()
	nsuccess, err := m1.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	testJoinState(t, m1, m2)
}

func TestMemberlist_JoinUniqueNetworkMask(t *testing.T) {
	m1, m2, cleanup, err := twoTestNodes()
	defer cleanup()
	require.Nil(t, err)
	m1.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/8"})
	require.Nil(t, err)
	m2.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/8"})
	require.Nil(t, err)
	addr := m2.LocalNodeState().Node.UDPAddress().String()
	nsuccess, err := m1.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	testJoinState(t, m1, m2)
}

func TestMemberlist_JoinMultiNetworkMasks(t *testing.T) {
	m1, cleanup1, err := newTestMemberlist(nil)
	if cleanup1 != nil {
		defer cleanup1()
	}
	require.Nil(t, err)
	m1.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/24", "127.0.1.0/24"})
	require.Nil(t, err)

	m2, cleanup2, err := newTestMemberlist(net.IPv4(127, 0, 1, 11))
	if cleanup2 != nil {
		defer cleanup2()
	}
	require.Nil(t, err)
	m2.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/24", "127.0.1.0/24"})
	require.Nil(t, err)

	addr := m2.LocalNodeState().Node.UDPAddress().String()
	nsuccess, err := m1.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	testJoinState(t, m1, m2)

	// rouge node from a different network but can "see" m1 and m2
	m3, cleanup3, err := newTestMemberlist(net.IPv4(127, 0, 2, 10))
	if cleanup3 != nil {
		defer cleanup3()
	}
	require.Nil(t, err)
	m3.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/8"})
	require.Nil(t, err)

	nsuccess, err = m3.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	testJoinState(t, m1, m2) // m1, m2 don't see m3

	// rogue node can see m1 and m2 but can not see itself!
	m4, cleanup4, err := newTestMemberlist(net.IPv4(127, 0, 2, 11))
	if cleanup4 != nil {
		defer cleanup3()
	}
	require.Nil(t, err)
	m4.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/24", "127.0.1.0/24"})
	require.Nil(t, err)

	nsuccess, err = m4.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	testJoinState(t, m1, m2) // m1, m2 don't see m3 and m4
}
