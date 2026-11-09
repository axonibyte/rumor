package browser

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Read is a `browser` observation: what the actor's own browser SHOWS at a page.
//
// FABLE'S RULING (coop r9/120) IS THE WHOLE OF THE SHAPE: the adapter reports
// the page and the element on describe, and "the driver navigates the actor's
// context to page, reads the element's text content from the LIVE DOM, and that
// is the `browser` observation". The adapter's own read of the served HTML stays
// `direct`, the engine's second oracle.
//
// THE ACTOR'S OWN CONTEXT AND NOT A SPARE ONE, which is deliverable 1's rule
// holding here: an observation read in another session would be a different
// reader's view, and WHOSE view it is decides what a statute may see. This is
// also why an observation can move the actor's page -- the navigation happens in
// that actor's session, so the next flow resumes by re-navigating, which `Flow`
// already does for its own reason.
//
// AN ABSENT ELEMENT IS AN ANSWER AND NOT AN ERROR, for the reason fable gave
// about `missing` at coop rumor/1669: a page that is not served is what a
// browser SEES, and so is a page that does not carry the element. The caller
// turns `false` into the walk's one spelling of that word; this function does
// not know it. A DRIVER THAT WOULD NOT ANSWER AT ALL is the third case and
// stays an error, because that is this build's problem rather than the page's.
func (d *Driver) Read(ctx context.Context, actor, base, page, element, token string) (
	text string, children int, present bool, err error) {
	// REFUSED HERE AND NAMED AT THE WALK. The walk refuses a browser
	// observation whose describe carries no page or element, with the message a
	// project reads; this is the same refusal one layer in, so a future caller
	// cannot reach "#" or an empty path by a route that skips it.
	if strings.TrimSpace(page) == "" {
		return "", 0, false, fmt.Errorf("a browser observation for %q names no page, so there is "+
			"nothing to navigate to", actor)
	}
	if strings.TrimSpace(element) == "" {
		return "", 0, false, fmt.Errorf("a browser observation for %q names no element, so there "+
			"is nothing to read", actor)
	}
	if strings.ContainsAny(element, "#.[]()>+~*,:\"' \t") {
		// THE ID VOCABULARY, held twice over: the wire refuses this on the
		// describe and so does this, because a selector reaching the DOM would
		// read as an element nobody can find rather than as a malformed answer.
		return "", 0, false, fmt.Errorf("a browser observation for %q names element %q, which is "+
			"a selector and not an ID", actor, element)
	}

	session, serr := d.SessionFor(ctx, actor)
	if serr != nil {
		return "", 0, false, serr
	}
	if nerr := d.navigate(ctx, session, base, page, token); nerr != nil {
		return "", 0, false, nerr
	}

	// THE ELEMENT IS FOUND ONCE AND BOTH FACTS COME OFF IT. Finding it twice --
	// once for the text and once for the children -- would read two different
	// elements on a page that changed between the two calls, and the walk would
	// then hold a text and a count that were never true together.
	//
	// AND A PAGE THAT MOVES MID-READ IS RE-READ ONCE, not reported. The find and
	// the two reads are three calls, so a navigation landing among them makes
	// the reference STALE -- which is not the page's answer about the
	// observation, it is the observation having asked while the page turned. One
	// retry reads the page that is there now; a second staleness is reported,
	// because a page that will not hold still for two reads is a fact a reader
	// needs rather than a loop to hide in.
	text, kids, found, rerr := d.readOnce(ctx, session, element)
	if isStale(rerr) {
		text, kids, found, rerr = d.readOnce(ctx, session, element)
	}
	if rerr != nil {
		return "", 0, false, rerr
	}
	return text, kids, found, nil
}

// readOnce finds the element and takes both facts off it, answering a stale
// reference as the error it is so the caller can decide to re-read.
func (d *Driver) readOnce(ctx context.Context, session, element string) (
	string, int, bool, error) {

	ref, found, ferr := d.element(ctx, session, "#"+element)
	if ferr != nil || !found {
		return "", 0, false, ferr
	}
	text, terr := d.textOf(ctx, session, ref)
	if terr != nil {
		return "", 0, false, terr
	}
	kids, kerr := d.childrenOf(ctx, session, ref)
	if kerr != nil {
		return "", 0, false, kerr
	}
	return text, kids, true, nil
}

// childrenOf counts an element's CHILD ELEMENTS, which is the `count` reading
// (fable, coop r9/124): "the number of the element's child elements, which is
// exactly what the adapter has computed from the served HTML all along".
//
// `:scope > *` AND NOT `*`, which is the whole of the word: a bare `*` from an
// element finds every DESCENDANT, so a list whose items carried a span each
// would count twice what a reader sees as items. The children are what the
// adapter's own oracle counts, so they are what this counts.
func (d *Driver) childrenOf(ctx context.Context, session, ref string) (int, error) {
	var out struct {
		Value []map[string]any `json:"value"`
	}
	if err := d.call(ctx, http.MethodPost,
		"/session/"+session+"/element/"+ref+"/elements",
		map[string]any{"using": cssSelector, "value": ":scope > *"}, &out); err != nil {
		// AN ELEMENT WITH NO CHILDREN IS NOT AN ERROR. A W3C find-elements
		// answers an empty list for a selector that matches nothing, but a
		// driver answering 404 for the same state would make an empty list read
		// as a failure -- and an empty list is the reading a non-admin's page
		// legitimately produces.
		if strings.Contains(err.Error(), "404") {
			return 0, nil
		}
		return 0, fmt.Errorf("counting the children of #%s: %w", ref, err)
	}
	return len(out.Value), nil
}
