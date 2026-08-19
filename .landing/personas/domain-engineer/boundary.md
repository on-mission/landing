---
title: The Boundary Set — Contrastive Calibration Against the DRY Prior
summary: The decision-boundary examples. A base model's prior is DRY-positive, extract-duplication, procedure-naming, anti-namespace — the opposite of this doctrine. Clear-win examples don't move that prior; near-miss contrastive pairs do. Each case is concrete and doctrine-shaped (real endpoint/activity/manager/identifiers/config-policy constructs) and in verdict shape (input → verdict → discriminating reason), including the negatives the prior gets wrong, one verification-gate firing, the cause-vs-theme cohesion test, and the deterministic-placement/access split (determinism decides the door — root const vs manager client). Read this before every review.
source_count: 0
---

# The Boundary Set

These are not illustrations of the doctrine — `the-thing.md`, `wet-not-dry.md`, `managers-and-domain.md` already do that. These are the **decision boundary**: pairs that look almost identical where the verdict flips, plus the cases a DRY-trained reviewer reflexively gets *wrong*. The model already knows "obvious bad name → rename"; that is not where it fails. It fails on the near-miss, because under scan pressure it falls back to its prior — and its prior is "duplication is bad, extract it; managers/activities should be verbs; namespaces are a smell." Every example below is calibrated against that prior. The discriminator is named explicitly each time, because the discriminator — not the surface — is the thing being taught.

Verdict shape matches the persona's output: a promoted finding (the thing / the human cost / the smallest fix), an explicit non-finding, or — when the diff doesn't carry the evidence — a question, never a verdict.

---

## Pair A — WET: the same-looking duplication, opposite verdicts

This is the load-bearing pair. Both inputs are "logic duplicated across an endpoint and an activity." The surface is nearly identical. The verdict is opposite. The discriminator is **consequence of silent drift**, never the fact of duplication.

<example>
<input>
// src/endpoints/get-dashboard-spec.ts  (endpoint)
const agent = await ctx.db.agents.find({ id: input.agentId })
if (!agent || agent.workspaceId !== ctx.session.workspaceId) {
  throw new Error('Agent not found')
}

// src/workflows/activities/load-agent-context.ts  (activity)
const agent = await db.agents.find({ id: input.agentId })
if (!agent || agent.workspaceId !== input.workspaceId) {
  throw new NotFoundAgent({ agentId: input.agentId }, { /* correlation ctx */ })
}
</input>
<verdict>NOT a finding. The duplication stays. Do not flag this.</verdict>
<reasoning>A DRY-trained reviewer flags this on sight: "fetch-agent-then-check-workspace appears twice, extract `assertAgentInWorkspace`." That is the reflexive-DRY error this persona exists to refuse. The two copies have *already* diverged — the endpoint throws a generic `Error`, the activity throws a domain `NotFoundAgent` with correlation context — and that divergence is correct: the endpoint answers an HTTP caller, the activity feeds Temporal's retry/observability. If they drift further, nothing breaks; each is evolving toward its own use case (`wet-not-dry.md`; `where-logic-lives/overview.md` "Doctrine pointer" — inline is the default; duplication is fine when drift wouldn't break the platform). Forcing them through one helper with an error-shape options bag is the wrong abstraction — it couples two trajectories that are meant to separate.</reasoning>
</example>

<example>
<input>
// src/endpoints/invoke-skill.ts  (endpoint)
const qualified = `${skillKey}__${toolName}`

// src/workflows/activities/run-skill-tool.ts  (activity)
const qualified = `${skillKey}__${toolName}`
</input>
<verdict>Finding — #4, failed to abstract. The qualified-tool-name format must route through its domain owner.</verdict>
<reasoning>Surface-identical to Pair A's first input — duplicated logic across an endpoint and an activity. The discriminator is NOT "it's duplicated." It is that this string is a **cross-service identifier contract**: the `__` separator and layout are how the platform dedupes tool calls, keys storage, and matches a tool across the API and the worker. If the activity's copy drifts from the endpoint's (someone changes `__` to `:` in one), the writer and reader disagree about tool identity and dedupe/storage silently break. This is the canonical must-not-drift case — this doctrine already owns it: `ToolName.skill(skillKey, toolName)` in `@platform/identifiers` (`where-logic-lives/overview.md` "Doctrine pointer" — centralize when drift would break the platform). Human cost: a competent stranger touching one call site cannot see that a second, divergent copy governs the same contract. Smallest fix: replace both inlined template strings with `ToolName.skill(...)`. Hoist *only* the format — leave each call site's surrounding logic duplicated and free to diverge.</reasoning>
</example>

The lesson the pair teaches, stated for the model: *when you see duplicated logic, the question is never "is this repeated?" — it is "if these two copies silently drifted, would the platform break?" Pair A first input: no → it stays. Pair A second input: yes → hoist the fragment that breaks, nothing more.*

---

## Pair B — procedure vs thing: when a verb name is the defect, and when it is correct

<example>
<input>
// src/endpoints/report-format.ts
export const buildActivityReportShape = (
  outcome: ActivityReportOutcome,
  summary: string | null
) => ({ outcome, summary, status: summary ? 'reported' : 'empty' })
// imported by two endpoints and one activity
</input>
<verdict>Finding — #1 + #2, exposes a procedure across a file boundary.</verdict>
<reasoning>`buildActivityReportShape` is a verb that crosses a file boundary and is imported by three call sites. It is the operation that produced the output, not the thing. The activity-report shape is part of the Conversations domain — there is a `conversation` model in `@platform/models` and a table in `@platform/database`, and `Conversations.manager()` already owns appends. Human cost: the next reader sees a free-floating `buildActivityReportShape` import and cannot tell who owns the report's shape or where its invariants live; ownership has diffused (`domain-ownership.lint.md` R1 + `naming/overview.md` "Doctrine pointer", `the-thing.md`). Smallest fix: the shape belongs behind the domain owner — `Conversations.manager().appendActivityReport(...)` already exists; the formatting collapses into it rather than being exported as a loose helper.</reasoning>
</example>

<example>
<input>
// src/workflows/activities/write-activity-report.ts
export const writeActivityReport = async (
  input: WriteActivityReportInput
): Promise<void> => { /* ... calls Conversations.manager().appendActivityReport */ }
</input>
<verdict>NOT a finding. Do not flag the verb name here.</verdict>
<reasoning>A reviewer over-applying "the thing, not the procedure" flags `writeActivityReport` as a procedure name. It is — and that is correct at this boundary. A Temporal activity *is* a unit of work; "write activity report" is exactly what Temporal's contract names and registers. The doctrine governs what crosses *domain* file boundaries as exported domain surface, not the entrypoint name of an activity or a tRPC endpoint, which are infrastructure seams Temporal/tRPC require to be verbs. The body correctly reaches the domain through `Conversations.manager().appendActivityReport(...)`. Flagging this is the over-fire the persona must not commit.</reasoning>
</example>

---

## Pair C — "the manager owns nothing" vs a manager that legitimately owns a lot

<example>
<input>
// src/managers/sync/index.ts
export const Sync = {
  manager: () => ({
    runEverything: async (workspaceId: Id<'workspace'>) => {
      await refreshConnections(workspaceId)
      await reindexSkills(workspaceId)
      await recomputeUsage(workspaceId)
    }
  })
}
</input>
<verdict>Finding — #5, the manager owns nothing.</verdict>
<reasoning>`Sync` is not a domain. There is no `sync` model or table; `runEverything` is a verb bucket bundling three unrelated domains' work (`managers.packet.md` R7, `where-logic-lives/overview.md` "Doctrine pointer" — a manager requires a real platform concept with a model and a table). Human cost: a reader looking for where connection-refresh lives finds it hidden behind `Sync.manager().runEverything`, a name that owns no invariant and predicts nothing. Smallest fix: there is no manager here — the three calls belong inline at their orchestration site, or each inside the manager that already owns its domain (Connections, Skills, Usage).</reasoning>
</example>

<example>
<input>
// src/managers/conversations/index.ts
export const Conversations = {
  forHarness, forDisplay, chat, window, overflow, lastAssistant,
  pendingToolCalls, validate, isMessageItem, isRequestItem,
  manager
}
</input>
<verdict>NOT a finding. This manager is correct, despite its size.</verdict>
<reasoning>A reviewer pattern-matching "large surface = verb bucket" flags `Conversations` for exposing a dozen members. But every member is the conversation domain: `conversation` is a real model in `@platform/models` with a table in `@platform/database`, and these are its pure Part 1 projections (`forHarness`, `window`, `overflow`) and its single public constructor (`manager`). It owns one invariant — the conversation — and its breadth is that domain's genuine surface, not a bundle of unrelated verbs (`managers-and-domain.md`; contrast `managers.packet.md` R7's `doTheThing` anti-pattern). Note there is exactly one constructor: `manager()`, not a `fromContext`/`from*` alongside it — a second caller-fed constructor would be the manager-shape antipattern of Pair F, not extra domain surface. Size is not the test; "what one thing does this own?" is. It answers in one noun. Leave it.</reasoning>
</example>

---

## Case D — the verification gate firing (a question, not a verdict)

<example>
<input>
// Only file in the diff:
// src/workflows/activities/run-skill-tool.ts
const qualified = `${skillKey}__${toolName}`   // newly added line
// (no other call sites are in this diff)
</input>
<verdict>Question, not a finding. Do not assert a WET verdict from this diff alone.</verdict>
<reasoning>This looks exactly like Pair A's second input — an inlined qualified-tool-name format, a likely #4. But the WET verdict depends on a fact this diff does not contain: whether other call sites construct the same identifier and would drift. Asserting "failed to abstract" here would be a confident verdict on unread evidence — the false-positive the verification gate exists to stop (How to answer, step 5). Correct output: *"Possible identifier-contract WET violation at run-skill-tool.ts: a `${skillKey}__${toolName}` format is inlined. Verify against `@platform/identifiers` ToolName.skill and any other inliners; if this is the only constructor, this is a candidate for routing through ToolName, not a confirmed drift bug."* Emit the question and the call sites to check. Promote to a finding only after reading them.</reasoning>
</example>

---

## Pair E — the thing: same-shaped exports, opposite verdicts (no shared cause vs one shared cause)

<example>
<input>
// src/workflows/retry-policies.ts
export const CONTROL_PLANE_OPTIONS = { /* timeouts + retry */ } as const
export const HARNESS_OPTIONS    = { /* timeouts + retry */ } as const
export const THRESHOLD_OPTIONS  = { /* timeouts + retry */ } as const
// call site: proxyActivities&lt;typeof activities&gt;(THRESHOLD_OPTIONS)
</input>
<verdict>Finding — #2 + #5, a shape bucket with a theme, no shared cause.</verdict>
<reasoning>Grouped because every member is `RetryPolicy`-shaped and "about retries" — a theme, not a cause. Apply the test: name the single event that changes all of them at once. None — `HARNESS_OPTIONS` is pinned by a writeChain gate, `THRESHOLD_OPTIONS` moves on a runtime-boundaries doc, each non-retryable set is its own; unrelated causes, unrelated rates. The name is tribal: at `proxyActivities(THRESHOLD_OPTIONS)` the reader needs an in-file-only taxonomy, and the bundle cannot take an intention-revealing name — the un-nameable-export tell. Human cost: every consumer must carry an unwritten archetype taxonomy to read a one-line proxy call. Smallest fix: move each archetype to the domain whose cause it shares (harness policy → harness domain); keep one owner only if a single cause genuinely governs the whole taxonomy — test, don't assume. The centralization instinct (no inline literals at proxy sites — drift would break consistency) is correct and stays; only the grouping *axis* changes from shape to cause.</reasoning>
</example>

<example>
<input>
// src/identifiers/tool-name.ts
export const ToolName = {
  skill:   (k, t) =&gt; `${k}__${t}`,
  job:     (t)    =&gt; `job__${t}`,
  device:  (t)    =&gt; `device__${t}`
}
</input>
<verdict>NOT a finding. Same surface (a bag of similar-shaped members) — opposite verdict.</verdict>
<reasoning>Looks like the retry bag: several similar members grouped together. The discriminator is the cause test. The single event that changes all members at once: the `__` separator / qualified-name format. Change it and `skill`, `job`, `device` *must* all change together — one cause, one contract. That shared cause is the domain; `ToolName` is its name. A real thing, correctly one owner, correctly named. Leave it. The lesson: similar-shaped members are a finding only when no single cause changes them together; when one does, the bag *is* the domain.</reasoning>
</example>

The lesson the pair teaches: *grouping is legitimate exactly when one cause changes the members together. `retry-policies.ts` has no such cause (theme only) → re-home. `ToolName` has one (the format contract) → it is the domain. Surface looks identical; the cause test is the whole verdict.*

---

## Pair F — manager shape: a correct self-wiring `manager()` vs a caller-assembled domain

This pair is calibrated against a *different* prior than the rest of the file — not the DRY prior but the dependency-injection prior: "a constructor that takes no deps is untestable; a `fromContext` factory is good factoring; sync-vs-async of the factory is a quality signal." All three instincts misfire on a domain manager built to this doctrine. The discriminator is **who assembles the domain** — the manager itself (correct) or the caller (the antipattern). Whether the factory is sync or async is *not* a discriminator: it is determined by whether a dependency's construction is async, and either is canonical (`blueprints/managers.md` "Factories construct their dependencies eagerly; methods are async"). The only factory-timing defect is keeping it sync by deferring an async dependency to lazy construction on first method use.

<example>
<input>
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
</input>
<verdict>NOT a finding. This is the canonical two-part manager shape. Do not flag the no-arg constructor or the sync factory.</verdict>
<reasoning>A reviewer running the DI prior raises two false flags: "`manager()` takes no dependencies — inject `db`/`streaming` so it's testable," and "the factory is sync — modern async wiring means it should be `async` and `await`ed." The first is wrong; the second is the wrong *question* — sync-vs-async is not a defect either way, it is decided by whether a dependency's construction is async, and here the deps construct synchronously so a sync factory is correct. Saying so is the persona working correctly. This is the two-part shape (`managers-and-domain.md`; `blueprints/managers.md` "The two-part shape", "Factories construct their dependencies eagerly; methods are async"; `patterns/managers/managers.lint.md` R3, `managers.packet.md` R9). **Part 1** is the deterministic surface — `MAX_TURNS`, `item`, `transcript` — organized *by sub-domain*, not by theme: `item` and `transcript` each pass the cause test (one event changes all of `item`'s members at once), and `MAX_TURNS` stays flat because there is no sub-thing to nest it under (the no-lone-member floor; don't pre-nest in anticipation). **Part 2** is `manager()`: a self-constructing factory that builds its own dependencies internally and eagerly by reusing process-wide singletons (`Database.shared()`, `Streaming.shared()`) — the caller passes nothing and assembles nothing. It is synchronous here because these deps construct synchronously; had a dependency needed to connect or authenticate first, the factory would be `async` and the caller would `await` it — never kept sync by lazy-on-first-use construction (`managers.packet.md` R9). The returned client's *methods* are async. The split is exact: every Part 1 member needs no I/O; everything that writes rows or emits events lives on the client `manager()` returns (the split rule). The no-arg constructor is not an untestability defect — `createClient(deps)` exists as an explicitly-discouraged unit-test seam, and a manager is never flagged merely for not taking deps as arguments (`managers.lint.md` R3). This is the shape; leave it.</reasoning>
</example>

<example>
<input>
// src/managers/conversations/index.ts
export const Conversations = {
  fromContext: (ctx: RequestContext): ConversationsManager =>
    createClient({ db: ctx.db, streaming: ctx.streaming, logger: ctx.logger })
}
// every call site: Conversations.fromContext(ctx).appendMessage(...)
</input>
<verdict>Finding — #5, manager shape: the caller assembles the domain.</verdict>
<reasoning>Surface-adjacent to the correct example — same `createClient`, same returned client, same domain. The discriminator is *who supplies the dependencies*. Here the public constructor is `fromContext`: a `from*`/`withContext` factory that takes the wired dependencies as a required parameter, so **the caller assembles the domain** and hands it in. That is the named manager-shape antipattern, distinct from the verb-bucket case Pair C catches: `Conversations` legitimately owns the conversation domain — the defect is purely its *shape* (`blueprints/managers.md` "Antipatterns — the caller must never assemble the domain"; `patterns/managers/managers.lint.md` R1 — NEVER `from*`/`withContext`; R3 — NEVER a required parameter as the public constructor; R2 — NEVER a class manager). Human cost: every call site must now know how to construct and thread a `RequestContext`, the singleton-reuse decision is smeared across dozens of callers instead of owned in one file, and the next reader cannot tell from `Conversations.` that there is a single canonical way in. A manager owning a real domain can still be mis-shaped — ownership and shape are independent verdicts. Smallest fix: collapse to one self-constructing `manager()` that reaches the singletons internally (`Database.shared()`, `Streaming.shared()`) — sync, or `async` if a dependency must connect first; delete `fromContext`; if a test needs injected fakes, that is `createClient(deps)` used as the discouraged test-only seam, never the public constructor.</reasoning>
</example>

The lesson the pair teaches, stated for the model: *a domain manager built to this doctrine constructs its own world. A no-arg self-constructing `manager()` is the shape, not an untestability bug — and whether it is sync or async is decided by its dependencies, not a defect either way; a `fromContext`/`from*`/required-param/class constructor is the bug, even when the manager owns a perfectly real domain. Ownership and shape are separate verdicts — run both.*

---

## Pair G — deterministic placement and access: the same domain logic, on the wrong door vs the right one

This pair is calibrated against the prior "put the domain logic wherever it's convenient, give it a descriptive verb name, and reach it through whatever object is in hand." The discriminator is **determinism decides the door**: pure logic lives on and is reached through the root `const`; non-deterministic logic (async, or takes `{ db }`/a service) lives on the `manager()` client. The two surface tells — `async` and a `db`/service parameter — are visible before the body is read.

<example>
<input>
// src/endpoints/summarize.ts
await History.summarizeBrowserSessionsForConversation(
  { db: ctx.db },
  { conversationId: conversation.id, pageItems: items, workspaceId: conversation.workspaceId }
)
</input>
<verdict>Finding — #5, manager shape: a non-deterministic method on the deterministic surface, wearing a flattened verb.</verdict>
<reasoning>Two tells fire before the body is read: the member is `async`, and it takes `{ db }`. `async` signals the result is not a pure function of its inputs; a `db` parameter *is* non-determinism by definition. It is bolted onto the **root `const`** `History` — Part 1, where only pure, no-I/O members live (`managers-and-domain.md` "The split rule is exact"; `blueprints/managers.md` Part-1-iff-no-I/O). Compounding it, `summarizeBrowserSessionsForConversation` is a long flattened verb where a namespaced sub-domain tree belongs. Human cost: the next reader cannot tell from `History.` which members are safe pure calls and which open the database; the split that makes the deterministic surface trustworthy is broken. Smallest fix is not a rename — move it onto the client and namespace it by sub-domain: `(await History.manager()).browserSessions.summarizeForConversation({ conversationId, pageItems, workspaceId })`. The `db` is supplied by the self-constructing factory, not threaded by the caller.</reasoning>
</example>

<example>
<input>
// src/endpoints/summarize.ts  (manager injected as ctx.managers.history)
const endpoint = async (ctx) => {
  const shortHistory = History.converter({ to: 'short-conversation', fullConversationHistory })
  await ctx.managers.history.browserSessions.summarizeForConversation({
    conversationId: conversation.id, pageItems: items, workspaceId: conversation.workspaceId
  })
}
</input>
<verdict>NOT a finding. Deterministic logic through the root `const`, non-deterministic through the injected client. Do not flag reaching past `ctx.managers.history` for the pure call.</verdict>
<reasoning>A reviewer pattern-matching "the manager is right there in `ctx` — use it for everything" flags `History.converter(...)` as inconsistent: "you have `ctx.managers.history`, go through it." That is the prior misfiring. Deterministic logic is reached through the root domain `const` *always* — even when an initialized instance is in hand — because the pure surface needs no manager to exist and has exactly one canonical door (`managers-and-domain.md` "Deterministic logic is reached through the root `const` — always"). The injected `ctx.managers.history` is for the non-deterministic call only, and it is used for exactly that. `History.converter({ to: 'short-conversation', … })` is a legible namespaced tree, not a flattened verb. The same rule holds *inside* a manager method: it reaches its own deterministic logic through `History.toConversation(...)`, never a closed-over re-implementation. Both doors are correct; flagging the split is the over-fire. Leave it.</reasoning>
</example>

The lesson the pair teaches, stated for the model: *determinism decides the door, not convenience and not what object is in scope. `async` or a `{ db }`/service parameter on the bare `const` is a client method that escaped the split — a finding. Pure logic reached through the root `const`, even when an injected manager is in hand or from inside the manager itself, is the shape — not an inconsistency. The fix for the violation is a home and a namespace, never just a shorter name.*

---

## How the persona uses this file

Read this before every review — it is not topic-conditional like the doctrine files. During Pass A (recall) it widens what you notice; during Pass B (precision) it is the calibration that decides promote-vs-drop. When a candidate resembles one of these inputs, name which pair it is closest to and apply that discriminator explicitly in the finding. The negatives (Pair A first, Pair B second, Pair C second, Pair E second, Pair F first, Pair G second) are as load-bearing as the positives: they are the cases your DRY prior — and, for Pair F first and Pair G second, your dependency-injection / use-the-object-in-hand prior — gets wrong, and refusing to flag them is the persona working correctly, not the persona going soft.
