package memberlist

import "time"

type Config struct {
	ID                   string // use a unique id instead of name
	Label                string
	EnableCompression    bool
	EncryptionVersion    encryptionVersion
	UDPBufferSize        int // maximum size of a udp packet
	AdvertiseAddr        string
	AdvertisePort        int
	MaxLongRunQueueDepth int
	TcpTimeout           time.Duration
	MaxConcurentPushPull int
}
