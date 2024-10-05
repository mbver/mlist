package memberlist

import "net"

func (m *Memberlist) isAddrAllowed(a net.Addr) error {
	return nil
}

func (m *Memberlist) IsIPAllowed(ip net.IP) error {
	return nil
}

func ParseCIDRs(v []string) ([]net.IPNet, error) {
	return nil, nil
}

type ipPort struct{}

func (m *Memberlist) resolveAddr(hostStr string) ([]ipPort, error) {
	return nil, nil
}

func (m *Memberlist) getAdvertise() (net.IP, uint16) {
	return nil, 0
}

func (m *Memberlist) setAdvertise(addr net.IP, port int) {}

// advertise address is still a mess with net transport.
