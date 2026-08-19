---
title: Testing
summary: Tests stay close to their subjects and prove product contracts at the narrowest useful boundary.
---

# Testing

Landing tests behavior that a caller or neighboring component depends on. Tests
are colocated, deterministic by default, and named after the source contract
they protect.

## Leaf

- [Test structure](test-structure.packet.md) — placement, naming, seams, and
  behavioral scope

Landing uses `go test`. The structural contract keeps tests close to their
subjects while preserving Go's package and fixture conventions.
