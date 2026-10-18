# FIXTURE for arm 5

SETUP: `stages.browser.enabled` goes true, without which the arm is vacuous.

CONDITION: a role holding both `browser` and `api`, so a browser-stage actor
could take the api action the twin already declares. No browser ACTION is
needed -- reachability is about the actor's contexts and the action's.

This role is deliberately not in the twin: quill's agreement test refuses a
declared role no case carries, and the twin's cases carry none.
