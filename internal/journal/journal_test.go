package journal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/paths"
)

func TestRecordJobOnlyPersistsTerminalRecords(t *testing.T) {
	state := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", state)
	RecordJob(context.Background(), harness.JobRecord{JobID: "running", Status: harness.JobStatusRunning})
	if _, err := os.Stat(filepath.Join(state, "jobs", "running.json")); !os.IsNotExist(err) {
		t.Fatalf("running record stat error = %v, want no journal file", err)
	}

	finished := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	routedBecause := "tier \"engineer\" selected codex at 80% available"
	RecordJob(context.Background(), harness.JobRecord{JobID: "done", Provider: "codex", Status: harness.JobStatusDone, FinishedAt: &finished, RoutedBecause: &routedBecause})
	record, err := ReadJob(context.Background(), "done")
	if err != nil || record == nil || record.Status != harness.JobStatusDone || record.Provider == nil || *record.Provider != "codex" {
		t.Fatalf("ReadJob(done) = %#v, %v; want terminal record", record, err)
	}
	if record.RoutedBecause == nil || *record.RoutedBecause != routedBecause {
		t.Fatalf("ReadJob(done) RoutedBecause = %v, want %q", record.RoutedBecause, routedBecause)
	}
}

func TestRecordJobPreservesAbsentRoutedBecause(t *testing.T) {
	t.Setenv("LANDING_STATE_DIR", t.TempDir())
	RecordJob(context.Background(), harness.JobRecord{JobID: "without-rationale", Status: harness.JobStatusDone})

	record, err := ReadJob(context.Background(), "without-rationale")
	if err != nil || record == nil {
		t.Fatalf("ReadJob(without-rationale) = %#v, %v; want terminal record", record, err)
	}
	if record.RoutedBecause != nil {
		t.Fatalf("ReadJob(without-rationale) RoutedBecause = %q, want nil", *record.RoutedBecause)
	}
}

func TestWriteAtomicReplacesRecordWithoutTemporaryArtifacts(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "job.json"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(directory, Record{JobID: "job", Status: harness.JobStatusDone}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(directory, "job.json"))
	if err != nil || string(contents) == "old" {
		t.Fatalf("replacement contents = %q, %v", contents, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "job.json" {
		t.Fatalf("directory entries = %#v, %v; want only replacement", entries, err)
	}
}

func TestPurgeJobsRetainsOldRecordAndRejectsPathTraversal(t *testing.T) {
	state := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", state)
	directory, err := paths.EnsureDirs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, jobID := range []string{"expired", "fresh"} {
		if err := writeAtomic(directory, Record{JobID: jobID, Status: harness.JobStatusDone}); err != nil {
			t.Fatal(err)
		}
	}
	expired := filepath.Join(directory, "expired.json")
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatal(err)
	}
	purgeJobRecords(context.Background(), directory, maxJobRecords)
	if record, err := ReadJob(context.Background(), "expired"); err != nil || record == nil {
		t.Fatalf("ReadJob(expired) = %#v, %v; want retained record", record, err)
	}
	for _, jobID := range []string{"../outside", "a/b", "a\\b", ""} {
		record, err := ReadJob(context.Background(), jobID)
		if err != nil || record != nil {
			t.Fatalf("ReadJob(%q) = %#v, %v; want nil, nil", jobID, record, err)
		}
	}
}

func TestPurgeJobRecordsEvictsOldestRecordsAtCap(t *testing.T) {
	directory := t.TempDir()
	base := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	for index, jobID := range []string{"job-0", "job-1", "job-2", "job-3"} {
		if err := writeAtomic(directory, Record{JobID: jobID, Status: harness.JobStatusDone}); err != nil {
			t.Fatal(err)
		}
		modified := base.Add(time.Duration(index) * time.Minute)
		if err := os.Chtimes(filepath.Join(directory, jobID+".json"), modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	purgeJobRecords(context.Background(), directory, 3)
	if _, err := os.Stat(filepath.Join(directory, "job-0.json")); !os.IsNotExist(err) {
		t.Fatalf("oldest journal stat error = %v, want deletion", err)
	}
	for _, jobID := range []string{"job-1", "job-2", "job-3"} {
		if _, err := os.Stat(filepath.Join(directory, jobID+".json")); err != nil {
			t.Fatalf("newest journal %q stat error = %v, want retained file", jobID, err)
		}
	}
}

func TestPurgeJobRecordsEvictsWholeConversationsAtCap(t *testing.T) {
	directory := t.TempDir()
	base := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	oldThread := "thread-old"
	newThread := "thread-new"
	for index, record := range []Record{
		{JobID: "old-first", ThreadID: &oldThread, Status: harness.JobStatusDone},
		{JobID: "old-second", ThreadID: &oldThread, Status: harness.JobStatusDone},
		{JobID: "new-first", ThreadID: &newThread, Status: harness.JobStatusDone},
		{JobID: "new-second", ThreadID: &newThread, Status: harness.JobStatusDone},
	} {
		if err := writeAtomic(directory, record); err != nil {
			t.Fatal(err)
		}
		modified := base.Add(time.Duration(index) * time.Minute)
		if err := os.Chtimes(filepath.Join(directory, record.JobID+".json"), modified, modified); err != nil {
			t.Fatal(err)
		}
	}

	purgeJobRecords(context.Background(), directory, 1)
	for _, jobID := range []string{"old-first", "old-second"} {
		if _, err := os.Stat(filepath.Join(directory, jobID+".json")); !os.IsNotExist(err) {
			t.Fatalf("old conversation record %q stat error = %v, want deletion", jobID, err)
		}
	}
	for _, jobID := range []string{"new-first", "new-second"} {
		if _, err := os.Stat(filepath.Join(directory, jobID+".json")); err != nil {
			t.Fatalf("new conversation record %q stat error = %v, want retained file", jobID, err)
		}
	}
}

func TestMaybePurgeJobsBatchesSweeps(t *testing.T) {
	directory := t.TempDir()
	sweeps := 0
	for _, jobID := range []string{"job-0", "job-1", "job-2", "job-3", "job-4", "job-5"} {
		if err := writeAtomic(directory, Record{JobID: jobID, Status: harness.JobStatusDone}); err != nil {
			t.Fatal(err)
		}
		if maybePurgeJobs(context.Background(), directory, jobID, 3, 3) {
			sweeps++
		}
	}
	if sweeps != 2 {
		t.Fatalf("maybePurgeJobs() sweeps = %d, want 2 for 6 records", sweeps)
	}
	if records := jobRecordFiles(t, directory); len(records) != 3 {
		t.Fatalf("journal record count = %d, want 3 after batched sweep", len(records))
	}
}

func TestMaybePurgeJobsCoordinatesAcrossProcesses(t *testing.T) {
	if worker := os.Getenv("LANDING_JOURNAL_PURGE_HELPER"); worker != "" {
		waitForPurgeRelease(t, os.Getenv("LANDING_JOURNAL_PURGE_BARRIER"), worker)
		directory, err := paths.EnsureDirs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if record, err := ReadJob(context.Background(), "job-5"); err != nil || record == nil {
			t.Fatalf("ReadJob(job-5) before concurrent purge = %#v, %v; want retained record", record, err)
		}
		maybePurgeJobs(context.Background(), directory, "worker-"+worker, 4, 1)
		if record, err := ReadJob(context.Background(), "job-5"); err != nil || record == nil {
			t.Fatalf("ReadJob(job-5) after concurrent purge = %#v, %v; want retained record", record, err)
		}
		return
	}

	state := t.TempDir()
	barrier := t.TempDir()
	directory := filepath.Join(state, "jobs")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	for index := range 6 {
		jobID := fmt.Sprintf("job-%d", index)
		if err := writeAtomic(directory, Record{JobID: jobID, Status: harness.JobStatusDone}); err != nil {
			t.Fatal(err)
		}
		modified := base.Add(time.Duration(index) * time.Minute)
		if err := os.Chtimes(filepath.Join(directory, jobID+".json"), modified, modified); err != nil {
			t.Fatal(err)
		}
	}

	type workerProcess struct {
		command *exec.Cmd
		output  *bytes.Buffer
	}
	processes := make([]workerProcess, 0, 4)
	for worker := range 4 {
		command := exec.Command(os.Args[0], "-test.run=^TestMaybePurgeJobsCoordinatesAcrossProcesses$", "-test.v")
		output := &bytes.Buffer{}
		command.Stdout = output
		command.Stderr = output
		command.Env = append(os.Environ(), "LANDING_STATE_DIR="+state, "LANDING_JOURNAL_PURGE_HELPER="+fmt.Sprint(worker), "LANDING_JOURNAL_PURGE_BARRIER="+barrier)
		if err := command.Start(); err != nil {
			t.Fatalf("start purge worker %d: %v", worker, err)
		}
		processes = append(processes, workerProcess{command: command, output: output})
	}
	waitForPurgeWorkers(t, barrier, len(processes))
	if err := os.WriteFile(filepath.Join(barrier, "release"), nil, 0o600); err != nil {
		t.Fatalf("release purge workers: %v", err)
	}
	for worker, process := range processes {
		if err := process.command.Wait(); err != nil {
			t.Fatalf("purge worker %d returned error: %v\n%s", worker, err, process.output.String())
		}
	}
	if records := jobRecordFiles(t, directory); len(records) != 4 {
		t.Fatalf("journal record count after concurrent purge = %d, want 4", len(records))
	}
	for _, jobID := range []string{"job-2", "job-3", "job-4", "job-5"} {
		if _, err := os.Stat(filepath.Join(directory, jobID+".json")); err != nil {
			t.Fatalf("concurrent purge removed retained journal %q: %v", jobID, err)
		}
	}
}

func jobRecordFiles(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if isJobRecordFile(entry.Name()) {
			files = append(files, entry.Name())
		}
	}

	return files
}

func waitForPurgeRelease(t *testing.T, barrier string, worker string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(barrier, "ready-"+worker), nil, 0o600); err != nil {
		t.Fatalf("signal purge worker %s readiness: %v", worker, err)
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(barrier, "release")); err == nil {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-ticker.C:
		}
	}
}

func waitForPurgeWorkers(t *testing.T, barrier string, workers int) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		entries, err := os.ReadDir(barrier)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == workers {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-ticker.C:
		}
	}
}
