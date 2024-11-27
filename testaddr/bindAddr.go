package testaddr

import (
	"container/list"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

var BindAddrs *AddressList

const testPort = 10101

// maybe add addresses with different masks too?
func init() {
	BindAddrs = newAddressList(10, 255)
}

// create an AddressList from 127.0.0.from to 127.0.0.to excluded
func newAddressList(from, to byte) *AddressList {
	ips := list.New()
	for b := from; b < to; b++ {
		ip := net.IPv4(127, 0, 0, b)
		ips.PushBack(ip)
	}
	return &AddressList{
		ips:           ips,
		gotFreeAddrCh: make(chan struct{}),
	}
}

type AddressList struct {
	l             sync.Mutex
	ips           *list.List
	gotFreeAddrCh chan struct{}
}

func (a *AddressList) NextAddr() net.IP {
	a.l.Lock()

	// signal from gotFreeAddrCh may be sent when the queue is not empty.
	// by the time it is drained, the address may be long-gone
	// so must check ips again recursively until the true signal arrives
	if a.ips.Len() == 0 {
		a.l.Unlock()        // avoid deadlock with ReturnAddr
		<-a.gotFreeAddrCh   // wait for address returned
		return a.NextAddr() // recursively call itself to reach next lines
	}
	ip := a.ips.Front()
	a.ips.Remove(ip)
	a.l.Unlock()
	return ip.Value.(net.IP)
}

func (a *AddressList) ReturnAddr(ip net.IP) {
	a.l.Lock()
	defer a.l.Unlock()
	a.ips.PushBack(ip)
	select {
	case a.gotFreeAddrCh <- struct{}{}:
	default:
	}
}

func (a *AddressList) NextAvailAddr() (ip net.IP, cleanUpFn func()) {
	attempts := 0
	for {
		attempts++
		if attempts == 20 {
			panic("no available address. make sure setup_subnet.sh is run on MacOS")
		}
		ip = a.NextAddr()

		addr := &net.TCPAddr{IP: ip, Port: testPort}

		ln, err := net.ListenTCP("tcp4", addr)
		if err != nil {
			a.ReturnAddr(ip)
			continue
		}

		if attempts > 3 {
			fmt.Fprintf(os.Stdout, "testutil: took %s after %d attempts\n", ip, attempts)
		}
		return ip, func() {
			ln.Close()
			time.Sleep(50 * time.Millisecond) // let the kernel cool down
			a.ReturnAddr(ip)
		}
	}
}
