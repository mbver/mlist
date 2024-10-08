package memberlist

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

type encryptionVersion uint8

const (
	maxEncryptionVersion = 0
	versionSize          = 1
	nonceSize            = 12
	tagSize              = 16
	maxPushSize          = 20 * 1024 * 1024
)

func encryptOverhead(vsn encryptionVersion) int {
	switch vsn {
	case 0:
		return versionSize + nonceSize + tagSize // 29
	default:
		panic("unsupported version")
	}
}

// vsn-nonce-ciphertext-tag
func encrypt(vsn encryptionVersion, key []byte, msg []byte, aad []byte) ([]byte, error) {
	// Get the AES block cipher
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Get the GCM cipher mode
	gcm, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	// Grow the buffer to make room for everything
	buf.Grow(encryptOverhead(vsn) + len(msg))

	buf.WriteByte(byte(vsn))

	// Add a random nonce. must use crypto/rand
	_, err = io.CopyN(buf, rand.Reader, nonceSize)
	if err != nil {
		return nil, err
	}

	// retrieve nonce
	nonce := buf.Bytes()[versionSize : versionSize+nonceSize]

	// ciphertext + tag
	authCT := gcm.Seal(nil, nonce, msg, aad)

	// write auth cipher text
	buf.Write(authCT)
	return buf.Bytes(), nil
}

func decrypt(key, msg []byte, aad []byte) ([]byte, error) {
	// Get the AES block cipher
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Get the GCM cipher mode
	gcm, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, err
	}

	// Decrypt the message
	nonce := msg[versionSize : versionSize+nonceSize]
	authCT := msg[versionSize+nonceSize:]

	plain, err := gcm.Open(nil, nonce, authCT, aad)
	if err != nil {
		return nil, err
	}

	// Success!
	return plain, nil
}

func decryptWithKeys(keys [][]byte, msg []byte, data []byte) ([]byte, error) {
	// Ensure we have at least one byte
	if len(msg) == 0 {
		return nil, fmt.Errorf("cannot decrypt empty payload")
	}

	// Verify the version
	vsn := encryptionVersion(msg[0])
	if vsn > maxEncryptionVersion {
		return nil, fmt.Errorf("unsupported encryption version %d", msg[0])
	}

	// Ensure the length is sane
	if len(msg) < encryptOverhead(vsn) {
		return nil, fmt.Errorf("payload is too small to decrypt: %d", len(msg))
	}

	for _, key := range keys {
		plain, err := decrypt(key, msg, data)
		if err == nil {
			return plain, nil
		}
	}

	return nil, fmt.Errorf("no installed keys could decrypt the message")
}

// encryptMsg-size-(version-nonce-encrypted-tag)
// size = version + nonce + tag + len(msg)
func encryptTcp(vsn encryptionVersion, key []byte, msg []byte, streamLabel string) ([]byte, error) {
	var buf bytes.Buffer

	// Write the encryptMsg byte
	buf.WriteByte(byte(encryptMsg))
	// write the size of encrypted msg
	writeEncryptedSize(vsn, len(msg), &buf)

	// AAD = encryptMsg + msgLength + streamLabel
	authData := tcpAAD(&buf, streamLabel)

	encrypted, err := encrypt(vsn, key, msg, authData)
	if err != nil {
		return nil, err
	}
	buf.Write(encrypted)
	return buf.Bytes(), nil
}

func writeEncryptedSize(vsn encryptionVersion, size int, buf *bytes.Buffer) {
	sizeBuf := make([]byte, 4)
	size = encryptOverhead(vsn) + size
	binary.BigEndian.PutUint32(sizeBuf, uint32(size))
	buf.Write(sizeBuf)
}

func tcpAAD(buf *bytes.Buffer, streamLabel string) []byte {
	authData := buf.Bytes()[:5]
	if streamLabel != "" {
		// don't change the underlying buf array
		authData = make([]byte, 5+len(streamLabel))
		authData = append(authData, buf.Bytes()[:5]...)
		authData = append(authData, []byte(streamLabel)...)
	}
	return authData
}

func decryptTcp(keys [][]byte, r io.Reader, streamLabel string) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(encryptMsg)) // to reconstruct authData
	_, err := io.CopyN(buf, r, 4)   // size
	if err != nil {
		return nil, err
	}

	authData := tcpAAD(buf, streamLabel)
	size := binary.BigEndian.Uint32(buf.Bytes()[1:5]) //
	if size > maxPushSize {
		return nil, fmt.Errorf("remote node state is larger than limit (%d)", size)
	}

	//Start reporting the size before you cross the limit
	if size > uint32(math.Floor(.6*maxPushSize)) {
		// m.logger.Printf("[WARN] memberlist: Remote node state size is (%d) limit is (%d)", size, maxPushStateBytes)
	}

	// Read in the rest of the payload
	_, err = io.CopyN(buf, r, int64(size))
	if err != nil {
		return nil, err
	}

	encrypted := buf.Bytes()[5:]

	// Decrypt the payload
	return decryptWithKeys(keys, encrypted, authData)
}
