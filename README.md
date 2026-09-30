# Landing

**When one AI hits a limit, your work stops. Landing keeps it moving.**

Landing is a local **AI execution optimizer** for coding work. It routes tasks
through the AI coding agents already installed on your machine according to
project policy and current availability, while preserving the prompted
perspective you asked to do the work.

Landing currently supports Codex, Claude Code, Grok, and Cline. Each harness
keeps control of its own authentication. Landing does not collect provider
credentials or imitate their clients.

## Why Landing exists

AI coding work is fragmented across models, providers, harnesses, and separate
usage pools. When one path is unavailable, the usual recovery is manual: choose
another tool, reconstruct the prompt, move whatever context still matters, and
try again.

Landing turns those separate execution paths into one project-owned policy:

| You decide | Landing concept |
| --- | --- |
| What kind of work this is | A routing tier |
| Which perspective should do it | An optional persona |
| Which eligible model and harness should run it | An availability-aware route |

The core separation is **who does the work** versus **where it runs**. A
security reviewer should remain a security reviewer whether the eligible route
is served by Codex, Claude Code, Grok, or Cline.

```text
task + optional persona
          │
          ▼
   project routing tier
          │
          ▼
 policy + current availability
          │
          ▼
 installed harness and model
```

## What Landing does

- **Optimizes execution.** Landing checks eligible routes and selects one using
  measured availability and the policy defined by the project.
- **Keeps work moving.** After confirmed exhaustion, Landing can mark that route
  temporarily unavailable and reroute the task once.
- **Preserves prompted expertise.** Project-owned personas carry their
  instructions and reference material independently of the selected model.
- **Delegates from the tools you already use.** A person or lead coding agent can
  invoke Landing without moving into another workspace or dashboard.
- **Convenes independent perspectives.** Meetings ask several personas the same
  question in clean contexts, then ask an arbiter persona to identify genuine
  conflicts.
- **Lets chats reach one another by name.** A chat watches with
  `landing messages monitor` and sends with `landing messages send`. Every
  project on the machine shares the bus; the address is the name.

Landing runs locally as a single Go binary. Project policy and personas live in
`.landing/` and can travel with the repository; personal authentication does
not.

## Install

Download a prebuilt binary for macOS, Linux, or Windows from
[GitHub Releases](https://github.com/on-mission/landing/releases), verify it
against the accompanying `checksums.txt`, and put it on your `PATH`.

With a Go toolchain, you can instead install the latest tagged version:

```sh
go install github.com/on-mission/landing/cmd/landing@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`. Builds produced by
`go install` are not stamped by Landing's release workflow, so `landing --version`
may report `dev`; the release artifacts contain the stamped version.

To build from a checkout:

```sh
go build -o landing ./cmd/landing
```

## Quickstart

Landing is designed to be configured by the coding agent already working with
you. You can ask it:

> Inspect the AI coding harnesses Landing can use in this project, show me the
> available routes, and configure a sensible default routing tier. Tell me
> exactly what you change.

The agent can discover the current command surface from `landing --help` and
the routes available on your machine from `landing harness list`.

To configure Landing directly, initialize the project and inspect its available
harnesses:

```sh
landing config init
landing harness list
```

Then create a tier from routes reported by `landing harness list`. A tier is a
named policy for a kind of work, not a provider:

```sh
landing tier add \
  --name default \
  --description "General coding work" \
  --route codex/gpt-5.6-terra \
  --route claude/claude-sonnet-5 --fallback-below 20 \
  --default
```

The second route in this example is held in reserve until every ordinary route
has less than 20% measured availability. Routes may use separate model pools
inside one provider, different providers, or both.

Now dispatch work without choosing a harness at invocation time:

```sh
landing "Review the authentication flow and identify the highest-risk flaw."
```

Landing returns the result and identifies the harness, model, and thread that
served it. When the harness supports continuation, reply in the same thread:

```sh
landing --reply <thread-id> "Now propose the smallest safe fix."
```

When a specific model is the point of the request rather than an
implementation detail, name the route instead of a tier and Landing skips
routing entirely:

```sh
landing model list
landing --model grok/grok-4.6 "Port this module and keep the public API."
```

A dispatch takes `--tier` or `--model`, never both. A pinned route has no
fallback: if you asked for that model, quietly substituting another defeats
the point. Replies continue on the route their thread already runs on.

## Reusable personas

A persona captures a prompted perspective and its project-specific reference
material. It has no provider, model, harness, or routing preference of its own.

Create one from an instructions file:

```sh
landing persona add \
  --name security-reviewer \
  --description "Finds exploitable boundaries and unsafe assumptions" \
  --instructions-file ./security-reviewer.md
```

Then ask that persona to do work through any configured tier:

```sh
landing --persona security-reviewer \
  "Review the authentication flow and identify the highest-risk flaw."
```

If availability moves the task to another eligible route, Landing preserves the
requested persona rather than silently replacing it with generic instructions.

## Meetings

Some decisions need independent perspectives, not another pass from the same
context. A Landing meeting runs one deliberation round: participants answer the
same question independently, then an arbiter persona reads their positions and
identifies conflicts that cannot all be acted on.

```sh
landing meeting \
  --arbiter staff-engineer \
  --persona security-reviewer \
  --persona product-reviewer \
  "Should we ship this authentication design?"
```

Landing returns every position and the arbiter's reading. It does not force
agreement, decide whether another round is needed, or hide minority concerns.
The lead agent or user remains responsible for synthesis and the final decision.

## Messages

A live chat can send a durable message to another chat by name, including a
chat in a different project on the same machine. Other chats cannot see this
conversation.

Write the standing instructions into existing `AGENTS.md` and `CLAUDE.md`:

```sh
landing messages install
```

That command does not create those files and does not write harness settings.
A chat then backgrounds `landing messages monitor` with the harness's own
backgrounding, reads the output when it exits, skips ids it has already
handled, and backgrounds the monitor again before other work.

Inspect names and send a message:

```sh
landing messages who
landing messages send --to <name> --message "I changed the routing boundary; review before editing it."
```

Send returns as soon as the message is stored. A reply arrives later through
the sender's own monitor. `--to all` reaches every registered name except the
sender. A dispatched Landing job is not a chat and does not start a monitor.

Messages coordinate work; they are not locks, permissions, or shared memory.

## Useful inspection commands

```sh
landing config show
landing harness list
landing model list
landing tier list
landing persona list
landing messages who
landing messages inbox
```

Run `landing --help` for the full command surface and `landing <command> --help`
for command-specific options. CLI help is the source of truth for current
syntax.

## Trust and execution permissions

Landing dispatches unattended coding agents. To prevent a routed task from
stalling at an approval prompt, the current Codex, Claude Code, and Cline
adapters use permissive execution modes. A dispatched agent may read and modify
files or run commands with the access granted to its harness.

Use Landing only in projects and environments you trust. Review your working
tree, harness configuration, sandbox settings, and the requested work before
dispatching. Landing reports what served a task, but routing does not make the
task itself safe.

Capacity probes are read-only and do not spend model credits or mutate provider
state. Landing uses the user's installed official tooling and never copies or
stores provider credentials.

## Boundaries

Landing is not:

- a promise to eliminate every usage limit;
- a mechanism for bypassing a provider's restrictions;
- a pool of shared credentials or consumer accounts;
- a universal shared-memory layer;
- a cross-machine messaging service;
- a hosted control plane; or
- another agent workspace that replaces the tools you already use.

Each configured route remains independently authenticated and subject to its
provider's terms. When one route is unavailable, Landing can choose another
route allowed by the project's policy; it does not alter or evade the first
route's limit.

## Contributing

Before sending a change, run:

```sh
go build ./...
go vet ./...
gofmt -l .        # must produce no output
go test ./... -race
```

Landing is licensed under the [MIT License](LICENSE).
