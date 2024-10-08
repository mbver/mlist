package memberlist

import (
	"bytes"
	"compress/lzw"
	"fmt"
	"io"
)

type compressionType uint8

const (
	lzwAlgo compressionType = iota
)

const (
	// Constant litWidth 2-8
	lzwLitWidth = 8
)

type compressed struct {
	Algo compressionType
	Buf  []byte
}

func (c *compressed) decompress() ([]byte, error) {
	if c.Algo != lzwAlgo {
		return nil, fmt.Errorf("cannot decompress unknown algorithm %d", c.Algo)
	}

	// Create a uncompressor
	uncomp := lzw.NewReader(bytes.NewReader(c.Buf), lzw.LSB, lzwLitWidth)
	defer uncomp.Close()

	// Read all the data
	var b bytes.Buffer
	_, err := io.Copy(&b, uncomp)
	if err != nil {
		return nil, err
	}

	// Return the uncompressed bytes
	return b.Bytes(), nil
}

func compress(msg []byte) ([]byte, error) {
	var buf bytes.Buffer
	compressor := lzw.NewWriter(&buf, lzw.LSB, lzwLitWidth)

	_, err := compressor.Write(msg)
	if err != nil {
		return nil, err
	}

	// flush
	if err := compressor.Close(); err != nil {
		return nil, err
	}

	// create compressed object
	c := compressed{
		Algo: lzwAlgo,
		Buf:  buf.Bytes(),
	}
	return encode(compressMsg, &c)
}

func decompressMsg(msg []byte) ([]byte, error) {
	var c compressed
	if err := decode(msg, &c); err != nil {
		return nil, err
	}
	return c.decompress()
}
