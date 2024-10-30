package memberlist

import (
	"bytes"
	"net"
	"reflect"
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
	ip := net.IPv4(127, 0, 0, 11)
	port := uint16(57732)
	msg := indirectPing{
		SeqNo:      42,
		ID:         "node 1",
		IP:         ip,
		Port:       port,
		SourceIP:   ip,
		SourcePort: port,
	}
	encoded, err := encode(indirectPingMsg, msg)
	require.Nil(t, err)
	buf, err := compress([]byte(encoded))
	require.Nil(t, err)
	decomp, err := decompressMsg(buf[1:])
	require.Nil(t, err)
	require.Equal(t, msgType(decomp[0]), indirectPingMsg)
	var decoded indirectPing
	decode(decomp[1:], &decoded)
	require.Equal(t, decoded.SeqNo, msg.SeqNo)
	require.Equal(t, decoded.ID, msg.ID)
	require.Equal(t, decoded.Port, msg.Port)
	require.Equal(t, decoded.SourcePort, msg.SourcePort)
	if !bytes.Equal(decoded.IP, msg.IP) || !bytes.Equal(decoded.SourceIP, msg.SourceIP) {
		t.Errorf("unmatched address")
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

	trunc, msgs, err := unpackCompoundMsg(compound[1:])
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	if trunc != 0 {
		t.Fatalf("should not truncate")
	}
	if len(msgs) != 3 {
		t.Fatalf("bad parts")
	}
	for _, p := range msgs {
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
	trunc, msgs, err := unpackCompoundMsg(compound[1:38]) // len(compound) = 53, missing 15 bytes. len(encoded) = 15
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	if trunc != 1 {
		t.Fatalf("should not truncate")
	}
	if len(msgs) != 2 {
		t.Fatalf("bad parts")
	}
	for _, p := range msgs {
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

func TestRandIntN(t *testing.T) {
	vals := make(map[int]struct{})
	for i := 0; i < 100; i++ {
		offset := randIntN(2 << 30)
		if _, ok := vals[offset]; ok {
			t.Fatalf("got collision")
		}
		vals[offset] = struct{}{}
	}
}

func TestRandIntN_Zero(t *testing.T) {
	if randIntN(0) != 0 {
		t.Fatalf("expect zero")
	}
}

func TestShuffleNodes(t *testing.T) {
	orig := []*nodeState{
		{
			State: StateDead,
		},
		{
			State: StateAlive,
		},
		{
			State: StateAlive,
		},
		{
			State: StateDead,
		},
		{
			State: StateAlive,
		},
		{
			State: StateAlive,
		},
		{
			State: StateDead,
		},
		{
			State: StateAlive,
		},
	}
	nodes := make([]*nodeState, len(orig))
	copy(nodes[:], orig[:])

	if !reflect.DeepEqual(nodes, orig) {
		t.Fatalf("should match")
	}

	shuffleNodes(nodes)

	if reflect.DeepEqual(nodes, orig) {
		t.Fatalf("should not match")
	}
}
