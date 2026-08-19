# Council

Convene a council of isolated specialist agents to stress-test a decision or
review a body of work, then return one synthesis.

The user's arguments identify the question and the requested members. Persona
selection is explicit: if no members and no `all`/`everyone` instruction are
given, ask one focused question before dispatching.

## Available council members

- `founding-engineer` — standing arbiter; added automatically
- `product` — user value, scope, and sequencing
- `domain-engineer` — ownership, naming, and justified abstraction
- `go-engineer` — Go concurrency lifecycles, error modeling, and package design
- `typescript-engineer` — soundness and type-system design
- `testing-engineer` — test value and failure-class coverage
- `security-engineer` — threats and trust boundaries
- `systems-engineer` — dataflow, capacity, and failure modes
- `frontend-engineer` — frontend architecture and state ownership
- `ui-engineer` — visual and interaction quality

## Protocol

1. Parse the exact question and requested roster. Add `founding-engineer` as
   arbiter unless the user explicitly requests no arbiter.
2. Create one isolated, read-only specialist task per member using the current
   harness's native sub-agent mechanism. Fan out independent members in parallel.
3. Give every member the verbatim user question, roster, repository root, and a
   requirement to inspect evidence directly. Do not pre-summarize files for them.
4. Collect round one. Identify real tensions: conflicting recommendations whose
   load-bearing assumptions differ. Different emphasis is not disagreement.
5. For each tension, send a focused follow-up to the relevant members, quoting
   the opposing claim faithfully. Resume the same specialist context when the
   harness supports it. Cap at three follow-ups per tension.
6. Ask the arbiter to resolve remaining technical disputes with the evidence and
   explicit trade-off. The orchestrator remains responsible for faithful
   synthesis, not role-playing a specialist.
7. Close or dismiss every member and return the synthesis.

Council members do not edit files, commit, push, or mutate external state. A
council advises; subsequent implementation is a separate request.

## Synthesis

Lead with the council's answer. Then include:

- the reasoning that carried the decision;
- important conditions or evidence;
- resolved disagreements and why one side won; and
- `Where the council disagreed` only when a tension remained after the cap.

Take a position. Do not flatten disagreement into a menu or claim consensus that
did not occur.

ARGUMENTS: $ARGUMENTS
