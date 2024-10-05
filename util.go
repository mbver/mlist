package memberlist

import (
	"bytes"

	"github.com/sean-/seed"
)

func init() {
	// Seed the random number generator
	seed.Init()

}
func encode(t msgType, in interface{}) (*bytes.Buffer, error) {
	return nil, nil
}

func decode(buf []byte, out interface{}) error {
	return nil
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
