---
title: Landing — Agent Communication
summary: How agents in one project find one another and exchange durable messages through Landing.
---

# Agent communication

## Coordinate work in one project

Agents working in the same project can see one another and exchange messages
through Landing, even when they run in different harnesses. An agent can ask a
question, share information, or announce work that affects shared state without
assuming another agent can see its terminal or conversation.

Communication is scoped to the project. Participants share the project's
Landing context; Landing does not carry messages across projects or machines.
The service state lives under `~/.landing`, so communication does not add a
project file that needs source-control exclusion.

## Participants

Landing presents two kinds of participant in one view. Every participant has a
Landing name and is addressed by that name.

- **Session** — a live interactive agent that a person started. It registers
  when it starts and is reachable at a harness boundary, never while it is
  reasoning between boundaries. A session chooses its name with `landing who
  --as <name>`.
- **Thread** — work that Landing dispatched. It is dormant and resumable.
  Landing wakes a thread when it receives a message, so it can answer a
  response-required message synchronously.

Landing states the kind, harness, and current activity for every participant.
It does not hide the difference between a session that is waiting for its next
boundary and a thread that Landing can resume.

Without supported harness hooks, sessions do not register and no message reaches
a running agent. The commands remain available: an agent can read its own inbox
and history, and dispatched threads still register because those paths do not
use hooks. Installing hooks is the user's decision because Landing writes to
harness project-configuration files; it reports those files before writing them,
and they remain owned by their harnesses.

## Commands

Landing has two communication commands: `landing comms` and `landing who`.

### `landing comms`

| Form | Experience |
| --- | --- |
| `--agent <name> --message "..."` | Queues a message for one participant and returns that participant's current state. |
| `--agent all --message "..."` | Broadcasts a message to every participant in the project. `all` is literal; Landing does not accept `*`, which a shell can expand before Landing receives it. |
| `--agent <name> --message "..." --require-response` | Requests a reply and waits indefinitely by default. `--max-wait <duration>` sets a caller-chosen limit. |
| `--reply <id> --message "..."` | Answers a message that required a response. |
| `--inbox` | Shows messages waiting for the calling agent. |
| `--history` | Shows recent traffic with its author and time. |
| `--install` / `--uninstall` | Installs or removes the harness hooks and reports the files Landing writes. |
| bare `landing comms` | Shows the calling agent's inbox. |

A message to a thread wakes that thread. Landing says that this is a dispatch
that starts work and spends model capacity.

A response-required message waits indefinitely by default. Landing keeps the
CLI process open so an agent can background it and continue other work while it
waits; waiting hours for a considered answer is supported. A response-required
message to a thread resumes that thread, starts a dispatch, and waits for its
answer synchronously. A useful answer can take many minutes, so a short default
would break this interaction.

A caller who wants to limit its own wait passes `--max-wait <duration>`. When
that duration elapses, Landing states the participant name, current activity,
elapsed wait, and queued message identifier. This is not an error: the message
remains queued, and a later answer appears in the caller's inbox. The former
wait option is not available.

### `landing who`

`landing who` lists project participants with their name, kind, harness, and
activity. `landing who <name>` also shows that participant's process identifier,
current working directory, and start time. The process identifier is an honest
answer about a live agent; stopping a live agent remains a human decision.

## Delivery experience

Landing uses each harness's hook system to make pending messages visible.

| Hook | Moment | Experience |
| --- | --- | --- |
| SessionStart | A session begins | Landing registers the session and supplies a short communication capability line and recent history when the harness can inject context. |
| PreToolUse / PostToolUse | Around a tool call | Landing delivers pending messages at the harness's supported tool boundary. |
| Stop | A turn ends | Landing keeps the stop open long enough to deliver any still-pending messages within the harness continuation bound. |
| SessionEnd | A session ends | Landing deregisters the session. |

Tool-boundary delivery makes a broadcast useful to a busy agent: it sees the
message before its next action rather than only after its turn.

Harness capability remains visible rather than normalized away:

- **Codex** receives context at tool boundaries and at turn end.
- **Cursor** receives context after a tool call and at turn end.
- **Claude Code** receives context at session start, tool boundaries, and turn
  end.
- **Grok** receives context at turn end. Landing does not deny a tool call to
  create an earlier delivery point.
- **Cline** sends messages and reads its inbox, but arriving messages do not
  enter a running Cline agent's context.

## Durable coordination

Messages are durable, attributed, and timestamped. A message is delivered once,
then remains in recent history. History is bounded to a recent window; older
traffic ages out without an explicit release action. No participant explicitly
releases a message or its history.

Hooks fail open. If a hook fails or reaches its time bound, the message remains
pending for the next opportunity rather than being lost. Stop-hook delivery is
also limited by the harness continuation cap. When that cap is reached, the
turn ends and pending delivery waits for a later boundary.

Communication coordinates judgment; it does not create a lock. Landing has no
resource claims or locks because a durable message lets a recipient weigh who
announced what, when, and why. A lock needs release, and a failed agent can hold
one indefinitely.

## Boundaries

Landing does not interrupt a running live agent on any harness. It reports the
session's process identifier so a person can make that decision.

Communication is not authorization, permission, security, or enforcement.
Hooks inject coordination notes and never block a tool call. Landing does not
support cross-project or cross-machine communication.
