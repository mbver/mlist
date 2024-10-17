package memberlist

import (
	"fmt"
	"testing"
	"time"
)

func TestSuspicion_remainingTime(t *testing.T) {
	s := newSuspicion(2*time.Second, 30*time.Second, 4, "node0", func(int) {})
	epsilon := 10 * time.Millisecond
	cases := []struct {
		i        int
		elapsed  time.Duration
		expected time.Duration
	}{
		{0, 0, 30 * time.Second},
		{1, 2 * time.Second, 14 * time.Second},
		{2, 3 * time.Second, 4810 * time.Millisecond},
		{3, 4 * time.Second, -2 * time.Second},
		{4, 5 * time.Second, -3 * time.Second},
		{5, 10 * time.Second, -8 * time.Second},
	}

	for i, c := range cases {
		s.Confirm(fmt.Sprintf("node%d", i))
		s.start = time.Now().Add(-c.elapsed) // controlled elapsed time
		got := s.remainingTime()
		if absDiff(c.expected, got) > epsilon {
			t.Errorf("mismatched remaining time: expect %s, got %s\n", c.expected, got)
		}
	}
}

func absDiff(t1, t2 time.Duration) time.Duration {
	if t1 > t2 {
		return t1 - t2
	}
	return t2 - t1
}

func TestSuspicion_ZeroCap(t *testing.T) {
	ch := make(chan struct{}, 1)
	f := func(int) {
		ch <- struct{}{}
	}
	// zero cap => no confirmation needed. timeout is always minimum
	s := newSuspicion(25*time.Millisecond, 30*time.Second, 0, "me", f)
	if s.Confirm("foo") {
		t.Fatalf("should not provide new information")
	}

	select {
	case <-ch:
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("should have fired")
	}
}

func TestSuspicion_Confirm(t *testing.T) {
	cap := 4
	me := "me"
	minTimeout := 500 * time.Millisecond
	maxTimeout := 2 * time.Second

	type confirm struct {
		from string
		new  bool
	}
	cases := []struct {
		confirms  []confirm
		expected  time.Duration
		nconfirms int
	}{
		{
			[]confirm{},
			maxTimeout,
			1,
		},
		{
			[]confirm{
				{"me", false},
				{"foo", true},
			},
			1250 * time.Millisecond,
			2,
		},
		{
			[]confirm{
				{"me", false},
				{"foo", true},
				{"foo", false},
				{"foo", false},
			},
			1250 * time.Millisecond,
			2,
		},
		{
			[]confirm{
				{"me", false},
				{"foo", true},
				{"bar", true},
			},
			810 * time.Millisecond,
			3,
		},
		{
			[]confirm{
				{"me", false},
				{"foo", true},
				{"bar", true},
				{"baz", true},
			},
			minTimeout,
			4,
		},
		{
			[]confirm{
				{"me", false},
				{"foo", true},
				{"bar", true},
				{"baz", true},
				{"zoo", false},
			},
			minTimeout,
			4,
		},
	}
	for i, c := range cases {
		ch := make(chan struct{}, 1)
		f := func(confirms int) {
			if confirms != c.nconfirms {
				t.Errorf("case %d: bad %d != %d", i, confirms, c.nconfirms)
			}

			ch <- struct{}{} // for logging
		}

		s := newSuspicion(minTimeout, maxTimeout, cap, me, f)
		fudge := 25 * time.Millisecond
		for _, p := range c.confirms {
			time.Sleep(fudge)
			if s.Confirm(p.from) != p.new {
				t.Fatalf("case %d: newInfo mismatch for %s", i, p.from)
			}
		}

		// Wait until right before the timeout and make sure the
		// timer hasn't fired.
		already := time.Duration(len(c.confirms)) * fudge
		time.Sleep(c.expected - already - fudge)
		select {
		case <-ch:
			t.Fatalf("case %d: should not have fired", i)
		default:
		}

		// Wait through the timeout and a little after and make sure it
		// fires.
		time.Sleep(2 * fudge)
		select {
		case <-ch:
		default:
			t.Fatalf("case %d: should have fired", i)
		}

		// late confirm has no effect
		s.Confirm("late")
		time.Sleep(fudge)
		select {
		case <-ch:
			t.Fatalf("case %d: should not have fired", i)
		default:
		}
	}
}
