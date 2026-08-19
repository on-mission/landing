---
title: Code Style
summary: Project-wide Go conventions for predictable, readable code.
---

# Code style

Landing code makes state changes and dependencies visible. Values are not
silently shared through mutable globals, functions have one consistent shape,
the happy path stays unindented, and packages do not hide work behind
unnecessary lifecycle.

## Leaves

- [Mechanical rules](language.lint.md) — mutable globals, return values,
  initialization, and guard clauses
- [Structural rules](structure.packet.md) — callable shape, explicit
  dependencies, dispatch, and comments

These rules apply to Go. Framework- or platform-required shapes are exceptions
only when the requirement is concrete and visible.
