---
title: Landing — Engineering Patterns
summary: The durable implementation rules that keep Landing coherent as a Go project.
---

# Engineering patterns

Patterns are binding rules for new and changed code. The inherited legacy
utility predates this catalog and may not comply; do not turn unrelated work
into a broad cleanup. New and changed code adopts these patterns from its first
file.

## Pattern map

- [Code style](code-style/overview.md) — explicit dependencies, simple callable
  shapes, guard clauses, and visible initialization
- [Go](go/overview.md) — soundness, modeling, runtime boundaries, and toolchain
  posture
- [Simplicity](simplicity/overview.md) — the smallest durable abstraction and a
  bias against speculative machinery
- [Errors](errors/overview.md) — typed expected failures, preserved causes, and
  honest terminal results
- [Configuration](configuration/overview.md) — validated project policy and
  centralized environment access
- [Testing](testing/overview.md) — colocated tests and behavior-first contracts
- [Domain design](domain/overview.md) — names and module boundaries that expose
  the product's concepts rather than procedures
- [External integrations](external-integrations/overview.md) — provider and
  harness behavior behind explicit adapter boundaries

## Canon

Each rule has one home and one owner. `*.lint.md` leaves are mechanically
checkable; `*.packet.md` leaves require judgment by a reviewer. An overview
explains the doctrine and links its leaves.

Hard words are deliberate:

- **MUST / NEVER** are requirements.
- **PREFER / SHOULD** are defaults that may be overridden by a documented local
  constraint.

Add a pattern only when the same decision applies in more than one real place,
violating it would create meaningful risk, and code or tooling cannot carry the
rule alone. Search this catalog before adding another home.
