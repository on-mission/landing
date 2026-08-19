// Package journal persists completed thread metadata for continuation.
package journal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/paths"
)

const (
	maxJobRecords  = 1_000
	purgeBatchSize = 256
	purgeSignal    = ".retention-check"
)

type Record struct {
	JobID         string            `json:"jobId"`
	Provider      *string           `json:"provider"`
	Model         *string           `json:"model"`
	Role          *string           `json:"role"`
	Label         *string           `json:"label"`
	CWD           *string           `json:"cwd"`
	ThreadID      *string           `json:"threadId"`
	Status        harness.JobStatus `json:"status"`
	StartedAt     *time.Time        `json:"startedAt"`
	FinishedAt    *time.Time        `json:"finishedAt"`
	ExitCode      *int              `json:"exitCode"`
	RoutedBecause *string           `json:"routedBecause"`
}

func RecordJob(ctx context.Context, job harness.JobRecord) {
	RecordConversation(ctx, job.JobID, job)
}

// RecordConversation persists a terminal job under its stable conversation ID.
func RecordConversation(ctx context.Context, conversationID string, job harness.JobRecord) {
	// A live process cannot be a durable continuation record: if this process
	// dies before finalization, a persisted running status would be a lie.
	if !job.Status.IsTerminal() || !isSafeJobID(conversationID) {
		return
	}
	if ctx.Err() != nil {
		return
	}

	directory, err := paths.EnsureDirs(ctx)
	if err != nil {
		// Dispatch completed; failure to preserve its optional continuation is not fatal.
		return
	}
	_, err = os.Stat(filepath.Join(directory, conversationID+".json"))
	created := errors.Is(err, os.ErrNotExist)
	if err := writeAtomic(directory, jobRecord(conversationID, job)); err != nil {
		// Dispatch completed; failure to preserve its optional continuation is not fatal.
		return
	}
	if created {
		maybePurgeJobs(ctx, directory, conversationID, maxJobRecords, purgeBatchSize)
	}
}

func ReadJob(ctx context.Context, jobID string) (*Record, error) {
	if !isSafeJobID(jobID) {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	directory, err := paths.JobsDir()
	if err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(filepath.Join(directory, jobID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		// The journal is best-effort continuation metadata, so an unreadable record
		// has the same caller-visible result as an absent one.
		return nil, nil
	}

	var record Record
	if err := json.Unmarshal(contents, &record); err != nil {
		return nil, nil
	}
	if record.JobID == "" || record.Status == "" {
		return nil, nil
	}
	if _, err := harness.ParseJobStatus(string(record.Status)); err != nil {
		return nil, nil
	}

	return &record, nil
}

func maybePurgeJobs(ctx context.Context, directory string, jobID string, maximum int, batchSize int) bool {
	if batchSize < 1 || ctx.Err() != nil {
		return false
	}
	signalPath := filepath.Join(directory, purgeSignal)
	signal, err := os.OpenFile(signalPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	if _, err := signal.Write([]byte{'\n'}); err != nil {
		if closeErr := signal.Close(); closeErr != nil {
			return false
		}
		return false
	}
	if err := signal.Close(); err != nil {
		return false
	}
	info, err := os.Stat(signalPath)
	if err != nil || info.Size() < int64(batchSize) {
		return false
	}

	claimedSignal := filepath.Join(directory, ".retention-"+jobID)
	if err := os.Rename(signalPath, claimedSignal); err != nil {
		return false
	}
	defer func() {
		if err := os.Remove(claimedSignal); err != nil && !errors.Is(err, os.ErrNotExist) {
			// The signal only schedules a best-effort sweep; a leftover marker delays cleanup but never loses a record.
			return
		}
	}()
	purgeJobRecords(ctx, directory, maximum)

	return true
}

func purgeJobRecords(ctx context.Context, directory string, maximum int) {
	if maximum < 0 || ctx.Err() != nil {
		return
	}
	names, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	type recordFile struct {
		name     string
		modTime  time.Time
		info     os.FileInfo
		thread   string
		provider string
	}
	records := make([]recordFile, 0, len(names))
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return
		}
		if !isJobRecordFile(name.Name()) {
			continue
		}
		info, err := name.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		thread := ""
		provider := ""
		contents, err := os.ReadFile(filepath.Join(directory, name.Name()))
		if err == nil {
			var record Record
			if err := json.Unmarshal(contents, &record); err == nil {
				if record.ThreadID != nil {
					thread = *record.ThreadID
				}
				if record.Provider != nil {
					provider = *record.Provider
				}
			}
		}
		records = append(records, recordFile{name: name.Name(), modTime: info.ModTime(), info: info, thread: thread, provider: provider})
	}
	type conversation struct {
		files   []recordFile
		modTime time.Time
		name    string
	}
	conversations := make(map[string]conversation, len(records))
	for _, record := range records {
		key := "record:" + record.name
		if record.thread != "" {
			key = "thread:" + record.provider + "\x00" + record.thread
		}
		current, exists := conversations[key]
		if !exists {
			conversations[key] = conversation{files: []recordFile{record}, modTime: record.modTime, name: record.name}
			continue
		}
		current.files = append(current.files, record)
		if record.modTime.After(current.modTime) || record.modTime.Equal(current.modTime) && record.name < current.name {
			current.modTime = record.modTime
			current.name = record.name
		}
		conversations[key] = current
	}
	if len(conversations) <= maximum {
		return
	}
	ordered := make([]conversation, 0, len(conversations))
	for _, conversation := range conversations {
		ordered = append(ordered, conversation)
	}
	sort.Slice(ordered, func(left int, right int) bool {
		if ordered[left].modTime.Equal(ordered[right].modTime) {
			return ordered[left].name < ordered[right].name
		}

		return ordered[left].modTime.Before(ordered[right].modTime)
	})
	for _, conversation := range ordered[:len(ordered)-maximum] {
		for _, record := range conversation.files {
			if err := ctx.Err(); err != nil {
				return
			}
			path := filepath.Join(directory, record.name)
			current, err := os.Stat(path)
			if err != nil || !os.SameFile(record.info, current) {
				continue
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				continue
			}
		}
	}
}

func jobRecord(conversationID string, job harness.JobRecord) Record {
	return Record{
		JobID:         conversationID,
		Provider:      stringPointer(job.Provider),
		Model:         job.Model,
		Role:          job.Role,
		Label:         job.Label,
		CWD:           stringPointer(job.CWD),
		ThreadID:      job.ThreadID,
		Status:        job.Status,
		StartedAt:     timePointer(job.StartedAt),
		FinishedAt:    job.FinishedAt,
		ExitCode:      job.ExitCode,
		RoutedBecause: job.RoutedBecause,
	}
}

func writeAtomic(directory string, record Record) error {
	contents, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, record.JobID+".*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			// Journal writes are best effort; a leftover temporary file cannot fail a dispatch.
			return
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return closeAfterFailure(temporary, err)
	}
	if _, err := temporary.Write(contents); err != nil {
		return closeAfterFailure(temporary, err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, filepath.Join(directory, record.JobID+".json"))
}

func closeAfterFailure(file *os.File, operationErr error) error {
	if closeErr := file.Close(); closeErr != nil {
		return errors.Join(operationErr, closeErr)
	}

	return operationErr
}

func isSafeJobID(jobID string) bool {
	if jobID == "" {
		return false
	}
	for _, character := range jobID {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}

	return true
}

func isJobRecordFile(name string) bool {
	jobID, ok := strings.CutSuffix(name, ".json")
	return ok && isSafeJobID(jobID)
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}

	return &value
}
