---
title: "Mocking and Test Doubles — Boundary Ownership, Fowler's Taxonomy, and When a Mock Is the Wrong Fix"
summary: >
  Primary-source guidance on test doubles in TypeScript repositories: what a mock is, what a
  stub is, where doubles help, where they distort confidence, and how to reason about owned
  boundaries versus external boundaries.
source_count: 10
---

# Mocking and Doubles

Most teams say "mock" when they mean "some replacement thing in a test." That imprecision is not harmless. It blurs what the test is actually proving.

Martin Fowler's taxonomy still matters because it forces the question: are we replacing a collaborator to control the world, to verify an interaction, to provide canned data, or to make the system faster and more deterministic? Different answers imply different confidence.

---

## 1. Start with Fowler's terms

Fowler's baseline:

> "mock objects are but one form of special case test object"
> — Martin Fowler, *Mocks Aren't Stubs*, https://martinfowler.com/articles/mocksArentStubs.html

And the family name:

> "Mocks and Stubs are two different kinds of Test Doubles"
> — Ham Vocke, Thoughtworks / Martin Fowler site, *The Practical Test Pyramid*, https://martinfowler.com/articles/practical-test-pyramid.html

In plain language:

- **Dummy**: passed only because something needs an argument.
- **Stub**: returns controlled data.
- **Fake**: working but simplified implementation, often in memory.
- **Spy**: records what happened so the test can inspect it.
- **Mock**: preprogrammed with expected interactions; failure is often about the interaction contract.

The vocabulary matters because "asserted the spy was called" and "exercised the real collaborator with a fake clock" are not equivalent kinds of evidence.

---

## 2. State verification vs behavior verification

Fowler's core distinction:

> "a distinction between state verification and behavior verification."
> — *Mocks Aren't Stubs*, https://martinfowler.com/articles/mocksArentStubs.html

State verification asks: after the behavior runs, is the resulting state/output correct?

Behavior verification asks: did we call collaborator X in the expected way?

For many app-level tests, state verification is stronger because it keeps the assertion closer to user-visible or system-visible behavior. Behavior verification earns its keep when the interaction itself is the contract:

- emitting the correct message to an outbound queue;
- calling a third-party API with the correct shape;
- recording an audit event;
- scheduling the next workflow step.

If the only reason for behavior verification is "the real collaborator was awkward to use," that is often a code-shape or test-design smell rather than proof that a mock is the right tool.

---

## 3. Mock the boundaries you do not own

Dodds's practical advice:

> "most of the time you can avoid mocking and you'll be better for it."
> — Kent C. Dodds, *Write tests. Not too many. Mostly integration.*, https://kentcdodds.com/blog/write-test

The good default is:

- mock or fake external services, time, randomness, and process environment when needed for determinism;
- avoid mocking your own modules when the real collaboration is cheap enough to exercise;
- prefer exercising real seams inside the repo, because that is where integration mistakes happen.

Playwright says the same thing from the browser-testing side:

> "Only test what you control."
> — Playwright Best Practices, https://playwright.dev/docs/best-practices

That means external sites and third-party servers are reasonable places to intercept or stub. Your own service layer usually is not.

---

## 4. Mocking your own code is often a smell

If the test mocks an internal module and then asserts the mock was called, ask what bug would make the real system fail while this test stays green. Usually the answer is "many."

Common failure pattern:

1. implementation calls collaborator with wrong transformed data;
2. mock is asserted only on call count or broad shape;
3. integration with the real collaborator is broken;
4. test still passes.

That is exactly the false-positive pattern Dodds warns about in implementation-detail tests: https://kentcdodds.com/blog/testing-implementation-details

When somebody says "we need to mock our own service because setting it up is painful," the better question is often:

"Why is the service so hard to exercise in-process?"

Sometimes the answer is valid. More often the answer is hidden global state, time, or overly broad orchestration.

---

## 5. Fakes and contract tests are the middle ground

Not every choice is "real production dependency" or "deep mock tree."

Useful middle grounds:

- **fake clock** instead of real sleeping;
- **in-memory fake** where the semantics are intentionally small and stable;
- **consumer/provider contract tests** where external integration is expensive but the boundary contract still needs verification.

Fowler's testing guide calls out contract tests as the answer when a double stands in for an external service and you still need confidence that the representation matches reality: https://martinfowler.com/testing/

This is especially useful when:

- the other side of the boundary is run by a different team;
- network calls are slow or unreliable;
- the contract is stable enough to encode.

But if both sides of the boundary live in the same repo and can be exercised together cheaply, a direct integration test is often better than a contract test plus multiple internal mocks.

---

## 6. Database boundaries

Integration with a database is exactly the sort of owned boundary where excessive mocking frequently lies to the team. Query behavior, transactions, constraints, indexes, locking, default values, and serialization details all live below the level a repository mock can honestly represent.

That is why many serious codebases adopt a rule against mocking the database in integration tests. Whether the rule is documented locally or only enforced culturally, the logic is the same:

- a mock repository proves your expectations about the repository API;
- a real database test proves that the query, schema, and transaction behavior actually cooperate.

If the database tests are too slow or too painful, the first fix is not "mock harder." The first fix is usually better fixtures, narrower test setup, or a cleaner test harness.

---

## 7. Time is a boundary too

Timer-heavy code is one of the few places where a fake or mock is routinely the right move because waiting in real time is wasted cost and a source of flakiness.

Jest's timer docs explain the motivation directly:

> "The native timer functions ... are less than ideal for a testing environment since they depend on real time to elapse."
> — Jest docs, *Timer Mocks*, https://jestjs.io/docs/timer-mocks

Google's older guidance is even sharper:

> "Sleeping != Synchronization"
> — Google Testing Blog, https://testing.googleblog.com/2008/08/tott-sleeping-synchronization.html

If a test sleeps, question it hard. A controllable clock or explicit latch is usually the right seam.

---

## 8. Practical rules

Use a double when it improves determinism, speed, or control of a boundary you do not own.

Avoid a double when:

- it replaces collaboration among code you do own;
- it turns a behavior test into an interaction-count test;
- it makes refactors expensive without increasing confidence;
- it lets a broken real integration hide behind a green suite.

Prefer:

- real collaborators for cheap in-repo seams;
- stubs/fakes for uncontrollable or expensive external boundaries;
- contract tests where the real external boundary cannot be exercised often;
- explicit clock/randomness seams for determinism.

The test is not "using mocks correctly" because the mocking library is sophisticated. The test is correct when the chosen double preserves the behavior claim the team actually cares about.
