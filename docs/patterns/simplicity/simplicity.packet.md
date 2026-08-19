---
title: Simplicity Rules
summary: Rules that prevent speculative layers, configuration, and abstractions.
owner: pattern-author
tier: packet
group: architecture
---

# Simplicity rules

## Rules

- **R1. MUST** solve the present product contract before designing for an
  imagined second system, provider shape, or deployment model.
- **R2. MUST** earn every abstraction with a named invariant, drift consequence,
  or repeated boundary. “Reusable” and “might need later” are not licenses.
- **R3. PREFER** direct control flow over indirection that only renames a call.
- **R4. MUST** keep policy data-driven only where users or product behavior
  actually vary it. Do not turn every constant into configuration.
- **R5. NEVER** introduce a generic registry, plugin system, event bus, or
  dependency-injection framework for one implementation.
- **R6. MUST** remove dead branches and compatibility code when their supported
  caller no longer exists; history belongs in version control.
- **R7. PREFER** a slightly duplicated readable workflow over a shared helper
  whose callers are likely to evolve independently.

## Review question

Name the current product requirement or invariant that each new layer protects.
If it has none, remove the layer.
