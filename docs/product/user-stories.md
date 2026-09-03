---
title: Landing — User Stories
summary: The user outcomes and acceptance expectations that shape Landing.
---

# User stories

These stories describe product outcomes, not implementation tasks. They are the
primary lens for deciding whether a feature makes Landing easier and more
dependable to use.

## Start using Landing

### Reach a useful default

As a new user, I want my coding agent to learn the harnesses and models Landing
can route to and configure a working policy through Landing so that I can begin
without designing a routing system or hand-writing configuration.

Landing validates the complete policy before writing it and reports the enabled
routes, tiers, and default tier after the change.

### Configure the current project

As a user, I want to initialize Landing in my current project so that its
routing policy and personas travel with the repository.

Landing shows the resolved project configuration file.

When no project configuration can be resolved, Landing does not dispatch. No
tiers define what the requested kind of work refers to, so Landing reports the
missing configuration and the directories it searched instead of inventing a
project grouping or spending capacity on a guess.

### Author project configuration directly

As a project owner, I want to author and edit the complete project configuration
directly so that I can express the routing policy through a surface my tools and
agents can use naturally.

Direct authoring is a supported path alongside agent-driven configuration. Both
paths establish the same project policy, which does not inherit user-machine
configuration.

### Use an existing authenticated harness

As a local user, I want Landing to use supported tooling I already installed and
authenticated so that I do not give Landing my provider credentials.

If a harness is not ready, setup identifies the official authentication flow
that owns its authentication state.

### Let my agent perform setup

As a new user, I want to describe the execution policy I need to my existing
agent so that it can inspect Landing's routable options, configure my current
project, and work from the current project policy.

The agent gathers the policy choices through our normal conversation, then uses
complete Landing requests. It summarizes the resulting change and verifies
readiness without spending model capacity merely to test configuration.

The agent can author or edit the project configuration directly when that is an
appropriate way to establish the user's policy.

## Configure the policy

### Choose participating models

As a user, I want to group detected connections and models into tiers so that
routing reflects the execution needs and available choices in my project.

A route outside a tier is not prohibited; Landing simply does not choose among
it for work requested through that tier, and a caller can still name it for one
request.

### Accept a neutral default policy

As a user, I want my coding agent to create a neutral routing policy from the
routes Landing can use so that I have useful choices without learning policy
terminology first.

The agent can use detailed tier descriptions, route lists, and explicit route
ordering. Tiers describe execution needs rather than assuming I use a particular
set of professions or personas.

### Add a custom tier

As a user, I want to name and describe another class of work and choose the
routes Landing chooses among for it in preference order so that my agents can
request execution suited to my project.

After I add or change a tier, Landing validates it, refreshes the corresponding
project policy, and reports the resulting state so that configuration and
prompts do not drift apart.

### Set a default tier

As a project owner, I want to name a default tier for unnamed work so that my
agents can dispatch ordinary work without repeating a tier choice.

When no default tier exists, Landing requires the caller to name one of the
configured tiers. It does not infer a tier from the task.

### Set project policy

As a project owner, I want to define the tiers for my project so that the
repository expresses its execution needs without copying credentials or
machine-specific account state.

I can see the resolved project configuration file that supplies the policy.

## Create and manage personas

### Create a reusable persona

As a user, I want to create a persona with a purpose, deep instructions, and
relevant context so that I can reuse a specialized perspective without rebuilding
its prompt in every conversation.

Landing keeps the persona in its own project directory, with its definition and
optional reference material. I can inspect, change, and remove the persona.
Landing does not require it to match a built-in engineer hierarchy or other
professional taxonomy.

### Define a persona for a project

As a user, I want to define a persona for my project so that its reusable
perspective, expectations, and relevant material travel with the repository.

Landing owns the persona as a project resource; it does not read or register a
persona supplied by another tool. The project does not carry my credentials or
machine-specific account state.

### Keep persona behavior separate from the model

As a user, I want a persona's instructions and context to remain stable when its
execution route changes so that availability recovery does not change who I
asked to do the work.

Diagnostics distinguish the requested persona from the model and harness that
executed it.

## Route everyday work

### Delegate without choosing a vendor

As a user or lead agent, I want to describe the work and its routing tier so that
Landing can choose an appropriate path without making me compare providers.

The result meets the tier's work contract and identifies where it ran when that
information is useful.

### Delegate to a persona

As a user or lead agent, I want to send work to a named persona so that the task
receives the right prompted perspective without requiring me to select its
model.

Landing runs the persona in a clean context with the delegated brief and the
persona's relevant material. If it cannot run the requested persona, it reports
the failure and does not return generic unprompted work.

The delegated worker receives bounded responsibility; the lead agent retains
responsibility for reviewing and integrating the result. These responsibilities
do not assume that one agent is larger, smaller, more senior, or tied to a
particular model.

### Name the route for this work

As a user or lead agent, I want to name the harness and model for one request so
that work whose point is a particular model runs on it.

Landing runs the named route without ranking it against alternatives and without
falling back, because substituting another model would discard the reason I
named this one. I can name any route my machine can reach, whether or not my
project's tiers use it, and I do not have to add a tier to make one request. If
the route cannot run the work, I get a failure that says so. When the harness
can enumerate models, Landing catches an unknown name before work starts;
otherwise the harness remains the authority for that decision.

### See the routes I can name

As a user or lead agent, I want to see the routes available to me, written the
way I would name them, so that I can choose one without guessing.

The report covers known model options and what my project configures, and says
which tiers use each route, so I can tell an established choice from an unused
one. It marks a non-exhaustive model list, so I know when the harness may accept
a model that Landing cannot list. It answers before my project has any policy,
and reading it does not spend capacity.

### Use current availability

As a user, I want Landing to consider current capacity so that a depleted route
does not force me to move work manually.

This includes independent model capacity pools within a single provider. When
capacity cannot be read, Landing says it is unknown rather than inventing a
number.

### Recover from exhaustion

As a user, I want Landing to try another route in the same tier after confirmed
exhaustion so that a temporary limit does not always become manual recovery.

Retry is bounded. If no suitable route remains, I receive a clear failure that
preserves the reason the task did not complete.

### Keep follow-up context

As a user, I want to continue completed work in its existing conversation when
the selected harness supports it.

If continuity is unavailable, Landing says so rather than pretending a fresh
conversation contains the prior context.

## Meet across personas

### Choose the participants for this task

As a user, I want to select several relevant personas when I convene a meeting
so that the perspectives fit the decision rather than a fixed product-owned
panel.

Landing does not require the participants to be engineers or to come from a
built-in catalog. Each participant retains its requested perspective regardless
of the route selected for its work. Naming several personas for ordinary work
does not convene a meeting. A meeting requires an arbiter and at least two
participants besides it.

### Name an arbiter with a perspective

As a user, I want to name one persona as arbiter so that the meeting has a
defined perspective to read the positions and identify genuine disagreement.

I can also seat the arbiter as a participant. Landing runs those responsibilities
in separate contexts, so the arbiter can advance its own position without its
participant instance influencing its judgment.

### Receive one independent round

As a user, I want each participant to begin with a clean context containing the
question and that persona's material, and for the arbiter to receive every
position, so that the room considers independent perspectives before anyone
interprets them.

Landing returns every position and the arbiter's reading to my lead agent. The
arbiter identifies where positions genuinely conflict and what the disagreement
turns on; differences of emphasis or coverage alone do not count as conflict.

### Direct the deliberation

As a user, I want my lead agent to decide whether another round would help so
that the deliberation follows the judgment available in the work rather than a
product-owned stopping rule.

When another round is useful, my lead agent carries the positions so far and
the arbiter's reading verbatim into its question. Landing does not decide
whether to continue, summarize positions in transit, or require a structured
response format.

### Receive a meeting synthesis

As a user, I want my lead agent to ask the arbiter for a synthesis after it
judges the deliberation complete, so that I can act without mistaking dissent
for agreement.

The synthesis is ordinary work. It can recommend or decide, explain resolved
disagreement, and retain unresolved positions.

Landing uses the meeting's tier to route the arbiter and every participant I do
not cast through the ordinary availability-driven policy. I can cast a
particular participant to any route Landing can reach through an installed
supported harness, even when that route is outside the meeting's tier. Landing
validates the harness and model, and the route belongs to this meeting rather
than to the persona. A meeting does not restrict participants to reading: they
can investigate, read, and run commands as ordinary Landing work can. Landing
does not cap what the meeting spends.

## Stay in control

### See other agents at work

As an agent working in a project, I want to see the other project participants,
their harnesses, kinds, and activity so that I can coordinate with the work
already in progress instead of acting as though I am alone.

Landing distinguishes live sessions from resumable dispatched threads and states
the relevant delivery limitation. A named live session also exposes its process
identifier, working directory, and start time; Landing does not stop it.

### Ask another agent a question

As an agent, I want to send a question to a named participant and request a
reply when it matters so that I can get information from the context already
doing the work.

Landing waits indefinitely by default for a response-required question, so I
can background the CLI and continue other work while I wait for a considered
answer. A question to a thread resumes it as a dispatch and returns its answer
synchronously; that answer can take many minutes. I can choose a maximum wait
when I need one. Reaching it is not an error: my question remains durable and
its eventual answer arrives in my inbox.

### Tell another agent something

As an agent, I want to send a durable message to one participant or every
participant in this project so that information does not depend on shared
terminal state or one harness.

Landing attributes and timestamps the message, delivers it once at the
recipient harness's available boundary, and retains it in bounded recent
history. It states when a harness cannot put an arriving message into a running
agent's context.

### Announce shared-state work

As an agent, I want to announce work that affects shared state so that other
agents can judge whether their current work conflicts with it.

An announcement is a durable coordination message, not a claim, lock,
permission, or enforcement mechanism. It carries the author, time, and reason
for recipients to evaluate; Landing does not require a release action.

### Understand a surprising result

As a user, I want to see the selected harness and model when diagnosing a result
so that I can distinguish a routing issue from a brief or capability issue.

Normal responses remain concise; detailed route reasoning is available through
diagnostics rather than added to every result.

### Observe configuration changes

As a user, I want Landing to report the project configuration, enabled routes,
changed tiers, and available personas after a policy change so that I remain in
control of my environment.

An invalid request reports why it cannot be applied and the routable options
that resolve its route error. A valid request updates the existing
Landing-owned project resources without creating duplicates.

### Remove Landing cleanly

As a user, I want Landing to remove only the `.landing/` project resources it
owns so that uninstalling it does not damage my instructions.

Ambiguous project ownership causes Landing to stop and ask rather than rewriting
surrounding content.

## Maintainer story

### Extend a supported execution path

As a maintainer, I want a new harness integration to satisfy a stable system
contract so that product behavior and callers do not become vendor-specific.

Provider-specific authentication, capacity interpretation, requests, results,
and exhaustion signals remain behind that integration boundary.
