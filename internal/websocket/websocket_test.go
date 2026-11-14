package websocket

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// THE SERVER SIDE IS WRITTEN AGAIN HERE, ON PURPOSE. These arms check the
// client's framing, and a server that reused the client's own encoder would
// agree with it whatever it did. So the frames below are built and parsed from
// RFC 6455 section 5.2 directly, and the duplication is the check.

// server accepts one connection, answers the handshake with answer (which may
// sabotage it), and then runs script over the raw socket.
func server(t *testing.T, answer func(key string) string, script func(nc net.Conn, br *bufio.Reader)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { _ = ln.Close(); <-done })
	go func() {
		defer close(done)
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = nc.Close() }()
		// A client that never sends what the script waits for fails the arm
		// NAMING the missing frame, rather than at the test binary's timeout.
		_ = nc.SetDeadline(time.Now().Add(5 * time.Second))
		br := bufio.NewReader(nc)
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		_, _ = io.WriteString(nc, answer(req.Header.Get("Sec-WebSocket-Key")))
		if script != nil {
			script(nc, br)
		}
	}()
	return "ws://" + ln.Addr().String() + "/session/x"
}

// rfcSample is section 1.3's worked example, an oracle from outside this tree.
const rfcSampleKey, rfcSampleAccept = "dGhlIHNhbXBsZSBub25jZQ==", "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="

func good(key string) string {
	return "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		Accept(key) + "\r\n\r\n"
}

// serverFrame encodes one UNMASKED server frame; b0 is the whole first byte so
// an arm can set FIN, RSV or a reserved opcode as it needs.
func serverFrame(b0 byte, payload []byte) []byte {
	out := []byte{b0}
	switch n := len(payload); {
	case n < 126:
		out = append(out, byte(n))
	case n <= 0xFFFF:
		out = append(out, 126, byte(n>>8), byte(n))
	default:
		out = append(out, 127)
		out = binary.BigEndian.AppendUint64(out, uint64(n))
	}
	return append(out, payload...)
}

type clientFrame struct {
	fin, masked bool
	lenBytes    int // how many bytes encoded the length beyond the first two: 0, 2 or 8
	op          byte
	key         [4]byte
	wire        []byte // the payload as it crossed the wire, still masked
	payload     []byte // unmasked by this file's own arithmetic
}

func readClientFrame(t *testing.T, br *bufio.Reader) clientFrame {
	t.Helper()
	var h [2]byte
	if _, err := io.ReadFull(br, h[:]); err != nil {
		t.Errorf("server: read a client frame: %v", err)
		return clientFrame{}
	}
	f := clientFrame{fin: h[0]&0x80 != 0, op: h[0] & 0x0F, masked: h[1]&0x80 != 0}
	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		_, _ = io.ReadFull(br, b[:])
		n = uint64(b[0])<<8 | uint64(b[1])
		f.lenBytes = 2
	case 127:
		var b [8]byte
		_, _ = io.ReadFull(br, b[:])
		n = binary.BigEndian.Uint64(b[:])
		f.lenBytes = 8
	}
	if f.masked {
		_, _ = io.ReadFull(br, f.key[:])
	}
	f.wire = make([]byte, n)
	_, _ = io.ReadFull(br, f.wire)
	f.payload = make([]byte, n)
	for i := range f.wire {
		f.payload[i] = f.wire[i] ^ f.key[i%4]
	}
	return f
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestAcceptAnswersTheRFCsOwnExample(t *testing.T) {
	if got := Accept(rfcSampleKey); got != rfcSampleAccept {
		t.Fatalf("Accept(%q) = %q, RFC 6455 section 1.3 says %q", rfcSampleKey, got, rfcSampleAccept)
	}
}

// Each sabotaged answer is refused NAMING what was wrong. The presence partner
// is every other arm in this file, each of which dials the unsabotaged answer.
func TestAHandshakeTheClientCannotTrustIsRefusedByName(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		answer     func(string) string
	}{
		{"not 101", "not 101 Switching Protocols", func(string) string {
			return "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"
		}},
		{"wrong accept", "does not answer the key", func(string) string { return good(rfcSampleKey) }},
		{"no upgrade", "Upgrade is", func(k string) string { return strings.Replace(good(k), "Upgrade: websocket\r\n", "", 1) }},
		{"no connection", "carries no upgrade", func(k string) string { return strings.Replace(good(k), "Connection: Upgrade\r\n", "", 1) }},
		{"an extension", "offered none", func(k string) string {
			return strings.Replace(good(k), "\r\n\r\n", "\r\nSec-WebSocket-Extensions: permessage-deflate\r\n\r\n", 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Dial(ctx(t), server(t, tc.answer, nil))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Dial: %v, want an error containing %q", err, tc.want)
			}
		})
	}
	if _, err := Dial(ctx(t), "wss://127.0.0.1:1/"); err == nil || !strings.Contains(err.Error(), "ws:// only") {
		t.Fatalf("wss:// was not refused by name: %v", err)
	}
}

// Section 5.3: every client frame masked, a fresh key each time. The three
// sizes reach the 7-bit, 16-bit and 64-bit length encodings.
func TestEveryClientFrameIsMaskedWithAFreshKey(t *testing.T) {
	msgs := [][]byte{bytes.Repeat([]byte("a"), 100), bytes.Repeat([]byte("b"), 300), bytes.Repeat([]byte("c"), 70000)}
	got := make(chan clientFrame, 2*len(msgs))
	url := server(t, good, func(_ net.Conn, br *bufio.Reader) {
		for range 2 * len(msgs) {
			got <- readClientFrame(t, br)
		}
	})
	c, err := Dial(ctx(t), url)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		for range 2 {
			if err := c.WriteText(ctx(t), m); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, m := range msgs {
		a, b := <-got, <-got
		for _, f := range []clientFrame{a, b} {
			if !f.fin || f.op != opText || !f.masked {
				t.Errorf("%d bytes: fin=%v op=%#x masked=%v, want a final masked text frame", len(m), f.fin, f.op, f.masked)
			}
			// Section 5.2: "the minimal number of bytes MUST be used to encode
			// the length", so each size has exactly one legal encoding.
			if want := minimalLenBytes(len(m)); f.lenBytes != want {
				t.Errorf("%d bytes: length encoded in %d extra bytes, want the minimal %d", len(m), f.lenBytes, want)
			}
			if !bytes.Equal(f.payload, m) {
				t.Errorf("%d bytes: unmasked payload differs from what was sent", len(m))
			}
			if bytes.Equal(f.wire, m) {
				t.Errorf("%d bytes: the wire carried the payload in the clear", len(m))
			}
		}
		if a.key == b.key {
			t.Errorf("%d bytes: two frames shared the mask key %x", len(m), a.key)
		}
	}
}

// A message in three fragments with a ping between them: the client returns
// the whole message, and answers the ping with a pong carrying its payload.
func TestAFragmentedMessageIsReassembledAroundAPing(t *testing.T) {
	pong := make(chan clientFrame, 1)
	url := server(t, good, func(nc net.Conn, br *bufio.Reader) {
		_, _ = nc.Write(serverFrame(opText, []byte("hel")))
		_, _ = nc.Write(serverFrame(0x80|opPing, []byte("p1")))
		_, _ = nc.Write(serverFrame(opContinuation, []byte("lo ")))
		_, _ = nc.Write(serverFrame(0x80|opContinuation, []byte("world")))
		pong <- readClientFrame(t, br)
	})
	c, err := Dial(ctx(t), url)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := c.ReadText(ctx(t))
	if err != nil || string(msg) != "hello world" {
		t.Fatalf("ReadText = %q, %v; want \"hello world\"", msg, err)
	}
	if p := <-pong; p.op != opPong || !p.masked || string(p.payload) != "p1" {
		t.Fatalf("the ping was answered with op=%#x masked=%v payload=%q, want a masked pong \"p1\"", p.op, p.masked, p.payload)
	}
}

func TestAServerCloseIsEchoedAndItsStatusNamed(t *testing.T) {
	echo := make(chan clientFrame, 1)
	url := server(t, good, func(nc net.Conn, br *bufio.Reader) {
		_, _ = nc.Write(serverFrame(0x80|opClose, []byte{0x03, 0xE9}))
		echo <- readClientFrame(t, br)
	})
	c, err := Dial(ctx(t), url)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ReadText(ctx(t))
	var closed *ErrClosed
	if !errors.As(err, &closed) || closed.Code != 1001 {
		t.Fatalf("ReadText = %v, want ErrClosed with status 1001", err)
	}
	if e := <-echo; e.op != opClose || !e.masked || !bytes.Equal(e.payload, []byte{0x03, 0xE9}) {
		t.Fatalf("the close was echoed as op=%#x masked=%v payload=%x, want a masked close 03e9", e.op, e.masked, e.payload)
	}
}

func TestCloseSendsNormalClosureAndWaitsForTheAnswer(t *testing.T) {
	sent := make(chan clientFrame, 1)
	url := server(t, good, func(nc net.Conn, br *bufio.Reader) {
		f := readClientFrame(t, br)
		sent <- f
		_, _ = nc.Write(serverFrame(0x80|opClose, f.payload))
	})
	c, err := Dial(ctx(t), url)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(ctx(t)); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if f := <-sent; f.op != opClose || !f.masked || !bytes.Equal(f.payload, []byte{0x03, 0xE8}) {
		t.Fatalf("Close sent op=%#x masked=%v payload=%x, want a masked close 03e8", f.op, f.masked, f.payload)
	}
}

// What section 5 forbids a server to send is refused, by name.
func TestAFrameTheServerMayNotSendIsRefused(t *testing.T) {
	huge := []byte{0x80 | opText, 127}
	huge = binary.BigEndian.AppendUint64(huge, MaxMessage+1)
	for _, tc := range []struct {
		name, want string
		frame      []byte
	}{
		{"masked", "masked a frame", append([]byte{0x80 | opText, 0x80 | 2, 1, 2, 3, 4}, 'h'^1, 'i'^2)},
		{"rsv bit", "RSV bit", serverFrame(0x80|0x40|opText, []byte("hi"))},
		{"binary", "binary frame", serverFrame(0x80|opBinary, []byte("hi"))},
		{"fragmented ping", "fragmented", serverFrame(opPing, []byte("p"))},
		{"reserved opcode", "reserved", serverFrame(0x80|0x3, []byte("hi"))},
		{"stray continuation", "no message to continue", serverFrame(0x80|opContinuation, []byte("hi"))},
		{"over the bound", "over the", huge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url := server(t, good, func(nc net.Conn, _ *bufio.Reader) { _, _ = nc.Write(tc.frame) })
			c, err := Dial(ctx(t), url)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.ReadText(ctx(t)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ReadText: %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// A read that nothing answers ends at the caller's deadline, not never.
func TestAReadEndsAtTheCallersDeadline(t *testing.T) {
	// Released AFTER server() registers its own cleanup, because cleanups run
	// last-in first-out and the server's waits for this script to return.
	hold := make(chan struct{})
	url := server(t, good, func(net.Conn, *bufio.Reader) { <-hold })
	t.Cleanup(func() { close(hold) })
	c, err := Dial(ctx(t), url)
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	// The read runs aside so that a client ignoring the deadline FAILS THIS ARM
	// by name instead of hanging until the test binary's own timeout.
	got := make(chan error, 1)
	go func() { _, err := c.ReadText(short); got <- err }()
	select {
	case err := <-got:
		if err == nil {
			t.Fatal("ReadText returned with nothing sent")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReadText was still blocked 2s past a 200ms deadline")
	}
}

func minimalLenBytes(n int) int {
	switch {
	case n < 126:
		return 0
	case n <= 0xFFFF:
		return 2
	}
	return 8
}

// THE WATCH READS ON ONE GOROUTINE AND WRITES ON ANOTHER, so a write that runs
// out of time must not take the read down with it. On the first cut both
// directions shared one SetDeadline, and a write's cancellation set the
// deadline in the past for BOTH -- killing a reader that had nothing to do with
// it, which for the BiDi watch is every event after.
//
// THE WRITE IS MADE TO BLOCK, by a server that stops reading and a payload far
// past the socket buffers, so its deadline fires mid-write rather than after.
//
// NOT CLAIMED HERE: that frames never interleave. A single net.Conn Write is
// already serialised by Go's own write lock, and writeFrame issues exactly one,
// so an arm for interleaving would pass with the mutex removed and prove
// nothing about it.
func TestAWriteThatTimesOutLeavesTheReaderAlive(t *testing.T) {
	release := make(chan struct{})
	url := server(t, good, func(nc net.Conn, _ *bufio.Reader) {
		<-release // read nothing until the client's write has given up
		_, _ = nc.Write(serverFrame(0x80|opText, []byte("still here")))
	})
	c, err := Dial(ctx(t), url)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan string, 1)
	go func() {
		msg, err := c.ReadText(context.Background())
		if err != nil {
			got <- "ERROR " + err.Error()
			return
		}
		got <- string(msg)
	}()
	short, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := c.WriteText(short, make([]byte, 32<<20)); err == nil {
		t.Fatal("a 32 MiB write to a server reading nothing finished inside 200ms")
	}
	close(release)
	select {
	case msg := <-got:
		if msg != "still here" {
			t.Fatalf("the reader got %q: the write's timeout reached it", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the reader heard nothing after the write gave up")
	}
}

// Shutdown is for a connection read elsewhere: it sends ONE close, the reader
// receives the server's answer as ErrClosed and does not echo it back, and
// nothing may be written after (section 5.5.1).
func TestShutdownClosesOnceAndTheReaderHearsTheAnswer(t *testing.T) {
	frames := make(chan clientFrame, 4)
	url := server(t, good, func(nc net.Conn, br *bufio.Reader) {
		f := readClientFrame(t, br)
		frames <- f
		_, _ = nc.Write(serverFrame(0x80|opClose, f.payload))
		// Anything further the client sends is a defect: read until it hangs up.
		var h [2]byte
		if _, err := io.ReadFull(br, h[:]); err == nil {
			frames <- clientFrame{op: h[0] & 0x0F}
		}
		close(frames)
	})
	c, err := Dial(ctx(t), url)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	go func() { _, err := c.ReadText(context.Background()); got <- err }()
	if err := c.Shutdown(ctx(t)); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	var closed *ErrClosed
	if err := <-got; !errors.As(err, &closed) || closed.Code != 1000 {
		t.Fatalf("the reader got %v, want ErrClosed with status 1000", err)
	}
	if err := c.WriteText(ctx(t), []byte("late")); err == nil || !strings.Contains(err.Error(), "closing") {
		t.Fatalf("a write after Shutdown answered %v, want a refusal naming the close", err)
	}
	// The client must HANG UP once the close is answered, and promptly: the
	// server's 5s deadline would end this loop anyway, so a client that never
	// closed its socket would otherwise pass here, slowly.
	var sent []clientFrame
	deadline := time.After(2 * time.Second)
	for open := true; open; {
		select {
		case f, ok := <-frames:
			if !ok {
				open = false
				break
			}
			sent = append(sent, f)
		case <-deadline:
			t.Fatal("the client never hung up after the close handshake")
		}
	}
	if len(sent) != 1 || sent[0].op != opClose || !bytes.Equal(sent[0].payload, []byte{0x03, 0xE8}) {
		t.Fatalf("the client sent %d frame(s) %+v, want exactly one close 03e8", len(sent), sent)
	}
}
