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
// THE FOUR HAND COUNTS WERE ALL WRONG, AND NOT BY A LITTLE. Measured on the
// 5-package / 47-case / 31-rule / 42-mutant corpus, one attaint pass compiles
// 17,462 times before the hoist and 816 after. Against 17,462 the eye-read
// figures are off by 3.59x, 3.70x, 7.51x and 7.64x -- and they fall in two
// clusters, so two readers agreeing with each other was not evidence either.
// The integer was never the finding; the method was.
//
// THE PER-COMPILE FIGURE NOW EXISTS, and it took matched provenance to get.
// Earlier attempts divided a wall clock by a count nobody had measured beside
// it, and all three authors withdrew them. The honest form is a DIFFERENCE
// with both terms from the same population: 200.43 CPU-seconds saved over
// 16,646 compiles removed = 12.04 ms of CPU per compile, taken in the ci jail
// in both orders (12.04 and 12.05, agreeing to 0.08%), with the count read by
// two actors on two boxes. It is nothing like the 1.7-4 ms the room had been
// working from -- that bracket was wrong by three to seven times and would
// have read as corroboration.
//
// WHAT THE HOIST BOUGHT, and what the arm costs. The hoist is 8.65x CPU on
// tests/attaint in the ci jail, both orders, 1542.3 CPU-seconds freed per
// package run. The pass-scale arm is NOT free: it clears both caches and runs
// the pass TWICE, which took tests/attaint on matrix from 36.146s to 51.369s,
// +42%, and costs roughly +45s in the jail against the 1138s the hoist freed
// there (quill, coop r9/392). Recorded here so it is known rather than
// discovered.
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
