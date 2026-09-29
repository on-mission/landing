// Package modeltarget resolves caller-written model targets into concrete routes.
package modeltarget

import (
	"fmt"
	"slices"
	"strings"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
)

type Registry interface {
	IDs() []string
	Resolve(string) (harness.Adapter, error)
}

type LatestSource string

const (
	LatestLandingDefault  LatestSource = "landing default"
	LatestProjectOverride LatestSource = "project override"
)

// Resolution keeps both the concrete route and the target expression that
// produced it. LatestSource is empty unless Written selected latest.
type Resolution struct {
	Route        config.Route
	Written      string
	LatestSource LatestSource
}

type Resolver struct {
	configuration config.Config
	registry      Registry
}

func New(configuration config.Config, registry Registry) Resolver {
	return Resolver{configuration: configuration, registry: registry}
}

// Resolve turns one target or a comma-separated target list into concrete,
// deduplicated routes. It never performs model validation or other I/O.
func (resolver Resolver) Resolve(target string) ([]Resolution, error) {
	if target == "" {
		return nil, fmt.Errorf("model target is empty")
	}
	parts := strings.Split(target, ",")
	resolved := make([]Resolution, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("model target %q has an empty list item", target)
		}
		routes, err := resolver.resolveOne(part)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, routes...)
	}

	return deduplicate(resolved), nil
}

// ResolveTier produces the tier an execution router consumes: every latest
// route in that tier becomes the concrete model selected by current policy.
// Other configured tiers remain unread and unprobed.
func (resolver Resolver) ResolveTier(tier config.Tier) (config.Tier, []Resolution, error) {
	routes := make([]config.Route, 0, len(tier.Routes))
	latest := make([]Resolution, 0)
	for _, route := range tier.Routes {
		if route.Model == nil || *route.Model != "latest" {
			routes = append(routes, route)
			continue
		}
		concrete, source, err := resolver.latest(route.Harness)
		if err != nil {
			return config.Tier{}, nil, fmt.Errorf("resolve tier %q route %s/latest: %w", tier.Name, route.Harness, err)
		}
		route.Model = stringPointer(concrete)
		routes = append(routes, route)
		latest = append(latest, Resolution{Route: route, Written: "latest", LatestSource: source})
	}
	tier.Routes = routes

	return tier, latest, nil
}

func (resolver Resolver) resolveOne(target string) ([]Resolution, error) {
	if harnessID, model, qualified := strings.Cut(target, "/"); qualified {
		if harnessID == "" || model == "" {
			return nil, fmt.Errorf("model target %q must use harness/model", target)
		}
		if _, err := resolver.registry.Resolve(harnessID); err != nil {
			return nil, fmt.Errorf("model target %q names unsupported harness %q; supported harnesses are %s", target, harnessID, strings.Join(resolver.harnessIDs(), ", "))
		}
		if model == "latest" {
			concrete, source, err := resolver.latest(harnessID)
			if err != nil {
				return nil, err
			}
			return []Resolution{{Route: config.Route{Harness: harnessID, Model: stringPointer(concrete)}, Written: target, LatestSource: source}}, nil
		}

		return []Resolution{{Route: config.Route{Harness: harnessID, Model: stringPointer(model)}, Written: target}}, nil
	}
	if target == "latest" {
		return resolver.allLatest(target)
	}

	tier, isTier := resolver.configuration.Tier(target)
	modelRoutes := resolver.modelRoutes(target)
	if isTier && len(modelRoutes) > 0 {
		return nil, fmt.Errorf("model target %q is ambiguous: it names tier %q and model %q", target, target, target)
	}
	if isTier {
		return resolver.tierRoutes(tier, target)
	}
	if len(modelRoutes) == 1 {
		return []Resolution{{Route: modelRoutes[0], Written: target}}, nil
	}
	if len(modelRoutes) > 1 {
		return nil, fmt.Errorf("model target %q is ambiguous: model hints claim it for %s", target, routeNames(modelRoutes))
	}
	if resolver.isHarness(target) {
		return []Resolution{{Route: config.Route{Harness: target}, Written: target}}, nil
	}

	return nil, fmt.Errorf("unknown bare model %q; name it as harness/%s", target, target)
}

func (resolver Resolver) tierRoutes(tier config.Tier, written string) ([]Resolution, error) {
	resolved := make([]Resolution, 0, len(tier.Routes))
	for _, route := range tier.Routes {
		if route.FallbackBelowPercent != nil {
			continue
		}
		if route.Model == nil || *route.Model != "latest" {
			resolved = append(resolved, Resolution{Route: route, Written: written})
			continue
		}
		model, source, err := resolver.latest(route.Harness)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, Resolution{Route: config.Route{Harness: route.Harness, Model: stringPointer(model)}, Written: written, LatestSource: source})
	}

	return resolved, nil
}

func (resolver Resolver) allLatest(written string) ([]Resolution, error) {
	resolved := make([]Resolution, 0, len(resolver.registry.IDs()))
	for _, harnessID := range resolver.harnessIDs() {
		model, source, err := resolver.latest(harnessID)
		if err != nil {
			continue
		}
		resolved = append(resolved, Resolution{Route: config.Route{Harness: harnessID, Model: stringPointer(model)}, Written: written, LatestSource: source})
	}

	return resolved, nil
}

func (resolver Resolver) latest(harnessID string) (string, LatestSource, error) {
	if model, ok := resolver.configuration.Latest[harnessID]; ok {
		return model, LatestProjectOverride, nil
	}
	adapter, err := resolver.registry.Resolve(harnessID)
	if err != nil {
		return "", "", fmt.Errorf("harness %q is unsupported", harnessID)
	}
	if model := adapter.ModelCatalog().Latest; model != "" {
		return model, LatestLandingDefault, nil
	}

	return "", "", fmt.Errorf("harness %q has no latest model", harnessID)
}

func (resolver Resolver) modelRoutes(model string) []config.Route {
	routes := make([]config.Route, 0)
	for _, harnessID := range resolver.harnessIDs() {
		adapter, err := resolver.registry.Resolve(harnessID)
		if err != nil {
			continue
		}
		catalog := adapter.ModelCatalog()
		if slices.Contains(catalog.Models, model) {
			routes = append(routes, config.Route{Harness: harnessID, Model: stringPointer(model)})
			continue
		}
		if canonical, ok := catalog.Aliases[model]; ok {
			routes = append(routes, config.Route{Harness: harnessID, Model: stringPointer(canonical)})
		}
	}

	return routes
}

func (resolver Resolver) harnessIDs() []string {
	ids := append([]string(nil), resolver.registry.IDs()...)
	slices.Sort(ids)

	return ids
}

func (resolver Resolver) isHarness(value string) bool {
	_, err := resolver.registry.Resolve(value)

	return err == nil
}

func deduplicate(routes []Resolution) []Resolution {
	seen := make(map[string]struct{}, len(routes))
	unique := make([]Resolution, 0, len(routes))
	for _, route := range routes {
		key := route.Route.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, route)
	}

	return unique
}

func routeNames(routes []config.Route) string {
	names := make([]string, 0, len(routes))
	for _, route := range routes {
		names = append(names, route.String())
	}

	return strings.Join(names, ", ")
}

func stringPointer(value string) *string {
	return &value
}
