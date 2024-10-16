package memberlist

import (
	"math"
	"sync"

	"github.com/mbver/heap"
)

// call it broadcast or broadcastItem?
type TransmitCapItem struct {
	name      string
	id        uint64
	transmits int
	msg       []byte
	notify    chan<- struct{}
}

func (i *TransmitCapItem) ID() uint64 {
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
	idSeq         uint64
}

func (m *Memberlist) NewBroadcastQueue() *TransmitCapQueue {
	return &TransmitCapQueue{
		NumNodes:      m.numNodes,
		TransmitScale: m.config.RetransmitScale,
		queue:         *heap.NewHeap(),
		exists:        map[string]*TransmitCapItem{},
	}
}

func (q *TransmitCapQueue) GetMessages(overhead, size int) [][]byte {
	if size <= overhead || q.Len() == 0 {
		return nil
	}
	q.l.Lock()
	defer q.l.Unlock()
	var (
		bytesUsed int
		picked    []*TransmitCapItem
		notPicked []*TransmitCapItem
	)
	var item *TransmitCapItem
	for {
		// TODO: has a minimum threshold to avoid popping all the msgs!
		item = q.Pop()
		if item == nil {
			break
		}
		if bytesUsed+len(item.msg)+overhead > size {
			notPicked = append(notPicked, item)
			continue
		}
		picked = append(picked, item)
		bytesUsed += (len(item.msg) + overhead)
	}
	for _, item := range notPicked {
		q.Push(item)
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
			q.Push(item)
		}
	}
	if q.queue.Len() == 0 {
		q.idSeq = 0 // reset counter when no msg in queue
	}
	return msgs
}

func transmitLimit(scale, n int) int {
	nodeScale := int(math.Ceil(math.Log10(float64(n + 1))))
	return scale * nodeScale
}

func (q *TransmitCapQueue) QueueMsg(name string, t msgType, msg []byte, notify chan<- struct{}) {
	q.l.Lock()
	defer q.l.Unlock()

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
	item.id = q.idSeq
	q.idSeq++

	q.Push(item)
}

// caller has to hold the lock
func (q *TransmitCapQueue) Push(item *TransmitCapItem) {
	if item.name != "" {
		if old, ok := q.exists[item.name]; ok {
			old.Finish()
			q.queue.Remove(old.ID())
		}
		q.exists[item.name] = item
	}
	q.queue.Push(item)
}

func (q *TransmitCapQueue) Pop() *TransmitCapItem {
	item := q.queue.Pop()
	if item == nil {
		return nil
	}
	res := item.(*TransmitCapItem)
	delete(q.exists, res.name)
	return res
}

func (q *TransmitCapQueue) Len() int {
	q.l.Lock()
	defer q.l.Unlock()
	return q.queue.Len()
}

const userMsgOverhead = 1

type UserBroadcasts interface {
	GetBroadcasts(overhead, limit int) [][]byte
}

// get broadcasts from memberlist and consumer
func (m *Memberlist) getBroadcasts(overhead, limit int) [][]byte {
	if limit <= overhead {
		return nil
	}
	msgs := m.mbroadcasts.GetMessages(overhead, limit)
	if m.ubroadcasts != nil {
		bytesUsed := 0
		for _, msg := range msgs {
			bytesUsed += len(msg) + overhead
		}
		limit = limit - bytesUsed
		umsgs := m.ubroadcasts.GetBroadcasts(overhead+userMsgOverhead, limit)
		for _, msg := range umsgs {
			buf := make([]byte, 1, len(msg)+1)
			buf[0] = byte(userMsg)
			buf = append(buf, msg...)
			msgs = append(msgs, buf)
		}
	}
	return msgs
}

// name is used to invalidate message, name = "" will not invalidate
func (m *Memberlist) broadcast(name string, t msgType, msg []byte, notify chan<- struct{}) {
	m.mbroadcasts.QueueMsg(name, t, msg, notify)
}
