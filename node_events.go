package memberlist

import (
	"fmt"
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
	if err := mng.send(&NodeEvent{NodeJoin, n}); err != nil {
		mng.logger.Printf("error sending event %s", err)
	}
}

func (mng *EventManager) NotifyLeave(n *Node) {
	if err := mng.send(&NodeEvent{NodeLeave, n}); err != nil {
		mng.logger.Printf("error sending event %s", err)
	}
}

func (mng *EventManager) NotifyUpdate(n *Node) {
	if err := mng.send(&NodeEvent{NodeUpdate, n}); err != nil {
		mng.logger.Printf("error sending event %s", err)
	}
}

func (mng *EventManager) send(e *NodeEvent) error {
	if mng.ch == nil {
		return nil
	}
	timeout := mng.timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	select {
	case mng.ch <- e:
	case <-time.After(timeout):
		return fmt.Errorf("timeout sending event")
	}
	return nil
}
