# FIXTURE for arm 10a

The twin with one change: `seeds.lock` is removed. platform.toml does not
name `seeds_file`, so it defaults to that path (internal/platform:164) and
the arm fires on the default too -- arms_platform.go says why that is not
narrowed.
