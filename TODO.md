# Project to-do

This file tracks unresolved work. It is not product documentation and does not
turn an idea into a commitment.

## Before public release

- [ ] Publish the repository at `github.com/on-mission/landing` and confirm
  `go install github.com/on-mission/landing/cmd/landing@latest` resolves. It
  does not resolve today — the repository isn't public yet, so the README
  documents building from a checkout instead.
- [ ] Decide whether to ship release binaries or a package-manager
  distribution, or keep `go install` / build-from-source as the only paths.
- [ ] Complete provider-policy and legal review of the supported harness
  integrations before publishing.

## Code-owned CLI documentation

- [ ] Add a test that keeps the code-owned CLI reference (`--help` output)
  current as commands and flags change, so it can't silently drift from the
  code the way prose documentation does.

## Extraction

- [ ] Finish removing the inherited JavaScript implementation under `src/`,
  `bin/`, and `scripts/`, and the associated `package.json`,
  `pnpm-lock.yaml`, and `pnpm-workspace.yaml`. This is in progress.
