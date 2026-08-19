---
title: "Errors — Values, Taxonomy, Wrapping, and the API You Commit To"
summary: >
  Primary-source corpus on Go error handling: Rob Pike's "errors are values", Dave Cheney's
  sentinel/type/opaque taxonomy and "handle an error once" rule, the Go 1.13 wrapping
  contract from Jonathan Amsterdam's blog post, Google's error-string and %w placement
  rules, and Go's panic policy. What each choice commits your package to.
source_count: 9
---

# Errors in Go

Go has no exceptions by design (see [philosophy.md](./philosophy.md)), so error handling
*is* the control flow. The verbosity people complain about is not the problem; the
problem is that most Go code checks errors without ever handling them.

---

## 1. Errors are values

> "Errors are values. Values can be programmed, and since errors are values, errors can
> be programmed."
> — Rob Pike, *Errors are values*, https://go.dev/blog/errors-are-values

Pike's essay is a response to the "`if err != nil` everywhere" complaint, and his answer
is not a syntax proposal — it is that error handling is ordinary programming, so ordinary
techniques apply. His `errWriter` example accumulates the first error in a small type so
a run of writes reads as a run of writes:

```go
type errWriter struct {
    w   io.Writer
    err error
}

func (ew *errWriter) write(buf []byte) {
    if ew.err != nil {
        return
    }
    _, ew.err = ew.w.Write(buf)
}
```

> "The key lesson, however, is that errors are values and the full power of the Go
> programming language is available for processing them."
> "Whatever you do, always check your errors!"
> — *Errors are values*, https://go.dev/blog/errors-are-values

The reviewable version of this idea: when a function has five consecutive identical
error checks, the fix is a type or a helper that owns the repetition — not a complaint
about the language, and not a discarded error.

---

## 2. Don't just check errors — handle them

> "Don't just check errors, handle them gracefully."
> — *Go Proverbs*, https://go-proverbs.github.io/

A check is `if err != nil`. Handling is a decision: retry, fall back, annotate and
return, surface to the user, or fail the process. Code that checks and returns unchanged
all the way to `main` produces the classic Go error message with no origin —
`connection refused` with no indication of what was connecting to what.

### Only handle an error once

> "Only handle an error once"
> — Dave Cheney, *Practical Go*,
> https://dave.cheney.net/practical-go/presentations/qcon-china.html

The anti-pattern is log-and-return:

```go
if err != nil {
    log.Printf("probing provider: %v", err)   // handled here...
    return err                                 // ...and again at every level above
}
```

Every layer logs, so one failure produces six log lines and the caller still has to
decide. **Annotate and return, or handle and stop — never both.** Logging is handling;
if you log it, you own it.

> "Code that encounters an error should make a deliberate choice about how to handle it.
> It is not usually appropriate to discard errors using `_` variables."
> — *Go Style Decisions*, https://google.github.io/styleguide/go/decisions.html

Deliberately ignoring an error is legitimate — `defer resp.Body.Close()` on a read-only
response — but it needs a comment saying why, because `_ = err` reads identically whether
it was reasoned about or forgotten.

### Define errors out of existence

> "One of the chapters in that book is called "Define Errors Out of Existence". We're
> going to try to apply this advice to Go."
> — Dave Cheney, *Practical Go*, citing John Ousterhout's *A Philosophy of Software
> Design*, https://dave.cheney.net/practical-go/presentations/qcon-china.html

Cheney's own section heading is *"Eliminate error handling by eliminating errors"*. The
strongest error-handling move is deleting the error path: make the zero value valid
so construction cannot fail, validate once at the boundary so interior code takes
already-valid types, and use `bufio.Scanner`-style types that accumulate an error to be
checked once at the end. An API with fewer error returns is a better API than one with
better-wrapped errors.

---

## 3. The taxonomy: sentinel, type, opaque

Cheney's classification is the most useful vocabulary for reviewing error design,
because each option has a different public-API cost.

### Sentinel errors

> "Sentinel errors become part of your public API"
> "By far the worst problem with sentinel error values is they create a source code
> dependency between two packages."
> — Dave Cheney, *Don't just check errors, handle them gracefully*,
> https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully

`var ErrNotFound = errors.New("not found")` is right when callers genuinely need to
branch on this exact condition and you are willing to support it forever — `io.EOF` and
`sql.ErrNoRows` earn it. Callers compare with `errors.Is`, never `==`, so wrapping still
works.

### Error types

A struct implementing `error` carries structured detail — a field name, a provider, a
retry-after. It costs the same public coupling as a sentinel, plus every field becomes
API. Callers inspect with `errors.As`. Right when the caller needs *data* from the
failure, not just its identity.

### Opaque errors

> "Assert errors for behaviour, not type"
> — Dave Cheney, *Don't just check errors, handle them gracefully*,
> https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully

The default. Return an error that the caller can annotate and propagate but not
introspect. When callers need one behavioral bit, expose the behavior as a method
(`Temporary() bool`, `Timeout() bool`) and let callers assert on a locally-declared
interface — no import of your package, no coupling to your concrete type.

**The decision rule:** opaque by default; behavioral interface when callers must branch
on a *kind* of failure; sentinel when they must branch on one exact condition; typed when
they need structured data. Every step down that list widens your API surface
permanently.

---

## 4. Wrapping: %w and what it commits you to

> "When this verb is present, the error returned by fmt.Errorf will have an Unwrap
> method returning the argument of %w, which must be an error."
> "When operating on wrapped errors, however, these functions consider all the errors in
> a chain."
> — Jonathan Amsterdam, *Working with Errors in Go 1.13*, https://go.dev/blog/go1.13-errors

The rule that most Go codebases get wrong:

> "Wrap an error to expose it to callers. Do not wrap an error when doing so would expose
> implementation details."
> "In other words, wrapping an error makes that error part of your API."
> — *Working with Errors in Go 1.13*, https://go.dev/blog/go1.13-errors

`%w` is not "the better `%v`". It publishes the wrapped error's identity and type to
every caller, forever. If a store wraps a `pq.Error`, callers will start matching on it,
and swapping the database becomes a breaking change. Use `%v` — or a fresh typed error —
when the cause is an implementation detail you do not want to guarantee.

### Mechanics

- `errors.Is(err, ErrX)` for identity; `errors.As(err, &target)` for structure. Never
  `==` on an error that might be wrapped, and never string matching:

> "don't use string comparison to check what type of error your function returns"
> — *Go Test Comments*, https://go.dev/wiki/TestComments

- Annotate with the operation, at the layer that knows it:
  `fmt.Errorf("loading routing policy %q: %w", path, err)`.
- **Do not include the word "failed" or the callee's name** — the chain already reads as
  a path. Each layer adds only what its own scope knows.
- `%w` placement follows Google's rule:

> "Prefer to place `%w` at the end of an error string" in the form `[...]: %w`
> — *Go Best Practices*, https://google.github.io/styleguide/go/best-practices.html

- Error strings themselves:

> "Error strings should not be capitalized (unless beginning with an exported name, a
> proper noun or an acronym) and should not end with punctuation."
> — *Go Style Decisions*, https://google.github.io/styleguide/go/decisions.html

- `errors.Join` (Go 1.20+) for genuinely independent failures — validating every field,
  closing every resource — not as a substitute for wrapping a cause chain.

---

## 5. Panic policy

> "Don't panic."
> — *Go Proverbs*, https://go-proverbs.github.io/

Panic is for programmer error that makes continuing meaningless: an impossible switch
default, a nil dependency at construction, a failed `regexp.MustCompile` on a package
constant. It is never for input validation, missing files, network failures, or provider
errors.

The FAQ's reasoning behind omitting assertions applies directly:

> "Proper error handling means that servers continue to operate instead of crashing
> after a non-fatal error."
> — *Go FAQ*, https://go.dev/doc/faq

**Never let a panic cross a package boundary.** A library that panics on bad input has
made a crash decision for the whole process. Convert at the boundary if you must recover
at all, and recover only where you own the lifecycle — the top of a request handler or a
supervised worker — never as general control flow.

A panic in a goroutine takes down the entire process regardless of what the parent is
doing. Any goroutine running caller-supplied or plugin code needs an explicit recover at
its top, and it must re-report the failure as an error rather than swallowing it.

---

## 6. Review checklist for error handling

1. Is each error handled exactly once — annotated and returned, or handled and stopped?
2. Does every `%w` name a cause you are willing to make permanent API?
3. Does every annotation add scope the caller does not already have?
4. Are comparisons `errors.Is`/`errors.As`, never `==` or string matching?
5. Are sentinels and error types justified by a caller that actually branches on them?
6. Is every ignored error commented with why?
7. Is every panic a genuine programmer error, and does it stay inside its package?
8. Could this error path be designed away — by a useful zero value or boundary
   validation — instead of handled better?
