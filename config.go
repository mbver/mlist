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
	ProbeInterval           time.Duration
	GossipNodes             int
	GossipInterval          time.Duration
	DeadNodeExpiredTimeout  time.Duration
	BroadcastWaitTimeout    time.Duration
	EventTimeout            time.Duration
	ReapInterval            time.Duration
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

func DefaultLANConfig() *Config {
	return &Config{
		ID:                      "",
		BindAddr:                "0.0.0.0",
		BindPort:                7946,
		AdvertiseAddr:           "",
		AdvertisePort:           7946,
		UDPBufferSize:           1400,
		DNSConfigPath:           "/etc/resolv.conf",
		EnableCompression:       true,
		EncryptionVersion:       0,
		TcpTimeout:              10 * time.Second,
		NumIndirectChecks:       3,
		PingTimeout:             500 * time.Millisecond,
		ProbeInterval:           1100 * time.Millisecond, // more than 2 times of PingTimeout so IndirectPing can get Nacks
		GossipNodes:             3,
		GossipInterval:          200 * time.Millisecond,
		DeadNodeExpiredTimeout:  30 * time.Second,
		BroadcastWaitTimeout:    3 * time.Second,
		EventTimeout:            5 * time.Second,
		ReapInterval:            3 * time.Second,
		MaxPushPulls:            128,
		PushPullInterval:        30 * time.Second,
		MaxAwarenessHealth:      8,
		SuspicionMult:           4,
		SuspicionMaxTimeoutMult: 6,
		MaxLongRunQueueDepth:    1024,
		QueueCheckInterval:      30 * time.Second,
		RetransmitMult:          4,
	}
}
