# FIXTURE for arm 19, venire cell

SETUP: `[[role]] author` in bindings.toml, because the cell pairs only where
the project declares roles AT ALL (ibis, coop r4/116; the twin declares none
and refusing it would refuse an example fable's D9 requires to pass).

CONDITION: `[[venire]] role = "wizard"`, which no [[role]] declares.
