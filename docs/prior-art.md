# Prior art and naming

Measured 2026-10-09 from public endpoints, anonymously and with no
credential. Every figure is reproducible by the command named beside it.
Nothing internal to the project's infrastructure appears here.

## 1. Registries

| Registry | `rumor` | Detail |
|---|---|---|
| crates.io | **TAKEN** | 0.1.0, published 2023-11-20, never updated. Description is literally `reserved; library for rumor file synchronization protocol`. 1,493 downloads all-time, 1 recent. |
| crates.io `rumor-client` | **TAKEN** | 0.1.0, same day, same owner |
| crates.io `rumor-server` | **TAKEN** | 0.1.0, same day, same owner |
| npm | TAKEN | `rumor` — "A logger", 10 versions, last modified 2022-06-26, maintainer `kparkov` |
| RubyGems | TAKEN | `rumor` 0.1.0, 4,171 downloads, author Mattias Putman |
| PyPI | **free** | 404 |
| Hex (Erlang/Elixir) | **free** | 404 |

Commands:

    curl -A '<ua>' https://crates.io/api/v1/crates/rumor
    curl -A '<ua>' https://crates.io/api/v1/crates/rumor/owners
    curl -A '<ua>' 'https://crates.io/api/v1/crates?q=rumor&per_page=20'
    curl https://pypi.org/pypi/rumor/json
    curl https://registry.npmjs.org/rumor
    curl https://rubygems.org/api/v1/gems/rumor.json
    curl https://hex.pm/api/packages/rumor

Not checked, and why: Maven Central, NuGet, Go modules, Packagist. The crate
name is what R0 must settle; those ecosystems cannot block a crates.io
publish, and a trademark question is cal's, not a registry lookup.

## 2. The crates.io holder — one person, all three names

All of `rumor`, `rumor-client` and `rumor-server` are held by the same
crates.io account:

    login  micycle8778
    name   Michael Campbell
    url    https://github.com/micycle8778

So the transfer ask is one ask to one person and it can cover the whole
`rumor*` prefix rather than the bare name alone.

State of the holder's project, measured:

    crate's declared repository   https://github.com/RainbowAsteroids/rumor
    that URL                      301 -> micycle8778/rumor (owner renamed)
    stars 0 | last push 2023-11-25 | archived false | licence MIT
    description "A file synchronization protocol and application"

Reading: a dormant two-year-old reservation with no users (1 recent
download), published under a description that calls itself reserved, in a
repo with no stars and no pushes since five days after publication. That is
the strongest case a transfer ask can have, and it is still a request —
the names are the holder's.

## 2a. The fallback is already decided, and it is free

The epic's **Decided** section settles this and I missed it on the first
pass: "Binary is `rumor`. The crate is `rumor-cli` unless the `rumor` crate
name is transferred." So no new decision is owed if the holder declines.

Measured 2026-10-09 ~21:50Z, same endpoint and UA as above:

    crates.io/api/v1/crates/rumor-cli    404  free
    crates.io/api/v1/crates/rumor-core   404  free
    crates.io/api/v1/crates/rumors       404  free
    crates.io/api/v1/crates/rumour       404  free
    pypi.org/pypi/rumor-cli/json         404  free

The consequence for R0: "the crate name is settled" is satisfiable on day
one with `rumor-cli`, and the transfer ask is an improvement rather than a
blocker. Nothing in R0 waits on the holder's answer.

## 3. GitHub — the semantic field is misinformation, not testing

`q=rumor in:name` returns **1,677** repositories. The top of the field by
stars is entirely rumour/misinformation detection and fact-checking:

    thunlp/Chinese_Rumor_Dataset     788   2020-06-26
    majingCUHK/Rumor_RvNN            234   2021-04-02  rumour detection on Twitter
    chunyuanY/RumorDetection         154   2021-02-21
    CarolHsu/rumor-checker           133   2024-05-30
    snikolov/rumor                   119   2013-09-10  Twitter timeseries classification
    cofacts/rumors-api               118   2026-10-07  active fact-checking suite
    exodrifter/unity-rumor           110   2023-10-09  narrative scripting language

Two things follow. First, no prior art names a *testing* tool `rumor`, so the
field is clear for the meaning this epic wants. Second, every search result a
new user meets for "rumor" plus software is misinformation detection, and the
one dev-tool neighbour (`unity-rumor`) is a scripting language — so
discoverability for this project rests on the crate name and the tagline,
never on the word alone.

    curl -H 'Accept: application/vnd.github+json' \
      'https://api.github.com/search/repositories?q=rumor+in:name&sort=stars&order=desc&per_page=15'

## 4. A live vocabulary collision inside Rust

In the Nostr protocol a **rumor** is a defined term: an unsigned event, per
NIP-59 gift-wrap / NIP-17. Crates on crates.io use it that way today:

    nmp-nip59             "NIP-59 gift-wrap / seal / rumor"
    nmp-nip17             "kind:14 chat-message rumor"
    nostr-double-ratchet  4,713 downloads

This is the collision that matters more than the taken name: a Rust reader
who knows Nostr will read `rumor` as "unsigned event" before "simulated user
with a liar". The epic's own vocabulary must not reuse the bare word for a
second technical meaning without saying so.

## 5. Meaning in other languages

From en.wiktionary (`action=parse&prop=wikitext`), the senses listed first:

| Language | Form | First senses |
|---|---|---|
| Latin | rumor | hearsay, gossip; the voice of the people; also rustle, murmur |
| Spanish | rumor | murmur |
| Portuguese | rumor | rumour; **continuous noise** |
| Italian | **rumore** | **noise, sound**; then reputation, rumour |
| Indonesian | rumor | rumour, gossip |
| Catalan, Tagalog | rumor | present as a borrowing |

Nothing is offensive or obscene in any language checked. One point worth a
line in the README: across the Romance languages the primary sense slides
from *hearsay* to **noise** — in Italian `rumore` is noise outright. For a
tool whose entire job is telling a real finding from noise, that is an
unfortunate second reading, and it is the one a non-English reader meets
first. It argues for a tagline that always carries the hearsay sense.

    curl 'https://en.wiktionary.org/w/api.php?action=parse&page=rumor&prop=wikitext&format=json'
    curl 'https://en.wiktionary.org/w/api.php?action=parse&page=rumore&prop=wikitext&format=json'

## 6. The transfer ask: deferred by the maintainer, not sent

Asked and answered on **2026-10-10**. The ask was drafted and put in front of
the maintainer, who deferred it:

> We can perhaps do this after the development of the project. Name collision
> is fine for now since we won't publish initially to crates.io.

So **no ask has been sent to the holder of the `rumor` crate name**, and none
will be until the project is further along. The reasoning is sound and worth
keeping beside the decision: the collision only costs anything at the moment
of publishing to crates.io, and that moment is not near. A name the project
does not yet need is not worth a stranger's attention.

What this fixes in place: the crate is `rumor-cli`, as the epic decided, and
nothing in the project depends on the bare name. If the ask is made later and
granted, publishing under `rumor` is an improvement landed by a patch run.

## 7. What this does not settle

- Whether the holder would transfer the name. **Unasked**, by the
  maintainer's decision above, rather than asked and refused.
- Trademark. No search of any trademark register was made.
- Whether `rumor-cli` remains the right crate name if the project never
  publishes to crates.io. It is a Decided item and stays until an ADR moves
  it; a crate named `rumor` that cannot be published would only defer the
  collision to the first `cargo publish`.
