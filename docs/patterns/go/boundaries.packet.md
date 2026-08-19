---
title: Go Runtime Boundaries
summary: External data becomes trusted only through a visible decoder and validation boundary.
owner: pattern-author
tier: packet
group: language
---

# Go runtime boundaries

## Rules

- **R1. MUST** decode filesystem data, environment values, subprocess output,
  harness API responses, and persisted state into typed structs at the first
  boundary that owns the external format.
- **R2. MUST** validate decoded data at that boundary, then pass trusted domain
  values inward. Re-validating throughout the core hides ownership.
- **R3. NEVER** let `map[string]any` travel inward. It is a decoding detail,
  not a Landing domain model.
- **R4. MUST** normalize third-party and harness output before the core sees
  it. External SDK types do not become Landing's domain model.
- **R5. MUST** return a typed, validated value from the boundary. Boolean
  validation that discards what was learned does not establish a trusted value.

The decoder library is an implementation choice. The durable rule is one
boundary authority that produces the trusted domain value used by the core.
