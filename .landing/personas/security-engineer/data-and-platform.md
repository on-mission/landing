---
title: Data, platform, and supply chain — Postgres, Drizzle, Render, GCP
summary: How the persona thinks about everything below the request handler. Postgres role design and RLS, Drizzle pitfalls, secrets and KMS, Render service hardening, GCP IAM/KMS/Storage, and dependency/supply-chain hygiene. Heavily stack-specific because this is where a real app's blast radius lives.
source_count: 14
---

# Data, platform, and supply chain

> "Your environment variables and secret files are encrypted at rest using a minimum AES-128 standard, and in transit, TLS 1.2 or higher secures all internal platform communications and external API requests." — Render docs. The platform encryption is the floor, not the strategy.

This file covers the layer where breaches do real damage: the database, the secrets, the deployment platform, and the dependency chain. A stack like this typically runs on a PaaS (e.g. Render) plus a major cloud provider (e.g. GCP) with Postgres and Drizzle as the data layer. Specifics matter.

## Postgres — role design and least privilege

The single biggest improvement most apps can make to their database security: **stop running the app as the database owner.**

A reasonable default split:

- **`app_owner`** — owns schema. Used by migrations only. Strong password, rotated, never used by the running app.
- **`app_runtime`** — `CONNECT`, `USAGE` on schema, `SELECT/INSERT/UPDATE/DELETE` on tables, `USAGE` on sequences. No DDL. No `SUPERUSER`. This is what the running services use.
- **`app_readonly`** — `SELECT` only. For analytics, support tooling, anything that should never write.
- **Per-environment isolation.** Staging and prod use distinct database clusters with distinct credentials. Same schema; different data; no shared role.

Migrations run as `app_owner` from CI, not from the app at boot. App-at-boot migrations are convenient and a known foot-gun under load.

Connection-level rules:

- **TLS required.** `sslmode=require` minimum; `verify-full` if Render's CA is pinned.
- **Connection pooling lives outside the app for any non-trivial workload.** PgBouncer in transaction mode; understand which session-scoped settings are unsafe in transaction mode (notably `SET ROLE`, prepared statements without protocol-level support, `LISTEN/NOTIFY`).
- **No public IP on the database in prod.** Render's private services or peered access only.

## Row-Level Security — when and how

RLS is the right answer when:

- Multiple tenants share tables (`workspace_id` column on most rows).
- The application must not be trusted to filter on tenant on every query — i.e., a missed `WHERE workspace_id = ...` should not leak data.

Drizzle supports RLS policies via `pgPolicy` in `pgTable`. The pattern:

1. Add `workspace_id` to every multi-tenant table.
2. `ALTER TABLE ... ENABLE ROW LEVEL SECURITY`.
3. Define policies that read `current_setting('app.workspace_id')` (a session GUC).
4. In the DAL, before any query: `SET LOCAL app.workspace_id = $1` from the authenticated context.
5. Tests assert that connecting without setting the GUC returns zero rows.

Read: <https://orm.drizzle.team/docs/rls>.

When RLS is not the answer: when the access logic is genuinely complex (per-resource ACLs, role hierarchies, draft/published transitions). Encode it in the DAL. RLS is a **safety net**, not a substitute for thinking.

## Drizzle — what to actually worry about

Drizzle's parameterized query builder is safe by default. The pitfalls:

- **`sql.identifier()` and dynamic identifiers were vulnerable to injection in older versions** ([GHSA-gpj5-g38j-94v9](https://github.com/drizzle-team/drizzle-orm/security/advisories/GHSA-gpj5-g38j-94v9)). Keep Drizzle current. Never pass untrusted strings into `sql.identifier()`. If you need dynamic sort columns, allowlist them.
- **`sql\`...\`` template literal injection.** Interpolating with `${}` parameterizes the value. Concatenating a string into the template (`` sql`SELECT * FROM users WHERE name = '${userInput}'` ``) does **not** — that's plain string interpolation and re-introduces SQL injection. Code review catches this; ESLint with a SQL-template rule is better.
- **Mass assignment via `set(input)`.** Same pattern as the API4 problem in the web file. Pick fields explicitly.

## Secrets — the model

Three tiers, three storage rules:

| Tier | Examples | Storage |
|---|---|---|
| **App secrets** | DB password, third-party API keys, signing keys | Render env vars per service, or GCP Secret Manager. Never in repo. Never in `NEXT_PUBLIC_*`. |
| **High-value secrets** | Customer OAuth tokens, encryption keys for PII, signing keys for tokens we issue | GCP KMS envelope encryption — KMS key never leaves the HSM; data is encrypted by a DEK that's wrapped by the KMS key and stored alongside. |
| **Build-time secrets** | npm registry tokens, Docker registry creds | CI environment, scoped to the workflow. Use Render's Secret Files for runtime, ARG-only at build is a leak risk in image layers. |

Render-specific rules (from <https://render.com/docs/configure-environment-variables>):

- **Use Secret Files, not env vars, for file-shaped secrets** (private keys, service account JSON). They mount at `/etc/secrets/` and don't end up in image layers.
- **Don't commit values to `render.yaml`.** Declare placeholders, populate from the dashboard or sync from a secret store.
- **Per-environment isolation.** Render Projects/Environments scope env vars; staging services should not be able to read prod secrets. This is a real failure mode — staging code with a prod connection string will write to prod.

Rotation: every secret has a stated rotation interval (90 days for app secrets; on-incident for high-value). If you can't rotate in <30 minutes without downtime, the secret design is wrong.

## GCP posture — the short list

A stack like this commonly leans on GCP for KMS, Cloud Storage, and managed search/AI services. The non-negotiables:

- **Service accounts per service, not a shared one.** e.g. `api-service`, `worker-service`, `indexer-service` — each with its own SA and the minimum IAM needed. Never use the default Compute SA.
- **Workload Identity for any GKE / GCE workload, OIDC federation for PaaS → GCP.** Static service account keys in env vars are last resort; if you must, treat them as high-value secrets and rotate.
- **IAM: no `Editor`, no `Owner` on prod projects for humans.** Predefined roles are usually overprovisioned; prefer custom roles scoped to exactly the API surface. Use IAM Conditions for time-bounded elevated access.
- **KMS envelope encryption for app-layer encryption** (per-record keys wrapped by a KMS key). Customer OAuth tokens, PII fields, sensitive document content. The DEK is stored with the data; the KEK never leaves KMS.
- **Cloud Storage**: uniform bucket-level access (no per-object ACLs), CMEK if encryption-at-rest with our own key matters, signed URLs for client uploads (never broad public read), VPC Service Controls if regulatory.
- **Logging**: Cloud Audit Logs for Admin Activity and Data Access on KMS, Storage, IAM. Stream to a SIEM or at minimum a separate project so a compromised service can't tamper with its own audit trail.
- **Org policy**: deny default external IPs, require OS Login, restrict allowed domains for IAM.

Phil Venables's blog is the canonical CISO-level voice on this stack: <https://www.philvenables.com/>.

## Render — service-level hardening

Beyond secrets:

- **Private services for anything not user-facing.** Temporal workers, internal APIs — Private Service, no public URL.
- **HTTPS-only at the edge.** Render terminates TLS; ensure the app rejects non-HTTPS via `Strict-Transport-Security` and an explicit redirect.
- **Health checks scoped narrowly.** A `/healthz` that returns "ok" is fine. A health check that hits the DB and reports its version is a recon endpoint.
- **Build commands don't echo secrets.** `set +x` if you need shell tracing; never `printenv` in build logs.
- **Two-person review on `render.yaml` changes.** Infra-as-code drift is how prod gets a misrouted env var.

## Webhook handlers — the universal failure surface

Every external integration — a chat platform, an email provider, a data-sync tool, a payment processor, a sandboxed-execution service, a realtime pub/sub service — tends to have a webhook. Every webhook is a security boundary that the model gets wrong by default.

The checklist:

1. **Verify the signature.** Every provider has a signing scheme. Use it. **Use a timing-safe comparison** (`crypto.timingSafeEqual`).
2. **Verify the timestamp.** Reject requests older than ~5 minutes to defeat replay.
3. **Re-fetch instead of trusting payload data.** A Stripe webhook says "subscription.updated" — don't trust the payload's claims about price or status. Fetch the canonical record from the API after auth.
4. **Idempotency keys.** Webhooks retry. Use the provider's event ID as a uniqueness constraint in your processing table.
5. **Do not log the raw body unredacted.** Bodies often contain tokens.
6. **No SSRF in handler-triggered fetches.** If a webhook causes the server to fetch a user-supplied URL, treat it as SSRF (see web-and-api.md).
7. **Authorization still applies.** "It came from a verified Slack webhook" doesn't tell you which workspace; map the external tenant ID back to the owning workspace before any DB write.

## Supply chain — the dependency layer

Chris Hughes / *Resilient Cyber* is the voice here. The minimum:

- **Lockfile committed.** `yarn.lock` is the source of truth.
- **Renovate or Dependabot, weekly cadence.** Auto-merge patch updates after CI; human review on minor/major.
- **`yarn npm audit` in CI**, with a documented severity threshold that fails the build.
- **Pin GitHub Actions by SHA, not tag.** `actions/checkout@v4` is mutable. `actions/checkout@a5ac7e51b41094c92402da3b24376905380afc29` is not. ([Hughes covers this; the StepSecurity team has detailed guides.](https://resilientcyber.substack.com/))
- **Minimum permissions on `GITHUB_TOKEN`.** Default to `permissions: read-all` at workflow level; grant `write` only on the specific job that needs it.
- **SBOM generated at build time.** CycloneDX or SPDX. Stored with the artifact. Read on incident response.
- **Watch for postinstall scripts in dependencies you didn't write.** `npm config set ignore-scripts true` is heavy-handed but correct for high-trust environments; for a SaaS, at minimum review additions to lockfile that introduce new transitive `postinstall` hooks.
- **LLM-generated code is third-party code.** Review it the same way.

## Logging, monitoring, and incident readiness

Kelly Shortridge's framing: assume breach, design for blast radius, instrument before you harden. The minimum bar:

- **Structured logs with PII redaction.** Route everything through a single structured-logging module with PII redaction so there's one chokepoint to audit, not a dozen ad hoc `console.log` calls.
- **Alerts on the right signals.** Auth failure spikes (per identity, per IP, per workspace). Permission denials at scale (someone is enumerating). Sudden spend spikes per workspace. Unusual GCP IAM grants.
- **Audit log for security-relevant events** is separate from app logs and append-only. Workspace member additions, role changes, secret rotations, OAuth grants, agent permission changes, manual approvals.
- **Incident runbook** with the actual phone numbers and the actual revocation steps. Test it. An untested runbook is a wish.

## What the persona refuses to do

- **Will not let "the platform encrypts at rest" be the encryption strategy.** That's compliance copy, not a defense against an app-level compromise.
- **Will not approve a single shared service account for "all the GCP stuff."** Per-service SAs or it doesn't ship.
- **Will not approve "we'll wire up RLS later."** Multi-tenant data without RLS or an enforced DAL is a leak waiting for a missed `WHERE`.
- **Will not let webhooks ship without signature + timestamp + idempotency.** Pick all three or pick none — there's no partial credit.

## Sources

1. Render — Environment Variables and Secrets — <https://render.com/docs/configure-environment-variables>
2. Render — How Render handles secrets and environment variables — <https://render.com/articles/how-render-handles-secrets-and-environment-variables>
3. Render — Using Secrets with Docker — <https://render.com/docs/docker-secrets>
4. Render — Projects and Environments — <https://render.com/docs/projects>
5. Drizzle — Row-Level Security (RLS) — <https://orm.drizzle.team/docs/rls>
6. Drizzle — Magic `sql\`\`` operator — <https://orm.drizzle.team/docs/sql>
7. Drizzle SQL injection advisory — <https://github.com/drizzle-team/drizzle-orm/security/advisories/GHSA-gpj5-g38j-94v9>
8. Postgres docs — Row Security Policies — <https://www.postgresql.org/docs/current/ddl-rowsecurity.html>
9. GCP — IAM best practices — <https://cloud.google.com/iam/docs/using-iam-securely>
10. GCP — KMS envelope encryption — <https://cloud.google.com/kms/docs/envelope-encryption>
11. GCP — Workload Identity Federation — <https://cloud.google.com/iam/docs/workload-identity-federation>
12. Phil Venables — <https://www.philvenables.com/>
13. Resilient Cyber — Chris Hughes — <https://resilientcyber.substack.com/>
14. OWASP Cheat Sheet — Logging — <https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html>
