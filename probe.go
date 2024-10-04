package memberlist

func (m *Memberlist) probe() {}

func (m *Memberlist) nextProbeNode() (*nodeState, error) {
	return nil, nil
}

func (m *Memberlist) probeNode(node *nodeState) {}
