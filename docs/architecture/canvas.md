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
Direct configuration ----------------> Configuration authoring
Configuration resolver -------------> Agent guidance
Configuration resolver -------------> Persona library -> Execution runtime
Persona request --------------------> Persona library
Meeting request --------------------> Meeting round convener -> Persona library
Meeting round convener -- parallel participant dispatches --> Execution runtime
Execution runtime -- participant positions --> Meeting round convener
Meeting round convener -- arbiter reading --> Execution runtime
Execution runtime -- arbiter reading --> Meeting round convener
Execution runtime ------------------> Local state
```

## Components

### Route discovery

Reports the harnesses Landing can route through on this machine, their
authentication readiness, their known remaining capacity, and their reachable
models. It reports only supported, routable choices so an agent can compose a
valid tier without inferring provider capabilities that Landing cannot use.

### Configuration authoring

Accepts a complete caller-supplied policy and validates every named harness,
model, and route against Route discovery before it writes project configuration
or generated agent guidance. It treats agent-supplied detail as intentional
rather than requiring abbreviated input. Direct configuration remains a
supported input to Configuration resolver and receives validation when read.

### Configuration resolver

Reads the nearest applicable project configuration by searching upward from the
invocation directory, then combines it with built-in defaults into one validated
view. Its policy precedence is exactly built-in defaults, then the project
configuration. Project configuration lives at `.landing/config.json`; all
project-owned Landing material lives beneath that same `.landing/` directory.
Absent project configuration is a terminal error that identifies the required
configuration and directories searched. Without project configuration, Landing
has no project tiers to route within: built-in defaults describe a sensible
starting grouping, not this project's grouping, and Landing does not invent
that grouping or spend capacity on the guess. Diagnostics identify the resolved
configuration, without carrying provider credentials into the project. It
validates that a declared default tier names a configured tier.

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

Owns one explicitly requested meeting round. It resolves the caller-selected
participants and arbiter, starts every participant in parallel with clean
contexts, then gives their positions to the arbiter for a separate reading. It
returns every participant position and the arbiter's prose reading. It does not
interpret a position, decide whether deliberation continues, or request a
synthesis. The lead agent carries positions and the arbiter's flagged conflicts
into any further round verbatim, decides when deliberation is finished, and
requests synthesis as ordinary work. This boundary remains beside Execution
runtime because the convener coordinates the fixed two-stage round while the
runtime owns each dispatch and its terminal result.

### Harness adapters

Translate the shared execution contract into supported harness behavior. They
contain provider-specific authentication boundaries, requests, results,
capacity interpretation, and failure classification.

### Agent guidance

Teaches supported local agents which tiers exist, what their descriptions
express, and when to use them. Guidance is generated from resolved policy and
written only inside bounded owned regions.

### Local state

Retains only operational facts needed for diagnosis, temporary route cooling,
and continuation identity. Provider credentials remain owned by their official
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
- A meeting round selects one tier for the participant routes Landing chooses
  and for the arbiter route. Those routes come from the tier's ordinary policy.
- A caller-selected route for a participant may be outside the meeting tier but
  must name a supported installed harness and a model reachable through it. It
  is scoped to that round; it does not become persona routing policy or affect
  another round.
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
- Generated agent guidance and resolved tier policy cannot evolve independently.
- Configuration resolution has exactly two policy layers: built-in defaults,
  then the nearest applicable project configuration.
- Configuration and personas are project-owned material beneath one
  `.landing/` directory. Machine-local operational state stays outside the
  project.
- Configuration is one project-owned policy surface. It supports direct
  authoring and validated authoring through Landing.
- Absent configuration terminates dispatch because Landing has no project tiers
  to route within. Built-in defaults are a sensible starting grouping, not a
  substitute for the project's grouping, so Landing does not spend capacity on
  an invented grouping.
- Configuration never stores raw provider credentials.
- Every emitted message concisely describes observed state rather than
  prescribing recovery.
- Every persistent write has a visible owner and a safe replacement boundary.
