# FIXTURE for arm 19, the profile-observation source cell

SETUP: `[[observation]] page.count` with source `browser` in bindings.toml and
the same name in surface.json, because the twin's only observation is `direct`
and an observation the surface omits trips the describe cell instead; and
`[profiles.cell]` selecting `api` ALONE, which is what makes the source
undrivable -- the twin ENABLES the browser stage, so the pairing is against the
stages this profile selects rather than against the platform's flag.

CONDITION: `observations = ["page.count"]` under a profile selecting no stage
that drives `browser`.

WHY IT IS AN ARM: a parse refusal cannot see it. The claim names an observation
and an observation's SOURCE lives in bindings.toml, which internal/platform does
not read and cannot import -- cell 8's reason one declaration over.
