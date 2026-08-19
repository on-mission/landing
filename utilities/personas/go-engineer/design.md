---
title: "Package, Interface, and API Design — What a Package Provides"
summary: >
  Primary-source corpus on structuring Go: package-oriented design from Bill Kennedy,
  Cheney's SOLID Go and Practical Go rules, the Go blog on package names, consumer-defined
  interfaces and "accept interfaces, return structs", useful zero values and construction
  patterns, explicit dependencies with no package-level state, generics guidance from the
  Go team, and service structure from Mat Ryer and Peter Bourgon.
source_count: 12
---

# Package and API design

Go has no visibility keywords beyond capitalization, no namespaces beyond packages, and
no inheritance. The package is the only unit of encapsulation you get, so package
boundaries carry the weight that class hierarchies carry elsewhere. Getting them wrong
is the most expensive mistake available in a Go codebase.

---

## 1. Packages provide, they do not contain

> "Name your package for what it provides, not what it contains."
> — Dave Cheney, *Practical Go*,
> https://dave.cheney.net/practical-go/presentations/qcon-china.html

> "Packages must be named with the intent to describe what it provides."
> "Packages must not become a dumping ground of disparate concerns."
> — Bill Kennedy, *Design Philosophy On Packaging*,
> https://www.ardanlabs.com/blog/2017/02/design-philosophy-on-packaging.html

The Go blog gives the mechanics:

> "Packages named `util`, `common`, or `misc` provide clients with no sense of what the
> package contains."
> "Since client code uses the package name as a prefix when referring to the package
> contents, the names for those contents need not repeat the package name."
> "The name of the package is a critical piece of its design."
> — *Package names*, https://go.dev/blog/package-names

Cheney's SOLID essay connects the naming rule to change frequency:

> "A package's name is both a description of its purpose, and a name space prefix."
> "Catch all packages like these become a dumping ground for miscellany, and because
> they have many responsibilities they change frequently and without cause."
> — Dave Cheney, *SOLID Go Design*, https://dave.cheney.net/2016/08/20/solid-go-design

**Concrete rules:**

- Package names are short, lower case, no underscores, no mixedCaps — usually a noun.
- No `util`, `common`, `helpers`, `base`, `misc`, `shared`, `types`, `models`. A
  `models` package that every other package imports is a distributed god object.
- No stutter: `http.Server`, not `http.HTTPServer`; `routing.Policy`, not
  `routing.RoutingPolicy`.
- Prefer fewer, larger packages. Cheney: *"Prefer fewer, larger packages."* Splitting a
  package because a file got long is a file-level problem with a file-level fix.
- `package main` stays tiny — Cheney notes it cannot be tested, so anything worth
  testing belongs elsewhere.

### Dependency direction

> "Two packages can't cross-import each other. Imports are a one way street."
> "A package that only depends on the standard library, has the highest level of
> portability in Go."
> — Bill Kennedy, *Design Philosophy On Packaging*,
> https://www.ardanlabs.com/blog/2017/02/design-philosophy-on-packaging.html

> "the import graph of a well designed Go program should be a wide, and relatively flat,
> rather than tall and narrow."
> — Dave Cheney, *SOLID Go Design*, https://dave.cheney.net/2016/08/20/solid-go-design

Go's import cycle error is a design tool, not an obstacle. A cycle means the two
packages share one concern and should be merged, or that a third package owns the type
they both need. The usual bad fix — a `types` package holding everything to break the
cycle — converts an honest cycle into a hidden one.

---

## 2. Interfaces belong to consumers

> "The bigger the interface, the weaker the abstraction."
> — *Go Proverbs*, https://go-proverbs.github.io/

> "A great rule of thumb for Go is **accept interfaces, return structs**." — Jack
> Lindamood, quoted in Dave Cheney, *SOLID Go Design*,
> https://dave.cheney.net/2016/08/20/solid-go-design

> "The consumer of the interface should define it (not the package implementing the
> interface), ensuring it includes only the methods they actually use."
> "Do not wrap RPC clients in new manual interfaces just for the sake of abstraction or
> testing."
> — *Go Style Decisions*, https://google.github.io/styleguide/go/decisions.html

This inverts the habit most engineers bring from Java or TypeScript. In Go the
implementing package exports a concrete type; the consuming package declares the
one- or two-method interface describing what it needs. Nobody declares the relationship.

**Consequences worth enforcing:**

- **Do not define an interface until there is a consumer.** A one-implementation
  interface written "for testing" or "for flexibility" is speculative coupling. If the
  only implementations are the real one and a mock, the interface is a mocking artifact.
- **Return concrete types.** Returning an interface hides fields and methods callers may
  legitimately need, and it makes future additions to your type invisible. `error` is
  the standard exception.
- **Small interfaces compose.** `io.Reader` and `io.Writer` survive because they name one
  behavior. A `Store` interface with twelve methods is a concrete type spelled with the
  `interface` keyword.
- Kennedy's constraint: *"Packages must prevent the need for type assertions to the
  concrete."* Needing to assert back to the concrete type means the interface was cut in
  the wrong place.
- **`any` says nothing.** An `any` parameter transfers the contract from the compiler to
  a comment. Name the behavior, or use a type parameter (§5).

---

## 3. Zero values, construction, and options

> "Make the zero value useful."
> — *Go Proverbs*, https://go-proverbs.github.io/

`sync.Mutex`, `bytes.Buffer`, and `http.Server` are all usable as declared. A type that
needs `New` before it is safe imposes that on every caller, every embedder, and every
test forever. Design for `var s Something` working first; add a constructor when
construction genuinely has to fail or has to acquire something.

**Construction ladder** — take the first rung that works:

1. **Useful zero value.** No constructor at all.
2. **Plain `New(...)` with required dependencies as parameters.** Explicit, ordinary,
   testable. `NewServer(logger, store, clock)`.
3. **Options struct.** Google's guidance: use one when callers need to specify several
   configuration parameters; field names self-document and the struct can grow without
   breaking call sites.
4. **Variadic functional options.** Google: prefer when *"most callers will not need to
   specify any options"* and there are many optional parameters; options should *"accept
   parameters rather than using presence to signal their value."*
   — https://google.github.io/styleguide/go/best-practices.html

Functional options are the most over-applied pattern in Go. Three required dependencies
passed as `WithLogger`/`WithStore`/`WithClock` turn compile-time requirements into
runtime nil checks. Required is a parameter; optional is an option.

Cheney's API rules for the signature itself:

> "APIs should be easy to use and hard to misuse."
> "Let functions define the behaviour they requires"
> — Dave Cheney, *Practical Go*,
> https://dave.cheney.net/practical-go/presentations/qcon-china.html

Adjacent parameters of the same type (`func Copy(src, dst string)`) are a misuse
generator; distinct named types or a struct fix them.

---

## 4. Explicit dependencies and no package-level state

> "Explicit dependencies"
> "No package level variables"
> "No func init"
> "Package-global objects can encode state and/or behavior that is hidden from external
> callers."
> — Peter Bourgon, *Go for Industrial Programming*,
> https://peter.bourgon.org/go-for-industrial-programming/

Package-level state is Go's most seductive mistake: a package-level `var db *sql.DB` or
a `func init()` that reads the environment looks like less code, and it makes the
package untestable in parallel, order-dependent at startup, and impossible to
instantiate twice. Bourgon's framing — *"on code that outlives any single engineer"* —
is the right lens.

**Rules:**

- Dependencies arrive as parameters or struct fields; never as package globals.
- `func init()` is for registering encodings and compiling constant regexps, not for
  configuration, connections, or logging setup.
- The `main` package composes: read config, construct dependencies, wire, run. Everything
  else takes what it needs.
- Time is a dependency. A package that calls `time.Now()` deep inside its logic cannot be
  tested deterministically; inject a clock at the boundary where determinism matters.
- Randomness, environment, and filesystem roots are dependencies for the same reason.

### Service shape

Mat Ryer's structure is the widely-adopted concrete expression of this, and it is worth
following in Landing's binaries:

- `NewServer` takes all dependencies as arguments and returns an `http.Handler`, with
  routing visible in one place.
- `main` is trivial and delegates to `run(ctx, args, stdin, stdout, stderr) error`, so
  the whole program is callable from a test.
- Handlers are closures returning `http.Handler`, giving each handler a private
  initialization scope.
- Request and response types are declared inside the handler that uses them.
- The context propagates everywhere so shutdown is graceful.
  — Mat Ryer, *How I write HTTP services in Go after 13 years*,
  https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/

The `run(ctx, args, ...) error` shape generalizes past HTTP: it is how a CLI becomes
end-to-end testable without spawning a process.

---

## 5. Types, structs, and generics

**Model with concrete types first.** Named types over bare primitives at boundaries where
confusion is possible — `type ProviderID string` prevents passing a model name where a
provider was meant, at zero runtime cost.

**Struct field ordering** is documentation: identity first, configuration next,
internal state last. Group mutex adjacent to the fields it protects, with a comment
naming what it guards.

**Embedding is for behavior promotion, not code reuse.** Embedding a struct to inherit
its methods reproduces the taxonomy Pike rejected. Embed an interface when you genuinely
want to expose and override it; otherwise use a named field and delegate explicitly.

**Generics.** Ian Lance Taylor's guidance is narrow on purpose:

> "If all you need to do with a value of some type is call a method on that value, use an
> interface type, not a type parameter."
> "If the implementation is different for each type, then use an interface type and write
> different method implementations, don't use a type parameter."
> "If you find yourself writing the exact same code multiple times, where the only
> difference between the copies is that the code uses different types, consider whether
> you can use a type parameter."
> — *When To Use Generics*, https://go.dev/blog/when-generics

The heuristic: type parameters are for code that is *identical* across types — general
containers, slice/map utilities, channel plumbing. They are not a replacement for
interfaces, and a generic function with one instantiation should be the concrete
function.

**`any` in a data structure** is a design smell that generics now usually fix. `any` at a
serialization boundary is sometimes unavoidable — constrain it to that boundary and
convert immediately.

---

## 6. Documentation and compatibility

> "Documentation is for users."
> "Design the architecture, name the components, document the details."
> — *Go Proverbs*, https://go-proverbs.github.io/

Doc comments are full sentences starting with the name being described. The package doc
says what the package provides and how to start using it. Cheney: *"Always document
public symbols"*, and *"Comments on variables and constants should describe their
contents not their purpose."*

Every exported identifier is a compatibility promise under Go's module rules. Exporting
is the decision to support something; the default is unexported, and widening later is
free while narrowing is not.

---

## 7. Review checklist for design

1. Does the package name describe what it provides, and is it free of `util`-class names?
2. Is the import graph wide and flat, with no cycle papered over by a `types` package?
3. Is every interface declared by its consumer, minimal, and backed by a real second
   implementation or a genuine seam?
4. Do functions return concrete types and accept interfaces?
5. Is the zero value usable, or is the constructor requirement justified?
6. Are required dependencies parameters and optional ones options — not the reverse?
7. Is there any package-level state or configuration-reading `init()`?
8. Is every type parameter carrying identical code across types, rather than replacing an
   interface?
9. Is every exported identifier something the package intends to support?
