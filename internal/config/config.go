// Package config owns Landing's configuration boundary.
//
// Everything Landing knows about which harnesses and models may run which kind
// of work enters the program here and nowhere else. Routing, dispatch, and the
// CLI receive one already-validated [Config] and never read a file, an
// environment variable, or a default table of their own.
//
// # Resolution
//
// Landing resolves the nearest applicable project configuration by searching
// upward from the directory it was invoked in for a file named
// [ConfigFileName]. The first one found wins; the search does not continue past
// it, and no configuration outside the project participates. Landing has no
// user-level or machine-level configuration scope.
//
// A project file names its complete tier set. Landing does not infer or merge
// tiers, because a route policy without a project's explicit choice is not a
// policy Landing can execute responsibly.
//
// A file whose "tiers" key is absent and one whose "tiers" object is empty
// are both invalid: each names no tiers. The resolver reports which condition
// it observed so a caller can distinguish an incomplete file from an explicit
// but empty tier set.
//
// A configuration may also name one configured tier as "defaultTier". The
// resolver validates that name after resolving the tier set; omitting it means
// the configuration has no default tier.
//
// When no file resolves, [Load] fails. A project that has not named a policy
// gets an error naming the file and the directories searched — never a route
// chosen on its behalf.
//
// # File format
//
// The file is JSON, so that Landing parses its own configuration with the
// standard library and ships as a single binary with no third-party
// dependencies. It is a complete, directly authorable interface: anything
// guided setup can produce, a person or an agent can write by hand, and setup
// keeps no private representation of its own.
//
//	{
//	  "version": 1,
//	  "defaultTier": "engineer",
//	  "tiers": {
//	    "engineer": {
//	      "description": "Work needing sustained reasoning or design judgment, not mechanical edits you would merge without reading.",
//	      "routes": [
//	        { "harness": "grok", "model": "grok-4.5" },
//	        { "harness": "codex", "model": "gpt-5.6-terra" }
//	      ]
//	    }
//	  }
//	}
//
// A route may omit "model" when its harness exposes only one. A route may carry
// "fallbackBelowPercent", which withholds it until every ordinary route in its
// tier is measured below that much remaining availability — a safety valve
// rather than a lane, kept out of normal rotation.
package config

import (
	"context"
	"slices"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/modelvalidation"
)

// Harnesses is the capability catalog configuration validates named harnesses
// against.
//
// It is deliberately not a routing policy. Which harnesses exist and which
// models each can reach is owned by the harness modules themselves; the
// built-in tiers are one opinion about how to use them. Deriving the catalog
// from that opinion would make a supported harness unconfigurable merely
// because no default tier happened to mention it.
//
// Each harness module owns its hints and validates a concrete model only when
// an authoring operation or dispatch needs that evidence. Hints never refuse a
// model.
type Harnesses interface {
	// IDs are the harness identifiers this build supports.
	IDs() []string

	// ModelCatalog returns model hints for the named harness.
	ModelCatalog(id string) harness.ModelCatalog
}

// AdapterRegistry resolves the adapters linked into this build.
//
// It is the narrow registry behavior configuration needs to form its capability
// catalog. The router's registry satisfies it without configuration depending on
// routing policy.
type AdapterRegistry interface {
	IDs() []string
	Resolve(string) (harness.Adapter, error)
}

type registryHarnesses struct {
	registry AdapterRegistry
}

// NewHarnesses exposes adapter capabilities through the configuration catalog.
func NewHarnesses(registry AdapterRegistry) Harnesses {
	return registryHarnesses{registry: registry}
}

func (harnesses registryHarnesses) IDs() []string {
	return harnesses.registry.IDs()
}

func (harnesses registryHarnesses) ModelCatalog(id string) harness.ModelCatalog {
	adapter, err := harnesses.registry.Resolve(id)
	if err != nil {
		return harness.ModelCatalog{Authority: harness.ModelCatalogAuthoritative}
	}

	return adapter.ModelCatalog()
}

// ValidateConfiguration delegates any harness-specific configuration
// invariants to the adapter that owns that harness. Adapters without the
// optional capability deliberately have no additional validation.
func (harnesses registryHarnesses) ValidateConfiguration(id string, routes []harness.ConfiguredRoute) error {
	adapter, err := harnesses.registry.Resolve(id)
	if err != nil {
		return nil
	}
	validator, ok := adapter.(harness.ConfigValidator)
	if !ok {
		return nil
	}

	return validator.ValidateConfiguration(routes)
}

// ValidateModel checks a concrete model before configuration authoring writes
// it. Loading direct configuration remains shape-only and never spends a probe.
func (harnesses registryHarnesses) ValidateModel(ctx context.Context, id string, model string) harness.ModelValidation {
	adapter, err := harnesses.registry.Resolve(id)
	if err != nil {
		return harness.UnverifiedModel("the configured harness could not be resolved")
	}

	return modelvalidation.Validate(ctx, adapter, model)
}

// ConfigFileName is the project configuration file Landing searches upward for.
const ConfigFileName = ".landing/config.json"

// SchemaVersion is the only "version" value Landing currently accepts. It
// exists so that a future format change can be reported as a version mismatch
// rather than as a parse failure the reader has to decode.
const SchemaVersion = 1

// Origin records which configuration source supplied a tier.
type Origin string

const (
	OriginProject Origin = "project configuration"
)

// Route is one executable path for a tier: a harness, and optionally the model
// to ask it for.
type Route struct {
	// Harness names the adapter that runs the work.
	Harness string

	// Model is the model to request. Nil means the harness exposes a single
	// model and chooses it itself; it does not mean "any model".
	Model *string

	// FallbackBelowPercent, when set, withholds this route until every
	// ordinary route in its tier is measured below this much remaining
	// availability. Nil is an ordinary route.
	//
	// A route that is cold after a confirmed failure counts as below the
	// threshold, because it cannot run at all. Without that, a tier whose
	// every ordinary route had failed would resolve to nothing while holding
	// an eligible safety valve in reserve.
	FallbackBelowPercent *float64
}

// String renders a route the way a caller writes one on --route or --model, so
// configuration, diagnostics, and a recorded job all name a route identically.
func (route Route) String() string {
	if route.Model == nil || *route.Model == "" {
		return route.Harness
	}

	return route.Harness + "/" + *route.Model
}

// Tier is a named class of work and the policy for executing it.
//
// A tier addresses two readers. The router consumes Routes. An agent deciding
// what to delegate consumes Description, which is why it is prose a person
// writes rather than a flag a program derives.
type Tier struct {
	// Name is the tier's stable identifier, named by a caller through --tier.
	// It shares no namespace with Landing's verbs, so a tier may be called
	// anything a project finds meaningful.
	Name string

	// Description says, in the author's own words, what kind of work this tier is
	// for — including what it is not for, when that is worth saying.
	Description string

	// Routes are the eligible execution paths, in declared preference order.
	// Preference breaks ties; it does not override measured availability.
	Routes []Route

	// Origin is the precedence layer this tier came from.
	Origin Origin
}

// Source records where a resolved configuration came from, including the search
// that found it. Landing reports the search itself, not just its result,
// because a caller looking at an unexpected policy needs to know which
// directories were considered.
type Source struct {
	// Path is the absolute path of the resolved configuration file.
	Path string

	// SearchedFrom is the invocation directory the upward search began at.
	SearchedFrom string

	// SearchedTo is the last directory the search examined.
	SearchedTo string
}

// Config is one validated view of Landing's policy. Every reference in it has
// already been checked, so consumers may use it without revalidating.
type Config struct {
	// Version is the schema version the project file declared.
	Version int

	// DefaultTier is the configured tier for work whose caller does not name one.
	// An empty value means the configuration has no default tier.
	DefaultTier string `json:"defaultTier"`

	// Tiers are the resolved tiers, keyed by name.
	Tiers map[string]Tier

	// Latest overrides Landing's shipped latest model for the harnesses this
	// project names. Harnesses absent from this map retain their shipped value.
	Latest map[string]string

	// Source records the file this configuration resolved from.
	Source Source
}

// Tier returns the named tier.
func (config Config) Tier(name string) (Tier, bool) {
	tier, ok := config.Tiers[name]

	return tier, ok
}

// TierNames returns every tier name in lexical order. Callers render these in
// help text and error messages, so the order must not vary between runs.
func (config Config) TierNames() []string {
	names := make([]string, 0, len(config.Tiers))
	for name := range config.Tiers {
		names = append(names, name)
	}
	slices.Sort(names)

	return names
}

// Load resolves the project configuration applicable to invocationDir.
//
// It searches upward for [ConfigFileName], parses it, validates the result,
// and returns one view every consumer can trust. It fails when no file
// resolves, when the file cannot be parsed, when it names no tiers, or when a
// tier references a harness Landing does not support or has no route.
//
// harnesses is the adapter capability catalog available in this build. Routes
// are validated against it so unsupported harnesses and models are reported at
// the configuration boundary rather than discovered at dispatch.
func Load(ctx context.Context, invocationDir string, harnesses Harnesses) (Config, error) {
	return resolve(ctx, invocationDir, harnesses)
}
