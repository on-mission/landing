---
title: TypeScript Expert Corpus
summary: Source corpus for the `/typescript-engineer` command. A reasoning-first reference on the TypeScript type system, domain modeling, compiler strictness, boundaries, and scale — grounded in primary sources from the TS team, Vanderkam (Effective TypeScript), Pocock (Total TypeScript), Basarat (TS Deep Dive), and large-codebase engineering posts (Bloomberg, Airbnb, Slack, Asana). Pros and cons of patterns at scale, not style rules.
---

# TypeScript Expert Corpus

Reference material for the `/typescript-engineer` Claude command (`.claude/commands/typescript-engineer.md`). The command embodies the collected reasoning of the most reputable TypeScript voices and consults these files before answering. The corpus deliberately excludes formatting, naming, and project-style rules — those belong in project-level pattern docs. What lives here are the design decisions, soundness trade-offs, and scaling consequences that make TypeScript code reviewable.

| File | Focus | Lines | Sources |
|---|---|---:|---:|
| [type-system.md](./type-system.md) | Design philosophy, structural typing, soundness non-goal, `any`/assertions, variance, inference, narrowing, type-space vs value-space | 321 | 26 |
| [modeling.md](./modeling.md) | Discriminated unions, valid states, brands, the golden rule of generics, conditional/mapped types, `interface` vs `type`, enums, optionals, the cleverness cliff | 289 | 22 |
| [strictness.md](./strictness.md) | `--strict` family, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, module/emit flags, build-perf flags, the strictness migration pattern | 293 | 24 |
| [boundaries.md](./boundaries.md) | Compile-time / runtime gap, `unknown` at the boundary, parse-don't-validate, schema-as-source-of-truth (Zod, io-ts), type predicates as contracts, error modeling, `.d.ts` discipline, inferred types as APIs | 284 | 23 |
| [scale.md](./scale.md) | Project references, `skipLibCheck`, the `tsc` cost model, type-checking separated from build, `isolatedDeclarations`, the Go-based native compiler (TS 7.0), tsserver perf, typed linting, type tests, dead-code detection, dual-package types | 303 | 26 |

**Total:** ~1,490 lines, 121 distinct sources. Every claim is backed by a verbatim quote with URL citation. Style/formatting prescriptions (Google TS Style Guide, Airbnb naming rules) are deliberately excluded.

## Source tier

1. **TypeScript team** — Anders Hejlsberg, Daniel Rosenwasser, Ryan Cavanaugh; release notes, the Performance wiki, Design Goals, the Handbook, the TSConfig reference.
2. **Dan Vanderkam** — *Effective TypeScript* book and blog (effectivetypescript.com).
3. **Matt Pocock** — Total TypeScript (totaltypescript.com), tsconfig cheat sheet, TS expert interviews.
4. **Basarat Ali Syed** — *TypeScript Deep Dive* (basarat.gitbook.io).
5. **Schema-library authors** — Colin McDonnell (Zod), Giulio Canti (io-ts).
6. **Engineering blogs at scale** — Bloomberg, Airbnb, Slack, Asana, typescript-eslint.
7. **Foundational essays** — Alexis King's *Parse, don't validate*.
