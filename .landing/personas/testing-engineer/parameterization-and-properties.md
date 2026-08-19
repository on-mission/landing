---
title: "Parameterized and Property-Based Testing — Table-Driven Defaults, Invariants, and Pure-Function-First Design"
summary: >
  Guidance on case tables and property-based testing in TypeScript repositories: when to use
  parameterized tests, how to choose invariants, and why pure, deterministic code shape is the
  prerequisite for simple high-signal tests.
source_count: 8
---

# Parameterization and Properties

A lot of test noise comes from writing one test per example when the real behavior is a single rule over a matrix of cases. Parameterization fixes that. Property-based testing pushes the same idea further: state the invariant once, generate many cases automatically, and let the tool shrink failures to the smallest counterexample.

Both approaches depend on the same underlying virtue: explicit inputs and deterministic behavior.

---

## 1. Table-driven tests should be the default for branching functions

When a function has multiple meaningful cases, a case table is usually better than a pile of similar `it(...)` blocks.

Why:

- the axes of variation become explicit;
- gaps are easier to spot;
- the assertion stays aligned across cases;
- adding a new edge case usually means a new row, not a new mini-test harness.

This is especially valuable in TypeScript where many logic-heavy helpers are already close to pure. The compiler gives you a stable surface; the table gives you behavioral coverage without repetition.

Good table candidates:

- parser normalization rules;
- reducers;
- status derivation functions;
- permission matrices;
- retry/backoff calculations;
- formatting logic with boundary conditions.

If you are copying the same setup three times with different inputs, you probably want a table.

---

## 2. Pure-function-first is what makes parameterization cheap

Parameterization is easy when the system under test:

- takes its dependencies as arguments;
- does not read time internally;
- does not call random or global state directly;
- returns an output rather than mutating a hidden collaborator.

Fowler's TDD note about tests shaping interfaces matters here:

> "thinking about the test first forces us to think about the interface"
> — Martin Fowler, *Test Driven Development*, https://martinfowler.com/bliki/TestDrivenDevelopment.html

The interface you want is the one where a table row can fully describe the case. If your table row needs six mocks and a setup function, the code likely wants to be split.

The testing lesson is design-driven:

- push branching logic downward into pure helpers;
- pull side effects upward into orchestration layers;
- pass clocks, random sources, or IO abstractions explicitly when needed.

Then the cheapest, clearest test becomes available.

---

## 3. Property-based testing is for invariants stronger than your examples

fast-check's introduction is the cleanest summary:

> "Instead of asking you to hand-pick the inputs your tests run on, it generates them for you, runs your assertion over hundreds of cases and shrinks failures down to the smallest inputs that still reproduces them."
> — fast-check docs, https://fast-check.dev/docs/introduction/

This is useful when the behavior you care about is not best expressed as a finite example list:

- encode/decode roundtrips;
- parser/serializer consistency;
- sorting/idempotence invariants;
- commutativity/associativity where they actually matter;
- generated-data edge cases that humans forget to enumerate;
- state machine model properties.

Hillel Wayne frames the hard part well:

> "What are good properties to test?"
> — Hillel Wayne, *Property Testing with Complex Inputs*, https://www.hillelwayne.com/post/property-testing-complex-inputs/

That is the right caution. Property-based testing is not magic bug dust. A weak property gives broad but shallow reassurance. A strong property can find classes of bugs example tests routinely miss.

---

## 4. Choose properties that express semantics, not coincidences

Good properties tend to be about invariants the domain actually promises:

- parsing then rendering preserves meaning;
- normalization is idempotent;
- applying permission filtering never produces unauthorized items;
- a stable sort preserves membership and order constraints;
- a state transition system never enters illegal states under legal commands.

Weak properties tend to be accidental:

- "the output is an array";
- "the result is defined";
- "the function doesn't throw" when throwing is allowed and meaningful.

If the property is so weak that a broken implementation still satisfies it, the generated breadth does not help.

fast-check's own docs emphasize concrete bug-finding value:

> "the concrete bugs this approach surfaces that example-based tests miss"
> — fast-check docs, https://fast-check.dev/docs/introduction/

That is the bar. Not novelty. Not sophistication. Actual bug classes.

---

## 5. Keep properties deterministic in CI

Random generation scares teams because they expect irreproducibility. A serious property-testing library addresses that with replay and shrinking. fast-check explicitly positions stability as a concern and not an afterthought:

> "property based testing already performs in a stable way"
> — fast-check blog, *Detect prototype pollution automatically*, https://fast-check.dev/blog/2023/09/21/detect-prototype-pollution-automatically/

Still, the discipline is on the team:

- log seeds or replay paths when failures occur;
- keep generators domain-relevant;
- avoid uncontrolled async behavior inside properties;
- use properties where the runtime cost is worth the search-space gain.

If the property routinely fails in ways engineers cannot reproduce or understand, the issue is usually generator design or an overly broad system under test.

---

## 6. When not to use properties

Do not reach for property-based testing when:

- the domain invariant is unclear;
- three example rows would be enough;
- the system under test is mostly side effects and orchestration;
- the team will not maintain the generators;
- a narrow regression test would communicate the failure better.

The point is not to maximize abstraction in the test suite. The point is to capture the behavior with the smallest, clearest artifact that keeps paying dividends.

---

## 7. Practical default

For a TypeScript repository:

1. If the function has multiple explicit branches, start with a case table.
2. If the real claim is an invariant over a wide input space, consider a property.
3. If either approach feels cumbersome, refactor toward explicit inputs and deterministic outputs first.

Parameterized tests are not just a syntactic convenience. They are a pressure toward honest surfaces. Property tests are not just "lots of random inputs." They are a way to encode domain truths once and force the code to survive many adversarial examples.
