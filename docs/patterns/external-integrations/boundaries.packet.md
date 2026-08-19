---
title: External Integration Boundaries
summary: Rules that contain provider volatility and preserve safe execution behavior.
owner: pattern-author
tier: packet
group: boundaries
---

# External integration boundaries

## Rules

- **R1. MUST** keep harness commands, SDK calls, response parsing, capacity
  interpretation, and provider failure classification inside an adapter.
- **R2. MUST** treat every external response and subprocess stream as untrusted
  input until parsed.
- **R3. MUST** expose normalized Landing concepts to routing and execution code.
  External SDK types and provider-specific message strings do not cross the
  adapter boundary.
- **R4. MUST** define timeout and cancellation behavior for every external
  operation. Cancellation stops owned subprocesses and releases resources.
- **R5. MUST** make capacity probes read-only and return explicit unknown state
  when a trustworthy measurement is unavailable.
- **R6. NEVER** collect, scrape, log, or persist consumer credentials. Supported
  official tooling or official APIs retain authentication ownership.
- **R7. MUST** preserve external exit status, stderr, and failure cause in
  diagnostic state without leaking secrets into user output.
- **R8. MUST** verify an adapter with contract tests that cover success,
  malformed output, timeout, exhaustion, unknown capacity, and cancellation.

An adapter may contain provider-specific complexity. That complexity is the
reason for the boundary and must not be “simplified” by leaking it into callers.
