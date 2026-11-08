package schedule

import (
	"context"
	"fmt"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// contextNone is the context an action taken outside any session declares.
// docs/contract.md:89 ("a context name or `\"none\"` for an action taken
// outside a session") and the token row at :110 both spell it this way, as do
// cmd/toy-adapter and internal/fake, so this is the literal the tree already
// uses rather than a second spelling of it.
const contextNone = "none"

// sessionKey is an actor in a context. A STRUCT AND NOT A JOINED STRING so
// there is no separator to argue about: an actor is `[a-z][a-z0-9_]*` and a
// context is whatever the project declared, and a join would need a character
// neither can contain.
type sessionKey struct {
	actor   string
	context string
}

// Sessions holds one token per actor per context, ACQUIRED AT FIRST USE.
//
// WHY FIRST USE AND NOT UP FRONT, which is the whole design and not an
// optimization: a run with four actors and three contexts would acquire twelve
// sessions before step one, most of them never used, and the adapter's session
// count would then be a function of the PROJECT rather than of the walk. The
// stronger reason is ordering -- cmd/toy keys its sessions by user NAME (ibis,
// coop r5/70), so a session for member_1 can only exist once some create_user
// has made a user called member_1. Acquiring up front would ask for every
// session before the walk had created a single user, and every one would fail.
// First use is the only order the system under test allows.
//
// AND ONE PER ACTOR PER CONTEXT, not one per actor: an actor acting in two
// contexts holds two sessions, because the context is what the token is for.
// Reusing one actor's api token for its browser actions would send a token the
// adapter issued for something else, and an adapter that happened to accept it
// would make the contexts indistinguishable from the engine's side.
type Sessions struct {
	sess *wire.Session
	held map[sessionKey]string

	// browser is the live browser a browser-context flow is driven through,
	// and base is the system's address that a `navigate` step resolves against.
	//
	// IT LIVES HERE BECAUSE A BROWSER SESSION IS PER ACTOR PER CONTEXT, which
	// is what this type is already for: the paragraphs above reason about an
	// actor's browser context holding a token of its own, and deliverable 1's
	// driver keys its sessions by actor for the same reason. Putting it on
	// Project was refused -- that struct is "what a walk needs from a project's
	// DOCUMENTS", and a running browser is not read from a file.
	//
	// THE STAGE OWNS THE LIFECYCLE AND THE WALK ONLY DRIVES (fable, coop
	// r9/110): the browser stage starts the pair and stops it, and hands the
	// walk what it needs to drive a flow. Nil here is the ordinary case -- a
	// project with no browser flows, or a box with no drivable pair -- and the
	// walk falls back to the adapter, which is what every run did before this.
	browser FlowDriver
	base    string

	// acquired counts the `session` exchanges actually sent, which is how a
	// test can tell "held and reused" from "asked for again": the token
	// returned is identical either way, so a cache that never cached would be
	// invisible without a count.
	acquired int
}

// FlowDriver drives one action's declared UI steps through a real browser and
// answers in the wire's own shape, so the engine's resume reads the same whether
// the adapter or a browser drove.
//
// IT IS AN INTERFACE HERE SO THE ENGINE DOES NOT IMPORT A DRIVER. internal/browser
// implements it; schedule names only what it needs, which keeps the walk
// testable with a stand-in and keeps a browser out of every build that has no
// use for one.
//
// THE DETAIL IS SEPARATE because wire.FlowResponse has no field for it: a
// condition that was not met names itself and the last text observed, and that
// sentence has nowhere to go on the wire (fable, coop r9/110).
type FlowDriver interface {
	Flow(ctx context.Context, actor, base, token string, req wire.FlowRequest) (
		resp wire.FlowResponse, detail string, err error)
}

// Browser is the live browser a walk drives flows through, as the stage that
// started it hands it over (fable, coop r9/110). The zero value drives nothing,
// which is every project without browser flows and every box without a
// drivable pair -- and in that case the adapter answers as it always has.
//
// IT CARRIES NO ADDRESS, AND THAT IS DELIBERATE. The system's base URL comes
// from the ADAPTER'S OWN DESCRIBE (fable, coop r9/111), and `Walk` reads it off
// the session it was handed -- so there is no field here for a caller to put a
// guessed host in, and no second place the address could come from. An adapter
// that reports none is a named refusal at the walk, not a default.
type Browser struct {
	Driver FlowDriver
}

// NewSessions holds tokens against one adapter session.
func NewSessions(sess *wire.Session) *Sessions {
	return &Sessions{sess: sess, held: map[sessionKey]string{}}
}

// UseBrowser hands this walk the browser the stage started, and the address its
// `navigate` steps resolve against. Called once, before the walk begins.
func (s *Sessions) UseBrowser(d FlowDriver, base string) {
	s.browser, s.base = d, base
}

// Browser is the live browser and its base address, or a nil driver when the
// flows are to go through the adapter as they always have.
func (s *Sessions) Browser() (FlowDriver, string) { return s.browser, s.base }

// Acquired is how many `session` requests have been sent.
func (s *Sessions) Acquired() int { return s.acquired }

// Token is the token for this actor in this context, acquiring one if none is
// held yet.
//
// THE EMPTY TOKEN FOR CONTEXT "none" IS THE CONTRACT'S OWN RULE and not a
// sentinel of mine: `token` is "present exactly when the action's context is
// not `\"none\"`" (docs/contract.md:110), ExecuteRequest.Token is
// `json:"token,omitempty"`, so returning "" makes the field absent. This is
// also the engine-side half of a check internal/wire deliberately declines:
// semantics() records that it does not check "that an execute's token matches
// its action's context, which needs the action's declared context and is
// therefore the engine's check and not the decoder's". Acquiring the token FOR
// a context is how the engine holds that half -- there is no second place where
// a token and a context could be paired up wrongly.
func (s *Sessions) Token(actor, ctxName string) (string, error) {
	if ctxName == contextNone {
		// NOTHING IS ACQUIRED AND NOTHING IS CACHED. An action outside a
		// session must not cause a session to exist: the count is what a
		// verdict reports, and a run whose argument-less actions quietly
		// acquired sessions would report a number nobody asked for.
		return "", nil
	}
	if ctxName == "" {
		return "", fmt.Errorf("schedule: %s was asked to act in the empty context: an action "+
			"taken outside a session declares %q, and an empty context is a declaration that was "+
			"never read", actor, contextNone)
	}
	if actor == "" {
		return "", fmt.Errorf("schedule: a session in context %q was asked for with no actor, and "+
			"the actor is who the token is FOR -- the wire carries no actor field on anything but "+
			"this request", ctxName)
	}
	key := sessionKey{actor: actor, context: ctxName}
	if tok, ok := s.held[key]; ok {
		return tok, nil
	}
	if s.sess == nil {
		return "", fmt.Errorf("schedule: %s needs a session in context %q and there is no adapter "+
			"session to ask for one", actor, ctxName)
	}
	resp, err := s.sess.StartSession(wire.SessionRequest{Actor: actor, Context: ctxName})
	s.acquired++
	if err != nil {
		return "", fmt.Errorf("schedule: acquiring %s's session in context %q: %w", actor, ctxName, err)
	}
	// AN EMPTY TOKEN IS REFUSED RATHER THAN HELD. It would be sent as an absent
	// field by omitempty, so an action whose context is NOT "none" would go out
	// looking exactly like one whose context is -- the adapter would answer 401
	// and the walk would report a could-not-run about the wrong thing.
	if resp.Token == "" {
		return "", fmt.Errorf("schedule: the adapter answered %s's session in context %q with an "+
			"empty token: an empty token is omitted from the request, so the execute would be "+
			"indistinguishable from one taken outside a session", actor, ctxName)
	}
	s.held[key] = resp.Token
	return resp.Token, nil
}
