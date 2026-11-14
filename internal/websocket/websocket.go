// Package websocket is the RFC 6455 client the browser stage's watchdog speaks
// WebDriver BiDi over (R9.3, fable's ruling at coop r9/164).
//
// WHY IN-TREE AND NOT A THIRD DIRECT DEPENDENCY. design.md holds that a direct
// dependency is a standing cost to everyone who audits this tree, and each of
// the two this module has earned its page. BiDi needs a small, fixed subset of
// the protocol, so the subset is written here, client side only:
//
//   - text frames, sent and received, with continuation fragments reassembled;
//   - ping answered with pong, pong ignored, close echoed;
//   - NO extensions and NO compression: none is offered in the handshake, and a
//     server that answers with one, or sets an RSV bit, is refused by name;
//   - `ws://` only. chromedriver and geckodriver hand out a loopback `ws://`
//     URL, and a `wss://` one is refused rather than dialled in the clear.
//
// What this package does NOT know is BiDi. It moves messages; the meaning of a
// message is the caller's.
package websocket

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// The opcodes this client handles (RFC 6455 section 5.2).
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// MaxMessage bounds one reassembled message. A BiDi event is a few kilobytes; a
// length field can claim 2^63 bytes, and a client that believed it would
// allocate whatever a broken or hostile peer asked for.
const MaxMessage = 16 << 20

// acceptGUID is the constant RFC 6455 section 1.3 appends to the key.
const acceptGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// ErrClosed is returned once the peer has sent a close frame. Code is the
// status it sent, 1005 when it sent none (section 7.1.5).
type ErrClosed struct{ Code int }

func (e *ErrClosed) Error() string {
	return fmt.Sprintf("websocket: the server closed the connection, status %d", e.Code)
}

// Conn is one client connection. ONE goroutine may read while others write:
// the BiDi watch reads events on its own goroutine and sends commands from the
// caller's, so writes are serialised and reads and writes keep separate
// deadlines -- a write's bound must not cut off a read that is waiting.
type Conn struct {
	nc net.Conn
	br *bufio.Reader

	wmu       sync.Mutex
	closeSent bool
}

// Accept is the Sec-WebSocket-Accept a server must answer for key.
func Accept(key string) string {
	sum := sha1.Sum([]byte(key + acceptGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Dial opens a connection to a `ws://` URL and completes the opening handshake.
func Dial(ctx context.Context, raw string) (*Conn, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("websocket: %q is not a URL: %w", raw, err)
	}
	if u.Scheme != "ws" {
		return nil, fmt.Errorf("websocket: scheme %q is refused: this client speaks ws:// only", u.Scheme)
	}
	var d net.Dialer
	nc, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, fmt.Errorf("websocket: dial %s: %w", u.Host, err)
	}
	c := &Conn{nc: nc, br: bufio.NewReader(nc)}
	if err := c.handshake(ctx, u); err != nil {
		_ = nc.Close()
		return nil, err
	}
	return c, nil
}

func (c *Conn) handshake(ctx context.Context, u *url.URL) error {
	defer c.bound(ctx, c.nc.SetDeadline)()
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("websocket: no randomness for the key: %w", err)
	}
	key := base64.StdEncoding.EncodeToString(nonce)
	req := "GET " + u.RequestURI() + " HTTP/1.1\r\nHost: " + u.Host +
		"\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key +
		"\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(c.nc, req); err != nil {
		return fmt.Errorf("websocket: send the handshake: %w", err)
	}
	resp, err := http.ReadResponse(c.br, &http.Request{Method: http.MethodGet})
	if err != nil {
		return fmt.Errorf("websocket: read the handshake answer: %w", err)
	}
	_ = resp.Body.Close()
	switch {
	case resp.StatusCode != http.StatusSwitchingProtocols:
		return fmt.Errorf("websocket: the server answered %s, not 101 Switching Protocols", resp.Status)
	case !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket"):
		return fmt.Errorf("websocket: the answer's Upgrade is %q, not websocket", resp.Header.Get("Upgrade"))
	case !headerHasToken(resp.Header, "Connection", "upgrade"):
		return fmt.Errorf("websocket: the answer's Connection %q carries no upgrade", resp.Header.Get("Connection"))
	case resp.Header.Get("Sec-WebSocket-Accept") != Accept(key):
		return fmt.Errorf("websocket: Sec-WebSocket-Accept %q does not answer the key sent", resp.Header.Get("Sec-WebSocket-Accept"))
	case resp.Header.Get("Sec-WebSocket-Extensions") != "":
		return fmt.Errorf("websocket: the server chose extension %q, and this client offered none", resp.Header.Get("Sec-WebSocket-Extensions"))
	}
	return nil
}

func headerHasToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, t := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), token) {
				return true
			}
		}
	}
	return false
}

// bound makes one direction of the socket obey ctx for one operation: set is
// SetReadDeadline, SetWriteDeadline or both. Its deadline becomes the socket's,
// a cancellation unblocks the operation at once, and the returned func undoes
// both.
func (c *Conn) bound(ctx context.Context, set func(time.Time) error) func() {
	dl, _ := ctx.Deadline()
	_ = set(dl)
	stop := context.AfterFunc(ctx, func() { _ = set(time.Unix(1, 0)) })
	return func() { stop(); _ = set(time.Time{}) }
}

// WriteText sends msg as one masked text frame.
func (c *Conn) WriteText(ctx context.Context, msg []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	defer c.bound(ctx, c.nc.SetWriteDeadline)()
	return c.writeFrame(opText, msg)
}

// writeFrame sends one final, MASKED frame: section 5.3 requires every client
// frame to be masked, with a fresh key per frame, and a server must close the
// connection on an unmasked one. The caller holds wmu. After our close frame
// nothing more may be sent (section 5.5.1), so a late frame is refused.
func (c *Conn) writeFrame(op byte, payload []byte) error {
	if c.closeSent {
		return errors.New("websocket: the connection is closing, and nothing may follow a close frame")
	}
	if op == opClose {
		c.closeSent = true
	}
	hdr := []byte{0x80 | op}
	switch n := len(payload); {
	case n < 126:
		hdr = append(hdr, 0x80|byte(n))
	case n <= 0xFFFF:
		hdr = append(hdr, 0x80|126)
		hdr = binary.BigEndian.AppendUint16(hdr, uint16(n))
	default:
		hdr = append(hdr, 0x80|127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	var key [4]byte
	if _, err := rand.Read(key[:]); err != nil {
		return fmt.Errorf("websocket: no randomness for the mask: %w", err)
	}
	frame := append(append(hdr, key[:]...), payload...)
	mask(frame[len(hdr)+4:], key)
	if _, err := c.nc.Write(frame); err != nil {
		return fmt.Errorf("websocket: write: %w", err)
	}
	return nil
}

func mask(b []byte, key [4]byte) {
	for i := range b {
		b[i] ^= key[i%4]
	}
}

// ReadText returns the next whole text message, reassembling its fragments and
// answering any control frame that arrives between them.
func (c *Conn) ReadText(ctx context.Context) ([]byte, error) {
	defer c.bound(ctx, c.nc.SetReadDeadline)()
	var msg []byte
	started := false
	for {
		fin, op, payload, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		switch op {
		case opPing:
			if err := c.send(opPong, payload); err != nil {
				return nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			code := 1005
			if len(payload) >= 2 {
				code = int(binary.BigEndian.Uint16(payload))
			}
			// Echoed unless our own close went first, in which case this IS
			// the answer to it. Either way the connection is over.
			_ = c.send(opClose, payload[:min(len(payload), 2)])
			_ = c.nc.Close()
			return nil, &ErrClosed{Code: code}
		case opBinary:
			return nil, errors.New("websocket: the server sent a binary frame, and BiDi is text only")
		case opText:
			if started {
				return nil, errors.New("websocket: a new text frame arrived inside an unfinished message")
			}
			started = true
		case opContinuation:
			if !started {
				return nil, errors.New("websocket: a continuation frame arrived with no message to continue")
			}
		default:
			return nil, fmt.Errorf("websocket: opcode %#x is reserved", op)
		}
		if len(msg)+len(payload) > MaxMessage {
			return nil, fmt.Errorf("websocket: a message exceeds %d bytes", MaxMessage)
		}
		msg = append(msg, payload...)
		if fin {
			return msg, nil
		}
	}
}

// readFrame reads one frame and refuses what section 5 forbids a server to send.
func (c *Conn) readFrame() (fin bool, op byte, payload []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(c.br, h[:]); err != nil {
		return false, 0, nil, fmt.Errorf("websocket: read: %w", err)
	}
	fin, op = h[0]&0x80 != 0, h[0]&0x0F
	if h[0]&0x70 != 0 {
		return false, 0, nil, errors.New("websocket: an RSV bit is set, and no extension was negotiated")
	}
	if h[1]&0x80 != 0 {
		return false, 0, nil, errors.New("websocket: the server masked a frame, which section 5.1 forbids")
	}
	n := uint64(h[1] & 0x7F)
	if op >= opClose && (n > 125 || !fin) {
		return false, 0, nil, fmt.Errorf("websocket: control frame %#x is fragmented or longer than 125 bytes", op)
	}
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(c.br, b[:]); err != nil {
			return false, 0, nil, fmt.Errorf("websocket: read: %w", err)
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(c.br, b[:]); err != nil {
			return false, 0, nil, fmt.Errorf("websocket: read: %w", err)
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > MaxMessage {
		return false, 0, nil, fmt.Errorf("websocket: a frame claims %d bytes, over the %d bound", n, MaxMessage)
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return false, 0, nil, fmt.Errorf("websocket: read: %w", err)
	}
	return fin, op, payload, nil
}

// send writes one control frame under the write lock, a no-op once our close
// has gone.
func (c *Conn) send(op byte, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closeSent {
		return nil
	}
	return c.writeFrame(op, payload)
}

// Shutdown sends a normal-closure frame and returns, for a connection whose
// reader runs elsewhere: that reader receives the server's answering close as
// ErrClosed, and the socket closes there.
func (c *Conn) Shutdown(ctx context.Context) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	defer c.bound(ctx, c.nc.SetWriteDeadline)()
	return c.writeFrame(opClose, []byte{0x03, 0xE8})
}

// Abort closes the socket without a close handshake, for a peer that did not
// answer Shutdown inside its bound. A reader blocked elsewhere returns at once.
func (c *Conn) Abort() error { return c.nc.Close() }

// Close is Shutdown for a connection nobody else is reading: it waits inside
// ctx for the server's close, and closes the socket whatever the server does.
func (c *Conn) Close(ctx context.Context) error {
	defer func() { _ = c.nc.Close() }()
	if err := c.Shutdown(ctx); err != nil {
		return err
	}
	defer c.bound(ctx, c.nc.SetReadDeadline)()
	for {
		_, op, _, err := c.readFrame()
		if err != nil {
			return fmt.Errorf("websocket: no close answered ours: %w", err)
		}
		if op == opClose {
			return nil
		}
	}
}
