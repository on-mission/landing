---
title: Go Soundness — Mechanical Rules
summary: Escape hatches and constructs that weaken or obscure Go's safety contract.
owner: pattern-author
tier: lint
group: language
candidates: "**/*.go"
---

# Go soundness — mechanical rules

## Rules

- **R1. NEVER** use `interface{}` or `any` to carry a value inward past the
  boundary that owns its format. It may appear only at a decoding edge with an
  inline rationale.
- **R2. NEVER** discard an error with `_ =`. An ignored error needs an inline
  rationale naming why the failure cannot matter.
- **R3. MUST** wrap an error with `%w` when a caller can reasonably branch on
  its cause. Reformatting the error string destroys that chain.
- **R4. NEVER** use a bare type assertion such as `x.(T)`. Use the comma-ok
  form and handle a failed assertion.
- **R5. NEVER** `panic` in library code for an expected failure. Panic is for a
  programmer error that cannot be recovered, not for a provider being down.
- **R6. MUST** take `context.Context` as the first parameter on a function that
  performs I/O, starts a subprocess, or can block.
- **R7. NEVER** use package-level mutable state.

Generated and vendored sources are excluded. Tests may use a documented escape
hatch only to prove an unsafe boundary behaves as intended.
