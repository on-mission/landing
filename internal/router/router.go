// Package router selects an eligible harness route from observed capacity.
package router

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
)

const (
	defaultColdDuration = 15 * time.Minute
	defaultProbeTimeout = 15 * time.Second
)

type ColdMark struct {
	ExpiresAt time.Time
}

type Route struct {
	Provider      string
	Model         *string
	Score         *float64
	RoutedBecause string
}

type ResolvedRoute struct {
	Route
	Capacities map[string]harness.Capacity
}

// Registry keeps concrete adapters at the CLI boundary instead of in package
// state, so routing can be constructed before every adapter is linked in.
type Registry interface {
	Resolve(id string) (harness.Adapter, error)
	IDs() []string
}

type MapRegistry struct {
	adapters map[string]harness.Adapter
}

func NewMapRegistry(adapters map[string]harness.Adapter) *MapRegistry {
	copyOfAdapters := make(map[string]harness.Adapter, len(adapters))
	for id, adapter := range adapters {
		copyOfAdapters[id] = adapter
	}

	return &MapRegistry{adapters: copyOfAdapters}
}

func (registry *MapRegistry) Resolve(id string) (harness.Adapter, error) {
	adapter, ok := registry.adapters[id]
	if !ok {
		return nil, fmt.Errorf("unknown adapter %q", id)
	}

	return adapter, nil
}

func (registry *MapRegistry) IDs() []string {
	ids := make([]string, 0, len(registry.adapters))
	for id := range registry.adapters {
		ids = append(ids, id)
	}

	return ids
}

type Router struct {
	registry     Registry
	probeTimeout time.Duration

	mu        sync.Mutex
	coldMarks map[string]ColdMark
}

func New(registry Registry) *Router {
	return &Router{
		registry:     registry,
		probeTimeout: defaultProbeTimeout,
		coldMarks:    make(map[string]ColdMark),
	}
}

func (router *Router) Adapter(id string) (harness.Adapter, error) {
	return router.registry.Resolve(id)
}

func (router *Router) Resolve(ctx context.Context, tier config.Tier) (ResolvedRoute, error) {
	if err := ctx.Err(); err != nil {
		return ResolvedRoute{}, err
	}

	providers := providersFor(tier)
	capacities := make(map[string]harness.Capacity, len(providers))
	probeContext, cancel := context.WithTimeout(ctx, router.probeTimeout)
	defer cancel()

	var capacitiesMu sync.Mutex
	var probes sync.WaitGroup
	for _, provider := range providers {
		probes.Add(1)
		go func(provider string) {
			defer probes.Done()
			adapter, err := router.registry.Resolve(provider)
			capacity := harness.UnknownCapacity()
			if err == nil {
				capacity = adapter.ProbeCapacity(probeContext)
			}
			capacitiesMu.Lock()
			capacities[provider] = capacity
			capacitiesMu.Unlock()
		}(provider)
	}

	probesDone := make(chan struct{})
	go func() {
		probes.Wait()
		close(probesDone)
	}()

	select {
	case <-probesDone:
	case <-probeContext.Done():
		if err := ctx.Err(); err != nil {
			return ResolvedRoute{}, err
		}
	}

	capacitiesMu.Lock()
	capacitySnapshot := cloneCapacities(capacities)
	capacitiesMu.Unlock()

	router.mu.Lock()
	coldMarks := cloneColdMarks(router.coldMarks)
	router.mu.Unlock()

	route := ResolveRoute(tier, coldMarks, capacitySnapshot, time.Now())
	return ResolvedRoute{Route: route, Capacities: capacitySnapshot}, nil
}

func (router *Router) MarkProviderCold(provider string, capacity harness.Capacity, now time.Time) time.Time {
	expiresAt := resetFor(capacity, now)
	router.mu.Lock()
	router.coldMarks[provider] = ColdMark{ExpiresAt: expiresAt}
	router.mu.Unlock()

	return expiresAt
}

func providersFor(tier config.Tier) []string {
	providers := make([]string, 0, len(tier.Routes))
	seen := make(map[string]struct{}, len(tier.Routes))
	for _, route := range tier.Routes {
		if _, ok := seen[route.Harness]; ok {
			continue
		}
		seen[route.Harness] = struct{}{}
		providers = append(providers, route.Harness)
	}

	return providers
}

func AvailabilityFor(provider string, model *string, capacity harness.Capacity) *float64 {
	if !capacity.IsKnown() {
		return nil
	}

	buckets := relevantBucketsFor(provider, model, capacity.Buckets())
	score := math.Inf(1)
	found := false
	for _, bucket := range buckets {
		if math.IsNaN(bucket.UsedPercent) || math.IsInf(bucket.UsedPercent, 0) {
			continue
		}
		availability := 100 - bucket.UsedPercent
		if availability < score {
			score = availability
		}
		found = true
	}
	if !found {
		return nil
	}

	return &score
}

func ResolveRoute(tier config.Tier, coldMarks map[string]ColdMark, capacities map[string]harness.Capacity, now time.Time) Route {
	skipped := make([]string, 0, len(tier.Routes))
	candidates := make([]candidate, 0, len(tier.Routes))
	for _, configured := range tier.Routes {
		if cold, ok := coldMarks[configured.Harness]; ok && cold.ExpiresAt.After(now) {
			note := fmt.Sprintf("%s is cold until %s", configured.Harness, formatTime(cold.ExpiresAt))
			if !contains(skipped, note) {
				skipped = append(skipped, note)
			}
			continue
		}
		capacity, ok := capacities[configured.Harness]
		if !ok {
			capacity = harness.UnknownCapacity()
		}
		candidates = append(candidates, candidate{
			provider:             configured.Harness,
			model:                configured.Model,
			fallbackBelowPercent: configured.FallbackBelowPercent,
			capacity:             capacity,
			score:                AvailabilityFor(configured.Harness, configured.Model, capacity),
		})
	}

	regular := make([]candidate, 0, len(candidates))
	fallbacks := make([]candidate, 0, len(candidates))
	for index := range candidates {
		candidate := candidates[index]
		if candidate.fallbackBelowPercent != nil {
			if allOrdinaryRoutesBelow(tier.Routes, coldMarks, capacities, *candidate.fallbackBelowPercent, now) {
				fallbacks = append(fallbacks, candidate)
			}
			continue
		}
		regular = append(regular, candidate)
	}

	selected := selectRouteCandidate(regular, fallbacks)
	if selected == nil {
		why := "no providers configured"
		if len(skipped) > 0 {
			why = join(skipped, "; ")
		}
		return Route{RoutedBecause: fmt.Sprintf("tier %q: no eligible provider (%s)", tier.Name, why)}
	}

	selection := ""
	if selected.fallbackBelowPercent != nil {
		selection = fmt.Sprintf(
			"%s (%s fallback; every ordinary route is measured below %g%% available)",
			selected.provider,
			tier.Name,
			*selected.fallbackBelowPercent,
		)
	} else {
		score := "capacity unknown"
		if selected.score != nil {
			score = fmt.Sprintf("score %v", *selected.score)
		}
		selection = fmt.Sprintf("%s (%s; %s)", selected.provider, score, describeCapacity(selected.provider, selected.capacity))
	}
	unknownSuffix := ""
	if selected.score == nil && selected.fallbackBelowPercent == nil {
		unknownSuffix = "; capacity was unreadable, so it was selected only after measured candidates"
		if !selected.capacity.HasGauge() {
			unknownSuffix = "; the harness exposes no capacity gauge, so it was selected only after measured candidates"
		}
	}
	reasons := append([]string{fmt.Sprintf("tier %q → %s%s", tier.Name, selection, unknownSuffix)}, skipped...)

	return Route{
		Provider:      selected.provider,
		Model:         cloneString(selected.model),
		Score:         cloneFloat(selected.score),
		RoutedBecause: join(reasons, "; "),
	}
}

func RouteAfterExhaustion(tier config.Tier, exhaustedProvider string, coldMarks map[string]ColdMark, capacities map[string]harness.Capacity, now time.Time, rerouteCount int) (*Route, map[string]ColdMark, time.Time) {
	if rerouteCount >= 1 {
		return nil, nil, time.Time{}
	}
	nextColdMarks := cloneColdMarks(coldMarks)
	expiresAt := resetFor(capacities[exhaustedProvider], now)
	nextColdMarks[exhaustedProvider] = ColdMark{ExpiresAt: expiresAt}
	route := ResolveRoute(tier, nextColdMarks, capacities, now)
	if route.Provider == "" {
		return &route, nextColdMarks, expiresAt
	}
	route.RoutedBecause = fmt.Sprintf("%s; %s exhausted and was marked cold; re-routed once", route.RoutedBecause, exhaustedProvider)
	return &route, nextColdMarks, expiresAt
}

type candidate struct {
	provider             string
	model                *string
	fallbackBelowPercent *float64
	capacity             harness.Capacity
	score                *float64
}

// allOrdinaryRoutesBelow reports whether every ordinary route in the tier has
// fallen below a fallback route's threshold.
//
// A cold route counts as below it. Cold means the route confirmed a failure
// recently and cannot run at all, which is a stronger reason to reach for the
// safety valve than merely running low — and treating it otherwise would make a
// tier whose every ordinary route is cold fail outright while holding an
// eligible fallback in reserve.
func allOrdinaryRoutesBelow(routes []config.Route, coldMarks map[string]ColdMark, capacities map[string]harness.Capacity, threshold float64, now time.Time) bool {
	for _, route := range routes {
		if route.FallbackBelowPercent != nil {
			continue
		}
		if cold, ok := coldMarks[route.Harness]; ok && cold.ExpiresAt.After(now) {
			continue
		}
		capacity, ok := capacities[route.Harness]
		if !ok {
			capacity = harness.UnknownCapacity()
		}
		score := AvailabilityFor(route.Harness, route.Model, capacity)
		if score == nil || *score >= threshold {
			return false
		}
	}

	return true
}

func selectRouteCandidate(regular []candidate, fallbacks []candidate) *candidate {
	if len(fallbacks) > 0 {
		return &fallbacks[0]
	}
	var selected *candidate
	for index := range regular {
		candidate := &regular[index]
		if candidate.score == nil {
			continue
		}
		if selected == nil || *candidate.score > *selected.score {
			selected = candidate
		}
	}
	if selected != nil {
		return selected
	}
	if len(regular) > 0 {
		return &regular[0]
	}

	return nil
}

func relevantBucketsFor(provider string, model *string, buckets []harness.Bucket) []harness.Bucket {
	if provider != "codex" {
		return buckets
	}
	bucketID := codexBucketFor(model)
	filtered := make([]harness.Bucket, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket.ID == bucketID {
			filtered = append(filtered, bucket)
		}
	}

	return filtered
}

// Codex meters Spark separately from everything else, so the bucket follows
// the model, not the role. Keying this on the role happens to work only while
// intern-on-codex is always Spark and silently mis-scores a second model.
func codexBucketFor(model *string) string {
	if model != nil && *model == "gpt-5.3-codex-spark" {
		return "codex_bengalfox"
	}

	return "codex"
}

func describeCapacity(provider string, capacity harness.Capacity) string {
	if !capacity.HasGauge() {
		return fmt.Sprintf("%s has no capacity gauge", provider)
	}
	if !capacity.IsKnown() {
		return fmt.Sprintf("%s capacity is unknown", provider)
	}
	buckets := capacity.Buckets()
	parts := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		parts = append(parts, fmt.Sprintf("%s %v%% used", bucket.ID, bucket.UsedPercent))
	}

	return fmt.Sprintf("%s: %s", provider, join(parts, ", "))
}

func resetFor(capacity harness.Capacity, now time.Time) time.Time {
	buckets := capacity.Buckets()
	relevant := make([]harness.Bucket, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket.UsedPercent >= 95 {
			relevant = append(relevant, bucket)
		}
	}
	if len(relevant) == 0 {
		relevant = buckets
	}
	var earliest time.Time
	for _, bucket := range relevant {
		if !bucket.ResetsAt.After(now) {
			continue
		}
		if earliest.IsZero() || bucket.ResetsAt.Before(earliest) {
			earliest = bucket.ResetsAt
		}
	}
	if !earliest.IsZero() {
		return earliest
	}

	return now.Add(defaultColdDuration)
}

func cloneCapacities(capacities map[string]harness.Capacity) map[string]harness.Capacity {
	copyOfCapacities := make(map[string]harness.Capacity, len(capacities))
	for provider, capacity := range capacities {
		copyOfCapacities[provider] = capacity
	}

	return copyOfCapacities
}

func cloneColdMarks(coldMarks map[string]ColdMark) map[string]ColdMark {
	copyOfColdMarks := make(map[string]ColdMark, len(coldMarks))
	for provider, mark := range coldMarks {
		copyOfColdMarks[provider] = mark
	}

	return copyOfColdMarks
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}

func formatTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}

	return false
}

func join(values []string, separator string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += separator
		}
		result += value
	}

	return result
}
