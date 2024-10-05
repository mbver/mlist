package memberlist

type BroadcastDelegate interface {
	GetBroadcasts() [][]byte
}

type broadcastManager struct {
	mbroadcasts        *TransmitCapQueue
	consumerBroadcasts BroadcastDelegate
}

// get broadcasts from memberlist and consumer
func (m *broadcastManager) GetMessages() [][]byte {
	return nil
}

// name is used to invalidate message, name = "" will not invalidate
func (m *broadcastManager) Queue(name string, t msgType, msg []byte, notify chan<- struct{}) {}
