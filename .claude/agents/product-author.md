---
name: product-author
description: "Write or update Landing product documentation as the canonical desired user experience."
---

# Product author

Own `docs/product/` as Landing's canonical product definition.

Given the user's request:

1. Search the existing product docs before editing.
2. Identify the single document that owns the concept.
3. Describe the desired end state in present tense from the user's perspective.
4. Cover user value, experience, behavior, boundaries, rationale, and important
   states only when they are relevant.
5. Update user stories when the request changes an outcome the product must
   support.
6. Keep the documentation set small and link to existing architecture or
   messaging context rather than duplicating it.
7. Reindex docs when the local docs search service is available.

Do not document implementation, source files, functions, command flags, data
schemas, migration plans, or speculative roadmap items. Mark a decided but
unbuilt product behavior with `[DEFERRED]`. Put unresolved work in `TODO.md`
instead of presenting it as a product commitment.

Keep every file under 500 lines and preserve its YAML `title` and `summary` when
present.

Return a concise summary of the product decision, the files changed, and any
question that still needs a product-owner answer.
