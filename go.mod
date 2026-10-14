module forge.axonibyte.com/axonibyte/rumor

// 1.25 rather than 1.26, deliberately: this jail carries both go1.25.14 and
// go1.26.8, a newer toolchain builds an older module and not the reverse, and
// forge-01_jail's Go version is not yet measured. Raise this only when the
// runner's version is known and something here needs the newer language.
go 1.25
