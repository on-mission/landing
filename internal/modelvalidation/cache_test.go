package modelvalidation

import (
	"context"
	"testing"
	"time"

	"github.com/on-mission/landing/internal/harness"
)

func TestCacheStoresOnlyValidResultsAndExpiresThemAfterADay(t *testing.T) {
	now := time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC)
	cache := NewCache(t.TempDir(), func() time.Time { return now })
	if err := cache.Put(context.Background(), "codex", "gpt-6-sol", harness.ValidModel("probe returned a reply")); err != nil {
		t.Fatalf("Cache.Put(valid) returned unexpected error: %v", err)
	}
	if err := cache.Put(context.Background(), "codex", "not-real", harness.InvalidModel("provider rejected the model")); err != nil {
		t.Fatalf("Cache.Put(invalid) returned unexpected error: %v", err)
	}
	if err := cache.Put(context.Background(), "codex", "unverified", harness.UnverifiedModel("rate-limited")); err != nil {
		t.Fatalf("Cache.Put(unverified) returned unexpected error: %v", err)
	}
	if !cache.Get(context.Background(), "codex", "gpt-6-sol") {
		t.Fatal("Cache.Get(valid) = false, want cached valid model")
	}
	if cache.Get(context.Background(), "codex", "not-real") || cache.Get(context.Background(), "codex", "unverified") {
		t.Fatal("Cache.Get() returned a cached invalid or unverified model")
	}

	expired := NewCache(cache.root, func() time.Time { return now.Add(24*time.Hour + time.Nanosecond) })
	if expired.Get(context.Background(), "codex", "gpt-6-sol") {
		t.Fatal("Cache.Get() = true after 24 hours, want expired validation")
	}
}

func TestValidateUsesAStoredSuccessfulResult(t *testing.T) {
	t.Setenv("LANDING_STATE_DIR", t.TempDir())
	adapter := &countingAdapter{}
	first := Validate(context.Background(), adapter, "gpt-6-sol")
	second := Validate(context.Background(), adapter, "gpt-6-sol")
	if first.Status != harness.ModelValid || second.Status != harness.ModelValid {
		t.Fatalf("Validate() results = %#v, %#v; want valid results", first, second)
	}
	if adapter.calls != 1 {
		t.Fatalf("Validate() called adapter %d times, want one probe followed by a cache hit", adapter.calls)
	}
}

type countingAdapter struct {
	harness.Adapter
	calls int
}

func (adapter *countingAdapter) ID() string {
	return "codex"
}

func (adapter *countingAdapter) ValidateModel(context.Context, string) harness.ModelValidation {
	adapter.calls++

	return harness.ValidModel("model probe returned a reply")
}
