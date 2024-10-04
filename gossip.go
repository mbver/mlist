package memberlist

func (m *Memberlist) gossip() {}

func (m *Memberlist) pickRandomNodes(num int, accept func(*nodeState) bool) []Node {
	return nil
}
