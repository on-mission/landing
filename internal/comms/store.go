package comms

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/on-mission/landing/internal/paths"
)

const (
	stateVersion         = 1
	historyRetention     = 30 * 24 * time.Hour
	pollInterval         = 10 * time.Millisecond
	responsePollInterval = 250 * time.Millisecond
)

// fileStore uses an advisory OS lock and atomic replacement of state.json. The
// lock serializes every process using one project store; this favors correctness
// over parallel writes, while an interrupted write leaves only an ignored temp file.
type fileStore struct {
	directory string
}

type storeState struct {
	Version       int                    `json:"version"`
	Participants  map[string]Participant `json:"participants"`
	ThreadPIDs    map[string]int         `json:"threadPids,omitempty"`
	ThreadAliases map[string]string      `json:"threadAliases,omitempty"`
	Messages      []storedMessage        `json:"messages"`
}

type storedMessage struct {
	Message    Message              `json:"message"`
	Deliveries map[string]time.Time `json:"deliveries"`
}

// Open returns the store for the project containing cwd.
func Open(ctx context.Context, cwd string) (Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	project, err := projectDirectory(ctx, cwd)
	if err != nil {
		return nil, err
	}
	stateRoot, err := paths.StateRoot()
	if err != nil {
		return nil, fmt.Errorf("resolve comms state root: %w", err)
	}
	projectKey := sha256.Sum256([]byte(project))
	directory := filepath.Join(stateRoot, "comms", hex.EncodeToString(projectKey[:]))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create comms state directory %q: %w", directory, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &fileStore{directory: directory}, nil
}

func (store *fileStore) Register(ctx context.Context, participant Participant) error {
	if err := validateParticipant(participant); err != nil {
		return err
	}

	return store.mutate(ctx, func(state *storeState) error {
		if participant.Kind == KindSession {
			if threadName := activeThreadNamedForPID(*state, participant.PID); threadName != "" {
				if state.ThreadAliases == nil {
					state.ThreadAliases = make(map[string]string)
				}
				state.ThreadAliases[participant.Name] = threadName
				return nil
			}
			delete(state.ThreadAliases, participant.Name)
		}
		state.Participants[participant.Name] = storedParticipant(state, participant)
		return nil
	})
}

func (store *fileStore) RegisterUnique(ctx context.Context, participant Participant) (string, error) {
	if err := validateParticipant(participant); err != nil {
		return "", err
	}

	name := ""
	err := store.mutate(ctx, func(state *storeState) error {
		name = uniqueParticipantName(state.Participants, state.ThreadAliases, participant.Name)
		participant.Name = name
		state.Participants[name] = storedParticipant(state, participant)
		mergeSessionIntoThread(state, participant)
		return nil
	})
	if err != nil {
		return "", err
	}

	return name, nil
}

func (store *fileStore) Deregister(ctx context.Context, name string) error {
	if name == "" {
		return errors.New("deregister participant: name is empty")
	}

	return store.mutate(ctx, func(state *storeState) error {
		delete(state.Participants, name)
		delete(state.ThreadPIDs, name)
		delete(state.ThreadAliases, name)
		removeThreadAliases(state, name)
		return nil
	})
}

func (store *fileStore) Rename(ctx context.Context, from string, to string) error {
	if from == "" || to == "" {
		return errors.New("rename participant: name is empty")
	}
	if to == AllAgents {
		return fmt.Errorf("rename participant: %q is reserved", AllAgents)
	}

	return store.mutate(ctx, func(state *storeState) error {
		participant, ok := state.Participants[from]
		if !ok {
			return fmt.Errorf("rename participant %q: not registered", from)
		}
		if _, exists := state.Participants[to]; exists || state.ThreadAliases[to] != "" {
			return fmt.Errorf("rename participant %q: already registered", to)
		}

		delete(state.Participants, from)
		participant.Name = to
		state.Participants[to] = participant
		if pid, exists := state.ThreadPIDs[from]; exists {
			delete(state.ThreadPIDs, from)
			state.ThreadPIDs[to] = pid
		}
		for alias, threadName := range state.ThreadAliases {
			if threadName == from {
				state.ThreadAliases[alias] = to
			}
		}
		for index := range state.Messages {
			message := &state.Messages[index]
			if message.Message.From == from {
				message.Message.From = to
			}
			if message.Message.To == from {
				message.Message.To = to
			}
			if deliveredAt, exists := message.Deliveries[from]; exists {
				delete(message.Deliveries, from)
				message.Deliveries[to] = deliveredAt
			}
		}

		return nil
	})
}

func (store *fileStore) ReportActivity(ctx context.Context, name string, activity Activity) error {
	if name == "" {
		return errors.New("report activity: name is empty")
	}
	if !validState(activity.State) {
		return fmt.Errorf("report activity for %q: invalid state %q", name, activity.State)
	}

	return store.mutate(ctx, func(state *storeState) error {
		name = participantName(*state, name)
		participant, ok := state.Participants[name]
		if !ok {
			return fmt.Errorf("report activity for %q: not registered", name)
		}
		participant.Activity = activity
		state.Participants[name] = participant
		return nil
	})
}

func (store *fileStore) Participants(ctx context.Context) ([]Participant, error) {
	participants := make([]Participant, 0)
	err := store.read(ctx, func(state storeState) error {
		participants = make([]Participant, 0, len(state.Participants))
		for _, participant := range state.Participants {
			if participantProcessEnded(state, participant) {
				participant.Activity.State = StateEnded
			}
			if participant.Kind == KindThread {
				participant.PID = 0
			}
			participants = append(participants, participant)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(participants, func(left int, right int) bool {
		return participants[left].Name < participants[right].Name
	})

	return participants, nil
}

func (store *fileStore) Send(ctx context.Context, message Message) (Message, error) {
	if err := validateMessage(message); err != nil {
		return Message{}, err
	}
	identifier, err := newMessageID()
	if err != nil {
		return Message{}, err
	}
	message.ID = identifier
	message.SentAt = time.Now().UTC()
	message.DeliveredAt = nil

	err = store.mutate(ctx, func(state *storeState) error {
		message.From = participantName(*state, message.From)
		if !message.Broadcast() {
			message.To = participantName(*state, message.To)
		}
		deliveries := make(map[string]time.Time)
		if message.Broadcast() {
			for name := range state.Participants {
				if name != message.From {
					deliveries[name] = time.Time{}
				}
			}
		} else {
			deliveries[message.To] = time.Time{}
		}
		state.Messages = append(state.Messages, storedMessage{
			Message:    message,
			Deliveries: deliveries,
		})
		pruneHistory(state, message.SentAt)
		return nil
	})
	if err != nil {
		return Message{}, err
	}

	return message, nil
}

func (store *fileStore) Pending(ctx context.Context, recipient string) ([]Message, error) {
	if recipient == "" {
		return nil, errors.New("pending messages: recipient is empty")
	}

	pending := make([]Message, 0)
	err := store.read(ctx, func(state storeState) error {
		recipient = participantName(state, recipient)
		for _, stored := range state.Messages {
			deliveredAt, addressed := stored.Deliveries[recipient]
			if !addressed || !deliveredAt.IsZero() {
				continue
			}
			pending = append(pending, messageForRecipient(stored, recipient))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortMessages(pending)

	return pending, nil
}

func (store *fileStore) MarkDelivered(ctx context.Context, recipient string, ids []string) error {
	if recipient == "" {
		return errors.New("mark delivered: recipient is empty")
	}
	if len(ids) == 0 {
		return nil
	}
	identifiers := make(map[string]struct{}, len(ids))
	for _, identifier := range ids {
		identifiers[identifier] = struct{}{}
	}

	return store.mutate(ctx, func(state *storeState) error {
		recipient = participantName(*state, recipient)
		deliveredAt := time.Now().UTC()
		for index := range state.Messages {
			message := &state.Messages[index]
			if _, requested := identifiers[message.Message.ID]; !requested {
				continue
			}
			if _, addressed := message.Deliveries[recipient]; addressed {
				message.Deliveries[recipient] = deliveredAt
			}
		}
		pruneHistory(state, deliveredAt)
		return nil
	})
}

func (store *fileStore) History(ctx context.Context, window time.Duration) ([]Message, error) {
	if window < 0 {
		return nil, fmt.Errorf("history: window %s is negative", window)
	}
	now := time.Now()
	cutoff := now.Add(-window)
	retentionCutoff := now.Add(-historyRetention)
	if cutoff.Before(retentionCutoff) {
		cutoff = retentionCutoff
	}
	history := make([]Message, 0)
	err := store.read(ctx, func(state storeState) error {
		for _, stored := range state.Messages {
			if stored.Message.SentAt.Before(cutoff) {
				continue
			}
			history = append(history, historyMessage(stored))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortMessages(history)

	return history, nil
}

func (store *fileStore) AwaitResponse(ctx context.Context, id string, bound time.Duration) (Message, bool, error) {
	if id == "" {
		return Message{}, false, errors.New("await response: message ID is empty")
	}
	if bound < 0 {
		return Message{}, false, fmt.Errorf("await response: bound %s is negative", bound)
	}
	deadline := time.Now().Add(bound)
	for {
		response, ok, err := store.response(ctx, id)
		if err != nil {
			return Message{}, false, err
		}
		if ok {
			return response, true, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Message{}, false, nil
		}
		if err := waitForResponse(ctx, minDuration(remaining, responsePollInterval)); err != nil {
			return Message{}, false, err
		}
	}
}

func (store *fileStore) response(ctx context.Context, id string) (Message, bool, error) {
	var response Message
	found := false
	err := store.read(ctx, func(state storeState) error {
		for _, stored := range state.Messages {
			if stored.Message.InResponseTo != id {
				continue
			}
			if !found || stored.Message.SentAt.Before(response.SentAt) {
				response = historyMessage(stored)
				found = true
			}
		}
		return nil
	})
	if err != nil {
		return Message{}, false, err
	}

	return response, found, nil
}

func (store *fileStore) read(ctx context.Context, read func(storeState) error) error {
	lock, err := acquireStateLock(ctx, filepath.Join(store.directory, "lock"), false)
	if err != nil {
		return err
	}
	state, readErr := loadState(store.directory)
	if readErr == nil {
		readErr = read(state)
	}
	closeErr := lock.Close()
	return errors.Join(readErr, closeErr)
}

func (store *fileStore) mutate(ctx context.Context, mutate func(*storeState) error) error {
	lock, err := acquireStateLock(ctx, filepath.Join(store.directory, "lock"), true)
	if err != nil {
		return err
	}
	state, mutateErr := loadState(store.directory)
	if mutateErr == nil {
		mutateErr = mutate(&state)
	}
	if mutateErr == nil {
		mutateErr = writeState(ctx, store.directory, state)
	}
	closeErr := lock.Close()
	return errors.Join(mutateErr, closeErr)
}

func projectDirectory(ctx context.Context, cwd string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve comms working directory %q: %w", cwd, err)
	}
	resolvedCWD, err := filepath.EvalSymlinks(absCWD)
	if err != nil {
		return "", fmt.Errorf("resolve comms working directory %q: %w", cwd, err)
	}
	for directory := resolvedCWD; ; directory = filepath.Dir(directory) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		info, err := os.Stat(filepath.Join(directory, ".landing"))
		if err == nil && info.IsDir() {
			return directory, nil
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("inspect comms project directory %q: %w", directory, err)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("find comms project directory from %q: no .landing directory", resolvedCWD)
		}
	}
}

func loadState(directory string) (storeState, error) {
	path := filepath.Join(directory, "state.json")
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return storeState{}, fmt.Errorf("open comms state %q: %w", path, err)
	}

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var state storeState
	decodeErr := decoder.Decode(&state)
	if decodeErr == nil {
		decodeErr = ensureEOF(decoder)
	}
	closeErr := file.Close()
	if decodeErr != nil {
		return storeState{}, fmt.Errorf("decode comms state %q: %w", path, errors.Join(decodeErr, closeErr))
	}
	if closeErr != nil {
		return storeState{}, fmt.Errorf("close comms state %q: %w", path, closeErr)
	}
	normalizeThreadPIDs(&state)
	if err := validateState(state); err != nil {
		return storeState{}, fmt.Errorf("validate comms state %q: %w", path, err)
	}
	return state, nil
}

func emptyState() storeState {
	return storeState{
		Version:       stateVersion,
		Participants:  make(map[string]Participant),
		ThreadPIDs:    make(map[string]int),
		ThreadAliases: make(map[string]string),
		Messages:      make([]storedMessage, 0),
	}
}

func writeState(ctx context.Context, directory string, state storeState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".state-")
	if err != nil {
		return fmt.Errorf("create temporary comms state in %q: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	if err := encodeState(temporary, state); err != nil {
		return removeTemporaryState(temporary, temporaryPath, err)
	}
	if err := temporary.Chmod(0o600); err != nil {
		return removeTemporaryState(temporary, temporaryPath, fmt.Errorf("set comms state permissions %q: %w", temporaryPath, err))
	}
	if err := temporary.Sync(); err != nil {
		return removeTemporaryState(temporary, temporaryPath, fmt.Errorf("sync temporary comms state %q: %w", temporaryPath, err))
	}
	if err := temporary.Close(); err != nil {
		return removeTemporaryState(nil, temporaryPath, fmt.Errorf("close temporary comms state %q: %w", temporaryPath, err))
	}
	if err := ctx.Err(); err != nil {
		return removeTemporaryState(nil, temporaryPath, err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(directory, "state.json")); err != nil {
		return removeTemporaryState(nil, temporaryPath, fmt.Errorf("replace comms state in %q: %w", directory, err))
	}

	return nil
}

func encodeState(file *os.File, state storeState) error {
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("encode temporary comms state %q: %w", file.Name(), err)
	}

	return nil
}

func removeTemporaryState(file *os.File, path string, cause error) error {
	if file != nil {
		if err := file.Close(); err != nil && !errors.Is(err, fs.ErrInvalid) {
			cause = errors.Join(cause, fmt.Errorf("close temporary comms state %q: %w", path, err))
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		cause = errors.Join(cause, fmt.Errorf("remove temporary comms state %q: %w", path, err))
	}

	return cause
}

func validateParticipant(participant Participant) error {
	if participant.Name == "" {
		return errors.New("register participant: name is empty")
	}
	if participant.Name == AllAgents {
		return fmt.Errorf("register participant: %q is reserved", AllAgents)
	}
	if participant.Kind != KindSession && participant.Kind != KindThread {
		return fmt.Errorf("register participant %q: invalid kind %q", participant.Name, participant.Kind)
	}
	if !validState(participant.Activity.State) {
		return fmt.Errorf("register participant %q: invalid state %q", participant.Name, participant.Activity.State)
	}

	return nil
}

func validateMessage(message Message) error {
	if message.From == "" {
		return errors.New("send message: sender is empty")
	}
	if message.To == "" {
		return errors.New("send message: recipient is empty")
	}
	if message.To == AllAgents && message.From == AllAgents {
		return errors.New("send message: all cannot send a broadcast")
	}

	return nil
}

func validateState(state storeState) error {
	if state.Version != stateVersion {
		return fmt.Errorf("unsupported version %d", state.Version)
	}
	if state.Participants == nil {
		return errors.New("participants is null")
	}
	if state.Messages == nil {
		return errors.New("messages is null")
	}
	for name, participant := range state.Participants {
		if name != participant.Name {
			return fmt.Errorf("participant key %q does not match name %q", name, participant.Name)
		}
		if err := validateParticipant(participant); err != nil {
			return err
		}
	}
	for name, pid := range state.ThreadPIDs {
		participant, exists := state.Participants[name]
		if !exists || participant.Kind != KindThread {
			return fmt.Errorf("thread pid %d has no thread participant %q", pid, name)
		}
		if pid <= 0 {
			return fmt.Errorf("thread participant %q has invalid pid %d", name, pid)
		}
	}
	for alias, name := range state.ThreadAliases {
		participant, exists := state.Participants[name]
		if alias == "" || !exists || participant.Kind != KindThread {
			return fmt.Errorf("thread alias %q has no thread participant %q", alias, name)
		}
	}
	identifiers := make(map[string]struct{}, len(state.Messages))
	for _, stored := range state.Messages {
		if stored.Message.ID == "" {
			return errors.New("message ID is empty")
		}
		if _, exists := identifiers[stored.Message.ID]; exists {
			return fmt.Errorf("message ID %q is duplicated", stored.Message.ID)
		}
		identifiers[stored.Message.ID] = struct{}{}
		if err := validateMessage(stored.Message); err != nil {
			return err
		}
		if stored.Deliveries == nil {
			return fmt.Errorf("message %q deliveries is null", stored.Message.ID)
		}
	}

	return nil
}

func validState(state State) bool {
	return state == StateInTurn || state == StateIdle || state == StateEnded
}

func participantProcessEnded(state storeState, participant Participant) bool {
	if participant.Kind == KindSession {
		return !processAlive(participant.PID)
	}
	if participant.Kind != KindThread || participant.Activity.State != StateInTurn {
		return false
	}

	return !processAlive(state.ThreadPIDs[participant.Name])
}

func uniqueParticipantName(participants map[string]Participant, aliases map[string]string, preferred string) string {
	if _, exists := participants[preferred]; !exists && aliases[preferred] == "" {
		return preferred
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", preferred, suffix)
		if _, exists := participants[candidate]; !exists && aliases[candidate] == "" {
			return candidate
		}
	}
}

func storeThreadPID(state *storeState, participant Participant) {
	delete(state.ThreadPIDs, participant.Name)
	if participant.Kind != KindThread || participant.Activity.State != StateInTurn || participant.PID <= 0 {
		return
	}
	if state.ThreadPIDs == nil {
		state.ThreadPIDs = make(map[string]int)
	}
	state.ThreadPIDs[participant.Name] = participant.PID
}

func normalizeThreadPIDs(state *storeState) {
	if state.ThreadPIDs == nil {
		state.ThreadPIDs = make(map[string]int)
	}
	for name, participant := range state.Participants {
		if participant.Kind != KindThread {
			continue
		}
		if participant.Activity.State == StateInTurn && participant.PID > 0 && state.ThreadPIDs[name] == 0 {
			state.ThreadPIDs[name] = participant.PID
		}
		participant.PID = 0
		state.Participants[name] = participant
	}
}

func storedParticipant(state *storeState, participant Participant) Participant {
	storeThreadPID(state, participant)
	if participant.Kind != KindThread {
		return participant
	}
	if participant.Activity.State != StateInTurn {
		removeThreadAliases(state, participant.Name)
	}
	participant.PID = 0
	return participant
}

func mergeSessionIntoThread(state *storeState, thread Participant) {
	if thread.Kind != KindThread || thread.PID <= 0 {
		return
	}
	for name, participant := range state.Participants {
		if participant.Kind != KindSession || participant.PID != thread.PID {
			continue
		}
		delete(state.Participants, name)
		if state.ThreadAliases == nil {
			state.ThreadAliases = make(map[string]string)
		}
		state.ThreadAliases[name] = thread.Name
		for index := range state.Messages {
			message := &state.Messages[index]
			if message.Message.From == name {
				message.Message.From = thread.Name
			}
			if message.Message.To == name {
				message.Message.To = thread.Name
			}
			if deliveredAt, exists := message.Deliveries[name]; exists {
				delete(message.Deliveries, name)
				message.Deliveries[thread.Name] = deliveredAt
			}
		}
	}
}

func removeThreadAliases(state *storeState, name string) {
	for alias, threadName := range state.ThreadAliases {
		if threadName == name {
			delete(state.ThreadAliases, alias)
		}
	}
}

func activeThreadNamedForPID(state storeState, pid int) string {
	if pid <= 0 {
		return ""
	}
	for name, threadPID := range state.ThreadPIDs {
		participant, exists := state.Participants[name]
		if !exists || participant.Kind != KindThread || participant.Activity.State != StateInTurn || threadPID != pid || !processAlive(pid) {
			continue
		}
		return name
	}
	return ""
}

func participantName(state storeState, name string) string {
	if threadName, exists := state.ThreadAliases[name]; exists {
		return threadName
	}
	return name
}

func pruneHistory(state *storeState, now time.Time) {
	cutoff := now.Add(-historyRetention)
	retained := make([]storedMessage, 0, len(state.Messages))
	for _, message := range state.Messages {
		if !message.Message.SentAt.Before(cutoff) {
			retained = append(retained, message)
		}
	}
	state.Messages = retained
}

func messageForRecipient(stored storedMessage, recipient string) Message {
	message := stored.Message
	deliveredAt := stored.Deliveries[recipient]
	if deliveredAt.IsZero() {
		message.DeliveredAt = nil
	} else {
		message.DeliveredAt = &deliveredAt
	}

	return message
}

func historyMessage(stored storedMessage) Message {
	message := stored.Message
	if !message.Broadcast() {
		return messageForRecipient(stored, message.To)
	}
	message.DeliveredAt = nil

	return message
}

func sortMessages(messages []Message) {
	sort.Slice(messages, func(left int, right int) bool {
		if messages[left].SentAt.Equal(messages[right].SentAt) {
			return messages[left].ID < messages[right].ID
		}

		return messages[left].SentAt.Before(messages[right].SentAt)
	})
}

func newMessageID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("create message ID: %w", err)
	}

	return hex.EncodeToString(bytes), nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra struct{}
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}

	return errors.New("contains an additional JSON value")
}

func waitForResponse(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func minDuration(left time.Duration, right time.Duration) time.Duration {
	if left < right {
		return left
	}

	return right
}
