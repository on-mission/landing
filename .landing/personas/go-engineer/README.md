---
title: Go Expert Corpus
summary: Source corpus for the `/go-engineer` command. A reasoning-first reference on Go design intent, concurrency and goroutine ownership, error handling, package and interface design, and testing and toolchain posture — grounded in primary sources from Rob Pike and the Go team, Dave Cheney, Bill Kennedy, Google's Go Style Guide, Mat Ryer, and Peter Bourgon. Design trade-offs and their API cost, not formatting rules.
---

# Go Expert Corpus

Reference material for the `/go-engineer` Claude command
(`.claude/commands/go-engineer.md`). The command consults these files before answering
non-trivial Go questions. The corpus deliberately excludes formatting and layout rules —
`gofmt` owns those, and Landing's local rules live in `docs/patterns/go/`. What lives
here is the reasoning that makes Go code reviewable: what each design choice commits a
package to, and when a Go idiom stops paying for itself.

| File | Focus | Lines |
|---|---|---:|
| [philosophy.md](./philosophy.md) | Go Proverbs, less-is-more, composition over hierarchy, the deliberate omissions (exceptions, assertions, late generics), readability doctrine, clear-over-clever failure modes | 234 |
| [concurrency.md](./concurrency.md) | Concurrency vs parallelism, goroutine ownership and leaks, channels vs mutexes, the context contract, pipelines and channel-closing rules, errgroup and run-groups, race correctness | 288 |
| [errors.md](./errors.md) | Errors as values, handle-once, sentinel/type/opaque taxonomy, `%w` as an API commitment, `errors.Is`/`As`, error string style, panic policy | 236 |
| [design.md](./design.md) | Package naming and dependency direction, consumer-defined interfaces, accept-interfaces-return-structs, useful zero values, construction ladder and functional options, no package-level state, service shape, generics guidance | 269 |
| [testing-and-tooling.md](./testing-and-tooling.md) | Table-driven tests and subtests, failure-message convention, no assertion libraries, fakes over mocks, end-to-end `run(ctx)` testing, gofmt/vet/-race/bench/fuzz, module hygiene | 199 |

**Total:** ~1,226 lines across the five topic files.

## Quote verification

All **64** verbatim quotes were checked mechanically against the raw HTML of the cited
source: each quoted string is present, character-for-character after whitespace and
HTML-entity normalization, in the document its adjacent citation names. Quotes taken from
section headings are reproduced without added terminal punctuation, and one original
misspelling (*"knowning"*, in Cheney's *Practical Go*) is preserved and flagged in place.

Claims that are synthesis rather than quotation are stated in the corpus's own voice and
never attributed to a source. When updating these files, re-run the check rather than
trusting a summarizing fetch layer — paraphrase presented as quotation is the failure
mode this corpus is built to avoid.

## Source tier

1. **Rob Pike and the Go team** — the Go Proverbs, *Less is exponentially more*, *Go at
   Google: Language Design in the Service of Software Engineering*, *Concurrency is not
   Parallelism*, *Errors are values*, the Go FAQ, and the Go blog's package-names,
   pipelines, context (Sameer Ajmani), Go 1.13 errors (Jonathan Amsterdam), and
   when-generics (Ian Lance Taylor) articles.
2. **Dave Cheney** — *Practical Go*, *SOLID Go Design*, *Don't just check errors, handle
   them gracefully*, *Never start a goroutine without knowing how it will stop*, *Prefer
   table driven tests*. The most complete practitioner doctrine in the ecosystem.
3. **Bill Kennedy (Ardan Labs)** — package-oriented design: purpose, usability,
   portability, and dependency direction.
4. **Google Go Style Guide** — `decisions.html` and `best-practices.html`; the largest
   published body of Go review rules with stated rationale.
5. **Mat Ryer** — *How I write HTTP services in Go after 13 years*; the widely adopted
   testable service shape.
6. **Peter Bourgon** — *Go for Industrial Programming*; explicit dependencies, no package
   state, actor lifecycles.

## Reading order

For a design question, read `design.md` then `philosophy.md`. For anything asynchronous,
`concurrency.md` first — goroutine ownership is the rule most often violated. For failure
modeling, `errors.md`. For testability complaints, read `testing-and-tooling.md`, then
check whether the real defect is in `design.md`'s territory.

## Boundary with Landing's pattern catalog

This corpus is external doctrine. `docs/patterns/go/` is Landing's local authority and
wins on conflict. When corpus guidance and a Landing pattern disagree, surface the gap to
`pattern-author` rather than quietly following either one.
