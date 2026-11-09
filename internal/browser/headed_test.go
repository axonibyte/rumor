package browser

import (
	"context"
	"strings"
	"testing"
)

// TestAHeadedRunWithNoDisplaySaysSo is issue 122's headed-by-flag deliverable at
// the point where the flag can go wrong: `--headed` on a box with no display.
//
// WHAT THE DRIVER SAYS ALONE IS TRUE AND USELESS. MEASURED on this box:
// `POST /session` answers 500 with "Chrome instance exited. Examine ChromeDriver
// verbose log to determine the cause" -- a log the run does not keep, about a
// cause the run already knows. The clause adds the two facts it has.
//
// THE CELLS ARE THE THREE STATES THAT DECIDE IT, driven through whyNoSession
// rather than through a browser: a real headed failure needs a displayless box
// AND a pair, so a unit cell is the only place all three can be held -- and the
// arm below drives the real one where this box allows it.
func TestAHeadedRunWithNoDisplaySaysSo(t *testing.T) {
	for _, c := range []struct {
		name     string
		headless bool
		display  string
		wayland  string
		says     bool
	}{
		{"headed with no display at all", false, "", "", true},
		{"headed under X", false, ":0", "", false},
		{"headed under wayland", false, "", "wayland-0", false},
		// HEADLESS SAYS NOTHING, which is the narrowing: the display is
		// irrelevant then, and a reader chasing a mention of it would be
		// chasing a red herring away from the real fault.
		{"headless with no display", true, "", "", false},
		{"headless under X", true, ":0", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("DISPLAY", c.display)
			t.Setenv("WAYLAND_DISPLAY", c.wayland)
			got := (&Driver{headless: c.headless}).whyNoSession()
			if c.says && !strings.Contains(got, "DISPLAY") {
				t.Errorf("the clause is %q and does not name the display, so the run relays a "+
					"chromedriver 500 and nothing else", got)
			}
			if !c.says && got != "" {
				t.Errorf("the clause is %q for %s, where the display is not the reason",
					got, c.name)
			}
		})
	}
}

// AND THE REAL FAILURE CARRIES IT, which the cells above cannot show: they hold
// the clause's own logic, and this holds that a session refused by a real
// chromedriver arrives with the clause attached.
//
// IT SKIPS WHERE A DISPLAY EXISTS, by the environment's own verdict: on a box
// with one, a headed session may well open, and asserting a failure there would
// be asserting that the box is poorer than it is.
func TestARealHeadedSessionCarriesTheDisplayClause(t *testing.T) {
	if whyNoSession := (&Driver{}).whyNoSession(); whyNoSession == "" {
		t.Skip("this box has a display, so a headed session need not fail here")
	}
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}

	// HEADED, which is the whole subject: the same pair opens a session headless
	// in every other arm in this file.
	d, err := Start(context.Background(), driver, browserBin, false)
	noteGroup(d)
	if err != nil {
		t.Fatalf("the driver itself would not start, which is a different fault: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	_, serr := d.SessionFor(context.Background(), "alice")
	if serr == nil {
		t.Skip("a headed session opened on a box reporting no display, so this box reaches one " +
			"by a route the clause does not know about -- which is why the clause is a hint and " +
			"not a refusal before the run")
	}
	for _, says := range []string{"HEADED", "DISPLAY", "WAYLAND_DISPLAY", "headless is the default"} {
		if !strings.Contains(serr.Error(), says) {
			t.Errorf("the refusal does not say %q, so its reader is sent to a chromedriver log "+
				"for a cause this run knew: %v", says, serr)
		}
	}
}
