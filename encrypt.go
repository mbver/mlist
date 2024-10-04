package memberlist

import (
	"bytes"
	"io"
)

func encryptOverhead() int {
	return 0
}

func encryptTcp(key []byte, msg []byte, streamLabel string) ([]byte, error) {
	return nil, nil
}

func writeEncryptSize(size int, buf *bytes.Buffer) {}

func aad(buf *bytes.Buffer, streamLabel string) []byte {
	return nil
}

func encrypt(key []byte, msg []byte, aad []byte, dest *bytes.Buffer) ([]byte, error) {
	return nil, nil
}

func decrypt(key, msg []byte, add []byte) ([]byte, error) {
	return nil, nil
}

func decryptWithKeys(keys [][]byte, msg []byte, data []byte) ([]byte, error) {
	return nil, nil
}

func decryptTcp(keys [][]byte, r io.Reader, streamLabel string) ([]byte, error) {
	return nil, nil
}
