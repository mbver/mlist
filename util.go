package memberlist

import (
	"bytes"
	"compress/lzw"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-msgpack/v2/codec"
	"github.com/sean-/seed"
)

func init() {
	// Seed the random number generator
	seed.Init()
}

func encode(t MsgType, in interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(uint8(t.Code()))
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

func splitToCompoundMsgs(msgs [][]byte) [][]byte {
	const maxMsgs = 255
	split := make([][]byte, 0, len(msgs)/maxMsgs+1)
	for len(msgs) > maxMsgs {
		split = append(split, packCompoundMsg(msgs[:maxMsgs]))
		msgs = msgs[maxMsgs:]
	}
	if len(msgs) > 0 {
		split = append(split, packCompoundMsg(msgs))
	}
	return split
}

// compoundMsg = type-len(msgs)-[]len(msg)-[]msg
func packCompoundMsg(msgs [][]byte) []byte {
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

func unpackCompoundMsg(msg []byte) (trunc int, msgs [][]byte, err error) {
	if len(msg) < 1 {
		err = fmt.Errorf("missing compound length byte")
		return
	}
	numParts := int(msg[0])
	msg = msg[1:]

	// Check we have enough bytes
	if len(msg) < numParts*2 {
		err = fmt.Errorf("truncated len slice")
		return
	}

	// Decode the lengths
	lengths := make([]uint16, numParts)
	for i := 0; i < numParts; i++ {
		lengths[i] = binary.BigEndian.Uint16(msg[i*2 : i*2+2])
	}
	msg = msg[numParts*2:]

	// Split each message
	for idx, msgLen := range lengths {
		if len(msg) < int(msgLen) {
			trunc = numParts - idx
			return
		}

		slice := msg[:msgLen]
		msg = msg[msgLen:]
		msgs = append(msgs, slice)
	}
	return
}

func randIntN(n int) int {
	if n == 0 { // if n == 0, modulo will panic
		return 0
	}
	return int(rand.Uint32() % uint32(n))
}

func shuffleNodes(nodes []*nodeState) {
	n := len(nodes)
	rand.Shuffle(n, func(i, j int) {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	})
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

func joinHostPort(host string, port uint16) string {
	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}

func UniqueID() string {
	id := uuid.New()
	str := id.String()
	return strings.ReplaceAll(str, "-", "")
}

func copyBytes(s []byte) []byte {
	res := make([]byte, len(s))
	copy(res, s)
	return s
}

func minSuspicionTimeout(scale, n int, interval time.Duration) time.Duration {
	nodeScale := math.Max(1.0, math.Log10(math.Max(1.0, float64(n))))
	return time.Duration(scale) * interval * time.Duration(nodeScale*1000) / 1000
}

func combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	sb := &strings.Builder{}
	for _, e := range errs {
		sb.WriteString(e.Error())
		sb.WriteString(";")
	}
	errMsg := strings.TrimSuffix(sb.String(), ";")
	return errors.New(errMsg)
}
