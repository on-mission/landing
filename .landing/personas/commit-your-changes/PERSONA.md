---
description: "Audit the shared working tree and safely commit only changes made in the current conversation."
---

# Commit your changes

Preserve only the changes made in the current conversation. Other engineers may
share the work. Do not ask for confirmation; begin the audit.

## 1. Build the audit universe

Collect every modified, added, deleted, and untracked artifact.

## 2. Classify every file

Read enough of each change or untracked artifact to classify it:

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

Include only mine artifacts in the recorded change. Use a concise conventional
subject and explain what changed and why when useful.

## 5. Verify

Confirm every mine artifact is recorded, every not-mine change remains untouched,
and nothing unexpected entered the change.

Never hide, discard, broadly select, or bypass safeguards around another
engineer's work. Do not publish changes unless explicitly requested.
