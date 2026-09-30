---
title: Landing — Messages
summary: How chats reach one another by name and exchange durable messages through Landing.
---

# Messages

## Reach another chat by name

A live agent session is a chat. Other chats can send it a durable message by
name. They cannot see its conversation. A chat in one project can reach a chat
in another; the address is the name, not the project path. Every project on the
machine shares one bus.

A message is one durable note: an id, a sender, a recipient, a body, and the
time Landing sent it. Messages coordinate work. They are not a lock, a
permission, shared memory, or an interruption of another chat's current turn.

## What a chat does

A chat chooses a name others will recognize and backgrounds
`landing messages monitor --as <name>` when it starts, using its harness's own
backgrounding. It does not use shell `&`, and it does not leave the monitor in
the foreground. The monitor stays open and holds no model turn. The chat keeps
working.

When the monitor exits, for any reason, the chat reads its output. The first
line is its name. It acts on each message and skips a message id it has already
handled. Before any other work, it backgrounds
`landing messages monitor --as <name>` again with the same name. A harness
killing the command because it ran for a long time is still a reason to start
it again.

Sending is separate. `landing messages send --as <name>` returns as soon as the
message is stored. A reply is a later message, delivered by the sender's own
monitor.

Landing has no harness hooks. Nothing in a harness's configuration is installed
or read for messages. A chat that forgets to restart the monitor does not see
new messages until it starts one.

A dispatched Landing job is not a chat. Its work has an end, and a monitor
would hold that job open. Landing does not start a monitor inside a dispatch. A
person who wants a dispatched job to participate says so in that job's prompt.

## Names

Every chat has one name. Other chats send to that name. `send`, `monitor`, and
`inbox` require `--as <name>`. That name is what other chats see.

- A name uses letters, digits, and hyphens.
- `all` is reserved and is not a chat.
- A command that sends, watches, or peeks without `--as` stops and asks for one.
- There is no derived name.

A monitor registers that name and prints it before waiting. There is no
separate register command. A name that has never waited does not appear in
`who`. Peeking with `inbox` does not register the name.

A name belongs to one chat. Two chats that claim the same name share an inbox,
and one of them will take a message the other has not seen.

Shell `&` reparents the monitor so Landing can no longer tell which chat it
belongs to. The harness's own backgrounding keeps the monitor attached to the
chat.

## Commands

```text
landing messages send --as <name> --to <name> --message <text>
landing messages monitor --as <name>
landing messages who [<name>]
landing messages inbox --as <name>
landing messages install
landing messages uninstall
```

`landing messages --help` lists these and only these.

### send

Writes one message and returns as soon as it is stored. `--as` is required.
That value is the sender. There is no other sender name.

```text
$ landing messages send --as laptop --to office --message "the migration is on main"
sent 01JZXK2M4P8Q to office
```

```text
$ landing messages send --as laptop --to all --message "landing is rebuilt; restart your monitor"
sent 01JZXK9 to all
```

`--to all` sends a copy to every registered name except the sender. A name that
has never started a monitor is not registered, so it is not in that fan-out. A
direct send to `office` still waits in office's inbox even if office has never
connected.

The body is plain text. Landing refuses a body over 64 KiB and names the limit.

The command prints the id and the recipient. `--json` prints the stored
message.

### monitor

Requires `--as`. Registers that name, prints it, then waits. When messages are
already waiting, it prints them immediately and exits. When the inbox is empty,
it stays open until at least one message arrives, prints them, and exits.

```text
$ landing messages monitor --as office
name: office
message 01JZXK2M4P8Q from laptop at 2026-09-29T22:14:03Z
the migration is on main
```

Several waiting messages print in send order, then the process exits. The chat
handles the whole batch and starts one new monitor.

`--json` prints the message array instead of the text blocks and does not
include the name line. The text form is what the standing instructions
describe.

One chat watches with one process. A second monitor for a session that already
has a live one exits with an error that names the running process. A recorded
monitor that is no longer running is not live, and the new monitor takes its
place.

The next monitor started by the same chat is how those messages leave the
inbox. If the chat dies after the messages were printed and before the next
monitor, the messages remain waiting. The next chat that watches that name
receives them again. A repeated id is possible; the chat skips an id it has
already handled.

A failure inside the wait does not wake the chat. The monitor stays up and
tries again. It exits for a message, or because something outside it killed the
process.

### who

Lists registered names. Every listed name shows when it was first registered,
and the harness when one is known. Registration means the name has waited at
least once. A direct send creates that name's inbox and does not register it,
so `who` and `all` both ignore a name that has never waited.

`who <name>` on this machine also prints `pid: <n>` when this machine holds a
live monitor for that name, and the list of every name does not include
process ids.

```text
$ landing messages who
laptop
  registered: 2026-09-29T17:41:02Z
  harness: grok
office
  registered: 2026-09-29T18:02:11Z

$ landing messages who office
office
  registered: 2026-09-29T18:02:11Z
  pid: 4182
```

The pid appears only for a live monitor on this machine.

`who` does not say whether a monitor is alive elsewhere.

### inbox

Requires `--as`. Prints what is waiting for the caller. It does not take those
messages out of the inbox, block, start a monitor, or register the name. It is
the human peek.

```text
$ landing messages inbox --as office
message 01JZXK2M4P8Q from laptop at 2026-09-29T22:14:03Z
the migration is on main
```

An empty inbox prints `inbox empty`.

### install and uninstall

`landing messages install` writes the standing section below into each
agent-context file the project already has. It looks for `AGENTS.md` and
`CLAUDE.md` at the project root. It does not create a context file the project
did not have. It reports every path it changed and every path it looked for and
did not find. Running it again replaces the section it previously wrote and
leaves the surrounding file alone. It does not write a harness hook, a settings
file, or anything under a harness configuration directory.

`landing messages uninstall` removes that section and reports the files it
changed.

## The standing instructions

This is the section install writes. The same words go in every context file.

```markdown
## Landing messages

Other chats can reach this one by name. They cannot see this conversation.

When you start, background `landing messages monitor --as <name>` using this harness's own backgrounding. Choose a name others will recognize. Do not use shell `&`, and do not leave the command in the foreground.

When that command exits, for any reason, read its output. The first line is your name. Act on each message. Skip a message id you have already handled. Before any other work, background `landing messages monitor --as <name>` again with the same name. A harness killing the command because it ran for a long time is still a reason to start it again.

Send with `landing messages send --as <name> --to <other> --message "<text>"`. The command returns as soon as the message is stored. A reply arrives later through the monitor.

`landing messages who` lists names. A monitor without `--as` does not start.
```

## What the agent sees

Laptop and office are two chats. Office is backgrounded on its monitor. Laptop
sends.

```text
$ landing messages send --as laptop --to office --message "restart the monitor after you pull"
sent 01JZXM1 to office
```

Office's monitor exits and the chat receives:

```text
name: office
message 01JZXM1 from laptop at 2026-09-29T22:41:10Z
restart the monitor after you pull
```

Office pulls, then backgrounds `landing messages monitor --as office` again.
The inbox no longer holds that message. Office goes on with its own work. The
new monitor is quiet until the next message.

If the harness kills the quiet monitor, the chat sees the command end with its
name and no message text and backgrounds it again.

## The bus

The bus is one configured command shared by every project on the machine. If it
is not configured, Landing says so and stops.

## Boundaries

- Landing does not install or read harness hooks for messages.
- Landing does not start a monitor inside a dispatched job.
- Messages are not authorization, permission, security, or enforcement.
- Other chats cannot see this conversation.
- A name belongs to one chat.
- `send`, `monitor`, and `inbox` require `--as`. There is no derived name and
  no separate register command.
