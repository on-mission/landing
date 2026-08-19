---
title: "Clean Architecture — SOLID, the Dependency Rule, and the Disagreement About DRY"
summary: >
  Classical architecture principles (SOLID, the Dependency Rule, boundaries, cohesion, coupling)
  as Uncle Bob stated them — and where modern front-end practice (Dan Abramov, Kent C. Dodds,
  Sandi Metz) pushes back on Clean-Code dogma around short functions, comments as failures, and
  duplication-as-enemy. Built from Uncle Bob's blog, Clean Code, Abramov's "Goodbye, Clean Code"
  and "The WET Codebase", Kent's AHA programming, and Sandi Metz's "The Wrong Abstraction".
source_count: 20
---

# Clean Architecture — SOLID, the Dependency Rule, and the Disagreement About DRY

> "The SOLID principles remain as relevant today as they were in the 90s." — Robert C. Martin

> "Let clean code guide you. Then let it go." — Dan Abramov

---

## 1. SOLID — the five principles in Uncle Bob's words

Canonical statements:

> "There should never be more than one reason for a class to change." — Martin, SRP

> "Software entities should be open for extension, but closed for modification." — Martin, OCP

> "Subtypes must be substitutable for their base types." — Martin, LSP

> "Clients should not be forced to depend upon interface methods that they do not use." — Martin, ISP

> "Depend upon abstractions, not concretes." — Martin, DIP

Restated in 2020 (*Solid Relevance*):

> "Gather together the things that change for the same reasons. Separate things that change for different reasons." — Martin on SRP

> "A Module should be open for extension but closed for modification." — Martin on OCP

> "A program that uses an interface must not be confused by an implementation of that interface." — Martin on LSP

> "Keep interfaces small so that users don't end up depending on things they don't need." — Martin on ISP

> "Depend in the direction of abstraction. High level modules should not depend upon low level details." — Martin on DIP

> "Simplicity requires disciplines guided by principles. It is those principles that define simplicity." — Martin

His clarifying SRP essay:

> "The Single Responsibility Principle (SRP) states that each software module should have one and only one reason to change." — Martin

> "This principle is about people." — Martin

> "When you write a software module, you want to make sure that when changes are requested, those changes can only originate from a single person, or rather, a single tightly coupled group of people representing a single narrowly defined business function." — Martin

> "A module should be responsible to one, and only one, actor." — Martin

---

## 2. SOLID applied to React

Uncle Bob wrote SOLID for class-based OO. The spirit translates cleanly; the mapping is different.

**SRP — Single Responsibility.** A component that fetches data, runs business logic, and renders markup answers to three actors — backend, product, design. Hooks exist to separate these axes of change. The `useX` seam IS the SRP seam.

**OCP — Open/Closed.** React's compositional APIs (`children`, render props, slots, prop getters, state reducers) let you add behavior via new code. A good component is extended via composition, not forked.

**LSP — Liskov Substitution.** Any two components implementing the same contract (same prop shape, same behavioral promises) should be swappable. Breaks when a "subclass" secretly requires extra props or coordinates through side-channels.

**ISP — Interface Segregation.** A context that exposes 20 fields forces every consumer to re-render on all of them. Prefer narrow hook APIs; split contexts by concern.

**DIP — Dependency Inversion.** Domain logic must not `import` from `react-dom`, a router, or a fetch library. Push those behind a port (a hook, a service module). The pure logic is testable without a renderer.

---

## 3. The Dependency Rule

The spine of *Clean Architecture*:

> "The overriding rule that makes this architecture work is The Dependency Rule." — Uncle Bob

> "This rule says that source code dependencies can only point inwards." — Uncle Bob

> "Nothing in an inner circle can know anything at all about something in an outer circle." — Uncle Bob

> "Entities encapsulate Enterprise wide business rules." — Uncle Bob

> "The software in this layer contains application specific business rules." — Uncle Bob, on Use Cases

> "No code inward of this circle should know anything at all about the database." — Uncle Bob

> "The Web is a detail. The database is a detail." — Uncle Bob

> "As you move inwards the level of abstraction increases." — Uncle Bob

### What this means for a React codebase

- `packages/models` and domain code have **zero** React dependency.
- React, router, fetch client, and UI libraries are **outer rings**.
- The **hook layer is the seam** — a hook wires React lifecycle to a pure function.
- If you can't extract the use case and run it in a Node test (no DOM, no router), your ring boundaries leak.

The most common violation: a component directly `import`ing a fetch library. The fix: push the call behind a hook/service.

---

## 4. Screaming Architecture — "the web is a detail"

> "The architecture would scream: house." — Uncle Bob, *Screaming Architecture*

> "So what does the architecture of your application scream?" — Uncle Bob

> "Architectures are not (or should not be) about frameworks." — Uncle Bob

> "Frameworks are tools to be used, not architectures to be conformed to." — Uncle Bob

> "If your architecture is based on frameworks, then it cannot be based on your use cases." — Uncle Bob

> "The Web is a delivery mechanism, and your application architecture should treat it as such." — Uncle Bob

> "Your business objects should be plain old objects that have no dependencies on frameworks or databases." — Uncle Bob

> "Your architectures should tell readers about the system, not about the frameworks you used in your system." — Uncle Bob

In a React project: the top-level folder structure shouldn't be `/components /hooks /pages /stores` (those are framework shapes). It should be `/invoices /customers /reports` (those are the use cases that are the reason the app exists).

---

## 5. Boundaries, coupling, cohesion

> "We want to increase the cohesion between things that change for the same reasons, and we want to decrease the coupling between those things that change for different reasons." — Uncle Bob

> "It is people who request changes. And you don't want to confuse those people, or yourself, by mixing together the code that many different people care about for different reasons." — Uncle Bob

> "In general the more variables a method manipulates the more cohesive that method is to its class." — Uncle Bob

### Front-end translation

- **Cohesion**: a feature folder (`features/invoices/`) that co-locates component + hook + types + API beats a codebase sliced by kind (`/components`, `/hooks`, `/api`). Things that change together belong together.
- **Coupling**: a prop drilled five levels couples the tree. Composition (lifting content as children), a scoped Context, or a feature-local store reduces it.
- **Boundaries**: package boundaries (`packages/frontend` ↔ `packages/api` ↔ `packages/models`) and feature boundaries inside an app. Enforce with lint rules — `features/X` must not `import` from `features/Y`.

---

## 6. The wrong abstraction costs more than duplication

Where Clean Code dogma breaks and modern practice swerves.

### Sandi Metz, *The Wrong Abstraction* (2016)

> "duplication is far cheaper than the wrong abstraction" — Metz

> "prefer duplication over the wrong abstraction" — Metz

The sequence: Programmer A sees duplication, extracts an abstraction. Programmer B has a case that's "almost" the same — adds a flag. C adds another. Eventually the shared function is a kitchen sink of conditionals.

> "A new requirement appears for which the current abstraction is almost perfect." — Metz

> "What was once a universal abstraction now behaves differently for different cases." — Metz

> "Loop until code becomes incomprehensible." — Metz

> "When dealing with the wrong abstraction, the fastest way forward is back." — Metz

> "Re-introduce duplication by inlining the abstracted code back into every caller." — Metz

> "If you find yourself passing parameters and adding conditional paths through shared code, the abstraction is incorrect." — Metz

> "This is not retreat, it's advance in a better direction." — Metz

### Kent C. Dodds, *AHA Programming*

> "AHA — Avoid Hasty Abstractions." — Kent C. Dodds

> "I'm fine with code duplication until you feel pretty confident that you know the use cases for that duplicate code." — Kent C. Dodds

> "the commonalities will scream at you for abstraction and you'll be in the right frame of mind" — Kent C. Dodds

### Dan Abramov, *Goodbye, Clean Code* (2020)

A direct rebuttal of Clean Code's "duplication is the primary enemy" stance:

> "The code worked. But it was repetitive. It wasn't clean." — Abramov

> "I had an idea. We could remove all duplication by grouping the code like this instead" — Abramov

> "Obsessing with 'clean code' and removing duplication is a phase many of us go through." — Abramov

> "'I'm the kind of person who writes clean code'. It's as powerful as any sort of self-deception." — Abramov

> "If someone tells us that abstraction is a virtue, we'll eat it." — Abramov

> "My code traded the ability to change requirements for reduced duplication, and it was not a good trade." — Abramov

> "I thought a lot about how the code looked — but not about how it evolved with a team of squishy humans." — Abramov

> "Don't be a clean code zealot. Clean code is not a goal." — Abramov

> "Rewriting your teammate's code without a discussion is a huge blow to your ability to effectively collaborate." — Abramov

> "Let clean code guide you. Then let it go." — Abramov

### *The WET Codebase* (Deconstruct 2019)

> "duplication isn't perfect in long term, but wrong abstraction is also not perfect in long term" — Abramov

> "Just because the structure of these two snippets looks similar, it might just mean that you don't really understand the problem yet" — Abramov

> "abstraction creates accidental coupling" — Abramov

> "we create this lasagna code where there are so many layers" — Abramov

> "abstraction also creates inertia in your code base" — Abramov

> "does your technology make it easier for you to get rid of them?" — Abramov's framework for evaluating abstractions

---

## 7. Uncle Bob on functions, names, comments

### Functions

> "The first rule of functions is that they should be small. The second rule of functions is that they should be smaller than that." — Martin

> "FUNCTIONS SHOULD DO ONE THING. THEY SHOULD DO IT WELL. THEY SHOULD DO IT ONLY." — Martin (caps in original)

> "Don't use flag arguments. Split method into several independent methods." — Martin

> "Have no side effects." — Martin

### Reading over writing

> "Indeed, the ratio of time spent reading versus writing is well over 10 to 1… making it easy to read makes it easier to write." — Martin

> "Clean code is simple and direct. Clean code reads like well-written prose." — Martin

### Naming

> "A long descriptive name is better than a short enigmatic name. A long descriptive name is better than a long descriptive comment." — Martin

> "Choose descriptive and unambiguous names. Make meaningful distinction. Use pronounceable names. Use searchable names." — Martin

### Comments as failures

> "The proper use of comments is to compensate for our failure to express ourself in code. Note that I used the word failure. I meant it. Comments are always failures." — Martin

> "Redundant comments are just places to collect lies and misinformation." — Martin

> "When you see commented-out code, delete it!" — Martin

> "Always try to explain yourself in code." — Martin

### Professionalism

> "Clean code is not written by following a set of rules. You don't become a software craftsman by learning a list of heuristics. Professionalism and craftsmanship come from values that drive disciplines." — Martin

> "Writing clean code is what you must do in order to call yourself a professional." — Martin

> "Truth can only be found in one place: the code." — Martin

> "It is not enough for code to work." — Martin

> "Leave the campground cleaner than you found it." — Martin, Boy Scout Rule

> "Duplication is the primary enemy of a well-designed system. It represents additional work, additional risk, and additional unnecessary complexity." — Martin

This last line is the one Abramov's *Goodbye, Clean Code* is a direct rebuttal of.

---

## 8. Disagreements — where modern practice diverges

Uncle Bob's **architectural spine** (SOLID, Dependency Rule, Screaming Architecture, boundaries, cohesion) is load-bearing.

Uncle Bob's **tactical Clean Code rules** (4-line functions, kill every comment, duplication is enemy #1) are where modern front-end practice has openly diverged.

### The clearest contrast

> "Duplication is the primary enemy of a well-designed system." — Uncle Bob

> "duplication is far cheaper than the wrong abstraction." — Sandi Metz

> "Writing clean code is what you must do in order to call yourself a professional." — Uncle Bob

> "Don't be a clean code zealot. Clean code is not a goal." — Dan Abramov

> "Comments are always failures." — Uncle Bob

Modern practice: comments are sometimes the only way to capture non-obvious WHY. They rot, and you delete them when they do. But the absolutist framing is wrong.

### Community critique

> "It's probably time to stop recommending Clean Code" — qntm

The qntm essay (widely circulated) points out that Clean Code's concrete examples often make code *worse* by over-extracting tiny functions that obscure control flow.

---

## 9. The synthesis

**Load-bearing (Uncle Bob, keep):**
- SOLID as a thinking tool.
- The Dependency Rule — domain code must not know about React, router, or DB.
- "The Web is a detail" — structure by use case, not by framework folder.
- Cohesion over coupling; boundaries; things that change together live together.
- "Make it easy to read" — the 10:1 read/write ratio.

**Where modern front-end disagrees (Abramov/Dodds/Metz, apply):**
- Duplication beats the wrong abstraction. Wait for the shape. Rule of Three / AHA.
- Comments are not "always failures" — they're necessary when the *why* is non-obvious. But most comments *are* noise and should be deleted.
- Short functions for their own sake can obscure logic. A straight-line 40-line function is often clearer than six 4-line functions that pass state between each other.
- "Rewriting teammate's code for cleanliness" without a conversation is a collaboration failure, not craftsmanship.

**Rule of thumb:** the architectural spine makes codebases last. The tactical dogma makes them smaller, more abstracted, harder to change. Keep the spine; discard the dogma.

---

## Sources

1. https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html
2. https://blog.cleancoder.com/uncle-bob/2011/09/30/Screaming-Architecture.html
3. https://blog.cleancoder.com/uncle-bob/2014/05/08/SingleReponsibilityPrinciple.html
4. https://blog.cleancoder.com/uncle-bob/2020/10/18/Solid-Relevance.html
5. https://blog.cleancoder.com/uncle-bob/2015/11/18/TheProgrammersOath.html
6. https://blog.cleancoder.com/uncle-bob/2017/02/23/NecessaryComments.html
7. https://www.goodreads.com/work/quotes/3779106-clean-code-a-handbook-of-agile-software-craftsmanship-robert-c-martin
8. https://gist.github.com/wojteklu/73c6914cc446146b8b533c0988cf8d29
9. https://cleancoders.com/episode/clean-code-episode-16
10. https://cleancoders.com/episode/clean-code-episode-11-p1
11. https://en.wikipedia.org/wiki/SOLID
12. https://overreacted.io/goodbye-clean-code/
13. https://www.deconstructconf.com/2019/dan-abramov-the-wet-codebase
14. https://kentcdodds.com/blog/aha-programming
15. https://sandimetz.com/blog/2016/1/20/the-wrong-abstraction
16. https://qntm.org/clean
17. https://news.ycombinator.com/item?id=12061453
18. https://news.ycombinator.com/item?id=22022466
19. https://kentcdodds.com/blog/when-to-break-up-a-component-into-multiple-components
20. https://blog.cleancoder.com/uncle-bob/
