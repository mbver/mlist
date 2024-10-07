package memberlist

const userMsgOverhead = 1

const (
	pingMsg msgType = iota
	indirectPingMsg
	ackRespMsg
	suspectMsg
	aliveMsg
	deadMsg
	pushPullMsg
	compoundMsg
	userMsg // User mesg, not handled by us
	compressMsg
	encryptMsg
	nackRespMsg
	hasCrcMsg
	errMsg
)

type UserBroadcasts interface {
	GetBroadcasts(overhead, limit int) [][]byte
}

type broadcastManager struct {
	mbroadcasts *TransmitCapQueue
	ubroadcasts UserBroadcasts
}

// get broadcasts from memberlist and consumer
func (b *broadcastManager) GetMessages(overhead, limit int) [][]byte {
	if limit <= overhead {
		return nil
	}
	msgs := b.mbroadcasts.GetMessages(overhead, limit)
	if b.ubroadcasts != nil {
		bytesUsed := 0
		for _, msg := range msgs {
			bytesUsed += len(msg) + overhead
		}
		limit = limit - bytesUsed
		umsgs := b.ubroadcasts.GetBroadcasts(overhead+userMsgOverhead, limit)
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
func (b *broadcastManager) QueueMsg(name string, t msgType, msg []byte, notify chan<- struct{}) {
	b.mbroadcasts.QueueMsg(name, t, msg, notify)
}
