// Package fakedriver is a W3C WebDriver that drives no browser, for the arms
// that have to measure the conversation without one.
//
// WHY IT IS NOT A TEST FILE. Two test binaries need it -- internal/browser's,
// which measures the protocol, and cmd/rumor's, which measures the stage that
// starts and stops a driver -- and a protocol fake copied into two places
// drifts, so one of the two arms ends up measuring a fake nobody else agrees
// with. internal/rollback/toyrestart is this tree's precedent for a package
// that exists to be driven by tests.
//
// WHAT IT IS NOT IS EVIDENCE ABOUT A REAL BROWSER. It answers the three
// endpoints a session's lifecycle needs and launches nothing, so an arm driven
// against it says that rumor speaks WebDriver correctly and says nothing about
// whether chromium starts. That claim needs the real pair, and in this tree
// that arm runs in CI.
package fakedriver

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SlowFor is how long mode "slow" withholds readiness. Short enough to be a
// cheap cell and long enough that a one-second bound cannot pass it.
const SlowFor = 3 * time.Second

const (
	// ModeEnv carries the behavior wanted: "ready" to answer /status ready,
	// "never" to answer it not-ready forever, "wedge" to come up ready and
	// then never answer a session -- which is a browser that launches and
	// hangs, the failure a readiness check cannot see -- or "refuse-close" to
	// open sessions and then refuse to delete them. Unset means this process is
	// not a fake driver at all.
	ModeEnv = "RUMOR_FAKE_DRIVER_MODE"

	// JournalEnv names a file the fake appends what it was asked to, so an arm
	// reads the conversation off the wire instead of inferring it from the code
	// that produced it.
	JournalEnv = "RUMOR_FAKE_DRIVER_JOURNAL"
)

// MainIfAsked serves as a fake driver when ModeEnv is set, and answers whether
// it did -- so a TestMain that wants one is three lines and cannot forget to
// stop the test run from continuing inside the child.
func MainIfAsked() bool {
	mode := os.Getenv(ModeEnv)
	if mode == "" {
		return false
	}
	serve(mode, os.Getenv(JournalEnv), os.Args[1:])
	return true
}

func serve(mode, journal string, args []string) {
	port := 0
	for _, a := range args {
		if strings.HasPrefix(a, "--port=") {
			port, _ = strconv.Atoi(strings.TrimPrefix(a, "--port="))
		}
	}
	if port == 0 {
		fmt.Fprintln(os.Stderr, "fake driver: started with no --port")
		os.Exit(2)
	}
	var mu sync.Mutex
	note := func(line string) {
		if journal == "" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		f, err := os.OpenFile(journal, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		defer func() { _ = f.Close() }()
		_, _ = fmt.Fprintln(f, line)
	}
	opened := 0
	mux := http.NewServeMux()
	// A DRIVER CAN BE SLOW RATHER THAN DEAD, which is the state rook measured on
	// a real chromedriver at coop r9/99: not ready, then ready. "slow" answers
	// not-ready for SlowFor and ready after, so a bound too tight to let a
	// starting driver finish is a failing cell rather than a false NOT DRIVABLE
	// on somebody's box.
	up := time.Now().Add(SlowFor)
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		ready := mode != "never"
		if mode == "slow" {
			ready = time.Now().After(up)
		}
		_, _ = fmt.Fprintf(w, `{"value":{"ready":%t}}`, ready)
	})
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		if mode == "wedge" {
			note("wedged")
			<-r.Context().Done()
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		opened++
		id := fmt.Sprintf("sess-%d", opened)
		mu.Unlock()
		note("open " + id + " " + strings.TrimSpace(string(body)))
		_, _ = fmt.Fprintf(w, `{"value":{"sessionId":%q,"capabilities":{}}}`, id)
	})
	mux.HandleFunc("/session/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			note("close " + strings.TrimPrefix(r.URL.Path, "/session/"))
			// A DELETE THAT FAILS IS A BROWSER LEFT RUNNING, and mode
			// "refuse-close" is how an arm measures whether anything notices.
			if mode == "refuse-close" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(w, `{"value":{"error":"unknown error","message":"no"}}`)
				return
			}
		}
		_, _ = fmt.Fprint(w, `{"value":null}`)
	})
	// THE PORT IS WRITTEN DOWN BEFORE IT IS SERVED, so an arm can keep asking it
	// after a failed start: that is the only way to tell a driver that was
	// cleaned up from one that was abandoned.
	note(fmt.Sprintf("listen %d", port))
	srv := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	_ = srv.ListenAndServe()
}
