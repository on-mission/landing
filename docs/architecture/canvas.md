---
title: Landing — Architecture Canvas
summary: The major components, contracts, and invariants of Landing, including project-owned personas and meetings, at the system level.
---

# Architecture canvas

Landing is a local-first execution layer between a user or lead agent and the AI
harnesses available in that user's environment.

## System flow

```text
Harness adapters -> Route discovery -> Configuration authoring -> Configuration resolver -> Tier policy -> Router -> Execution runtime -> Harness adapters
Direct configuration ----------------> Configuration resolver
Configuration authoring -- concrete models --> Model target resolution and validation
Model target resolution and validation -- validation results --> Configuration authoring
Configuration resolver -------------> Agent guidance
Configuration resolver -------------> Persona library -> Execution runtime
Caller model target + Configuration resolver + Route discovery --> Model target resolution and validation -> Execution runtime
Persona request --------------------> Persona library
Meeting request --------------------> Meeting round convener -> Persona library
Meeting round convener -- parallel seat dispatches --> Execution runtime
Execution runtime -- participant positions --> Meeting round convener
Meeting round convener -- arbiter reading --> Execution runtime
Execution runtime -- arbiter reading --> Meeting round convener
Model target resolution and validation --------------------------> Local state
Execution runtime ----------------------------------------------> Local state
```

## Components

### Route discovery

Reports the harnesses Landing can route through on this machine, their
authentication readiness, their known remaining capacity, and their model
hints. Hints identify known models, short names, and a harness's shipped
`latest` default; they help callers discover and resolve routes, but never
prove that another model cannot run.

### Configuration authoring

Accepts a complete caller-supplied policy and validates its configuration shape
and named harnesses before it writes project configuration or generated agent
guidance. It supplies every concrete tier-route model and project `latest`
override to Model target resolution and validation, and refuses to write when
either is `invalid` or `unverified`. A tier route written as `latest` is not a
concrete model at write time; it resolves when the route is used. It treats
agent-supplied detail as intentional rather than requiring abbreviated input.

### Configuration resolver

Reads the nearest applicable project configuration by searching upward from the
invocation directory, then combines it with built-in defaults into one validated
view. Its policy precedence is exactly built-in defaults, then the project
configuration. The project owns its tiers and optional per-harness `latest`
overrides; Landing owns the fallback `latest` defaults. A tier route may name
`latest`, which becomes a concrete route only during target resolution. Project
configuration lives at `.landing/config.json`; all project-owned Landing
material lives beneath that same `.landing/` directory.
Absent project configuration is a terminal error that identifies the required
configuration and directories searched. Without project configuration, Landing
has no project tiers to route within: built-in defaults describe a sensible
starting grouping, not this project's grouping, and Landing does not invent
that grouping or spend capacity on the guess. Diagnostics identify the resolved
configuration, without carrying provider credentials into the project. It
validates that a declared default tier names a configured tier.

Direct configuration receives shape validation when Configuration resolver
reads it, without a model probe. Its concrete models validate when their routes
are used.

### Model target resolution and validation [DEFERRED]

Turns every caller-written model target, and every concrete model Configuration
authoring supplies, into one or more concrete routes before it is accepted for
use. It resolves an explicit harness/model to that route; an explicit
harness/latest to that harness's latest; `latest` to every harness with a
defined latest; a tier to its primary routes; a comma list to the deduplicated
union of its targets; and a bare model through exactly one adapter's hints.
Reserve routes are not tier targets. It consults project tiers, project
`latest` overrides, adapter-shipped `latest` defaults, and adapter hints. A
bare word matching more than one namespace is an error rather than a guess; an
unknown bare model also requires an explicit harness. A target that must select
one route, including ordinary work and an arbiter, fails if it resolves to more
than one.

Validates every concrete route through the adapter that owns its harness. The
adapter returns `valid`, `invalid`, or `unverified` with its evidence. An
adapter uses its live model listing when it has one; otherwise it minimally
probes exactly the selected model. A provider's definitive rejection is
`invalid`; a timeout, rate limit, or other inconclusive failure is
`unverified`, and neither result dispatches work. Each adapter recognizes its
own unknown-model rejection, including the nonzero rejection behavior observed
for Claude, Codex, Grok, and Cline, so a successful probe proves the selected
model was not silently substituted. Only `valid` results are retained, per
harness and model, for one day. Model validation covers direct work, casts,
arbiters, tier routes, and project `latest` overrides.

### Persona library

Owns the project's personas as durable characters. Each persona has its own
directory beneath `.landing/`, containing its definition and, when applicable,
the reference material belonging to that perspective. It makes the requested
persona's unchanged instructions and material available to Execution runtime.
It contains no routing policy, route eligibility, model choice, harness choice,
or tier affinity: any selected tier may execute the same persona.

### Tier policy

Groups routes into named classes of work through one author-written description
and a declared preference order, including routes held in reserve as fallback.
It identifies the configuration's default tier for work whose caller omits a
tier. A route absent from a tier is not a candidate for that routing choice; it
is not forbidden. Tier policy is user-facing product policy rather than
provider-specific logic.

### Router

Selects an eligible route using normalized policy signals and temporary failure
state. It never needs the caller to understand provider metering details.

### Execution runtime

Owns one dispatch from invocation through a terminal result, including bounded
retry and honest thread continuation. It is not a durable background queue.

### Meeting round convener

Owns one explicitly requested meeting round. [DEFERRED] Its seat and preflight
contract treats a seat—one persona on one concrete route—as its unit of
dispatch, reporting, and handoff to the arbiter; the same persona may occupy
several seats. It resolves the caller-selected participants, seats, and
arbiter, validates every distinct seat and arbiter route in parallel before
dispatching any seat, then starts all seats in parallel with clean contexts. It
gives the arbiter every completed position verbatim for a separate reading and
returns every seat outcome and the arbiter's prose reading. A harness that
becomes unavailable after validation is a visible failed seat; the round
proceeds when at least two seats answer. It does not interpret a position,
decide whether deliberation continues, or request a synthesis. The lead agent
carries positions and the arbiter's flagged conflicts into any further round
verbatim, decides when deliberation is finished, and requests synthesis as
ordinary work. This boundary remains beside Execution runtime because the
convener coordinates the fixed two-stage round while the runtime owns each
dispatch and its terminal result.

### Harness adapters

Translate the shared execution contract into supported harness behavior. They
contain provider-specific authentication boundaries, requests, results,
capacity interpretation, failure classification, model hints, and model
validation evidence. Every adapter's model knowledge is advisory: it supports
inference, short names, and `latest`, but no catalog refuses a model. The
adapter alone recognizes its harness's definitive unknown-model rejection.

### Agent guidance

Teaches supported local agents which tiers exist, what their descriptions
express, and when to use them. Guidance is generated from resolved policy and
written only inside bounded owned regions.

### Local state

Retains only operational facts needed for diagnosis, temporary route cooling,
continuation identity, and one-day successful model validations. A thread
records the concrete route that answered, so continuation stays on that route
after `latest` changes. Provider credentials remain owned by their official
tooling.

## Contracts and invariants

- A dispatch selects exactly one configured tier before routing: a caller-named
  tier takes precedence; otherwise the configured default applies. If neither
  supplies a tier, dispatch terminates and reports the configured tiers.
- Routing selects a route only among those grouped in the selected tier; it
  never selects a tier on the caller's behalf.
- Routing selects from tier policy without considering a requested persona.
- A requested persona runs unchanged or dispatch terminates; it is never
  silently dropped, exchanged, or ignored. Recovery to another route preserves
  that persona's instructions and context.
- Persona diagnostics distinguish the requested character from the harness and
  model that execute it.
- A meeting is requested explicitly. Naming several personas on an ordinary
  dispatch does not convene a meeting.
- A meeting round requires one caller-selected arbiter and at least two
  caller-selected participants besides it. Landing has no standing panel or
  product-owned roster. The arbiter is a persona with its own perspective, not
  a neutral position.
- Each participant receives only the question and that persona's own material.
  Participants are isolated from one another until their positions return.
- An arbiter who is also a participant runs as two separate instances with
  separate contexts. Neither instance receives the other instance's work.
- A meeting's seat belongs to that round, not to its persona. A caller-selected
  seat route may be outside the meeting tier; it is scoped to that round and
  does not become persona routing policy or affect another round.
- A meeting validates every distinct concrete seat and arbiter route in
  parallel before it dispatches a seat. `invalid` and `unverified` routes stop
  the round before work spends capacity.
- Participant dispatches retain the ordinary capabilities of Execution runtime;
  a meeting round does not restrict them to reading.
- The arbiter receives every participant position verbatim. Landing never
  summarizes, paraphrases, or otherwise interprets a position in transit.
- Every round prompt and result is prose. Landing never asks a persona for a
  machine-readable response format.
- Landing convenes exactly one round. The lead agent judges whether positions
  conflict, whether another round is useful, and when deliberation is finished.
- A round returns every participant outcome and the arbiter's reading. A failed
  participant remains visible as failed; Landing never renders a missing
  position as agreement.
- Product policy is provider-neutral; provider-specific behavior stays behind an
  adapter boundary.
- Capacity checks are read-only and unknown capacity remains explicitly unknown.
- Retry is bounded and preserves the original execution failure.
- A continuation stays on the route that owns its context.
- A continuation records and uses its concrete route, never a moving `latest`
  target.
- Generated agent guidance and resolved tier policy cannot evolve independently.
- Configuration resolution has exactly two policy layers: built-in defaults,
  then the nearest applicable project configuration.
- Configuration and personas are project-owned material beneath one
  `.landing/` directory. Machine-local operational state stays outside the
  project.
- Configuration is one project-owned policy surface. It supports direct
  authoring and validated authoring through Landing.
- Configuration authoring refuses to write a concrete model that is `invalid`
  or `unverified`; direct configuration read validates shape without probing and
  defers concrete-model validation until its route is used.
- Absent configuration terminates dispatch because Landing has no project tiers
  to route within. Built-in defaults are a sensible starting grouping, not a
  substitute for the project's grouping, so Landing does not spend capacity on
  an invented grouping.
- Configuration never stores raw provider credentials.
- Project configuration may override `latest` per harness, and tier routes may
  name `latest`; Landing supplies defaults for harnesses without an override.
- Model hints are never a model-acceptance gate. Every concrete model is
  validated by its owning adapter before Configuration authoring writes it or
  work runs it, and only successful validations are cached.
- Every emitted message concisely describes observed state rather than
  prescribing recovery.
- Every persistent write has a visible owner and a safe replacement boundary.
