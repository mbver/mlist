package memberlist

import (
	"log"
	"time"
)

type NodeEventType int

const (
	NodeJoin NodeEventType = iota
	NodeLeave
	NodeUpdate
)

func (t NodeEventType) String() string {
	switch t {
	case NodeJoin:
		return "join"
	case NodeLeave:
		return "leave"
	case NodeUpdate:
		return "update"
	}
	return "unknown event"
}

type NodeEvent struct {
	Type NodeEventType
	Node *Node
}

type EventManager struct {
	ch      chan<- *NodeEvent
	timeout time.Duration
	logger  *log.Logger
}

func (mng *EventManager) NotifyJoin(n *Node) {
	mng.send(&NodeEvent{NodeJoin, n})
}

func (mng *EventManager) NotifyLeave(n *Node) {
	mng.send(&NodeEvent{NodeLeave, n})
}

func (mng *EventManager) NotifyUpdate(n *Node) {
	mng.send(&NodeEvent{NodeUpdate, n})
}

func (mng *EventManager) send(e *NodeEvent) {
	if mng.ch == nil {
		return
	}
	timeout := mng.timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	select {
	case mng.ch <- e:
	case <-time.After(timeout):
		mng.logger.Printf("[ERR] memberlist: Failed to send event")
	}
}
