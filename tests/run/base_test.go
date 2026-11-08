package run_test

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// TestTheAdapterReportsTheWorldsAddress is fable's ruling (coop r9/111) at the
// place it decided: "the adapter REPORTS its base on describe ... The
// toy-adapter reports the base it serves or was given."
//
// IT RUNS IN THIS PACKAGE because this TestMain is the one that builds the real
// pair -- `toy` and `toy-adapter` both on PATH -- and the claim is about the
// REAL adapter. A fake answering a base it was told to answer would measure the
// fixture.
//
// BOTH CASES, because the ruling names both and they reach `a.base` by
// different code: an adapter that started the system itself, and one handed an
// address through TOY_ADDR. One cell passing proves nothing about the other.
func TestTheAdapterReportsTheWorldsAddress(t *testing.T) {
	t.Run("the toy it started itself", func(t *testing.T) {
		sess, err := wire.Open(context.Background(),
			wire.Spawn{Command: []string{"toy-adapter"}}, 40*time.Second)
		if err != nil {
			t.Fatalf("opening the real toy-adapter: %v", err)
		}
		t.Cleanup(func() { _ = sess.Close() })

		base := sess.Described().Base
		if base == "" {
			t.Fatal("the adapter reported no `base`, so a declared flow's `navigate` has no " +
				"address to resolve against and the walk must refuse it")
		}
		if !strings.HasPrefix(base, "http://") {
			t.Errorf("the base is %q, which is not an address a `navigate` can resolve", base)
		}

		// AND IT IS THE WORLD IT IS ACTUALLY TALKING TO, not a plausible
		// string: the port is dialed. An address that parses and answers
		// nothing is exactly what a guessed host looks like, so the arm would
		// pass over the defect it exists to catch.
		host := strings.TrimPrefix(base, "http://")
		conn, derr := net.DialTimeout("tcp", host, 10*time.Second)
		if derr != nil {
			t.Fatalf("nothing is listening at the base the adapter reported (%s): %v", base, derr)
		}
		_ = conn.Close()
	})

	t.Run("the address it was given", func(t *testing.T) {
		// A REAL LISTENER, so the address is one that exists. The adapter does
		// not probe TOY_ADDR -- it records it -- so this cell is about what it
		// REPORTS, and a port nothing holds would make the arm's own address
		// the thing under test.
		ln, lerr := net.Listen("tcp", "127.0.0.1:0")
		if lerr != nil {
			t.Fatalf("holding a port for TOY_ADDR: %v", lerr)
		}
		defer func() { _ = ln.Close() }()
		addr := ln.Addr().String()

		t.Setenv("TOY_ADDR", addr)
		// TOY_ADDR IS NAMED IN THE SPAWN, which is the production path and not
		// a convenience: wire.Spawn's Env is a list of variable NAMES passed
		// through, so an adapter inherits nothing the project did not declare
		// ("an adapter needing BASE_URL says so in platform.toml"). Setting the
		// variable alone proved it: the adapter never saw it and started a toy
		// of its own, and the first draft of this arm read that as the adapter
		// ignoring TOY_ADDR.
		sess, err := wire.Open(context.Background(),
			wire.Spawn{Command: []string{"toy-adapter"}, Env: []string{"TOY_ADDR"}},
			40*time.Second)
		if err != nil {
			t.Fatalf("opening the toy-adapter against TOY_ADDR=%s: %v", addr, err)
		}
		t.Cleanup(func() { _ = sess.Close() })

		if got, want := sess.Described().Base, "http://"+addr; got != want {
			t.Errorf("the adapter reported base %q for TOY_ADDR=%s, want %q: an adapter handed "+
				"a world must report THAT world, or a browser drives one the run is not "+
				"observing", got, addr, want)
		}
	})
}

// TestAnAdapterReportingNoBaseIsStillAValidDescribe keeps the field ADDITIVE,
// which is what lets the contract stay "2" (fable, same ruling). `rumor
// fake-adapter` sets no base, and a run against it must still describe, which
// is the state every adapter in the wild is in today.
func TestAnAdapterReportingNoBaseIsStillAValidDescribe(t *testing.T) {
	if _, err := os.Stat("fixtures/walkable"); err != nil {
		t.Skipf("the walkable fixture is not here: %v", err)
	}
	sess, err := wire.Open(context.Background(),
		wire.Spawn{Command: []string{"rumor", "fake-adapter"}}, 40*time.Second)
	if err != nil {
		t.Fatalf("opening the fake adapter: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	if base := sess.Described().Base; base != "" {
		t.Errorf("the fake adapter reported base %q; it sets none, so an empty field is the "+
			"state the optional `base` has to decode cleanly from", base)
	}
	if len(sess.Described().Actions) == 0 {
		t.Error("the describe carried no actions, so the handshake did not survive the new " +
			"optional field")
	}
}
