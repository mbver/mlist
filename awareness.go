// Copyright (c) HashiCorp, Inc.
// Copyright (c) 2024 Phuoc Phi
// SPDX-License-Identifier: MPL-2.0
package memberlist

import (
	"sync"
	"time"
)

type awareness struct {
	l      sync.RWMutex
	health int
	max    int
}

func newAwareness(max int) *awareness {
	return &awareness{
		max: max,
	}
}
func (a *awareness) GetHealth() int {
	a.l.RLock()
	defer a.l.RUnlock()
	return a.health
}

func (a *awareness) Punish(delta int) {
	a.l.Lock()
	defer a.l.Unlock()
	a.health += delta
	if a.health < 0 {
		a.health = 0
	} else if a.health > (a.max - 1) {
		a.health = a.max - 1
	}
}

func (a *awareness) ScaleTimeout(timeout time.Duration) time.Duration {
	a.l.RLock()
	h := a.health
	a.l.RUnlock()
	return time.Duration(h+1) * timeout
}
