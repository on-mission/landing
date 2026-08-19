---
title: External Integrations
summary: Harness and provider behavior remains behind adapters with safe, normalized contracts.
---

# External integrations

Provider tooling is outside Landing's control. Adapters contain volatility and
translate it into stable Landing concepts without taking ownership of user
credentials.

## Leaf

- [Integration boundaries](boundaries.packet.md) — authentication, parsing,
  timeouts, cancellation, capacity, and error normalization

This pattern applies to local CLI harnesses and future official API
integrations. It does not authorize a provider or distribution model; those are
product and policy decisions.
