---
title: Extraction and Duplication
summary: Abstraction is licensed by harmful drift, not repetition count.
owner: pattern-author
tier: packet
group: architecture
---

# Extraction and duplication

## Rules

- **R1. PREFER** duplication until two copies share a contract whose silent
  divergence would break product behavior.
- **R2. NEVER** extract because code appeared twice, looked similar, or might be
  reused. Similar procedures can belong to different causes.
- **R3. MUST** name the concrete drift consequence before creating a shared
  abstraction.
- **R4. MUST** hoist only the smallest fragment whose drift is dangerous. Keep
  caller-specific orchestration local.
- **R5. MUST** place an earned abstraction with the domain concept that owns the
  shared contract, not in a cross-domain helper bucket.
- **R6. MUST** inspect every real call site before deciding that duplication is
  harmful or that an existing abstraction represents one cause.
- **R7. SHOULD** inline an abstraction when its callers no longer share a drift
  consequence, even if the helper still reduces line count.

Examples of earned contracts include serialized identifiers, configuration
precedence, normalized capacity, and error codes. Similar setup screens or
provider-specific request builders usually evolve independently and should stay
separate.
