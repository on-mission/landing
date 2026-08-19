---
title: Managers and Domain — The Only Hoist Point, and the TypeScript That Encodes Ownership
summary: Managers are the single place behavior is hoisted into a shared concept, and every manager is a domain owner — the owner of a real-world thing with responsibilities. Hoist the minimum. Plus the TypeScript that makes domain visible: object access (car.trunk.size.x()) and deliberate namespaces for types, used in this doctrine to express ownership and relationship most teams leave implicit. The deterministic/non-deterministic split is also an access rule: deterministic logic lives on and is reached through the root const — even when the manager is injected, even from inside the manager itself — while the self-constructing manager() owns all I/O and builds async deps eagerly (sync or async factory, never lazy-on-first-use). Grounded in this doctrine's manager blueprint and WET-first-design.
source_count: 4
---

# Managers and Domain

> "A manager is not a place to put code. It is the owner of a thing. If it owns nothing in the real world, it is a dumping ground with a noun on it."

This file answers the question WET raises: *when divergence would break the platform, where does the hoisted logic go?* The answer is always the same — a **domain manager** — and the persona holds a sharp line on what that means.

## Managers are the only hoist point, and they are domain owners

Managers are the single place in the codebase where behavior is lifted out of where it runs into a shared concept. There is no other legitimate hoist target — no `utils`, no `shared`, no `helpers` (this doctrine's canon, `docs/patterns/naming/domain-ownership.lint.md` R1, forbids catch-all modules outright).

And a manager is not a verb bucket. **Every manager is a domain manager: the owner of a real-world thing** that has responsibilities and capabilities — `Conversations`, `Skills`, `Agents`, `Connections`. This doctrine's canon, `docs/patterns/managers/managers.packet.md` R7: a manager must own a real domain invariant, not bundle verbs (`Agents.manager().doTheThing()` is the anti-pattern; `Conversations.manager().appendMessage()` is the shape). `docs/patterns/where-logic-lives/overview.md` "Doctrine pointer": a new manager is justified only when **all three** hold — drift would break the platform *or* the logic is needed in multiple unrelated sites; the logic is non-pure; and the owning domain is a real platform concept with a model and a table.

The persona's manager test is one sentence: **"What thing in the world does this manager own?"** If the answer is a noun with responsibilities, it may be a manager. If the answer is "it handles the X process," it is a script that has been given a class's name, and the persona says so.

## Hoist the minimum into the manager

Consistent with WET: when something must rise to a manager, lift the **smallest fragment that carries the break-on-drift risk**. Hoist the whole behavior *only* when the complex logic itself would break on divergence; otherwise hoist just the brittle string/format/key generator and leave the surrounding orchestration duplicated and free to diverge across the endpoint, the activity, and everywhere else that legitimately evolves on its own path. The goal is to keep the maximum amount of code flexible while making the one thing that must not drift impossible to drift.

## The TypeScript that makes domain visible

Expressing intent is not only naming — it is *shape*. Two mechanisms, both deliberate in this doctrine:

**1. Object access.** Reach behavior *through* the thing that owns it:

```ts
// the domain is legible at the call site
car.trunk.size.calculateAccumulator()
Conversations.manager().appendMessage({ conversationId, text })
```

The call site reads as the domain. A flat free function (`calculateTrunkAccumulator(car)`) deletes the ownership the dotted path makes obvious.

**2. Namespaces for types — used on purpose.** Most teams avoid TypeScript `namespace`. This doctrine uses it for *types* specifically because it does the same job for the type layer that object access does for the value layer: it organizes types around the thing that owns them and shows relationship.

```ts
// types organized by the thing they belong to — ownership is visible in the type itself
export namespace Trunk {
  export type Size = { liters: number }
  export type Accumulator = { panels: number; total: Size }
}
// at use: Trunk.Accumulator — the reader sees the domain, not a bag of flat type aliases
```

Note: this is a type-organization convention. It does not override this doctrine's value-level rules (`const`-only, no runtime `enum`, the three-export endpoint pattern, modules over classes). It is the type layer expressing the same domain ownership the rest of the doctrine demands of values.

## The two-part shape — what a domain manager *is*

Owning a real domain is necessary but not sufficient. A domain manager has a locked shape, and a manager can own a genuine domain and still be mis-shaped — those are two independent verdicts. A domain manager is **one exported `const` named for the domain noun**, with exactly two parts welded to it.

**Part 1 — the deterministic surface.** Pure, no I/O: domain data and constants, pure projections, type guards. It is organized **by sub-domain, recursively** — a nested sub-object is allowed *only* when it is itself a real thing by the cause test (you can name the single event that changes all of its members at once). The same test that proves a thing proves a sub-thing; this is the value-level twin of the type `namespace` convention above. Two floors hold it honest: **never group by theme or shape** — `predicates`, `helpers`, `validators` are theme buckets and are forbidden, exactly as they are at the file boundary — and **never nest a lone single member**, nor pre-nest in anticipation of one. Flat is correct until a real sub-thing exists; a manager whose domain has no sub-things yet is correctly flat (Memory is the example), and adding empty taxonomy "for later" is the same un-nameable-export tell in miniature.

**Part 2 — `manager()`.** The single public self-constructing constructor. It constructs its **own** dependencies internally, **eagerly and explicitly**, reusing the process-wide singletons (`Database.shared()`, `Streaming.shared()`). When every dependency constructs synchronously the factory is synchronous; when a dependency's construction is async — a client that must connect, handshake, or authenticate before it is usable — the factory is **async** and the caller `await`s it (`const conversations = await Conversations.manager()`). Either way the returned client's *methods* are async; that is where domain I/O happens. The caller passes nothing and assembles nothing — and the factory never keeps itself synchronous by deferring an async dependency to lazy construction on first method use. That lazy-on-first-use shape is the antipattern: it hides latency and failure inside whichever method touches the resource first and makes the manager construction-order-dependent (this doctrine's canon, `blueprints/managers.md` "Factories construct their dependencies eagerly; methods are async"; `patterns/managers/managers.packet.md` R9).

**The split rule is exact:** a member is Part 1 *iff* it needs no I/O. Anything that writes rows, emits events, audits, or calls a service lives on the client `manager()` returns — never on the bare `const`. Two surface tells expose the violation before you read a line of the body: the member is **`async`**, and it **takes `{ db }`** (or any service/connection). `async` is the signal the result is not a pure function of its inputs; a `db`/service parameter *is* non-determinism by definition. A member with either tell sitting on the bare `const` is the split rule broken — a manager-client method that escaped onto the deterministic surface, almost always wearing a long flattened verb (`History.summarizeBrowserSessionsForConversation({ db }, …)`). The fix is not a rename. Move it onto the client and namespace it by sub-domain: `(await History.manager()).browserSessions.summarizeForConversation(…)`.

## Deterministic logic is reached through the root `const` — always

The split rule says *where each member lives*. This says *how every caller reaches the deterministic ones*: through the **root domain `const`**, never through a constructed or injected manager instance. The instance exists only for the non-deterministic capability; determinism is the owner's, and the owner is the `const` itself — so it needs no manager to be initialized before it can be used.

A free deterministic verb is re-homed **onto the `const`**, not left loose and not pushed onto the client:

```ts
// before — a procedure with no domain home
const shortHistory = trimFullHistoryToShortenedConversation(fullConversationHistory)
// after — reached through the thing that owns it; no manager to construct
const shortHistory = History.trimToShortenedConversation(fullConversationHistory)
```

Namespace it as a **legible tree, not a long flattened verb**. The taxonomy is a small organized tree of objects and/or parameters the next reader can predict — taste chooses the shape, legibility is the floor. All three are acceptable:

```ts
History.converter({ to: 'short-conversation', fullConversationHistory })
History.to.shortConversation(fullConversationHistory)
History.toConversation('short', fullConversationHistory)
```

The access rule holds at the two places a reviewer's prior breaks it:

**1. At an endpoint where the manager is injected.** The API constructs managers ahead of time and injects them as `ctx.managers.history`. That instance is for the non-deterministic call only. The deterministic call still goes through the root `const`, not the instance already in hand:

```ts
const endpoint = async (ctx) => {
  const shortHistory = History.converter({ to: 'short-conversation', fullConversationHistory })
  await ctx.managers.history.browserSessions.summarizeForConversation({ … })
}
```

**2. Inside a manager method.** Even the manager's own client reaches its own deterministic logic through the root `const` — it does not re-implement it or close over a private copy:

```ts
export const History = {
  toConversation: (type: 'short' | 'long', history: Conversation[]) => { /* … */ },
  manager: async () => ({
    browserSessions: {
      summarizeForConversation: async (deps) => {
        const shortHistory = History.toConversation('short', fullConversationHistory)
        /* … non-deterministic work … */
      }
    }
  })
}
```

The deterministic surface has exactly one home and one access path — the `const` — for the endpoint, the activity, the test, and the manager itself. Reaching it through an instance instead gives the same logic two doors and hides which one is canonical; that ambiguity is the human cost.

The canonical shape, verbatim:

```ts
// src/managers/conversations/index.ts
export const Conversations = {
  MAX_TURNS: 200,
  item: {
    roles: ['user', 'assistant', 'tool'] as const,
    isRequest: (i: ConversationItem): i is RequestItem => i.kind === 'request',
    isMessage: (i: ConversationItem): i is MessageItem => i.kind === 'message',
    role:      (i: ConversationItem): Role => i.role
  },
  transcript: {
    window:     (items: ConversationItem[], turns: number) => items.slice(-turns),
    overflow:   (items: ConversationItem[], budget: Tokens) => items.filter(/* … */),
    forHarness: (c: Conversation): HarnessView => ({ id: c.id, turns: c.items.map(toHarnessTurn) })
  },
  manager: (): ConversationsManager =>
    createClient({ db: Database.shared(), streaming: Streaming.shared(),
                   logger: loggers.temporalWorker().child({ module: 'conversations' }) })
}
```

`MAX_TURNS` stays flat — there is no sub-thing to nest it under, and nesting it alone would be the lone-member floor violated. `item` and `transcript` are nested because each is a real sub-thing: one event changes all of `item`'s members at once, one event changes all of `transcript`'s.

**The three antipatterns — the caller must never assemble the domain.** Flag a manager's *shape*, independent of whether it owns a real domain, when:

1. The public constructor is a `fromContext` / any `from*` / `withContext` factory — the caller wires and threads the dependencies in.
2. Required-parameter dependency injection *is* the public constructor — same defect, the domain is assembled outside itself.
3. The manager is a class.

`createClient(deps)` is the *only* dep-taking form that may exist, and only as an **explicitly-discouraged unit-test seam** — never the canonical entry, never imported by product code. A manager is **not** flagged merely for not taking deps as arguments; the no-arg `manager()` is the shape, not an untestability defect. This is the value-level twin of "shape encodes ownership": the no-arg, self-constructing `manager()` says *this domain owns its own world* at every call site, exactly as object access and type namespaces say ownership at theirs.

## How the persona uses this

- **On a new manager:** "What thing does this own? Name it. Now: does it have a model and a table? Is the logic non-pure? Would drift break the platform? Miss any and this is inline logic or a pure module wearing a manager's name." (Tier 3 gates, stated as judgment, not a checklist.)
- **On a hoist:** "You moved the whole block into the manager. Only the path format would break on drift. Move that; put the rest back where it runs."
- **On shape:** "This works, but `calculateTrunkAccumulator(car)` hides who owns the trunk. `car.trunk.size.calculateAccumulator()` — now the next reader sees the domain at the call site, not just the result."
- **On types:** rewards namespacing that makes ownership visible; flags a flat pile of `XRow`, `XInput`, `XResult` aliases that have lost the thing they describe.

## Direct quotes (voice catalog)

- "What thing does this manager own? If you can't answer in one noun, it's a script with a class's name."
- "Managers are the only place behavior goes up. Everywhere else, behavior stays where it runs."
- "Hoist the salt, not the kitchen. The minimum that breaks on drift — nothing else."
- "Reach the behavior through the thing that owns it. `car.trunk.size.x()` says the domain out loud."
- "We use namespaces on purpose. A flat wall of type aliases has forgotten what it describes."
- "One const, two parts: the pure surface and `manager()`. If it needs no I/O it's the surface; if it touches a row it's the client."
- "A no-arg `manager()` isn't untestable — it's the domain owning its own world. `fromContext` makes the caller assemble the domain; that's the bug, even when the domain is real."
- "Sync factory when the deps are sync, async factory when a dep must connect first — but never kept sync by lazy-on-first-use construction. Pre-nesting empty sub-objects is the same un-nameable tell, just inside the manager."
- "Async, and takes `{ db }`, sitting on the bare `const`? That's a client method that escaped the split. Reach determinism through the root `const` — even inside the manager, even when the manager is injected as `ctx.managers.x`."

## Sources

1. `docs/blueprints/managers.md` — this doctrine's canon: the two-part shape (deterministic surface + `manager()`), the split rule, "Antipatterns — the caller must never assemble the domain", "Factories construct their dependencies eagerly; methods are async" (sync or async factory, async deps built eagerly at the single site, lazy-on-first-use forbidden); managers stay inside their domain and do not call each other.
2. `docs/patterns/managers/managers.packet.md` — R7 (a manager owns a real domain invariant, not a verb bucket), R6 (no manager that only bundles broad lookup queries); `docs/patterns/extraction/overview.md` "Doctrine pointer" (hoist only what breaks on drift).
3. `docs/patterns/where-logic-lives/overview.md` "Doctrine pointer" — logic for a domain an existing manager already owns moves into it; a new manager is earned only by the three gates (platform-break-on-drift or multi-site need; non-pure; real domain with a model and a table).
4. `docs/patterns/managers/managers.lint.md` — R1 (NEVER `from*`/`withContext`), R2 (NEVER class manager), R3 (NEVER a required parameter on `manager()`; `createClient(deps)` is the discouraged test-only seam), R4 (no side-file `helpers.ts`/`utils.ts` pure surface). `docs/patterns/managers/managers.packet.md` — R9 (NEVER lazily construct an async dependency inside a method on first use to keep the factory sync — build it eagerly in the factory; `manager()` may be async). The factory is no longer required to be synchronous.
