package memberlist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemberlist_DeadNode_AlreadyDead(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	a := alive{
		Lives: 1,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)

	d := dead{
		Lives: 1,
		ID:    "test",
	}
	m.deadNode(&d, nil)

	d.Lives = 2
	m.deadNode(&d, nil)

	node := m.GetNodeState("test")
	require.Equal(t, StateDead, node.State)

	msgs := dumpQueue(m.mbroadcasts)
	selected := []*TransmitCapItem{}
	for _, m := range msgs {
		if m.name == "test" {
			selected = append(selected, m)
		}
	}
	require.Equal(t, 1, len(selected))
	msg := selected[0]
	require.Equal(t, deadMsg, msgType(msg.msg[0]))
	err = decode(msg.msg[1:], &d)
	require.Nil(t, err)
	require.Equal(t, 1, int(d.Lives))
}

func TestMemberlist_DeadNode_OldDead(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	a := alive{
		Lives: 10,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)

	d := dead{
		Lives: 1,
		ID:    "test",
	}
	m.deadNode(&d, nil)

	node := m.GetNodeState("test")
	require.Equal(t, StateAlive, node.State)
}

func TestMemberlist_DeadNode_OldAlive(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	a := alive{
		Lives: 10,
		ID:    "test",
		IP:    []byte{127, 0, 0, 1},
		Port:  7946,
	}
	m.aliveNode(&a, nil)

	d := dead{
		Lives: 10,
		ID:    "test",
	}
	m.deadNode(&d, nil)

	m.aliveNode(&a, nil)

	node := m.GetNodeState("test")
	require.Equal(t, StateDead, node.State)
}

func TestMemberlist_DeadNodeRefute(t *testing.T) {
	m, cleanup, err := newTestMemberlistNoSchedule()
	defer cleanup()
	require.Nil(t, err)

	require.Equal(t, 0, m.awr.GetHealth())
	d := dead{
		ID:    m.config.ID,
		Lives: 1,
	}

	m.deadNode(&d, nil)

	node := m.LocalNodeState()
	require.Equal(t, StateAlive, node.State)
	require.Equal(t, 2, int(node.Lives))

	require.Equal(t, 1, m.awr.GetHealth()) // should be punished

	msgs := dumpQueue(m.mbroadcasts)
	require.Equal(t, 1, len(msgs))
	require.Equal(t, aliveMsg, msgType(msgs[0].msg[0]))
	var a alive
	err = decode(msgs[0].msg[1:], &a)
	require.Nil(t, err)
	require.Equal(t, 2, int(a.Lives))
}
