package wire

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// DefaultRequestTimeout bounds ONE exchange inside a kept session.
//
// IT IS PER EXCHANGE AND NOT PER RUN, which is the whole difference from
// Describe. Describe bounds the entire call with exec.CommandContext, so the
// timeout and the process's lifetime are the same thing -- correct for a
// handshake and wrong for a run, where the context that kills the adapter has
// to outlive every individual request. A run-long ceiling would also mean one
// slow action and one wedged adapter are the same observation.
const DefaultRequestTimeout = 60 * time.Second

// answer is one line off the adapter's stdout, or the reason there will not be
// another.
type answer struct {
	line []byte
	err  error
}

// Session is one adapter spawned, described, and KEPT for the whole run, which
// is what docs/contract.md's transport section says the wire client does:
// "The wire client spawns the adapter from [adapter].command in platform.toml,
// sends describe first, and keeps the process for the whole run."
//
// WHY A SECOND ENTRY POINT BESIDE Describe RATHER THAN A WIDER Describe:
// Describe's own comment records that it kills the process rather than waiting,
// so that "an adapter that keeps the connection open for a run it is not going
// to get" cannot hold a probe to its timeout. That is right for validate's one
// question and is exactly what a run must not do. The two share `Spawn.start`
// and nothing above it: a probe closes stdin and kills, a run keeps both.
//
// ONE OUTSTANDING REQUEST AT A TIME is the protocol's rule, and what enforces
// it here is the ID CHECK rather than a flag (contract.md:36): "`id` is a check
// rather than a router: a response whose `id` is not the outstanding request's
// is a wire refusal naming both." That is also what an adapter writing an
// unsolicited line produces -- the next exchange reads that line and finds a
// stale id.
//
// I WROTE A REENTRANCY FLAG FIRST AND REMOVED IT. Because an exchange returns
// before the next can start, nothing single-threaded can reach it; the only
// caller that could is a concurrent one, which this type does not support, and
// a test driving two goroutines at an unsynchronized bool would be a data race
// the race detector should refuse. docs/testing-methodology.md's "An
// unreachable guard is not a cautious one" is the rule, and a guard that
// cannot be tested while advertising protection is worse than its absence: the
// id check catches the observable consequence either way.
//
// A Session IS NOT SAFE FOR CONCURRENT USE, deliberately: the protocol says
// "where N sessions are wanted, N processes" rather than N outstanding requests
// on one, because the engine owns the timing.
//
// EVERY FAILURE HERE IS could-not-run AND NEVER A REFUSAL OF THE PROJECT, the
// rule the top of client.go states. A timed-out or malformed exchange POISONS
// the session: the stream is mid-line or the adapter is wedged, and a later
// request on it could only read somebody else's answer. The poison is sticky
// and every subsequent call returns it, so a caller cannot accidentally carry
// on against a dead adapter.
type Session struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	answers chan answer
	done    chan struct{}
	// once makes Close idempotent under CONCURRENCY and not only in sequence.
	// The select-then-close it replaces was two steps: two callers could both
	// find `done` open and the second `close(done)` would panic. sync.Once also
	// gives the second caller a stronger guarantee than the old early return --
	// Do BLOCKS until the first call's teardown has finished, so a Close that
	// returns means the adapter is stopped rather than merely being stopped.
	once     sync.Once
	cancel   context.CancelFunc
	describe DescribeResponse
	timeout  time.Duration
	n        int
	dead     error
}

// Open spawns the adapter, sends `describe` as its first message, keeps the
// process, and hands back the session.
//
// The context governs the PROCESS and must outlive the run; `timeout` governs
// each exchange, the describe included -- the handshake is simply the first
// one, so there is no second ceiling to keep in step with this one.
func Open(ctx context.Context, s Spawn, timeout time.Duration) (*Session, error) {
	if len(s.Command) == 0 {
		return nil, fmt.Errorf("the platform declares no [adapter].command, so there is nothing to spawn")
	}
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	ctx, cancel := context.WithCancel(ctx)
	cmd, stdin, stdout, err := s.start(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	sess := &Session{
		cmd:     cmd,
		stdin:   stdin,
		answers: make(chan answer, 4),
		done:    make(chan struct{}),
		cancel:  cancel,
		timeout: timeout,
	}

	// ONE READER GOROUTINE, because a pipe read cannot carry a deadline: the
	// only way to bound an exchange is to read somewhere else and select. It
	// sends on `done` as well as `answers` so a poisoned session does not leave
	// it blocked on a send nobody will receive.
	go func() {
		br := bufio.NewReader(stdout)
		for {
			raw, rerr := br.ReadBytes('\n')
			if len(raw) > 0 {
				select {
				case sess.answers <- answer{line: raw}:
				case <-sess.done:
					return
				}
			}
			if rerr != nil {
				select {
				case sess.answers <- answer{err: rerr}:
				case <-sess.done:
				}
				return
			}
		}
	}()

	msg, err := sess.exchange(DescribeRequest{}, KindDescribe)
	if err != nil {
		_ = sess.Close()
		return nil, err
	}
	resp, ok := msg.(DescribeResponse)
	if !ok {
		_ = sess.Close()
		return nil, fmt.Errorf("the adapter answered %q where a describe was asked", msg.Env().Kind)
	}
	sess.describe = resp
	return sess, nil
}

// Described is the adapter's own account of its surface, from the handshake.
// It is held rather than re-asked: the protocol sends describe once.
func (s *Session) Described() DescribeResponse { return s.describe }

// Execute runs one action against the system under test.
func (s *Session) Execute(req ExecuteRequest) (ExecuteResponse, error) {
	var zero ExecuteResponse
	msg, err := s.exchange(req, KindExecute)
	if err != nil {
		return zero, err
	}
	resp, ok := msg.(ExecuteResponse)
	if !ok {
		return zero, s.poison(fmt.Errorf("the adapter answered %q where an execute was asked", msg.Env().Kind))
	}
	return resp, nil
}

// Observe asks the adapter what it sees.
func (s *Session) Observe(req ObserveRequest) (ObserveResponse, error) {
	var zero ObserveResponse
	msg, err := s.exchange(req, KindObserve)
	if err != nil {
		return zero, err
	}
	resp, ok := msg.(ObserveResponse)
	if !ok {
		return zero, s.poison(fmt.Errorf("the adapter answered %q where an observe was asked", msg.Env().Kind))
	}
	return resp, nil
}

// StartSession asks the adapter for an actor's session in some context.
//
// NAMED StartSession AND NOT Session because the type is already called that;
// the protocol's kind is still `session` and nothing on the wire changes.
func (s *Session) StartSession(req SessionRequest) (SessionResponse, error) {
	var zero SessionResponse
	msg, err := s.exchange(req, KindSession)
	if err != nil {
		return zero, err
	}
	resp, ok := msg.(SessionResponse)
	if !ok {
		return zero, s.poison(fmt.Errorf("the adapter answered %q where a session was asked", msg.Env().Kind))
	}
	return resp, nil
}

// exchange writes one request and reads the one response to it.
//
// The request's envelope is set HERE and not by the caller: the id is the
// session's own counter, so no caller can reuse one or skip one, and the
// contract version is this build's. A caller that could set the id could
// defeat the check that the id exists to make.
func (s *Session) exchange(req Message, kind Kind) (Message, error) {
	if s.dead != nil {
		return nil, s.dead
	}
	s.n++
	id := strconv.Itoa(s.n)
	env := Envelope{Kind: kind, Contract: ContractVersion, ID: id}

	// THE ENVELOPE IS STAMPED BY TYPE, not through a promoted field on an
	// interface value: Message carries Env() and nothing settable, so each
	// request shape is named once here. The same go1.26 language floor that
	// keeps Describe's literal spelled out applies (see client.go).
	var body any
	switch kind {
	case KindDescribe:
		r, _ := req.(DescribeRequest)
		r.Envelope = env
		body = r
	case KindExecute:
		r, _ := req.(ExecuteRequest)
		r.Envelope = env
		body = r
	case KindObserve:
		r, _ := req.(ObserveRequest)
		r.Envelope = env
		body = r
	case KindSession:
		r, _ := req.(SessionRequest)
		r.Envelope = env
		body = r
	default:
		return nil, fmt.Errorf("wire: %q is not a request kind this session can send", kind)
	}

	line, err := json.Marshal(body)
	if err != nil {
		return nil, s.poison(fmt.Errorf("encoding the %s request: %w", kind, err))
	}

	if _, err := s.stdin.Write(append(line, '\n')); err != nil {
		return nil, s.poison(fmt.Errorf("writing %s to the adapter: %w", kind, err))
	}

	var got answer
	select {
	case got = <-s.answers:
	case <-time.After(s.timeout):
		return nil, s.poison(fmt.Errorf("the adapter did not answer the %s request %s within %s: "+
			"the exchange is unfinished, so this session cannot be used again", kind, id, s.timeout))
	}
	if got.err != nil && len(got.line) == 0 {
		// EOF WITH NOTHING READ IS THE ADAPTER DECLINING TO SPEAK, and
		// docs/contract.md is explicit that EOF before the run ends is
		// could-not-run with the status in the detail.
		return nil, s.poison(fmt.Errorf("the adapter answered nothing to the %s request and closed: %w",
			kind, got.err))
	}

	msg, derr := DecodeResponse(got.line)
	if derr != nil {
		return nil, s.poison(fmt.Errorf("the adapter's answer to the %s request is not a well-formed "+
			"response: %w", kind, derr))
	}
	if gotID := msg.Env().ID; gotID != id {
		// THE REFUSAL NAMES BOTH, which is contract.md:36's own wording. It is
		// also what an unsolicited line produces: the next exchange reads it
		// and finds an id that is not the one outstanding.
		return nil, s.poison(fmt.Errorf("the adapter answered id %q where the outstanding %s request "+
			"was id %q: id is a check and not a router, so this session cannot be trusted to be in "+
			"step again", gotID, kind, id))
	}
	if errResp, ok := msg.(ErrorResponse); ok {
		// AN ERROR ENVELOPE IS NOT A POISON. It is a well-formed answer to
		// this request, and contract.md is explicit that every envelope is
		// could-not-run and that an adapter cannot refuse a project -- so the
		// session stays usable and the caller decides what the fault means.
		return nil, fmt.Errorf("the adapter answered a %s fault to the %s request: %s",
			errResp.Error.Kind, kind, errResp.Error.Detail)
	}
	return msg, nil
}

// poison records why this session is finished and returns that reason, so a
// call site can `return s.poison(err)` and a later call gets the same answer
// rather than a fresh and more confusing one.
func (s *Session) poison(err error) error {
	if s.dead == nil {
		s.dead = err
	}
	return s.dead
}

// Close stops the adapter.
//
// IT GIVES EOF A BOUNDED CHANCE BEFORE KILLING, through stopAdapter -- see its
// comment for the race this had and for why the kill is kept. Closing stdin is
// not a courtesy to a well-behaved adapter, it is the stop signal: the
// adapter's own teardown is a `defer`, and a defer does not run under SIGKILL,
// so killing at once made whether it cleaned up a coin flip.
//
// THE CANCEL COMES LAST AND THAT ORDER IS THE FIX. `s.cancel()` is the cancel
// of an exec.CommandContext, so calling it IS a kill -- it was running before
// the wait, which meant the grace this function now takes could never have been
// taken. Nothing is lost by moving it: the process is already reaped by then,
// and the call still releases the context.
func (s *Session) Close() error {
	s.once.Do(func() {
		close(s.done)
		stopAdapter(s.cmd, s.stdin)
		s.cancel()
	})
	return nil
}
