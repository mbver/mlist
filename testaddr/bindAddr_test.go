package testutil

import (
	"net"
	"testing"
	"time"
)

func TestBindAddr(t *testing.T) {
	bindAddrs := newAddressList(10, 20)
	for i := 10; i < 20; i++ {
		bindAddrs.NextAddr()
	}
	ipCh := make(chan net.IP)
	go func() {
		addr, cleanup := bindAddrs.NextAvailAddr()
		defer cleanup()
		ipCh <- addr
	}()
	timeout := 5 * time.Millisecond
	select {
	case <-ipCh:
		t.Fatalf("expect timeout")
	case <-time.After(timeout):
	}
	ip := net.IPv4(127, 0, 0, 15)
	var res net.IP
	bindAddrs.ReturnAddr(ip)
	select {
	case res = <-ipCh:
	case <-time.After(timeout):
		t.Fatalf("expect no timeout")
	}
	if !res.Equal(ip) {
		t.Fatalf("unmatched ip. expect %s, got %s", ip, res)
	}
}
