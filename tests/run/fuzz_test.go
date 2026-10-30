package run_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/fuzz"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// THE FUZZ STAGE AGAINST THE LIVE TOY, which is D2's subject and D5's "a live
// server". TestMain builds `toy` and `toy-adapter` onto PATH here, and
// toy-adapter spawns `toy` itself -- so this is the real pairing and not a
// fake surface.
//
// WHY THE ARM LIVES IN tests/run AND NOT internal/fuzz: internal/fuzz's own
// arms are over the DERIVATION, which needs no server, and an arm that spawned
// one from a unit package would make every `go test ./internal/...` depend on
// two binaries being built first. This suite already builds them.
func TestTheFuzzStageDrivesTheToyAndFindsNo5xx(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join("..", "..", "examples", "toy")

	prep, err := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if err != nil || !prep.OK() {
		t.Fatalf("platform: %v %v", err, prep.Refusals)
	}
	brep, err := bindings.Load(filepath.Join(dir, "bindings.toml"), decl.TOML)
	if err != nil || !brep.OK() {
		t.Fatalf("bindings: %v %v", err, brep.Refusals)
	}
	actors, err := schedule.Actors(prep.Platform.Venire)
	if err != nil {
		t.Fatalf("Actors: %v", err)
	}

	sess, err := wire.Open(ctx, wire.Spawn{
		Command: prep.Platform.Adapter.Command,
		Cwd:     dir,
		Env:     prep.Platform.Adapter.Env,
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rep, err := fuzz.Run(ctx, brep.Bindings, sess, actors[0])
	if err != nil {
		t.Fatalf("fuzz.Run: %v", err)
	}

	// THE SETUP RAN THE DERIVED ORDER, not a hand-written one.
	if len(rep.Setup) == 0 {
		t.Fatal("the stage ran no setup, so every handle case sent an id it never obtained")
	}

	// D2: NO 5xx over the whole corpus. This is the clause, and the toy
	// passing it is the finding's absence rather than the arm's vacuity --
	// which the next assertion is for.
	if len(rep.Findings) != 0 {
		for _, f := range rep.Findings {
			t.Errorf("finding: %s", f)
		}
	}

	// AND THE CORPUS IS ACTUALLY HOSTILE, which is the half that keeps the
	// clause from being satisfied by a stage nothing refused: a corpus every
	// case of which the system ACCEPTED would report no 5xx and have tested
	// nothing about refusals.
	refused, accepted, faulted := 0, 0, 0
	for _, r := range rep.Results {
		switch {
		case r.Fault != "":
			faulted++
		case r.Status >= 400 && r.Status < 500:
			refused++
		case r.Status >= 200 && r.Status < 300:
			accepted++
		}
	}
	if refused == 0 {
		t.Error("not one case was refused with a 4xx: the corpus is not hostile to this system")
	}
	// EVERY ANSWER IS COUNTED IN EXACTLY ONE BAND, so a status outside them --
	// a 3xx, or a zero with no fault -- cannot sit in the report unexamined
	// while the three counts still look plausible.
	if refused+accepted+faulted != len(rep.Results) {
		t.Errorf("%d result(s) but %d refused + %d accepted + %d faulted: an answer went uncounted",
			len(rep.Results), refused, accepted, faulted)
	}
	t.Logf("PROOF setup %v; %d case(s): %d refused, %d accepted, %d faulted; statuses %s",
		rep.Setup, len(rep.Results), refused, accepted, faulted, rep.Statuses())

	// EVERY REFUSAL NAMES SOMETHING, which is D4's subject and is measured
	// here rather than asserted: a 4xx with an empty body says the system
	// refused and nothing about what it objected to.
	silent := 0
	for _, r := range rep.Results {
		if r.Status >= 400 && r.Status < 500 && strings.TrimSpace(r.Error) == "" {
			silent++
		}
	}
	t.Logf("PROOF %d of %d refusals carry no error text", silent, refused)
}
