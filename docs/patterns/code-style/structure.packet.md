---
title: Code Style — Structural Rules
summary: Judgment-based rules for control flow, callable shape, dependencies, and comments.
owner: pattern-author
tier: packet
group: language
---

# Code style — structural rules

## Rules

- **R1. PREFER** package-level functions for behavior without receiver state.
  A method is earned by a real domain value, lifecycle, or interface contract,
  not by grouping verbs.
- **R2. MUST** make package dependencies explicit through parameters or an
  explicit construction boundary. Hidden service location and ambient mutation
  make tests and execution order unreliable.
- **R3. PREFER** a `switch` or named helper over an `else if` ladder when data
  already contains the dispatch key.
- **R4. NEVER** comment what well-named code already says. Comments explain a
  non-obvious constraint, invariant, or external workaround and do not narrate
  the current task.

Small symmetric branches and required dynamic imports are valid when their
reason is visible from the surrounding contract.

## Review signals

- A method with no receiver state or interface contract.
- A package that discovers dependencies through globals or mutable singleton
  setters.
- A comment that restates an assignment, branch, or function name.
