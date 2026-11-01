# FIXTURE for arm 19, the profile-observation name cell

SETUP: `[profiles.cell]` in platform.toml, because a profile's claim is the
subject and the twin declares no profiles.

CONDITION: `observations = ["nosuch"]`, a name no [[observation]] declares.

WHY THIS CELL IS THE DANGEROUS ONE: a profile's claim is also what takes a name
OUT of the project's set (platform.Observes), so a misspelled claim removes
nothing and leaves the real observation taken by EVERY profile -- restoring the
reading the level was ruled in to prevent, while reading as a working
declaration from inside the profile that wanted it.
