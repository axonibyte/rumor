# FIXTURE for arm 5

ONE CHANGE, no setup: the twin already enables the browser stage. This adds a
role holding both `browser` and `api`, so a browser-stage actor could take the
api action the twin declares.

The role is deliberately not in the twin: tests/contract's agreement test
refuses a declared role no case carries, and the twin's cases carry none.
