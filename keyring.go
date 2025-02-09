// Copyright (c) HashiCorp, Inc.
// Copyright (c) 2024 Phuoc Phi
// SPDX-License-Identifier: MPL-2.0
package memberlist

import (
	"bytes"
	"fmt"
	"sync"
)

type Keyring struct {
	primaryKey []byte
	keys       [][]byte
	lock       sync.Mutex
}

func NewKeyring(keys [][]byte, primaryKey []byte) (*Keyring, error) {
	r := &Keyring{
		keys: [][]byte{},
	}
	if primaryKey != nil {
		if err := r.AddKey(primaryKey); err != nil {
			return nil, err
		}
		if err := r.UseKey(primaryKey); err != nil {
			return nil, err
		}
	}
	for _, k := range keys {
		if err := r.AddKey(k); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// caller must hold lock to keyring
func (r *Keyring) AddKey(key []byte) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	if err := ValidateKey(key); err != nil {
		return err
	}
	for _, k := range r.keys {
		if bytes.Equal(key, k) {
			return nil
		}
	}
	r.keys = append(r.keys, copyBytes(key))
	return nil
}

func ValidateKey(key []byte) error {
	if l := len(key); l != 16 && l != 24 && l != 32 {
		return fmt.Errorf("key size must be 16, 24 or 32 bytes")
	}
	return nil
}

func (r *Keyring) UseKey(key []byte) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	for _, k := range r.keys {
		if bytes.Equal(key, k) {
			r.primaryKey = copyBytes(key)
			return nil
		}
	}
	return fmt.Errorf("key %x is not in keyring", key)
}

func (r *Keyring) RemoveKey(key []byte) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	if bytes.Equal(key, r.primaryKey) {
		return fmt.Errorf("removing primary key is not allowed")
	}
	exist := false
	for i, k := range r.keys {
		if bytes.Equal(key, k) {
			exist = true
			r.keys = append(r.keys[:i], r.keys[i+1:]...)
		}
	}
	if exist {
		return nil
	}
	return fmt.Errorf("key %x not in keyring", key)
}

func (r *Keyring) GetKeys() [][]byte {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.primaryKey == nil {
		return [][]byte{}
	}
	keys := make([][]byte, 0, len(r.keys))
	keys = append(keys, copyBytes(r.primaryKey))
	for _, k := range r.keys {
		if bytes.Equal(k, r.primaryKey) {
			continue
		}
		keys = append(keys, copyBytes(k))
	}
	return keys
}

func (r *Keyring) GetPrimaryKey() []byte {
	r.lock.Lock()
	defer r.lock.Unlock()
	return copyBytes(r.primaryKey)
}
