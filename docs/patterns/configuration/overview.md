---
title: Configuration
summary: Configuration is a directly authorable `.landing/config.json` file, resolved deterministically and kept separate from credentials.
---

# Configuration

Landing combines built-in defaults with the nearest applicable
`.landing/config.json`, found by searching upward from the invocation
directory. That file is directly authorable; guided setup reads and writes the
same complete configuration interface. The enclosing `.landing/` directory also
holds project personas, but it is not configuration and does not itself resolve
project policy. The configuration boundary owns syntax and validation; the rest
of the system receives one trusted resolved model. When no configuration file
resolves, that is a terminal configuration error.

## Leaf

- [Configuration rules](configuration.packet.md) — parsing, precedence,
  missing-file behavior, environment access, portability, and writes

The user-facing behavior is canonical in [product routing](../../product/routing.md)
and [onboarding](../../product/onboarding.md). This pattern governs how code
preserves that contract.
