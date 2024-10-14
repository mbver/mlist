package memberlist

import "time"

type Config struct {
	ID                   string // use a unique id instead of name
	BindAddr             string
	BindPort             int
	AdvertiseAddr        string
	AdvertisePort        int
	TcpTimeout           time.Duration
	PingTimeout          time.Duration
	ProbeInterval        time.Duration
	Label                string
	EnableCompression    bool
	EncryptionVersion    encryptionVersion
	UDPBufferSize        int // maximum size of a udp packet
	MaxLongRunQueueDepth int
	MaxConcurentPushPull int
	NumIndirectChecks    int
}

func DefaultLANConfig(id string) *Config {
	return &Config{
		ID:                   id,
		BindAddr:             "0.0.0.0",
		BindPort:             7496,
		TcpTimeout:           10 * time.Second,
		PingTimeout:          500 * time.Millisecond,
		ProbeInterval:        1 * time.Second,
		NumIndirectChecks:    3,
		EnableCompression:    true,
		EncryptionVersion:    0,
		MaxLongRunQueueDepth: 1024,
	}
}
