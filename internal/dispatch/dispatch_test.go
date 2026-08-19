package dispatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func (conversationLifecycleAdapter) ID() string { return "fake" }

func (conversationLifecycleAdapter) Models() []string { return nil }

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
