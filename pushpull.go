package memberlist

import (
	"io"
	"net"

	"github.com/hashicorp/go-msgpack/v2/codec"
)

func (m *Memberlist) pushPull() {}

func (m *Memberlist) pushPullWithNode(a net.Addr) error {
	return nil
}

func (m *Memberlist) sendAndReceiveState(a net.Addr) ([]stateToMerge, []byte, error) {
	return nil, nil, nil
}

func (m *Memberlist) sendLocalState(conn net.Conn, streamLabel string) error {
	return nil
}

func (m *Memberlist) localState() []stateToMerge {
	return nil
}

func (m *Memberlist) encodePushPullMsg(localState []stateToMerge) ([]byte, error) {
	return nil, nil
}

// don't care about join or not. it will be assigned as an atomic field joining
func (m *Memberlist) readRemoteState(r io.Reader, dec *codec.Decoder) ([]stateToMerge, error) {
	return nil, nil
}

// may notify merge?
func (m *Memberlist) mergeState(remote []stateToMerge) {}
