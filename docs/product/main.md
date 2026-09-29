---
title: Landing — Product Definition
summary: The value, users, experience, boundaries, and capabilities that define Landing.
---

# Product definition

## User problems

AI work becomes unreliable when execution capacity, prompted behavior, and the
work itself are treated as one choice.

- Capacity is fragmented across models, providers, harnesses, and independent
  usage pools. When one route is depleted, users manually move work or stop.
- Users repeatedly choose a model even when their real intent is to choose the
  kind of work or perspective the task needs.
- Deeply prompted roles are often trapped inside one conversation or harness.
  Switching models can discard the instructions and context that made the role
  useful.
- Installing a general-purpose agent harness does not automatically give users
  a dependable way to create and reuse specialized perspectives for their own
  domain.
- A lead agent lacks a dependable way to hand bounded work into an independent,
  specialized context and receive it back for integration without carrying the
  lead conversation into every worker.
- Important decisions benefit from independent, differently prompted
  perspectives, but convening them and preserving their dissent is manual
  orchestration.

The availability-aware optimizer must remain useful without requiring the user
to adopt personas or multi-persona workflows. Those capabilities extend the
same execution layer; they do not replace its core.

## Product promise

Landing makes AI execution feel dependable and intentional. A user or lead
agent describes the work, the standard it must meet, and—when it matters—the
persona that should perform it. Landing preserves that requested behavior while
selecting an eligible execution path without requiring a provider decision for
every task.

The core thesis separates **who does the work** from **where it runs**:

- A persona is durable, deeply prompted behavior and relevant context.
- A model and harness provide execution capability and capacity.
- Routing selects an eligible execution path for the requested work.

Personas are user-defined within projects. Landing supports arbitrary
perspectives; it does not impose engineering roles or another fixed professional
taxonomy.

Landing creates value by:

- reducing repeated model and provider selection;
- using available execution capacity more intelligently;
- recovering from route exhaustion when another route in the tier is available; and
- preserving reusable prompted behavior when work moves between routes.

Availability includes independent model pools within one provider as well as
differences across providers. Cross-provider routing is supported, but it is not
the product's defining promise.

## Primary users

### Individual AI power user

Uses more than one model or harness and wants reliable execution without
manually tracking which route is suitable or available.

### Agent-assisted developer

Works through a coding agent and wants that agent to delegate appropriate work
through Landing, optionally request a specialized persona, and preserve the lead
agent's context and responsibility.

### Project owner

Wants a project to carry shared routing policy and personas and teach every
supported agent how the project uses Landing.

## Desired experience

Landing behaves like infrastructure, not another workspace to operate.

- A coding agent configures the project through Landing and reports every
  persistent change.
- Normal use starts with the work, not a vendor or model comparison.
- Users name a persona only when a specific perspective matters.
- Routing decisions stay out of the way unless the user asks for diagnosis.
- Failures are honest, bounded, and descriptive.
- Project policy travels with the project; personal access and credentials do
  not.
- Advanced customization is available without making it a prerequisite for
  first use.

Landing's health, history, and policy interface is observational and
administrative; users do not need to babysit a scheduler to complete work.

## Core capabilities

### Agent-driven configuration

A user describes the desired project policy to their coding agent. The agent
asks Landing for supported installed harnesses, authentication readiness,
remaining capacity, and known model options, then asks Landing to write the
complete validated configuration. Landing marks a non-exhaustive model list so
an agent can name a newly available model without waiting for Landing to update.
Known models are hints for naming and inference, never a closed list that
refuses a harness-qualified model.

Direct configuration authoring remains supported and Landing validates it when
read. Agent-driven configuration makes the configuration Landing writes
structurally valid without requiring an agent to construct its file format.

### Project configuration

Users configure Landing inside a project repository. All project-owned Landing
files live in `.landing/`, with configuration at `.landing/config.json` and
personas beneath it. The nearest applicable project configuration groups the
routes it wants Landing to reach into named tiers for work from the directory
where Landing is invoked. Built-in defaults offer a sensible starting grouping,
but they are not this project's grouping. When no applicable configuration
exists, Landing does not dispatch. It reports the missing configuration and the
directories searched because no tiers define what the caller's kind of work
refers to. Landing does not invent the project's grouping or spend capacity on
that guess.

### Named routing tiers

Users and agents ask for a meaningful class of execution rather than selecting a
vendor. A tier has a stable name, a free-form description of its work, and
routes that Landing chooses among in preference order.

It is not a persona or profession. A configuration can identify one tier as the
default for unnamed work. Without a default, the caller names a configured tier;
Landing does not choose one.

When the model itself is the request, a caller names a model target instead of
a tier, and Landing reports the routes available to name. [DEFERRED] A target
can name a route, an inferred known model, a harness's `latest`, `latest` across
harnesses, a tier's primary routes, or a comma-separated combination. Ordinary
work needs one resolved route; meetings can use several. This stays the
exception: a tier is what lets Landing answer to availability.

### Invoke work

Dispatching work is Landing's ordinary action: a caller supplies a prompt and
receives a result. Naming a tier is optional when the project has a default
tier. Continuing an existing thread is an option on that same ordinary
invocation when its harness supports continuity.

Landing waits until a dispatch, supported thread continuation, or
meeting round reaches its terminal state, however long that takes. A caller can
background the CLI and continue other work; waiting hours is supported. A
caller who wants a ceiling passes `--timeout`; only then does Landing stop
waiting, terminate the relevant harness work, and report `timeout`. An
ordinary-work thread remains continuable after timeout. Landing never supplies
an underlying harness with a time limit that the caller did not request.

Landing first interprets input as a command. Management commands name the
managed thing before the action, which keeps their surface predictable for a
caller that cannot ask questions. Input that does not match a command is a
prompt and Landing dispatches it.

Input that matches a command exactly is taken as that command, not as a prompt.
A caller that needs certainty supplies the prompt through a file or standard
input. This trade keeps ordinary work natural while preserving an unambiguous
path for the exceptional ambiguous input.

### Personas

Users can create, inspect, change, and remove personas that capture a purpose,
deep instructions, and relevant context. A persona is a project-owned character,
not a routing policy: it has no routes, models, harnesses, or tier affinity.
Personas can represent any perspective the user needs. Landing does not require
or privilege an engineer hierarchy.

When a user or lead agent gives bounded work to a requested persona, Landing
preserves that exact perspective in an independent context and returns the
result for review and integration. The persona remains stable when the selected
execution route changes; Landing never silently drops, exchanges, or ignores
it. Work either runs as the requested persona or reports a failure.

Lead and worker describe responsibilities within one delegation, not a permanent
hierarchy based on model size, cost, or status.

### Meetings

For a consequential question, a lead agent can convene a meeting: one round of
a deliberation protocol. It names an arbiter and at least two participants
besides it. [DEFERRED] A seat is one persona on one route, so a persona may
hold several seats and offer the same perspective through several models. A cast
resolves a model target into seats; the arbiter may be cast to exactly one route.
Landing gives every participant seat a clean, independent context with the
question and its own material, then returns every position and the arbiter's
reading of genuine conflict to the lead agent. An arbiter is a persona with a
perspective, not a neutral position, and may also participate through a separate
instance and context.

[DEFERRED] Landing validates every distinct participant and arbiter route before it
dispatches a seat. An invalid or unverified route stops the meeting before it
spends on participant work. A seat that becomes unavailable after validation is
reported, never dropped; the meeting continues with the positions that answer
when at least two seats answer.

The lead agent decides whether to convene another round, carrying prior
positions and the arbiter's reading verbatim, or to ask the arbiter for a
synthesis as ordinary work. This preserves judgment where it belongs and keeps
Landing from manufacturing consensus or reducing disagreement to a structured
verdict. A meeting uses one tier and ordinary availability-driven routing, with
an optional model target for a particular participant or the arbiter that
belongs to the meeting alone and can name a route outside the tier. Participants
retain ordinary work capabilities. Meetings use several times the capacity of
one dispatch, and Landing does not cap what they spend.

A meeting is requested explicitly. Naming several personas for ordinary work
does not create one. Meetings use several times the capacity of one dispatch
and return a different kind of result, so they remain an optional workflow.

### Policy-driven route selection

Landing chooses among the routes in the requested tier using known capability,
availability, preference, and failure state. Quality, latency, cost, and context
requirements are valid policy inputs as the product can measure them honestly.
[DEFERRED] `latest` names Landing's current frontier default for each harness with one;
projects can override that choice per harness, and the resolved route is
reported. Model hints help Landing resolve known short names, but do not refuse
an unfamiliar harness-qualified model.

A request that names its own target skips this selection. [DEFERRED] Landing validates each
resolved route through live provider evidence or a minimal probe, then runs it
or reports it as valid, invalid, or unverified. An unverified route does not
dispatch, and Landing never substitutes another. The small validation probe is
the confined exception to Landing's non-spending capacity checks.

### Bounded recovery

When a route is exhausted, Landing can mark it temporarily unavailable and try
another route in the tier. Recovery remains bounded and preserves the original
failure for diagnosis.

### Agent enablement

Setup makes the project policy discoverable to supported agents so the user's
agents know Landing exists, which tiers and personas are available, and how
returned work must be checked.

### Agent communication

Agents in the same project can see active participants and exchange durable,
attributed messages across supported harnesses. Landing distinguishes a live
session from a resumable dispatched thread, delivers at each harness's honest
boundaries, and states delivery latency or limitations rather than implying
shared live context. Communication coordinates work; it is neither a lock nor
an authorization mechanism. [Agent communication](comms.md) owns this
experience.

### Observable results

Results identify the execution route when that information helps diagnosis.
Detailed scoring and route tie-breaking remain operational information rather
than noise in every response.

## Product boundaries

The product is local-first. It uses user-installed supported harnesses
and lets those tools own authentication. Landing does not collect consumer
credentials, imitate official clients, or use capacity checks that spend or
mutate provider resources.

Landing is not:

- a promise to eliminate all usage limits;
- a product for pooling or bypassing consumer subscriptions;
- a universal shared-memory layer across unrelated harnesses;
- a fixed catalog of engineering or professional roles;
- a guarantee that multiple perspectives will agree;
- a durable background job queue; or
- a full agent workspace.

Landing is not a commercial API router, shared context-continuity layer, or
hosted control plane.

## Product principles

- Ask for the work, not the vendor.
- Separate prompted behavior from execution capacity.
- Optimize execution, not model novelty.
- Provide a useful default before advanced configuration.
- Keep policy and agent context aligned.
- Make decisions observable without making users operate the router.
- Degrade honestly when capability or capacity is unknown.
- Accept new model names without waiting for a catalog update, and verify them
  before work runs.
- Treat every message as model-readable context: state what Landing observed
  concisely and descriptively without prescribing a recovery.
- Preserve meaningful disagreement; do not manufacture consensus.
- Prefer official integration surfaces and preserve user control.

## Founding decisions

- The working category is **AI execution optimizer**.
- Optimization is the value; orchestration and routing are mechanisms.
- The system runs beneath the user's normal workflow.
- Routing tiers describe execution contracts; personas describe prompted
  behavior and context.
- Persona names and purposes are user-defined, not a product-owned role system.
- A meeting is one deliberation round over the same routing foundation; the lead
  agent runs the ongoing protocol, not Landing.
- An arbiter is a meeting role played by a persona with a perspective; it is
  not a neutral synthesis mechanism.
- Landing configuration belongs to a project; it does not inherit a
  user-machine policy.
- Agent-driven configuration and direct configuration authoring are supported
  paths to the project's complete policy.
- Landing does not dispatch when it cannot resolve project configuration.
- Management commands name the managed thing before the action; unmatched input
  is work to dispatch.
- Agent-assisted onboarding is a first-class path.
- Public messaging leads with execution optimization, not subscription usage.
