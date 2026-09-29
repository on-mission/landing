// Package modelvalidation validates concrete harness models and remembers only
// successful validations in machine-local state.
package modelvalidation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/paths"
)

const validFor = 24 * time.Hour

type Cache struct {
	root string
	now  func() time.Time
}

type cacheFile struct {
	Entries map[string]cacheEntry `json:"entries"`
}

type cacheEntry struct {
	Harness     string    `json:"harness"`
	Model       string    `json:"model"`
	ValidatedAt time.Time `json:"validatedAt"`
}

func NewCache(root string, now func() time.Time) Cache {
	return Cache{root: root, now: now}
}

func LocalCache() (Cache, error) {
	root, err := paths.StateRoot()
	if err != nil {
		return Cache{}, err
	}

	return NewCache(root, time.Now), nil
}

func (cache Cache) Get(ctx context.Context, harnessID string, model string) bool {
	if err := ctx.Err(); err != nil {
		return false
	}
	file, err := cache.read()
	if err != nil {
		return false
	}
	entry, ok := file.Entries[cacheKey(harnessID, model)]
	if !ok {
		return false
	}

	return entry.ValidatedAt.Add(validFor).After(cache.now())
}

func (cache Cache) Put(ctx context.Context, harnessID string, model string, validation harness.ModelValidation) error {
	if validation.Status != harness.ModelValid {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := cache.read()
	if err != nil {
		return err
	}
	if file.Entries == nil {
		file.Entries = make(map[string]cacheEntry)
	}
	file.Entries[cacheKey(harnessID, model)] = cacheEntry{Harness: harnessID, Model: model, ValidatedAt: cache.now().UTC()}

	return cache.write(file)
}

func (cache Cache) read() (cacheFile, error) {
	contents, err := os.ReadFile(cache.path())
	if errors.Is(err, fs.ErrNotExist) {
		return cacheFile{Entries: make(map[string]cacheEntry)}, nil
	}
	if err != nil {
		return cacheFile{}, fmt.Errorf("read model validation cache: %w", err)
	}
	var file cacheFile
	if err := json.Unmarshal(contents, &file); err != nil {
		return cacheFile{}, fmt.Errorf("decode model validation cache: %w", err)
	}
	if file.Entries == nil {
		file.Entries = make(map[string]cacheEntry)
	}

	return file, nil
}

func (cache Cache) write(file cacheFile) error {
	if err := os.MkdirAll(cache.root, 0o700); err != nil {
		return fmt.Errorf("create local state directory: %w", err)
	}
	contents, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("encode model validation cache: %w", err)
	}
	temporary, err := os.CreateTemp(cache.root, ".model-validations-")
	if err != nil {
		return fmt.Errorf("create model validation cache: %w", err)
	}
	temporaryPath := temporary.Name()
	if _, err := temporary.Write(contents); err != nil {
		return closeAndRemove(temporary, temporaryPath, err)
	}
	if err := temporary.Chmod(0o600); err != nil {
		return closeAndRemove(temporary, temporaryPath, err)
	}
	if err := temporary.Close(); err != nil {
		return closeAndRemove(nil, temporaryPath, err)
	}
	if err := os.Rename(temporaryPath, cache.path()); err != nil {
		return closeAndRemove(nil, temporaryPath, err)
	}

	return nil
}

func (cache Cache) path() string {
	return filepath.Join(cache.root, "model-validations.json")
}

func cacheKey(harnessID string, model string) string {
	return harnessID + "\x00" + model
}

func closeAndRemove(file *os.File, path string, cause error) error {
	if file != nil {
		if err := file.Close(); err != nil {
			cause = errors.Join(cause, err)
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		cause = errors.Join(cause, err)
	}

	return cause
}
