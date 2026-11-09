package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// WaitCadence is how often a wait's condition is re-evaluated. fable's ruling
// forbids a fixed sleep for the WAIT ITSELF -- the condition is polled until
// satisfied or out of budget rather than slept past -- and this is the polling
// interval, not a bound.
const WaitCadence = 50 * time.Millisecond

// waitBudget is how long a condition is given, and it is READ FROM THE FLOW
// rather than chosen here.
//
// fable's r9/110 says "until the flow's declared budget for the step runs out",
// and NO SUCH BUDGET IS DECLARED: bindings.Step carries Step, To, Field, Value,
// Target and Until, platform.toml's `budget` belongs to a PROFILE and bounds a
// whole run, and nothing names a per-step figure. So the only referent that
// exists is the bound a flow already has -- wire.DefaultRequestTimeout, which
// bounds one exchange inside a kept session and is what bounds the adapter-driven
// flow today. Honouring the caller's deadline introduces no new bound, which is
// the other half of that ruling; this is the fallback when the caller set none.
func waitBudget(ctx context.Context) time.Time {
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return time.Now().Add(wire.DefaultRequestTimeout)
}

// elementRef is the W3C key an element reference arrives under. It is a fixed
// UUID in the specification rather than a name a driver chooses, so it is
// written once here instead of at each decode.
const elementRef = "element-6066-11e4-a52e-4f735466cecf"

// cssSelector is the only location strategy this driver uses. W3C offers six;
// one is enough while `target` and `field` name element ids, and a second would
// be a choice no declaration asks for.
const cssSelector = "css selector"

// Flow drives one action's declared steps through this actor's session and
// answers in the wire's own shape, because the engine's resume depends on it:
// `Reached` is where it got to, `Stopped` is why it stopped short, and
// `Response` is what the system said.
//
// IT REPRODUCES THE ADAPTER'S CONTRACT (cmd/toy-adapter/main.go:699-790) rather
// than inventing one. The adapter is the stand-in -- wire.go:79 says KindFlow is
// "the steps a browser driver executes" -- so a browser-driven flow that
// answered a different shape would make the engine's retry read differently
// depending on who drove, which is the one thing a stand-in must not do.
//
// THE PATH IS CARRIED BETWEEN STEPS, as it is there: a click acts on the page a
// navigate opened, and a resume walks the steps before `From` for their path
// WITHOUT performing them. That is what "the request before it is never
// re-sent" means for a browser too -- except that a browser holds its own page,
// so the walk-forward re-navigates rather than only remembering a string.
// THE DETAIL IS A THIRD RETURN BECAUSE THE WIRE HAS NOWHERE FOR IT. fable's
// ruling (coop r9/110) is that a failed condition "names the condition and the
// last text observed"; wire.FlowResponse carries Reached, Stopped and the
// system's Response and no message, and the engine names the condition from the
// bindings when it records a ui_timeout -- so the LAST TEXT has no field on the
// wire at all. Rather than widen a contract an adapter also answers, the driver
// hands it back beside the response for the caller to report.
func (d *Driver) Flow(ctx context.Context, actor, base, token string,
	req wire.FlowRequest) (wire.FlowResponse, string, error) {

	if len(req.Steps) == 0 {
		return wire.FlowResponse{}, "", fmt.Errorf("a flow for %q carried no steps, so there is "+
			"nothing to drive", req.Action)
	}
	if req.From < 0 || req.From >= len(req.Steps) {
		return wire.FlowResponse{}, "", fmt.Errorf("a flow for %q asked to resume at %d with %d step(s)",
			req.Action, req.From, len(req.Steps))
	}
	// EVERY REFUSAL A READER CAN SETTLE IS SETTLED HERE, BEFORE ANYTHING OPENS.
	// This pass was bought by the gate: the checks below used to live inside the
	// executing loop, so a flow of [navigate, wait ""] drove its navigate and
	// only then refused the wait -- and that arm passed only because a navigate
	// to an address nothing serves SUCCEEDED. With the dial in place the
	// navigate refused first and masked the wait's own refusal, which is how the
	// ordering defect became visible: a step whose fault is legible in the step
	// LIST must never cost a page, a session, or a change to the world.
	if rerr := readSteps(req); rerr != nil {
		return wire.FlowResponse{}, "", rerr
	}
	session, err := d.SessionFor(ctx, actor)
	if err != nil {
		return wire.FlowResponse{}, "", err
	}

	// THE BEFORE-TEXT IS CAPTURED BY LOOKING AHEAD, because the ruling pins it to
	// a moment that has already passed by the time the wait is read: a
	// `<id>-changed` condition compares against "its text content captured
	// immediately before the preceding step ran" (fable, coop r9/110). So before
	// performing step j the driver asks whether step j+1 is such a wait, and
	// reads that element first. Capturing after the step would compare the
	// element to itself.
	before := map[string]string{}
	path := ""
	for i, st := range req.Steps {
		kind, _ := st["step"].(string)
		if i+1 < len(req.Steps) && i+1 >= req.From {
			if id, changed := waitTarget(req.Steps[i+1]); changed {
				// NO PAGE YET IS "ABSENT", NOT "EMPTY". When the preceding step
				// is the navigate itself there is no document to read, and the
				// ruling settles what that means: "if the element was absent
				// before, its appearance is a change". Recording the empty
				// string instead would reach the same verdict here by accident
				// and the WRONG one for an element whose text is legitimately
				// empty -- found by this arm's own vacuity guard.
				//
				// AND IT DOES NOT SKIP THE STEP, which it did until an arm
				// drove a flow of [navigate, wait "x-changed"]: this branch
				// ended in `continue`, which continues the STEP LOOP and not
				// the look-ahead, so the navigate that was about to run was
				// never performed. The wait then compared against a page
				// nothing had opened and the flow reported it unmet -- a
				// `ui_timeout` for a page the driver never visited.
				//
				// IT ONLY FIRES WHERE THE WAIT FOLLOWS THE FIRST STEP, which is
				// why every other arm passed over it: in the toy's own flow the
				// `-changed` wait follows a CLICK, so `path` is set by the
				// navigate before it and this branch is not taken.
				if path == "" {
					before[id] = absentBefore
				} else {
					text, present, terr := d.text(ctx, session, "#"+id)
					if terr != nil {
						return wire.FlowResponse{}, "", fmt.Errorf("step %d of %q: reading #%s before step %d: %w",
							i+1, req.Action, id, i+2, terr)
					}
					// AN ABSENT ELEMENT IS RECORDED AS ABSENT AND NOT AS EMPTY,
					// because the ruling makes its APPEARANCE a change: "if the
					// element was absent before, its appearance is a change". An
					// empty string would make an element that appears empty read
					// as unchanged.
					if present {
						before[id] = text
					} else {
						before[id] = absentBefore
					}
				}
			}
		}
		switch kind {
		case bindings.StepNavigate:
			// READ ALREADY: readSteps has refused an empty `to` and an
			// unresolved placeholder, so what is left here is the navigation.
			path, _ = st["to"].(string)
			// A RESUME RE-NAVIGATES rather than only remembering the string,
			// which is where a browser differs from the adapter: the adapter's
			// `path` is a prefix it will POST to later, and a browser's is a
			// page that must actually be open for the next click to land on it.
			// Re-opening a page is not re-sending the step the walk already
			// performed -- the step is the navigation, and its effect is the
			// page, which a resumed session may no longer be showing.
			if err := d.navigate(ctx, session, base, path, token); err != nil {
				return wire.FlowResponse{}, "", fmt.Errorf("step %d of %q: %w", i+1, req.Action, err)
			}
		case bindings.StepFill:
			if i < req.From {
				continue
			}
			field, _ := st["field"].(string)
			value, _ := st["value"].(string)
			el, found, ferr := d.element(ctx, session, "#"+field)
			if ferr != nil {
				return wire.FlowResponse{}, "", fmt.Errorf("step %d of %q: %w", i+1, req.Action, ferr)
			}
			if !found {
				// THE FIELD IS NOT THERE, WHICH IS THE PAGE'S ANSWER AND NOT A
				// FAULT OF THE PROJECT'S. A declared field the page does not
				// carry is the system failing to offer what the bindings say it
				// offers, so it stops the flow at this step the way a refused
				// click does.
				return stoppedAt(i, wire.StopStep), "", nil
			}
			if err := d.sendKeys(ctx, session, el, value); err != nil {
				return wire.FlowResponse{}, "", fmt.Errorf("step %d of %q: %w", i+1, req.Action, err)
			}
		case bindings.StepClick:
			if i < req.From {
				continue
			}
			target, _ := st["target"].(string)
			clicked, cerr := d.click(ctx, session, target)
			if cerr != nil {
				return wire.FlowResponse{}, "", fmt.Errorf("step %d of %q: %w", i+1, req.Action, cerr)
			}
			if !clicked {
				return stoppedAt(i, wire.StopStep), "", nil
			}
		case bindings.StepWait:
			if i < req.From {
				continue
			}
			until, _ := st["until"].(string)
			id, changed := waitTarget(st)
			met, last, werr := d.await(ctx, session, id, changed, before[id])
			if werr != nil {
				return wire.FlowResponse{}, "", fmt.Errorf("step %d of %q: %w", i+1, req.Action, werr)
			}
			if !met {
				// A CONDITION THAT DID NOT COME TRUE IS StopWait, which is the
				// one stop the engine RE-CHECKS at the same position rather than
				// treating as a failure -- and on the second failure of the same
				// wait it records a ui_timeout. So this is not an error: it is
				// the flow stopping where the page did not do what the bindings
				// say it does.
				return stoppedAt(i, wire.StopWait),
					redact(fmt.Sprintf("%q was not met: #%s read %s", until, id, last), token),
					nil
			}
		default:
			// UNREACHABLE: readSteps refuses a kind this driver does not drive
			// before anything opens. It is an error and not a fall-through
			// because a kind added to bindings without a case here would
			// otherwise be silently skipped -- a step the project declared and
			// the run did not take.
			return wire.FlowResponse{}, "", fmt.Errorf("step %d of %q is %q, which is not a step this "+
				"driver drives", i+1, req.Action, kind)
		}
	}
	// WHAT A "RESPONSE" MEANS FOR A BROWSER STEP IS THIS PHASE'S QUESTION, and
	// examples/toy/bindings.toml:305-309 says so outright. The adapter answers
	// the HTTP status its POST received; a W3C driver is not told one -- the
	// page is all it gets. So this answers NO response rather than a status it
	// cannot know, which keeps the adapter's own rule that a flow asking nothing
	// of the system reports no status instead of a `0` dressed as one.
	return wire.FlowResponse{Reached: len(req.Steps)}, "", nil
}

// readSteps settles every refusal that is legible in the step list, before a
// session is opened or a page is touched.
//
// WHY IT IS A SEPARATE PASS AND NOT THE LOOP'S FIRST LINES: the loop drives as
// it goes, so a fault in step 3 was reached only after steps 1 and 2 had
// already acted on the world. A flow of [navigate, wait ""] navigated and THEN
// refused -- and the arm asserting that refusal passed only because a navigate
// to an address nothing serves silently succeeded. Both defects were one
// ordering mistake, and the sibling arm's name said the rule aloud the whole
// time: a flow refuses what it cannot drive BEFORE opening anything.
//
// IT MIRRORS THE LOOP'S OWN SKIPPING rather than validating more than the loop
// would: a resume's earlier steps were driven by the call that stopped, so their
// fields are not re-read here. `navigate` is the exception in both places,
// because a resumed session may no longer be showing the page.
func readSteps(req wire.FlowRequest) error {
	path := ""
	for i, st := range req.Steps {
		kind, _ := st["step"].(string)
		switch kind {
		case bindings.StepNavigate:
			to, _ := st["to"].(string)
			if to == "" {
				return fmt.Errorf("step %d of %q navigates nowhere", i+1, req.Action)
			}
			if strings.Contains(to, "{") {
				// THE ENGINE RESOLVES PLACEHOLDERS, and a driver handed the
				// literal has no entity to point at. The adapter says this in
				// the same words; guessing would make a 404 look like the
				// system's answer.
				return fmt.Errorf("step %d of %q navigates to %q, which still carries a "+
					"placeholder: the engine resolves those before sending", i+1, req.Action, to)
			}
			path = to
		case bindings.StepFill:
			if i < req.From {
				continue
			}
			if path == "" {
				return fmt.Errorf("step %d of %q fills before anything navigated", i+1, req.Action)
			}
			if field, _ := st["field"].(string); field == "" {
				return fmt.Errorf("step %d of %q fills no field", i+1, req.Action)
			}
		case bindings.StepClick:
			if i < req.From {
				continue
			}
			if path == "" {
				return fmt.Errorf("step %d of %q clicks before anything navigated", i+1, req.Action)
			}
			if target, _ := st["target"].(string); target == "" {
				return fmt.Errorf("step %d of %q clicks nothing", i+1, req.Action)
			}
		case bindings.StepWait:
			if i < req.From {
				continue
			}
			if until, _ := st["until"].(string); until == "" {
				return fmt.Errorf("step %d of %q waits for nothing", i+1, req.Action)
			}
			if path == "" {
				return fmt.Errorf("step %d of %q waits before anything navigated", i+1, req.Action)
			}
		default:
			return fmt.Errorf("step %d of %q is %q, which is not a step this driver drives",
				i+1, req.Action, kind)
		}
	}
	return nil
}

// stoppedAt is a flow that stopped short at step i, in the wire's shape.
func stoppedAt(i int, why wire.Stop) wire.FlowResponse {
	return wire.FlowResponse{Reached: i, Stopped: why}
}

// navigate opens a page for this session, carrying the actor's token.
//
// THE TOKEN GOES IN THE QUERY BECAUSE THE PAGE TAKES IT NOWHERE ELSE: cmd/toy's
// session() reads an Authorization header or `?token=`, and a browser
// navigation cannot set a header. It is a credential in a URL, so it is never
// put in an error or a line -- the redaction below is why `to` and not the full
// URL is what any message carries.
func (d *Driver) navigate(ctx context.Context, session, base, to, token string) error {
	u, err := url.Parse(strings.TrimSuffix(base, "/") + to)
	if err != nil {
		return fmt.Errorf("the page %q is not a URL against %q: %w", to, base, err)
	}
	if token != "" {
		q := u.Query()
		q.Set("token", token)
		u.RawQuery = q.Encode()
	}
	if rerr := d.reachable(ctx, u.Host); rerr != nil {
		return rerr
	}
	var out struct {
		Value struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		} `json:"value"`
	}
	if err := d.call(ctx, http.MethodPost, "/session/"+session+"/url",
		map[string]any{"url": u.String()}, &out); err != nil {
		return fmt.Errorf("navigating to %q: %w", to, err)
	}
	if out.Value.Error != "" {
		// THE DRIVER'S OWN MESSAGE IS REDACTED, not trusted: it is the one
		// string here that can contain the URL this function built.
		return fmt.Errorf("navigating to %q: %s: %s", to, out.Value.Error,
			redact(out.Value.Message, token))
	}
	return nil
}

// reachable proves something answers at a system address before a page is asked
// for, and remembers the answer.
//
// WHY IT IS HERE AT ALL: a navigation to an address nothing serves SUCCEEDS.
// chromedriver answers the navigate and loads the browser's own error page, so
// the next step finds no element and the flow stops -- recorded as the SYSTEM
// refusing a step it never saw. W3C WebDriver has no HTTP status for a
// navigation, so this is the one place the distinction can be made.
//
// WHAT IT PROVES AND WHAT IT DOES NOT, stated because the bound is narrow: a
// TCP connection proves A LISTENER AT THAT ADDRESS and nothing more. It does
// not prove the listener is the system under test, or that it serves the path
// the flow names -- an address pointing at the WRONG live server still drives.
// It refuses exactly one state, the one measured above: an address where
// nothing answers at all.
func (d *Driver) reachable(ctx context.Context, host string) error {
	if err, seen := d.worlds[host]; seen {
		return err
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err == nil {
		_ = conn.Close()
		d.worlds[host] = nil
		return nil
	}
	// THE ADAPTER IS NAMED AS THE SOURCE, because the address is not the
	// project's and not this package's: it came back on the adapter's describe
	// (fable, coop r9/111), and an operator told only "connection refused"
	// would look for the fault in the browser.
	d.worlds[host] = fmt.Errorf("nothing answers at %s, which is the address this run's adapter "+
		"reported on its describe: a flow cannot be driven against a system that is not "+
		"there, and a browser navigated at it would load its own error page and read as the "+
		"page refusing a step: %w", host, err)
	return d.worlds[host]
}

// element finds one element by css selector, answering whether it is there.
//
// ABSENT IS NOT AN ERROR, which is the distinction the two callers need: a
// field or target the page does not carry is the SYSTEM's answer and stops the
// flow, while a driver that would not answer at all is this build's problem.
func (d *Driver) element(ctx context.Context, session, selector string) (string, bool, error) {
	var out struct {
		Value map[string]any `json:"value"`
	}
	if err := d.call(ctx, http.MethodPost, "/session/"+session+"/element",
		map[string]any{"using": cssSelector, "value": selector}, &out); err != nil {
		// A NO SUCH ELEMENT ANSWER ARRIVES AS A 404, which `call` reports as an
		// error: the absence is the page's answer and is handed back as one.
		if strings.Contains(err.Error(), "404") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("looking for %q: %w", selector, err)
	}
	ref, _ := out.Value[elementRef].(string)
	if ref == "" {
		return "", false, nil
	}
	return ref, true, nil
}

// click clicks the element a `target` names, answering whether it was there.
//
// A FORM IS SUBMITTED THROUGH ITS OWN CONTROL, not by clicking the form: the
// toy's `target` is `join` and that id belongs to a <form>, whose click does
// nothing at all. So a target that resolves to a form clicks the submit control
// inside it, which is what a reader of `click target = "join"` means and what
// the adapter's POST to the form's own action does.
func (d *Driver) click(ctx context.Context, session, target string) (bool, error) {
	for _, selector := range []string{
		"#" + target + " [type=submit]",
		"#" + target,
	} {
		el, found, err := d.element(ctx, session, selector)
		if err != nil {
			return false, err
		}
		if !found {
			continue
		}
		var out struct {
			Value struct {
				Error   string `json:"error"`
				Message string `json:"message"`
			} `json:"value"`
		}
		if err := d.call(ctx, http.MethodPost, "/session/"+session+"/element/"+el+"/click",
			map[string]any{}, &out); err != nil {
			return false, fmt.Errorf("clicking %q: %w", selector, err)
		}
		if out.Value.Error != "" {
			return false, fmt.Errorf("clicking %q: %s: %s", selector, out.Value.Error,
				out.Value.Message)
		}
		return true, nil
	}
	return false, nil
}

// sendKeys types a value into an element.
func (d *Driver) sendKeys(ctx context.Context, session, el, value string) error {
	var out struct {
		Value struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		} `json:"value"`
	}
	if err := d.call(ctx, http.MethodPost, "/session/"+session+"/element/"+el+"/value",
		map[string]any{"text": value}, &out); err != nil {
		return fmt.Errorf("filling an element: %w", err)
	}
	if out.Value.Error != "" {
		return fmt.Errorf("filling an element: %s: %s", out.Value.Error, out.Value.Message)
	}
	return nil
}

// Redacted is what a token is replaced by wherever one could otherwise be
// printed.
const Redacted = "<token redacted>"

// redact removes the actor's token from anything on its way into an error, a
// detail or a line.
//
// fable's ruling (coop r9/110): navigate carries the token by `?token=` because
// the driver gives no header, "and that value is REDACTED in every line, error,
// log and fixture the stage emits". THE LEAK IS NOT IN WHAT THIS BUILD WRITES
// but in what it passes through: no error here formats the built URL, and
// chromedriver's own message for a failed navigation can echo it back verbatim.
// A page's text can carry it too -- the toy's own form action embeds the token
// -- so a wait's detail is redacted for the same reason.
//
// AN EMPTY TOKEN REDACTS NOTHING, which matters: replacing "" would rewrite
// every message into markers.
//
// AND A TOKEN IS A WHOLE VALUE, NOT A SUBSTRING OF A WORD. A blanket
// ReplaceAll corrupts ordinary prose when the token is short -- MEASURED by
// this package's own arm, which drives with the token "t" and read back
// `"never" was no<token redacted> me<token redacted>: #never read absen...`.
// Every letter t in the sentence had been redacted.
//
// THE ALTERNATIVE WOULD HAVE BEEN A LENGTH THRESHOLD, and that is a number
// chosen rather than measured -- the fault this phase has already paid for
// twice. A boundary rule needs no figure: an occurrence counts only where the
// characters around it are not themselves part of a value, so "t" inside "not"
// is left alone and `token=t` is not.
func redact(text, token string) string {
	if token == "" {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], token) &&
			!valueChar(byteBefore(text, i)) &&
			!valueChar(byteAt(text, i+len(token))) {
			b.WriteString(Redacted)
			i += len(token)
			continue
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String()
}

// valueChar is whether a byte could be part of a token, which is what decides
// whether a match is the whole value or the middle of a word.
func valueChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '-', c == '_':
		return true
	}
	return false
}

func byteBefore(s string, i int) byte {
	if i == 0 {
		return 0
	}
	return s[i-1]
}

func byteAt(s string, i int) byte {
	if i >= len(s) {
		return 0
	}
	return s[i]
}

// absentBefore stands for "the element was not there", which is not the same as
// an empty text: the ruling makes an appearance a change, so the two must be
// told apart.
const absentBefore = "\x00absent"

// waitTarget reads a wait step's condition into the element id it names and
// whether the condition is the CHANGED form.
//
// THE VOCABULARY IS fable's, NOT THIS BUILD'S (coop r9/110, carried into
// docs/vocabulary.md by this commit): a condition is `<id>` or `<id>-changed`,
// `<id>` being an element id on the page under drive. So "count-changed" is
// #count's text differing and "doc-published" is #doc-published being present.
func waitTarget(st map[string]any) (string, bool) {
	until, _ := st["until"].(string)
	if kind, _ := st["step"].(string); kind != bindings.StepWait || until == "" {
		return "", false
	}
	if id := strings.TrimSuffix(until, "-changed"); id != until {
		return id, true
	}
	return until, false
}

// await polls one condition at the driver's cadence until it is satisfied or the
// flow's budget runs out, and answers what it last saw either way.
//
// NO FIXED SLEEP, which is the ruling's own words and bindings.go:1095's reason
// for refusing a duration: the condition is asked repeatedly and the loop ends
// when it is true or when the budget is spent, never after a chosen pause.
func (d *Driver) await(ctx context.Context, session, id string, changed bool,
	was string) (bool, string, error) {

	deadline := waitBudget(ctx)
	last := "<never read>"
	for {
		// THE BUDGET IS CHECKED BEFORE THE POLL, NOT AFTER IT. Checking after
		// meant the first poll past the deadline went out on an expired
		// context, and the HTTP client's own "context deadline exceeded" came
		// back as an ERROR about this build -- which read as a broken driver
		// where the truth was a condition that did not come true in its budget.
		if !time.Now().Before(deadline) {
			return false, last, nil
		}
		text, present, err := d.text(ctx, session, "#"+id)
		if err != nil {
			// AND A READ THAT FAILED BECAUSE THE BUDGET RAN OUT UNDER IT is the
			// same event seen from the other side: the deadline can pass while
			// the request is in flight.
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return false, last, nil
			}
			return false, last, fmt.Errorf("reading #%s: %w", id, err)
		}
		switch {
		case !changed && present:
			return true, text, nil
		case changed && present && was == absentBefore:
			// IT WAS NOT THERE AND NOW IS, which the ruling counts as a change.
			return true, text, nil
		case changed && present && text != was:
			return true, text, nil
		}
		if present {
			last = fmt.Sprintf("%q", text)
		} else {
			last = "absent"
		}
		if !time.Now().Before(deadline) {
			return false, last, nil
		}
		select {
		case <-ctx.Done():
			// A DEADLINE REACHED IS THE BUDGET SPENT, NOT AN ERROR. waitBudget
			// reads the budget FROM the caller's deadline, so the ctx expiring
			// and the budget running out are the same event -- and the ruling
			// makes that StopWait, which the engine re-checks, rather than a
			// failure about this build. A CANCELLATION is different: nobody
			// asked the condition to fail, the run was stopped.
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return false, last, nil
			}
			return false, last, ctx.Err()
		case <-time.After(WaitCadence):
		}
	}
}

// text reads one element's text content, answering whether it is there.
func (d *Driver) text(ctx context.Context, session, selector string) (string, bool, error) {
	el, found, err := d.element(ctx, session, selector)
	if err != nil || !found {
		return "", false, err
	}
	t, terr := d.textOf(ctx, session, el)
	if terr != nil {
		// A STALE REFERENCE IS NOT AN ERROR HERE, IT IS THE PAGE MOVING. The
		// element is found and read in TWO calls, so a navigation that lands
		// between them invalidates the reference and the driver answers 404
		// "stale element reference". For a wait that is not a fault: the page
		// changing is the thing it is waiting FOR, so this tick simply has no
		// reading and the next one re-finds.
		//
		// MEASURED, AND IT COST A RED CI: the forge failed task 1116 on
		// `count-changed` exactly here -- the click's POST response landed
		// between the find and the read -- and it reproduced 1 run in 6 on this
		// box, which is why it passed the gate and failed there.
		if isStale(terr) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("reading the text of %q: %w", selector, terr)
	}
	return t, true, nil
}

// isStale answers whether a driver refused a reference because the element it
// named is gone -- the page having moved under it.
//
// IT MATCHES THE W3C ERROR NAME AND NOT THE STATUS, which is the narrowing that
// matters: a 404 from this protocol also means "no such element" and "no such
// window", and treating every 404 as a moved page would swallow a selector that
// matches nothing into silence.
func isStale(err error) bool {
	return err != nil && strings.Contains(err.Error(), "stale element reference")
}

// textOf reads one element's text content by REFERENCE, and it exists so there
// is ONE reader of that fact: a flow's wait finds an element by selector and a
// browser observation already holds a reference, and two calls to the same
// endpoint would be free to drift in the path or in what they decode.
func (d *Driver) textOf(ctx context.Context, session, ref string) (string, error) {
	var out struct {
		Value string `json:"value"`
	}
	if err := d.call(ctx, http.MethodGet, "/session/"+session+"/element/"+ref+"/text",
		nil, &out); err != nil {
		return "", err
	}
	return out.Value, nil
}
