---
title: "TypeScript at Scale — Build Performance, Project References, Monorepos, and Tooling"
summary: >
  Primary-source corpus on TypeScript in large codebases: project references, build perf,
  monorepo structures, isolated declarations, the Go-based native compiler (TS 7.0),
  type-checking separated from build (swc/esbuild), tsserver memory pressure, typed linting,
  type tests, and dead-code detection. Drawn from the TS team's Performance wiki, the
  Project References handbook, the TS 5.5 / 5.8 / 7.0 release announcements, Hejlsberg's
  Go port post, Vanderkam, Pocock, and engineering posts from Bloomberg, Airbnb, Slack,
  Asana, and others.
source_count: 26
---

# TypeScript at Scale

> "5-20 projects is an appropriate range — fewer may result in editor slowdowns and more may result in excessive overhead." — TypeScript Performance wiki

The TS team's own bound on monorepo project counts is the load-bearing rule of thumb behind every "do I need project references?" debate. Below five, splitting overhead exceeds the cache benefit; above twenty, editor and build cost of tracking inter-project state begins to dominate.

---

## 1. Project references

> "Project references allows you to structure your TypeScript programs into smaller pieces, available in TypeScript 3.0 and newer."
> "By doing this, you can greatly improve build times, enforce logical separation between components, and organize your code in new and better ways."
> "By separating into multiple projects, you can greatly improve the speed of typechecking and compiling, reduce memory usage when using an editor, and improve enforcement of the logical groupings of your program."
> "Referenced projects must have the new `composite` setting enabled."
> "Running `tsc --build` (`tsc -b` for short) will... find all referenced projects, detect if they are up-to-date, build out-of-date projects in the correct order."
> — *Project References* handbook, https://www.typescriptlang.org/docs/handbook/project-references.html

> "When building up any codebase of a non-trivial size with TypeScript, it is helpful to organize the codebase into several independent _projects_."
> "5-20 projects is an appropriate range — fewer may result in editor slowdowns and more may result in excessive overhead."
> "If you're working in a monorepo, this can be as simple as creating a project for each package and mirroring the package dependency graph in project references."
> — TS Performance Wiki, https://github.com/microsoft/TypeScript/wiki/Performance

Pocock has publicly retreated from project references for his own monorepo:

> "Moved my monorepo from tsconfig references to @turborepo, and turbo watch mode. It's pretty nice, folks."
> — Pocock, https://x.com/mattpocockuk/status/1810980463097311710

> "This setup is not hard to manage: 1. A dev script that runs tsc --watch 2. A build script that runs tsc 3. Use @total-typescript/tsconfig/tsc/no-dom/library-monorepo as the tsconfig 4. Use @turborepo to co-ordinate running these scripts."
> — Pocock, https://x.com/mattpocockuk/status/1791391792476168518

**Trade-off.** Project references buy you incremental compilation across packages and stronger boundary enforcement at the cost of configuration complexity and the requirement that every referenced project emit `.d.ts`. At small-to-medium monorepo scale, per-package `tsc --watch` orchestrated by Turborepo can replace project references, trading TS-native incremental for an external task-graph cache. Both compose with `composite: true` / `declarationMap: true`. The decision turns on team familiarity and tsserver responsiveness — there is reasonable evidence (TS Issue #42607) that very large project-reference graphs degrade language-service performance.

---

## 2. `skipLibCheck` and the cost of checking `.d.ts`

> "you can also enable the `skipLibCheck` flag to skip checking _all_ `.d.ts` files in a compilation."
> "The options `skipDefaultLibCheck` and `skipLibCheck` can often hide misconfiguration and conflicts in .d.ts files, so we suggest using them only for faster builds."
> — TS Performance Wiki, https://github.com/microsoft/TypeScript/wiki/Performance

> "Skips checking the types of `.d.ts` files. This is important for performance, because otherwise all `node_modules` will be checked."
> — Pocock, *TSConfig Cheat Sheet*, https://www.totaltypescript.com/tsconfig-cheat-sheet

Empirical confirmation of cost:

> "Adding a single dependency adds a whopping 400M of memory usage and roughly 4 seconds of runtime."
> "[with --skipLibCheck] not _that_ much of an improvement, but we got rid of the check time, and about _~100M_ of memory usage."
> — Arpad Borsos, https://swatinem.de/blog/optimizing-tsc/

**Trade-off.** Cheapest large win in the entire config. The footgun is real but asymmetric: type errors *between two libraries* (e.g., conflicting `@types/node` versions in a monorepo) get silently ignored. App code: turn it on. Library code that publishes types: probably leave it on but run a stricter check in CI.

---

## 3. What makes `tsc` slow — the type-system cost model

The TS Performance wiki is unusually candid about expensive constructs:

> "if your union has more than a dozen elements, it can cause real problems in compilation speed."
> "Every time an argument is passed to `printSchedule`, it has to be compared to each element of the union."
> "to eliminate redundant members from a union, the elements have to be compared pairwise, which is quadratic."
> "Every time `foo` is called, TypeScript has to re-run the conditional type."
> "Relating any two instances of `SomeType` requires re-relating the structure of the return type of `foo`."
> "Interfaces create a single flat object type that detects property conflicts, which are usually important to resolve!"
> "Type relationships between interfaces are also cached, as opposed to intersection types as a whole."
> "Adding type annotations, especially return types, can save the compiler a lot of work."
> "Named types tend to be more compact than anonymous types... which reduces the amount of time spent reading and writing declaration files."
> "You should always make sure that your configuration files aren't including too many files at once."
> "`extendedDiagnostics` can give you a sense of the work the compiler is spending time on."
> "`showConfig` can explain what `tsc` will calculate for an invocation."
> — TS Performance Wiki, https://github.com/microsoft/TypeScript/wiki/Performance

**Three concrete cost rules from the official wiki, in priority order.** Unions cost O(n²) in member count. Conditional types are re-evaluated at every call site that lacks a name. Intersections defeat caching that interfaces enjoy. These are the only "type system at scale" claims worth treating as authoritative without further benchmarking.

---

## 4. Type-checking separated from build

> "type-checking typically requires information from other files, and can be relatively expensive compared to other steps like transforming/emitting code."
> — TS Performance Wiki

> "Unlike the native TypeScript compiler, tools like SWC and Babel compile each file separately and cannot determine whether an imported name is a type or value; the `isolatedModules` option prevents syntax that SWC and Babel cannot compile correctly."
> — Leapcell blog, https://leapcell.io/blog/navigating-typescript-transpilers-a-guide-to-tsc-esbuild-and-swc

> "isolatedModules is a sensible default because it makes your code more portable if you ever need to switch to a different transpiler."
> "[verbatimModuleSyntax] enforces the `import type` syntax while also preventing module system mixing. When enabled, TypeScript will error if you try to use `require` in an ESM file, or `import` in a CJS file."
> — Pocock, *Configuring TypeScript*, https://www.totaltypescript.com/books/total-typescript-essentials/configuring-typescript

**The pattern.** Use `tsc --noEmit` for type-checking; use swc/esbuild for actual emit. Type-check time stays roughly proportional to your code; build time becomes near-instant. `isolatedModules` and `verbatimModuleSyntax` are the correctness guardrails that keep this swap safe.

---

## 5. `isolatedDeclarations` (TS 5.5)

> "If a fast tool could generate all those declaration files for `core` _in parallel_, TypeScript then could immediately follow that by type-checking `core`, `frontend`, and `backend` also _in parallel_."
> "Work on `isolatedDeclarations` has been a long-time collaborative effort between the TypeScript team and the infrastructure and tooling teams within Bloomberg and Google."
> "Declaration files only require the types of the public API of a module – in other words, the types of the things that are exported."
> — *Announcing TypeScript 5.5*, https://devblogs.microsoft.com/typescript/announcing-typescript-5-5/

**The point.** `isolatedDeclarations` is the fix for the structural reason monorepos hit a wall with project references: declaration emit currently requires full type-checking, so package N+1 cannot start until package N is fully checked. Decoupling those two stages — Bloomberg's Titian Cernicova-Dragomir is named as the driver — is what enables a cluster scheduler (Bazel, Nx, Turbo) to fan out across cores. The cost: every exported symbol must have an explicit type annotation. The benefit: parallel-by-package builds.

---

## 6. The Go-based native compiler (TS 7.0)

> "The native implementation will drastically improve editor startup, reduce most build times by 10x, and substantially reduce memory usage."
> "We expect to be able to preview a native implementation of `tsc` capable of command-line typechecking by mid-2025, with a feature-complete solution by the end of the year."
> "The current time to load the entire project in the editor on a fast computer is about 9.6 seconds. This drops down to about 1.2 seconds with the native language service, an 8x improvement."
> "Overall memory usage also appears to be roughly half of the current implementation."
> — *A 10x TypeScript*, https://devblogs.microsoft.com/typescript/typescript-native-port/

> "Half of the speedup comes from shared memory concurrency and using multiple cores. The other half comes from native code."
> "Go is lower-level than C# … Go has better support for producing native code."
> — 2ality on Hejlsberg interview, https://2ality.com/2025/03/typescript-in-go.html

> "TypeScript 7.0 is often about 10 times faster than TypeScript 6.0."
> "The new Go codebase was methodically ported from our existing implementation rather than rewritten from scratch."
> "Already in use in multiple multi-million line-of-code codebases both inside and outside Microsoft."
> — *Announcing TypeScript 7.0 Beta*, https://devblogs.microsoft.com/typescript/announcing-typescript-7-0-beta/

Vanderkam:

> "In terms of new TypeScript features, 2025 was a very quiet year."
> "The TypeScript team at Microsoft has been working on a massive project to port `tsc` and `tsserver` from TypeScript to Go."
> "So bootstrapping is, in principle, good. That being said, a 10x speedup should make you question your principles."
> "Sometime next year, you'll update your packages and everything will get 10x faster."
> "Slow compiler and language service performance has always been one of the biggest complaints about TypeScript."
> — Vanderkam, https://effectivetypescript.com/2025/12/19/ts-2025/

A real migration measurement:

> "Initial .husky/pre-push baseline: 289.14s"
> "Full .husky/pre-push after the change: 14.36s"
> "tsgo via bun run typecheck: 7.02s" versus "tsc fallback via typecheck:tsc: 200.68s"
> "A 5-minute check becomes a negotiation... A 14-second check is different. You just run it."
> "slow checks do not just waste time. They change behavior. Fast checks make the right behavior easy."
> "I would not ship this migration without a `tsc` fallback."
> — Yann Cabral, https://www.yanncabral.dev/blog/tsgo-typescript-checks

**The point.** The Go port is strategically the biggest event in TS-at-scale tooling since project references. Hejlsberg's framing is that half the win is concurrency (which `tsc` could never get on Node) and half is native code. The behavioral observation in Cabral's migration post — that checks below ~15s change developer behavior — is the practical case for chasing the 10x. Strict-flag adoption decisions made under "tsc takes 5 minutes" should be revisited under "tsc takes 15 seconds."

---

## 7. Bloomberg, Airbnb, Slack, Asana — scaling lessons

Rob Palmer (Bloomberg JS infra and tooling lead):

> "Our platform supports an internal ecosystem of packages that uses a common tooling and publishing system. This allows us to encourage and enforce best practices, such as defaulting to TypeScript's 'strict mode', as well as ensuring global invariants."
> "Engineers were self-starting conversions and championing the process! When we launched the beta version of our TypeScript platform support, more than 200 projects opted into TypeScript in the first year alone. Zero projects went back."
> — InfoQ summary of Bloomberg post, https://www.infoq.com/news/2020/11/bloomberg-typescript-adoption/

Bloomberg's footprint: "more than 50 million lines of JS code, more than 10,000 apps, more than 2,000 software engineers" (Bloomberg, *10 Insights from Adopting TypeScript at Scale*, https://www.bloomberg.com/company/stories/10-insights-adopting-typescript-at-scale/).

Airbnb on `ts-migrate`:

> "We were able to convert projects with more than 50,000 lines of code and 1,000+ files from JavaScript to TypeScript in one day with the use of codemods!"
> "An all-in migration will guarantee that the state of every file is the same, and engineers won't need to remember where they can use TypeScript features."
> "At this time, ~86% of our 6M-line frontend monorepo has been converted to TypeScript and we're on track for 95% by the end of the year."
> "Our goal is to get a compiling TypeScript project with basic type coverage that does not result in an application runtime behavior change."
> — Airbnb, https://medium.com/airbnb-engineering/ts-migrate-a-tool-for-migrating-to-typescript-at-scale-cd23bfeb5cc

Slack:

> "Modern JavaScript is valid TypeScript, meaning that one can use TypeScript without changing a single line of code."
> "We were surprised by the number of small bugs we found when converting our code."
> "TypeScript was such a boon to our stability and sanity that we started using it for all new code within days of starting the conversion."
> "By the time a Pull Request is opened, we already have the confidence that the structural dependencies within our code are sound."
> — Slack, https://slack.engineering/typescript-at-slack/

Asana:

> "In our current code base there are many changes that we want to make but we are scared of what we will break when we make the change."
> "Refactoring support and better code navigation also make a huge difference to developer productivity."
> "Strong typing allows the compiler and IDE to catch errors early... There is a large productivity difference between finding out about errors while you are coding and finding out when your tests run several hours later."
> "static typing helps us to ensure that the client and server agree on a protocol."
> — Asana, https://asana.com/inside-asana/asana-switching-typescript

**The pattern across all four.** Mixed JS/TS monorepos rot — developers stop reasoning about which file they're in and lose the type-system guarantee at every boundary. All-in conversion is the right shape; codemods make it tractable. The qualitative payoff that shows up consistently: types as a *refactoring tool*, enabling structural changes that fear had previously prevented.

---

## 8. tsserver / IDE performance at scale

> "TSServer memory consumption is often 1-2 GB, which is too much for comfortable work, and the TypeScript Language Server (tsserver) often causes high CPU and memory usage, leading to slow performance in editors like VS Code."
> "Using global imports in every file forces the compiler to handle a huge number of file dependencies, greatly slowing down compilation because TypeScript has to re-analyze the same declarations repeatedly."
> "When under memory pressure, the CPU spends ~90-95% non-productively garbage collecting, and some requests seem to never complete or complete 10-20X slower."
> — Aggregated from microsoft/TypeScript issues #18055, #30034, #30981 and microsoft/vscode #140090

**The practical fix today.** Project references (with the 5–20 projects rule), `skipLibCheck: true`, named return types on hot generics, splitting large unions into base-type hierarchies, and avoiding deeply-nested conditional types in the hot path. The fix tomorrow: TS 7.0.

---

## 9. Typed linting — what `tsc` doesn't catch

> "Linting with type information, also called 'typed linting' or 'type-aware linting', is the act of writing lint rules that use type information to understand your code."
> "The `any` type can easily slip into code and reduce type safety, despite being allowed by the TypeScript type checker."
> "Determining whether code is creating a floating Promise is only possible when the types of code are known."
> "It is inevitable that typed linting will slow your linting down to roughly the speed of type checking your project."
> "The additional bug catching and features added by typed linting are well worth the costs of configuration and performance."
> — typescript-eslint, https://typescript-eslint.io/blog/typed-linting/

**The gap.** `tsc` does not catch floating promises, accidental `any`s, unused promises, mis-typed `void` callbacks, or redundant `as` casts. Typed-linting rules from typescript-eslint (`no-floating-promises`, `no-misused-promises`, `no-unsafe-*`, `no-unnecessary-type-assertion`) catch the most common production bug classes that the type checker accepts.

---

## 10. Dead-code detection and type tests

Dead code:

> "Basically, yes! `knip` uses the same sort of mark-and-sweep algorithm as `ts-prune` to find dead code... But it's much more ambitious in the sorts of issues it tries to find."
> "While `ts-prune` was effective at its core job, it always had a few shortcomings: it couldn't detect unused dependencies or mutually recursive dead code."
> "Once you get down to zero errors, add `knip` to your CI to ensure that you never have dead code again!"
> — Vanderkam, https://effectivetypescript.com/2023/07/29/knip/

Type tests:

> "expect-type is a tool for compile-time tests for types that helps ensure types don't regress into being overly-permissive as changes go in over time."
> "expect-type requires no extra build step, CLI tool, IDE extension, or lint plugin — you just import the function and start writing tests, with failures appearing at compile time in your IDE and when you run tsc."
> "expect-type checks generics properly and strictly, whereas tsd doesn't."
> — Aggregated from https://www.npmjs.com/package/expect-type and https://github.com/tsdjs/tsd

**The point.** A library's published types are part of its public API. Without type tests, behavioral regressions in those types are invisible until a consumer breaks. expect-type and tsd both treat types as a first-class testing target — used by tRPC, Apollo, Prisma, Bun, Vue, Puppeteer, Socket.IO.

---

## 11. Public package types — dual ESM/CJS, NodeNext, bundler resolution

> "`module: NodeNext` is the best option for Node. `moduleResolution: NodeNext` is implied."
> "`module: preserve` is the best option because it most closely mimics how bundlers treat modules. `moduleResolution: Bundler` is implied."
> "`declaration: true` — Tells TypeScript to emit `.d.ts` files. This is needed so that libraries can get autocomplete on the `.js` files you're creating."
> "If you're building for a library in a monorepo: `composite: true`, `declarationMap: true`."
> — Pocock, https://www.totaltypescript.com/tsconfig-cheat-sheet

> "The 'types' field MUST be first in each export condition within the package.json exports object. This ensures TypeScript can properly locate type definitions before attempting to resolve the actual implementation files."
> — Mayank, *Dual Packages*, https://mayank.co/blog/dual-packages/

**The trap.** Library authors using `bundler` resolution because it's smoother for development can ship packages whose types resolve correctly only for bundler-using consumers. Plain Node consumers hit "module has no exported member" errors that the author's CI never sees. Use `node16` / `nodenext` for libraries and verify resolution in a clean Node-native consumer.

---

## Direct quotes (catalog)

- "5-20 projects is an appropriate range." — TS Performance wiki
- "if your union has more than a dozen elements, it can cause real problems in compilation speed." — TS Performance wiki
- "to eliminate redundant members from a union, the elements have to be compared pairwise, which is quadratic." — TS Performance wiki
- "Every time `foo` is called, TypeScript has to re-run the conditional type." — TS Performance wiki
- "Adding type annotations, especially return types, can save the compiler a lot of work." — TS Performance wiki
- "The native implementation will drastically improve editor startup, reduce most build times by 10x." — Hejlsberg
- "Half of the speedup comes from shared memory concurrency and using multiple cores. The other half comes from native code." — 2ality on Hejlsberg
- "a 10x speedup should make you question your principles." — Vanderkam
- "Sometime next year, you'll update your packages and everything will get 10x faster." — Vanderkam
- "A 5-minute check becomes a negotiation... A 14-second check is different. You just run it." — Cabral
- "slow checks do not just waste time. They change behavior. Fast checks make the right behavior easy." — Cabral
- "Zero projects went back." — Rob Palmer, Bloomberg
- "An all-in migration will guarantee that the state of every file is the same." — Airbnb
- "In our current code base there are many changes that we want to make but we are scared of what we will break." — Asana
- "By the time a Pull Request is opened, we already have the confidence that the structural dependencies within our code are sound." — Slack
- "It is inevitable that typed linting will slow your linting down to roughly the speed of type checking your project." — typescript-eslint
- "If a fast tool could generate all those declaration files for `core` _in parallel_, TypeScript then could immediately follow that by type-checking `core`, `frontend`, and `backend` also _in parallel_." — TS 5.5
- "Once you get down to zero errors, add `knip` to your CI to ensure that you never have dead code again!" — Vanderkam

---

## Sources

1. https://github.com/microsoft/TypeScript/wiki/Performance — Canonical TS team performance guide.
2. https://www.typescriptlang.org/docs/handbook/project-references.html — `composite`, `references`, `tsc --build`.
3. https://devblogs.microsoft.com/typescript/typescript-native-port/ — Hejlsberg/Rosenwasser announcement of the Go port.
4. https://devblogs.microsoft.com/typescript/announcing-typescript-7-0-beta/ — TS 7.0 beta; named scale-customer list.
5. https://devblogs.microsoft.com/typescript/announcing-typescript-5-5/ — `isolatedDeclarations`.
6. https://2ality.com/2025/03/typescript-in-go.html — Detailed analysis of the Go port.
7. https://effectivetypescript.com/ — Vanderkam index.
8. https://effectivetypescript.com/2025/12/19/ts-2025/ — 2025 retrospective focused on Go rewrite.
9. https://effectivetypescript.com/2023/07/29/knip/ — Vanderkam recommending Knip.
10. https://www.totaltypescript.com/tsconfig-cheat-sheet — Pocock's tsconfig recommendations.
11. https://www.totaltypescript.com/books/total-typescript-essentials/configuring-typescript — `isolatedModules`, `verbatimModuleSyntax`.
12. https://x.com/mattpocockuk/status/1810980463097311710 — Pocock leaving project references for Turborepo.
13. https://x.com/mattpocockuk/status/1791391792476168518 — Pocock's tsc-watch + Turborepo recipe.
14. https://www.bloomberg.com/company/stories/10-insights-adopting-typescript-at-scale/ — Bloomberg 50M-line scale story.
15. https://www.infoq.com/news/2020/11/bloomberg-typescript-adoption/ — Rob Palmer quotes.
16. https://medium.com/airbnb-engineering/ts-migrate-a-tool-for-migrating-to-typescript-at-scale-cd23bfeb5cc — Airbnb 6M-line migration playbook.
17. https://slack.engineering/typescript-at-slack/ — Slack gradual-typing story.
18. https://asana.com/inside-asana/asana-switching-typescript — Asana on types as a refactoring tool.
19. https://typescript-eslint.io/blog/typed-linting/ — Typed-linting rationale and perf trade-off.
20. https://www.yanncabral.dev/blog/tsgo-typescript-checks — Production tsgo migration with concrete numbers.
21. https://swatinem.de/blog/optimizing-tsc/ — Memory cost of dependencies and `skipLibCheck` quantified.
22. https://github.com/microsoft/TypeScript/issues/42607 — Project references decreasing tsserver perf (counter-evidence).
23. https://github.com/microsoft/TypeScript/issues/18055 — tsserver memory pressure issue.
24. https://www.npmjs.com/package/expect-type — Type-test tool used by tRPC, Apollo, Prisma.
25. https://github.com/tsdjs/tsd — Type-test tool used by Bun, Puppeteer, Vue.
26. https://mayank.co/blog/dual-packages/ — Dual ESM/CJS publishing rules.
