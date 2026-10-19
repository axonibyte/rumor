package schedule

import (
	"context"
	"fmt"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/handle"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// The nemesis kinds, as docs/design.md names them: action kinds that draw from
// THE SAME POOL as the project's own actions, with their own weights. `suborn`
// feeds an actor a false claim, `canard` is a tell that passes on a planted
// claim, `palter` is a half-true claim, `essoin` is an abandoned flow.
//
// WHY THEY ARE CONSTANTS HERE AND NOT IN internal/vocabulary: that package
// holds the version constant R4's contract pairs against, and its own comment
// says so. These four are the SCHEDULER's pool kinds -- docs/vocabulary.md
// defines the terms and says of canard that it is "a weight and a precondition
// over the kinds above, not a fact kind and not a tag", so none of them is a
// fact kind the ledger carries. If fable would rather the names live beside
// VocabularyVersion, moving them is a rename and nothing more.
const (
	Suborn = "suborn"
	Canard = "canard"
	Palter = "palter"
	Essoin = "essoin"
)

// NemesisKinds in a fixed order. The order does not reach the draw -- Pick
// sorts by kind for D1's reason -- but a fixed slice keeps the REFUSALS below
// reproducible, since the first undeclared weight is the one reported.
var NemesisKinds = []string{Suborn, Canard, Palter, Essoin}

// Pool is the candidate set for one step of the walk: every applicable action
// and every applicable nemesis kind, each carrying the weight platform.toml
// resolves for it at this stage and profile. A lie is therefore scheduled
// exactly the way an honest action is, which is the design's own sentence and
// the reason the vocabulary's control needs no trust model: the engine knows
// which claims it planted because it planted them, and the statute does not.
//
// AN UNDECLARED WEIGHT IS REFUSED AND NOT TREATED AS ZERO. The two are
// different statements and the difference is the day's lesson one layer over
// (rook coop r5/22): zero means "never chosen", which is a decision the project
// made and `Pick` honors, while absent means nobody decided. Inventing a weight
// would put a kind in the draw the project never weighted, and defaulting to
// zero would silently remove one it may have meant to tune. Either way the
// walk's composition would stop being a function of platform.toml.
//
// A PROJECT ACTION NAMED LIKE A NEMESIS KIND IS REFUSED, because the pool is
// keyed by kind: two candidates spelled `suborn` would be drawn as one kind
// with two weights, and `Pick` would answer a name that no longer says which
// of the two was chosen. Refusing here is not the static home for it -- a
// validate arm pairing bindings against the vocabulary would be, and the arm
// count is fable's -- but a silent double-weight is worse than an early error.
func Pool(ctx context.Context, statute string, actions []bindings.Action, l *ledger.Ledger,
	actor string, w platform.Weights, stage, profile string, t *handle.Table) ([]Candidate, error) {

	reserved := map[string]bool{}
	for _, k := range NemesisKinds {
		reserved[k] = true
	}
	for _, a := range actions {
		if reserved[a.Name] {
			return nil, fmt.Errorf("schedule: the bindings declare an action named %q, which is a "+
				"nemesis kind: the pool is keyed by kind, so both would draw as one kind carrying "+
				"two weights and the draw could not say which was chosen", a.Name)
		}
	}

	app, err := Applicable(ctx, statute, actions, l, actor)
	if err != nil {
		return nil, err
	}

	out := make([]Candidate, 0, len(app)+len(NemesisKinds))
	add := func(kind string) error {
		v, ok := w.Weight(stage, profile, kind)
		if !ok {
			return fmt.Errorf("schedule: platform.toml declares no weight for %q (stage %q, profile %q): "+
				"an undeclared weight is not a zero -- zero means never chosen and the project decided it, "+
				"absent means nobody did -- so the walk's composition would stop being a function of the "+
				"weight file", kind, stage, profile)
		}
		out = append(out, Candidate{Kind: kind, Weight: v})
		return nil
	}

	// RESOLVABILITY IS THE SECOND HALF OF APPLICABILITY (coop r5/58). A
	// statute precondition says the world permits the action; a resolvable
	// handle says the walk can actually name what the action acts on. An
	// action whose flow needs a `{kind}` no bound handle satisfies is
	// inapplicable, not drawn-and-then-refused -- a suborn mints a handle for
	// an action that never ran, and the walk must never ask the wire for a
	// thing that does not exist.
	usable := make([]bindings.Action, 0, len(app))
	for _, a := range app {
		if ok, kind := Resolvable(a, t); !ok {
			_ = kind
			continue
		}
		usable = append(usable, a)
	}
	app = usable

	for _, a := range app {
		if err := add(a.Name); err != nil {
			return nil, err
		}
	}
	for _, k := range NemesisKinds {
		live, err := nemesisApplicable(k, app, l, actor)
		if err != nil {
			return nil, err
		}
		if !live {
			continue
		}
		if err := add(k); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// nemesisApplicable is each nemesis kind's precondition, as fable ruled them
// at coop r5/49 after I built the first three unconditioned and ASKed.
//
// SUBORN AND PALTER ARE AN HONEST ACTION TAKEN DISHONESTLY, so their
// precondition is THE HONEST ACTION'S OWN. The vocabulary's planted claim is "a
// lie an actor tells about its own action", which means the lie needs an action
// the actor could actually take and that emits something to lie about -- the
// engine plants the claim that action WOULD have emitted, false in whole for
// suborn and false in a recorded part for palter (breakout-r5/1). An
// unconditioned suborn would lie about an action the actor could not take,
// which is a lie of a kind the vocabulary does not have.
//
// MY OWN REASONING FOR LEAVING THEM OPEN WAS WRONG IN ITS DIRECTION, and the
// correction is worth keeping: I argued that over-offering was the safe side,
// because the class rook found at r5/25 was a permitted action VANISHING. fable:
// "over-offering is not the safe direction here ... an impossible lie offered is
// the opposite error." A silent drop and an impossible offer are two faults, not
// one fault and its safe side.
//
// ESSOIN IS INAPPLICABLE IN R5, not a gap: abandoning mid-flow needs flow state
// the engine gains at R9. It keeps a declared weight in platform.toml -- zero in
// the toy -- so the weight table stays complete and arm 19's new cell can pair
// it, and it is simply never drawn until R9 gives it something to abandon.
func nemesisApplicable(kind string, app []bindings.Action, l *ledger.Ledger, actor string) (bool, error) {
	switch kind {
	case Canard:
		subjects, err := engine.CanardSubjects(l, actor)
		if err != nil {
			return false, fmt.Errorf("schedule: canard's precondition for %s: %w", actor, err)
		}
		return len(subjects) > 0, nil

	case Suborn, Palter:
		// AN APPLICABLE ACTION THAT EMITS SOMETHING. `app` is already the
		// actions whose preconditions hold for this actor, so the honest half
		// of the precondition is inherited rather than recomputed; the `emits`
		// half is what gives the planted claim a type to be.
		for _, a := range app {
			if len(a.Emits) > 0 {
				return true, nil
			}
		}
		return false, nil

	case Essoin:
		return false, nil
	}
	return false, fmt.Errorf("schedule: %q is not a nemesis kind, and this function is the only "+
		"place that decides applicability for one", kind)
}
