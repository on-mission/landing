# Agent surfaces and docs search

This runbook owns Landing's agent-role synchronization, specialist knowledge
corpora, and searchable documentation setup.

## Surface model

`.claude/commands/` is canonical. `utilities/sync-commands.mjs` renders each
metadata entry into:

- `.claude/agents/<role>.md` for Claude agents; and
- `.codex/skills/<role>/SKILL.md` for Codex skills.

Do not edit generated targets directly. Role metadata lives in
`.codex/skill-meta.json`; generated hashes live in
`.codex/skill-sync.lock.json`.

## Role catalog

Author roles own documentation surfaces:

- `product-author` — desired product behavior and user experience
- `architecture-author` — system components, contracts, and invariants
- `pattern-author` — durable implementation rules

Advisory and engineering roles provide specialist judgment:

- `product` — product-owner feedback, value, focus, and sequencing
- `founding-engineer` — consequential architecture and technical strategy
- `domain-engineer` — naming, ownership, and justified abstraction
- `go-engineer` — Go concurrency lifecycles, error modeling, package and
  interface boundaries, and testability
- `typescript-engineer` — soundness, modeling, runtime boundaries, and compiler
  posture
- `testing-engineer` — test value, deterministic design, and test implementation
- `security-engineer` — threats, credentials, trust boundaries, and supply chain
- `systems-engineer` — dataflow, failure, capacity, and operability
- `frontend-engineer` — frontend architecture, components, and state ownership
- `ui-engineer` — visual hierarchy, interaction, accessibility, and motion

Orchestration and utility roles manage workflows:

- `engineering-manager` (`/em`) — runs one workstream through specialists
- `director` — supervises a slate through isolated engineering managers
- `council` and `council-runner` — multi-persona deliberation and arbitration
- `deep-reviewer` (`/deep-review`) — end-of-phase multi-lane review
- `persona` — creates and registers another source-grounded role
- `commit-your-changes` — audits and commits only conversation-owned changes
- `tldr` — read-only architectural walkthrough of the current solution
- `watchdog` — recurring monitoring until a job reaches terminal state
- `thank-you` — concise closeout and handoff without starting more work
- `sync` — keeps generated surfaces current

## Specialist corpora

Six roles have reference corpora under `utilities/personas/`. These files
contain deeper doctrine, calibration examples, and citations that do not belong
in the command prompt or pattern catalog.

- `utilities/personas/go-engineer/`
- `utilities/personas/typescript-engineer/`
- `utilities/personas/domain-engineer/`
- `utilities/personas/testing-engineer/`
- `utilities/personas/security-engineer/`
- `utilities/personas/frontend-engineer/`

Commands identify which corpus file to read. The corpus's inherited examples
are calibration; Landing's pattern docs remain the local authority.

## Synchronize roles

After changing a canonical command or `.codex/skill-meta.json`, run:

```sh
node utilities/sync-commands.mjs
node utilities/sync-commands.mjs --check
```

The check must pass before the role change is complete. Commit the canonical
source, generated targets, metadata, and lock file together.

Landing discovers personas from `.claude/agents/`. From the repository root,
list the synchronized names with:

```sh
landing persona list
```

## Documentation search

`.mcp.json` points the shared fat-docs MCP server at Landing's `docs/`
directory. The service is expected at `http://127.0.0.1:18800/mcp`.

Refresh the index from the repository root:

```sh
fat-docs index
```

Search from a terminal:

```sh
fat-docs search "routing tier"
```

Agents use the docs MCP search first and fall back to `rg` if the service is
unavailable. Search before creating a document so each concept keeps one home.

## Add a role

Add a role only when a durable surface or recurring judgment needs a distinct
owner. Create the canonical command, add metadata, synchronize, and update this
catalog. Do not create a role merely to mirror a folder or one-time task.
