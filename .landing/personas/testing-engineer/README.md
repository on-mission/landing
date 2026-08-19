---
title: Testing Engineer Corpus
summary: Source corpus for the `/testing-engineer` command. A TypeScript-aware testing reference on confidence, what TypeScript already proves, what behavior-level tests are worth their cost, mocking discipline, parameterization, determinism, and framework-level practice.
---

# Testing Engineer Corpus

Reference material for the `/testing-engineer` command (`.claude/commands/testing-engineer.md`). The command embodies a particular testing philosophy: tests are a maintenance cost paid for confidence, TypeScript already proves a meaningful subset of correctness, and the best testing move is often to improve the code shape before expanding the harness.

This corpus is deliberately narrow. It is not a catalog of every testing trick. It focuses on the judgments that matter in a modern TypeScript repository:

- what runtime mistakes are worth spending tests on;
- what *not* to test because the compiler already proves it or the code is trivial;
- when a hard-to-test function is telling you the design is wrong;
- how to use mocks, tables, properties, and framework tools without turning them into ceremony.

The corpus is grounded in primary sources and near-primary practice:

- Kent Beck on confidence and TDD as a design tool.
- Kent C. Dodds and Testing Library on behavior over implementation details.
- Martin Fowler on test doubles, integration seams, and test taxonomy.
- Google on speed, determinism, and the operational cost of flakes.
- fast-check, Vitest, Jest, Playwright, and tsd on tool-level constraints.
- TypeScript's own handbook where the type/runtime split matters.

| File | Focus | Sources |
|---|---|---:|
| [philosophy.md](./philosophy.md) | Confidence, cost, compiler-vs-runtime proof, pyramid/trophy/Google lenses, determinism as a value | 12 |
| [what-to-test.md](./what-to-test.md) | High-value test targets, low-value test targets, smells, refactor-before-testing guidance | 11 |
| [mocking-and-doubles.md](./mocking-and-doubles.md) | Test-double taxonomy, boundary ownership, contract testing, state vs behavior verification | 10 |
| [parameterization-and-properties.md](./parameterization-and-properties.md) | Table-driven tests, properties, invariants, pure-function-first design, fast-check | 8 |
| [tooling.md](./tooling.md) | Vitest, Jest, Playwright, RTL, type tests, fake timers, CI determinism, flake handling | 10 |

Read order for humans:

1. Start with [philosophy.md](./philosophy.md) if the question is "should we even test this?"
2. Move to [what-to-test.md](./what-to-test.md) if the question is "what shape of test is worth writing?"
3. Read [mocking-and-doubles.md](./mocking-and-doubles.md) before introducing new mocks or spies.
4. Read [parameterization-and-properties.md](./parameterization-and-properties.md) when the case count is growing.
5. Use [tooling.md](./tooling.md) when you already know the testing goal and need the right mechanism.

The command is opinionated because the sources are opinionated. That is the point. Generic advice like "write good tests" or "increase coverage" is intentionally excluded.
