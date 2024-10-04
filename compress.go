package memberlist

import "bytes"

type compress struct{}

func (c *compress) decompress() ([]byte, error) {
	return nil, nil
}

func compressMsg(msg []byte) (*bytes.Buffer, error) {
	return nil, nil
}

func decompressMsg(msg []byte) ([]byte, error) {
	return nil, nil
}
