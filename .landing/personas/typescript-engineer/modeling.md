---
title: "Domain Modeling — Discriminated Unions, Brands, Generics, Conditional Types, and the Cleverness Cliff"
summary: >
  Primary-source corpus on shaping TypeScript types for domain logic at scale. Covers
  discriminated unions, branded/nominal types, the golden rule of generics, conditional
  vs overloaded signatures, interface vs type, the enum debate, and optional-property
  smell. Drawn from the TypeScript handbook, Daniel Rosenwasser's release notes, the
  TS performance wiki, Dan Vanderkam's Effective TypeScript, Matt Pocock, Basarat, and
  Nicholas Jamieson. Pros and cons, not rules.
source_count: 22
---

# Domain Modeling with TypeScript

> "Prefer types that only represent valid states. Even if they are longer or harder to express, they will save you time and pain in the end!" — Dan Vanderkam, *Effective TypeScript*, Item 29

The job of a TypeScript model is to make wrong programs unrepresentable, not to mirror the runtime values verbatim. Most modeling pathologies come from picking the easiest type that compiles instead of the type that encodes the actual constraints.

---

## 1. Discriminated (tagged) unions

The single highest-leverage modeling tool in TypeScript. They turn a runtime branch into compile-time information.

> "When every type in a union contains a common property with literal types, TypeScript considers that to be a _discriminated union_, and can narrow out the members of the union."
> "The problem with this encoding... is that the type-checker doesn't have any way to know whether or not `radius` or `sideLength` are present based on the `kind` property. We need to communicate what _we_ know to the type checker."
> — *Narrowing*, TypeScript Handbook, https://www.typescriptlang.org/docs/handbook/2/narrowing.html

Exhaustiveness comes from `never`:

> "When narrowing, you can reduce the options of a union to a point where you have removed all possibilities and have nothing left. In those cases, TypeScript will use a `never` type to represent a state which shouldn't exist."
> "The `never` type is assignable to every type; however, no type is assignable to `never` (except `never` itself). This means you can use narrowing and rely on `never` turning up to do exhaustive checking in a `switch` statement."
> — same source

> "If a new case is added at compile time you will get a compile error. If a new value appears at runtime you will get a runtime error."
> — Basarat, *Discriminated Unions*, https://basarat.gitbook.io/typescript/type-system/discriminated-unions

Pocock's framing of the upside — discriminated unions force you to think in **states**, not **flags**:

> "By ensuring you can only access data when the status equals `success`, you're encouraged to think of your app in terms of its states, and only access data in the states it's available."
> "When you start thinking of your app in terms of discriminated states, a lot of things get easier. Instead of a big optional bag of data, you'll start understanding the connections between data and UI."
> — Matt Pocock, *Discriminated Unions for Frontend Developers*, https://www.totaltypescript.com/discriminated-unions-are-a-devs-best-friend

Vanderkam's framing of the inverse — bag-of-optionals is usually a missed discriminated union:

> "Interfaces with multiple properties that are union types are often a mistake because they obscure the relationships between these properties."
> "Use tagged unions to facilitate control flow analysis. Because they are so well supported, this pattern is ubiquitous in TypeScript code."
> — Vanderkam, Item 34, https://github.com/danvk/effective-typescript/blob/main/samples/ch-design/union-of-interfaces.md

**When *not* to unify** — Vanderkam's counterweight (Item 32):

> "you should prefer to unify your types rather than model small differences between them."
> "It would be counterproductive to 'unify' these types... The two `Response` types are fundamentally different, so they should not be unified... This will make TypeScript less effective at finding bugs in your `Response`-handling code."
> — Vanderkam, https://effectivetypescript.com/2021/04/09/unify-over-model/

**Trade-off.** Discriminated unions impose verbosity (every variant carries the discriminant tag) and a small runtime cost (the tag must exist on the value). The payoff is that the type system enforces "you can only read `data` when status is `success`" without runtime checks. Almost every "this could be three optional booleans" object benefits from being remodeled as a tagged union — until the variants share so little structure that the union becomes a fiction. When variants are fundamentally different things, give them different names.

---

## 2. Make illegal states unrepresentable

Vanderkam's Item 29 is the operational form of this canon:

> "Types that represent both valid and invalid states are likely to lead to confusing and error-prone code."
> "Prefer types that only represent valid states. Even if they are longer or harder to express, they will save you time and pain in the end!"
> — Vanderkam, https://github.com/danvk/effective-typescript/blob/main/samples/ch-design/valid-states.md

The dual rule — push nullability to the perimeter:

> "Values are easier to work with when they're either completely null or completely non-null, rather than a mix."
> "When variable A is non-null, you know that variable B is also non-null and vice versa. These implicit relationships are confusing both for human readers of your code and for the type checker."
> "Avoid designs in which one value being `null` or not `null` is implicitly related to another value being `null` or not `null`."
> "Push `null` values to the perimeter of your API by making larger objects either `null` or fully non-`null`."
> — Vanderkam, https://effectivetypescript.com/2020/03/24/null-values-to-perimeter/

**Reviewing implication.** When you see correlated optionals (`a?: X; b?: Y` where `b` is only meaningful when `a` is set), the type is lying. The state is one tagged union with two variants — `{ kind: "with-a"; a: X; b: Y }` and `{ kind: "without"; }`. The type-system pays you back every time you'd otherwise have to write `if (a && b)` defensively.

---

## 3. Branded / nominal types

Structural typing makes `UserId` and `OrderId` interchangeable when both are `string`. Brands are the workaround.

> "The TypeScript type system is structural. There are real-world use cases for a system where you want two variables to be differentiated because they have a different *type name*."
> "A very common use case is *identity* structures (which are generally just strings with semantics associated with their *name*)."
> — Basarat, *Nominal Typing*, https://basarat.gitbook.io/typescript/main-1/nominaltyping

> "With nominal typing, a value has a type because you say it has a type, not because it has the same shape as that type."
> — Vanderkam, *Brands recipe*, https://github.com/danvk/effective-typescript/blob/main/samples/ch-recipes/brands.md

> "A Brand type lets you use a bit of 'nominal' typing inside TypeScript. By declaring a unique symbol, 'brand', we can use this Brand type helper to create types with 'names'."
> "Branded types let you assign different 'labels' to values."
> — Pocock, *Four Essential TypeScript Patterns*, https://www.totaltypescript.com/four-essential-typescript-patterns

**Trade-off.** Brands are zero-runtime and high-leverage at the boundary of a domain — once a `string` becomes a `UserId`, the system prevents accidental mixing across the entire codebase. The cost is at the perimeter: minting a brand requires a constructor function or a deliberate cast, which means you need a single audited place where untrusted strings become typed identifiers. Sprinkling `as UserId` throughout the codebase defeats the entire point — the brand becomes a structural type with extra steps.

When to brand: identifiers (`UserId`, `OrderId`), units of measurement (`Meters`, `Seconds`), and pre-validated strings (`Email`, `Url` after parsing). When to skip: types that are already nominal because they have at least one unique field.

---

## 4. Generics — the golden rule

Vanderkam's "type parameter must appear twice" is the closest thing TypeScript has to a rule of three for generics:

> "Type parameters should appear twice. Type parameters are for relating the types of multiple values."
> "If a type parameter only appears in one location, it's not relating anything."
> "If a type parameter only appears in one location, strongly reconsider if you actually need it."
> "So-called 'return-only generics' are dangerous because they're equivalent to `any`, but don't use the word `any`."
> — Vanderkam, *The Golden Rule of Generics*, https://effectivetypescript.com/2020/08/12/generics-golden-rule/

Summary version:

> "Type parameters must appear multiple times to establish relationships between types — each parameter should show up at least twice."
> "Eliminate 'return-only generics' — avoid parameters that only appear in return types."
> "Replace with `unknown` — unneeded type parameters can often be substituted with the `unknown` type instead."
> — Vanderkam, https://github.com/danvk/effective-typescript/blob/main/samples/ch-generics/golden-rule.md

Pocock on the vocabulary:

> "There is no such thing as a 'generic'. There are **generic types**, **generic functions**, and **generic classes**."
> "'generic' is not a noun, it's an adjective."
> "a **type parameter** is like a function parameter. It declares that you can pass a type argument to the type, function, or class."
> — Pocock, *There Is No Such Thing As A Generic*, https://www.totaltypescript.com/no-such-thing-as-a-generic

**Trade-off.** Generics earn their complexity when they relate at least two positions — input to output, two inputs to each other, an array element to its lookup result. A generic that only appears in the return type is `any` wearing a costume: it lets the caller assert what they want without giving the implementation any way to verify. Reviewing a generic function: count the appearances of each type parameter. One? Likely a smell. Pocock's broader warning, from his Epic Web interview: a `fetch<T>(url)` whose `T` is asserted (not parsed) is "lying to yourself" — see `boundaries.md`.

---

## 5. Conditional and mapped types — the readability cliff

Conditional types are powerful where overloads collapse:

> "Prefer conditional types to overloaded type signatures. By distributing over unions, conditional types allow your declarations to support union types without additional overloads."
> "If the union case is implausible, consider whether your function would be clearer as two or more functions with different names."
> — Vanderkam, Item 52, https://github.com/danvk/effective-typescript/blob/main/samples/ch-generics/conditional-overload.md

But the cost is real, and Vanderkam frames it as **display**:

> "We talk all the time about how to define and use types in TypeScript, but we rarely talk about how TypeScript chooses to _display_ our types."
> "When you're writing code that works with types, you should consider safety and correctness first and foremost. But once you have those, you should _also_ consider how your types display."
> "Both cases of the conditional type are variations on the identity function. It doesn't look like it should do anything at all!" *(on extracting a conditional type purely to control hover output)*
> "Sometimes the display of a type is bad for a specific, important case of your generic. In these situations it can be worthwhile to handle those cases specially using a conditional type."
> — Vanderkam, *Display of Types*, https://effectivetypescript.com/2022/02/25/gentips-4-display/

The TS team itself documents the compile-time cost (cf. `scale.md`):

> "Every time `foo` is called, TypeScript has to re-run the conditional type. What's more, relating any two instances of `SomeType` requires re-relating the structure of the return type of `foo`."
> "If the return type in this example was extracted out to a type alias, more information can be cached by the compiler."
> — TS Performance Wiki, https://github.com/microsoft/TypeScript/wiki/Performance

**Trade-off.** A conditional type is a function the human must mentally evaluate every time they read the call site. It's also a function the compiler must re-evaluate. Use one when the alternative is a combinatorial explosion of overloads or a `unknown`-typed return; avoid one when two named functions would do the same job more legibly. If you must use one, name the result type and check what it displays as on hover. The "types are too clever" failure mode is when a type is correct but unintelligible to the next reader — and on a team, that reader is always more expensive than the elegant solution.

---

## 6. `interface` vs `type`

The TS team's performance wiki is unambiguous:

> "Interfaces create a single flat object type that detect property conflicts, which are usually important to resolve!"
> "Type relationships between interfaces are also cached, as opposed to intersection types as a whole."
> "Interfaces also display consistently better, whereas type aliases to intersections can't be displayed in part of other intersections."
> — TS Wiki, https://github.com/microsoft/TypeScript/wiki/Performance

Vanderkam's counterweight — the merging trade-off:

> "Declaration merging is surprising and it's given `interface` a bit of a bad rap."
> "Declaration merging really shines when you look at the [`lib` setting](https://www.typescriptlang.org/tsconfig#lib) in `tsconfig.json`, which models the ECMAScript version that will be available at runtime."
> "Declaration merging is surprising and controversial, but it's not all bad."
> "If you add declarations that don't reflect reality at runtime, you can create a really confusing situation."
> — Vanderkam, *In Defense of Interface*, https://effectivetypescript.com/2021/06/03/interface/

**Trade-off.** The choice is rarely about preference. `interface` wins on: cached identity, conflict detection at declaration time, and consistent hover display. `type` wins on: union types, mapped types, conditional types, and tuple types — all things `interface` can't express. Declaration merging is `interface`-only, which is upside (modeling `lib` evolution, allowing consumers to augment a library) and downside (action at a distance). The pragmatic rule: object shapes that won't be unioned or computed → `interface`. Anything that needs `|`, `&`, `keyof`, or conditionals → `type`.

---

## 7. Enums vs string-literal unions

The TS team has been gradually de-emphasizing enums. The 5.0 release smoothed the worst sharp edges:

> "TypeScript 5.0 manages to make all enums into union enums by creating a unique type for each computed member."
> "Enum literal types gave each enum member its own type, and turned the enum itself into a union of each member type."
> "assigning an out-of-domain literal to an enum type will now error as one might expect."
> — *Announcing TypeScript 5.0 Beta*, https://devblogs.microsoft.com/typescript/announcing-typescript-5-0-beta/

But `const enum` actively breaks library consumers:

> "If you are writing a library and you export a `const enum`, some developers will not be able to compile their applications if they import your library."
> "This compilation process does not read imported modules, so it's not possible for it to support the replacement of `const enum` members."
> — Nicholas Jamieson, *Don't Export `const enum`*, https://ncjamieson.com/dont-export-const-enums/

Pocock's case against:

> "Numeric and string enums actually behave differently in a couple of ways."
> "I like my TypeScript to be just JavaScript with types. Enums feel like they break that rule."
> "There are currently 71 issues marked as bugs related to enums in the TypeScript repo."
> "If you're desperate to use enums, I'd strongly recommend using string enums only."
> — Pocock, *Why I Don't Like TypeScript Enums*, https://www.totaltypescript.com/why-i-dont-like-typescript-enums

The team has now shipped a flag that explicitly disqualifies enum syntax for the type-strip-only future:

> "erasableSyntaxOnly marks enums, namespaces and class parameter properties as errors."
> "'Erasable' syntax means that the syntax can be deleted without the runtime behaviour being affected. Enums, namespaces and class parameter properties do not obey this rule."
> "the TypeScript team is looking toward a future where these syntaxes will no longer be used."
> — Pocock, *Erasable Syntax Only*, https://www.totaltypescript.com/erasable-syntax-only

**Trade-off.** Enums emit runtime objects, behave inconsistently between numeric and string variants, break under `--isolatedModules` in their `const enum` form, and disqualify code from native type-stripping. String-literal unions emit nothing, narrow naturally, and compose freely. The 5.0 cleanup makes enums *less* sharp, not equally good. Default to a union of string literals; reach for enums only when you specifically need the bidirectional mapping (`MyEnum.Value` ↔ `MyEnum[0]`) or runtime iteration.

---

## 8. Optional properties

Vanderkam's Item 37 — limit them:

> "Optional properties can prevent the type checker from finding bugs and can lead to repeated and possibly inconsistent code for filling in default values."
> "Think carefully before making properties optional. Evaluate whether requiring them instead would improve your design."
> "Create distinct types for unnormalized input data versus normalized data used internally in your application."
> "Prevent a combinatorial explosion of options by limiting optional properties."
> — Vanderkam, https://github.com/danvk/effective-typescript/blob/main/samples/ch-design/avoid-optional.md

**Trade-off.** Each `?` doubles the cardinality of representable shapes. N optional fields produce 2^N states — and most of those states the code didn't actually consider. The fix is rarely "make them required"; it's "split this into two types." Input type (loosely shaped, optionals welcome) → parser → normalized type (no optionals, every field provided or absent via discriminated variant). The optional `?` belongs at boundaries, not in internal models.

There is also the three-way distinction between `key?: T`, `key: T | undefined`, and `key?: T | undefined`, which `--exactOptionalPropertyTypes` makes meaningful. See `strictness.md`.

---

## 9. The "types are too clever" failure mode

The TS team and Vanderkam both gesture at this. The clearest statements:

> "named types tend to be more compact than anonymous types (which the compiler might infer), which reduces the amount of time spent reading and writing declaration files."
> "Every time `foo` is called, TypeScript has to re-run the conditional type."
> — TS Performance Wiki, https://github.com/microsoft/TypeScript/wiki/Performance

> "When you're writing code that works with types, you should consider safety and correctness first and foremost. But once you have those, you should _also_ consider how your types display."
> "Sometimes the display of a type is bad for a specific, important case of your generic."
> — Vanderkam, *Display of Types*, https://effectivetypescript.com/2022/02/25/gentips-4-display/

**Trade-off.** A type that takes longer to read than the code it describes is failing its job. A type that produces a 40-line hover popup is failing its job. A type that recompiles every callsite slowly is failing its job. None of these failures show up as type errors — they show up as PR review fatigue and tsserver lag. When reviewing a clever type, ask: (1) does this catch a class of bug worth this much complexity, (2) what does it look like on hover, (3) could a runtime parser plus a simpler type do the same work for a fraction of the cognitive cost. Often the answer is yes.

---

## Direct quotes (catalog)

- "When every type in a union contains a common property with literal types, TypeScript considers that to be a _discriminated union_..." — TS Handbook
- "If a new case is added at compile time you will get a compile error." — Basarat
- "By ensuring you can only access data when the status equals `success`, you're encouraged to think of your app in terms of its states." — Pocock
- "Interfaces with multiple properties that are union types are often a mistake." — Vanderkam Item 34
- "The two `Response` types are fundamentally different, so they should not be unified." — Vanderkam
- "Prefer types that only represent valid states. Even if they are longer or harder to express..." — Vanderkam Item 29
- "Push `null` values to the perimeter of your API." — Vanderkam
- "With nominal typing, a value has a type because you say it has a type, not because it has the same shape as that type." — Vanderkam
- "Type parameters should appear twice." — Vanderkam Golden Rule
- "Return-only generics are dangerous because they're equivalent to `any`, but don't use the word `any`." — Vanderkam
- "There is no such thing as a 'generic'." — Pocock
- "Prefer conditional types to overloaded type signatures." — Vanderkam Item 52
- "Sometimes the display of a type is bad for a specific, important case of your generic." — Vanderkam
- "Interfaces create a single flat object type that detect property conflicts." — TS Wiki
- "Declaration merging is surprising and it's given `interface` a bit of a bad rap." — Vanderkam
- "If you are writing a library and you export a `const enum`, some developers will not be able to compile their applications." — Jamieson
- "I like my TypeScript to be just JavaScript with types. Enums feel like they break that rule." — Pocock
- "Optional properties can prevent the type checker from finding bugs..." — Vanderkam Item 37
- "Prevent a combinatorial explosion of options by limiting optional properties." — Vanderkam

---

## Sources

1. https://www.typescriptlang.org/docs/handbook/2/narrowing.html — Discriminated unions, exhaustive `never` checks.
2. https://github.com/microsoft/TypeScript/wiki/Performance — `interface` vs intersection caching, naming complex types, return-type annotations.
3. https://devblogs.microsoft.com/typescript/announcing-typescript-5-0-beta/ — "All Enums Are Union Enums" cleanup in TS 5.0.
4. https://basarat.gitbook.io/typescript/type-system/discriminated-unions — Tagged unions, `never` exhaustiveness.
5. https://basarat.gitbook.io/typescript/main-1/nominaltyping — Structural typing, brand techniques.
6. https://effectivetypescript.com/ — *Effective TypeScript* index.
7. https://github.com/danvk/effective-typescript/blob/main/samples/ch-design/valid-states.md — Item 29: valid states.
8. https://effectivetypescript.com/2020/03/24/null-values-to-perimeter/ — Push null to perimeter.
9. https://github.com/danvk/effective-typescript/blob/main/samples/ch-design/union-of-interfaces.md — Item 34: unions of interfaces.
10. https://github.com/danvk/effective-typescript/blob/main/samples/ch-design/avoid-optional.md — Item 37: limit optionals.
11. https://github.com/danvk/effective-typescript/blob/main/samples/ch-recipes/brands.md — Brands recipe.
12. https://effectivetypescript.com/2020/08/12/generics-golden-rule/ — Golden Rule of Generics.
13. https://github.com/danvk/effective-typescript/blob/main/samples/ch-generics/conditional-overload.md — Item 52: conditional types vs overloads.
14. https://effectivetypescript.com/2022/02/25/gentips-4-display/ — Display of Types.
15. https://effectivetypescript.com/2021/04/09/unify-over-model/ — When *not* to unify.
16. https://effectivetypescript.com/2021/06/03/interface/ — In defense of `interface`; declaration merging.
17. https://www.totaltypescript.com/discriminated-unions-are-a-devs-best-friend — Pocock on discriminated unions.
18. https://www.totaltypescript.com/four-essential-typescript-patterns — Pocock on brands, assertion functions.
19. https://www.totaltypescript.com/why-i-dont-like-typescript-enums — Pocock against enums.
20. https://www.totaltypescript.com/erasable-syntax-only — Pocock on `erasableSyntaxOnly`.
21. https://www.totaltypescript.com/no-such-thing-as-a-generic — Pocock on generics vocabulary.
22. https://ncjamieson.com/dont-export-const-enums/ — Jamieson on `const enum` breaking `--isolatedModules` consumers.
