package memberlist

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-msgpack/v2/codec"
	"github.com/mbver/mlist/testaddr"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestNetPacking_PackUnpackUDP(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	msg := []byte("this is a message")

	packed, err := m.packUdp(msg)
	require.Nil(t, err, "pack msg failed")

	unpacked, err := m.unpackPacket(packed, m.config.Label)
	require.Nil(t, err, "unpack msg failed")

	if !bytes.Equal(unpacked, msg) {
		t.Fatalf("unmatched pack-unpack result. expect: %s, got: %s", msg, unpacked)
	}
}

func TestNetPacking_PackUnpackTCP(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	m.Shutdown() // stop udp, tcp receivers
	m.transport.Start()
	defer m.transport.Shutdown()

	msg := []byte("this is a message")
	encoded, err := encode(pushPullMsg, msg)
	require.Nil(t, err, "fail to encode message")

	packed, err := m.packTcp(encoded, m.config.Label)
	require.Nil(t, err)

	addr := m.LocalNodeState().Node.UDPAddress().String()
	timeout := 2 * time.Second
	conn, err := m.transport.DialTimeout(addr, timeout)
	require.Nil(t, err)
	defer conn.Close()

	recvConn := <-m.transport.TcpConnCh()
	defer recvConn.Close()
	recvConn.SetDeadline(time.Now().Add(timeout))

	_, err = conn.Write(packed)
	require.Nil(t, err)

	msgType, _, dec, err := m.unpackStream(recvConn, m.config.Label)
	require.Nil(t, err, "fail to unpack stream")
	require.Equal(t, msgType, pushPullMsg, "unmatched message type")

	out := make([]byte, len(msg))
	err = dec.Decode(out)
	require.Nil(t, err, "fail to decode msg")
	if !bytes.Equal(out, msg) {
		t.Fatalf("unmatched unpack msg stream, expect: %s, got: %s", msg, out)
	}
}

func newTestTransport() (*NetTransport, func(), error) {
	addr, cleanup := testaddr.BindAddrs.NextAvailAddr()
	addrs := []string{addr.String()}

	logger := log.New(os.Stderr, "testtransport", log.LstdFlags)
	var tr *NetTransport
	tr, err := NewNetTransport(addrs, 0, logger)
	if err != nil {
		return tr, cleanup, err
	}
	err = tr.Start()
	if err != nil {
		return nil, cleanup, err
	}
	cleanup1 := func() {
		tr.Shutdown()
		cleanup()
	}
	return tr, cleanup1, nil
}

func twoTransports() (*NetTransport, *NetTransport, func(), error) {
	t1, cleanup1, err := newTestTransport()
	if err != nil {
		return nil, nil, cleanup1, err
	}
	t2, cleanup2, err := newTestTransport()
	cleanup := getCleanup(cleanup1, cleanup2)
	if err != nil {
		return nil, nil, cleanup, err
	}
	return t1, t2, cleanup, err
}

func TestNetTransport_SendReceiveUDP(t *testing.T) {
	t1, t2, cleanup, err := twoTransports()
	defer cleanup()
	require.Nil(t, err)

	addr1, port1, err := t1.GetFirstAddr()
	require.Nil(t, err)
	udpAddr1 := net.UDPAddr{IP: addr1, Port: port1}

	addr2, port2, err := t2.GetFirstAddr()
	require.Nil(t, err)
	udpAddr2 := net.UDPAddr{IP: addr2, Port: port2}

	msg := ping{SeqNo: 42, ID: "Node1", SourceIP: addr1, SourcePort: uint16(port1)}
	encoded, err := encode(pingMsg, msg)
	require.Nil(t, err)

	t1.SendUdp(encoded, &udpAddr2)

	var packet *Packet
	select {
	case packet = <-t2.PacketCh():
	case <-time.After(5 * time.Millisecond):
		t.Fatalf("expect no timeout")
	}

	require.Equal(t, packet.From.String(), udpAddr1.String(), "expect matching FROM address")

	received := packet.Buf
	require.Equal(t, msgType(received[0]), pingMsg, "expect ping msg")

	var decoded ping
	err = decode(received[1:], &decoded)
	require.Nil(t, err)
	require.Equal(t, decoded.SeqNo, msg.SeqNo)
	require.Equal(t, decoded.ID, msg.ID)
	if !bytes.Equal(decoded.SourceIP, addr1) {
		t.Errorf("source addr not matched. expect %s, got %s", addr1, decoded.SourceIP)
	}
	require.Equal(t, decoded.SourcePort, uint16(port1))
}

func TestNetTransport_SendReceiveTCP(t *testing.T) {
	t1, t2, cleanup, err := twoTransports()
	defer cleanup()
	require.Nil(t, err)

	msg := ping{SeqNo: 42, ID: "Node1"}
	encoded, err := encode(pingMsg, msg)
	require.Nil(t, err)

	addr2, port2, err := t2.GetFirstAddr()
	require.Nil(t, err)
	tcpAddr2 := net.TCPAddr{IP: addr2, Port: port2}

	timeout := 2 * time.Second
	conn1, err := t1.DialTimeout(tcpAddr2.String(), timeout)
	require.Nil(t, err)
	defer conn1.Close()

	var conn2 net.Conn
	select {
	case conn2 = <-t2.TcpConnCh():
	case <-time.After(timeout):
		t.Fatalf("expect no timeout!")
	}
	defer conn2.Close()

	conn1.SetDeadline(time.Now().Add(timeout))
	conn2.SetDeadline(time.Now().Add(timeout))

	conn1.Write(encoded)

	buf := [1]byte{0}
	_, err = io.ReadFull(conn2, buf[:])
	require.Nil(t, err)
	require.Equal(t, msgType(buf[0]), pingMsg, "expect ping msg")

	hd := codec.MsgpackHandle{}
	dec := codec.NewDecoder(conn2, &hd)
	var decoded ping
	err = dec.Decode(&decoded)
	require.Nil(t, err)
	require.Equal(t, decoded.SeqNo, msg.SeqNo)
	require.Equal(t, decoded.ID, msg.ID)

	res := ack{msg.SeqNo, []byte("abc")}
	encoded, err = encode(ackMsg, res)
	require.Nil(t, err)
	conn2.Write(encoded)

	_, err = io.ReadFull(conn1, buf[:])
	require.Nil(t, err)
	require.Equal(t, msgType(buf[0]), ackMsg, "expect ack msg")

	var decRes ack
	dec = codec.NewDecoder(conn1, &hd)
	err = dec.Decode(&decRes)
	require.Nil(t, err)
	require.Equal(t, decRes.SeqNo, res.SeqNo)
	require.Equal(t, decRes.Payload, res.Payload)
}

// specialized to count error in tcp backoff
type countWriter struct {
	t        *testing.T
	numCalls *int32
}

func (w *countWriter) Write(p []byte) (n int, err error) {
	atomic.AddInt32(w.numCalls, 1)
	if !strings.Contains(string(p), "memberlist: Error accepting TCP connection") {
		w.t.Error("did not receive expected log message")
	}
	w.t.Log("record tcp error")
	return len(p), nil
}

func TestNetTransport_TcpBackoff(t *testing.T) {
	var numErr int32
	w := countWriter{t, &numErr}
	logger := log.New(&w, "test", log.LstdFlags)

	tr := NetTransport{
		tcpConnCh: make(chan net.Conn),
		logger:    logger,
	}

	tr.wg.Add(1)

	ip, cleanup := testaddr.BindAddrs.NextAvailAddr()
	addr := net.TCPAddr{
		IP:   ip,
		Port: 7946,
	}
	defer cleanup()
	l, err := net.ListenTCP("tcp", &addr)
	require.Nil(t, err)
	err = l.Close()
	require.Nil(t, err)

	go tr.tcpListen(l)

	time.Sleep(4 * time.Second)
	atomic.StoreInt32(&tr.shutdown, 1)

	done := make(chan struct{})
	go func() {
		tr.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for transport tcpListen to stop")
	}

	require.True(t, numErr > 8)
	require.True(t, numErr < 14)
}

func TestResolveAddr(t *testing.T) {
	m := &Memberlist{
		config: &Config{
			DNSConfigPath: "/etc/resolv.conf",
		},
	}

	cases := []struct {
		name string
		host string // the input address
		err  bool
		ips  []net.IP
		port uint16
	}{
		{
			name: "localhost",
			host: "localhost",
			port: 0,
		},
		{
			name: "ipv6 pair",
			host: "[::1]:80",
			ips:  []net.IP{net.IPv6loopback},
			port: 80,
		},
		{
			name: "ipv6 non-pair",
			host: "[::1]",
			ips:  []net.IP{net.IPv6loopback},
			port: 0,
		},
		{
			name: "hostless port",
			host: ":80",
			err:  true,
		},
		{
			name: "hostname port combo",
			host: "localhost:80",
			port: 80,
		},
		{
			name: "too high port",
			host: "localhost:80000",
			err:  true,
		},
		{
			name: "ipv4 port combo",
			host: "127.0.0.1:80",
			ips:  []net.IP{net.IPv4(127, 0, 0, 1)},
			port: 80,
		},
		{
			name: "ipv6 port combo",
			host: "[2001:db8:a0b:12f0::1]:80",
			ips:  []net.IP{{0x20, 0x01, 0x0d, 0xb8, 0x0a, 0x0b, 0x12, 0xf0, 0, 0, 0, 0, 0, 0, 0, 0x1}},
			port: 80,
		},
		{
			name: "ipv4 only",
			host: "127.0.0.1",
			ips:  []net.IP{net.IPv4(127, 0, 0, 1)},
			port: 0,
		},
		{
			name: "ipv6 only",
			host: "[2001:db8:a0b:12f0::1]",
			ips:  []net.IP{{0x20, 0x01, 0x0d, 0xb8, 0x0a, 0x0b, 0x12, 0xf0, 0, 0, 0, 0, 0, 0, 0, 0x1}},
			port: 0,
		},
	}

	for _, tc := range cases {
		tc := tc // store the current variable as the we run test in parallel
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ips, port, err := resolveAddr(tc.host, m.config.DNSConfigPath)
			if tc.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.port, port)
				if tc.ips != nil {
					require.Equal(t, tc.ips, ips)
				}
			}
		})
	}
}

type mockDNS struct {
	t *testing.T
}

func (h mockDNS) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) != 1 {
		h.t.Fatalf("bad: %#v", r.Question)
	}

	name := "join.service.consul."
	question := r.Question[0]
	// only accept query for "join.service.consul."
	if question.Name != name || question.Qtype != dns.TypeANY {
		h.t.Fatalf("bad: %#v", question)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.RecursionAvailable = false
	// set an ipv4 answer
	m.Answer = append(m.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET},
		A: net.ParseIP("127.0.0.1"),
	})
	// set an ipv6 answer
	m.Answer = append(m.Answer, &dns.AAAA{
		Hdr: dns.RR_Header{
			Name:   name,
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET},
		AAAA: net.ParseIP("2001:db8:a0b:12f0::1"),
	})
	if err := w.WriteMsg(m); err != nil {
		h.t.Fatalf("err: %v", err)
	}
}

func setupMockDNS(t *testing.T, bind string) (path string, cleanup func(), err error) {
	errCh := make(chan error, 1)
	startedCh := make(chan struct{})
	notifyStarted := func() { close(startedCh) }
	server := &dns.Server{
		Addr:              bind,
		Handler:           mockDNS{t},
		Net:               "tcp",
		NotifyStartedFunc: notifyStarted, // notify waiting wg when server started successfully
	}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-startedCh:
	case err = <-errCh:
		return "", nil, err
	case <-time.After(2 * time.Second):
		return "", nil, fmt.Errorf("timeout waiting for dns server to start")
	}

	cleanup = func() {
		server.Shutdown()
		if path != "" {
			os.Remove(path)
		}
	}

	// create a fake resolv.conf
	tmpFile, err := os.CreateTemp("", "")
	if err != nil {
		return "", cleanup, err
	}

	path = tmpFile.Name()

	content := []byte(fmt.Sprintf("nameserver %s", bind))
	if _, err := tmpFile.Write(content); err != nil {
		return "", cleanup, err
	}
	if err := tmpFile.Close(); err != nil {
		return "", cleanup, err
	}
	return path, cleanup, nil
}
func TestResolveAddr_TCP_First(t *testing.T) {
	// setup fake name server
	bind := "127.0.0.1:8600"
	path, cleanup, err := setupMockDNS(t, bind)
	if cleanup != nil {
		defer cleanup()
	}

	require.Nil(t, err)

	m := &Memberlist{
		config: &Config{
			DNSConfigPath: path,
		},
	}
	hosts := []string{
		"join.service.consul.",
		"join.service.consul", // the . will be added
	}

	for _, host := range hosts {
		ips, port, err := resolveAddr(host, m.config.DNSConfigPath)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		expectedIPs := []net.IP{
			net.ParseIP("127.0.0.1").To4(),
			net.ParseIP("2001:db8:a0b:12f0::1"),
		}
		require.Equal(t, expectedIPs, ips)
		require.Equal(t, uint16(0), port)
	}
}

func TestAdvertiseAddr(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	require.Equal(t, m.config.AdvertiseAddr, m.config.BindAddr)
	require.Equal(t, m.config.BindPort, m.config.AdvertisePort)

	ip, port, err := m.GetAdvertiseAddr()
	require.Nil(t, err)
	require.Equal(t, m.config.AdvertiseAddr, ip.String())
	require.Equal(t, m.config.BindPort, int(port))
}
