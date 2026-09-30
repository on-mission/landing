---
title: Landing — Onboarding
summary: The intended experience from discovering Landing to a verified project setup.
---

# Onboarding

Installation alone does not make Landing usable. A project is ready when it has
configuration that groups the connections and models it wants Landing to reach
into named tiers, expresses its routing policy, and makes the relevant agents
aware of Landing. Landing keeps all project-owned files in `.landing/`,
including configuration at `.landing/config.json` and any project personas
beneath it. A user reaches that state by directing a coding agent to configure
the project through Landing, or by authoring the configuration directly.
Creating a persona is optional; availability-aware execution is useful without
one.

## Configuration paths

### Agent-driven configuration

A user describes the intended project policy to a coding agent. The agent asks
Landing what it can route to on the current machine, then asks Landing to
create or change the configuration. Landing validates every supplied route and
writes the configuration itself.

Landing reports the supported harnesses installed on the machine, their
authentication readiness, their remaining capacity, and the models each can
authoritatively enumerate or otherwise names as useful examples. It marks an
example list as incomplete, so the agent does not mistake it for a closed
provider catalog. If the agent supplies an unsupported harness, Landing fails
clearly. A harness-qualified model Landing does not know is accepted and
validated before Landing writes the policy; model lists aid inference and never
refuse a model.

Configuration-management requests accept the complete policy an agent needs to
express: route lists, their preference order, and detailed tier descriptions.
The agent does not construct a configuration file or infer its syntax. This
makes an invalid policy impossible to write through Landing rather than leaving
the agent to detect errors in generated text.

### Direct authoring

A user or agent acting for the user may author and edit the complete project
configuration directly. Landing validates that configuration whenever it reads
it. This path supports workflows that own configuration outside Landing; the
agent-driven path is the path Landing can validate before it writes.

## Agent-driven entry

The user tells their coding agent what execution policy they want for the
project. The agent uses Landing's installed help, current project context, and
routable-options report to establish or change that policy, verify readiness,
and summarize the resulting state.

The project context is the operational source of truth for an agent that works
in the project. Landing's website supports discovery and installation, but a
website setup page is not a configuration protocol. Landing never requests
provider credentials; official harness tooling owns authentication.

## Configuration experience

### 1. Identify the project

Landing configures the project repository where the agent works. Its
configuration defines the policy for work from that project.

### 2. Discover routable access

Landing identifies the supported installed harnesses, their authentication
readiness, their available capacity, and the models it can route to. Inspection
does not read credentials or spend execution capacity.

### 3. Express the project policy

The agent supplies the project's participating routes, tiers, route ordering,
fallback treatment, descriptions, and default tier when unnamed work is to be
resolved automatically. Tier descriptions express execution contracts rather
than professions or personas.

Advanced customization remains optional. A useful project policy does not
require scoring rules or provider-capacity terminology.

### Optional persona configuration

Landing lets the agent create a project-owned persona with its own instructions
and, when useful, reference material. Each persona has its own directory beneath
`.landing/`. Landing applies the persona to requested work without binding it to
a route, model, harness, or tier.

### 4. Validate and apply

Landing validates the complete requested policy before writing it. A request
that names an unavailable harness fails clearly. A model that
validates as invalid or unverified also fails clearly; Landing accepts an
unfamiliar harness-qualified model and uses live provider evidence or a minimal
probe to validate it.
Landing writes the project configuration inside `.landing/`. Agents discover
the current policy from that project-owned context without Landing writing into
unrelated instructions.

### 5. Report readiness

After a configuration change, Landing reports the active configuration source,
enabled routes, active tiers, default tier, available personas, and updated
agent targets. The coding agent uses this report to summarize the persistent
change for the user. Readiness checks do not consume model capacity merely to
prove that the configuration is usable.

## Decision boundary

Each configuration-management request is complete on its own. Landing does not
seek terminal input or pause an applied request for a conversational decision.
The coding agent obtains any policy choice from the user in its normal working
conversation, then gives Landing a complete, validated request. Invalid input
produces a descriptive failure without a malformed or semantically empty
configuration.

## Reconfiguration

The agent inspects the current project state, determines the intended policy,
and submits a focused change through Landing. Landing validates the change,
updates the project policy, and reports the resulting state. Direct authoring
remains available for configuration changes and receives validation when Landing
reads the file.

Landing uses the nearest applicable project configuration found by searching
upward from the directory where it is called. Built-in defaults offer a sensible
starting grouping, but they do not supply the project's tiers. When no
configuration is found, Landing does not dispatch and reports the missing
configuration and the directories searched. Without tiers, Landing cannot map
the caller's kind of work to routes to choose among, so it does not invent that
grouping or spend capacity on a guess. Diagnostics identify the resolved
configuration when one is available.

## Agent context

The project-owned policy lets an agent discover:

- what Landing does;
- which tiers exist and what work belongs in each;
- which personas exist and when a user or agent should request one;
- that meetings are available for consequential questions and that the agent
  finds their protocol in Landing's current help;
- how complete a delegated brief must be;
- how returned work is reviewed or validated;
- how continuation behaves; and
- how to discover current command syntax from code-owned help.

It also prevents a worker launched through Landing from recursively dispatching
the same work as though it were the lead agent.

## Messages setup

`landing messages install` writes the standing messages section into
`AGENTS.md` and `CLAUDE.md` when those files already exist at the project
root. It does not create either file, and it does not write harness settings.
Running it again replaces only the section it previously wrote.

After installation, a chat in the project backgrounds
`landing messages monitor --as <name>` with the harness's own backgrounding, as
the standing section directs. The detailed experience is defined in
[Messages](messages.md).

## Success standard

A user can describe an execution policy in ordinary language to their coding
agent, receive a structurally valid project configuration, understand the
resulting persistent state, and delegate work from their existing workflow. A
user who configures a persona can request that persona without choosing its
underlying model.
