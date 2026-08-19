---
title: "What to Test in a TypeScript Codebase — High-Value Targets, Low-Value Noise, and Refactor-Before-Test Signals"
summary: >
  Concrete guidance on what deserves tests in a TypeScript repository, what usually does not,
  and the common smells that indicate the code shape should change before the test suite grows.
source_count: 11
---

# What to Test

The highest-value testing question is not "what framework should we use?" It is "what in this change can fail at runtime in a way TypeScript will not prevent?" Once you answer that, the right test shape usually gets smaller and clearer.

This file is intentionally specific. It assumes an app-oriented TypeScript codebase where the compiler, runtime validators, and integration seams all matter.

---

## 1. High-value targets

### Pure functions and reducers

Pure branching logic is cheap to test and easy to trust. It should usually be the first place you harvest confidence from a change.

Why it is high value:

- the runtime behavior is real, not duplicated from the type system;
- the input/output surface is explicit;
- cases can be parameterized cleanly;
- failures are precise.

These are ideal for table-driven tests. If you have multiple cases and are writing one `it(...)` per branch, step back and build a case table instead.

### Parsers and validators at boundaries

External input is where the typed world stops. Request payloads, webhook bodies, queue messages, environment variables, database rows after raw SQL, LLM output, and third-party API payloads all arrive untrusted.

TypeScript cannot prove those values are valid at runtime. Tests here should focus on:

- representative good inputs;
- representative bad inputs;
- invariants after normalization;
- bug-regression cases for previously malformed payloads.

The TypeScript handbook's "types don't affect runtime" rule is the reason these tests exist at all: https://www.typescriptlang.org/docs/handbook/2/basic-types.html

### State machines and workflow step logic

If the code describes allowed and forbidden transitions, test those transitions directly.

That usually means:

- every documented legal transition succeeds;
- every illegal transition is rejected;
- state-derived side effects happen only on the intended path.

This is especially important when the runtime model is richer than the type model. A discriminated union can represent the *shape* of states, but not always the full behavioral legality of moving between them.

### Integration seams you actually depend on

Dodds's core point is that integration often buys disproportionate confidence:

> "you often don't need to bother testing them in isolation."
> — Kent C. Dodds, *Write tests. Not too many. Mostly integration.*, https://kentcdodds.com/blog/write-test

Good integration targets in this codebase shape:

- DB-backed services and repositories;
- Temporal workflow/activity seams;
- tRPC endpoint resolvers and service integration;
- HTTP clients/adapters at owned boundaries;
- serialization and deserialization boundaries.

Test where the pieces really meet. Do not replace the seam with mocks and then claim you have integration confidence.

### Regressions on specific bugs

Regression tests are some of the easiest tests to justify because they answer a concrete question: "did we ever break this exact thing before?" If yes, and the bug is subtle enough to recur, lock it in.

Be specific. The best regression test is small, named after the failure mode, and tied to the observed scenario rather than a giant end-to-end script that happens to cover it.

---

## 2. Lower-value or usually-noisy targets

### Trivially correct typed code

Examples:

- a one-line re-export or delegation with no branching;
- a typed mapper that renames fields with no conditions and no boundary crossing;
- a constant;
- a selector that simply returns a field.

Kent Beck's confidence framing applies here: if you do not typically make mistakes of this kind, and the compiler already constrains the implementation, a test may not buy enough signal to cover its maintenance cost.

### Reasserting type facts at runtime

Smells:

- `expect(typeof result.id).toBe("string")` when `result` is already typed and locally constructed;
- "returns all required fields" tests after a fully typed object literal;
- exhaustiveness tests for a sealed discriminated union when the compiler already fails non-exhaustive code paths.

If the type is the public contract of a library utility, type tests can make sense. For app code, this is usually duplicate proof.

### Snapshot tests for moving behavior

Snapshot tests have a narrow good use case: serialization invariants or small, intentionally stable render outputs. Outside that, they often capture too much noise and too little intent.

A snapshot that changes because the DOM structure moved but user behavior did not is not strong confidence. A snapshot that stays green while a click path stopped working is worse.

### Framework behavior

Do not spend test budget proving React, Zod, Vitest, Playwright, or your router basically work. Test your contract with them. If your component renders a button and clicks save, test that. Don't test that `useState` updates state or that a built-in matcher behaves as documented.

---

## 3. React and endpoint-specific guidance

### React components

Testing Library's principle is direct:

> "your test should resemble how users interact with your code"
> — Testing Library, https://testing-library.com/docs/queries/about/

Good React tests assert:

- visible content;
- accessible roles and names;
- user-event interactions;
- state reflected in the DOM;
- async updates observable by the user.

Bad React tests assert:

- internal state names;
- refs;
- component instance methods;
- child-component implementation details;
- brittle DOM trees or CSS-class topology.

Playwright says the same for browser-level tests:

> "Automated tests should verify that the application code works for the end users, and avoid relying on implementation details"
> — Playwright Best Practices, https://playwright.dev/docs/best-practices

### tRPC resolvers and endpoint logic

If the codebase exposes resolver functions separately from the HTTP layer, test those first. They usually give most of the value with less ceremony:

- schema/authorization/service orchestration can be exercised directly;
- the full network surface can be reserved for a few path-critical end-to-end cases.

This is the general lesson of subcutaneous/component testing in Fowler's taxonomy: stay just under the expensive outer shell when you can still test the real contract.

---

## 4. Test smells worth calling out in review

Kent C. Dodds's implementation-detail critique gives two recurring smell outcomes:

> "False negatives"
> "False positives"
> — *Testing Implementation Details*, https://kentcdodds.com/blog/testing-implementation-details

In practice, that surfaces as:

- tests that fail on harmless refactors;
- tests that pass while user-visible behavior is broken;
- tests with mocks for every collaborator and assertions only on call counts;
- "should fail" tests that never actually hit the failure path;
- hidden data coupling between test cases;
- broad snapshots blessed without inspection;
- large end-to-end tests used to cover logic that should have been a pure function test.

Google adds another smell class: flakiness caused by test-owned assumptions:

> "Invalid assumptions about the state of test data."
> "Dependencies on the order in which the tests are run."
> — Google Testing Blog, *Test Flakiness*, https://testing.googleblog.com/2020/12/test-flakiness-one-of-main-challenges.html

Those are not random environmental facts. They are design failures in the test.

---

## 5. Refactor-before-test signals

Sometimes the right review comment is not "add a test." It is "reshape this so a good test is possible."

Signals:

- hidden time or randomness inside the function;
- branching logic mixed with IO;
- five internal collaborators that all need to be mocked;
- setup that is 30 lines long to test 3 lines of logic;
- the only way to verify the behavior is through deep framework internals.

Fowler's TDD description points at why this matters:

> "thinking about the test first forces us to think about the interface"
> — Martin Fowler, *Test Driven Development*, https://martinfowler.com/bliki/TestDrivenDevelopment.html

If the interface is telling you the function is impossible to exercise without scaffolding a small universe, believe it. Split orchestration from calculation. Pass dependencies as parameters. Move normalization logic into a pure helper. Expose a seam where deterministic inputs can be supplied.

The goal is not "make this mockable." The goal is "make the behavior easy to state and exercise."

---

## 6. A practical filter

Before adding a test in a TypeScript app, ask:

1. What runtime bug would this catch?
2. Would `tsc`, linting, or an existing higher-level test already catch it?
3. Is the code shape forcing this test to be harder than it should be?
4. Is there a smaller deterministic seam that proves the same thing?
5. If this test fails after a refactor, will I trust the failure?

If you cannot answer those cleanly, the test idea is underdesigned. Fix the reasoning before you add the file.
