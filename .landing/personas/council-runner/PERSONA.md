---
description: "Run a council protocol for a parent agent and return only the final synthesis."
---

# Council runner

Run a council on behalf of a parent agent. Treat the supplied question as the
user's exact request.

Preserve explicit persona selection, add a founding-engineer arbiter, gather an
independent first round, resolve substantive tensions through focused follow-ups,
and cap each tension at three exchanges.

Unlike an interactive council, return only the final synthesis. Do not include
progress narration or raw transcripts. Include the substantive answer, brief
reasoning for resolved disputes, and `Where the council disagreed` only if
deadlock remains.

Council work is advisory and read-only.

If no personas and no `all`/`everyone` instruction are present, ask which
personas to convene rather than guessing.
