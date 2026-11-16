package engine

import "sync/atomic"

// THE COMPILE COUNTER, and it exists because the figure R10 d1 rests on cannot
// be had by reading the tree. Four hand counts of how many times a statute is
// compiled in one attaint pass disagreed -- 62,651, 64,672, 131,130 and
// 133,339 -- and no two of them were wrong in the same way. A number four
// readers cannot agree on is not a measurement, so the code says it instead.
//
// WHAT IT COUNTS IS THE COMPILE, NOT A PROXY FOR ONE. Every increment sits
// immediately above a `rego.New(...).Eval(ctx)`, which is the OPA one-shot
// call that parses and compiles the module before it evaluates. Counting
// evaluations, rules or cases instead would each be a different number that
// happens to be close, and "close" is how the four hand counts happened.
//
// THE PER-COMPILE MILLISECOND FIGURE IS NOT EVIDENCE. Both authors who quoted
// one withdrew it; it divided a wall-clock total by a count neither of them
// had measured, so it carried the count's error into a rate and hid it there.
// This counter gives the count. The duration is a separate measurement and has
// to be taken as one.
//
// THE INVARIANT THAT KEEPS IT HONEST THROUGH THE HOIST: the call moves where
// the compile moves. When PrepareForEval replaces the one-shot call, the
// increment goes with it to the prepare, not to the eval -- otherwise the
// counter would report the post-hoist tree still compiling per evaluation and
// the hoist would look like it had done nothing. An increment left behind is
// the one way this file can lie.
var compiles atomic.Uint64

// RecordCompile notes one module compile. Called immediately before the
// one-shot rego call it counts.
func RecordCompile() { compiles.Add(1) }

// Compiles reads the count without disturbing it.
func Compiles() uint64 { return compiles.Load() }

// ResetCompiles zeroes the count and returns what it was, so a measurement can
// bound itself to one pass rather than to the life of the process.
func ResetCompiles() uint64 { return compiles.Swap(0) }
