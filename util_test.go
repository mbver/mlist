package memberlist

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestCompounMsg_PackUnpack(t *testing.T) {
	msg := &ping{SeqNo: 100}
	encoded, err := encode(pingMsg, msg)
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}

	msgs := [][]byte{encoded, encoded, encoded}
	compound := packCompoundMsg(msgs)
	require.Equal(t, msgType(compound[0]), compoundMsg, "compoundMsg type")

	trunc, parts, err := unpackCompoundMsg(compound[1:])
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	if trunc != 0 {
		t.Fatalf("should not truncate")
	}
	if len(parts) != 3 {
		t.Fatalf("bad parts")
	}
	for _, p := range parts {
		if !bytes.Equal(encoded, p) {
			t.Errorf("unmatched msg after unpacking")
		}
	}
}

func TestCompoundMsg_MissingBytes(t *testing.T) {
	msg := &ping{SeqNo: 100}
	encoded, err := encode(pingMsg, msg)
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}

	msgs := [][]byte{encoded, encoded, encoded}
	compound := packCompoundMsg(msgs)
	require.Equal(t, msgType(compound[0]), compoundMsg, "compoundMsg type")

	trunc, parts, err := unpackCompoundMsg(compound[1:38])
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	if trunc != 1 {
		t.Fatalf("should not truncate")
	}
	if len(parts) != 2 {
		t.Fatalf("bad parts")
	}
	for _, p := range parts {
		if !bytes.Equal(encoded, p) {
			t.Errorf("unmatched msg after unpacking")
		}
	}
}

func TestCompoundMsg_MissingMsgLengths(t *testing.T) {
	buf := []byte{0xff}
	_, _, err := unpackCompoundMsg(buf)
	require.Error(t, err)
	require.Equal(t, err.Error(), "truncated len slice")
}
