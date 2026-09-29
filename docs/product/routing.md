---
title: Landing — Routing
summary: The user-facing policy for route selection, persona guarantees, availability, and customization.
---

# Routing

Routing is a product policy: the caller names the kind of work, and Landing
chooses a route from that tier. When the model is the point of the request
rather than a detail of it, the caller names the route instead, and Landing runs
it.

When the caller names a persona, Landing also preserves that prompted
perspective. Provider and model selection are normally an implementation detail
rather than a recurring user decision. A caller who has a reason to make that
decision is not required to turn it into project policy first.

## Work, personas, and execution

Landing treats three concerns independently:

- The task describes what must be accomplished.
- A persona describes the durable behavior and context that should shape the
  work.
- A route identifies the model and harness that provide execution capacity.

A caller can request a task without a persona. When a persona is requested, it
does not affect route selection: Landing selects from the requested tier's
routes, or runs the route the caller named. Changing routes does not silently change or remove the
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
selects a tier on the caller's behalf. A request that names its own route does
not use a tier at all, default or otherwise.

A coding agent can create a small neutral policy that distinguishes work by
execution needs from Landing's current routable options. Tiers are not named
after professions and do not imply a built-in persona catalog. A project owner
can change, rename, remove, or supplement them. Product documentation does not
hard-code model names.

## Selection policy

When the caller names a kind of work, Landing considers only routes in the
requested tier. Within that set, it uses known signals such as:

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

## Naming a route and model targets

Sometimes the model is the request rather than a detail of it: a caller
comparing two models, reproducing an earlier result, or wanting a second reading
from a model the project's tiers do not reach. That is a deliberate choice, and
Landing honors it. The caller supplies a model target, which resolves to one or
more routes.

A target can name an exact `harness/model` route or a bare `model` whose harness
Landing can infer. It can also name one harness's latest model as
`harness/latest`, `latest` across every harness with one, a tier's primary
routes, or a comma-separated combination of targets. Resolution removes
duplicate routes. A bare model is inferred only when Landing's model hints
identify exactly one harness; an unknown bare model requires its harness rather
than inviting a guess. A name that has more than one meaning is rejected with
the competing meanings.

`latest` is Landing's current frontier choice for a harness. Landing ships a
default for each harness that has one, and a project can override that choice
for the harnesses it names while retaining the other defaults. `latest` is
usable anywhere a model is named, including a tier. It means every harness with
a defined latest; a harness without one is omitted. Landing reports the
resolved route and whether a `latest` choice came from the project or Landing,
so callers can see what answered. A continued thread remains on the concrete
route that answered even when `latest` later changes.

A named target replaces the tier for that request. A caller names a kind of
work or a target, never both, because the two answer the same question. Ordinary
work needs a target that resolves to exactly one route; meetings can use a
multi-route target to create several seats.

A named route is not filtered by project policy. A caller may name any route
through a supported harness present on the machine, whether or not a tier
configures it. Model hints help infer routes and offer useful short names; they
never refuse a model. Landing accepts a harness-qualified model it has not seen
and validates it before work runs.

Validation reports a route as `valid`, `invalid`, or `unverified`. Landing uses
a live provider listing when it can settle the question, then uses a minimal
probe on that exact model when it cannot. Only a definitive provider rejection
makes a route invalid. A rate limit, network failure, timeout, or other
inconclusive result is unverified, and Landing refuses to dispatch it. A valid
result is remembered for one day; invalid and unverified routes are checked
again when requested. A probe spends a trivial amount of credit. That is a
deliberate, confined exception to Landing's rule that capacity checks do not
spend credits: it lets Landing accept newly released models without pretending
they work.

A named route does not fall back. Landing does not rank it against alternatives,
hold it in reserve, or move the work elsewhere when capacity runs out, because
substituting another model would discard the reason the caller named this one.
Work runs on the named route or reports a failure that says so. This is the
guarantee Landing already makes about a requested persona, for the same reason.

Unreadable capacity does not withhold a named route. Availability decides among
a tier's routes; a request that names its own route asks no such question, so an
unreadable gauge is not grounds to refuse it. A harness that cannot run the work
at all — absent, or reporting that it is not authenticated — is.

A continued conversation stays on the route that owns its context, so continuing
work that began on a named route requires no further route choice.

Naming a route stays the exception. A tier describes work by what it is rather
than by which model should do it, and it is what lets Landing answer to
availability. Naming a route is not a way to supply a tier the project is
missing: a kind of work the project dispatches repeatedly belongs in its policy.

## Available routes

Naming a route means knowing which routes exist, so Landing reports them. The
report names every model a harness can authoritatively enumerate, and useful
known examples from a harness that cannot enumerate them, along with every
route the project's tiers configure. Each is written the way a caller names it,
with the harness's observed state and the tiers that use it.

Landing marks an advisory model list as incomplete. A caller may still name a
model that the list does not show; Landing validates it before dispatch. The
report never presents examples as a closed provider catalog.

That distinction is the point of the report: it separates a route this project
has already chosen from one that is merely available, so a caller can see both
what the policy prefers and what it has passed over. The report answers before a
project has configured any policy, because which routes exist does not depend on
that policy.

Reading the report never spends capacity or changes provider state. It says what
a caller can name; measured availability is reported where availability is the
question.

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

Retry belongs to tier routing; a named route does not reroute. Retry is bounded.
A malformed task does not tour every provider, and exhaustion never becomes an
empty success. Temporary route marks expire so stale state does
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

A seat is one persona on one route. A persona may have several seats, allowing
the same perspective to be heard from several models. Casting uses a model
target, so one cast can resolve to several routes; each distinct resolved route
becomes one seat for that persona. The arbiter may be cast, but its target must
resolve to exactly one route. A cast belongs to this meeting, never to the
persona.

Before dispatching any seat, Landing resolves and validates every distinct
route for the seats and arbiter. An invalid or unverified route stops the
meeting before participant work is spent and identifies each failing target.
Landing then puts the question to every participant seat in parallel. Each
begins in a clean context containing the question and that persona's own
material. Landing then puts every position to the arbiter. The arbiter reads
them and reports where positions genuinely conflict and what those disagreements
turn on. Landing returns every position and the arbiter's reading to the lead
agent.

Results identify each seat by persona and concrete route. If a validated route
becomes unavailable, Landing reports its seat as failed rather than silently
dropping it. The meeting continues with the positions that answer when at least
two seats answer; failed seats remain visible to the lead agent.

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
may cast a particular participant or the arbiter to a model target, including a
route outside the meeting's tier. Personas have no routes, models, harnesses,
or tier affinity.
Participants retain the capabilities of ordinary Landing work, including investigating, reading, and running commands. Landing does not
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

- Capacity reads never spend credits or mutate provider state, except for the
  confined model-validation probe described above.
- Authentication stays in supported official tooling or official APIs.
- A route with unreadable capacity reports unknown.
- A tier with no route is invalid.
- A work request that names neither a kind of work nor a route requires a
  configured default tier.
- A named route runs as named or fails; Landing never substitutes another.
- A requested persona is never replaced with another perspective or unprompted
  work.
- Results preserve failures and identify the selected route when useful.
- Detailed scoring and tie-breaking remain diagnostic information.
