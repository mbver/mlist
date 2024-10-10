package memberlist

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-msgpack/v2/codec"
)

func (m *Memberlist) packUdp(msg []byte) ([]byte, error) {
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
		encrypted, err := encrypt(m.config.EncryptionVersion, key, msg, []byte(m.config.Label))
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

func (m *Memberlist) packTcp(msg []byte, streamLabel string) ([]byte, error) {
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
		encrypted, err := encryptTcp(m.config.EncryptionVersion, key, msg, streamLabel)
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

const (
	udpPacketBufSize = 65536
	udpSocketBufSize = 2 * 1024 * 1024
)

const (
	compoundHeaderOverhead = 2 // compoundMsgType + len(msgs). each is uint8
	lenMsgOverhead         = 2 // len(msg), uint16
)

type Packet struct {
	Buf       []byte
	From      net.Addr
	Timestamp time.Time
}

type NetTransport struct {
	bindAddrs    []string
	bindPort     int
	logger       *log.Logger
	packetCh     chan *Packet
	udpConns     []*net.UDPConn
	tcpConnCh    chan net.Conn
	tcpListeners []*net.TCPListener
	wg           sync.WaitGroup // to synchronize closing listeners when shutdown
	shutdown     int32
}

func NewNetTransport(addrs []string, port int, logger *log.Logger) (*NetTransport, error) {
	if len(addrs) == 0 {
		return nil, fmt.Errorf("at least one bind address is required")
	}

	return &NetTransport{
		bindAddrs: addrs,
		bindPort:  port,
		packetCh:  make(chan *Packet),
		tcpConnCh: make(chan net.Conn),
	}, nil
}

func (t *NetTransport) Start() error {
	var ok bool
	// Clean up listeners if there's an error.
	defer func() {
		if !ok {
			t.Shutdown()
		}
	}()

	// create tcp and udp listeners on its addresses and port
	for _, addr := range t.bindAddrs {
		ip := net.ParseIP(addr)

		tcpAddr := &net.TCPAddr{IP: ip, Port: t.bindPort}
		tcpLn, err := net.ListenTCP("tcp", tcpAddr)
		if err != nil {
			return fmt.Errorf("failed to start TCP listener on %q port %d: %v", addr, t.bindPort, err)
		}
		t.tcpListeners = append(t.tcpListeners, tcpLn)

		// if port == 0, use the dynamic port assigned by OS
		if t.bindPort == 0 {
			t.bindPort = tcpLn.Addr().(*net.TCPAddr).Port
		}

		udpAddr := &net.UDPAddr{IP: ip, Port: t.bindPort}
		udpConn, err := net.ListenUDP("udp", udpAddr)
		if err != nil {
			return fmt.Errorf("failed to start UDP listener on %q port %d: %v", addr, t.bindPort, err)
		}
		if err := setUDPSocketBuf(udpConn); err != nil {
			return fmt.Errorf("failed to resize UDP buffer: %v", err)
		}
		t.udpConns = append(t.udpConns, udpConn)
	}

	// start listening on incoming requests
	for i := 0; i < len(t.bindAddrs); i++ {
		t.wg.Add(2)
		go t.tcpListen(t.tcpListeners[i])
		go t.udpListen(t.udpConns[i])
	}
	ok = true
	return nil
}

// try to set udp socket buffer size to its largest value
func setUDPSocketBuf(c *net.UDPConn) error {
	size := udpSocketBufSize
	var err error
	for size > 0 {
		if err = c.SetReadBuffer(size); err == nil {
			return nil
		}
		size = size / 2
	}
	return err
}

func (t *NetTransport) tcpListen(l *net.TCPListener) {
	defer t.wg.Done() // notify the waiting Shutdown

	// for exponential backoff
	const baseDelay = 5 * time.Millisecond
	const maxDelay = 1 * time.Second
	var loopDelay time.Duration

	for {
		conn, err := l.AcceptTCP()
		if err != nil {
			if s := atomic.LoadInt32(&t.shutdown); s == 1 {
				break
			}
			// exponential backoff
			if loopDelay == 0 {
				loopDelay = baseDelay
			} else {
				loopDelay *= 2
			}

			if loopDelay > maxDelay {
				loopDelay = maxDelay
			}
			t.logger.Printf("[ERR] memberlist: Error accepting TCP connection: %v", err)
			time.Sleep(loopDelay)
			continue
		}
		// No error, reset loop delay
		loopDelay = 0

		t.tcpConnCh <- conn
	}
}

func (t *NetTransport) TcpConnCh() <-chan net.Conn {
	return t.tcpConnCh
}

func (t *NetTransport) udpListen(c *net.UDPConn) {
	defer t.wg.Done()
	for {
		buf := make([]byte, udpPacketBufSize)
		n, addr, err := c.ReadFrom(buf)
		ts := time.Now() // timestamp just after IO
		if err != nil {
			if s := atomic.LoadInt32(&t.shutdown); s == 1 {
				break
			}

			t.logger.Printf("[ERR] memberlist: Error reading UDP packet: %v", err)
			continue
		}

		// msg must have at least 1 byte
		if n < 1 {
			// t.logger.Printf("[ERR] memberlist: UDP packet too short (%d bytes) %s", len(buf), LogAddress(addr))
			continue
		}

		// Ingest the packet.
		t.packetCh <- &Packet{
			Buf:       buf[:n],
			From:      addr,
			Timestamp: ts,
		}
	}
}

func (t *NetTransport) PacketCh() <-chan *Packet {
	return t.packetCh
}

func (t *NetTransport) SendUdp(msg []byte, addr string) (time.Time, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return time.Time{}, err
	}
	// use the first udp conn
	_, err = t.udpConns[0].WriteTo(msg, udpAddr)
	return time.Now(), err
}

func (m *Memberlist) sendUdp(addr string, msg []byte) error {
	msg, err := m.packUdp(msg)
	if err != nil {
		return err
	}

	_, err = m.transport.SendUdp(msg, addr)
	return err
}

func (t *NetTransport) Shutdown() {
	if atomic.LoadInt32(&t.shutdown) == 1 {
		return
	}

	atomic.StoreInt32(&t.shutdown, 1)

	// Rip through all the connections and shut them down.
	for _, ln := range t.tcpListeners {
		ln.Close()
	}
	t.tcpListeners = nil
	for _, conn := range t.udpConns {
		conn.Close()
	}
	t.udpConns = nil
	// Block until all the listener threads have died.
	t.wg.Wait()
}

func (m *Memberlist) sendTcp(conn net.Conn, msg []byte, streamLabel string) error {
	msg, err := m.packTcp(msg, streamLabel)
	if err != nil {
		return err
	}

	if n, err := conn.Write(msg); err != nil {
		return err
	} else if n != len(msg) { // missing bytes
		return fmt.Errorf("only %d of %d bytes written", n, len(msg))
	}

	return nil
}

// get some msgs in broadcast queue and send it together with msg
func (m *Memberlist) sendMsgPiggyback(addr string, msg []byte) error {
	bytesRemaining := m.config.UDPBufferSize - len(msg) - compoundHeaderOverhead
	if m.EncryptionEnabled() {
		bytesRemaining -= encryptOverhead(m.config.EncryptionVersion)
	}

	piggy := m.getBroadcasts(lenMsgOverhead, bytesRemaining)

	// no piggyback msgs
	if len(piggy) == 0 {
		return m.sendUdp(addr, msg)
	}

	// got piggyback msgs
	msgs := make([][]byte, 0, 1+len(piggy))
	msgs = append(msgs, msg)
	msgs = append(msgs, piggy...)
	compound := makeCompoundMsg(msgs)
	return m.sendUdp(addr, compound)
}
