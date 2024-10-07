package memberlist

import (
	"math"
	"sync"

	"github.com/mbver/heap"
)

// call it broadcast or broadcastItem?
type TransmitCapItem struct {
	name      string
	id        int
	transmits int
	msg       []byte
	notify    chan<- struct{}
}

func (i *TransmitCapItem) ID() int {
	return i.id
}

func (i *TransmitCapItem) Less(other heap.Item) bool {
	i1 := other.(*TransmitCapItem)
	if i.transmits == i1.transmits {
		if len(i.msg) == len(i1.msg) {
			return i.id > i1.id
		}
		return len(i.msg) > len(i1.msg)
	}
	return i.transmits < i1.transmits
}

func (i *TransmitCapItem) Finish() {
	select {
	case i.notify <- struct{}{}:
	default:
	}
}

type TransmitCapQueue struct {
	NumNodes      func() int
	TransmitScale int
	l             sync.Mutex
	queue         heap.Heap
	exists        map[string]*TransmitCapItem
	idSeq         int64
}

func (m *Memberlist) NewBroadcastQueue() *TransmitCapQueue {
	return &TransmitCapQueue{
		NumNodes: m.numNodes,
		// RetransmitScale: ,
		queue:  *heap.NewHeap(),
		exists: map[string]*TransmitCapItem{},
	}
}

func (q *TransmitCapQueue) GetMessages(overhead, limit int) [][]byte {
	if limit <= overhead {
		return nil
	}
	q.l.Lock()
	defer q.l.Unlock()
	if q.Len() == 0 {
		return nil
	}
	var (
		bytesUsed int
		picked    []*TransmitCapItem
		notPicked []*TransmitCapItem
	)
	var item *TransmitCapItem
	var free int
	for q.queue.Len() != 0 {
		free = limit - bytesUsed - overhead
		if free <= 0 {
			break
		}
		item = q.queue.Pop().(*TransmitCapItem)
		if len(item.msg) > free {
			notPicked = append(notPicked, item)
			continue
		}
		picked = append(picked, item)
		bytesUsed += len(item.msg)
	}

	for _, item := range notPicked {
		q.queueItem(item)
	}

	transLimit := transmitLimit(q.TransmitScale, q.NumNodes())
	msgs := [][]byte{}
	for _, item := range picked {
		msg := append([]byte{}, item.msg...) // copy
		msgs = append(msgs, msg)
		item.transmits++
		if item.transmits >= transLimit {
			item.Finish()
		} else {
			q.queueItem(item)
		}
	}
	return msgs
}

func transmitLimit(scale, n int) int {
	nodeScale := int(math.Ceil(math.Log10(float64(n + 1))))
	return scale * nodeScale
}

func (q *TransmitCapQueue) QueueMsg(name string, t msgType, msg []byte, notify chan<- struct{}) {
	msg, err := encode(t, msg)
	if err != nil {
		// log error
		return
	}
	item := &TransmitCapItem{
		name:   name,
		msg:    msg,
		notify: notify,
	}
	q.l.Lock()
	item.id = int(q.idSeq)
	if q.idSeq == math.MaxInt64 { // wrap around
		q.idSeq = 0
	}
	q.idSeq++
	q.l.Unlock()

	q.queueItem(item)
}

func (q *TransmitCapQueue) queueItem(item *TransmitCapItem) {
	q.l.Lock()
	defer q.l.Unlock()

	if item.name != "" {
		if old, ok := q.exists[item.name]; ok {
			old.Finish()
			q.queue.Remove(old)
		}
		q.exists[item.name] = item
	}
	q.queue.Push(item)
}

func (q *TransmitCapQueue) Len() int {
	return q.queue.Len()
}
