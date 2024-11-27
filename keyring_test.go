package memberlist

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var TestKeys [][]byte = [][]byte{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
	{8, 9, 10, 11, 12, 13, 14, 15, 0, 1, 2, 3, 4, 5, 6, 7},
}

func TestKeyring_EmptyRing(t *testing.T) {
	// Keyrings can be created with no encryption keys (disabled encryption)
	keyring, err := NewKeyring(nil, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	keys := keyring.GetKeys()
	if len(keys) != 0 {
		t.Fatalf("expect 0 keys but have %d", len(keys))
	}
}

func TestKeyring_PrimaryOnly(t *testing.T) {
	// Keyrings can be created using only a primary key
	keyring, err := NewKeyring(nil, TestKeys[0])
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	keys := keyring.GetKeys()
	if len(keys) != 1 {
		t.Fatalf("expect 1 key but have %d", len(keys))
	}
}

func TestKeyring_GetPrimaryKey(t *testing.T) {
	keyring, err := NewKeyring(TestKeys, TestKeys[1])
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// GetPrimaryKey returns correct key
	primaryKey := keyring.GetPrimaryKey()
	if !bytes.Equal(primaryKey, TestKeys[1]) {
		t.Fatalf("unexpected primary key: %v", primaryKey)
	}
}

func TestKeyring_AddRemoveUse(t *testing.T) {
	keyring, err := NewKeyring(nil, TestKeys[1])
	if err != nil {
		t.Fatalf("err :%s", err)
	}

	// Use non-existent key throws error
	if err := keyring.UseKey(TestKeys[2]); err == nil {
		t.Fatalf("Expected key not installed error")
	}

	// Add key to ring
	if err := keyring.AddKey(TestKeys[2]); err != nil {
		t.Fatalf("err: %s", err)
	}

	keys := keyring.GetKeys()
	if len(keys) != 2 {
		t.Fatalf("Expected 2 keys but have %d", len(keys))
	}

	if !bytes.Equal(keyring.GetPrimaryKey(), TestKeys[1]) {
		t.Fatalf("unexpected primary key change")
	}

	// Use key that exists should succeed
	if err := keyring.UseKey(TestKeys[2]); err != nil {
		t.Fatalf("err: %s", err)
	}

	primaryKey := keyring.GetPrimaryKey()
	if !bytes.Equal(primaryKey, TestKeys[2]) {
		t.Fatalf("unexpected primary key: %v", primaryKey)
	}

	// Removing primary key should fail
	if err := keyring.RemoveKey(TestKeys[2]); err == nil {
		t.Fatalf("expect primary key removal error")
	}

	// Removing non-primary key should succeed
	if err := keyring.RemoveKey(TestKeys[1]); err != nil {
		t.Fatalf("err: %s", err)
	}

	keys = keyring.GetKeys()
	if len(keys) != 1 {
		t.Fatalf("expect 1 key but have %d", len(keys))
	}
}

func TestKeyRing_MultiKeyEncryptDecrypt(t *testing.T) {
	plaintext := []byte("this is a plain text message")
	extra := []byte("random data")
	vsn := encryptionVersion(0)

	keyring, err := NewKeyring(TestKeys, TestKeys[0])
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// First encrypt using the primary key and make sure we can decrypt
	encrypted, err := encrypt(vsn, TestKeys[0], plaintext, extra)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	msg, err := decryptWithKeys(keyring.GetKeys(), encrypted, extra)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if !bytes.Equal(msg, plaintext) {
		t.Fatalf("bad: %v", msg)
	}

	// Now encrypt with a secondary key and try decrypting again.

	encrypted, err = encrypt(vsn, TestKeys[2], plaintext, extra)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	msg, err = decryptWithKeys(keyring.GetKeys(), encrypted, extra)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if !bytes.Equal(msg, plaintext) {
		t.Fatalf("bad: %v", msg)
	}

	// Remove a key from the ring, and then try decrypting again
	if err := keyring.RemoveKey(TestKeys[2]); err != nil {
		t.Fatalf("err: %s", err)
	}

	_, err = decryptWithKeys(keyring.GetKeys(), encrypted, extra)
	if err == nil {
		t.Fatalf("Expected no keys to decrypt message")
	}
}

func TestKeyRing_AlwaysDumpPrimaryKeyFirst(t *testing.T) {
	kr, err := NewKeyring(TestKeys, TestKeys[0])
	require.Nil(t, err)
	keys := kr.GetKeys()
	require.True(t, bytes.Equal(keys[0], TestKeys[0]))
	require.Equal(t, 3, len(keys))
	kr.UseKey(TestKeys[2])
	keys = kr.GetKeys()
	require.True(t, bytes.Equal(keys[0], TestKeys[2]))
	require.Equal(t, 3, len(keys))
}
