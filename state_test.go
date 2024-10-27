package memberlist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
