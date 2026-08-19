---
title: "Concurrency — Goroutine Ownership, Channels, Context, and Cancellation"
summary: >
  Primary-source corpus on asynchronous Go: Pike's concurrency-versus-parallelism
  distinction, Cheney's goroutine lifetime rules, the Go blog's pipeline and cancellation
  patterns, the context contract from Sameer Ajmani's original article, errgroup and
  run-group lifecycle management, and the mutex-versus-channel decision. Structure first,
  primitives second.
source_count: 10
---

# Concurrency in Go

Goroutines are cheap, which is exactly why they are dangerous: the cost of starting one
is so low that engineers start them without deciding who owns them. Nearly every real
concurrency bug in a Go service is an ownership bug — an unowned goroutine, an unowned
channel, or an uncancelled call — not a subtle memory-model bug.

---

## 1. Concurrency is not parallelism

> "Concurrency is about dealing with lots of things at once. Parallelism is about doing
> lots of things at once. Not the same, but related. Concurrency is about structure,
> parallelism is about execution."
> **Concurrency** — "Programming as the composition of independently executing
> processes."
> **Parallelism** — "Programming as the simultaneous execution of (possibly related)
> computations."
> "Concurrency enables parallelism."
> — Rob Pike, *Concurrency is not Parallelism*, https://go.dev/talks/2012/waza.slide

The practical consequence: **concurrency is a decomposition decision, not a performance
technique.** Adding goroutines to code that is not decomposed into independent processes
adds nondeterminism without adding throughput. Before approving concurrent code, name
the independent processes. If you cannot name them, the change is speculative
parallelism and the sequential version is better.

Goroutines are not threads, and the runtime's multiplexing is what makes the
decomposition affordable:

> "A newly minted goroutine is given a few kilobytes, which is almost always enough. The
> CPU overhead averages about three cheap instructions per function call. It is
> practical to create hundreds of thousands of goroutines in the same address space. If
> goroutines were just threads, system resources would run out at a much smaller number."
> — *Go FAQ*, https://go.dev/doc/faq

Cheap does not mean free. Each live goroutine pins its stack and everything its closure
captures, which is why leaks present as memory growth rather than as hangs.

---

## 2. Goroutine ownership is the primary rule

This is the single most load-bearing rule in asynchronous Go, and the one most often
violated in code that otherwise looks clean.

> "Every time you use the `go` keyword in your program to launch a goroutine, you must
> know how, and when, that goroutine will exit."
> "Every time you write the statement `go` in a program, you should consider the
> question of how, and under what conditions, the goroutine you are about to start,
> will end."
> — Dave Cheney, *Never start a goroutine without knowing how it will stop*,
> https://dave.cheney.net/2016/12/22/never-start-a-goroutine-without-knowing-how-it-will-stop

The canonical leak he demonstrates is a receiver with no guaranteed close:

```go
ch := somefunction()
go func() {
    for range ch { }
}()
```

His question — when will `ch` be closed, and by whom? — is the review question. If the
answer is "whenever the producer happens to finish, if it does," the goroutine is a
leak.

**Every goroutine in Landing code must answer three questions in the code itself:**

1. **Who stops it?** A cancelled `context.Context`, a closed channel, or a returned
   `errgroup` — named explicitly.
2. **Who waits for it?** A `sync.WaitGroup`, an `errgroup.Group`, or a run-group. A
   goroutine nobody waits for cannot be shut down cleanly.
3. **Where does its error go?** A goroutine that logs and drops its error has removed a
   failure from the system's control flow.

### Leave concurrency to the caller

> "Leave concurrency to the caller"
> "Never start a goroutine without knowning when it will stop"
> (his section headings; the misspelling is in the original)
> — Dave Cheney, *Practical Go*,
> https://dave.cheney.net/practical-go/presentations/qcon-china.html

A library function that internally spawns goroutines has made a policy decision for
every caller: concurrency level, error aggregation, and cancellation semantics. The Go
convention is to expose a synchronous function and let the caller decide to run it in a
goroutine. `filepath.WalkDir` walks synchronously; the caller parallelizes if it wants
to. When Landing code hides goroutines behind a plain-looking function, say so in the
doc comment — an undocumented internal goroutine is an undocumented lifecycle
obligation.

---

## 3. Channels versus mutexes

> "Do not communicate by sharing memory. Instead, share memory by communicating."
> — *Go FAQ*, https://go.dev/doc/faq

> "Channels orchestrate; mutexes serialize."
> — *Go Proverbs*, https://go-proverbs.github.io/

That proverb is the decision rule, and it is more precise than the more famous slogan.
The FAQ itself scopes the slogan to higher-level coordination:

> "For higher-level operations, such as coordination among concurrent servers,
> higher-level techniques can lead to nicer programs, and Go supports this approach
> through its goroutines and channels."
> — *Go FAQ*, https://go.dev/doc/faq

**Use a channel when** ownership of a value transfers between independent processes,
when you are signalling an event, when you are distributing work, or when the structure
of the problem is a pipeline.

**Use a mutex when** several goroutines read and write one piece of shared state that
does not move — a cache, a registry, a counter, a connection pool. Reaching for a
channel-guarded state machine here produces slower and less obvious code than
`sync.Mutex` embedded next to the field it protects. `sync/atomic` is for single words
under contention, not for building lock-free data structures by hand.

The tell that a channel is being misused as a mutex: a single goroutine looping over a
channel of "operations" against a struct it exclusively owns, with reply channels to
return values. That is a mutex with extra latency and a leak surface.

---

## 4. Context is the cancellation contract

> "A Context carries a deadline, cancellation signal, and request-scoped values across
> API boundaries."
> "Done returns a channel that is closed when this Context is canceled or times out."
> — Sameer Ajmani, *Go Concurrency Patterns: Context*, https://go.dev/blog/context

Cancellation is hierarchical, and derived contexts cannot cancel upward:

> "when a Context is canceled, all Contexts derived from it are also canceled."
> — *Go Concurrency Patterns: Context*, https://go.dev/blog/context

The propagation rule that makes it work across teams:

> "pass a Context parameter as the first argument to every function on the call path
> between incoming and outgoing requests."
> — *Go Concurrency Patterns: Context*, https://go.dev/blog/context

> "`context.Context` is always the first parameter" — with narrow exceptions for HTTP
> handlers, streaming RPC methods, and test functions.
> — *Go Style Decisions*, https://google.github.io/styleguide/go/decisions.html

### The context rules that matter in review

- **`ctx` is the first parameter, named `ctx`, and never stored in a struct** except in
  the rare request-scoped struct that documents why.
- **Never pass a `nil` Context.** Use `context.TODO()` when the plumbing is not done yet
  — it is a signal to the reader, not a placeholder to forget.
- **`defer cancel()` immediately after every `WithCancel`/`WithTimeout`/`WithDeadline`.**
  Skipping it leaks the parent's child list until the parent dies. `go vet`'s `lostcancel`
  check catches the obvious cases and not the subtle ones.
- **A blocking operation that ignores `ctx.Done()` is not cancellable**, regardless of
  how much context plumbing surrounds it. Cancellation only exists at the select
  statements and context-aware library calls that actually observe it.
- **Timeouts belong at the boundary that knows the budget** — the incoming request, the
  CLI invocation, the probe — and propagate inward. Every outbound call inherits it.
- **Deadline before work:** `Deadline` lets a function decide not to start work it
  cannot finish. This is the cheap win most services skip.
- **`context.Value` is for request-scoped data that cannot travel any other way** —
  request IDs, auth tokens, trace spans.

> "only use context.Value for data that can't be passed through your program in any
> other way"
> — Peter Bourgon, *Go for Industrial Programming*,
> https://peter.bourgon.org/go-for-industrial-programming/

Dependencies — loggers, clients, configuration — are parameters and struct fields, not
context values. A `context.Value`-carried dependency is an untyped global with extra
steps.

---

## 5. Pipelines, fan-out, and who closes a channel

> "Multiple functions can read from the same channel until that channel is closed; this
> is called fan-out."
> "a function can read from multiple inputs and proceed until all are closed by
> multiplexing the input channels onto a single channel that's closed when all the
> inputs are closed. This is called fan-in."
> — *Go Concurrency Patterns: Pipelines and cancellation*, https://go.dev/blog/pipelines

The two structural rules:

> "Stages close their outbound channels when all the send operations are done."
> "Stages keep receiving values from inbound channels until those channels are closed"
> — *Go Concurrency Patterns: Pipelines and cancellation*, https://go.dev/blog/pipelines

**The sender closes; the receiver never does.** A channel closed by a receiver, or by
two senders, is a panic waiting for load. When multiple senders share one channel, they
need a `sync.WaitGroup` and one closer goroutine that waits on it.

Cancellation in a pipeline is broadcast by closing a shared channel — today, that is
`ctx.Done()`:

> "main can unblock all the senders simply by closing the done channel. This close is
> effectively a broadcast signal to the senders."
> — *Go Concurrency Patterns: Pipelines and cancellation*, https://go.dev/blog/pipelines

Every send in a pipeline stage must therefore be a `select` over the send and
`ctx.Done()`. A bare `ch <- v` in a cancellable pipeline blocks forever when the
consumer goes away — the exact leak Cheney's rule is designed to catch.

---

## 6. Structured lifecycles: errgroup and run-groups

Raw `go` plus `sync.WaitGroup` loses errors. `golang.org/x/sync/errgroup` is the
default for a bounded set of related concurrent operations because it does three things
at once: waits for all, returns the first error, and cancels the shared context when
any member fails.

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(8) // bound concurrency; unbounded fan-out is a capacity bug
for _, probe := range probes {
    g.Go(func() error { return probe.Run(ctx) })
}
if err := g.Wait(); err != nil {
    return fmt.Errorf("probing providers: %w", err)
}
```

Use `errgroup.WithContext`, not the bare `errgroup.Group`, whenever a failure should
stop the siblings. Use `SetLimit` whenever the item count is driven by input size —
unbounded fan-out over an unbounded list is how a service exhausts file descriptors or
a provider's rate limit.

For long-lived actors — a server, a watcher, a background reconciler — the pattern is a
run-group, where each actor registers both its run function and its interrupt function:

> "Add queues a goroutine to be run, but also tracks a function that will interrupt the
> goroutine when it needs to be killed."
> — Peter Bourgon, *Go for Industrial Programming*,
> https://peter.bourgon.org/go-for-industrial-programming/

The property that matters: it is structurally impossible to register an actor without
saying how it stops. That is Cheney's rule turned into an API.

---

## 7. Correctness mechanics

- **The race detector is not optional.** `go test -race` on any package with concurrent
  code, and in CI. A data race is undefined behavior in Go, not merely a torn read.
- **Happens-before comes from the synchronization primitives**, not from timing. Sleeps
  do not synchronize anything; a test that passes because of a `time.Sleep` is a test
  that will fail in CI.
- **Loop variable capture** was fixed in Go 1.22 for `for` loops — each iteration has
  its own variable. Code targeting older semantics, or capturing in other constructs,
  still needs care. Check `go.mod`'s language version before assuming.
- **`sync.WaitGroup.Add` happens before the `go` statement**, never inside the
  goroutine.
- **Buffered channels are a capacity decision, not a performance knob.** A buffer size
  chosen to "avoid blocking" is hiding backpressure that the system needs to feel.
- **Send on a closed channel panics; close of a closed channel panics.** These are
  ownership violations, so fix the ownership rather than adding recovery.
- **A `nil` channel blocks forever** — occasionally the right tool in a `select` to
  disable a case, and otherwise a bug.

---

## 8. Review checklist for asynchronous Go

1. Name the independent processes. If they cannot be named, remove the concurrency.
2. For every `go` statement: what stops it, who waits for it, where its error goes.
3. Every derived context has `defer cancel()`.
4. Every blocking operation in a cancellable path observes `ctx.Done()`.
5. Every channel has exactly one closer, and it is a sender.
6. Every fan-out over input-sized work has an explicit concurrency bound.
7. Shared mutable state is protected by a mutex adjacent to it, not by convention.
8. Tests for this code run under `-race` and do not depend on sleeps.
