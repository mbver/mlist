package memberlist

import (
	"net"
	"time"
)

type Config struct {
	ID                      string // use a unique id instead of name
	BindAddr                string
	BindPort                int
	AdvertiseAddr           string
	AdvertisePort           int
	CIDRsAllowed            []net.IPNet
	Label                   string
	UDPBufferSize           int // maximum size of a udp packet
	DNSConfigPath           string
	EnableCompression       bool
	EncryptionVersion       encryptionVersion
	TcpTimeout              time.Duration
	NumIndirectChecks       int
	PingTimeout             time.Duration
	ProbeTimeout            time.Duration
	ProbeInterval           time.Duration
	GossipNodes             int
	GossipInterval          time.Duration
	DeadNodeExpiredTimeout  time.Duration
	ReapInterval            time.Duration
	ReconnectInterval       time.Duration
	MaxPushPulls            int
	PushPullInterval        time.Duration
	MaxAwarenessHealth      int
	SuspicionMult           int
	SuspicionMaxTimeoutMult int
	MaxLongRunQueueDepth    int
	QueueCheckInterval      time.Duration
	RetransmitMult          int
	Tags                    []byte
}

func DefaultLANConfig(id string) *Config {
	return &Config{
		ID:                      id,
		BindAddr:                "0.0.0.0",
		BindPort:                7496,
		TcpTimeout:              10 * time.Second,
		PingTimeout:             500 * time.Millisecond,
		ProbeInterval:           1500 * time.Millisecond, // at least 3 times more than PingTimeout
		NumIndirectChecks:       3,
		EnableCompression:       true,
		EncryptionVersion:       0,
		DNSConfigPath:           "/etc/resolv.conf",
		MaxLongRunQueueDepth:    1024,
		RetransmitMult:          4,
		MaxAwarenessHealth:      8,
		SuspicionMult:           4,
		SuspicionMaxTimeoutMult: 6,
	}
}
