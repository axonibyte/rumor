package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// shadowRef is the W3C key a shadow root reference arrives under.
const shadowRef = "shadow-6066-11e4-a52e-4f735466cecf"

// Snapshot is what a page was when a finding was recorded on it (issue 124's
// deliverable 2, fable's ruling at coop r9/191): the files written, named
// relative to the directory the caller gave, and what was not written and why.
type Snapshot struct {
	// HTML is the DOM, redacted. Always written when a snapshot is taken.
	HTML string
	// PNG is the screenshot, written only when asked for and only when the
	// check found the token nowhere on the page. Empty otherwise.
	PNG string
	// Note is the sentence about the screenshot: withheld and why, kept and
	// what the check could not see, or not asked for. It is the finding's to
	// carry, so a reader is never left to infer which.
	Note string
}

// ScreenshotResidue is what the token check cannot see, which every kept
// screenshot's note names and the flag's help repeats. A token painted as
// pixels leaves nothing in the DOM to find. And a CLOSED shadow root is checked
// only where the driver hands it back: chromedriver 156 does (measured, the
// /closed arm), but the WebDriver spec answers "no such shadow root" for one,
// so a driver that follows it leaves that content unseen.
const ScreenshotResidue = "canvas and image pixels cannot be checked for the token, nor a closed shadow root the driver will not hand back"

// Snapshot is Capture said as one sentence, which is what the walk records on
// the step and the run prints under its FOUND line: the files, relative to the
// directory, then the note about the screenshot.
func (d *Driver) Snapshot(ctx context.Context, actor, token, dir, name string, screenshot bool) (string, error) {
	snap, err := d.Capture(ctx, actor, token, dir, name, screenshot)
	if err != nil {
		return "", err
	}
	files := snap.HTML
	if snap.PNG != "" {
		files += ", " + snap.PNG
	}
	return files + "; " + snap.Note, nil
}

// Capture writes the actor's page into dir as name.html, and as name.png too
// when screenshot is true and the page carries no trace of the token.
//
// THE DOM IS REDACTED AND THE PNG CANNOT BE (coop r9/186-190). A credential in
// the page source is text and redact() takes it out; one drawn on the screen is
// pixels, and nothing can be taken out of those. So the PNG is opt-in, an
// unsafe screenshot being one somebody asked for, never a default (r9/191).
// And even when asked for it is WITHHELD if the token is anywhere a check can
// reach. That is the page source, every form control's LIVE value, every frame,
// and every shadow root, because each of those was measured painting the token
// while a narrower check read clean.
func (d *Driver) Capture(ctx context.Context, actor, token, dir, name string, screenshot bool) (Snapshot, error) {
	session, ok := d.sessions[actor]
	if !ok {
		return Snapshot{}, fmt.Errorf("no session for %q, so there is no page to snapshot", actor)
	}
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return Snapshot{}, fmt.Errorf("snapshot name %q must be a plain file name", name)
	}
	// Back to the top document whatever happens: the walk acts there, and a
	// check that left the session inside a frame would move the next step.
	defer func() {
		_ = d.call(context.Background(), http.MethodPost, "/session/"+session+"/frame", map[string]any{"id": nil}, nil)
	}()
	if err := d.call(ctx, http.MethodPost, "/session/"+session+"/frame", map[string]any{"id": nil}, nil); err != nil {
		return Snapshot{}, fmt.Errorf("returning to the top document: %w", err)
	}
	var src struct {
		Value string `json:"value"`
	}
	if err := d.call(ctx, http.MethodGet, "/session/"+session+"/source", nil, &src); err != nil {
		return Snapshot{}, fmt.Errorf("reading the page source: %w", err)
	}
	snap := Snapshot{HTML: name + ".html"}
	if err := os.WriteFile(filepath.Join(dir, snap.HTML), []byte(redact(src.Value, token)), 0o600); err != nil {
		return Snapshot{}, fmt.Errorf("writing the DOM snapshot: %w", err)
	}
	if !screenshot {
		snap.Note = "screenshot not asked for"
		return snap, nil
	}
	if token != "" {
		where, err := d.tokenIn(ctx, session, token, 0)
		if err != nil {
			return Snapshot{}, fmt.Errorf("checking the page for the token before a screenshot: %w", err)
		}
		if where != "" {
			snap.Note = "screenshot withheld: the actor's token is on the page, in " + where
			return snap, nil
		}
	}
	var shot struct {
		Value string `json:"value"`
	}
	if err := d.call(ctx, http.MethodGet, "/session/"+session+"/screenshot", nil, &shot); err != nil {
		return Snapshot{}, fmt.Errorf("taking the screenshot: %w", err)
	}
	png, err := base64.StdEncoding.DecodeString(shot.Value)
	if err != nil {
		return Snapshot{}, fmt.Errorf("the driver's screenshot is not base64: %w", err)
	}
	snap.PNG = name + ".png"
	if err := os.WriteFile(filepath.Join(dir, snap.PNG), png, 0o600); err != nil {
		return Snapshot{}, fmt.Errorf("writing the screenshot: %w", err)
	}
	snap.Note = "screenshot kept: the token was in no page source, form control value, frame or shadow root; " + ScreenshotResidue
	return snap, nil
}

// maxFrameDepth bounds the frame walk: a page can nest frames without end, and
// a check that never finishes cannot withhold anything.
const maxFrameDepth = 8

// tokenIn answers where in the CURRENT browsing context the token is, or "".
// It reads the source, every form control's live value, every shadow root, and
// recurses into every frame, leaving the session in the context it found.
func (d *Driver) tokenIn(ctx context.Context, session, token string, depth int) (string, error) {
	if depth > maxFrameDepth {
		return fmt.Sprintf("frames nested past %d deep, which the check will not follow", maxFrameDepth), nil
	}
	var src struct {
		Value string `json:"value"`
	}
	if err := d.call(ctx, http.MethodGet, "/session/"+session+"/source", nil, &src); err != nil {
		return "", err
	}
	if strings.Contains(src.Value, token) {
		return "the page source" + frameSuffix(depth), nil
	}
	base := "/session/" + session
	if where, err := d.tokenInElements(ctx, session, base+"/elements", token, depth, false); where != "" || err != nil {
		return where, err
	}
	frames, err := d.refs(ctx, base+"/elements", "iframe, frame")
	if err != nil {
		return "", err
	}
	for _, f := range frames {
		if err := d.call(ctx, http.MethodPost, base+"/frame", map[string]any{"id": map[string]any{elementRef: f}}, nil); err != nil {
			return "", fmt.Errorf("entering a frame: %w", err)
		}
		where, err := d.tokenIn(ctx, session, token, depth+1)
		if perr := d.call(ctx, http.MethodPost, base+"/frame/parent", map[string]any{}, nil); perr != nil && err == nil {
			err = fmt.Errorf("leaving a frame: %w", perr)
		}
		if where != "" || err != nil {
			return where, err
		}
	}
	return "", nil
}

// tokenInElements checks every element a `find elements` endpoint answers: its
// live value when it is a form control, its text when it sits in a shadow root
// (whose content the page source does not serialise), and the shadow root it
// hosts, recursively.
func (d *Driver) tokenInElements(ctx context.Context, session, find, token string, depth int, inShadow bool) (string, error) {
	all, err := d.refs(ctx, find, "*")
	if err != nil {
		return "", err
	}
	base := "/session/" + session
	where := frameSuffix(depth)
	if inShadow {
		where = " inside a shadow root" + where
	}
	for _, el := range all {
		var v struct {
			Value any `json:"value"`
		}
		if err := d.call(ctx, http.MethodGet, base+"/element/"+el+"/property/value", nil, &v); err == nil {
			if s, ok := v.Value.(string); ok && strings.Contains(s, token) {
				return "a form control's live value" + where, nil
			}
		}
		if inShadow {
			var t struct {
				Value string `json:"value"`
			}
			if err := d.call(ctx, http.MethodGet, base+"/element/"+el+"/text", nil, &t); err == nil && strings.Contains(t.Value, token) {
				return "the text" + where, nil
			}
		}
		var sr struct {
			Value map[string]any `json:"value"`
		}
		if err := d.call(ctx, http.MethodGet, base+"/element/"+el+"/shadow", nil, &sr); err != nil {
			continue // no shadow root here, the common answer
		}
		root, _ := sr.Value[shadowRef].(string)
		if root == "" {
			continue
		}
		if w, err := d.tokenInElements(ctx, session, base+"/shadow/"+root+"/elements", token, depth, true); w != "" || err != nil {
			return w, err
		}
	}
	return "", nil
}

// refs answers the element references a `find elements` endpoint returns.
func (d *Driver) refs(ctx context.Context, find, selector string) ([]string, error) {
	var out struct {
		Value []map[string]any `json:"value"`
	}
	if err := d.call(ctx, http.MethodPost, find, map[string]any{"using": cssSelector, "value": selector}, &out); err != nil {
		return nil, fmt.Errorf("finding %q: %w", selector, err)
	}
	refs := make([]string, 0, len(out.Value))
	for _, v := range out.Value {
		if r, ok := v[elementRef].(string); ok {
			refs = append(refs, r)
		}
	}
	return refs, nil
}

func frameSuffix(depth int) string {
	if depth == 0 {
		return ""
	}
	return fmt.Sprintf(" of a frame %d deep", depth)
}
