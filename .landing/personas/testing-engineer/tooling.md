---
title: "Testing Tooling for TypeScript — Vitest, Jest, Playwright, Testing Library, Type Tests, and CI Determinism"
summary: >
  Tool-level guidance for modern TypeScript testing stacks. Covers when to use RTL, Playwright,
  Vitest/Jest timers and mocks, type-level testing with expectTypeOf and tsd, and how to keep CI
  deterministic and trustworthy.
source_count: 10
---

# Tooling

Framework choice matters less than discipline, but it does not matter zero. Each tool makes some kinds of tests easy and some kinds of mistakes easy to hide. This file is about using the mainstream TypeScript testing stack with the right expectations.

---

## 1. React Testing Library: test accessible behavior, not component internals

Testing Library's principle:

> "your test should resemble how users interact with your code"
> — https://testing-library.com/docs/queries/about/

And its query guidance is concrete:

> "`getByRole` ... should be your top preference for just about everything."
> — same source

That makes RTL strong for:

- component behavior visible in the DOM;
- accessible names, labels, and roles;
- form interactions via `user-event`;
- async UI state changes;
- regression tests for user-visible bugs.

It is weak by design for:

- private state assertions;
- instance methods;
- DOM topology snapshots with no behavioral claim.

If a React test feels hard in RTL, first ask whether you are trying to test an implementation detail. Dodds's "false negatives / false positives" warning usually applies there: https://kentcdodds.com/blog/testing-implementation-details

---

## 2. Playwright: reserve it for critical cross-browser user paths

Playwright's own guidance starts with behavior:

> "Test user-visible behavior"
> — Playwright Best Practices, https://playwright.dev/docs/best-practices

And isolation:

> "Each test should be completely isolated from another test"
> — same source

Good Playwright targets:

- sign-in and sign-out;
- checkout or payment-critical paths;
- permission-gated flows;
- cross-browser regressions;
- workflows whose correctness depends on the full browser/runtime stack.

Bad Playwright targets:

- every branch of a pure formatter;
- internal component logic already covered by RTL or a pure helper test;
- third-party sites you do not control.

Playwright is expensive relative to unit or seam-level integration tests. Spend that expense where it buys confidence you cannot get more cheaply.

---

## 3. Vitest and Jest: runner choices, not philosophy choices

Vitest and Jest both support the common unit/integration-style test patterns. The meaningful questions are usually:

- does the repo already use one;
- what fake timer semantics exist;
- what mocking APIs are available;
- how type tests integrate with the runner.

Jest is explicit about fake timers:

> "The native timer functions ... are less than ideal for a testing environment since they depend on real time to elapse."
> — Jest docs, https://jestjs.io/docs/timer-mocks

That is a reminder to control time rather than sleep. Use fake timers when the contract is time-based and the code is designed to accept that seam cleanly.

Do not let runner convenience become design laziness. Easy mocks are still capable of creating bad tests.

---

## 4. Type-level tests: only when the type is the contract

Vitest supports type tests directly:

> "Vitest allows you to write tests for your types, using `expectTypeOf` or `assertType` syntaxes."
> — Vitest docs, https://main.vitest.dev/guide/testing-types.html

It also states what those tests are:

> "they are only statically analyzed by the compiler."
> — same source

Likewise, `tsd` is for declaration/type-definition testing:

> "Check TypeScript type definitions"
> — `tsd` README, https://github.com/tsdjs/tsd

And:

> "These `.test-d.ts` files will not be executed"
> — same source

This is excellent when:

- you ship a library type API;
- the type inference behavior *is* the user contract;
- you maintain utility types;
- you expose generated client types that consumers rely on.

It is usually overkill when:

- the repo is an application;
- `tsc --noEmit` already verifies the normal code surface;
- the "type test" is trying to prove a runtime behavior question.

Type tests are real tests, but they are compiler tests. Use them for compiler-facing contracts.

---

## 5. Boundary-specific tools

### Timers and clocks

Use fake timers or injected clocks to avoid real waiting. Tests that sleep are slow and often flaky.

### Temporal

Temporal's TypeScript SDK includes `@temporalio/testing`, which is the right seam for workflow/activity behavior when you need Temporal-aware integration rather than ad hoc mocks: https://nodejs.temporal.io/

### Database-backed code

For DB-owning code, the high-value integration test is usually against a real database harness, not a repository mock. The tool may be Drizzle test setup, a local Postgres harness, or repo-specific fixtures. The principle is constant: use the real semantics where query behavior matters.

---

## 6. CI determinism and flake handling

Google's guidance is operationally direct:

> "automated tests that do not provide a consistent signal will slow down the entire development process."
> — Google Testing Blog, *Test Flakiness*, https://testing.googleblog.com/2020/12/test-flakiness-one-of-main-challenges.html

And their analysis shows the larger trend:

> "There's a clear increase in flakiness from small to medium and from medium to large."
> — Google Testing Blog, *Where do our flaky tests come from?*, https://testing.googleblog.com/2017/04/where-do-our-flaky-tests-come-from.html

Practical implications:

- prefer smaller deterministic tests when they cover the same bug class;
- isolate data per test;
- avoid hidden dependencies on execution order;
- control clocks and async boundaries;
- quarantine and root-cause flakes instead of normalizing them through automatic reruns.

If a test fails only in CI, the answer is not "CI is weird." The answer is usually "the test or the system has a hidden dependency."

---

## 7. Coverage is a diagnostic, not a goal

None of these tools can rescue a bad objective. Coverage reports can tell you where nothing is exercising code. They cannot tell you whether the exercised code is being tested for the right thing.

High coverage with implementation-detail tests can still produce a fragile, low-confidence suite. Low coverage can still be acceptable in low-risk or compiler-proven zones. The metric is useful only when interpreted through risk and behavior.

---

## 8. Practical tool selection defaults

For a modern TypeScript app:

- use pure function tests plus tables for branch logic;
- use RTL for component behavior;
- use real seam integration tests for DB / workflow / endpoint boundaries;
- use Playwright sparingly for critical user-visible flows;
- use fake timers or injected clocks for time;
- use `expectTypeOf` or `tsd` only when type behavior itself is the contract;
- keep CI deterministic, fast enough, and worthy of trust.

The tool does not make the test valuable. The value comes from asking the right behavioral question and then choosing the cheapest trustworthy mechanism to answer it.
