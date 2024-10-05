package memberlist

// call it broadcast or broadcastItem?
type TransmitCapItem struct{}

func (i *TransmitCapItem) Less(other interface{}) bool {
	return false
}

func (i *TransmitCapItem) Finish() {}

type TransmitCapQueue struct{}

func (q *TransmitCapQueue) GetMessages() [][]byte {
	return nil
}

func (q *TransmitCapQueue) Queue(item *TransmitCapItem) {}

func (q *TransmitCapQueue) Len() int {
	return 0
}

func (q *TransmitCapQueue) add(item *TransmitCapItem) {}

func (q *TransmitCapQueue) delete(item *TransmitCapItem) {}
