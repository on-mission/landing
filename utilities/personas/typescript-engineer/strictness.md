---
title: "Compiler Strictness — What Each Flag Catches and What It Costs"
summary: >
  Primary-source corpus on TypeScript compiler configuration trade-offs at scale. Covers
  the --strict family, the high-value flags outside --strict (noUncheckedIndexedAccess,
  exactOptionalPropertyTypes, noPropertyAccessFromIndexSignature, noImplicitOverride,
  noFallthroughCasesInSwitch), the module/emit family (isolatedModules, verbatimModuleSyntax,
  erasableSyntaxOnly), and the strictness-migration pattern. Drawn from typescriptlang.org/tsconfig,
  TS team release notes, Cavanaugh's PRs, the TS Wiki FAQ, Pocock, Vanderkam, Bloomberg's
  scaling work, and the Allegro typescript-strict-plugin.
source_count: 24
---

# TypeScript Compiler Strictness at Scale

> "Future versions of TypeScript may introduce additional stricter checking under this flag, so upgrades of TypeScript might result in new type errors in your program." — TSConfig reference on `--strict`

`--strict` is a moving floor, not a finished contract. The flags that catch the highest-leverage real-world bugs at scale (`noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`) are deliberately *not* in `--strict`. Treating `strict: true` as "we did the strictness thing" is the most common configuration mistake at scale.

---

## 1. The `--strict` umbrella — floor, not ceiling

> "The `strict` flag enables a wide range of type checking behavior that results in stronger guarantees of program correctness. Turning this on is equivalent to enabling all of the _strict mode family_ options, which are outlined below. You can then turn off individual strict mode family checks as needed."
> "Future versions of TypeScript may introduce additional stricter checking under this flag, so upgrades of TypeScript might result in new type errors in your program. When appropriate and possible, a corresponding flag will be added to disable that behavior."
> — TSConfig reference, https://www.typescriptlang.org/tsconfig/

The current `--strict` family includes `alwaysStrict`, `strictNullChecks`, `strictBindCallApply`, `strictBuiltinIteratorReturn`, `strictFunctionTypes`, `strictPropertyInitialization`, `noImplicitAny`, `noImplicitThis`, `useUnknownInCatchVariables`. It does **not** include `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `noPropertyAccessFromIndexSignature`, `noImplicitOverride`, or `noFallthroughCasesInSwitch`.

> "strict: true — Enables all strict type checking options. Indispensable."
> — Matt Pocock, *TSConfig Cheat Sheet*, https://www.totaltypescript.com/tsconfig-cheat-sheet

**Two operational consequences.** First, "strict" is a moving target — a TypeScript minor upgrade can legitimately surface new errors in code that previously type-checked. Plan for that on upgrades. Second, the flags with the highest catch-rate at scale are deliberately outside `--strict` so the team can ship them without breaking ecosystems. If your config is `strict: true` and nothing else, you've left half the value on the table.

---

## 2. `--strictNullChecks`

The single highest-impact correctness flag in the language. Introduced in TypeScript 2.0.

> "`strictNullChecks` — `null` and `undefined` have their own distinct types; you'll get type errors when using them where concrete values are expected."
> — TSConfig reference, https://www.typescriptlang.org/tsconfig/

**Migration cost.** The dominant pattern across migrations is to defer `strictNullChecks` until *after* `noImplicitAny` is clean — nulls flow through inferred-`any` paths and produce runaway error counts otherwise. Per-file pragmas are the standard mitigation:

> "By adding //@ts-strict-ignore comment at the top of a file, its whole content will be removed from strict type checking."
> — Allegro, *typescript-strict-plugin*, https://github.com/allegro/typescript-strict-plugin

> "A recommended approach is to use typescript-strict-plugin to exempt all existing code from strict mode, enable it for new code and periodically update the old code to be strict-compliant."
> — Allegro engineering blog, https://blog.allegro.tech/2021/09/How-to-turn-on-TypeScript-strict-mode-in-specific-files.html

**Trade-off.** Turning this on for a brownfield codebase is the largest one-time effort in the strictness migration — and the largest one-time payoff. Expected null bugs (the "billion-dollar mistake") get caught by the compiler instead of by users.

---

## 3. `--strictFunctionTypes` and the bivariance hole

This is one of the more subtle flags because it has an explicit, intentional hole.

> "Under `strictFunctionTypes` function type parameter positions are checked _contravariantly_ instead of _bivariantly_."
> "The stricter checking applies to all function types, _except_ those originating in method or constructor declarations. Methods are excluded specifically to ensure generic classes and interfaces (such as `Array<T>`) continue to mostly relate covariantly."
> — *TypeScript 2.6 Release Notes*, https://www.typescriptlang.org/docs/handbook/release-notes/typescript-2-6.html

> "During development of this feature, we discovered a large number of inherently unsafe class hierarchies, including some in the DOM. Because of this, the setting only applies to functions written in _function_ syntax, not to those in _method_ syntax."
> — TSConfig reference for `strictFunctionTypes`, https://www.typescriptlang.org/tsconfig/strictFunctionTypes.html

The TS Wiki FAQ on why methods stay bivariant:

> "Method and function signatures behave differently, specifically that narrower argument types are unsoundly allowed in subtypes of methods, but not functions."
> "A cursory check in a small project shows hundreds of errors in longstanding code where there aren't any existing complaints of unsoundness due to bivariance."
> — TS Wiki FAQ, https://github.com/microsoft/TypeScript/wiki/FAQ

**Trade-off.** The flag closes the hole only for arrow-property syntax. Teams that care about variance in callback-heavy interfaces (event emitters, comparators, reducers) standardize on `compare: (a: T, b: T) => number` field syntax over `compare(a: T, b: T): number` method syntax even when noisier — see `type-system.md` §6.

---

## 4. `--noImplicitAny`, `--noImplicitThis`, `--strictBindCallApply`, `--alwaysStrict`

> "Item 62 — Don't Consider Migration Complete Until You Enable noImplicitAny."
> "Your next goal is to turn on the noImplicitAny option. TypeScript code without noImplicitAny is best thought of as transitional because it can mask real errors you've made in your type declarations."
> "Turn on noImplicitAny unless you are transitioning a JS project to TS."
> — Vanderkam, *Effective TypeScript* Item 62, summarized at https://github.com/trungk18/effective-typescript-summary

> "`strictBindCallApply` — Checks that built-in function methods `call`, `bind`, and `apply` are invoked with correct arguments for the underlying function."
> "`noImplicitThis` — Raises errors on `this` expressions with an implied `any` type."
> "`alwaysStrict` — Ensures files are parsed in ECMAScript strict mode and emit \"use strict\" for each source file."
> — TSConfig reference, https://www.typescriptlang.org/tsconfig/

**Trade-off.** All four are subsumed by `--strict`. The catch rates are uneven (`noImplicitAny` is high-leverage, the others are lower) but the cost is near-zero on greenfield code. `noImplicitAny` does not prevent *explicit* `any`; it only forces the decision to be visible.

---

## 5. `--useUnknownInCatchVariables`

Added in TS 4.4, included in `--strict`.

> "TypeScript 4.4 introduces a new flag called `--useUnknownInCatchVariables`. This flag changes the default type of `catch` clause variables from `any` to `unknown`."
> "This flag is enabled under the `--strict` family of options."
> — *Announcing TypeScript 4.4 Beta*, https://devblogs.microsoft.com/typescript/announcing-typescript-4-4-beta/

**Trade-off.** Cost is concentrated in `catch (e) { console.error(e.message) }` style code, which is everywhere in legacy codebases. Standard remediation is `instanceof Error` narrowing or a tiny `toError(e: unknown): Error` helper. The payoff is real — pre-flag, code silently accepted non-Error throws (strings, plain objects, rejected promises with arbitrary payloads).

---

## 6. `--noUncheckedIndexedAccess` — the big one outside `--strict`

Cavanaugh's PR description:

> "any indexed access expression `obj[index]` used in a read position will include `undefined` in its type, unless `index` is a string literal or numeric literal with previous narrowing in effect."
> "Indexed access types, e.g. `type A = SomeType[number]`, retain their current meaning (`undefined` is not added to this type)."
> "writes to `obj[index]` and `obj.prop` forms retain their normal behavior."
> — Ryan Cavanaugh, PR #39560, https://github.com/microsoft/TypeScript/pull/39560

> "Turning on `noUncheckedIndexedAccess` will add `undefined` to any un-declared field in the type."
> — TSConfig reference, https://www.typescriptlang.org/tsconfig/noUncheckedIndexedAccess.html

The TS team has explicitly declined folding it into `--strict` (https://github.com/microsoft/TypeScript/issues/49169) and the flag has known holes — `SomeType[number]` does not get `undefined`; `for...in` over `Record` does not narrow.

There is even a 5–10% compiler perf cost from the underlying narrowing change:

> "every narrowing is based on syntactic patterns, and adding the form `e[i]` to the list of things we narrow on incurred a 5-10% performance penalty."
> — Cavanaugh, PR #39560

**Trade-off.** This is the single highest-leverage correctness flag *not* in strict. It catches "I trusted that `arr[i]` returned a value" and every `Record<string, T>` lookup that didn't actually have the key. Cost is real — every loop, every array access, every dictionary lookup acquires `| undefined` and forces a narrowing or a `!`. Greenfield: turn it on. Brownfield: gate per-file with a strict-plugin and ratchet.

---

## 7. `--exactOptionalPropertyTypes`

Added in TS 4.4. Not in `--strict`. Requires `--strictNullChecks`.

> "In TypeScript 4.4, the new flag `--exactOptionalPropertyTypes` specifies that optional property types should be interpreted exactly as written, meaning that `| undefined` is not added to the type."
> "This flag is not part of the `--strict` family and needs to be turned on explicitly if you'd like this behavior. It also requires `--strictNullChecks` to be enabled as well."
> — *Announcing TypeScript 4.4 Beta*, https://devblogs.microsoft.com/typescript/announcing-typescript-4-4-beta/

> "Setting the value to `undefined` will allow most JavaScript runtime checks for the existence to fail, which is effectively falsy. However, this isn't quite accurate; `colorThemeOverride: undefined` is not the same as `colorThemeOverride` not being defined. For example, `\"colorThemeOverride\" in settings` would have different behavior with `undefined` as the key compared to not being defined."
> — TSConfig reference, https://www.typescriptlang.org/tsconfig/exactOptionalPropertyTypes.html

**Trade-off.** Object spreads (`{ ...partial, optionalKey: maybeUndefined }`) become invalid because the spread key may be `undefined`. APIs that use `{ key?: T }` to mean "may be omitted" interact badly with builder patterns that explicitly assign `undefined` to mean "clear this." The right discipline becomes:
- `key?: T` = "may be omitted, never present-as-undefined"
- `key: T | undefined` = "always present, value may be undefined"
- `key?: T | undefined` = "both omission and explicit undefined are allowed"

Pre-flag those collapse; post-flag they are three distinct contracts. The payoff is precise modeling at the cost of more verbose interactions with `Partial<T>` patterns.

---

## 8. The smaller correctness flags

> "This setting ensures consistency between accessing a field via the \"dot\" (`obj.key`) syntax, and \"indexed\" (`obj[\"key\"]`) and the way which the property is declared in the type. The goal of this flag is to signal intent in your calling syntax about how certain you are this property exists."
> — TSConfig reference, https://www.typescriptlang.org/tsconfig/noPropertyAccessFromIndexSignature.html

> "`noImplicitOverride` — Ensures subclass methods override base class methods with the `override` keyword."
> "`noFallthroughCasesInSwitch` — Reports errors for fallthrough cases in switch statements."
> — TSConfig reference, https://www.typescriptlang.org/tsconfig/

**Trade-off.** All cheap correctness wins. `noImplicitOverride` catches "parent renamed, child silently no longer overrides." `noFallthroughCasesInSwitch` catches missing `break`. `noPropertyAccessFromIndexSignature` is more controversial — it forces `obj["key"]` for index-signature access, which some teams find noisy enough to disable; Pocock recommends most strictness flags but is selective on this one.

---

## 9. Module / emit flags that affect correctness

### `--isolatedModules`

> "Setting the `isolatedModules` flag tells TypeScript to warn you if you write certain code that can't be correctly interpreted by a single-file transpilation process."
> "It does not change the behavior of your code, or otherwise change the behavior of TypeScript's checking and emitting process."
> "Single-file transpilers don't know whether `someType` produces a value or not, so it's an error to export a name that only refers to a type."
> "when `isolatedModules` is set, it is an error to reference an ambient `const enum` member."
> — TSConfig reference, https://www.typescriptlang.org/tsconfig/isolatedModules.html

Bloomberg enforces it as a baseline at corporate scale (Bloomberg, *10 Insights from Adopting TypeScript at Scale*, https://www.bloomberg.com/company/stories/10-insights-adopting-typescript-at-scale/).

### `--verbatimModuleSyntax`

> "The rules are much simpler – any imports or exports without a `type` modifier are left around. Anything that uses the `type` modifier is dropped entirely."
> "With this new option, what you see is what you get."
> "Because `--verbatimModuleSyntax` provides a more consistent story than `--importsNotUsedAsValues` and `--preserveValueImports`, those two existing flags are being deprecated in its favor."
> — *Announcing TypeScript 5.0*, https://devblogs.microsoft.com/typescript/announcing-typescript-5-0/

### `--moduleResolution: "bundler"` vs node

> "To model how bundlers work, TypeScript now introduces a new strategy: `--moduleResolution bundler`."
> "If you are using a modern bundler like Vite, esbuild, swc, Webpack, Parcel, and others that implement a hybrid lookup strategy, the new `bundler` option should be a good fit for you."
> "If you're writing a library that's meant to be published on npm, using the `bundler` option can hide compatibility issues that may arise for your users who _aren't_ using a bundler."
> — same source

### `--erasableSyntaxOnly` (TS 5.8)

> "When this flag is enabled, TypeScript will error on most TypeScript-specific constructs that have runtime behavior."
> — *Announcing TypeScript 5.8*, https://devblogs.microsoft.com/typescript/announcing-typescript-5-8/

> "Today TypeScript 5.8 Beta ships the new \"erasableSyntaxOnly\" flag... It is designed to pair with Node's built-in TypeScript support, guiding users away from TS-only runtime features such as: ❌ enum ❌ runtime namespace ❌ parameter properties. Remember: TS = JS + Types."
> — Rob Palmer (Bloomberg, TypeScript steering committee), https://x.com/robpalmer2/status/1884715044585259127

**Trade-off.** This whole module/emit family is "correctness for the toolchain" rather than "correctness for the program logic." `isolatedModules` and `verbatimModuleSyntax` together let you swap `tsc` for esbuild/swc/Babel without surprise. `erasableSyntaxOnly` is the future-facing version, anticipating that `node app.ts` is now real. For libraries published to npm: do *not* use `bundler` resolution; use `node16`/`nodenext` so your published types match what non-bundler consumers see.

---

## 10. Build-perf flags affecting correctness

> "Tells TypeScript to save information about the project graph from the last compilation to files stored on disk. This creates a series of `.tsbuildinfo` files in the same folder as your compilation output."
> — TSConfig reference for `incremental`, https://www.typescriptlang.org/tsconfig/incremental.html

> "Referenced projects must have the new `composite` setting enabled. This setting is needed to ensure TypeScript can quickly determine where to find the outputs of the referenced project."
> "By separating into multiple projects, you can greatly improve the speed of typechecking and compiling, reduce memory usage when using an editor, and improve enforcement of the logical groupings of your program."
> — Project References handbook, https://www.typescriptlang.org/docs/handbook/project-references.html

> "Highly recommend turning on 'skipLibCheck' (unless you have loads of .d.ts files in your source) as without it TypeScript is probably running much slower for very little gain."
> — Pocock, https://www.totaltypescript.com/tsconfig-cheat-sheet

**Trade-off on `skipLibCheck`.** Cheapest large win in the entire config, with one well-known footgun: type errors *between two libraries* (e.g., conflicting `@types/node` versions in a monorepo) will be silently ignored. Small app: turn it on. Monorepo with version drift across `@types/*`: the bugs `skipLibCheck: false` reveals are real.

See `scale.md` for the full project-references / build-perf treatment.

---

## 11. The strictness migration pattern

The dominant pattern across published migration write-ups is **ratchet, not flip**:

1. Get to `noImplicitAny` clean.
2. Then `strictNullChecks` clean (the big one).
3. Then the rest of `--strict`.
4. Then `noUncheckedIndexedAccess` and `exactOptionalPropertyTypes` last, gated per-directory.

> "The typescript-strict-plugin allows adding strict mode to a TypeScript project without fixing all the errors at once."
> "Version 2.0 comes with a new script update-strict-comments, which detects all files with at least one strict error and adds the ignore comment to ease the migration."
> — Allegro, https://github.com/allegro/typescript-strict-plugin

> "This plugin lets you upgrade to your desired compilerOptions (e.g. strict, noUncheckedIndexedAccess, erasableSyntaxOnly) across your entire codebase, while letting problematic lines fall back to the old compilerOptions."
> — `@ts-migrating`, https://github.com/ycmjason/ts-migrating

> "When Bloomberg launched the beta version of their TypeScript platform support, more than 200 projects opted into TypeScript in the first year alone, and zero projects went back."
> — Bloomberg, *10 Insights from Adopting TypeScript at Scale* (via InfoQ summary, https://www.infoq.com/news/2020/11/bloomberg-typescript-adoption/)

The pattern that consistently works at scale is "per-file pragma + a CI check that prevents *new* files without the strict treatment." Old files migrate opportunistically.

Airbnb's tactical companion rule:

> "The 3.9 release of TypeScript introduced `@ts-expect-error` comments. When a line is prefixed with a `@ts-expect-error` comment, TypeScript will suppress that error. If there's no error, TypeScript will report that `@ts-expect-error` wasn't necessary."
> — Airbnb engineering, *ts-migrate*, https://medium.com/airbnb-engineering/ts-migrate-a-tool-for-migrating-to-typescript-at-scale-cd23bfeb5cc

**Why `@ts-expect-error` over `@ts-ignore`.** Suppressions become self-deleting once the underlying bug is fixed. Otherwise suppressions accumulate forever and the team loses track of which ones still hide a real issue.

---

## Direct quotes (catalog)

- "Future versions of TypeScript may introduce additional stricter checking under this flag." — TSConfig
- "strict: true — Indispensable." — Pocock
- "Methods are excluded specifically to ensure generic classes and interfaces (such as `Array<T>`) continue to mostly relate covariantly." — TS 2.6
- "A cursory check in a small project shows hundreds of errors in longstanding code where there aren't any existing complaints of unsoundness due to bivariance." — TS Wiki FAQ
- "Don't Consider Migration Complete Until You Enable noImplicitAny." — Vanderkam
- "any indexed access expression `obj[index]` used in a read position will include `undefined` in its type." — Cavanaugh PR
- "Indexed access types, e.g. `type A = SomeType[number]`, retain their current meaning." — Cavanaugh PR
- "every narrowing is based on syntactic patterns, and adding the form `e[i]` to the list of things we narrow on incurred a 5-10% performance penalty." — Cavanaugh PR
- "`colorThemeOverride: undefined` is not the same as `colorThemeOverride` not being defined." — TSConfig
- "any imports or exports without a `type` modifier are left around. Anything that uses the `type` modifier is dropped entirely." — TS 5.0
- "If you're writing a library that's meant to be published on npm, using the `bundler` option can hide compatibility issues." — TS 5.0
- "When this flag is enabled, TypeScript will error on most TypeScript-specific constructs that have runtime behavior." — TS 5.8 (`erasableSyntaxOnly`)
- "Remember: TS = JS + Types." — Rob Palmer
- "Highly recommend turning on 'skipLibCheck'... without it TypeScript is probably running much slower for very little gain." — Pocock
- "Zero projects went back." — Rob Palmer, Bloomberg
- "By adding //@ts-strict-ignore comment at the top of a file, its whole content will be removed from strict type checking." — Allegro

---

## Sources

1. https://www.typescriptlang.org/tsconfig/ — TSConfig reference (canonical).
2. https://www.typescriptlang.org/tsconfig/noUncheckedIndexedAccess.html
3. https://www.typescriptlang.org/tsconfig/exactOptionalPropertyTypes.html
4. https://www.typescriptlang.org/tsconfig/strictFunctionTypes.html
5. https://www.typescriptlang.org/tsconfig/noPropertyAccessFromIndexSignature.html
6. https://www.typescriptlang.org/tsconfig/isolatedModules.html
7. https://www.typescriptlang.org/tsconfig/verbatimModuleSyntax.html
8. https://www.typescriptlang.org/tsconfig/incremental.html
9. https://www.typescriptlang.org/docs/handbook/release-notes/typescript-2-6.html — `--strictFunctionTypes`.
10. https://www.typescriptlang.org/docs/handbook/release-notes/typescript-4-4.html — `useUnknownInCatchVariables`, `exactOptionalPropertyTypes`.
11. https://devblogs.microsoft.com/typescript/announcing-typescript-4-4-beta/ — Rosenwasser on 4.4 flags.
12. https://devblogs.microsoft.com/typescript/announcing-typescript-5-0/ — `verbatimModuleSyntax`, `bundler` resolution.
13. https://devblogs.microsoft.com/typescript/announcing-typescript-5-8/ — `erasableSyntaxOnly`.
14. https://www.typescriptlang.org/docs/handbook/project-references.html
15. https://github.com/microsoft/TypeScript/wiki/FAQ — strictFunctionTypes / method bivariance rationale.
16. https://github.com/microsoft/TypeScript/pull/39560 — Cavanaugh's `--noUncheckedIndexedAccess` PR.
17. https://github.com/microsoft/TypeScript/issues/49169 — Declined proposal to fold `noUncheckedIndexedAccess` into `--strict`.
18. https://www.totaltypescript.com/tsconfig-cheat-sheet — Pocock's prescriptive cheat sheet.
19. https://github.com/trungk18/effective-typescript-summary — Vanderkam Item 62 summary.
20. https://github.com/allegro/typescript-strict-plugin — Per-file `@ts-strict-ignore`.
21. https://blog.allegro.tech/2021/09/How-to-turn-on-TypeScript-strict-mode-in-specific-files.html — Migration playbook.
22. https://github.com/ycmjason/ts-migrating — Per-line fallback for non-strict-family flags.
23. https://medium.com/airbnb-engineering/ts-migrate-a-tool-for-migrating-to-typescript-at-scale-cd23bfeb5cc — Airbnb on `@ts-expect-error`.
24. https://x.com/robpalmer2/status/1884715044585259127 — Rob Palmer on `erasableSyntaxOnly`.
