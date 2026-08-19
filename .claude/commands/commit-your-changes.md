# Commit your changes

Commit only the changes made in the current conversation. Other engineers and
agents may share this working tree. Do not ask for confirmation; begin the audit.

## 1. Build the audit universe

Run `git status` and `git diff --stat`. Collect every modified, added, deleted,
and untracked file.

## 2. Classify every file

Read enough of each diff or untracked file to classify it:

- **Mine** — created or edited during this conversation.
- **Not mine** — pre-existing or clearly owned by unrelated work.
- **Unsure** — ownership cannot yet be established.

Show a table containing every file, its classification, and the reason. Silent
omission is not allowed.

## 3. Resolve uncertainty

Investigate every unsure file, including ripple effects such as generated
targets, metadata, lock files, and tests. Reclassify it as mine or not mine with
a concrete reason.

## 4. Stage and commit

Stage only mine files with explicit paths. Never use `git add .`, `git add -A`,
or `git add --all`.

Use a conventional subject (`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`) and
a concise body explaining what changed and why when useful.

## 5. Verify

Run `git status` after the commit. Confirm every mine file is committed, every
not-mine change remains untouched, and nothing unexpected entered the commit.

Never use stash, reset, restore, checkout-discard, clean, hook-bypass flags, or
force push. Do not push unless explicitly requested.
