---
title: Front-End Expert Corpus
summary: Source corpus for the `/frontend-engineer` command. A composite voice of modern React architecture — Dan Abramov, Kent C. Dodds, Sebastian Markbåge, Ryan Florence, Tanner Linsley, Mark Erikson, Sandi Metz, and Robert C. Martin — compiled from their own essays, talks, docs, and gists.
---

# Front-End Expert Corpus

Reference material for the `/frontend-engineer` Claude command (`.claude/commands/frontend-engineer.md`). The command acts as a composite front-end architecture authority and consults these files before answering.

The voices in this corpus:

- **Dan Abramov** — React core alum; the dominant mental-model voice (overreacted.io, react.dev).
- **Kent C. Dodds** — patterns, testing, state management; Epic React, kentcdodds.com.
- **Sebastian Markbåge** — foundational React rules and API-design principles.
- **Ryan Florence / Michael Jackson** — Remix, React Router; web-platform pragmatism.
- **Tanner Linsley / TkDodo** — TanStack Query; the server-state framing.
- **Mark Erikson** — Redux maintainer; measured "when to reach for Redux" perspective.
- **Sandi Metz** — *The Wrong Abstraction*; AHA's older counterweight.
- **Robert C. "Uncle Bob" Martin** — SOLID, Clean Architecture, Dependency Rule.

| File | Focus | Lines | Sources |
|---|---|---:|---:|
| [react-mental-models.md](./react-mental-models.md) | UI as a tree, pure components, state as a snapshot, effects as synchronization, position-based identity, RSC / "two computers" | 357 | 30 |
| [component-design.md](./component-design.md) | Composition over inheritance, custom hooks, compound components, render props, state reducers, AHA, Thinking in React | 341 | 22 |
| [state-and-data.md](./state-and-data.md) | Server vs client state, colocation, lifting, URL-as-state, Redux debates, Context ≠ state management, Remix data flow | 328 | 29 |
| [clean-architecture.md](./clean-architecture.md) | SOLID, Dependency Rule, Screaming Architecture, cohesion/coupling, The Wrong Abstraction, disagreements with Clean Code dogma | 339 | 20 |
| [anti-patterns.md](./anti-patterns.md) | useEffect overreach, premature memoization, Context abuse, global state, testing implementation details, derived state, clean-code cargo cult | 399 | 20 |

**Total:** ~1,764 lines across 5 topic files, 121 sources. Each file kept under 400 lines for readability; verbatim quotes and source URLs preserved.
