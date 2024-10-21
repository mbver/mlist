package memberlist

type NodeEventType int

const (
	NodeJoin NodeEventType = iota
	NodeLeave
	NodeUpdate
)

type NodeEvent struct {
	Type NodeEventType
	Node *Node
}

type EventManager struct {
	ch chan<- *NodeEvent
}

func (mng *EventManager) NotifyJoin(n *Node) {
	if mng.ch != nil {
		mng.ch <- &NodeEvent{NodeJoin, n}
	}
}

func (mng *EventManager) NotifyLeave(n *Node) {
	if mng.ch != nil {
		mng.ch <- &NodeEvent{NodeLeave, n}
	}
}

func (mng *EventManager) NotifyUpdate(n *Node) {
	if mng.ch != nil {
		mng.ch <- &NodeEvent{NodeUpdate, n}
	}
}
