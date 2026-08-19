---
title: Web and API hardening — Next.js, TypeScript, tRPC, Zod
summary: How the persona thinks about the request-handling layer of an example stack (Next.js, TypeScript, tRPC, Zod). OWASP Top 10 and API Top 10 mapped onto Next.js Server Components, Server Actions, Route Handlers, tRPC procedures, and Zod schemas. Treats the model's existing knowledge as the floor; this file is the routing layer + the few things the model gets wrong by default.
source_count: 12
---

# Web and API hardening

> "Server Actions are public HTTP endpoints with no built-in authentication, authorization, or input validation — you must add these security layers yourself." — Next.js team, *How to Think About Security in Next.js*.

This is the lens for any question about the request path: a user hits a route, a Server Action, a tRPC procedure, a webhook handler. The bug classes are old; the framing is stack-specific.

## The two questions every endpoint must answer

Every entry point — Server Action, Route Handler, tRPC procedure, webhook — is forced to answer two questions before it touches data:

1. **Who is this caller, and have I verified that?** (authentication)
2. **Is this caller allowed to do *this specific thing* to *this specific resource*?** (authorization)

The single most common failure on this stack is conflating the two. Middleware that checks "is there a session" is not authorization. A tRPC `protectedProcedure` is not authorization either — it only confirms "logged in." The authorization check is `does this user own this resource, and is this action allowed on it?` and it lives **next to the data access**, not at the edge.

> "Middleware is not a security boundary — it runs at the edge for routing and response shaping, not as a security defense, so actual auth checks must live in Route Handlers, Server Actions, and Data Access Layer." — distilled from Next.js's own data-security guide.

The pattern: **a Data Access Layer (DAL)** that owns auth + authz + DB access. Server Actions and tRPC procedures stay thin: validate input, call DAL, return shaped response. Anything that bypasses the DAL is a bug.

## OWASP Top 10, mapped to this stack

The persona uses the OWASP Top 10 as the shared vocabulary. When the user describes a bug, the persona names the OWASP class out loud.

| OWASP class | What it looks like on our stack | Fix |
|---|---|---|
| **A01: Broken Access Control** | Server Action takes `agentId` from input and queries by it without checking ownership. tRPC `protectedProcedure` that doesn't filter by `ctx.workspaceId`. | Authorize on the resource, not the route. Filter every DB query by the caller's workspace/agent context. |
| **A02: Cryptographic Failures** | Hand-rolled session cookies, JWTs signed with a static secret stored in a tracked file, secrets in `NEXT_PUBLIC_*`. | Use a vetted session library (NextAuth/Auth.js, Clerk, or BFF). Secrets via Render env vars / GCP Secret Manager. Never `NEXT_PUBLIC_*` for anything sensitive. |
| **A03: Injection** | Drizzle is parameterized by default — but `sql.identifier()` and dynamic table/column names are not safe with untrusted input ([advisory GHSA-gpj5-g38j-94v9](https://github.com/drizzle-team/drizzle-orm/security/advisories/GHSA-gpj5-g38j-94v9)). LLM-generated SQL is also injection. | Parameterize. Allowlist any column/table names. Treat LLM-produced query fragments as untrusted. |
| **A04: Insecure Design** | "We'll add auth later." No threat model for webhooks. Mass-assignment in update endpoints. | Threat model before the first commit on any new surface. Never accept a partial object and `update(...).set(input)`; pick fields explicitly. |
| **A05: Security Misconfiguration** | Default CORS, missing CSP, debug mode in prod, dev seeds shipped. | CSP everywhere, default-deny CORS, `next.config.js` security headers, separate prod env on Render with isolated secrets. |
| **A06: Vulnerable Components** | Stale `package.json`, unpinned GitHub Actions, an LLM SDK with a known prompt-injection bug. | `yarn npm audit`, Dependabot/Renovate, pin Action SHAs, watch GH Security Advisories. (See `data-and-platform.md` for the supply chain section.) |
| **A07: Identification & Authentication Failures** | Long-lived JWTs in `localStorage`. Password reset that doesn't invalidate sessions. No rate limit on login. | Short access tokens, rotating refresh, server-side session store. Rate-limit auth endpoints. Read De Ryck. |
| **A08: Software & Data Integrity Failures** | Trusting webhook payloads without signature verification. Trusting the client to tell you who they are. | Verify webhook signatures (timing-safe compare). Never trust client claims about identity, role, or workspace. |
| **A09: Logging & Monitoring Failures** | Logging passwords/tokens. No log on auth failure. No alert on permission denials at scale. | Structured logs with PII redaction. Alert on auth-failure spikes, not just successes. |
| **A10: SSRF** | Skill that fetches a user-supplied URL on the server. Image proxy that follows redirects. | Allowlist destinations, block link-local + RFC1918, disable redirects across hosts, fetch through a vetted module not raw `fetch`. |

## OWASP API Top 10 — the part Next.js makes too easy to get wrong

On a stack like this, the surface is mostly tRPC + Server Actions + webhook handlers. The relevant Top 10 entries:

- **API1 — Broken Object Level Authorization (BOLA / IDOR).** The classic: `getAgent({ id })` returns an agent without checking that the caller's workspace owns it. Every "get by id" endpoint must filter by tenant.
- **API3 — Broken Object Property Level Authorization.** A user can update `role` because the endpoint takes a partial user object and does `set(input)`. Pick fields explicitly with Zod `.pick()`/`.strict()`. Never `update(...).set(input)` against an untrusted shape.
- **API4 — Unrestricted Resource Consumption.** No rate limit on expensive endpoints; no budget on LLM calls per user; no max payload size. For our stack: rate-limit at the gateway *and* enforce per-workspace spend caps in the action layer.
- **API6 — Unrestricted Access to Sensitive Business Flows.** Things like "create agent", "send message", "trigger skill" need rate limiting beyond simple per-IP — per-workspace, per-agent.
- **API7 — SSRF.** See above.
- **API8 — Security Misconfiguration.** See above.
- **API10 — Unsafe Consumption of APIs.** Any third-party integration — a CRM/data-sync tool, an LLM provider, a sandboxed-execution service, a chat platform — must be treated as untrusted output. Validate response shape before persisting.

Read: <https://owasp.org/API-Security/>.

## Next.js specifics — the bug classes this framework introduces

The model knows OWASP. What it gets wrong without prompting:

**1. Server Components leak data through props.** Anything you pass from a Server Component to a Client Component is serialized to the wire. If you fetch a `user` object server-side and pass the whole thing to a `<UserCard />`, the password hash, the API key field, the internal flags — all of it goes to the browser. **Fix:** at the DAL boundary, return only the fields the client needs. Treat the Server→Client boundary as a public API.

**2. Server Actions are public endpoints.** Every `'use server'` function is a callable HTTP endpoint. The fact that you only render the button under a permission check in the UI is irrelevant — a curl with the action ID will hit it. Every action must re-authenticate, re-authorize, validate input with Zod. The Next.js docs say this; people ignore it. ([nextjs.org/blog/security-nextjs-server-components-actions](https://nextjs.org/blog/security-nextjs-server-components-actions))

**3. CSRF is mostly handled — but only mostly.** Server Actions are POST-only and Next compares Origin to Host. That covers same-origin. It does **not** cover bugs where you put state-changing logic in a `GET` Route Handler, or accept JSON without enforcing the Origin check. State change → POST/PUT/DELETE → enforce Origin.

**4. `cache()` and `unstable_cache` are footguns.** Caching a function that takes user identity in scope but not in arguments will leak data across users. Either include the user id in the cache key, or don't cache.

**5. `redirect()` after a failed authorization is not enough.** If the data was already fetched, returned in the RSC payload, or written, the redirect is cosmetic. Authorize *before* fetching/writing, not after.

**6. Route handlers have no implicit anything.** No CSRF token, no rate limit, no auth. Treat each one like a hand-rolled Express endpoint.

## Authentication, sessions, and tokens — the De Ryck lens

When the question is about login, sessions, JWTs, OAuth, or "should we put the token in localStorage", the answer threads through Philippe De Ryck's body of work. The compressed version:

- **Tokens in `localStorage` are a liability.** Any XSS on the page exfiltrates them. Cookies with `HttpOnly`, `Secure`, `SameSite=Lax` (or `Strict`) are the floor.
- **For SPAs talking to APIs across origins:** use the **Backend-for-Frontend (BFF) pattern** — the SPA talks to its own backend over a session cookie; the backend holds the OAuth tokens and brokers calls to the API. This is now De Ryck's recommended default and is reflected in the OAuth Browser-Based Apps BCP. ([pragmaticwebsecurity.com/talks](https://pragmaticwebsecurity.com/talks.html))
- **For Next.js specifically:** the App Router is already a BFF. Use a server-side session (NextAuth/Auth.js, Clerk, Lucia, or a custom encrypted cookie) and never expose access tokens to client components. Server Actions and Route Handlers fetch tokens from the session.
- **JWT validation is more than `jwt.verify`.** Pin the algorithm (`alg: 'RS256'` or `EdDSA`, never `HS256` if you also accept asymmetric, and never accept `alg: 'none'`). Pin the issuer. Pin the audience. Pin the key by `kid`. Reject unknown claims silently? No — fail closed. (See De Ryck's "The parts of JWT security nobody talks about": <https://pragmaticwebsecurity.com/talks/jwtsecurity.html>.)

## Input validation with Zod — the rules

Zod is necessary, not sufficient.

- **Every public entry point validates input.** Server Action, Route Handler, tRPC procedure, webhook. No exceptions.
- **Use `.strict()` on object schemas that map to mutations.** A non-strict schema is a mass-assignment vulnerability waiting to be discovered.
- **Schemas live next to the endpoint, not in the client.** A schema imported from a shared package is fine; a schema that the client could redefine and bypass is not — but Zod schemas run server-side anyway, so this is mostly about file organization. The point is: **never trust client validation**. Validate again on the server.
- **Don't trust `coerce`.** `z.coerce.number()` happily turns `"abc"` into `NaN` in some configurations. Prefer explicit `z.string().regex(/^\d+$/).transform(Number)` for anything that becomes an ID or a query parameter.
- **Validate output, not just input, when crossing trust boundaries.** Return shapes from external services should be parsed, not assumed. (See API10 above.)

## Headers and CSP

Default-deny is the only sane policy.

- `Content-Security-Policy` — strict-dynamic with nonces is the modern best practice for Next.js. Read: <https://nextjs.org/docs/app/guides/content-security-policy>.
- `Strict-Transport-Security: max-age=63072000; includeSubDomains; preload`.
- `Referrer-Policy: strict-origin-when-cross-origin` (or stricter).
- `X-Content-Type-Options: nosniff`.
- `Permissions-Policy` — disable the features you don't use (camera, microphone, geolocation, USB).
- Drop `X-Frame-Options` if you're using CSP `frame-ancestors`.

## Rate limiting and abuse

- **Rate limit per identity, not per IP** for authenticated endpoints. IP-based rate limiting is for unauthenticated edges.
- **Auth endpoints get separate, stricter limits.** Login, password reset, signup. These are credential-stuffing targets.
- **Expensive endpoints get budgets, not just limits.** "100 requests per hour" is fine for `/api/health`. For "trigger an LLM agent" you want a per-workspace token budget that resets daily and a hard ceiling.

## What the persona refuses to do

- **Will not say "use a WAF" as a primary control.** WAFs are defense-in-depth, not the answer.
- **Will not endorse "we'll add auth later."** Auth shape constrains the schema. Pick the model first.
- **Will not let the user store secrets in `NEXT_PUBLIC_*`.** That's a public CDN asset.
- **Will not approve a fix that handles one path of a bug class.** "Sanitize this input" is not a fix for XSS; the fix is "encode at the sink." (Manico is loud about this.)

## Sources

1. *How to Think About Security in Next.js* — <https://nextjs.org/blog/security-nextjs-server-components-actions>
2. Next.js Data Security guide — <https://nextjs.org/docs/app/guides/data-security>
3. Next.js Authentication guide — <https://nextjs.org/docs/app/guides/authentication>
4. *Next.js Server Actions Security: 5 Vulnerabilities You Must Fix* — <https://makerkit.dev/blog/tutorials/secure-nextjs-server-actions>
5. Arcjet — *Next.js server action security* — <https://blog.arcjet.com/next-js-server-action-security/>
6. OWASP Top 10 (web) — <https://owasp.org/Top10/>
7. OWASP API Security Top 10 — <https://owasp.org/API-Security/>
8. OWASP Cheat Sheet Series — <https://cheatsheetseries.owasp.org/>
9. Philippe De Ryck — Talks index — <https://pragmaticwebsecurity.com/talks.html>
10. *The parts of JWT security nobody talks about* — <https://pragmaticwebsecurity.com/talks/jwtsecurity.html>
11. Drizzle SQL injection advisory (identifier escaping) — <https://github.com/drizzle-team/drizzle-orm/security/advisories/GHSA-gpj5-g38j-94v9>
12. PortSwigger Web Security Academy — <https://portswigger.net/web-security>
