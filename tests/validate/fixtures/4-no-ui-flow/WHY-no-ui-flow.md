# FIXTURE for arm 4

SETUP: `stages.browser.enabled` goes true. Both browser arms are VACUOUS
without it (docs/contract.md:419), so the flag is what makes the condition
reachable rather than part of it.

CONDITION: a browser-stage action declaring no flow.

The added action carries `requires` on purpose: without it the setup would
trip arm 3 too, and fable's rule is that a fixture whose setup edit trips an
arm by itself is two fixtures.
