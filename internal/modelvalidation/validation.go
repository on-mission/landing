package modelvalidation

import (
	"context"

	"github.com/on-mission/landing/internal/harness"
)

func Validate(ctx context.Context, adapter harness.Adapter, model string) harness.ModelValidation {
	cache, err := LocalCache()
	if err == nil && cache.Get(ctx, adapter.ID(), model) {
		return harness.ValidModel("successful validation was cached within the last 24 hours")
	}

	validation := adapter.ValidateModel(ctx, model)
	if validation.Status != harness.ModelValid {
		return validation
	}
	if err := cache.Put(ctx, adapter.ID(), model, validation); err != nil {
		return harness.ValidModel("model probe returned a reply; local validation cache was not persisted")
	}

	return validation
}
