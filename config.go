package memberlist

import "time"

type Config struct {
	ID                      string // use a unique id instead of name
	BindAddr                string
	BindPort                int
	AdvertiseAddr           string
	AdvertisePort           int
	TcpTimeout              time.Duration
	PingTimeout             time.Duration
	ProbeTimeout            time.Duration
	Label                   string
	EnableCompression       bool
	EncryptionVersion       encryptionVersion
	UDPBufferSize           int // maximum size of a udp packet
	MaxLongRunQueueDepth    int
	MaxConcurentPushPull    int
	NumIndirectChecks       int
	RetransmitScale         int
	MaxAwarenessHealth      int
	SuspicionMult           int
	SuspicionMaxTimeoutMult int
}

func DefaultLANConfig(id string) *Config {
	return &Config{
		ID:                      id,
		BindAddr:                "0.0.0.0",
		BindPort:                7496,
		TcpTimeout:              10 * time.Second,
		PingTimeout:             500 * time.Millisecond,
		ProbeTimeout:            1500 * time.Millisecond, // at least 3 times more than PingTimeout
		NumIndirectChecks:       3,
		EnableCompression:       true,
		EncryptionVersion:       0,
		MaxLongRunQueueDepth:    1024,
		RetransmitScale:         4,
		MaxAwarenessHealth:      8,
		SuspicionMult:           4,
		SuspicionMaxTimeoutMult: 6,
	}
}
