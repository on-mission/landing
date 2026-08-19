---
title: Go Modeling
summary: Rules for representing valid product states without type-system theater.
owner: pattern-author
tier: packet
group: language
---

# Go modeling

## Rules

- **R1. MUST** use a closed interface with an unexported marker method for
  states with correlated fields. Only types in the owning package can satisfy
  it, so invalid combinations do not become a bag of optional fields.
- **R2. SHOULD** use a struct with an explicit `Kind` field instead when its
  constructors make illegal combinations unconstructable. The constructor,
  not convention, protects the correlation.
- **R3. MUST** use named types rather than bare `string` or `int` for domain
  identifiers when confusing structurally equal values creates product risk.
- **R4. MUST** model a closed set with string constants and a `Parse` or
  validating constructor. Do not use unvalidated strings as domain state.
- **R5. MUST** make a zero value either valid or unconstructable. A caller must
  not accidentally obtain a plausible but invalid product state.
- **R6. MUST** return an error from the `default` branch when switching over a
  closed set. Go cannot provide compile-time exhaustiveness, so keeping every
  required decision current is a reviewer obligation.
- **R7. PREFER** named, direct structs and interfaces over reflection or clever
  generic machinery. Type cleverness spends reader and compiler budget.

## Review signals

- Several optional fields whose validity depends on a status or kind.
- A domain identifier represented by an undifferentiated primitive.
- A string constant accepted without validation at construction.
- A closed-state switch with no error-producing `default` branch.
