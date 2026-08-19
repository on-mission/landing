---
title: "Testing and Toolchain — Table Tests, Fakes, and the Standard Tooling Contract"
summary: >
  Primary-source corpus on testable Go: the Go team's Test Comments wiki, Google's style
  guide on assertion libraries and failure messages, Dave Cheney on table-driven tests and
  subtests, testable program structure (run(ctx) error, dependency injection), and the
  standard toolchain — gofmt, go vet, -race, benchmarks, fuzzing, and module hygiene.
source_count: 9
---

# Testing and tooling

Go ships one test framework, one formatter, one vet, and one race detector, and the
community expectation is that you use them rather than replace them. Most "hard to test"
Go is a design report, not a testing problem: package-level state, internal goroutines,
`time.Now()` in business logic, and interfaces invented for mocks.

---

## 1. Table-driven tests are the default

> "Table-driven tests should be used whenever many different test cases can be tested
> using similar testing logic."
> "when some test cases need to be checked using different logic from other test cases,
> it is more appropriate to write multiple test functions."
> — *Go Test Comments*, https://go.dev/wiki/TestComments

> "For each test case only the input, the expected output, and name of the test case
> change. Everything else is boilerplate."
> — Dave Cheney, *Prefer table driven tests*,
> https://dave.cheney.net/2019/05/07/prefer-table-driven-tests

The second half of that wiki quote is the part reviewers forget: a table whose body is a
thicket of `if tc.wantErr`, `if tc.skipValidation`, `if tc.useFake` conditionals has
become a small interpreter. Split it into separate test functions.

**The canonical shape:**

```go
func TestSelectPath(t *testing.T) {
    tests := map[string]struct {
        req  Request
        want Path
    }{
        "prefers cheapest eligible provider": {req: ..., want: ...},
        "falls back when primary is saturated": {req: ..., want: ...},
    }

    for name, tc := range tests {
        t.Run(name, func(t *testing.T) {
            got, err := SelectPath(tc.req)
            if err != nil {
                t.Fatalf("SelectPath(%v) returned unexpected error: %v", tc.req, err)
            }
            if diff := cmp.Diff(tc.want, got); diff != "" {
                t.Errorf("SelectPath(%v) mismatch (-want +got):\n%s", tc.req, diff)
            }
        })
    }
}
```

Subtests matter for a specific reason:

> "after the first failing test case we stop testing the other cases."
> — Dave Cheney, *Prefer table driven tests*,
> https://dave.cheney.net/2019/05/07/prefer-table-driven-tests

`t.Run` isolates each case so one failure does not mask the rest, and it makes
`go test -run 'TestSelectPath/falls_back'` work. Google's guidance is to use field names
when initializing test-case struct literals, and to prefer subtests plus `t.Fatal` over
`t.Error` and `continue` inside a table body.

**Case names describe the behavior, not the input.** `"empty input"` is weaker than
`"returns no candidates when every provider is over budget"`. The wiki notes to choose
subtest names that remain useful and readable after escaping — spaces become
underscores, so avoid punctuation that mangles.

---

## 2. Failure messages are the product of a failing test

> "Test outputs should output the actual value that the function returned before printing
> the value that was expected. A usual format for printing test outputs is
> `YourFunc(%v) = %v, want %v`."
> "Failure messages should include the name of the function that failed"
> — *Go Test Comments*, https://go.dev/wiki/TestComments

Go's convention is **got before want**, and it is worth enforcing because a mixed
codebase makes every failure ambiguous. `cmp.Diff` takes `(want, got)` and prints a
diff — label the diff direction in the message, as above.

Cheney's argument for `cmp.Diff` over `reflect.DeepEqual` is that a boolean tells you
*that* two values differ and a diff tells you *why* — down to the field of the element
that changed.

`reflect.DeepEqual` also compares unexported fields and treats nil and empty slices as
different, which produces failures that are correct and useless. Prefer
`github.com/google/go-cmp/cmp` with explicit options.

---

## 3. No assertion libraries

> "Do not create "assertion libraries" as helpers for testing."
> — *Go Style Decisions*, https://google.github.io/styleguide/go/decisions.html

The Go position is that a test should read as ordinary Go: call the function, compare,
report. Assertion DSLs (`assert.Equal(t, ...)`) hide which comparison ran and produce
generic failure text, and they encourage tests that assert on shape rather than
behavior. `cmp` for comparison plus plain `if` for control flow is the standard.

Related mechanics from the same sources:

- `t.Helper()` attributes failures to the caller's line — correct for genuine setup
  helpers, and explicitly not a license to build an assert library.
- `t.Cleanup` over `defer` for teardown that must survive subtests.
- Prefer `t.Error` so a test keeps going; reserve `t.Fatal` for setup failures and for
  the point inside a subtest where continuing is meaningless.
- Never call `t.Fatal`/`t.FailNow` from a goroutine other than the one running the test —
  Google's guide states this outright. Send the failure back over a channel or record it
  and fail on the test goroutine.

---

## 4. What to test, and with what doubles

**Test exported behavior.** Tests in `package foo_test` can only reach the public API,
which keeps tests from ossifying internals. Use the internal `package foo` test only when
an algorithm genuinely needs direct coverage.

**Prefer fakes to mocks.** Google's rule against wrapping RPC clients in manual
interfaces purely for testing (see [design.md](./design.md)) extends to doubles
generally: a mock that asserts a call sequence tests the implementation, and it passes
after a refactor only if the refactor changed nothing. A small in-memory fake that
satisfies the same consumer-defined interface tests behavior and survives refactoring.

**Test the whole program where you can.** Ryer's `run(ctx, args, stdin, stdout, stderr)
error` shape exists so a test can call the entire binary in-process and assert on real
output, replacing a stack of redundant unit tests with one honest one. This is the
highest-leverage testability decision available in a Go CLI or service.

**Determinism is a design property.** Injected clock, injected randomness, no reliance on
map iteration order, no `time.Sleep` for synchronization. A test that sleeps to wait for
a goroutine is a test that fails under CI load.

**Error assertions check identity or type, never strings** — `errors.Is`/`errors.As`.
The wiki is explicit that string comparison on errors is wrong; error text is for humans
and is free to change.

**Testable examples** (`func ExampleFoo`) are compiled and run by `go test` when they
have an `// Output:` comment, so they are documentation that cannot rot. Use them for
package-level entry points.

---

## 5. The toolchain contract

| Tool | Contract |
|---|---|
| `gofmt` / `gofumpt` | Formatting is not a review topic. Not formatted, not merged. |
| `go vet` | Baseline correctness: printf verbs, lost cancels, copied locks, unreachable code. Runs by default under `go test`. |
| `go test -race` | Mandatory for any package with concurrency. A race is undefined behavior. |
| `go test -cover` | A missing-coverage detector, not a quality target. |
| `go test -bench` / `-benchmem` | Required evidence for any performance claim. Use `b.ReportAllocs`, `b.ResetTimer`, and `benchstat` for comparisons. |
| `go test -fuzz` | For parsers, decoders, and anything consuming untrusted bytes. |
| `staticcheck` | The standard extra linter; catches real bugs vet does not. |
| `go mod tidy` | Committed and clean. A dirty `go.mod` is a review blocker. |

**Performance rules that follow from the tooling:**

- No optimization without a benchmark and a profile (`pprof`). "This allocates less" is a
  claim, not a finding.
- Preallocate slices with a known length (`make([]T, 0, n)`) — cheap, measurable, and
  not premature.
- Escape analysis (`go build -gcflags=-m`) explains heap allocation; guesses do not.
- `sync.Pool` is for genuine allocation pressure demonstrated by a profile, not for
  general reuse.

**Module hygiene:** the `go` directive in `go.mod` sets language semantics (loop variable
scoping, for one) — check it before assuming a version's behavior. Keep dependencies few;
Kennedy's portability argument is that standard-library-only packages travel best, and
the "little copying is better than a little dependency" proverb applies most strongly to
small utility dependencies.

---

## 6. Review checklist for tests

1. Does the test assert on behavior a user depends on, or on how the code is written?
2. Table-driven with named subtests, or genuinely different logic split into separate
   functions?
3. Got before want, function name and inputs in the failure message, `cmp.Diff` for
   structures?
4. Fakes over mocks, and no interface introduced solely to enable a mock?
5. Errors compared with `errors.Is`/`errors.As`, never strings?
6. Deterministic — injected clock, no sleeps, no map-order assumptions?
7. Does the package's concurrent code run under `-race` in CI?
8. Is any performance claim backed by a benchmark or profile?
