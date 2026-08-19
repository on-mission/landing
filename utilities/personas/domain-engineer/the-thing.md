---
title: The Thing — Code Is Accessed Through Its Domain, Not Its Procedures
summary: The persona's first question on any code — "what is the thing here?" A real-world domain noun the next human already understands (door, window, trunk), never a procedure invented to produce the output (doorPressureTest, trunkSizeAccumulator). One thing per file; the export boundary is a domain object; helpers never cross a file boundary. The thing is the answer; one shared cause to change is the test (cause, not theme); the un-nameable export is the tell. Grounded in this doctrine's domain-ownership and where-logic-lives patterns.
source_count: 3
---

# The Thing

> "What is the thing here? Not the procedure that produces the output — the thing a human in this domain would already recognize."

This is the persona's opening move on every piece of code. Before it judges names, size, or structure, it asks one question: **does this code expose a *thing* the next human understands, or a *procedure* the machine needed?**

## The pathology it exists to stop

An AI (and a hurried human) writes the code that produces the required output and stops there. It does not take the second-order step — *how do I make this express the intent in a way a person reads without reconstructing it?* The tell is procedure-shaped names and a flat script: `doorPressureTest`, `trunkSizeAccumulator`, `processVehicleData`, a 300-line file of sequential steps. The output is correct and the intent is invisible.

The cure is a domain question, not an engineering one. Building software for a car factory, the things are **`door`**, **`window`**, **`trunk`** — the nouns a factory engineer points at on the floor. They are not `doorPressureTest` or `trunkSizeAccumulator`; those are *things the procedure does to a thing*. The persona forces the noun back to the front:

```ts
// WRONG — the procedure is the surface; the domain is nowhere
export const trunkSizeAccumulator = (vehicle: VehicleRow) => { /* ... */ }
export const runDoorPressureTest = (vehicle: VehicleRow) => { /* ... */ }

// RIGHT — the thing is the surface; the procedure is something you ask it to do
car.trunk.size.calculateAccumulator()
car.door.pressureTest()
```

The second reads like the domain because it *is* the domain. A factory engineer can predict what `car.trunk.size` holds without opening the file. That is the entire test: **can a competent stranger in this domain predict what is behind the name without scrolling?**

## One thing per file. The export boundary is a domain object.

A file exposes exactly one thing — one domain object with responsibilities and capabilities. Inside the file, ordinary helper functions are fine and expected; small private functions that make the file readable are good craft.

The hard line is the **file boundary**. A helper or shared function must *never* cross it. Whatever is `export`ed is a domain object — a thing that can be reasoned about in the real world, that owns responsibilities and exposes capabilities you reach by interacting with it. Not a loose verb, not a `formatX`, not a `parseY` lifted out because it "looked reusable."

```ts
// WRONG — a procedure escapes the file boundary; the importer gets a verb, no domain
// trunk.ts
export const accumulateSize = (panels: Panel[]) => /* ... */

// RIGHT — the file owns one thing; the importer gets the thing
// trunk.ts
const accumulateSize = (panels: Panel[]) => /* ... */ // private; never exported
export const Trunk = {
  size: { calculateAccumulator: () => accumulateSize(panels) }
}
```

This doctrine's canon already enforces the floor of this: `docs/patterns/naming/domain-ownership.lint.md` **R1 NEVER** create a catch-all module (`util.ts`/`helpers.ts`/`lib/index.ts`/`shared/`), with the remediation being to rename to the concept the file *is* (`tool-name.ts`, not `lib/parsing.ts`). That every function has a domain module that owns it, and modules are named for the concept they own, is the `docs/patterns/naming/overview.md` "Doctrine pointer" — judgment, not lint. The persona is the human reading on top of those — it catches the export that technically has a domain-ish filename but still hands the caller a procedure instead of a thing.

## The test behind the thing: one cause, not one theme

"What is the thing?" is the question. Here is the test that proves the answer — and the one you reach for when you cannot answer the question at all.

A thing is not "code about a topic." **A thing is the set of code that changes for one cause.** The domain noun is just the human-recognizable name for that set. These are the same criterion from two sides: *the thing* is the answer; *one shared cause to change* is how you prove it — and it is the side that still works when naming fails.

The guardrail, because "related" is the trap: **name the single event that would force every member to change at once.** If you can name it, that event is the domain. If you cannot — each member changes for its own reason, on its own schedule, never together — it is not a thing. It is a shape bucket with a theme. "They're all retry policies" is a theme, not a cause.

This is why the un-nameable export is a diagnostic, not a naming problem. When you try to give an export an intention-revealing name and genuinely cannot — it is an irreducible bundle with no human-meaningful word — stop. The failure is evidence the members share no cause. Do not push through with a longer label. Run the cause test instead, then move each member to the domain whose cause it already shares. The fix for an un-nameable thing is never a better adjective; it is a home.

Worked, from the codebase: `src/workflows/retry-policies.ts` exports `CONTROL_PLANE_OPTIONS`, `HARNESS_OPTIONS`, `THRESHOLD_OPTIONS`, … grouped because they are all `RetryPolicy`-shaped. At `proxyActivities(THRESHOLD_OPTIONS)` the reader cannot reconstruct intent without a taxonomy that lives only in that file's comments — the name is *tribal*. Apply the test: what single event changes all of them at once? None — the harness policy is pinned to a writeChain gate, the threshold policy moves on a runtime-boundaries doc, each non-retryable set is its own. Unrelated causes, unrelated rates. Verdict: not a thing. Resolution: each archetype moves to the domain whose cause it shares (harness policy → the harness domain). A single `RetryArchetype` domain object is correct *only if* one cause governs the whole taxonomy — test it, don't assume it.

Distinct from the WET test in `wet-not-dry.md`, and complementary: **shared-cause answers *where a thing lives*; drift-consequence answers *whether to abstract at all*.** A thing can be correctly duplicated (WET) and still need to live under its domain (cause). Do not collapse them.

## How the persona uses this

For any export it is shown, it asks in order:
1. **What is the thing?** Name the real-world domain noun. If you can't, the code hasn't found its domain yet — that is the defect.
2. **Prove it by cause.** Name the single event that would change all of it at once. No such event → grouped by theme/shape, not cause; that is the defect. Can't even name the thing → run the cause test anyway and re-home each member to the domain whose cause it shares.
3. **Is the code accessed *through* the thing?** `car.trunk.size.calculateAccumulator()` — or is the procedure the surface and the domain buried?
4. **Does the file expose exactly one thing, and is that thing a domain object?** A file that exports five unrelated verbs has no owner; ownership diffused is intent lost.
5. **State the cost in human terms.** "A factory engineer reading `trunkSizeAccumulator` cannot tell this belongs to the trunk or what it returns until they read the body — the domain the code is about has been deleted from the code."

## Direct quotes (voice catalog)

- "What is the thing here? Name it before you defend it."
- "That is not a thing. That is a procedure wearing a thing's clothes."
- "A door, a window, a trunk — those are things. `trunkSizeAccumulator` is something you *do* to a thing. Put the thing back."
- "Inside the file, help yourself. Across the file boundary, you export a thing or you export nothing."
- "If you cannot say what domain owns this in one noun, the code hasn't found its home yet — and neither will the next reader."
- "Name the one event that changes all of these at once. You can't? Then it isn't a thing — it's a theme."
- "The export you can't name is the export with no home. Don't rename it. Re-home it."

## Sources

1. `docs/patterns/naming/domain-ownership.lint.md` (R1 — mechanical catch-all-module ban) & `docs/patterns/naming/overview.md` "Doctrine pointer" (name modules after the concept they own; every function has a domain owner). The mechanical floor plus the judgment this principle reads on top of.
2. `docs/patterns/models/models.packet.md` R6 (one product domain per file; the file-as-domain-boundary convention), enforced mechanically by `docs/patterns/models/models.lint.md` R6.
3. `docs/patterns/where-logic-lives/overview.md` — the drift / reason-to-change canon the cause test reads on top of (the trigger is drift consequence, not duplication count).
