package schedule

import (
	"context"
	"fmt"
	"strings"

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

	// Tell is the HONEST kind, and it is not one of the four above: a tell
	// passes on a claim the run did not plant. fable ruled it into R5.4 at coop
	// r5/114 as canard's mirror, weighted in the honest band because it is
	// honest. It is listed in PoolKinds and deliberately NOT in NemesisKinds.
	Tell = "tell"

	// Advance moves the RUN'S OWN CLOCK, and it is R7.3's kind. It is not a
	// lie and not an action of the project's: nobody in the system under test
	// does it, which is why it is drawn from the pool like `tell` and listed
	// outside NemesisKinds.
	//
	// IT IS WEIGHTED LIKE EVERY OTHER KIND because the walk's composition is a
	// function of platform.toml and nothing else. A project that never wants
	// its clock moved declares zero, exactly as examples/toy does for
	// `essoin` -- and absent stays a refusal, since absent means nobody
	// decided.
	Advance = "advance"
)

// NemesisKinds in a fixed order. The order does not reach the draw -- Pick
// sorts by kind for D1's reason -- but a fixed slice keeps the REFUSALS below
// reproducible, since the first undeclared weight is the one reported.
var NemesisKinds = []string{Suborn, Canard, Palter, Essoin}

// PoolKinds is every kind the ENGINE puts in the pool beside a project's own
// actions: the four nemesis kinds, the honest tell and R7.3's `advance`.
//
// IT EXISTS BECAUSE "NEMESIS KIND" STOPPED BEING THE SAME SET AS "ENGINE KIND"
// when R5.4 added the honest tell. Every reader asking "does this name belong
// to the engine rather than to the project" wants THIS list -- a weight must
// exist for each, and `rumor validate` pairs them -- while readers asking "is
// this a lie" still want NemesisKinds, which stays the four docs/design.md
// names in one breath and which tests/validate pins to that sentence.
var PoolKinds = append(append([]string{}, NemesisKinds...), Tell, Advance)

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
	actor string, w platform.Weights, stage, profile string, t *handle.Table,
	roles []bindings.Role, known, actors []string, landings []int64) ([]Candidate, error) {

	reserved := map[string]bool{}
	for _, k := range PoolKinds {
		reserved[k] = true
	}
	for _, a := range actions {
		if reserved[a.Name] {
			return nil, fmt.Errorf("schedule: the bindings declare an action named %q, which is a "+
				"nemesis kind: the pool is keyed by kind, so both would draw as one kind carrying "+
				"two weights and the draw could not say which was chosen", a.Name)
		}
	}

	// THE ROLE GATE RUNS FIRST, AND OVER THE PROJECT'S ACTIONS RATHER THAN THE
	// APPLICABLE ONES: AN ACTOR WHOSE ROLE DOES NOT HOLD THE ACTION'S CONTEXT
	// IS INAPPLICABLE, NOT ATTEMPTED (issue 78). The engine would otherwise
	// acquire a session in a context the project says this role never has, and
	// the adapter would answer about an actor it should never have been asked
	// about -- or worse, answer successfully, which would make the project's
	// role model unfalsifiable from a walk.
	//
	// IT WAS INSIDE THE RESOLVABILITY LOOP BELOW AND THAT WAS A DEFECT, found
	// by ibis at coop r5/93 and placed here by fable at r5/95. A role the
	// declaration omits is a REFUSAL rather than an empty pool, and the refusal
	// lives in HoldsContext -- but reached only for an action the statute had
	// already called applicable, it could not fire for an actor nothing is
	// applicable to. R5.4's own `has_user` makes that actor ordinary: a minted
	// actor with no user has no `api` action applicable, so a missing
	// `[[role]]` and a world permitting nothing were the same answer again,
	// which is the fault the refusal exists to prevent. The arm that should
	// have caught it had ARRANGED a user to reach the gate, and said so in a
	// comment: a harness that sets up the premise cannot test it.
	//
	// OVER `actions` AND NOT OVER `app` for the same reason: an undeclared role
	// is a fault in the PROJECT, not a condition of one step, so it must not
	// depend on what the statute says this actor may do right now. The
	// narrowing that remains is the honest one -- preconditions are no longer
	// evaluated for actions this role could never take.
	gated := make([]bindings.Action, 0, len(actions))
	for _, a := range actions {
		held, err := HoldsContext(roles, a.Name, actor, a.Context)
		if err != nil {
			return nil, err
		}
		if !held {
			continue
		}
		gated = append(gated, a)
	}

	app, err := Applicable(ctx, statute, gated, l, actor)
	if err != nil {
		return nil, err
	}

	out := make([]Candidate, 0, len(app)+len(PoolKinds))
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
	//
	// AND THE HANDLE MUST BE ONE THIS ACTOR KNOWS OF, which is R5.4's close of
	// the narrowing Resolvable's own comment had declared and left open (fable,
	// coop r5/114).
	//
	// `known` IS THE CALLER'S AND NOT READ HERE, so the pool and the step that
	// follows it quantify over ONE set. Reading it twice -- once to decide the
	// pool and once to compose the draw -- would be two answers to one
	// question, and the pool offering an action whose draw then refuses is
	// exactly the defect being closed.
	usable := make([]bindings.Action, 0, len(app))
	for _, a := range app {
		if ok, kind := Resolvable(a, t, known); !ok {
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
	for _, k := range PoolKinds {
		live, err := engineKindApplicable(k, app, l, actor, actors, landings)
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
func engineKindApplicable(kind string, app []bindings.Action, l *ledger.Ledger, actor string,
	actors []string, landings []int64) (bool, error) {

	// BOTH TELLING KINDS CARRY THE RECEIVER HALF, which neither did at first.
	// A subject nobody still needs is not a tell anybody can make, and leaving
	// that to the step aborted three walks in eighty. Tellable is the same
	// function the step draws through, so the gate and the draw cannot answer
	// differently.
	switch kind {
	case Tell:
		subjects, err := engine.TellSubjects(l, actor)
		if err != nil {
			return false, fmt.Errorf("schedule: tell's precondition for %s: %w", actor, err)
		}
		live, err := Tellable(l, actors, actor, subjects)
		if err != nil {
			return false, fmt.Errorf("schedule: tell's receiver half for %s: %w", actor, err)
		}
		return len(live) > 0, nil
	case Canard:
		subjects, err := engine.CanardSubjects(l, actor)
		if err != nil {
			return false, fmt.Errorf("schedule: canard's precondition for %s: %w", actor, err)
		}
		live, err := Tellable(l, actors, actor, subjects)
		if err != nil {
			return false, fmt.Errorf("schedule: canard's receiver half for %s: %w", actor, err)
		}
		return len(live) > 0, nil
	}
	switch kind {
	case Suborn:
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

	case Palter:
		// PALTER NEEDS SUBORN'S PRECONDITION AND ONE CLAUSE MORE, which is
		// design.md's own: the field it falsifies "is one a statute READS --
		// `visible_to` where the action's claim carries it, else `expect` --
		// and a kind whose action's claim carries neither is INAPPLICABLE for
		// that action". So an emitting action is not enough; the action must
		// carry a falsifiable field.
		//
		// WITHOUT THIS CLAUSE THE POOL OFFERS WHAT THE WALK CANNOT DO. Measured
		// 2026-10-21 against the toy: the pool held palter applicable for every
		// emitting action, the walk drew it at step 3, and the step died --
		// which is the drawn-and-then-refused shape Resolvable exists to
		// prevent, arriving by the other door (ibis, coop r5/88).
		for _, a := range app {
			if len(a.Emits) > 0 && palterable(a) {
				return true, nil
			}
		}
		return false, nil

	case Essoin:
		return false, nil

	case Advance:
		// LIVE IFF THERE IS SOMETHING TO LAND ON, which is fable's ruling 3 at
		// coop r7/42: "no live target means INAPPLICABLE, not a no-op --
		// advance is live iff the project declares `[clock]` AND a target
		// exists, so it is never drawn into nothing and D1 holds without a
		// wasted step".
		//
		// BOTH HALVES ARE IN `landings` AND NEITHER IS READ HERE, which is the
		// Tellable rule one function up: the pool's gate and the step's draw
		// must quantify over ONE set, or the pool offers what the walk cannot
		// do. The caller builds the list from the register -- an unclocked run
		// holds no pending whose bound is an instant, so its list is empty by
		// construction rather than by a second check of the `[clock]` table.
		//
		// AND IT IS LANDINGS RATHER THAN BOUNDS for the reason `Landings`
		// states: a bound one tick ahead offers landings the mover accepts and
		// one it refuses, so a gate over bounds would say live where the draw
		// cannot act.
		return len(landings) > 0, nil
	}
	return false, fmt.Errorf("schedule: %q is not a nemesis kind, and this function is the only "+
		"place that decides applicability for one", kind)
}

// RoleOf is the role an actor's name was built from: the `member` of
// `member_2`.
//
// IT STRIPS THE TRAILING `_<n>` AND NOT EVERYTHING AFTER THE FIRST UNDERSCORE,
// because Actors() builds `<role>_<n>` from a role that may itself contain one
// -- `super_admin_1` is role `super_admin`, not role `super`. The ordinal is
// digits by construction there, so an actor name that does not end in one is
// not a name this run minted and says so.
func RoleOf(actor string) (string, bool) {
	i := strings.LastIndexByte(actor, '_')
	if i <= 0 || i == len(actor)-1 {
		return "", false
	}
	for _, c := range actor[i+1:] {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return actor[:i], true
}

// HoldsContext reports whether this actor's role holds the context an action
// needs, and refuses where the question cannot be answered.
//
// CONTEXT "none" IS HELD BY EVERYBODY, because it is not a context: the
// contract calls it "an action taken outside a session" (docs/contract.md:89),
// so there is no session for a role to be entitled to. Gating it on a role's
// list would make an action nobody needs a session for unreachable to an actor
// whose role declares no contexts at all.
//
// THE NARROWING, AND EXACTLY WHAT IT COVERS: a project that declares NO roles
// gets no gate here. That is not "absent means permitted" applied to one role
// -- a declaration that exists and omits this actor's role IS refused below,
// for the reason an undeclared weight is refused one function up. It is that a
// project modelling no roles at all cannot have this question asked of it, and
// the pairing that catches a context no role holds is static: it needs the
// bindings alone, so it belongs in `rumor validate` beside the arms that
// already pair declarations against each other, and the arm count is fable's.
// Said here so a reader does not take the silence for a check.
func HoldsContext(roles []bindings.Role, action, actor, ctxName string) (bool, error) {
	if ctxName == contextNone {
		return true, nil
	}
	if len(roles) == 0 {
		return true, nil
	}
	role, ok := RoleOf(actor)
	if !ok {
		return false, fmt.Errorf("schedule: %q is not an actor name this run could have minted "+
			"(<role>_<n>), so the role whose contexts gate %q cannot be named", actor, action)
	}
	for _, r := range roles {
		if r.Name != role {
			continue
		}
		for _, c := range r.Contexts {
			if c == ctxName {
				return true, nil
			}
		}
		return false, nil
	}
	// A DECLARATION THAT EXISTS AND OMITS THIS ROLE IS REFUSED, not read as
	// holding nothing: reading it as nothing would make every context-bearing
	// action inapplicable to this actor, and a walk that stalled for a missing
	// [[role]] would look exactly like a walk whose world permits nothing.
	return false, fmt.Errorf("schedule: the bindings declare %d role(s) and none of them is %q, "+
		"which is actor %s's: an absent role is not a role holding no context, and %q needs "+
		"context %q", len(roles), role, actor, action, ctxName)
}
