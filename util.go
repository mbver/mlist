package memberlist

import (
	"bytes"

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

func makeCompoundMsg(msgs [][]byte) []byte {
	return nil
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
