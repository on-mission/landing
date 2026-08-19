---
title: "Boundaries — Public APIs, Runtime Validation, the Compile-Time/Runtime Gap, and Error Modeling"
summary: >
  Primary-source corpus on what TypeScript cannot guarantee — the values that cross
  network, JSON, env, IPC, and FormData boundaries — and the disciplines that make those
  boundaries safe: parse-don't-validate, schema-as-source-of-truth (Zod, io-ts), unknown
  vs any vs never, type predicates and assertion functions as auditable contracts, error
  modeling, declaration files, and inferred types as public APIs (tRPC). Drawn from the
  TypeScript handbook, design goals, Cavanaugh, Vanderkam, Pocock, Basarat, Alexis King,
  Colin McDonnell (Zod), Giulio Canti (io-ts), Effect docs, and the DefinitelyTyped guide.
source_count: 23
---

# Boundaries

> "Add or rely on run-time type information in programs, or emit different code based on results of the type system." — TypeScript Design Non-Goal

The compiler cannot help across a boundary. Every external input — network response, `JSON.parse` result, environment variable, IPC message, FormData, file I/O — arrives at runtime as a value with no type. Whatever type the codebase claims it has is a *promise* that has to be enforced *somewhere* by code, not by the compiler. Boundaries are where bugs cluster and where strictness pays the most.

---

## 1. The compile-time / runtime gap

The TypeScript design goals list two non-goals that together define the gap:

> "Use a consistent, fully erasable, structural type system."
> "Add or rely on run-time type information in programs, or emit different code based on results of the type system." *(non-goal)*
> "Apply a sound or 'provably correct' type system." *(non-goal)*
> — *TypeScript Design Goals*, https://github.com/microsoft/TypeScript-wiki/blob/main/TypeScript-Design-Goals.md

Vanderkam's framing is the clearest:

> "Every value has a static type, but this is only accessible in type space."
> "Type space constructs such as `type` and `interface` are erased and are not accessible in value space."
> "Some constructs, such as `class` or `enum`, introduce both a type and a value."
> — Dan Vanderkam, *Effective TypeScript* Item 8, https://github.com/danvk/effective-typescript/blob/main/samples/ch-types/type-value-space.md

**The takeaway.** Two non-goals — "no runtime type info" and "no provably correct type system" — together mean the compiler cannot, by design, know what an `unknown` actually *is* at runtime. Every claim about runtime data is a contract that has to be enforced by something other than `tsc`.

---

## 2. `unknown` is the type at the boundary

`unknown` was added in TS 3.0 specifically to be the type-safe top:

> "TypeScript 3.0 introduces a new top type `unknown`. `unknown` is the type-safe counterpart of `any`. Anything is assignable to `unknown`, but `unknown` isn't assignable to anything but itself and `any` without a type assertion or a control flow based narrowing. Likewise, no operations are permitted on an `unknown` without first asserting or narrowing to a more specific type."
> — *TypeScript 3.0 Release Notes*, https://www.typescriptlang.org/docs/handbook/release-notes/typescript-3-0.html

Pocock on the smell:

> "`any` is an extremely powerful type in TypeScript. It lets you treat a value as if you were in JavaScript, not TypeScript. This means that it disables all of TypeScript's features — type checking, autocomplete, and safety."
> "Using `any` is rightly considered harmful by most of the community."
> — Matt Pocock, *`any` Considered Harmful*, https://www.totaltypescript.com/any-considered-harmful

Pocock also pushes back on lazy `unknown` substitution:

> "The `unknown` type is extremely 'yelly'. It errors whenever you access a property or assign it to something that isn't `unknown`."
> "Substituting `unknown` for `any` merely changed the problem — shifting from spreading `any`s across an application to spreading `unknown`s."
> — Pocock, *An `unknown` can't always fix an `any`*, https://www.totaltypescript.com/an-unknown-cant-always-fix-an-any

**The point.** `unknown` is the right type *at the boundary* (the value just arrived from the wild) precisely because no operations are legal on it without a narrowing step. `any` is the *absence* of a type. They look interchangeable but they're opposites: `unknown` forces a parse step; `any` deletes the boundary entirely.

---

## 3. Parse, don't validate

Alexis King's essay is the canonical statement of the discipline. Adopted across the TypeScript community as the design rule for runtime validation:

> "the difference between validation and parsing lies almost entirely in how information is preserved."
> "a parser is just a function that consumes less-structured input and produces more-structured output."
> "validateNonEmpty obeys the typechecker well enough, but only parseNonEmpty takes full advantage of it."
> "write functions on the data representation you wish you had, not the data representation you are given."
> "Shotgun parsing necessarily deprives the program of the ability to reject invalid input instead of processing it."
> — Alexis King, *Parse, don't validate*, https://lexi-lambda.github.io/blog/2019/11/05/parse-don-t-validate/

**The mechanic.** Validation returns a boolean and discards what was learned; the type system gets nothing. Parsing returns a *narrower type* and hands proof to the type system. Every Zod `.parse(unknown) → T` call is an instance of this idiom. Once data is parsed, the rest of the codebase operates on the narrower type and stops paying defensive-check tax.

---

## 4. Schema-inferred types

The shared design move across io-ts, Zod, Valibot, and ArkType: **the schema is the single source of truth and the static type is derived from it.**

io-ts:

> "A value of type `Type<A, O, I>` (called 'codec') is the runtime representation of the static type `A`."
> "A codec can: decode inputs of type `I` (through `decode`), encode outputs of type `O` (through `encode`), be used as a custom type guard (through `is`)."
> "Static types can be extracted from codecs using the `TypeOf` operator."
> "This library is conceived, tested and is supposed to be consumed by TypeScript with the `strict` flag turned on."
> — Giulio Canti, *io-ts*, https://github.com/gcanti/io-ts/blob/master/index.md

Colin McDonnell on Zod's central design:

> "Zod is acting as a reliable source of type safety that lets you confidently implement the rest of your application logic — including transforms."
> "Re-validating the type between each transform is overkill; TypeScript's type checker already does that."
> — *Why Zod 2 isn't leaving beta*, https://colinhacks.com/essays/why-zod-2-isnt-leaving-beta

> "Every Zod schema actually tracks two types, which is the output of the thing and the input of the thing. If you do a .transform, then those two diverge."
> "The whole idea is inference... Ideally the only time that Zod has an API that actually expects you to pass something in as a type hint."
> — McDonnell, Total TypeScript interview, https://www.totaltypescript.com/bonuses/typescript-expert-interviews/colin-mcdonnell-talks-about-the-design-choices-behind-zod

> "All checks (`.refine()`, `.min()`, `.max()`, etc.) are still executed in both directions."
> — McDonnell, *Introducing Zod Codecs*, https://colinhacks.com/essays/introducing-zod-codecs

**The point.** Both io-ts and Zod treat the schema as the single source of truth and *derive* the static type from it. Any time a codebase writes the type and the validator separately, they will eventually diverge — usually silently, in the direction that breaks production. Schema-as-source-of-truth means there's exactly one place where the boundary contract lives.

**Trade-offs to weigh on review.** Bundle size (Zod is non-trivial; Valibot is smaller). Compile-time cost (deeply-nested schemas can balloon tsserver memory — see `scale.md`). Error UX at runtime (default Zod errors are noisy; you usually want to format them). None of this argues against schema validation; all of it argues for picking the schema library deliberately and keeping schemas shallow at the boundary.

---

## 5. Type predicates and assertion functions — auditable contracts

The handbook on user-defined type guards:

> "To define a user-defined type guard, we simply need to define a function whose return type is a _type predicate_."
> "A predicate takes the form `parameterName is Type`, where `parameterName` must be the name of a parameter from the current function signature."
> "Any time `isFish` is called with some variable, TypeScript will _narrow_ that variable to that specific type if the original type is compatible."
> — *Narrowing*, TypeScript Handbook, https://www.typescriptlang.org/docs/handbook/2/narrowing.html

Vanderkam on the unaudited side:

> "If the type guard returns `true` then `x` is `T`. If the type guard returns `false` then `x` is not `T`."
> "This sort of incorrect type predicate can lead to unsoundness."
> "TypeScript does very little to check that they're valid."
> "There are expectations around the `false` case, and getting it right matters!"
> "When you write a user-defined type guard, it's easy to only think about the `true` case... that's only half the battle."
> "In order for a type guard to be completely safe, it's also important to know what the type of the parameter is when it returns `false`. This is the hidden side of type predicates."
> — Vanderkam, *The Hidden Side of Type Predicates*, https://effectivetypescript.com/2024/02/27/type-guards/

Assertion functions (TS 3.7):

> "`asserts condition` says that whatever gets passed into the `condition` parameter must be true if the `assert` returns (because otherwise it would throw an error). That means that for the rest of the scope, that condition must be truthy."
> "Here `asserts val is string` ensures that after any call to `assertIsString`, any variable passed in will be known to be a `string`."
> — *TypeScript 3.7 Release Notes*, https://www.typescriptlang.org/docs/handbook/release-notes/typescript-3-7.html

**The point.** Both `x is T` and `asserts x is T` are *unchecked* by the compiler. The signature creates a hole that the implementation must honor. The compiler trusts the developer; the developer is signing a soundness contract on behalf of every caller. This is precisely why these primitives belong at module boundaries — written once, audited carefully — not scattered through application code.

---

## 6. Error modeling — why TypeScript has no checked exceptions

Cavanaugh, in the typed-errors discussion:

> "The assignability of two function types is unaffected by what errors they might throw."
> — Issue #57943, *A Pragmatic, Not-Really-Typed Errors Proposal*, https://github.com/microsoft/TypeScript/issues/57943

**The design rationale.** If `throws` becomes part of the type, adding a new throw is a breaking change for every caller — and JavaScript's libraries throw freely. Soundness here would mean unsoundness with the JS ecosystem, violating the design goal of preserving JavaScript runtime behavior.

The Effect framework re-creates checked exceptions as a *value-level* construct:

> "Expected errors **are tracked** at the type level by the `Effect` data type in the 'Error' channel."
> "These errors, also referred to as _failures_, _typed errors_ or _recoverable errors_, are errors that developers anticipate as part of the normal program execution."
> "Unexpected errors, also referred to as _defects_, _untyped errors_, or _unrecoverable errors_, are errors that developers do not anticipate occurring during normal program execution. Since these errors are not expected, Effect **does not track** them at the type level."
> — Effect Documentation, *Two Types of Errors*, https://effect.website/docs/error-management/two-error-types/

> "the `Effect` type captures not only what the program returns on success but also what type of error it might produce."
> "Effect automatically keeps track of the possible errors that can occur during the execution of the program as a union of those error types."
> — Effect Documentation, *Expected Errors*, https://effect.website/docs/error-management/expected-errors/

**Trade-off.** Effect (or any Result-typed alternative) gives you typed errors back at the cost of every function returning an `Effect` and every caller using its API. It's all-or-nothing — the moment you cross into a `throw`-using library, the discipline collapses. For most TypeScript codebases the pragmatic compromise is: model expected errors as a discriminated-union `Result<T, E>` at boundaries (where the cost is justified) and let `throw` handle the rest.

---

## 7. Declaration files — the most fragile boundary

Basarat on what `.d.ts` actually is:

> "If a file has the extension `.d.ts` then each root level definition must have the `declare` keyword prefixed to it."
> "This helps make it clear to the author that there will be *no code emitted by TypeScript*."
> "Ambient declarations is a promise that you are making with the compiler."
> "The author needs to ensure that the declared item will exist at runtime."
> "If these do not exist at runtime and you try to use them, things will break without warning."
> "Ambient declarations are like docs. If the source changes the docs need to be kept updated."
> — Basarat, *Declaration Files*, https://basarat.gitbook.io/typescript/type-system/intro/d.ts

DefinitelyTyped's discipline:

> "We do not support exposing undocumented internal implementation details of libraries in `.d.ts` files."
> — DefinitelyTyped Contribution Guide, https://definitelytyped.org/guides/contributing.html

**The point.** A `.d.ts` is a *promise* to the compiler with no runtime enforcement. This is the most extreme version of the compile-time/runtime gap: there's nothing to validate against. Hand-written declaration files are pure boundary specifications, and DefinitelyTyped's "no internal details" rule operationalizes "boundaries should be narrow." When reviewing a `.d.ts`: every type listed there is a hand-signed claim that nothing in `tsc` can check.

---

## 8. Declaration merging and module augmentation

> "'declaration merging' means that the compiler merges two separate declarations declared with the same name into a single definition."
> "Non-function members of the interfaces should be unique. If they are not unique, they must be of the same type."
> "Currently, classes can not merge with other classes or with variables."
> "You can't declare new top-level declarations in the augmentation — just patches to existing declarations."
> "Default exports also cannot be augmented, only named exports (since you need to augment an export by its exported name, and `default` is a reserved word)."
> — *Declaration Merging*, TypeScript Handbook, https://www.typescriptlang.org/docs/handbook/declaration-merging.html

**Trade-off.** Module augmentation is the supported mechanism for *consumers* to extend a library's public types — it's a deliberate boundary. The gotchas (no new top-level decls, no default-export augmentation, no class merging) define the limits of what a library author lets consumers extend. If a consumer needs to augment more than the boundaries permit, the library has the wrong boundary.

---

## 9. Inferred types as the public API

tRPC's design rests on the same inference move as Zod, applied across a process boundary:

> "tRPC gives you end-to-end type safety from your (node-)server to your client, _without even declaring types_."
> "The `result` is type inferred from what the backend returns in the function."
> "There's no code generation involved & you can pretty easily add it to your existing Next.js/CRA/Express project."
> — Alex Johansson, *Introducing tRPC*, https://trpc.io/blog/introducing-trpc

The boundary still exists (HTTP, JSON), but the *types* describing it are inferred from a single source — the server's procedure definitions — and `import type` is used so the client carries no runtime payload from the server. Schema-as-source-of-truth applied at a service boundary.

Pocock's caution about generics-as-API on libraries — a counterweight to lazy use of generics:

> When a library function says it returns whatever type you passed in, "you're lying to yourself. Lying to yourself is fine if you know that you're doing it, but by using a generic, you're lying to yourself and not knowing it."
> Example: "with a fetch function... it's tempting to build a generic endpoint function where you just pop in a generic slot, when really fetch returns any, so you're hiding any underneath a beautiful looking generic signature."
> — Matt Pocock, paraphrased from interview at https://www.epicweb.dev/bonuses/interviews-with-experts/the-magic-of-typescript-with-matt-pocock

**The point.** tRPC's generics are *honest* because they're constrained by a real schema. A typical `fetch<T>(url)` is *dishonest* because the `T` is asserted, not parsed. Generics at a boundary must be backed by a runtime parser; otherwise the generic is `any` painted in fancier colors.

---

## 10. Module boundaries are where strictness matters most

Combining the above: TypeScript's design (no runtime type info), the parse-don't-validate principle, the unsoundness of `is`/`asserts`, and the `.d.ts` "promise" model all point to the same operational rule. The compiler cannot help across a boundary — only schemas can. Therefore:

- Every external input (`fetch`, `JSON.parse`, `process.env`, IPC, FormData, file I/O) should be typed `unknown` and parsed via a schema at the boundary.
- Every type predicate / assertion function / `as X` is a boundary contract; concentrate them in a thin parsing layer and audit them like security-critical code.
- Public library types should be inferred from one source (a schema, a server router, a procedure registry) so runtime and static descriptions cannot drift.
- Hand-written `.d.ts` files are the most fragile boundary in the system; minimize what they expose.
- If a generic parameter doesn't relate at least two positions in the signature, it's not earning its complexity (Vanderkam's golden rule from `modeling.md`); at a boundary, it's actively unsafe.

---

## Direct quotes (catalog)

- "TypeScript 3.0 introduces a new top type `unknown`. `unknown` is the type-safe counterpart of `any`." — TS 3.0
- "Anything is assignable to `unknown`, but `unknown` isn't assignable to anything but itself and `any` without a type assertion or a control flow based narrowing." — TS 3.0
- "Every value has a static type, but this is only accessible in type space." — Vanderkam
- "Type space constructs such as `type` and `interface` are erased and are not accessible in value space." — Vanderkam
- "the difference between validation and parsing lies almost entirely in how information is preserved." — Alexis King
- "a parser is just a function that consumes less-structured input and produces more-structured output." — Alexis King
- "write functions on the data representation you wish you had, not the data representation you are given." — Alexis King
- "Shotgun parsing necessarily deprives the program of the ability to reject invalid input instead of processing it." — Alexis King
- "The `unknown` type is extremely 'yelly'." — Pocock
- "Substituting `unknown` for `any` merely changed the problem." — Pocock
- "A value of type `Type<A, O, I>` (called 'codec') is the runtime representation of the static type `A`." — Canti, io-ts
- "Static types can be extracted from codecs using the `TypeOf` operator." — Canti
- "Re-validating the type between each transform is overkill; TypeScript's type checker already does that." — McDonnell, Zod
- "Every Zod schema actually tracks two types, which is the output of the thing and the input of the thing." — McDonnell
- "TypeScript does very little to check that [type predicates are] valid." — Vanderkam
- "In order for a type guard to be completely safe, it's also important to know what the type of the parameter is when it returns `false`. This is the hidden side of type predicates." — Vanderkam
- "`asserts condition` says that whatever gets passed into the `condition` parameter must be true if the `assert` returns." — TS 3.7
- "The assignability of two function types is unaffected by what errors they might throw." — Cavanaugh
- "Expected errors are tracked at the type level by the `Effect` data type in the 'Error' channel." — Effect docs
- "Ambient declarations is a promise that you are making with the compiler." — Basarat
- "If these do not exist at runtime and you try to use them, things will break without warning." — Basarat
- "We do not support exposing undocumented internal implementation details of libraries in `.d.ts` files." — DefinitelyTyped
- "tRPC gives you end-to-end type safety from your (node-)server to your client, _without even declaring types_." — Johansson
- "you're lying to yourself and not knowing it" *(on `fetch<T>(url)`)* — Pocock

---

## Sources

1. https://www.typescriptlang.org/docs/handbook/release-notes/typescript-3-0.html — `unknown` top type and rationale.
2. https://www.typescriptlang.org/docs/handbook/release-notes/typescript-3-7.html — Assertion functions.
3. https://www.typescriptlang.org/docs/handbook/2/narrowing.html — Type guards (`x is T`).
4. https://www.typescriptlang.org/docs/handbook/declaration-merging.html — Merging rules and augmentation limits.
5. https://github.com/microsoft/TypeScript-wiki/blob/main/TypeScript-Design-Goals.md — Goals and Non-Goals.
6. https://github.com/microsoft/TypeScript/issues/57943 — Cavanaugh on assignability and exceptions.
7. https://github.com/danvk/effective-typescript/blob/main/samples/ch-types/type-value-space.md — Vanderkam Item 8.
8. https://effectivetypescript.com/2024/02/27/type-guards/ — Vanderkam, *The Hidden Side of Type Predicates*.
9. https://lexi-lambda.github.io/blog/2019/11/05/parse-don-t-validate/ — Alexis King, *Parse, don't validate*.
10. https://www.totaltypescript.com/any-considered-harmful — Pocock on `any`.
11. https://www.totaltypescript.com/an-unknown-cant-always-fix-an-any — Pocock on `unknown` limits.
12. https://www.totaltypescript.com/concepts/any-type — Pocock on `any` vs `unknown`.
13. https://colinhacks.com/essays/why-zod-2-isnt-leaving-beta — McDonnell on Zod design.
14. https://colinhacks.com/essays/introducing-zod-codecs — Bidirectional transforms at boundaries.
15. https://www.totaltypescript.com/bonuses/typescript-expert-interviews/colin-mcdonnell-talks-about-the-design-choices-behind-zod — Inference, generics in Zod.
16. https://github.com/gcanti/io-ts/blob/master/index.md — Codec-as-runtime-representation.
17. https://effect.website/docs/error-management/two-error-types/ — Expected vs defects.
18. https://effect.website/docs/error-management/expected-errors/ — Typed error channel.
19. https://basarat.gitbook.io/typescript/type-system/intro/d.ts — `.d.ts` semantics.
20. https://basarat.gitbook.io/typescript/type-system/intro — Ambient declarations.
21. https://definitelytyped.org/guides/contributing.html — Public-API discipline for declaration files.
22. https://trpc.io/blog/introducing-trpc — Inferred types crossing the client/server boundary.
23. https://www.epicweb.dev/bonuses/interviews-with-experts/the-magic-of-typescript-with-matt-pocock — Pocock on honesty of generics in library APIs.
