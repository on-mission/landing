package dispatch

import (
	"strings"
	"testing"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
)

func TestRoleForResolvesTiersAndPinnedRoutes(t *testing.T) {
	configuration := config.Config{Tiers: map[string]config.Tier{
		"engineer": {Name: "engineer"},
	}}

	tests := []struct {
		name     string
		role     *string
		provider string
		want     string
		wantErr  string
	}{
		{name: "configured tier", role: stringPointer("engineer"), provider: "grok", want: "engineer"},
		{name: "route the caller pinned", role: stringPointer("grok/grok-4.6"), provider: "grok", want: "grok/grok-4.6"},
		{name: "route naming another provider", role: stringPointer("grok/grok-4.6"), provider: "codex", wantErr: `job "job-1" was created under tier "grok/grok-4.6"`},
		{name: "removed tier", role: stringPointer("intern"), provider: "cline", wantErr: `job "job-1" was created under tier "intern"`},
		{name: "no stored role", provider: "grok", wantErr: `job "job-1" has no stored tier`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := roleFor(harness.JobRecord{JobID: "job-1", Provider: test.provider, Role: test.role}, configuration)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("roleFor() = %q, %v; want an error containing %q", got, err, test.wantErr)
				}

				return
			}
			if err != nil || got != test.want {
				t.Fatalf("roleFor() = %q, %v; want %q, nil", got, err, test.want)
			}
		})
	}
}

func TestRoleForReadsAConfiguredTierBeforeARoute(t *testing.T) {
	// A project may name a tier anything, including something shaped like a
	// route. Its own configuration decides what its jobs mean.
	configuration := config.Config{Tiers: map[string]config.Tier{
		"grok/grok-4.6": {Name: "grok/grok-4.6"},
	}}
	got, err := roleFor(harness.JobRecord{JobID: "job-1", Provider: "grok", Role: stringPointer("grok/grok-4.6")}, configuration)
	if err != nil || got != "grok/grok-4.6" {
		t.Fatalf("roleFor() = %q, %v; want the configured tier", got, err)
	}
}

func TestPinnedRoleNamesTheRouteWhenNoTierWasRequested(t *testing.T) {
	engine := New(nil, nil, config.Config{Tiers: map[string]config.Tier{"engineer": {Name: "engineer"}}})
	model := "grok-4.6"
	route := &config.Route{Harness: "grok", Model: &model}

	role, err := engine.pinnedRole(Request{Route: route})
	if err != nil || role != "grok/grok-4.6" {
		t.Fatalf("pinnedRole(no tier) = %q, %v; want the pinned route", role, err)
	}

	// A meeting cast runs inside a tier and keeps that tier's name.
	role, err = engine.pinnedRole(Request{Tier: "engineer", Route: route})
	if err != nil || role != "engineer" {
		t.Fatalf("pinnedRole(tier) = %q, %v; want the tier name", role, err)
	}

	if _, err = engine.pinnedRole(Request{Tier: "absent", Route: route}); err == nil {
		t.Fatal("pinnedRole(unconfigured tier) returned nil error, want a tier resolution error")
	}
}

func TestRouteStringRendersWhatACallerWrites(t *testing.T) {
	model := "grok-4.6"
	if got := (config.Route{Harness: "grok", Model: &model}).String(); got != "grok/grok-4.6" {
		t.Fatalf("Route.String() = %q, want %q", got, "grok/grok-4.6")
	}
	if got := (config.Route{Harness: "cline"}).String(); got != "cline" {
		t.Fatalf("Route.String() = %q, want %q", got, "cline")
	}
}
