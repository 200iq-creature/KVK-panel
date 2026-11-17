package app

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The embedded protocols (TUIC, AmneziaWG) are terminated by the panel itself.
// Decrypted connections are handed to xray through a local VLESS inbound
// ("kvk-bridge") authenticated with the client's UUID, so xray accounts the
// traffic to the right client and applies filtering, WARP and every other
// routing rule exactly like for its own protocols.

const bridgeTag = "kvk-bridge"

// bridgeDest is a destination for the bridge: an IP or a domain plus port.
type bridgeDest struct {
	IP     netip.Addr
	Domain string
	Port   uint16
}

func (b bridgeDest) String() string {
	h := b.Domain
	if h == "" {
		h = b.IP.Unmap().String()
	}
	return net.JoinHostPort(h, strconv.Itoa(int(b.Port)))
}

func parseUUID(s string) ([16]byte, error) {
	var u [16]byte
	h := strings.ReplaceAll(s, "-", "")
	if len(h) != 32 {
		return u, errors.New("bad uuid")
	}
	b, err := hex.DecodeString(h)
	if err != nil {
		return u, err
	}
	copy(u[:], b)
	return u, nil
}

// vlessHeader builds a VLESS request header (version 0, no addons).
func vlessHeader(uuid [16]byte, udp bool, dst bridgeDest) []byte {
	b := make([]byte, 0, 64)
	b = append(b, 0)
	b = append(b, uuid[:]...)
	b = append(b, 0) // addons length
	if udp {
		b = append(b, 2)
	} else {
		b = append(b, 1)
	}
	b = binary.BigEndian.AppendUint16(b, dst.Port)
	switch {
	case dst.Domain != "":
		d := dst.Domain
		if len(d) > 255 {
			d = d[:255]
		}
		b = append(b, 2, byte(len(d)))
		b = append(b, d...)
	case dst.IP.Unmap().Is4():
		a := dst.IP.Unmap().As4()
		b = append(b, 1)
		b = append(b, a[:]...)
	default:
		a := dst.IP.As16()
		b = append(b, 3)
		b = append(b, a[:]...)
	}
	return b
}

// bridgeConn is a VLESS stream to the local xray inbound. The response header
// is consumed lazily on the first Read.
type bridgeConn struct {
	net.Conn
	gotResp bool
	udp     bool
	rmu     sync.Mutex
	wmu     sync.Mutex
	counter *trafficCounter
}

func (c *bridgeConn) readResp() error {
	if c.gotResp {
		return nil
	}
	var h [2]byte
	if _, err := io.ReadFull(c.Conn, h[:]); err != nil {
		return err
	}
	if h[0] != 0 {
		return fmt.Errorf("vless: unexpected response version %d", h[0])
	}
	if h[1] > 0 {
		if _, err := io.CopyN(io.Discard, c.Conn, int64(h[1])); err != nil {
			return err
		}
	}
	c.gotResp = true
	return nil
}

func (c *bridgeConn) Read(p []byte) (int, error) {
	if err := c.readResp(); err != nil {
		return 0, err
	}
	n, err := c.Conn.Read(p)
	if c.counter != nil && n > 0 {
		c.counter.down.Add(int64(n))
	}
	return n, err
}

func (c *bridgeConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if c.counter != nil && n > 0 {
		c.counter.up.Add(int64(n))
	}
	return n, err
}

// ReadPacket reads one length-prefixed UDP packet (VLESS UDP command).
func (c *bridgeConn) ReadPacket(p []byte) (int, error) {
	c.rmu.Lock()
	defer c.rmu.Unlock()
	if err := c.readResp(); err != nil {
		return 0, err
	}
	var l [2]byte
	if _, err := io.ReadFull(c.Conn, l[:]); err != nil {
		return 0, err
	}
	n := int(binary.BigEndian.Uint16(l[:]))
	if n > len(p) {
		if _, err := io.CopyN(io.Discard, c.Conn, int64(n)); err != nil {
			return 0, err
		}
		return 0, nil
	}
	if _, err := io.ReadFull(c.Conn, p[:n]); err != nil {
		return 0, err
	}
	if c.counter != nil {
		c.counter.down.Add(int64(n))
	}
	return n, nil
}

// WritePacket writes one UDP packet with its 2-byte length prefix.
func (c *bridgeConn) WritePacket(p []byte) error {
	if len(p) > 65535 {
		return errors.New("packet too large")
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	b := make([]byte, 2+len(p))
	binary.BigEndian.PutUint16(b, uint16(len(p)))
	copy(b[2:], p)
	_, err := c.Conn.Write(b)
	if err == nil && c.counter != nil {
		c.counter.up.Add(int64(len(p)))
	}
	return err
}

// trafficCounter accumulates bytes passed through an embedded inbound.
type trafficCounter struct {
	up, down atomic.Int64
}

// dialBridge opens a VLESS connection through xray for the client UUID.
func (a *App) dialBridge(uuid string, udp bool, dst bridgeDest, ctr *trafficCounter) (*bridgeConn, error) {
	port := a.bridgePort()
	if port == 0 {
		return nil, errors.New("bridge is not configured")
	}
	id, err := parseUUID(uuid)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("xray bridge: %w", err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	if _, err := conn.Write(vlessHeader(id, udp, dst)); err != nil {
		conn.Close()
		return nil, err
	}
	return &bridgeConn{Conn: conn, udp: udp, counter: ctr}, nil
}

func (a *App) bridgePort() int {
	a.emb.mu.Lock()
	defer a.emb.mu.Unlock()
	return a.emb.bridgePort
}

// pipe copies data in both directions and closes both ends when done.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		buf := make([]byte, 32*1024)
		_, _ = io.CopyBuffer(dst, src, buf)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = dst.SetReadDeadline(time.Now().Add(5 * time.Second))
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	t := time.NewTimer(10 * time.Second)
	select {
	case <-done:
	case <-t.C:
	}
	t.Stop()
	a.Close()
	b.Close()
}
