package memberlist

import (
	"bytes"
	"testing"
)

func TestCompressDecompress(t *testing.T) {
	buf, err := compress([]byte("testing"))
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}

	decomp, err := decompressMsg(buf[1:])
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}

	if !bytes.Equal(decomp, []byte("testing")) {
		t.Fatalf("bad payload: %v", decomp)
	}
}
