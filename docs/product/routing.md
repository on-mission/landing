---
title: Landing — Routing
summary: The user-facing policy for route selection, persona guarantees, availability, and customization.
---

# Routing

Routing is a product policy: the caller names the kind of work, and Landing
chooses a route from that tier.

When the caller names a persona, Landing also preserves that prompted
perspective. Provider and model selection are normally an implementation detail
rather than a recurring user decision.

## Work, personas, and execution

Landing treats three concerns independently:

- The task describes what must be accomplished.
- A persona describes the durable behavior and context that should shape the
  work.
- A route identifies the model and harness that provide execution capacity.

A caller can request a task without a persona. When a persona is requested, it
does not affect route selection: Landing selects from the requested tier's
routes. Changing routes does not silently change or remove the
requested persona.

## Routing tiers

A tier is a named policy for a kind of work. Tiers are semantic profiles, not
necessarily a ladder from weakest to strongest. Two task classes may require
different capabilities without one being above or below the other.

Each tier expresses:

- a stable name;
- a free-form description of the work it is for, including any boundaries the
  author wants agents to consider;
- model and harness routes that Landing chooses among in declared preference
  order; and
- fallback treatment for a route held in reserve.

A model may participate in more than one tier and may be treated differently in
each because the work contract differs.

## Default tier

Each configuration can designate one configured tier as its default. A caller
may name a tier for a work request. When the caller does not name one, Landing
uses the configured default tier. When neither is present, Landing fails and
reports the configured tiers. Routing selects a route within a tier; it never
selects a tier on the caller's behalf.

A coding agent can create a small neutral policy that distinguishes work by
execution needs from Landing's current routable options. Tiers are not named
after professions and do not imply a built-in persona catalog. A project owner
can change, rename, remove, or supplement them. Product documentation does not
hard-code model names.

## Selection policy

Landing considers only routes in the requested tier. Within that set,
it uses known signals such as:

- capability for the requested work;
- normalized remaining availability;
- declared preference or fallback treatment; and
- temporary unavailability after a confirmed failure.

Quality, latency, cost, and context requirements can become policy signals when
Landing has a trustworthy way to represent them. Unknown information remains
unknown; the product does not fabricate precision.

Independent capacity pools are treated independently, including separate model
buckets inside one provider. When one allowance is constrained by several time
windows, the tightest known window determines its useful headroom.

## Personas

A persona is a project-defined character: a durable prompted perspective with
its own instructions and, when it has one, its own body of reference material.
Any perspective useful to a project is a valid persona. Engineering roles can
be useful examples, but they are not privileged product concepts.

Landing owns personas. Each persona lives in its own project directory with its
definition and optional reference material. Landing applies the persona's
definition to its work and makes that material available, so a user does not
need to copy a large body of reference into every prompt.

A persona has no routes, models, harnesses, or tier affinity. The requested
tier selects the execution path; the route does not alter the persona.

When a caller requests a persona, Landing preserves that exact perspective:

- Landing never silently drops, exchanges, or ignores the requested persona.
- The persona's instructions and context remain unchanged when recovery moves
  work to another route.
- Diagnostics distinguish the requested persona from the harness and model that
  served it.

Work either runs as the requested persona or reports a failure. Landing never
returns generic unprompted work in place of the requested perspective.

## Unknown availability

Unknown availability is neither zero nor unlimited.

Capacity reads improve route selection; they do not prevent execution. An
unreadable route ranks below comparable measured routes rather than being
disabled.

- A route with measured availability is preferred over an otherwise comparable
  route whose availability is unknown.
- If all routes in the tier are unknown, Landing follows the tier's declared
  order and lets execution provide the next signal.
- User-facing diagnostics distinguish unknown capacity from measured headroom.

This keeps Landing honest without making an unreadable gauge disable all work.

## Exhaustion and recovery

When an integration confirms exhaustion, Landing records the failure, marks the
route temporarily unavailable, and may retry on another route in the tier.

Retry is bounded. A malformed task does not tour every provider, and exhaustion
never becomes an empty success. Temporary route marks expire so stale state does
not permanently block execution.

A continued conversation is not newly routed. It remains on the harness and
model that own its context; otherwise Landing says that continuation is not
available.

## Meetings

A meeting is one round of a protocol run by the lead agent already
orchestrating the work. The lead agent names an arbiter and at least two
participants besides it. There is no standing panel or product-owned roster.
Naming several personas for ordinary work does not convene a meeting: a meeting
uses several times the capacity of one dispatch and returns a different kind of
result.

The arbiter is a required persona with its own perspective, not a neutral
position. It may also be seated as a participant. Landing runs its participant
and arbiter responsibilities as separate instances with separate contexts, so
the arbiter can hold a position and then read the room without either instance
having access to the other's work.

Landing puts the question to every participant in parallel. Each begins in a
clean context containing the question and that persona's own material. Landing
then puts every position to the arbiter. The arbiter reads them and reports
where positions genuinely conflict and what those disagreements turn on.
Landing returns every position and the arbiter's reading to the lead agent.

The lead agent decides whether the deliberation is complete. When another round
is useful, it convenes one with a question that carries the positions so far,
verbatim, and the arbiter's reading. When the deliberation is complete, the lead
agent asks the arbiter for a synthesis as ordinary work. The synthesis can
recommend or decide, explain resolved disagreement, and retain unresolved
positions so the user can act without mistaking dissent for agreement.

Landing does not decide whether positions conflict, whether another round is
useful, or when deliberation is complete. It does not summarize positions while
passing them between personas or require a structured response format. Every
prompt and response is prose.

Landing describes this protocol in its own current help. Project standing agent
context names the meeting capability and directs agents to that help instead of
restating the protocol.

A meeting has one tier. Landing uses that tier's ordinary availability-driven
policy for the arbiter and every participant the caller does not cast. A caller
may cast a particular participant to any route Landing can reach through an
installed supported harness, whether or not that route appears in the meeting's
tier. Landing validates that the harness is supported and installed and that it
can reach the selected model. The cast route belongs to the meeting, not to the
persona: personas have no routes, models, harnesses, or tier affinity. A tier
defines the routes Landing chooses among; it does not constrain a caller's
specific route choice. Participants retain the capabilities of ordinary Landing
work, including investigating, reading, and running commands. Landing does not
cap a meeting's spend; the caller chooses a workflow that costs several times
one dispatch.

## Custom tiers

Users can create a tier by naming the kind of work, writing its description in
their own words, choosing the model and harness routes Landing chooses among in
their preferred order, and marking any reserve route for fallback treatment.
Landing validates that the tier has at least one routable route before it writes
the change and reports the resulting policy.

Adding or changing a tier updates the project's routing configuration. Removing
a tier leaves unrelated human-authored instructions untouched.

## Project configuration

The nearest applicable project configuration, found by searching upward from
the directory where Landing is invoked, defines the tiers for that project's
work. Landing keeps all project-owned files in `.landing/`: configuration lives
at `.landing/config.json`, and personas live beneath that directory. Built-in
defaults offer a sensible starting grouping, but they do not supply this
project's tiers. If Landing cannot resolve project configuration, it does not
dispatch and reports the missing configuration and the directories searched. No
tiers identify the routes to choose among for the caller's kind of work, so
Landing does not invent the project's grouping or spend capacity on that guess.

Project policy does not inherit a user-machine configuration.

Project policy references available local connections without storing raw
credentials, cookies, or tokens in the project. Machine-local operational state
remains outside the project.

## Safety and visibility

- Capacity reads never spend credits or mutate provider state.
- Authentication stays in supported official tooling or official APIs.
- A route with unreadable capacity reports unknown.
- A tier with no route is invalid.
- An unnamed work request requires a configured default tier.
- A requested persona is never replaced with another perspective or unprompted
  work.
- Results preserve failures and identify the selected route when useful.
- Detailed scoring and tie-breaking remain diagnostic information.
