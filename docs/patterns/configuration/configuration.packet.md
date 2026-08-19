---
title: Configuration Rules
summary: Rules for deterministic `.landing/config.json` resolution and safe configuration boundaries.
owner: pattern-author
tier: packet
group: boundaries
---

# Configuration rules

Project configuration is a JSON file at `.landing/config.json`. The `.landing/`
directory is the project container for that file and for project personas. JSON
is available through Go's standard library, keeping the Landing binary
dependency-free.

## Rules

- **R1. MUST** parse configuration and environment input at one owning boundary
  before it reaches routing or execution code.
- **R2. MUST** represent precedence explicitly: built-in defaults, then the
  nearest applicable project configuration file at `.landing/config.json` found
  by searching upward from the invocation directory.
- **R3. MUST** distinguish absent values, explicit disablement, and inherited
  values supplied by built-in defaults. Truthiness is not a configuration merge
  strategy.
- **R4. NEVER** call `os.Getenv` or `os.LookupEnv` throughout the domain.
  Environment access is centralized and converted into a typed runtime
  configuration.
- **R5. NEVER** store provider tokens, cookies, or passwords in Landing project
  policy files.
- **R6. MUST** keep project configuration portable. Machine-specific connection
  bindings are resolved locally and diagnosed when missing.
- **R7. MUST** validate references after resolving built-in defaults with the
  nearest applicable project configuration, including tier names, model routes,
  and connection identifiers.
- **R8. MUST** preview persistent changes and write them atomically so an
  interrupted setup does not leave partial policy.
- **R9. MUST** return a terminal configuration error when no applicable
  `.landing/config.json` resolves. A `.landing/` directory or personas beneath
  it do not satisfy configuration resolution. The error identifies the
  configuration file and the directories searched.
- **R10. MUST** make `.landing/config.json` a complete directly authorable
  configuration interface. Guided setup reads and writes that same file without
  requiring a separate configuration representation; the surrounding
  `.landing/` directory is not one.
