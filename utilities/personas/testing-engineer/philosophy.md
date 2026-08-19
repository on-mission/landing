---
title: "Testing Philosophy for TypeScript Repositories — Confidence, Runtime Risk, and the Cost of Redundant Tests"
summary: >
  Primary-source corpus on why to test, what TypeScript already proves, what runtime behavior
  tests are for, and how to choose the right level of testing by confidence and cost rather than
  by dogma or coverage goals.
source_count: 12
---

# Testing Philosophy

Testing has one job: increase confidence that the software still works after you change it. Everything else is secondary. The useful question is not "do we have tests?" It is "what realistic mistake would this catch that our other mechanisms would miss?"

In a TypeScript repository, that question gets sharper because the compiler is already part of the testing story. Some facts are proven before runtime. Others are not provable until values actually cross a boundary, branches execute, time advances, or components interact. Good testing strategy starts by separating those two categories.

---

## 1. Tests are a cost paid for confidence

Kent Beck's most concise answer on test quantity is still one of the best:

> "I get paid for code that works, not for tests."
> "test as little as possible to reach a given level of confidence."
> — Kent Beck, Stack Overflow answer archived at https://stackoverflow.com/questions/1094580/when-stop-testing-using-tdd

That is not anti-test. It is anti-unpriced test accumulation. A test suite has real costs:

- authoring time;
- execution time in CI and locally;
- maintenance when code changes but behavior does not;
- review overhead;
- false confidence when tests are focused on the wrong thing.

Beck's framing gives a useful review question: if we removed this test, what confidence would disappear? If the honest answer is "none, because `tsc` already proves it" or "none, because the code is a trivial delegation," the test may be waste.

Martin Fowler describes the deeper payoff of test-first work as design feedback, not merely regression coverage:

> "writing tests first made a significant improvement to the design process."
> — Martin Fowler, *Test Driven Development*, https://martinfowler.com/bliki/TestDrivenDevelopment.html

That matters because some tests are valuable mainly because they force better seams. But once the seam exists, the ongoing test still has to earn itself by catching plausible bugs.

---

## 2. TypeScript already proves a meaningful subset of correctness

TypeScript is not runtime verification, but it is still a strong filter against certain categories of mistakes:

- required properties missing from values you construct;
- impossible branches over sealed unions;
- many refactor breakages caused by renamed fields or changed signatures;
- mis-typed calls across module boundaries that remain inside the typed world.

The handbook is explicit about the limit:

> "Type annotations never change the runtime behavior of your program."
> — TypeScript Handbook, *Basics*, https://www.typescriptlang.org/docs/handbook/2/basic-types.html

That sentence cuts both ways.

It means TypeScript cannot prove that incoming JSON matches your declared type. It also means a runtime test that merely reasserts compile-time facts is often weak value. If `parseUser` returns `User` only after Zod parsing, then a test saying "the result has an `id` field" is usually not the valuable test. The valuable test is that malformed boundary input is rejected, optional fields are normalized correctly, or branch logic routes the parsed value into the right downstream behavior.

For app code, `tsc --noEmit` is already a test. Treat it as one. Don't write a second, noisier test that asserts the same type-level fact through `expect(typeof x).toBe("string")`.

---

## 3. Valuable tests catch runtime behavior mistakes

Kent C. Dodds's practical question is confidence:

> "The thing you should be thinking about when writing tests is how much confidence they bring you that your project is free of bugs."
> — Kent C. Dodds, *Write tests. Not too many. Mostly integration.*, https://kentcdodds.com/blog/write-test

Confidence comes from catching mistakes that competent engineers still make:

- wrong branch logic;
- boundary-condition bugs;
- missing or misordered side effects;
- parser or validator mismatches;
- state transitions that should be illegal;
- integration seams where individually correct pieces do not cooperate;
- regressions on previously observed bugs.

This is where runtime tests matter. The compiler cannot tell you that February 29 handling is off by one day, that a Temporal workflow rejects a legal transition, or that a React form no longer submits after a refactor. Those are behavior questions.

Dodds also points out why isolated tests are not enough:

> "Integration tests strike a great balance on the trade-offs between confidence and speed/expense."
> — same source

That is not a universal command to maximize integration tests. It is a reminder that confidence is about behavior in coordination, not merely components in isolation.

---

## 4. Behavior beats implementation detail

Dodds's clearest argument against implementation-detail testing is not aesthetic. It is about error signal:

> "Can break when you refactor application code. False negatives"
> "May not fail when you break application code. False positives"
> — Kent C. Dodds, *Testing Implementation Details*, https://kentcdodds.com/blog/testing-implementation-details

That gives a durable review heuristic:

- if a test fails because a private state field got renamed but user-visible behavior is unchanged, the test is overspecified;
- if a test passes while the real workflow is broken because it only verified a mocked call, the test is underspecified.

Testing Library encodes the same principle in API design:

> "your test should resemble how users interact with your code"
> — Testing Library, *About Queries*, https://testing-library.com/docs/queries/about/

For React work, this is why `getByRole(..., { name })` is stronger than a CSS selector or DOM-shape assertion:

> "`getByRole` ... should be your top preference for just about everything."
> — same source

The philosophy generalizes beyond UI. At any layer, prefer asserting the observable contract over the internal mechanism, unless the mechanism *is* the contract.

---

## 5. Pyramid, trophy, and Google size are all trying to solve the same cost problem

The old test pyramid is a cost-and-speed heuristic. Dodds's "testing trophy" argument reacts against teams that overlearned "mostly unit tests" and underinvested in integration confidence. Google's small/medium/large framing reacts against terminology drift by focusing on speed and determinism.

Google's framing is operationally useful:

> "write the smallest possible test for a given piece of functionality."
> — *Software Engineering at Google*, Chapter 11, https://abseil.io/resources/swe-book/html/ch11.html

And the reason is explicit:

> "the most important qualities we want from our test suite are speed and determinism"
> — same source

Dodds's corrective is confidence:

> "As you move up the pyramid, the confidence quotient of each form of testing increases."
> — *Write tests. Not too many. Mostly integration.*, https://kentcdodds.com/blog/write-test

These are not contradictions. They are different angles on the same trade-off:

- narrower tests are cheaper and usually more precise;
- broader tests can buy more realistic confidence;
- expensive broad tests should be used where the failure cost justifies them.

The right shape is dictated by the cost of the failure and the cost of the test, not by a universal ratio.

---

## 6. Determinism is a first-class testing value

Google is unusually blunt about flaky tests:

> "If a large test is nonhermetic, it is almost impossible to guarantee determinism."
> — *Software Engineering at Google*, Chapter 14, https://abseil.io/resources/swe-book/html/ch14.html

And:

> "Flaky tests are tests that exhibit both a passing and a failing result with the same code."
> — Google Testing Blog, *Where do our flaky tests come from?*, https://testing.googleblog.com/2017/04/where-do-our-flaky-tests-come-from.html

Why this matters is not just annoyance. It is signal destruction:

> "We start viewing these tests as unreliable and eventually they lose their value."
> — same source

This is why "just retry it in CI" is not a testing strategy. It teaches the team that red may be meaningless. Determinism is not a nice-to-have. It is the condition that makes every other testing investment credible.

In practice, this means:

- own your data setup and cleanup;
- control time;
- control randomness or expose it as an input;
- avoid order dependencies;
- keep broad tests hermetic where possible;
- quarantine loudly if a flake must be temporarily isolated.

---

## 7. A TypeScript-aware default stance

For TypeScript repositories, the philosophy collapses into a useful default:

1. Let the compiler carry what the compiler can carry.
2. Spend runtime tests on behavior, boundaries, and interactions.
3. If the test is awkward, fix the code shape before inflating the harness.
4. Prefer the smallest deterministic test that catches the bug class you care about.
5. Escalate to integration and end-to-end only where their extra confidence buys something real.

When somebody asks "should we test this?", the first answer is often another question:

"What mistake are we trying to catch that TypeScript, the linter, or an existing integration test would not already catch?"

If there is a good answer, test it. If there is not, resist the urge to manufacture one.
