package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/on-mission/landing/internal/paths"
)

const messagesSection = "## Landing messages\n" +
	"\n" +
	"Other chats can reach this one by name. They cannot see this conversation.\n" +
	"\n" +
	"When you start, background `landing messages monitor` using this harness's own backgrounding. Do not use shell `&`, and do not leave the command in the foreground.\n" +
	"\n" +
	"When that command exits, for any reason, read its output. Act on each message. Skip a message id you have already handled. Before any other work, background `landing messages monitor` again. A harness killing the command because it ran for a long time is still a reason to start it again.\n" +
	"\n" +
	"Send with `landing messages send --to <name> --message \"<text>\"`. The command returns as soon as the message is stored. A reply arrives later through the monitor.\n" +
	"\n" +
	"`landing messages who` lists names. `landing messages monitor --as <name>` claims a stable name.\n"

const messagesHeading = "## Landing messages"

type monitorRecord struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

type presentedRecord struct {
	Recipient string   `json:"recipient"`
	IDs       []string `json:"ids"`
}

func messagesConfigPath() (string, error) {
	root, err := paths.StateRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "messages.json"), nil
}

func messageStateDir(kind string) (string, error) {
	root, err := paths.StateRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "messages", kind), nil
}

func sessionFile(directory, session string) (string, error) {
	if session == "" || session != filepath.Base(session) || strings.Contains(session, "..") {
		return "", fmt.Errorf("message session key %q is not a file name", session)
	}

	return filepath.Join(directory, session), nil
}

func readMonitor(session string) (monitorRecord, bool, error) {
	directory, err := messageStateDir("monitors")
	if err != nil {
		return monitorRecord{}, false, err
	}
	path, err := sessionFile(directory, session)
	if err != nil {
		return monitorRecord{}, false, err
	}
	var record monitorRecord
	ok, err := readJSONFile(path, &record)
	if err != nil || !ok {
		return monitorRecord{}, ok, err
	}

	return record, true, nil
}

func writeMonitor(session string, record monitorRecord) error {
	directory, err := messageStateDir("monitors")
	if err != nil {
		return err
	}
	path, err := sessionFile(directory, session)
	if err != nil {
		return err
	}

	return writeJSONFile(path, record)
}

func readPresented(session string) (presentedRecord, bool, error) {
	directory, err := messageStateDir("presented")
	if err != nil {
		return presentedRecord{}, false, err
	}
	path, err := sessionFile(directory, session)
	if err != nil {
		return presentedRecord{}, false, err
	}
	var record presentedRecord
	ok, err := readJSONFile(path, &record)
	if err != nil || !ok {
		return presentedRecord{}, ok, err
	}

	return record, true, nil
}

func writePresented(session string, record presentedRecord) error {
	directory, err := messageStateDir("presented")
	if err != nil {
		return err
	}
	path, err := sessionFile(directory, session)
	if err != nil {
		return err
	}

	return writeJSONFile(path, record)
}

func removePresented(session string) error {
	directory, err := messageStateDir("presented")
	if err != nil {
		return err
	}
	path, err := sessionFile(directory, session)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove presented messages %s: %w", paths.Display(path), err)
	}

	return nil
}

func liveMonitorPIDs(name string) ([]int, error) {
	directory, err := messageStateDir("monitors")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read monitor records %s: %w", paths.Display(directory), err)
	}
	pids := make([]int, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var record monitorRecord
		ok, err := readJSONFile(filepath.Join(directory, entry.Name()), &record)
		if err != nil {
			return nil, err
		}
		if !ok || record.Name != name || !processAlive(record.PID) {
			continue
		}
		pids = append(pids, record.PID)
	}
	slices.Sort(pids)

	return pids, nil
}

// readJSONFile decodes one JSON file at the local-record boundary.
func readJSONFile(path string, value any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", paths.Display(path), err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return false, fmt.Errorf("read %s: %w", paths.Display(path), err)
	}

	return true, nil
}

func writeJSONFile(path string, value any) error {
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode %s: %w", paths.Display(path), err)
	}

	return writePrivateFile(path, payload.Bytes())
}

func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", paths.Display(filepath.Dir(path)), err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", paths.Display(temporary), err)
	}
	if err := os.Rename(temporary, path); err != nil {
		removeErr := os.Remove(temporary)
		if removeErr != nil {
			return fmt.Errorf("replace %s: %w", paths.Display(path), errors.Join(err, removeErr))
		}

		return fmt.Errorf("replace %s: %w", paths.Display(path), err)
	}

	return nil
}

func contextFilePaths(root string) []string {
	return []string{filepath.Join(root, "AGENTS.md"), filepath.Join(root, "CLAUDE.md")}
}

func editContextFiles(root string, stdout io.Writer, install bool) (int, error) {
	for _, path := range contextFilePaths(root) {
		changed, found, err := editContextFile(root, path, install)
		if err != nil {
			return exitFailed, err
		}
		if !found {
			if !install {
				continue
			}
			if _, err := fmt.Fprintf(stdout, "%s not found\n", path); err != nil {
				return exitFailed, err
			}
			continue
		}
		if !changed {
			continue
		}
		if _, err := fmt.Fprintf(stdout, "updated %s\n", path); err != nil {
			return exitFailed, err
		}
	}

	return exitOK, nil
}

func editContextFile(root, path string, install bool) (bool, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("stat %s: %w", paths.Display(path), err)
	}
	if info.IsDir() {
		return false, false, fmt.Errorf("%s is a directory", paths.Display(path))
	}
	if pathInHarnessConfig(root, path) {
		return false, true, fmt.Errorf("%s resolves under a harness configuration directory", paths.Display(path))
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return false, false, fmt.Errorf("read %s: %w", paths.Display(path), err)
	}
	original := string(contents)
	updated := removeMessagesSection(original)
	if install {
		updated = installMessagesSection(original)
	}
	if updated == original {
		return false, true, nil
	}
	if err := replaceFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return false, true, err
	}

	return true, true, nil
}

func pathInHarnessConfig(root, path string) bool {
	candidates := []string{path}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		candidates = append(candidates, resolved)
	}
	for _, candidate := range candidates {
		relative, err := filepath.Rel(root, candidate)
		if err != nil {
			if harnessConfigSegment(candidate) {
				return true
			}
			continue
		}
		if harnessConfigSegment(relative) {
			return true
		}
	}

	return false
}

func harnessConfigSegment(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".claude" || part == ".codex" || part == ".grok" {
			return true
		}
	}

	return false
}

func replaceFile(path string, data []byte, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	temporary := path + ".landing-tmp"
	if err := os.WriteFile(temporary, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", paths.Display(temporary), err)
	}
	if err := os.Rename(temporary, path); err != nil {
		removeErr := os.Remove(temporary)
		if removeErr != nil {
			return fmt.Errorf("replace %s: %w", paths.Display(path), errors.Join(err, removeErr))
		}

		return fmt.Errorf("replace %s: %w", paths.Display(path), err)
	}

	return nil
}

func installMessagesSection(contents string) string {
	start, end, found := messagesSectionBounds(contents)
	if !found {
		return appendMessagesSection(contents)
	}
	updated := contents[:start] + messagesSection + contents[end:]
	tail := start + len(messagesSection)

	return updated[:tail] + removeMessagesSection(updated[tail:])
}

func appendMessagesSection(contents string) string {
	if contents == "" {
		return messagesSection
	}
	if !strings.HasSuffix(contents, "\n") {
		contents += "\n"
	}
	if !strings.HasSuffix(contents, "\n\n") {
		contents += "\n"
	}

	return contents + messagesSection
}

func removeMessagesSection(contents string) string {
	for {
		start, end, ok := messagesSectionBounds(contents)
		if !ok {
			return contents
		}
		contents = contents[:start] + contents[end:]
	}
}

func messagesSectionBounds(contents string) (int, int, bool) {
	offset := 0
	for offset <= len(contents) {
		line, next, ok := nextContentLine(contents, offset)
		if !ok {
			return 0, 0, false
		}
		if line == messagesHeading {
			scan := next
			for {
				following, followingNext, followingOK := nextContentLine(contents, scan)
				if !followingOK {
					return offset, len(contents), true
				}
				if sectionBoundary(following) {
					return offset, scan, true
				}
				scan = followingNext
			}
		}
		if next <= offset {
			return 0, 0, false
		}
		offset = next
	}

	return 0, 0, false
}

func sectionBoundary(line string) bool {
	if strings.HasPrefix(line, "###") {
		return false
	}

	return strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ")
}

func nextContentLine(contents string, offset int) (string, int, bool) {
	if offset >= len(contents) {
		return "", offset, false
	}
	end := strings.IndexByte(contents[offset:], '\n')
	if end < 0 {
		return strings.TrimRight(contents[offset:], "\r"), len(contents), true
	}
	line := strings.TrimRight(contents[offset:offset+end], "\r")

	return line, offset + end + 1, true
}
