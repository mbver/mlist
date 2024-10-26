package memberlist

import (
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/mbver/mlist/testaddr"
	"github.com/stretchr/testify/require"
)

func defaultTestConfig() *Config {
	conf := DefaultLANConfig()
	conf.TcpTimeout = 50 * time.Millisecond
	conf.PingTimeout = 20 * time.Millisecond
	conf.ProbeInterval = 50 * time.Millisecond
	conf.RetransmitMult = 2
	conf.SuspicionMaxTimeoutMult = 1
	return conf
}

func newTestMemberlist(ip net.IP, port int) (*Memberlist, func(), error) {
	cleanup := func() {}
	b := &MemberlistBuilder{}
	key := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	keyRing, err := NewKeyring(nil, key)
	if err != nil {
		return nil, cleanup, err
	}
	b.WithKeyRing(keyRing)

	config := defaultTestConfig()
	config.Label = "label"
	if ip == nil {
		ip, cleanup = testaddr.BindAddrs.NextAvailAddr()
	}
	config.BindAddr = ip.String()
	config.ID = config.BindAddr
	if port != 0 {
		config.BindPort = port
		config.ID = fmt.Sprintf("%s:%d", config.BindAddr, config.BindPort)
	}
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
			f()
		}
	}
}

func twoTestNodes() (*Memberlist, *Memberlist, func(), error) {
	m1, cleanup1, err := newTestMemberlist(nil, 0)
	if err != nil {
		return nil, nil, getCleanup(cleanup1), err
	}
	m2, cleanup2, err := newTestMemberlist(nil, 0)
	cleanup := getCleanup(cleanup1, cleanup2)
	if err != nil {
		return nil, nil, cleanup, err
	}
	return m1, m2, cleanup, nil
}

func threeTestNodes() (*Memberlist, *Memberlist, *Memberlist, func(), error) {
	m1, cleanup1, err := newTestMemberlist(nil, 0)
	if err != nil {
		return nil, nil, nil, getCleanup(cleanup1), err
	}
	m2, cleanup2, err := newTestMemberlist(nil, 0)
	if err != nil {
		return nil, nil, nil, getCleanup(cleanup1, cleanup2), err
	}
	m3, cleanup3, err := newTestMemberlist(nil, 0)
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
	m, cleanup, err := newTestMemberlist(nil, 0)
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

func TestMemberlist_JoinSingleNetMask(t *testing.T) {
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

func TestMemberlist_JoinMultiNetMasks(t *testing.T) {
	m1, cleanup1, err := newTestMemberlist(nil, 0)
	defer cleanup1()
	require.Nil(t, err)
	m1.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/24", "127.0.1.0/24"})
	require.Nil(t, err)

	m2, cleanup2, err := newTestMemberlist(net.IPv4(127, 0, 1, 11), 0)
	defer cleanup2()
	require.Nil(t, err)
	m2.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/24", "127.0.1.0/24"})
	require.Nil(t, err)

	addr := m2.LocalNodeState().Node.UDPAddress().String()
	nsuccess, err := m1.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	testJoinState(t, m1, m2)

	// rouge node from a different network but can "see" m1 and m2
	m3, cleanup3, err := newTestMemberlist(net.IPv4(127, 0, 2, 10), 0)
	defer cleanup3()
	require.Nil(t, err)
	m3.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/8"})
	require.Nil(t, err)

	nsuccess, err = m3.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	testJoinState(t, m1, m2) // m1, m2 don't see m3

	// rogue node can see m1 and m2 but can not see itself!
	m4, cleanup4, err := newTestMemberlist(net.IPv4(127, 0, 2, 11), 0)
	defer cleanup4()
	require.Nil(t, err)
	m4.config.CIDRsAllowed, err = ParseCIDRs([]string{"127.0.0.0/24", "127.0.1.0/24"})
	require.Nil(t, err)

	nsuccess, err = m4.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	testJoinState(t, m1, m2) // m1, m2 don't see m3 and m4
}

func ipv6LoopbackOK(t *testing.T) bool {
	const ipv6LoopbackAddress = "::1"
	ifaces, err := net.Interfaces()
	require.NoError(t, err)

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		require.NoError(t, err)

		for _, addr := range addrs {
			ipaddr := addr.(*net.IPNet)
			if ipaddr.IP.String() == ipv6LoopbackAddress {
				return true
			}
		}
	}
	return false
}

func TestMemberlist_Join_IPv6(t *testing.T) {
	if !ipv6LoopbackOK(t) {
		t.SkipNow()
		return
	}

	m1, cleanup1, err := newTestMemberlist(net.IPv6loopback, 23456)
	defer cleanup1()
	require.Nil(t, err)

	m2, cleanup2, err := newTestMemberlist(net.IPv6loopback, 23457)
	defer cleanup2()
	require.Nil(t, err)

	addr := m2.LocalNodeState().Node.UDPAddress().String()
	nsuccess, err := m1.Join([]string{addr})
	require.Nil(t, err)
	require.Equal(t, nsuccess, 1)
	testJoinState(t, m1, m2)
}

func TestMemberlist_Join_DeadNode(t *testing.T) {
	m1, cleanup1, err := newTestMemberlist(nil, 0)
	defer cleanup1()
	require.Nil(t, err)

	// a fake node, can connect but doesn't respond
	addr2, cleanup2 := testaddr.BindAddrs.NextAvailAddr()
	defer cleanup2()
	addrStr2 := fmt.Sprintf("%s:%d", addr2, 7946)
	l, err := net.Listen("tcp", addrStr2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer l.Close()

	// Ensure we don't hang forever
	timer := time.AfterFunc(100*time.Millisecond, func() {
		t.Fatalf("should have timeout by now")
	})
	defer timer.Stop()

	// can write to remote conn but will not able to read
	num, err := m1.Join([]string{addrStr2})
	require.NotNil(t, err)
	require.Equal(t, num, 0)
}

func waitForCond(check func() bool) bool {
	t := time.NewTicker(5 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if check() {
				return true
			}
		case <-time.After(20 * time.Second):
			return false
		}
	}
}

func TestMemberlist_JoinShutdown(t *testing.T) {
	m1, cleanup1, err := newTestMemberlist(nil, 0)
	require.Nil(t, err)
	defer cleanup1()

	m2, cleanup2, err := newTestMemberlist(nil, 0)
	require.Nil(t, err)
	defer cleanup2()

	addr2 := m2.LocalNodeState().Node.UDPAddress().String()
	n, err := m1.Join([]string{addr2})
	require.Nil(t, err)
	require.Equal(t, n, 1)
	testJoinState(t, m1, m2)
	m1.Shutdown()
	if !waitForCond(func() bool {
		return m2.NumActive() == 1
	}) {
		t.Fatalf("expect %d node, got %d", 1, m2.NumActive())
	}
}

func TestMemberlist_Leave(t *testing.T) {}
