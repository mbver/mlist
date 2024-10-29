package memberlist

import (
	"math"
	"sync"
	"time"
)

type suspicion struct {
	l          sync.Mutex
	minTimeout time.Duration
	maxTimeout time.Duration
	confirmCap int
	confirmMap map[string]struct{}
	start      time.Time
	timer      *time.Timer
	timeoutFn  func()
}

func newSuspicion(minTimeout time.Duration, maxTimeout time.Duration, confirmCap int, from string, timeoutFn func(int)) *suspicion {
	s := &suspicion{
		minTimeout: minTimeout,
		maxTimeout: maxTimeout,
		confirmCap: confirmCap,
		confirmMap: map[string]struct{}{},
	}
	s.confirmMap[from] = struct{}{}
	s.timeoutFn = func() {
		s.l.Lock()
		confirms := len(s.confirmMap)
		s.l.Unlock()
		timeoutFn(confirms)
	}
	timeout := maxTimeout
	if confirmCap <= 1 {
		timeout = minTimeout
	}
	s.timer = time.AfterFunc(timeout, s.timeoutFn)
	s.start = time.Now()
	return s
}

// caller must hold lock
func (s *suspicion) remainingTime() time.Duration {
	elapsed := time.Since(s.start)
	confirms := len(s.confirmMap)
	if confirms >= s.confirmCap {
		return s.minTimeout - elapsed
	}
	f := math.Log(float64(confirms)) / math.Log(float64(s.confirmCap))
	maxMinDiff := s.maxTimeout.Seconds() - s.minTimeout.Seconds()
	sec := s.maxTimeout.Seconds() - f*maxMinDiff
	timeout := time.Duration(math.Floor(1000.0*sec)) * time.Millisecond
	return timeout - elapsed
}

// Confirm is called with nodeLock hold.
func (s *suspicion) Confirm(from string) bool {
	s.l.Lock()
	defer s.l.Unlock()
	if len(s.confirmMap) >= s.confirmCap {
		return false
	}
	if _, ok := s.confirmMap[from]; ok {
		return false
	}
	s.confirmMap[from] = struct{}{}
	remaining := s.remainingTime()
	if s.timer.Stop() {
		if remaining > 0 {
			s.timer.Reset(remaining)
		} else {
			go s.timeoutFn()
		}
	}
	return true
}
