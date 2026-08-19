---
title: Error Contracts
summary: Rules for classifying and preserving failure across adapters, routing, and CLI output.
owner: pattern-author
tier: packet
group: reliability
---

# Error contracts

## Rules

- **R1. MUST** classify expected failures with stable string codes or error
  types. Callers do not branch on message text.
- **R2. MUST** preserve the original error with `%w` or structured diagnostic
  context when translating across a boundary.
- **R3. MUST** return expected failures as errors and normalize external
  failures at the owning boundary.
- **R4. NEVER** swallow a failure, convert it to an empty success, or log and
  continue without an explicit recovery policy.
- **R5. MUST** let the boundary that understands an external harness translate
  its failures. Provider-specific messages do not leak into routing logic.
- **R6. MUST** retry only failures the policy declares recoverable and retain the
  first failure when the final result is unsuccessful.
- **R7. MUST** distinguish usage/configuration errors from execution failures in
  process exit behavior.
- **R8. NEVER** include credentials, tokens, cookies, or raw authentication
  payloads in messages, causes, logs, or serialized job state.
- **R9. MUST** make every caller-visible message a concise description of its
  observed or resulting state. Include the exact paths, filenames, search
  range, identifiers, and observed values that establish the condition whenever
  they are known.
- **R10. NEVER** prescribe an action or recovery to the caller. A message MAY
  name an official command when it states a fact about system ownership, such
  as `Grok authentication is owned by grok login.`; it MUST NEVER turn that fact
  into a direction, such as `Run grok login, then retry.`

Unexpected invariant violations may panic. Expected user and route states should
be representable without forcing consumers to scrape error messages.
