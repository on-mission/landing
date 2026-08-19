# Architecture documentation structure

Architecture documentation is Landing's 1,000-foot view. It explains major
components, the contracts between them, durable invariants, and why boundaries
exist.

It does not document source files, functions, command flags, data fields, or
implementation steps. Those belong with the code and its tests.

## Shape

- `canvas.md` is the entry point and system map.
- Add another architecture page only when one canvas node needs durable context
  that cannot fit clearly in the canvas.
- `canvas.md` stays under 300 lines and 12 named nodes.
- Every other architecture page stays under 100 lines.
- Use present tense and explain rationale, not history.
- Mark a decided but unbuilt boundary with `[DEFERRED]`.
- Mark an unresolved architectural question with `[FLAG]` and route it to the
  architecture author rather than inventing an answer.

Less architecture documentation is better. A code change does not earn a new
page unless it changes a cross-component contract or durable invariant.
