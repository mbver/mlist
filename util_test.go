package memberlist

import (
	"bytes"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	msg := &ping{SeqNo: 100}
	bytes, err := encode(pingMsg, msg)
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	var out ping
	if err := decode(bytes[1:], &out); err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	if msg.SeqNo != out.SeqNo {
		t.Fatalf("bad sequence no")
	}
}

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
