---
title: Go Toolchain Posture
summary: Toolchain guarantees that keep Landing's safety and distribution properties explicit.
owner: pattern-author
tier: packet
group: language
---

# Go toolchain posture

## Rules

- **R1. MUST** run `go vet` and a linter such as `staticcheck` or
  `golangci-lint`; their failures are errors, not warnings.
- **R2. MUST** pin the Go version in `go.mod`.
- **R3. MUST** build release binaries with `CGO_ENABLED=0` so they remain
  static and do not require a runtime dependency.
- **R4. MUST** cross-compile supported binaries for darwin, linux, and windows
  on amd64 and arm64.
- **R5. NEVER** silence a linter with `//nolint` without a concrete same-line
  rationale and a removal condition.

Tool selection may change without changing this posture. Safety and
distribution guarantees are product-quality decisions, not formatting.
