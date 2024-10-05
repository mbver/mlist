package memberlist

import "time"

type suspicion struct{}

func newSuspicion(
	minTimeout time.Duration,
	maxTimeout time.Duration,
	confirmCap int, from string,
	timeoutFn func(uint32),
) *suspicion {
	return nil
}

func (s *suspicion) remainingTime() time.Duration {
	return 0
}

func (s *suspicion) Confirm(from string) bool {
	return false
}
