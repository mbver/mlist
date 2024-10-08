package memberlist

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncrypt_EncryptDecrypt(t *testing.T) {
	k := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	label := []byte("label")
	tests := []struct {
		name string
		msg  []byte
	}{
		{
			"typical msg",
			[]byte("this is a typical message"),
		},
		{
			"empty msg",
			[]byte{},
		},
		{
			"nil msg",
			nil,
		},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			encrypted, err := encrypt(0, k, c.msg, label)
			require.Nil(t, err, "no encryption error")
			decrypted, err := decrypt(k, encrypted, label)
			require.Nil(t, err, "no decryption error")
			if !bytes.Equal(decrypted, c.msg) {
				t.Errorf("decrypted msg failed. expect: %s, got: %s", c.msg, decrypted)
			}
		})
	}
}

func TestEncrypt_EncryptDecryptTcp(t *testing.T) {
	keys := [][]byte{{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}}
	label := "label"
	tests := []struct {
		name string
		msg  []byte
	}{
		{
			"typical msg",
			[]byte("this is a typical message"),
		},
		{
			"empty msg",
			[]byte{},
		},
		{
			"nil msg",
			nil,
		},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			encrypted, err := encryptTcp(0, keys[0], c.msg, label)
			require.Nil(t, err, "no encryption error")
			r := bytes.NewReader(encrypted[1:]) // 1st byte will be consumed in read stream
			decrypted, err := decryptTcp(keys, r, label)
			require.Nil(t, err, "no decryption error")
			if !bytes.Equal(decrypted, c.msg) {
				t.Errorf("decryption failed. expect: %s, got: %s", c.msg, decrypted)
			}
		})
	}
}
