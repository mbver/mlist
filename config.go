package memberlist

type Config struct {
	Label             string
	EnableCompression bool
	EncryptionVersion encryptionVersion
	UDPBufferSize     int // maximum size of a udp packet
}
