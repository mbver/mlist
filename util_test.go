package memberlist

import "testing"

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
