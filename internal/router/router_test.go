package router

import (
	"strings"
	"testing"
	"time"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
)

var routingNow = time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)

func routingCapacity(buckets ...harness.Bucket) harness.Capacity {
	return harness.KnownCapacity(buckets)
}

func routingBucket(id string, usedPercent float64) harness.Bucket {
	return harness.Bucket{ID: id, UsedPercent: usedPercent, ResetsAt: routingNow.Add(time.Minute)}
}

func routingTier(name string, routes ...config.Route) config.Tier {
	return config.Tier{Name: name, Routes: routes}
}

func routingModel(value string) *string {
	return &value
}

func routingFallback(value float64) *float64 {
	return &value
}

// internShapedTier mirrors the multi-route, cline-fallback shape these tests
// need. It is stated here rather than borrowed from a shipped configuration so
// that changing Landing's own routing policy cannot silently change what these
// tests cover.
func internShapedTier() config.Tier {
	return routingTier(
		"intern",
		config.Route{Harness: "grok", Model: routingModel("grok-4.5")},
		config.Route{Harness: "codex", Model: routingModel("gpt-5.3-codex-spark")},
		config.Route{Harness: "claude", Model: routingModel("claude-haiku-4-5-20251001")},
		config.Route{Harness: "codex", Model: routingModel("gpt-5.6-luna")},
		config.Route{Harness: "cline", FallbackBelowPercent: routingFallback(20)},
	)
}

func engineerTier() config.Tier {
	return routingTier("engineer",
		config.Route{Harness: "grok", Model: routingModel("grok-4.5")},
		config.Route{Harness: "codex", Model: routingModel("gpt-5.6-terra")},
		config.Route{Harness: "claude", Model: routingModel("claude-sonnet-5")},
	)
}

func internTier() config.Tier {
	return routingTier("intern",
		config.Route{Harness: "grok", Model: routingModel("grok-4.5")},
		config.Route{Harness: "codex", Model: routingModel("gpt-5.3-codex-spark")},
		config.Route{Harness: "claude", Model: routingModel("claude-haiku-4-5-20251001")},
		config.Route{Harness: "codex", Model: routingModel("gpt-5.6-luna")},
		config.Route{Harness: "cline", FallbackBelowPercent: routingFallback(20)},
	)
}

func TestResolveRouteCapacitySelection(t *testing.T) {
	tests := []struct {
		name       string
		tier       config.Tier
		capacities map[string]harness.Capacity
		provider   string
		model      string
		score      float64
	}{
		{
			name: "highest availability wins across providers",
			tier: engineerTier(),
			capacities: map[string]harness.Capacity{
				"grok":   routingCapacity(routingBucket("credits", 40)),
				"codex":  routingCapacity(routingBucket("codex", 30), routingBucket("codex_bengalfox", 0)),
				"claude": routingCapacity(routingBucket("five_hour", 10), routingBucket("seven_day", 10)),
			},
			provider: "claude", model: "claude-sonnet-5", score: 90,
		},
		{
			name: "most constrained provider window controls score",
			tier: engineerTier(),
			capacities: map[string]harness.Capacity{
				"grok":   routingCapacity(routingBucket("credits", 60)),
				"codex":  routingCapacity(routingBucket("codex", 80)),
				"claude": routingCapacity(routingBucket("five_hour", 90), routingBucket("seven_day", 20)),
			},
			provider: "grok", model: "grok-4.5", score: 40,
		},
		{
			name: "spark uses its separate codex bucket",
			tier: internTier(),
			capacities: map[string]harness.Capacity{
				"grok":   routingCapacity(routingBucket("credits", 90)),
				"codex":  routingCapacity(routingBucket("codex", 95), routingBucket("codex_bengalfox", 10)),
				"claude": routingCapacity(routingBucket("five_hour", 90), routingBucket("seven_day", 90)),
				"cline":  harness.UnknownCapacity(),
			},
			provider: "codex", model: "gpt-5.3-codex-spark", score: 90,
		},
		{
			name: "ties preserve declared provider preference",
			tier: engineerTier(),
			capacities: map[string]harness.Capacity{
				"grok":   routingCapacity(routingBucket("credits", 50)),
				"codex":  routingCapacity(routingBucket("codex", 50)),
				"claude": routingCapacity(routingBucket("five_hour", 50), routingBucket("seven_day", 50)),
			},
			provider: "grok", model: "grok-4.5", score: 50,
		},
		{
			name: "known capacity outranks unknown capacity",
			tier: engineerTier(),
			capacities: map[string]harness.Capacity{
				"grok":   harness.UnknownCapacity(),
				"codex":  routingCapacity(routingBucket("codex", 20)),
				"claude": harness.UnknownCapacity(),
			},
			provider: "codex", model: "gpt-5.6-terra", score: 80,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			route := ResolveRoute(test.tier, map[string]ColdMark{}, test.capacities, routingNow)
			if route.Provider != test.provider || route.Model == nil || *route.Model != test.model || route.Score == nil || *route.Score != test.score {
				t.Fatalf("route = %#v, want provider=%q model=%q score=%v", route, test.provider, test.model, test.score)
			}
			if !strings.Contains(route.RoutedBecause, "tier \""+test.tier.Name+"\" → "+test.provider) {
				t.Fatalf("routedBecause = %q, want selected provider explanation", route.RoutedBecause)
			}
		})
	}
}

func TestAvailabilityForCodexModelBuckets(t *testing.T) {
	capacity := routingCapacity(routingBucket("codex", 90), routingBucket("codex_bengalfox", 0))
	spark := "gpt-5.3-codex-spark"
	luna := "gpt-5.6-luna"

	if score := AvailabilityFor("codex", &spark, capacity); score == nil || *score != 100 {
		t.Fatalf("Spark score = %v, want 100", score)
	}
	if score := AvailabilityFor("codex", &luna, capacity); score == nil || *score != 10 {
		t.Fatalf("Luna score = %v, want 10", score)
	}
}

func TestResolveRouteClineFallbackAndColdMarks(t *testing.T) {
	tier := internShapedTier()
	available := ResolveRoute(tier, map[string]ColdMark{}, map[string]harness.Capacity{
		"grok":   routingCapacity(routingBucket("credits", 81)),
		"codex":  routingCapacity(routingBucket("codex", 99), routingBucket("codex_bengalfox", 80)),
		"claude": routingCapacity(routingBucket("five_hour", 99), routingBucket("seven_day", 99)),
		"cline":  harness.UnknownCapacity(),
	}, routingNow)
	if available.Provider != "codex" {
		t.Fatalf("intern route with 21%% measured availability = %#v, want codex instead of cline", available)
	}

	capacities := map[string]harness.Capacity{
		"grok":   routingCapacity(routingBucket("credits", 81)),
		"codex":  routingCapacity(routingBucket("codex", 99), routingBucket("codex_bengalfox", 81)),
		"claude": routingCapacity(routingBucket("five_hour", 81), routingBucket("seven_day", 81)),
		"cline":  harness.UnknownCapacity(),
	}
	route := ResolveRoute(tier, map[string]ColdMark{}, capacities, routingNow)
	if route.Provider != "cline" || !strings.Contains(route.RoutedBecause, "intern fallback") {
		t.Fatalf("low-capacity intern route = %#v, want cline fallback", route)
	}

	allCold := ResolveRoute(tier, map[string]ColdMark{
		"grok":   {ExpiresAt: routingNow.Add(time.Minute)},
		"codex":  {ExpiresAt: routingNow.Add(time.Minute)},
		"claude": {ExpiresAt: routingNow.Add(time.Minute)},
	}, capacities, routingNow)
	if allCold.Provider != "cline" || strings.Count(allCold.RoutedBecause, "codex is cold") != 1 {
		t.Fatalf("all-cold intern route = %#v, want cline and one codex cold explanation", allCold)
	}

	cold := ResolveRoute(engineerTier(), map[string]ColdMark{"grok": {ExpiresAt: routingNow.Add(time.Minute)}}, map[string]harness.Capacity{
		"grok":   routingCapacity(routingBucket("credits", 0)),
		"codex":  routingCapacity(routingBucket("codex", 20)),
		"claude": routingCapacity(routingBucket("five_hour", 50), routingBucket("seven_day", 50)),
	}, routingNow)
	if cold.Provider != "codex" || !strings.Contains(cold.RoutedBecause, "grok is cold until") {
		t.Fatalf("active-cold route = %#v, want grok excluded", cold)
	}

	expired := ResolveRoute(engineerTier(), map[string]ColdMark{"grok": {ExpiresAt: routingNow.Add(-time.Second)}}, map[string]harness.Capacity{
		"grok":   routingCapacity(routingBucket("credits", 0)),
		"codex":  routingCapacity(routingBucket("codex", 20)),
		"claude": routingCapacity(routingBucket("five_hour", 50), routingBucket("seven_day", 50)),
	}, routingNow)
	if expired.Provider != "grok" {
		t.Fatalf("expired-cold route provider = %q, want grok", expired.Provider)
	}

	allUnknown := ResolveRoute(engineerTier(), map[string]ColdMark{}, map[string]harness.Capacity{
		"grok": harness.UnknownCapacity(), "codex": harness.UnknownCapacity(), "claude": harness.UnknownCapacity(),
	}, routingNow)
	if allUnknown.Provider != "grok" || !strings.Contains(allUnknown.RoutedBecause, "capacity was unreadable") {
		t.Fatalf("all-unknown route = %#v, want declared first provider with unknown explanation", allUnknown)
	}
}

func TestResolveRouteSingleRouteTier(t *testing.T) {
	tier := routingTier("single", config.Route{Harness: "grok", Model: routingModel("grok-4.5")})
	route := ResolveRoute(tier, map[string]ColdMark{}, map[string]harness.Capacity{
		"grok": routingCapacity(routingBucket("credits", 30)),
	}, routingNow)

	if route.Provider != "grok" || route.Model == nil || *route.Model != "grok-4.5" || route.Score == nil || *route.Score != 70 {
		t.Fatalf("ResolveRoute(single route) = %#v, want grok with score 70", route)
	}
}

func TestResolveRouteEvaluatesFallbackRoutesIndependently(t *testing.T) {
	tier := routingTier("custom",
		config.Route{Harness: "grok", Model: routingModel("grok-4.5")},
		config.Route{Harness: "fallback-at-10", FallbackBelowPercent: routingFallback(10)},
		config.Route{Harness: "fallback-at-20", FallbackBelowPercent: routingFallback(20)},
	)
	route := ResolveRoute(tier, map[string]ColdMark{}, map[string]harness.Capacity{
		"grok":           routingCapacity(routingBucket("credits", 85)),
		"fallback-at-10": harness.UnknownCapacity(),
		"fallback-at-20": harness.UnknownCapacity(),
	}, routingNow)

	if route.Provider != "fallback-at-20" || !strings.Contains(route.RoutedBecause, "below 20% available") {
		t.Fatalf("ResolveRoute(independent fallbacks) = %#v, want only the 20%% fallback", route)
	}
}

func TestResolveRouteWithholdsFallbackWhenAnOrdinaryRouteIsAboveThreshold(t *testing.T) {
	tier := routingTier("custom",
		config.Route{Harness: "grok", Model: routingModel("grok-4.5")},
		config.Route{Harness: "fallback", FallbackBelowPercent: routingFallback(20)},
	)
	route := ResolveRoute(tier, map[string]ColdMark{}, map[string]harness.Capacity{
		"grok":     routingCapacity(routingBucket("credits", 79)),
		"fallback": harness.UnknownCapacity(),
	}, routingNow)

	if route.Provider != "grok" || route.Score == nil || *route.Score != 21 {
		t.Fatalf("ResolveRoute(ordinary route above threshold) = %#v, want grok with score 21", route)
	}
}

func TestRouteAfterExhaustionReroutesOnlyOnce(t *testing.T) {
	capacities := map[string]harness.Capacity{
		"grok":   routingCapacity(routingBucket("credits", 100)),
		"codex":  routingCapacity(routingBucket("codex", 20)),
		"claude": routingCapacity(routingBucket("five_hour", 30), routingBucket("seven_day", 30)),
	}
	route, coldMarks, _ := RouteAfterExhaustion(engineerTier(), "grok", map[string]ColdMark{}, capacities, routingNow, 0)
	if route == nil || route.Provider != "codex" || !strings.Contains(route.RoutedBecause, "re-routed once") {
		t.Fatalf("first exhaustion route = %#v, want one-hop reroute to codex", route)
	}
	second, _, _ := RouteAfterExhaustion(engineerTier(), "codex", coldMarks, capacities, routingNow, 1)
	if second != nil {
		t.Fatalf("second exhaustion route = %#v, want no second reroute", second)
	}
}

// A fallback route is a last resort, so it must still be reachable when every
// ordinary route is cold rather than merely low. Before this was covered, an
// all-cold tier resolved to no provider at all while holding an eligible
// fallback in reserve.
func TestFallbackServesATierWhoseOrdinaryRoutesAreAllCold(t *testing.T) {
	tier := internShapedTier()
	// Deliberately high availability: the threshold alone would withhold the
	// fallback, so only the cold marks can make it eligible.
	capacities := map[string]harness.Capacity{
		"grok":   routingCapacity(routingBucket("credits", 5)),
		"codex":  routingCapacity(routingBucket("codex", 5), routingBucket("codex_bengalfox", 5)),
		"claude": routingCapacity(routingBucket("five_hour", 5), routingBucket("seven_day", 5)),
		"cline":  harness.UnknownCapacity(),
	}
	route := ResolveRoute(tier, map[string]ColdMark{
		"grok":   {ExpiresAt: routingNow.Add(time.Minute)},
		"codex":  {ExpiresAt: routingNow.Add(time.Minute)},
		"claude": {ExpiresAt: routingNow.Add(time.Minute)},
	}, capacities, routingNow)
	if route.Provider != "cline" {
		t.Fatalf("all-cold tier route = %#v; want the fallback route as a last resort", route)
	}
}
