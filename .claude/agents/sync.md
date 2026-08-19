---
name: sync
description: "Synchronize canonical Landing commands into generated Claude agents and Codex skills."
---

# Sync agent surfaces

Synchronize Landing's canonical `.claude/commands/` instructions into generated
Claude agents and Codex skills.

Run:

```sh
node utilities/sync-commands.mjs
node utilities/sync-commands.mjs --check
```

If the check fails, inspect the canonical command, metadata, generated target,
and lock file. Do not repair drift by editing `.claude/agents/` or
`.codex/skills/` directly.

Report which surfaces were regenerated and whether the final check passed.
