# Landing

Landing is a command-line router for AI coding work. You describe the kind of
work you need done; Landing picks which installed harness and model actually
has capacity to run it right now, dispatches to it, and tells you which one
served the work.

Landing is a Go CLI, distributed as a single static binary with no third-party
dependencies, for macOS, Linux, and Windows.

## What it does

Landing sits between you (or your lead coding agent) and the AI coding
harnesses already installed on your machine — currently `codex`, `claude`,
`grok`, and `cline`. It never handles credentials; each harness authenticates
through its own existing tooling. Landing only reads capacity from each
harness, which costs nothing and changes nothing.

You ask for a **tier** — a named kind of work — instead of naming a vendor
directly. A tier is a list of routes (a harness and, usually, a model) that
your project defines, in preference order. Landing picks among a tier's
routes by measured availability and reports which route actually served the
request.

Configuration is project-scoped, at `.landing/config.json`, created and edited
through Landing's own commands (or by hand — Landing validates whatever it
reads). There are no global settings.

Two capabilities build on the same dispatch mechanism:

- **Meetings** put one question to several personas in parallel and have an
  arbiter persona read their positions.
- **Comms** let agents working in the same project message each other across
  harnesses.

## Install

Landing isn't published to a package registry or as a release binary yet.
From a checkout of this repository:

```
go build -o landing ./cmd/landing
```

That produces a `landing` binary in the current directory. Put it on your
`PATH`, or invoke it directly (`./landing`).

## Quickstart

Landing does nothing until a project has a configuration naming at least one
tier. Starting from an empty directory:

```
$ landing "write a haiku about compilers"
error: CONFIG_NOT_FOUND: no .landing/config.json exists between "/tmp/demo" and "/"
```

Create one:

```
$ landing config init
configuration: /tmp/demo/.landing/config.json
tiers: none; configuration file names no tiers, and dispatch resolves only when at least one exists
default tier: none
```

A fresh config names no tiers on purpose — dispatch still won't work. See
what Landing can actually route to on this machine, then add a tier that uses
one of those routes:

```
$ landing harness list
claude: ready
  path: /home/you/.local/bin/claude
  capacity: five_hour 4.00% used, seven_day 1.00% used
  models: claude-opus-5, claude-sonnet-5, claude-haiku-4-5-20251001
codex: ready
  path: /opt/homebrew/bin/codex
  capacity: codex_bengalfox 3.00% used, codex 23.00% used
  models: gpt-5.6-terra, gpt-5.3-codex-spark, gpt-5.6-luna
...

$ landing tier add --name default --description "General work" --route claude/claude-sonnet-5 --default
added tier: default
```

Now dispatch:

```
$ landing "say hi in exactly 3 words"
Hey there, Ray!
thread: 720653a4-36e5-43a1-81e1-7dce733e1132
harness: claude (model: claude-sonnet-5)
```

Landing reports the thread it created and which route actually served the
work. Continue that thread with `--reply`:

```
$ landing --reply 720653a4-36e5-43a1-81e1-7dce733e1132 "now say bye in 3 words"
Bye for now!
thread: 720653a4-36e5-43a1-81e1-7dce733e1132
harness: claude (model: claude-sonnet-5)
```

## Beyond one route

A tier can list more than one route, in preference order, and reserve a
lower-preference route until capacity on the one above it drops below a
threshold:

```
landing tier add --name default \
  --route claude/claude-sonnet-5 \
  --route codex/gpt-5.6-luna --fallback-below 20 \
  --default
```

Run `landing --help` for the full command surface, and `landing <command>
--help` (for example `landing tier add --help`) for a specific command's
flags. Help text and `landing config show` reflect the exact, current syntax
and configured state — that reference isn't duplicated here because it drifts
from the code otherwise.

## Meetings

A meeting dispatches one question to multiple personas in parallel, then
gives their independent positions to an arbiter persona, which responds in
prose:

```
landing meeting --arbiter <persona> --persona <persona> --persona <persona> "<question>"
```

## Comms

Agents dispatched through Landing in the same project can message each other
across harnesses with `landing comms`. Delivery to a *running* session
depends on that harness trusting a hook Landing installs into its project
configuration:

```
landing comms --install
```

Landing cannot grant that trust on your behalf — each harness must be
configured to accept it. Without installed hooks, an agent can still read its
own inbox and history (`landing comms --inbox`, `landing comms --history`),
and threads dispatched through Landing still register.

## Known work

See [TODO.md](TODO.md) for what's still open before a public release.

## Contributing

Before sending a change, run:

```
go build ./...
go vet ./...
gofmt -l .        # must produce no output
go test ./... -race
```

Licensed under the [MIT License](LICENSE).
