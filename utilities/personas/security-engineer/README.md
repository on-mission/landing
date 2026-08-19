---
title: Security engineer persona corpus
summary: Source corpus for the `/security-engineer` command. Composite of the most prolific application-security voices (Janca, Hunt, Miessler, Gibler, De Ryck, Manico, Shkedy/Yalon, Hoffman, Shortridge, Venables, Hughes) tuned to a common stack for this kind of product — Next.js, TypeScript, tRPC, Zod, Drizzle, Postgres, Render, GCP. Deliberately small: the model already knows this material; this corpus is pointers + stack-specific glue.
---

# Security engineer corpus

Reference material for the `/security-engineer` Claude command (`.claude/commands/security-engineer.md`). The command acts as a senior application-security engineer composed from the voices in `voices.md`, biased toward an example stack (Next.js, TypeScript, tRPC, Zod, Drizzle, Postgres, deployed on a PaaS + a major cloud provider).

| File | Focus | Lines | Sources |
|---|---|---:|---:|
| [voices.md](./voices.md) | Who the persona is composed of — Janca, Hunt, Miessler, Gibler, De Ryck, Manico, Shkedy/Yalon, Hoffman, Shortridge, Venables, Hughes — and which voice owns which question | 111 | 14 |
| [web-and-api.md](./web-and-api.md) | Request-path hardening — OWASP Top 10 / API Top 10 mapped onto Next.js Server Actions, RSC, tRPC, Zod, sessions, CSP, rate limiting | 129 | 12 |
| [data-and-platform.md](./data-and-platform.md) | Postgres roles + RLS, Drizzle pitfalls, secrets, Render service hardening, GCP IAM/KMS/Storage, webhooks, supply chain | 159 | 14 |

**Total:** ~399 lines, 40 sources. Intentionally lean: the corpus is a routing layer + stack-specific guidance, not a re-derivation of OWASP. When the user wants depth, the persona points at canonical sources (OWASP Cheat Sheets, PortSwigger Academy, De Ryck talks, Phil Venables blog) rather than reproducing them.
