# FIXTURE for arm 19, the profile-venire role cell

SETUP: `[[role]] author` in bindings.toml, because the cell pairs only where the
project declares roles AT ALL; and `[profiles.cell]` in platform.toml, because an
APPENDED row is the subject and the twin declares no profiles.

CONDITION: `[[profiles.cell.venire]] role = "wizard"`, which no [[role]] declares.
The refusal NAMES THE PROFILE, because `draws role wizard` with no table sends a
reader to the platform's own venire, which is not where it is written.
