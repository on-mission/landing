---
title: Go
summary: Landing's Go posture for soundness, modeling, runtime boundaries, and toolchain guarantees.
---

# Go

The compiler proves shape, not validity. Landing uses Go types to make product
states explicit while treating zero values, untyped `interface{}`, and silent
error returns as escape hatches that need evidence.

## Leaves

- [Soundness rules](soundness.lint.md) — banned escape hatches, error handling,
  contexts, and mutable globals
- [Modeling](modeling.packet.md) — closed state, identifiers, zero values, and
  exhaustive review
- [Runtime boundaries](boundaries.packet.md) — decode external input and keep
  Landing's domain model separate from SDKs
- [Toolchain posture](toolchain.packet.md) — static analysis, pinned versions,
  static binaries, and cross-compilation
