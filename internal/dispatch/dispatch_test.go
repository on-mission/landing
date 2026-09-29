package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
	"github.com/on-mission/landing/internal/journal"
	"github.com/on-mission/landing/internal/router"
)

func TestDispatchReportsRequestedTierAndConfiguredTiers(t *testing.T) {
	engine := New(nil, nil, config.Config{Tiers: map[string]config.Tier{
		"engineer": {Name: "engineer"},
		"review":   {Name: "review"},
	}})

	_, err := engine.Dispatch(context.Background(), Request{Tier: "intern"})
	if err == nil {
		t.Fatal("Dispatch(intern) returned nil error, want an unknown-tier error")
	}
	if !strings.Contains(err.Error(), "requested tier \"intern\" is not configured") || !strings.Contains(err.Error(), "\"engineer\", \"review\"") {
		t.Fatalf("Dispatch(intern) error = %q, want requested and configured tier names", err)
	}
}

func TestTierDispatchRecoversFromModelValidationFailure(t *testing.T) {
	t.Setenv("LANDING_STATE_DIR", t.TempDir())
	project := conversationTestProject(t)
	invalidModel := "missing"
	validModel := "available"
	invalid := &validationLifecycleAdapter{id: "invalid", validation: harness.InvalidModel("provider rejected missing")}
	valid := &validationLifecycleAdapter{id: "valid", validation: harness.ValidModel("probe returned a reply")}
	store := jobs.NewStore(context.Background())
	t.Cleanup(func() { store.Shutdown(context.Background()) })
	engine := New(router.New(router.NewMapRegistry(map[string]harness.Adapter{"invalid": invalid, "valid": valid})), store, config.Config{Tiers: map[string]config.Tier{
		"engineer": {Name: "engineer", Routes: []config.Route{{Harness: "invalid", Model: &invalidModel}, {Harness: "valid", Model: &validModel}}},
	}})
	response, err := engine.Dispatch(context.Background(), Request{Tier: "engineer", Prompt: "work", CWD: project})
	if err != nil {
		t.Fatalf("Dispatch(tier validation fallback) returned unexpected error: %v", err)
	}
	if response.Provider != "valid" || valid.starts != 1 || invalid.starts != 0 {
		t.Fatalf("Dispatch(tier validation fallback) = %#v; starts invalid=%d valid=%d; want only valid replacement route", response, invalid.starts, valid.starts)
	}
}

func TestNamedRouteRefusesModelValidationFailureWithoutSubstitution(t *testing.T) {
	t.Setenv("LANDING_STATE_DIR", t.TempDir())
	project := conversationTestProject(t)
	model := "missing"
	invalid := &validationLifecycleAdapter{id: "invalid", validation: harness.InvalidModel("provider rejected missing")}
	valid := &validationLifecycleAdapter{id: "valid", validation: harness.ValidModel("probe returned a reply")}
	store := jobs.NewStore(context.Background())
	t.Cleanup(func() { store.Shutdown(context.Background()) })
	engine := New(router.New(router.NewMapRegistry(map[string]harness.Adapter{"invalid": invalid, "valid": valid})), store, config.Config{})
	_, err := engine.Dispatch(context.Background(), Request{Prompt: "work", CWD: project, Route: &config.Route{Harness: "invalid", Model: &model}})
	if err == nil || !strings.Contains(err.Error(), "model invalid/missing is invalid") {
		t.Fatalf("Dispatch(named validation failure) error = %v, want named model rejection", err)
	}
	if invalid.starts != 0 || valid.starts != 0 {
		t.Fatalf("Dispatch(named validation failure) started invalid=%d valid=%d routes, want none", invalid.starts, valid.starts)
	}
}

func TestReplyKeepsInitialConversationRecord(t *testing.T) {
	project := conversationTestProject(t)
	state := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", state)
	t.Setenv("LANDING_DISPATCH_CONVERSATION_HELPER", "1")
	adapter := conversationLifecycleAdapter{}
	store := jobs.NewStore(context.Background())
	engine := New(router.New(router.NewMapRegistry(map[string]harness.Adapter{"fake": adapter})), store, config.Config{Tiers: map[string]config.Tier{"engineer": {Name: "engineer"}}})
	t.Cleanup(func() { store.Shutdown(context.Background()) })
	label := "fix-b2"
	initial, err := engine.Dispatch(context.Background(), Request{
		Tier:   "engineer",
		Prompt: "start",
		CWD:    project,
		Label:  &label,
		Route:  &config.Route{Harness: "fake"},
	})
	if err != nil {
		t.Fatalf("Dispatch() returned unexpected error: %v", err)
	}
	for _, prompt := range []string{"first reply", "second reply"} {
		response, err := engine.Reply(context.Background(), ReplyRequest{JobID: initial.JobID, Prompt: prompt})
		if err != nil {
			t.Fatalf("Reply(%q) returned unexpected error: %v", prompt, err)
		}
		if response.JobID != initial.JobID {
			t.Fatalf("Reply(%q) job id = %q, want initial conversation id %q", prompt, response.JobID, initial.JobID)
		}
	}
	record, err := journal.ReadJob(context.Background(), initial.JobID)
	if err != nil || record == nil {
		t.Fatalf("ReadJob(%q) = %#v, %v; want retained conversation", initial.JobID, record, err)
	}
	if record.ThreadID == nil || *record.ThreadID != "thread-123" {
		t.Fatalf("ReadJob(%q) thread = %v, want thread-123", initial.JobID, record.ThreadID)
	}
	if record.Label == nil || *record.Label != label {
		t.Fatalf("ReadJob(%q) label = %v, want %q", initial.JobID, record.Label, label)
	}
	entries, err := os.ReadDir(filepath.Join(state, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			files++
		}
	}
	if files != 1 {
		t.Fatalf("retained conversation records = %d, want 1", files)
	}
}

func conversationTestProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".landing"), 0o700); err != nil {
		t.Fatal(err)
	}

	return project
}

type conversationLifecycleAdapter struct{}

type validationLifecycleAdapter struct {
	id         string
	validation harness.ModelValidation
	starts     int
}

func (adapter *validationLifecycleAdapter) ID() string {
	return adapter.id
}

func (adapter *validationLifecycleAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{Authority: harness.ModelCatalogAdvisory}
}

func (adapter *validationLifecycleAdapter) ValidateModel(context.Context, string) harness.ModelValidation {
	return adapter.validation
}

func (adapter *validationLifecycleAdapter) Detect(context.Context) harness.Detection {
	return harness.Detection{Status: harness.DetectionReady}
}

func (adapter *validationLifecycleAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{}
}

func (adapter *validationLifecycleAdapter) Validate(harness.StartParams) error {
	return nil
}

func (adapter *validationLifecycleAdapter) BuildStart(harness.StartParams) (harness.Request, error) {
	adapter.starts++
	return harness.Request{Command: os.Args[0], Args: []string{"-test.run=^TestValidationLifecycleHelper$"}}, nil
}

func (adapter *validationLifecycleAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return adapter.BuildStart(harness.StartParams{})
}

func (adapter *validationLifecycleAdapter) OnStdoutLine(string, *harness.JobRecord) {}

func (adapter *validationLifecycleAdapter) Finalize(context.Context, harness.FinalizeParams) (harness.Finalized, error) {
	return harness.Finalized{Status: harness.JobStatusDone}, nil
}

func (adapter *validationLifecycleAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return harness.UnknownCapacity()
}

func (adapter *validationLifecycleAdapter) SpawnPath() string {
	return ""
}

func TestValidationLifecycleHelper(t *testing.T) {}

func (conversationLifecycleAdapter) ID() string { return "fake" }

func (conversationLifecycleAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{Authority: harness.ModelCatalogAuthoritative}
}

func (conversationLifecycleAdapter) ValidateModel(context.Context, string) harness.ModelValidation {
	return harness.ValidModel("test validation")
}

func (conversationLifecycleAdapter) Detect(context.Context) harness.Detection {
	return harness.Detection{Status: harness.DetectionAbsent, Capacity: harness.UnknownCapacity()}
}

func (conversationLifecycleAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{Continuation: true}
}

func (conversationLifecycleAdapter) Validate(harness.StartParams) error { return nil }

func (conversationLifecycleAdapter) BuildStart(harness.StartParams) (harness.Request, error) {
	return harness.Request{Command: os.Args[0], Args: []string{"-test.run=^TestDispatchConversationHelper$"}}, nil
}

func (adapter conversationLifecycleAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return adapter.BuildStart(harness.StartParams{})
}

func (conversationLifecycleAdapter) OnStdoutLine(line string, record *harness.JobRecord) {
	if threadID, ok := strings.CutPrefix(line, "thread="); ok {
		record.ThreadID = &threadID
	}
}

func (conversationLifecycleAdapter) Finalize(context.Context, harness.FinalizeParams) (harness.Finalized, error) {
	return harness.Finalized{Status: harness.JobStatusDone}, nil
}

func (conversationLifecycleAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return harness.UnknownCapacity()
}

func (conversationLifecycleAdapter) SpawnPath() string { return "" }

func TestDispatchConversationHelper(t *testing.T) {
	if os.Getenv("LANDING_DISPATCH_CONVERSATION_HELPER") != "1" {
		return
	}
	if _, err := fmt.Fprintln(os.Stdout, "thread=thread-123"); err != nil {
		t.Fatal(err)
	}
}

func TestReplyReportsRemovedStoredTier(t *testing.T) {
	store := jobs.NewStore(context.Background())
	role := "intern"
	provider := "cline"
	threadID := "thread-123"
	_, err := store.Adopt(context.Background(), journal.Record{
		JobID:    "job-123",
		Provider: &provider,
		Role:     &role,
		ThreadID: &threadID,
		Status:   harness.JobStatusDone,
	}, "")
	if err != nil {
		t.Fatalf("Adopt() returned unexpected error: %v", err)
	}
	engine := New(nil, store, config.Config{Tiers: map[string]config.Tier{
		"engineer": {Name: "engineer"},
	}})

	_, err = engine.Reply(context.Background(), ReplyRequest{JobID: "job-123", Prompt: "continue"})
	if err == nil {
		t.Fatal("Reply() returned nil error, want a removed-tier error")
	}
	if !strings.Contains(err.Error(), "job \"job-123\" was created under tier \"intern\"") || !strings.Contains(err.Error(), "configured tiers are \"engineer\"") {
		t.Fatalf("Reply() error = %q, want stored and configured tier names", err)
	}
}

func TestAwaitUsesTimeoutOnlyWhenCallerSuppliesOne(t *testing.T) {
	t.Setenv("LANDING_DISPATCH_TIMEOUT_HELPER", "1")
	store := jobs.NewStore(context.Background())
	engine := New(nil, store, config.Config{})
	// Created before the cleanup below is registered, so it is removed after
	// the helper processes have exited: Windows refuses to delete a directory
	// a running process still uses as its working directory.
	cwd := t.TempDir()
	jobIDs := make([]string, 0, 2)
	t.Cleanup(func() {
		store.Shutdown(context.Background())
		for _, jobID := range jobIDs {
			if _, err := store.Wait(context.Background(), jobID); err != nil {
				t.Errorf("Wait(%q) during cleanup returned unexpected error: %v", jobID, err)
			}
		}
	})

	withoutTimeout, err := store.Start(context.Background(), timeoutLifecycleAdapter{}, jobs.StartOptions{CWD: cwd})
	if err != nil {
		t.Fatalf("Start() returned unexpected error: %v", err)
	}
	jobIDs = append(jobIDs, withoutTimeout.JobID)
	deadline, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, _, err := engine.await(deadline, withoutTimeout.JobID, 0, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("await() without a timeout error = %v; want deadline exceeded", err)
	}
	running, ok := store.Get(withoutTimeout.JobID)
	if !ok || running.Status != harness.JobStatusRunning {
		t.Fatalf("job after await() without a timeout = %#v, exists %t; want running", running, ok)
	}

	withTimeout, err := store.Start(context.Background(), timeoutLifecycleAdapter{}, jobs.StartOptions{CWD: cwd})
	if err != nil {
		t.Fatalf("Start() returned unexpected error: %v", err)
	}
	jobIDs = append(jobIDs, withTimeout.JobID)
	timedOut, message, err := engine.await(context.Background(), withTimeout.JobID, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("await() with a timeout returned unexpected error: %v", err)
	}
	if timedOut.Status != harness.JobStatusTimeout || message == "" {
		t.Fatalf("await() with a timeout = %#v, %q; want a timeout record and message", timedOut, message)
	}
}

type timeoutLifecycleAdapter struct{}

func (timeoutLifecycleAdapter) ID() string { return "timeout" }

func (timeoutLifecycleAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{Authority: harness.ModelCatalogAuthoritative}
}

func (timeoutLifecycleAdapter) ValidateModel(context.Context, string) harness.ModelValidation {
	return harness.ValidModel("test validation")
}

func (timeoutLifecycleAdapter) Detect(context.Context) harness.Detection {
	return harness.Detection{Status: harness.DetectionAbsent, Capacity: harness.UnknownCapacity()}
}

func (timeoutLifecycleAdapter) Capabilities() harness.Capabilities { return harness.Capabilities{} }

func (timeoutLifecycleAdapter) Validate(harness.StartParams) error { return nil }

func (timeoutLifecycleAdapter) BuildStart(harness.StartParams) (harness.Request, error) {
	return harness.Request{Command: os.Args[0], Args: []string{"-test.run=^TestDispatchTimeoutHelper$"}}, nil
}

func (adapter timeoutLifecycleAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return adapter.BuildStart(harness.StartParams{})
}

func (timeoutLifecycleAdapter) OnStdoutLine(string, *harness.JobRecord) {}

func (timeoutLifecycleAdapter) Finalize(context.Context, harness.FinalizeParams) (harness.Finalized, error) {
	return harness.Finalized{Status: harness.JobStatusDone}, nil
}

func (timeoutLifecycleAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return harness.UnknownCapacity()
}

func (timeoutLifecycleAdapter) SpawnPath() string { return "" }

func TestDispatchTimeoutHelper(t *testing.T) {
	if os.Getenv("LANDING_DISPATCH_TIMEOUT_HELPER") != "1" {
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
