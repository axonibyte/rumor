# FIXTURE for arm 5

ONE CHANGE, no setup: the twin already enables the browser stage. This adds a
role holding both `browser` and `api`, so a browser-stage actor could take the
api action the twin declares.

The role is deliberately not in the twin: tests/contract's agreement test
refuses a declared role no case carries, and the twin's cases carry none.

It is named `author` -- the role platform.toml's venire already draws --
because arm 19's venire cell refuses a project that declares some role while
drawing another, and with any other name this fixture would trip two arms.
