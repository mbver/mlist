package memberlist

import (
	"bytes"
	"compress/lzw"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/hashicorp/go-msgpack/v2/codec"
	"github.com/sean-/seed"
)

func init() {
	// Seed the random number generator
	seed.Init()

}
func encode(t msgType, in interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(uint8(t))
	h := codec.MsgpackHandle{}
	enc := codec.NewEncoder(buf, &h)
	if err := enc.Encode(in); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decode(buf []byte, out interface{}) error {
	r := bytes.NewReader(buf)
	h := codec.MsgpackHandle{}
	dec := codec.NewDecoder(r, &h)
	return dec.Decode(out)
}

// compoundMsg = type-len(msgs)-[]len(msg)-[]msg
func makeCompoundMsg(msgs [][]byte) []byte {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(uint8(compoundMsg))
	buf.WriteByte(uint8(len(msgs)))
	for _, m := range msgs {
		binary.Write(buf, binary.BigEndian, uint16(len(m)))
	}
	for _, m := range msgs {
		buf.Write(m)
	}
	return buf.Bytes()
}

func makeCompoundMsgs(msgs [][]byte) []*bytes.Buffer {
	return nil
}

func decodeCompoundMsg(buf []byte) (trunc int, parts [][]byte, err error) {
	return 0, nil, nil
}

func randIdxN(n int) int {
	return 0
}

func shuffleNodes(nodes []*nodeState) {}

func joinHostPort(host string, port uint16) string {
	return ""
}

// compression
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

func hasPort(s string) bool {
	return false
}

func ensurePort(s string, port int) string {
	return ""
}
