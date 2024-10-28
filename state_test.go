package memberlist

import (
	"bytes"
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
	m.eventMng.logger = m.logger
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

func prepareTestNode() (*Memberlist, chan *NodeEvent, func(), error) {
	eventCh := make(chan *NodeEvent, 2) // update event and join event
	m, cleanup, err := nodeWithEventChNoSchedule(eventCh)
	if err != nil {
		return nil, nil, cleanup, err
	}

	a := alive{
		Lives: 1,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)
	return m, eventCh, cleanup, checkEventInCh(eventCh, NodeJoin, "test")
}

func isRecent(t1 time.Time) bool {
	return t1.After(time.Now().Add(-1 * time.Second))
}

func TestMemberlist_AliveNode_NewNode(t *testing.T) {
	m, _, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)

	node := m.GetNodeState("test")
	require.Equal(t, 1, int(node.Lives))
	require.Equal(t, StateAlive, node.State)
	require.True(t, isRecent(node.StateChange))
}

func TestMemberlist_AliveNode_SuspectNode(t *testing.T) {
	m, eventCh, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)

	// fake suspect, without creating a suspect timer
	m.nodeL.Lock()
	m.nodeMap["test"].State = StateSuspect
	m.nodeMap["test"].StateChange = time.Now().Add(-1 * time.Hour)
	m.nodeL.Unlock()

	a := alive{
		Lives: 1,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)
	node := m.GetNodeState("test")
	require.Equal(t, StateSuspect, node.State)

	a.Lives = 2
	m.aliveNode(&a, nil)
	node = m.GetNodeState("test")
	require.Equal(t, StateAlive, node.State)
	require.True(t, isRecent(node.StateChange))
	require.Zero(t, len(eventCh))
	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", aliveMsg, 2)) // no event
}

func TestMemberlist_AliveNode_Idempotent(t *testing.T) {
	m, _, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)
	node0 := m.GetNodeState("test")
	a := alive{
		Lives: 2,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)
	node1 := m.GetNodeState("test")
	require.Equal(t, StateAlive, node1.State)
	require.Equal(t, node0.StateChange, node1.StateChange)
	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", aliveMsg, 2))
}

func TestMemberlist_AliveNode_Update(t *testing.T) {
	m, eventCh, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)

	a := alive{
		Lives: 2,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
		Tags:  []byte("new tag"),
	}
	m.aliveNode(&a, nil)
	node := m.GetNodeState("test")
	require.Equal(t, 2, int(node.Lives))
	if !bytes.Equal(a.Tags, node.Node.Tags) {
		t.Fatalf("tags not updated")
	}
	require.Nil(t, checkEventInCh(eventCh, NodeUpdate, "test"))
	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", aliveMsg, 2))
}

func TestMemberlist_SuspectNode(t *testing.T) {
	m, _, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)
	m.config.ProbeInterval = 1 * time.Millisecond // to compute suspicion minTimeout

	m.nodeL.Lock()
	m.nodeMap["test"].StateChange = time.Now().Add(-1 * time.Hour)
	m.nodeL.Unlock()

	s := suspect{
		Lives: 1,
		ID:    "test",
		From:  "some node",
	}
	m.suspectNode(&s)
	node := m.GetNodeState("test")
	require.Equal(t, StateSuspect, node.State)
	change := node.StateChange
	require.True(t, isRecent(change))
	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", suspectMsg, 1))

	time.Sleep(10 * time.Millisecond) // wait for suspicion timeout
	node = m.GetNodeState("test")
	require.Equal(t, StateDead, node.State)
	require.True(t, node.StateChange.After(change))
	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", deadMsg, 1))
}

func TestMemberlist_SuspectNode_DoubleSuspect(t *testing.T) {
	m, _, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)
	m.config.ProbeInterval = 2 * time.Second // to compute suspicion minTimeout

	m.nodeL.Lock()
	m.nodeMap["test"].StateChange = time.Now().Add(-1 * time.Hour)
	m.nodeL.Unlock()

	s := suspect{
		Lives: 1,
		ID:    "test",
		From:  "some node",
	}
	m.suspectNode(&s)
	node := m.GetNodeState("test")
	require.Equal(t, StateSuspect, node.State)
	change := node.StateChange
	require.True(t, isRecent(change))

	m.suspectNode(&s)
	node = m.GetNodeState("test")
	require.Equal(t, change, node.StateChange)
}

func TestSuspectNode_NoNode(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	s := suspect{
		Lives: 1,
		ID:    "test",
		From:  "some node",
	}
	m.suspectNode(&s)

	require.Nil(t, m.GetNodeState("test"))
}

func TestMemberlist_SuspectNode_OldSuspect(t *testing.T) {
	m, _, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)

	m.nodeL.Lock()
	m.nodeMap["test"].Lives = 10
	m.nodeL.Unlock()

	s := suspect{
		Lives: 1,
		ID:    "test",
		From:  "some node",
	}
	m.suspectNode(&s)
	node := m.GetNodeState("test")
	require.Equal(t, StateAlive, node.State)
	require.NotNil(t, checkMsgInQueue(m.mbroadcasts, "test", suspectMsg, 1))
}

func TestMemberlist_SuspectNode_Refute(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	require.Zero(t, m.Health())

	s := suspect{
		Lives: 1,
		ID:    m.ID(),
		From:  "some node",
	}
	m.suspectNode(&s)
	node := m.LocalNodeState()
	require.Equal(t, 2, int(node.Lives))
	require.Equal(t, StateAlive, node.State)
	require.Equal(t, 1, m.Health()) // should be punish
	require.Nil(t, checkMsgInQueue(m.mbroadcasts, m.ID(), aliveMsg, 2))
}

func TestMemberlist_DeadNode(t *testing.T) {
	m, eventCh, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)

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

func TestMemberlist_DeadNode_LeftRejoin(t *testing.T) {
	m, eventCh, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)

	d := dead{
		Lives: 1,
		ID:    "test",
		Left:  true,
	}

	m.deadNode(&d, nil)

	node := m.GetNodeState("test")
	require.Equal(t, StateLeft, node.State)

	require.Nil(t, checkEventInCh(eventCh, NodeLeave, "test"))
	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", deadMsg, 1))
	a := alive{
		Lives: 2,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7496,
		Tags:  []byte("new tag"),
	}
	m.aliveNode(&a, nil)
	node = m.GetNodeState("test")
	require.Equal(t, StateAlive, node.State)
	require.Equal(t, 2, int(node.Lives))
	if !bytes.Equal(a.Tags, node.Node.Tags) {
		t.Fatalf("tags is not updated")
	}

	require.Nil(t, checkEventInCh(eventCh, NodeJoin, "test"))
	require.Nil(t, checkEventInCh(eventCh, NodeUpdate, "test"))
	require.Nil(t, checkMsgInQueue(m.mbroadcasts, "test", aliveMsg, 2))
}

func TestMemberlist_DeadNode_NoNode(t *testing.T) {
	eventCh := make(chan *NodeEvent, 2) // event join and update
	m, cleanup, err := nodeWithEventChNoSchedule(eventCh)
	defer cleanup()
	require.Nil(t, err)

	d := dead{
		Lives: 1,
		ID:    "test",
		Left:  true,
	}

	m.deadNode(&d, nil)
	require.Nil(t, m.GetNodeState("test"))
	require.Zero(t, len(eventCh)) // no event
}
func TestMemberlist_DeadNode_AlreadyDead(t *testing.T) {
	m, eventCh, cleanup, err := prepareTestNode()
	defer cleanup()
	require.Nil(t, err)

	d := dead{
		Lives: 1,
		ID:    "test",
	}
	m.deadNode(&d, nil)

	require.Nil(t, checkEventInCh(eventCh, NodeLeave, "test"))

	d.Lives = 2
	m.deadNode(&d, nil)
	require.Zero(t, len(eventCh)) // no event

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
	require.Zero(t, len(eventCh)) // no event

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

	require.Equal(t, 0, m.Health())
	d := dead{
		ID:    m.ID(),
		Lives: 1,
	}

	m.deadNode(&d, nil)
	require.Zero(t, len(eventCh)) // no event

	node := m.LocalNodeState()
	require.Equal(t, StateAlive, node.State)
	require.Equal(t, 2, int(node.Lives))

	require.Equal(t, 1, m.Health()) // should be punished

	require.Nil(t, checkMsgInQueue(m.mbroadcasts, m.ID(), aliveMsg, 2))
}
