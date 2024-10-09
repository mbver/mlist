package memberlist

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net"

	"github.com/hashicorp/go-msgpack/v2/codec"
)

func (m *Memberlist) EncryptionEnabled() bool {
	return m.keyring != nil && len(m.keyring.GetKeys()) > 0
}

func (m *Memberlist) packMsgUdp(msg []byte, packetLabel string) ([]byte, error) {
	// Check if compression is enabled
	if m.config.EnableCompression {
		compressed, err := compress(msg)
		if err != nil {
			return msg, err
		} else {
			// only compress if it reduce size
			if len(compressed) < len(msg) {
				msg = compressed
			}
		}
	}
	// add crc if it is a packet msg
	crc := crc32.ChecksumIEEE(msg)
	header := make([]byte, 5, 5+len(msg))
	header[0] = byte(hasCrcMsg)
	binary.BigEndian.PutUint32(header[1:], crc)
	msg = append(header, msg...)

	// Check if encryption is enabled
	if m.EncryptionEnabled() {
		key := m.keyring.GetPrimaryKey()
		encrypted, err := encrypt(encryptVersion, key, msg, []byte(packetLabel))
		if err != nil {
			return msg, err
		}
		msg = encrypted // encryptMsg is not included, checking m.EncryptionEnabled in the receiver will be enough
	}
	return msg, nil
}

func (m *Memberlist) unpackPacket(msg []byte, packetLabel string) ([]byte, error) {
	if m.EncryptionEnabled() {
		aad := []byte(packetLabel)
		// check msg size and decrypt
		plain, err := decryptWithKeys(m.keyring.GetKeys(), msg, aad)
		if err != nil {
			return nil, err
		}
		msg = plain
	}

	if len(msg) < 5 || msgType(msg[0]) != hasCrcMsg {
		return nil, fmt.Errorf("expect crc in udp message")
	}

	crc := crc32.ChecksumIEEE(msg[5:])
	expected := binary.BigEndian.Uint32(msg[1:5])
	if crc != expected {
		// m.logger.Printf("[WARN] memberlist: Got invalid checksum for UDP packet: %x, %x", crc, expected)
		return nil, fmt.Errorf("crc for packet invalid, expect: %x, got %x", expected, crc)
	}
	msg = msg[5:]

	if msgType(msg[0]) == compressMsg {
		var err error
		msg, err = decompressMsg(msg)
		if err != nil {
			return nil, err
		}
	}
	return msg, nil
}

func (m *Memberlist) packMsgTcp(msg []byte, streamLabel string) ([]byte, error) {
	// Check if compression is enabled
	if m.config.EnableCompression {
		compressed, err := compress(msg)
		if err != nil {
			return msg, err
		} else {
			// only compress if it reduce size
			if len(compressed) < len(msg) {
				msg = compressed
			}
		}
	}

	// Check if encryption is enabled
	if m.EncryptionEnabled() {
		key := m.keyring.GetPrimaryKey()
		encrypted, err := encryptTcp(encryptVersion, key, msg, streamLabel)
		if err != nil {
			return msg, err
		}
		msg = encrypted
	}
	return msg, nil
}

func (m *Memberlist) unpackStream(conn net.Conn, streamLabel string) (msgType, io.Reader, *codec.Decoder, error) {
	var connReader io.Reader = bufio.NewReader(conn)

	// Read the message type
	buf := [1]byte{0}
	if _, err := io.ReadFull(conn, buf[:]); err != nil {
		return 0, nil, nil, err
	}

	mtype := msgType(buf[0])
	encrypted := mtype == encryptMsg
	encryptionEnabled := m.EncryptionEnabled()

	if encryptionEnabled && !encrypted {
		return 0, nil, nil,
			fmt.Errorf("encryption is enabled but stream is NOT encrypted")
	} else if !encryptionEnabled && encrypted {
		return 0, nil, nil,
			fmt.Errorf("stream is encrypted but encryption is NOT enabled")
	} else if encryptionEnabled && encrypted {
		plain, err := decryptTcp(m.keyring.GetKeys(), connReader, streamLabel)
		if err != nil {
			return 0, nil, nil, err
		}
		// Reset message type and connReader
		mtype = msgType(plain[0])
		connReader = bytes.NewReader(plain[1:])
	}

	// Get the msgPack decoder
	hd := codec.MsgpackHandle{}
	dec := codec.NewDecoder(connReader, &hd)

	// if stream is compressed, unpack further
	if mtype == compressMsg {
		var c compressed
		if err := dec.Decode(&c); err != nil {
			return 0, nil, nil, err
		}
		data, err := c.decompress()
		if err != nil {
			return 0, nil, nil, err
		}

		// Reset the message type, connReader and decoder
		mtype = msgType(data[0])
		connReader = bytes.NewReader(data[1:])
		dec = codec.NewDecoder(connReader, &hd)
	}
	return mtype, connReader, dec, nil
}
