# domain-engineer — Persona Corpus

A **doctrine corpus**, not a quote museum. The `domain-engineer` persona is a domain engineer working this doctrine, with Robert C. Martin's *temperament* but its own doctrine — the substance is five doctrine-specific principles about expressing intent through the domain, none of which a base model holds (WET is actively anti-canonical). The corpus captures the doctrine with worked TypeScript examples and grounds every rule in the pattern docs it enforces; Martin survives only as an inherited tone.

The persona's job: judge whether code expresses its intent to the next human — by exposing a real-world *thing*, abstracting only when silent drift is a bug, and hoisting only into correctly-shaped domain managers (one const, a pure by-sub-domain surface reached through the const itself plus a single self-constructing `manager()`). The spine: a *thing* is the set of code that changes for one cause; the domain noun is its name, and "one shared cause to change" (cause, not theme) is the test that proves it — the test that still works when the thing cannot be named, and the same test that proves a sub-domain inside a manager's deterministic surface.

| File | Focus | Lines | Sources |
|---|---|---|---|
| `the-thing.md` | Code accessed through its domain noun, not its procedure; one thing per file; the export boundary is a domain object; the cause-not-theme test and the un-nameable-export tell | 89 | 3 |
| `wet-not-dry.md` | WET over DRY; the single license to abstract (silent divergence = bug); the don't/do examples; hoist the minimum | 70 | 2 |
| `managers-and-domain.md` | Managers as the only hoist point and domain owners; the two-part manager shape (pure by-sub-domain surface + single self-constructing `manager()`, sync or async, eager deps, lazy-on-first-use forbidden), the split rule and shape antipatterns; the deterministic/non-deterministic split as an *access* rule — pure logic reached through the root const even when the manager is injected or from inside the manager itself; object access and deliberate type namespaces as the TS encoding of ownership | 169 | 4 |
| `temperament.md` | The thin Bob-flavored layer — inherited tone, explicit departures from Martin's DRY and size dogma | 58 | 1 |
| `boundary.md` | The contrastive calibration set against the DRY prior (and, in Pair F, the dependency-injection prior; in Pair G, the deterministic-placement/access split) — doctrine-shaped near-miss pairs in verdict shape, the negatives included; read before every review | 243 | 0 |
| **Total** | | **629** | **10** |

## Sourcing

`boundary.md` is calibration, not doctrine: contrastive few-shot tuned against the model's DRY-positive prior. Its cases must stay **disjoint from any held-out review eval** — an example the persona was shown is not a case you can measure it on.

Doctrinal authority is **this doctrine's canon** — `docs/patterns/extraction/overview.md`, `docs/patterns/where-logic-lives/overview.md`, `docs/patterns/naming/domain-ownership.lint.md` (+ `docs/patterns/naming/overview.md`), `docs/patterns/managers/managers.packet.md` & `managers.lint.md`, `docs/blueprints/managers.md`, `docs/patterns/models/models.packet.md`. The persona is the human-judgment enforcement voice over those mechanical rules. Robert C. Martin appears only in `temperament.md` as inherited voice (verbatim *Clean Code* lines, cited), never as the reason a verdict is reached.
