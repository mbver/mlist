package memberlist

import (
	"net"
	"time"
)

type Config struct {
	ID                      string            `yaml:"id"`
	BindAddr                string            `yaml:"bind_addr"`
	BindPort                int               `yaml:"bind_port"`
	AdvertiseAddr           string            `yaml:"advertise_addr"`
	AdvertisePort           int               `yaml:"advertise_port"`
	CIDRsAllowed            []net.IPNet       `yaml:"cidrs_allowed"`
	Label                   string            `yaml:"label"`
	UDPBufferSize           int               `yaml:"udp_buffer_size"`
	DNSConfigPath           string            `yaml:"dns_config_path"`
	EnableCompression       bool              `yaml:"enable_compression"`
	EncryptionVersion       encryptionVersion `yaml:"encryption_version"`
	TcpTimeout              time.Duration     `yaml:"tcp_timeout"`
	NumIndirectChecks       int               `yaml:"num_indirect_checks"`
	PingTimeout             time.Duration     `yaml:"ping_timeout"`
	ProbeInterval           time.Duration     `yaml:"probe_interval"`
	MaxRTT                  time.Duration     `yaml:"max_rtt"`
	GossipNodes             int               `yaml:"gossip_nodes"`
	GossipInterval          time.Duration     `yaml:"gossip_interval"`
	DeadNodeExpiredTimeout  time.Duration     `yaml:"deadnode_expired_timeout"`
	BroadcastWaitTimeout    time.Duration     `yaml:"broadcast_wait_timeout"`
	EventTimeout            time.Duration     `yaml:"event_timeout"`
	ReapInterval            time.Duration     `yaml:"reap_interval"`
	MaxPushPulls            int               `yaml:"max_pushpulls"` // num of maximum concurrent pushpulls
	PushPullInterval        time.Duration     `yaml:"pushpull_interval"`
	MaxAwarenessHealth      int               `yaml:"max_awareness_health"`
	SuspicionMult           int               `yaml:"suspicion_mult"`
	SuspicionMaxTimeoutMult int               `yaml:"suspicion_max_timeout_mult"`
	MaxLongRunQueueDepth    int               `yaml:"max_longrun_queue_depth"`
	QueueCheckInterval      time.Duration     `yaml:"queue_check_interval"`
	RetransmitMult          int               `yaml:"retransmit_mult"`
	Tags                    []byte            `yaml:"tags"` // to be overwritten with serf's tag
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
		MaxRTT:                  30 * time.Millisecond,
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
