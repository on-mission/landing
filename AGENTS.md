# Landing

Landing is an AI execution optimizer. Users and lead agents describe the work;
Landing selects an eligible execution path across models, providers, and agent
harnesses.

## Start with documentation

Documentation describes the intended product; inherited code may lag behind it.
Before changing behavior, read `docs/README.md`, search the relevant docs, and
resolve gaps through the owning author role.

Use the local docs search service when available. Fall back to `rg` when it is
not.

## Documentation ownership

- `docs/product/` — `product-author`
- `docs/architecture/` — `architecture-author`
- `docs/patterns/` — `pattern-author`
- `docs/runbooks/` — open to the agent doing the related operational work

Implementers do not edit author-owned docs incidentally. Surface the gap and
route it through the owner. Product docs describe user value and behavior;
architecture stays at the component and contract level; patterns contain
durable implementation rules; runbooks contain repeatable operations.

Keep every Markdown file under 500 lines. Architecture-specific limits live in
`docs/architecture/STRUCTURE.md`. Volatile CLI syntax belongs in code-owned help
and tests.

## Runbooks

Read the relevant runbook before operating project infrastructure:

- [Agent surfaces and docs search](docs/runbooks/agent-surfaces.md) — canonical
  Claude commands, generated Claude/Codex roles, specialist corpora, sync, and
  fat-docs indexing/search

## Implementation contract

The pattern catalog at `docs/patterns/overview.md` is binding for Landing's new
and changed Go code. Do not turn focused work into an unrelated cleanup sweep.

The repository carries inherited legacy JavaScript under `src/`, `bin/`, and
`scripts/`. It is not the product model and is tracked in `TODO.md`.

Do not add or change product code until the user asks for implementation.

## Working safely

- Protect unrelated and concurrent changes. Never discard work you did not
  create.
- Never run destructive Git operations without explicit human authorization.
- Supported provider integrations use official tooling or official APIs;
  Landing does not collect or imitate consumer credentials.
- Capacity probes are read-only and never spend credits or mutate provider
  state.

## Tooling

Landing uses Go: `go.mod`, `go build`, `go test`, and `go vet` are the
toolchain.
