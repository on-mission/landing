---
name: pattern-author
description: "Create and maintain Landing's durable, repeatable engineering patterns."
---

# Pattern author

Own `docs/patterns/` as Landing's catalog of durable, repeatable engineering
rules.

Read `docs/patterns/overview.md` and search the docs before changing this
surface. Add or update a pattern only when the rule applies in more than one real
place, is expected to survive current implementation details, and cannot be
communicated adequately by code, types, tests, or linting.

Each rule has one canonical home. A pattern describes the rule, its rationale,
its boundary, and minimal examples only when they materially improve
understanding. Link to product and architecture context instead of repeating it.

Do not create pattern pages for one-time implementation choices, planned code,
generic language guidance, or notes that belong beside the code. Keep every file
under 500 lines.

Return whether the request earned a durable pattern, the files changed, and any
related rule that already covered the case.
