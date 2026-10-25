// Package vocabulary holds the version of the vocabulary this module
// implements.
//
// WHY IT EXISTS AND WHY NOW. docs/vocabulary.md's preamble promised this
// constant to R4's contract and said that, until it existed, the Revisions
// section at the end of that file was the version. R4.1 is that contract, so
// the constant lands here and the preamble now states the pairing instead; the
// v1 Revisions row records the change.
//
// THE PROMISE IS PARAPHRASED AND NOT QUOTED, deliberately: the sentence it was
// made in no longer exists, and tests/vocabulary refuses a quote the document
// no longer carries -- which is how this comment's first draft was caught.
//
// WHY THE NAME REPEATS THE PACKAGE, against Go's usual advice about stutter:
// the release gate's revision arm matches the IDENTIFIER, so a Go-idiomatic
// `vocabulary.Version` would leave that arm as dead as `.rs` did -- which is
// the defect this phase measured (quill coop r4/5: the arm wanted
// `CONTRACT_VERSION` in a path ending `.rs`, and the tree holds no such file in
// any spelling). A guard that cannot see the constant it guards is worse than
// no guard, because its self-test is green.
//
// AND THE CONSTANT IS ITSELF CHECKED: tests/vocabulary requires it to equal the
// LAST row of that document's Revisions table, so this is not a token for a
// gate to match but a claim the document convicts.
package vocabulary

// VocabularyVersion is the vocabulary docs/vocabulary.md declares, as that
// document spells it. A phase that changes a term there bumps this with it and
// adds a Revisions entry; the release gate refuses the bump that arrives alone.
const VocabularyVersion = "v2"
