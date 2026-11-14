package browser

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// feed runs a timeline through a fresh arm at the default rate and answers the
// faults it raised, in order. Each entry is a response at that millisecond.
func feed(responses []int64, logs []int64) []Fault {
	a := rateArm{r: DefaultRate}
	var out []Fault
	li := 0
	for _, ts := range responses {
		for li < len(logs) && logs[li] <= ts {
			a.log(logs[li])
			li++
		}
		if f, ok := a.response(ts); ok {
			out = append(out, f)
		}
	}
	return out
}

// every spreads n responses evenly through [from, from+span) milliseconds.
func every(from, span int64, n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = from + int64(i)*span/int64(n)
	}
	return out
}

func join(parts ...[]int64) []int64 {
	var out []int64
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// THE THRESHOLD'S EDGE, ON BOTH SIDES (rook, coop r9/175): the comparison is
// AT LEAST PerSecond in each second, so exactly 20 per second fires and 19 in
// either second does not -- the one place where > and >= read differently.
func TestTheRateFiresAtTheThresholdAndNotBelowIt(t *testing.T) {
	for _, c := range []struct {
		name  string
		resp  []int64
		fires int
	}{
		{"exactly 20 in each of two seconds", join(every(1, 1000, 20), every(1001, 1000, 20)), 1},
		{"19 in the first second", join(every(1, 1000, 19), every(1001, 1000, 20)), 0},
		{"19 in the second second", join(every(1, 1000, 20), every(1001, 1000, 19)), 0},
		{"20 per second for one second only", every(1, 1000, 20), 0},
		{"a 100ms poll, 10 per second for ten seconds", every(1, 10000, 100), 0},
		// A heavy page load: sixty responses in half a second, then quiet. It
		// must exceed the window's total of 40 (20 x 2s), or a rule reading the
		// TOTAL would pass this cell too -- which it did when the burst was 30.
		{"a load burst of 60 in half a second", every(1, 500, 60), 0},
		{"a fetch loop, 150 per second for three seconds", every(1, 3000, 450), 1},
		{"an honest 1-second poll for ten seconds", every(1, 10000, 10), 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := feed(c.resp, nil); len(got) != c.fires {
				t.Fatalf("fired %d time(s) %v, want %d", len(got), got, c.fires)
			}
		})
	}
}

// ONE FINDING PER EPISODE: a loop that runs for ten seconds is one loop, and
// the arm re-arms only after a second falls short, so a second loop later is a
// second finding.
func TestTheRateFiresOncePerEpisodeAndRearmsAfterAGap(t *testing.T) {
	loop := func(from int64) []int64 { return every(from, 5000, 200) } // 40/s for 5s
	if got := feed(loop(1), nil); len(got) != 1 {
		t.Fatalf("one five-second loop fired %d times, want once", len(got))
	}
	if got := feed(join(loop(1), loop(20001)), nil); len(got) != 2 {
		t.Fatalf("two loops fifteen seconds apart fired %d time(s), want twice", len(got))
	}
}

// LOG ENTRIES ARE COUNTED BESIDE AND NEVER SUMMED IN (fable r9/173): forty log
// entries do not lift nineteen responses a second over the line, and when the
// arm does fire, the sentence carries the log count on its own.
func TestLogEntriesAreCountedBesideTheRateAndNeverSummedIn(t *testing.T) {
	logs := every(1, 1800, 40) // all forty land before the last response, at 1901
	if got := feed(join(every(1, 1000, 19), every(1001, 1000, 19)), logs); len(got) != 0 {
		t.Fatalf("19 responses a second with 20 log entries a second fired %v: the logs were summed in", got)
	}
	got := feed(join(every(1, 1000, 20), every(1001, 1000, 20)), logs)
	if len(got) != 1 || !strings.HasPrefix(got[0].String(), "request rate: ") || !strings.Contains(got[0].Text, "40 responses in 2s") || !strings.Contains(got[0].Text, "40 log entries") {
		t.Fatalf("got %v, want one rate fault naming 40 responses and 40 log entries apart", got)
	}
}

// THE ARM AGAINST THE MEASURED PAGES, on the real pair. The step closes at
// load, so a test, unlike the stage, may dwell: it asks Since until the arm
// fires or the dwell runs out. The dwell is the TEST's patience and not the
// arm's property -- the arm's window is on the browser's clock.
//
// ONE DWELL FOR BOTH CLAIMS, 15s. A loop stops the moment the arm fires; a
// quiet page must stay quiet for all of it. The dwell was 3.5s, and the full
// suite measured that short once: the fetch loop was heard NOT at all inside
// 3.5s at load 12-14, though under 14 spinners alone it fired in 1.3-1.4s, 4
// of 4. The suite-only delay is not explained. And an absence needs at least
// the time a presence gets (rook, coop r9/182): the same delay that held back
// the loop's events could hold back a slipped poll's past a shorter dwell, so
// "quiet at any load" is measured over the window the firing claim uses. It
// costs about 45s of this package's time, the price of the absence being held.
// Time-to-fire is logged on every run, so it gets measured, not guessed at.
func TestTheRateArmAgainstTheMeasuredPages(t *testing.T) {
	ctx, d, w, session, base := realWatch(t, "", func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api":
			rw.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(rw, "{}")
		case "/poll1s":
			page(rw, `<script>setInterval(() => fetch("/api"), 1000)</script>`)
		case "/poll100":
			page(rw, `<script>setInterval(() => fetch("/api"), 100)</script>`)
		case "/fetchloop":
			page(rw, `<script>(async () => { for (;;) { await fetch("/api") } })()</script>`)
		case "/burst":
			imgs := ""
			for i := range 60 { // past the window total of 40, so a total would fire
				imgs += fmt.Sprintf(`<img src="/api?i=%d">`, i)
			}
			page(rw, imgs)
		default:
			page(rw, `<p>static</p>`)
		}
	})
	for _, c := range []struct {
		page  string
		fires bool
	}{
		{"/clean", false}, {"/burst", false}, {"/poll1s", false},
		// An honest page that must NOT fire at any load (r9/179). At a default
		// of 10 it sat on the line and the box's load decided the verdict.
		{"/poll100", false},
		{"/fetchloop", true},
	} {
		t.Run(c.page, func(t *testing.T) {
			const dwell = 15 * time.Second
			began := time.Now()
			var heard []Fault
			heard = append(heard, step(t, ctx, d, w, session, base, c.page, "")...)
			for end := began.Add(dwell); time.Now().Before(end) && !hasRate(heard); {
				time.Sleep(250 * time.Millisecond)
				more, err := w.Since(ctx)
				if err != nil {
					t.Fatal(err)
				}
				heard = append(heard, more...)
			}
			t.Logf("%s: rate fired=%v after %s", c.page, hasRate(heard), time.Since(began).Round(100*time.Millisecond))
			if hasRate(heard) != c.fires {
				t.Fatalf("%s: rate fired=%v over a %s dwell, want %v; heard %v", c.page, hasRate(heard), dwell, c.fires, heard)
			}
			// Leave the page, so a loop stops before the next case's boundary.
			if err := d.navigate(ctx, session, base, "/clean", ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func hasRate(fs []Fault) bool {
	for _, f := range fs {
		if f.Kind == "rate" {
			return true
		}
	}
	return false
}
