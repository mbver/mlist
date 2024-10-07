package memberlist

import "bytes"

type compressor struct{}

func (c *compressor) decompress() ([]byte, error) {
	return nil, nil
}

func compress(msg []byte) (*bytes.Buffer, error) {
	return nil, nil
}

func decompressMsg(msg []byte) ([]byte, error) {
	return nil, nil
}
