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
	key = append([]byte{}, key...) // copy
	r.keys = append(r.keys, key)
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
			r.primaryKey = append([]byte{}, key...) // copy
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
	keys := make([][]byte, len(r.keys))
	for i, k := range r.keys {
		keys[i] = append([]byte{}, k...) // copy
	}
	return keys
}

func (r *Keyring) GetPrimaryKey() []byte {
	r.lock.Lock()
	defer r.lock.Unlock()
	return append([]byte{}, r.primaryKey...) // copy
}
