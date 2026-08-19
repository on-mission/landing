---
title: Test Structure
summary: Rules for colocated tests, behavioral assertions, and external-boundary seams.
owner: pattern-author
tier: packet
group: testing
---

# Test structure

## Rules

- **R1. MUST** place unit and module tests in the same package directory as the
  source they cover.
- **R2. MUST** name a test file `<module>_test.go` to mirror its sibling source
  module.
- **R3. MUST** keep file fixtures in `testdata/`. Go excludes it from normal
  package builds, and it stays beside the package that owns the contract.
- **R4. PREFER** a table-driven test when one behavior has several named inputs
  and expected results. A new case becomes a row rather than a copied function.
- **R5. MUST** assert observable behavior and boundary contracts rather than
  private call order or internal helper shape.
- **R6. MUST** use deterministic fakes for time, capacity, subprocesses, and
  provider output in the normal verification path.
- **R7. MUST** keep integration tests that invoke installed harnesses clearly
  separated and opt-in. A normal test run does not spend model capacity.
- **R8. NEVER** weaken production types or expose production-only helpers merely
  to make a test convenient.
- **R9. PREFER** one focused regression test for a failure contract over a broad
  snapshot that obscures which behavior matters.

End-to-end suites may live at the repository root when they exercise the CLI as
an installed product rather than one source module.
