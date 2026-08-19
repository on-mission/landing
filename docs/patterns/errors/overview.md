---
title: Errors
summary: Failures remain typed, causal, model-readable, and honest across routing boundaries.
---

# Errors

Landing routes work through several failure domains. Its caller is usually a
model, so every caller-visible message is context the model reads. An error
must preserve whether the user supplied invalid input, configuration is
unusable, a route is unavailable, execution failed, or Landing itself violated
an invariant.

## Leaf

- [Error contracts](error-contracts.packet.md) — classification, causes,
  normalization, message content, retries, and terminal results

Error names and codes are product contracts once callers branch on them. They
are not free-form log messages.
