package memberlist

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func nodeWithEventChNoSchedule(ch chan *NodeEvent) (*Memberlist, func(), error) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	if err != nil {
		return nil, cleanup, err
	}
	m.eventMng.ch = ch
	return m, cleanup, err
}

func checkMsgInQueue(q *TransmitCapQueue, name string, t msgType, lives int) error {
	msgs := dumpQueue(q)
	selected := []*TransmitCapItem{}
	for _, m := range msgs {
		if m.name == name {
			selected = append(selected, m)
		}
	}
	if len(selected) != 1 {
		return fmt.Errorf("wrong num of msgs %d", len(selected))
	}
	msg := selected[0]
	gotType := msgType(msg.msg[0])
	if t != gotType {
		return fmt.Errorf("wrong msg type %s", gotType)
	}
	switch t {
	case aliveMsg:
		var a alive
		err := decode(msg.msg[1:], &a)
		if err != nil {
			return err
		}
		if int(a.Lives) != lives {
			return fmt.Errorf("wrong lives %d", a.Lives)
		}
	case suspectMsg:
		var s suspect
		err := decode(msg.msg[1:], &s)
		if err != nil {
			return err
		}
		if int(s.Lives) != lives {
			return fmt.Errorf("wrong lives %d", s.Lives)
		}
	case deadMsg:
		var d dead
		err := decode(msg.msg[1:], &d)
		if err != nil {
			return err
		}
		if int(d.Lives) != lives {
			return fmt.Errorf("wrong lives %d", d.Lives)
		}
	}
	return nil
}

func checkEventInCh(ch chan *NodeEvent, t NodeEventType, id string) error {
	var e *NodeEvent
	select {
	case e = <-ch:
	default:
		return fmt.Errorf("not having event")
	}
	if e.Type != t {
		return fmt.Errorf("wrong event type %s", e.Type)
	}
	if e.Node.ID != id {
		return fmt.Errorf("wrong node %s", e.Node.ID)
	}
	return nil
}

func TestMemberlist_DeadNode(t *testing.T) {
	eventCh := make(chan *NodeEvent, 1)
	m, cleanup, err := nodeWithEventChNoSchedule(eventCh)
	defer cleanup()
	require.Nil(t, err)
	a := alive{
		Lives: 1,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)
	require.Nil(t, checkEventInCh(eventCh, NodeJoin, "test"))

	m.nodeL.Lock()
	m.nodeMap["test"].StateChange = time.Now().Add(-time.Hour)
	m.nodeL.Unlock()

	d := dead{
		Lives: 1,
		ID:    "test",
	}
	m.deadNode(&d, nil)

	node := m.GetNodeState("test")
	require.Equal(t, StateDead, node.State)
	require.True(t, time.Since(node.StateChange) < 1*time.Second)

	require.Nil(t, checkEventInCh(eventCh, NodeLeave, "test"))

	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", deadMsg, 1))
}

func TestMemberlist_DeadNode_AlreadyDead(t *testing.T) {
	eventCh := make(chan *NodeEvent, 1)
	m, cleanup, err := nodeWithEventChNoSchedule(eventCh)
	defer cleanup()
	require.Nil(t, err)

	a := alive{
		Lives: 1,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)

	require.Nil(t, checkEventInCh(eventCh, NodeJoin, "test"))

	d := dead{
		Lives: 1,
		ID:    "test",
	}
	m.deadNode(&d, nil)

	require.Nil(t, checkEventInCh(eventCh, NodeLeave, "test"))

	d.Lives = 2
	m.deadNode(&d, nil)
	require.NotNil(t, checkEventInCh(eventCh, NodeLeave, "test"))

	node := m.GetNodeState("test")
	require.Equal(t, StateDead, node.State)

	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", deadMsg, 1))
}

func TestMemberlist_DeadNode_OldDead(t *testing.T) {
	eventCh := make(chan *NodeEvent, 1)
	m, cleanup, err := nodeWithEventChNoSchedule(eventCh)
	defer cleanup()
	require.Nil(t, err)

	a := alive{
		Lives: 10,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)
	require.Nil(t, checkEventInCh(eventCh, NodeJoin, "test"))

	d := dead{
		Lives: 1,
		ID:    "test",
	}
	m.deadNode(&d, nil)
	require.NotNil(t, checkEventInCh(eventCh, NodeLeave, "test"))

	node := m.GetNodeState("test")
	require.Equal(t, StateAlive, node.State)

	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", aliveMsg, 10))
}

func TestMemberlist_DeadNode_OldAlive(t *testing.T) {
	eventCh := make(chan *NodeEvent, 1)
	m, cleanup, err := nodeWithEventChNoSchedule(eventCh)
	defer cleanup()
	require.Nil(t, err)

	a := alive{
		Lives: 10,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)
	require.Nil(t, checkEventInCh(eventCh, NodeJoin, "test"))

	d := dead{
		Lives: 10,
		ID:    "test",
	}
	m.deadNode(&d, nil)
	require.Nil(t, checkEventInCh(eventCh, NodeLeave, "test"))

	m.aliveNode(&a, nil)
	node := m.GetNodeState("test")
	require.Equal(t, StateDead, node.State)

	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", deadMsg, 10))
}

func TestMemberlist_DeadNodeRefute(t *testing.T) {
	eventCh := make(chan *NodeEvent, 1)
	m, cleanup, err := nodeWithEventChNoSchedule(eventCh)
	defer cleanup()
	require.Nil(t, err)

	require.Equal(t, 0, m.awr.GetHealth())
	d := dead{
		ID:    m.config.ID,
		Lives: 1,
	}

	m.deadNode(&d, nil)
	require.NotNil(t, checkEventInCh(eventCh, NodeLeave, d.ID)) // no event

	node := m.LocalNodeState()
	require.Equal(t, StateAlive, node.State)
	require.Equal(t, 2, int(node.Lives))

	require.Equal(t, 1, m.awr.GetHealth()) // should be punished

	require.Nil(t, checkMsgInQueue(m.mbroadcasts, m.config.ID, aliveMsg, 2))
}
