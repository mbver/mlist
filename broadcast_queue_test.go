package memberlist

import (
	"testing"

	"github.com/mbver/heap"
	"github.com/stretchr/testify/require"
)

func TestBroadcast_ItemLess(t *testing.T) {
	cases := []struct {
		name string
		less *TransmitCapItem
		more *TransmitCapItem
	}{
		{
			"different transmits",
			&TransmitCapItem{transmits: 0},
			&TransmitCapItem{transmits: 1},
		},
		{
			"same transmits, different msg length",
			&TransmitCapItem{transmits: 0, msg: []byte("abc")},
			&TransmitCapItem{transmits: 0, msg: []byte("a")},
		},
		{
			"same transmits, same msg length, different id",
			&TransmitCapItem{transmits: 0, msg: []byte("abc"), id: 100},
			&TransmitCapItem{transmits: 0, msg: []byte("abc"), id: 90},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !c.less.Less(c.more) {
				t.Errorf("less failed: %v not less than %v", c.less, c.more)
			}
		})
	}
}

func dumpQueue(q *TransmitCapQueue) []*TransmitCapItem {
	dump := []*TransmitCapItem{}
	q.l.Lock()
	defer q.l.Unlock()
	for {
		item := q.Pop()
		if item == nil {
			break
		}
		dump = append(dump, item)
	}
	for _, item := range dump {
		q.Push(item)
	}
	return dump
}

func TestBroadcast_QueueMsg(t *testing.T) {
	q := &TransmitCapQueue{TransmitScale: 1, NumNodes: func() int { return 1 }, queue: *heap.NewHeap(), exists: map[string]*TransmitCapItem{}}
	q.QueueMsg("a", 0, nil, nil)
	q.QueueMsg("b", 0, nil, nil)
	q.QueueMsg("c", 0, nil, nil)

	if q.Len() != 3 {
		t.Fatalf("bad length")
	}
	dump := dumpQueue(q)
	if dump[0].name != "c" {
		t.Errorf("item 0 expect %s, got %s", "c", dump[0].name)
	}
	if dump[1].name != "b" {
		t.Errorf("item 1 expect %s, got %s", "b", dump[1].name)
	}
	if dump[2].name != "a" {
		t.Errorf("item 2 expect %s, got %s", "a", dump[2].name)
	}

	// should invalidate old msg
	q.QueueMsg("c", 0, nil, nil)

	if q.Len() != 3 {
		t.Fatalf("bad length, expect: %d, got: %d", 3, q.Len())
	}
	dump = dumpQueue(q)
	if dump[0].name != "c" {
		t.Errorf("item 0 expect %s, got %s", "c", dump[0].name)
	}
	if dump[1].name != "b" {
		t.Errorf("item 1 expect %s, got %s", "b", dump[1].name)
	}
	if dump[2].name != "a" {
		t.Errorf("item 2 expect %s, got %s", "a", dump[2].name)
	}
}

func TestBroadcast_GetMessages(t *testing.T) {
	q := &TransmitCapQueue{TransmitScale: 3, NumNodes: func() int { return 10 }, queue: *heap.NewHeap(), exists: map[string]*TransmitCapItem{}}
	q.QueueMsg("", 0, []byte("1. this is a test."), nil) // 20 bytes each msg after encoding
	q.QueueMsg("", 0, []byte("2. this is a test."), nil)
	q.QueueMsg("", 0, []byte("3. this is a test."), nil)
	q.QueueMsg("", 0, []byte("4. this is a test."), nil)

	msgs := q.GetMessages(2, 88)

	require.Equal(t, 4, len(msgs), "expect 4 msgs")

	msgs = q.GetMessages(3, 88)

	require.Equal(t, 3, len(msgs), "expect 3 msgs")
}

func TestBroadcast_TransmitLimit(t *testing.T) {
	q := &TransmitCapQueue{TransmitScale: 1, NumNodes: func() int { return 10 }, queue: *heap.NewHeap(), exists: map[string]*TransmitCapItem{}}
	require.Equal(t, 2, transmitLimit(q.TransmitScale, q.NumNodes()), "transmit limit is not sane")
	require.Equal(t, uint64(0), q.idSeq, "initial idSeqNo is 0")

	q.QueueMsg("", 0, []byte("1. this is a test."), nil) // 20 bytes each msg after encoding
	q.QueueMsg("", 0, []byte("2. this is a test."), nil)
	q.QueueMsg("", 0, []byte("3. this is a test."), nil)
	q.QueueMsg("", 0, []byte("4. this is a test."), nil)

	// get messages until queue is empty

	msgs := q.GetMessages(3, 88)
	require.Equal(t, 3, len(msgs), "expect 3 messages")
	require.Equal(t, uint64(4), q.idSeq, "idSeq should not reset if queue is not empty")

	msgs = q.GetMessages(3, 88)
	require.Equal(t, 3, len(msgs), "expect 3 messages")
	require.Equal(t, uint64(4), q.idSeq, "idSeq should not reset if queue is not empty")

	msgs = q.GetMessages(3, 88)
	require.Equal(t, 2, len(msgs), "expect 2 messages")
	require.Equal(t, uint64(0), q.idSeq, "idSeq should not reset if queue is not empty")

	// queue is empty now
	msgs = q.GetMessages(3, 88)
	require.Equal(t, 0, len(msgs), "expect 2 messages")
	require.Equal(t, uint64(0), q.idSeq, "idSeq should not reset if queue is not empty")
}
