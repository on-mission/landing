# Setting up Landing in a project

You are setting up Landing in someone's project. This page is context, not a
procedure. It describes what Landing is, what a configured project looks like,
and which decisions belong to the person you are working with. How you get
there is yours to work out with them.

## What Landing is

Landing is a local-first execution layer between a caller and the AI coding
harnesses already installed on a machine. A caller gives it work; Landing
chooses which harness and model runs that work, waits for a result, and returns
it.

It keeps three things apart that are usually collapsed into one choice:

- **The work** — what needs to happen.
- **The persona** — the durable perspective that should do it, when that
  matters.
- **The route** — the harness and model that supply execution capacity.

A caller asks for a *kind of work* rather than a vendor. Landing reads how much
capacity remains on each eligible route, prefers the one with the most headroom,
and reports which one served the result. When a route is exhausted it is marked
cold, and Landing may retry once elsewhere; a failure is never converted into an
empty success.

Landing never handles credentials. Each harness owns its own authentication
through its own official tooling, and Landing only reads capacity, which costs
nothing and changes nothing.

## Messages

A live chat can send a durable message to another chat by name. Other chats
cannot see this conversation. A chat in one project can reach a chat in
another; the address is the name, not the project path. Every project on the
machine shares one bus.

`landing messages install` writes the standing messages section into existing
`AGENTS.md` and `CLAUDE.md` at the project root. It does not create those
files, and it does not write harness settings. Whether to add that section is
the user's decision.

A chat backgrounds `landing messages monitor` with the harness's own
backgrounding. When that command exits, for any reason, the chat reads its
output, acts on each message, skips an id it has already handled, and
backgrounds the monitor again before other work. Send returns as soon as the
message is stored. A reply arrives later through the sender's own monitor.

A dispatched Landing job is not a chat and does not start a monitor. The
detailed experience is defined in the product documentation for messages.

## What a tier is

A tier is a named class of work. It carries a description in the author's own
words and an ordered list of routes, each naming a harness and usually a model.
Order expresses preference; it does not override measured availability. A route
may be held in reserve so it is only reached when everything else in the tier is
low or unavailable.

Tiers are the project's vocabulary for its own work. They are not professions,
not seniority levels, and not personas. A project may have two tiers or six, and
the names should be ones the people in that project would actually use.

One tier may be marked as the default, which is where work goes when the caller
names no tier. Without a default, unnamed work fails rather than Landing
choosing on the caller's behalf.

## What a persona is

A persona is a durable character — a perspective, a set of concerns, a way of
reading a problem. It has no harness, no model, no route, and no tier. Any
persona can run on any route, which is what keeps the two decisions separate:
the persona is who does the work, the tier is what capacity does it.

Personas live in `.landing/personas/<name>/PERSONA.md`. That file opens with
front matter carrying a one-line `description` and continues with the persona's
instructions in prose. Any other file placed alongside it is corpus — reference
material that persona carries with it.

A project needs no personas at all. They earn their place when a project keeps
returning to the same perspective, or when a decision needs perspectives that
genuinely disagree.

## Meetings

A meeting puts one question to several personas at once. Each answers in a
clean context holding only the question and its own material, so the positions
are genuinely independent. An arbiter persona then reads every position and
reports where they actually conflict and what the disagreement turns on.

A meeting requires an arbiter and at least two participants. The arbiter is a
perspective of its own rather than a neutral referee, and it may also sit as a
participant, in a separate instance that cannot see its own arbiter work.

Landing returns one round: every position, and the arbiter's reading of them.
Deciding whether the deliberation is finished belongs to the agent that convened
it, which either puts another round with the positions so far carried forward
verbatim, or asks the arbiter for a synthesis as ordinary work. Landing does not
judge conflict, count rounds, or declare consensus.

A meeting costs several times one dispatch, so naming several personas is not a
way to get a better answer to ordinary work.

## What a configured project has

A configured project has a `.landing/` directory at the project root. Inside it,
`config.json` is the whole of Landing's configuration, and `personas/` holds any
personas the project defines. The configuration holds no credentials, tokens, or
machine-specific state, and it describes policy rather than any one machine — so
it is written to be shared with everyone working in that project, who then see
it change the way they see any other change.

Landing keeps its own operational state — records of past dispatches, temporary
marks on routes that recently failed — outside the project entirely, under the
user's home directory. That state is machine-local and disposable. Nothing
Landing writes inside a project is private to one person or one machine.

Landing dispatches nothing until that file resolves. A caller names a kind of
work, and the project's tiers are what that name refers to; without them there
is nothing to route within. Built-in defaults describe a sensible starting
grouping, but they are not this project's grouping, and running on them would
mean guessing at how the project wants its work organized and spending real
capacity on the guess.

## What only the person you are working with can decide

Landing has no opinion about any of the following, and neither should you until
you have asked:

- **Which harnesses this project should use.** Some are installed but
  unauthenticated; some are installed and the person may still not want this
  project routing through them.
- **Which models within those harnesses**, and in what order of preference.
- **What classes of work this project has, and what to call them.** This is the
  decision that most repays a real conversation. A project's existing habits are
  the best evidence: what kinds of tasks do they hand off, and how differently
  do they treat the results?
- **Whether ordinary unnamed work should have somewhere to go**, and if so
  which tier.
- **Which perspectives this project keeps returning to**, if any, and whether
  they should become personas. This is optional and Landing is useful without
  any. It is worth asking about when the project makes decisions that benefit
  from perspectives that disagree, because that is what a meeting needs.
- **Whether to install the standing messages section** into existing
  `AGENTS.md` and `CLAUDE.md`. `landing messages install` writes that section
  only; it does not create those files or write harness settings. Without it,
  a chat in this project has no standing instruction to watch for mail.

Draw these out of them rather than proposing a policy and asking for approval.
A tier they described is one they will use; a tier you invented is one they will
delete. Where they have no opinion, say what you would choose and why, and let
them react to that.

Some of this you can infer and confirm rather than ask cold — what the project
is, what languages and tools it uses, what work they seem to repeat. Ask about
what you cannot see.

## What Landing will tell you

Ground the conversation in what is actually true on this machine rather than
what is plausible.

`landing harness list` reports every harness Landing supports: whether it is
installed, where, whether it is authenticated, how much capacity remains, and
which models it can route to. It distinguishes a harness that reported an
authentication failure from one whose capacity simply could not be read, and it
never resolves the second into the first.

`landing config show` reports the resolved configuration, where it came from,
and which directories were searched to find it.

`landing --help` is the current, authoritative description of every command and
option. It is generated from the code, so it is right when this page is stale.
Read it rather than guessing at syntax.

## How the configuration gets made

Landing writes its own configuration. Its commands accept a complete tier — its
name, description, routes in order, reserve treatment, and whether it is the
default — validate every part against what the harnesses can actually reach, and
write the file atomically. A route naming a model its harness does not support
is rejected, and the rejection names the models that harness does support.

This exists so that you never compose JSON by hand. A configuration produced
through Landing cannot parse correctly and mean nothing.

Editing `.landing/config.json` directly is also supported, and Landing validates it
whenever it reads it. It is the right tool for a bulk edit or a change that is
easier to see whole. It is not the tool for routine changes, because it is the
path where a mistake survives until something reads the file.

## What agents working here should know afterward

A project that is configured but whose agents do not know Landing exists gets
used once, by whoever set it up, and then forgotten. Configuration alone does
not change what anyone does.

Most projects keep standing context for the agents that work in them —
`CLAUDE.md`, `AGENTS.md`, or whatever this one already uses to give an agent its
bearings at the start of a session. That is where Landing has to end up, or it
will not be reached again.

A project often has more than one such file, because different harnesses read
different ones, and Landing exists precisely so that several harnesses work in
the same project. Guidance placed in only one of them reaches only the agents
that read that file — and the agent doing the setup is itself running on one
harness, so the file it thinks of first is its own. Every standing-context file
the project keeps needs this, or the harnesses whose file was skipped stay
unaware that Landing is here at all.

What earns its place there is what an agent needs in order to *decide*, without
running anything first: that Landing is available and what it is for, which
tiers this project has and what kind of work belongs in each, and that the
current authoritative detail comes from Landing's own commands rather than from
that file. An agent that knows a tier called `research` exists and what it is
for can choose to use it; one that has to go looking will not.

The same applies to anything else the project can now reach. If personas exist,
name them and what each is for. If they are the kind that would disagree
usefully, say that a meeting can put one question to several of them at once and
that `landing meeting --help` describes how a round works. An agent that has
never heard of a capability does not go looking for it.

What does not belong there is a transcription of the configuration. Routes,
models, and orderings change, and a copy of them rots quietly while the file it
was copied from stays correct one command away.

For messages, run `landing messages install` rather than transcribing how a
chat watches and sends. That command writes the standing section into existing
`AGENTS.md` and `CLAUDE.md` and leaves the surrounding file alone.

Two things matter about how the rest of the Landing guidance is written. It
should read as its own section, so that whoever revises it later can tell what
belongs to Landing and what does not. And leave everything already in those
files alone — they are someone's own instructions, and they were there first.

## Knowing that it works

A configuration that resolves is not the same as a configuration that runs.

Landing reports which harness and model served each result, so a single small
piece of real work — something with a checkable answer, in a tier the person
cares about — tells you more than any amount of inspection. If a tier the person
expects to use cannot produce a route, it is better to find that now than the
first time they rely on it.

Reading capacity and listing models cost nothing. Dispatching work spends real
model capacity, so keep any check small and make it count.

A harness that reports healthy capacity can still refuse to run. Some ask a
person to approve a directory the first time they are used in it, and that
approval cannot be given by an agent or by Landing — the capacity read succeeds
because it never enters the directory at all. If a harness looks ready and then
declines to execute, this is a likely cause, and resolving it belongs to the
person you are working with rather than to you.

## When setup is complete

Landing is set up when all of the following are true, and you have confirmed
rather than assumed each one:

- `.landing/config.json` exists and resolves without error.
- Its tiers, their names, and their descriptions are ones the person recognizes
  as their own.
- Every route in it names a harness that is installed and authenticated on this
  machine, and a model that harness can reach.
- Unnamed work either has a default tier or is understood to fail deliberately.
- Real work has been dispatched through at least one tier and returned a result.
  If your own environment cannot dispatch — a sandbox that withholds the
  harness credentials or state, for instance — say so plainly and hand this
  check to the person you are working for rather than treating configuration
  that merely resolves as configuration that runs. Their confirmation is
  evidence about their environment; it is not evidence about yours.
- Any personas the person wanted exist under `.landing/personas/`, each with a
  `PERSONA.md` that `landing persona list` reports.
- Every standing-context file the project keeps — not only the one your own
  harness reads — names Landing, this project's tiers, and whatever else the
  project can now reach, with everything that was already there untouched.
- The person can say what each tier is for without reading the file.

Finish by telling them what now exists, in their words rather than in Landing's:
which tiers they have, what each is for, where the configuration lives, and what
changed on their machine. Anything you could not complete, and anything you
decided on their behalf, belongs in that summary too.
