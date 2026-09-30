package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var (
	fakeMessagesProviderOnce sync.Once
	fakeMessagesProviderBin  string
	fakeMessagesProviderErr  error
)

func TestMessagesCommandsSpeakOneJSONObject(t *testing.T) {
	// Bug class: a command sends the wrong op, frames more than one JSON
	// object, or drops the inbox/monitor block distinction.
	tests := []struct {
		name  string
		args  []string
		setup func(*testing.T, string)
		check func(*testing.T, []map[string]any, string)
	}{
		{
			name: "send",
			args: []string{"messages", "send", "--to", "office", "--message", "the migration is on main", "--as", "laptop"},
			check: func(t *testing.T, requests []map[string]any, stdout string) {
				if len(requests) != 1 {
					t.Fatalf("send provider calls = %d, want 1", len(requests))
				}
				request := requests[0]
				if got := requestString(request, "op"); got != "send" {
					t.Fatalf("send op = %q, want %q", got, "send")
				}
				if requestString(request, "from") != "laptop" || requestString(request, "to") != "office" || requestString(request, "body") != "the migration is on main" {
					t.Fatalf("send request = %#v, want laptop -> office body", request)
				}
				id := requestString(request, "id")
				if id == "" || requestString(request, "sentAt") == "" {
					t.Fatalf("send request = %#v, want id and sentAt", request)
				}
				if _, ok := request["block"]; ok {
					t.Fatalf("send request included block: %#v", request)
				}
				want := "sent " + id + " to office\n"
				if stdout != want {
					t.Fatalf("send stdout = %q, want %q", stdout, want)
				}
			},
		},
		{
			name: "blocking wait",
			args: []string{"messages", "monitor", "--as", "office"},
			check: func(t *testing.T, requests []map[string]any, stdout string) {
				if len(requests) != 1 {
					t.Fatalf("monitor provider calls = %d, want 1", len(requests))
				}
				request := requests[0]
				if got := requestString(request, "op"); got != "wait" {
					t.Fatalf("monitor op = %q, want %q", got, "wait")
				}
				if requestString(request, "recipient") != "office" {
					t.Fatalf("monitor recipient = %q, want %q", requestString(request, "recipient"), "office")
				}
				if _, ok := request["block"]; ok {
					t.Fatalf("monitor wait included block: %#v", request)
				}
				if !strings.Contains(stdout, "message 01TESTMESSAGE000000000000 from laptop at 2026-09-29T22:14:03Z") {
					t.Fatalf("monitor stdout = %q, want presented message", stdout)
				}
			},
		},
		{
			name: "ack",
			args: []string{"messages", "monitor", "--as", "office"},
			setup: func(t *testing.T, _ string) {
				chat := testChat(t, "office")
				if err := writePresented(chat.session, presentedRecord{Recipient: "office", IDs: []string{"01TESTACK0000000000000000"}}); err != nil {
					t.Fatalf("writePresented() returned unexpected error: %v", err)
				}
			},
			check: func(t *testing.T, requests []map[string]any, _ string) {
				if len(requests) != 2 {
					t.Fatalf("monitor with presented ids provider calls = %d, want 2", len(requests))
				}
				ack := requests[0]
				if requestString(ack, "op") != "ack" || requestString(ack, "recipient") != "office" {
					t.Fatalf("ack request = %#v, want ack for office", ack)
				}
				ids, _ := ack["ids"].([]any)
				if len(ids) != 1 || ids[0] != "01TESTACK0000000000000000" {
					t.Fatalf("ack ids = %#v, want presented id", ids)
				}
				wait := requests[1]
				if requestString(wait, "op") != "wait" {
					t.Fatalf("request after ack op = %q, want wait", requestString(wait, "op"))
				}
				if _, ok := wait["block"]; ok {
					t.Fatalf("wait after ack included block: %#v", wait)
				}
			},
		},
		{
			name: "list",
			args: []string{"messages", "who"},
			check: func(t *testing.T, requests []map[string]any, stdout string) {
				if len(requests) != 1 {
					t.Fatalf("who provider calls = %d, want 1", len(requests))
				}
				request := requests[0]
				if requestString(request, "op") != "list" {
					t.Fatalf("who op = %q, want %q", requestString(request, "op"), "list")
				}
				if len(request) != 1 {
					t.Fatalf("list request = %#v, want only op", request)
				}
				if stdout != "office\n" {
					t.Fatalf("who stdout = %q, want %q", stdout, "office\n")
				}
			},
		},
		{
			name: "inbox",
			args: []string{"messages", "inbox", "--as", "office"},
			setup: func(t *testing.T, providerDir string) {
				writeProviderFile(t, providerDir, "wait.json", `{"messages":[]}`+"\n")
			},
			check: func(t *testing.T, requests []map[string]any, stdout string) {
				if len(requests) != 1 {
					t.Fatalf("inbox provider calls = %d, want 1", len(requests))
				}
				request := requests[0]
				if requestString(request, "op") != "wait" || requestString(request, "recipient") != "office" {
					t.Fatalf("inbox request = %#v, want wait for office", request)
				}
				block, ok := request["block"].(bool)
				if !ok || block {
					t.Fatalf("inbox wait block = %#v, want false", request["block"])
				}
				if stdout != "inbox empty\n" {
					t.Fatalf("inbox stdout = %q, want %q", stdout, "inbox empty\n")
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project, providerDir := configureMessages(t)
			if test.setup != nil {
				test.setup(t, providerDir)
			}
			code, err, stdout, stderr := runCLI(t, project, test.args)
			if err != nil || code != exitOK {
				t.Fatalf("Run(%q) = %d, %v, stderr %q; want %d, nil", test.args, code, err, stderr.String(), exitOK)
			}
			test.check(t, recordedRequests(t, providerDir), stdout.String())
		})
	}
}

func TestMessagesSecondMonitorNamesLivePID(t *testing.T) {
	// Bug class: a second monitor for a live session is allowed, or the error omits the pid.
	project, _ := configureMessages(t)
	chat := testChat(t, "office")
	pid := startPinnedProcess(t)
	if err := writeMonitor(chat.session, monitorRecord{PID: pid, Name: chat.name}); err != nil {
		t.Fatalf("writeMonitor() returned unexpected error: %v", err)
	}
	code, err, stdout, _ := runCLI(t, project, []string{"messages", "monitor", "--as", "office"})
	if code != exitFailed || err == nil {
		t.Fatalf("Run(messages monitor) = %d, %v, stdout %q; want %d, live-pid error", code, err, stdout.String(), exitFailed)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(pid)) {
		t.Fatalf("second monitor error = %q, want live pid %d", err, pid)
	}
	if stdout.String() != "" {
		t.Fatalf("second monitor stdout = %q, want empty", stdout.String())
	}
}

func TestMessagesMonitorReplacesRecordedPIDThatIsNotRunning(t *testing.T) {
	// Bug class: a stale monitor pid is treated as live and blocks a new monitor.
	project, _ := configureMessages(t)
	chat := testChat(t, "office")
	dead := deadPID(t)
	if err := writeMonitor(chat.session, monitorRecord{PID: dead, Name: chat.name}); err != nil {
		t.Fatalf("writeMonitor() returned unexpected error: %v", err)
	}
	code, err, stdout, stderr := runCLI(t, project, []string{"messages", "monitor", "--as", "office"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(messages monitor) = %d, %v, stderr %q; want %d, nil", code, err, stderr.String(), exitOK)
	}
	if !strings.Contains(stdout.String(), "message 01TESTMESSAGE000000000000 from laptop") {
		t.Fatalf("monitor stdout = %q, want presented message", stdout.String())
	}
	record, ok, readErr := readMonitor(chat.session)
	if readErr != nil || !ok || record.PID != os.Getpid() || record.Name != "office" {
		t.Fatalf("monitor record = %#v, ok %t, %v; want pid %d name office", record, ok, readErr, os.Getpid())
	}
}

func TestMessagesMonitorRetriesProviderFailureOffStdout(t *testing.T) {
	// Bug class: a provider error wakes the chat on stdout, or is not retried.
	project, providerDir := configureMessages(t)
	writeProviderFile(t, providerDir, "failures_left", "1\n")
	writeProviderFile(t, providerDir, "stderr.txt", "temporary provider outage\n")
	code, err, stdout, stderr := runCLI(t, project, []string{"messages", "monitor", "--as", "office"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(messages monitor) = %d, %v, stderr %q; want %d, nil after retry", code, err, stderr.String(), exitOK)
	}
	if strings.Contains(stdout.String(), "temporary provider outage") || strings.Contains(stdout.String(), "provider failed") {
		t.Fatalf("monitor stdout = %q, want provider failure kept off stdout", stdout.String())
	}
	if !strings.Contains(stdout.String(), "message 01TESTMESSAGE000000000000 from laptop") {
		t.Fatalf("monitor stdout = %q, want presented message after retry", stdout.String())
	}
	requests := recordedRequests(t, providerDir)
	if len(requests) != 2 {
		t.Fatalf("monitor provider calls = %d, want 2 (one failure, one retry)", len(requests))
	}
	for index, request := range requests {
		if requestString(request, "op") != "wait" {
			t.Fatalf("retry request[%d] = %#v, want wait", index, request)
		}
		if _, ok := request["block"]; ok {
			t.Fatalf("retry request[%d] included block: %#v", index, request)
		}
	}
}

func TestMessagesInstallWritesExistingContextFilesOnly(t *testing.T) {
	// Bug class: install creates a missing context file or writes harness config dirs.
	t.Setenv("LANDING_STATE_DIR", t.TempDir())
	project := t.TempDir()
	agents := filepath.Join(project, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("# Project\n\nIntro.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	canaries := map[string]string{
		filepath.Join(project, ".claude", "CLAUDE.md"): "claude canary",
		filepath.Join(project, ".codex", "AGENTS.md"):  "codex canary",
		filepath.Join(project, ".grok", "AGENTS.md"):   "grok canary",
	}
	for path, contents := range canaries {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	code, err, stdout, stderr := runCLI(t, project, []string{"messages", "install"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(messages install) = %d, %v, stderr %q; want %d, nil", code, err, stderr.String(), exitOK)
	}
	if !strings.Contains(stdout.String(), "updated "+agents) {
		t.Fatalf("install stdout = %q, want updated AGENTS.md", stdout.String())
	}
	claude := filepath.Join(project, "CLAUDE.md")
	if !strings.Contains(stdout.String(), claude+" not found") {
		t.Fatalf("install stdout = %q, want missing CLAUDE.md reported", stdout.String())
	}
	if _, statErr := os.Stat(claude); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(CLAUDE.md) = %v, want not exist", statErr)
	}
	contents, readErr := os.ReadFile(agents)
	if readErr != nil || !strings.Contains(string(contents), messagesHeading) || !strings.Contains(string(contents), "Intro.") {
		t.Fatalf("AGENTS.md = %q, %v; want standing section and original intro", contents, readErr)
	}
	for path, want := range canaries {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != want {
			t.Fatalf("harness file %s = %q, %v; want unchanged %q", path, got, readErr, want)
		}
	}
}

func TestMessagesInstallReplacesStandingSectionAndLeavesTheRest(t *testing.T) {
	// Bug class: a second install duplicates the section or rewrites surrounding copy.
	t.Setenv("LANDING_STATE_DIR", t.TempDir())
	project := t.TempDir()
	agents := filepath.Join(project, "AGENTS.md")
	original := "# Project\n\nIntro.\n\n" + messagesHeading + "\n\nstale instructions\n\n## Other\n\nKeep this.\n"
	if err := os.WriteFile(agents, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	code, err, _, stderr := runCLI(t, project, []string{"messages", "install"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(messages install) = %d, %v, stderr %q; want %d, nil", code, err, stderr.String(), exitOK)
	}
	first, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(first), messagesHeading) != 1 || strings.Contains(string(first), "stale instructions") {
		t.Fatalf("AGENTS.md after install = %q, want one replaced standing section", first)
	}
	if !strings.HasPrefix(string(first), "# Project\n\nIntro.\n\n") || !strings.Contains(string(first), "## Other\n\nKeep this.\n") {
		t.Fatalf("AGENTS.md after install = %q, want surrounding copy preserved", first)
	}
	mutated := strings.Replace(string(first), "Other chats can reach this one by name.", "stale again", 1)
	if mutated == string(first) {
		t.Fatal("could not mutate standing section for second install")
	}
	if err := os.WriteFile(agents, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	code, err, _, stderr = runCLI(t, project, []string{"messages", "install"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(messages install) second time = %d, %v, stderr %q; want %d, nil", code, err, stderr.String(), exitOK)
	}
	second, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != string(first) {
		t.Fatalf("AGENTS.md after second install = %q, want restored section with surrounding copy %q", second, first)
	}
}

func TestMessagesReportsNotConfiguredWhenStateFileIsMissing(t *testing.T) {
	// Bug class: a missing messages.json is read from ~/.landing or treated as a default store.
	state := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", state)
	project := t.TempDir()
	commands := [][]string{
		{"messages", "send", "--to", "office", "--message", "hi", "--as", "laptop"},
		{"messages", "monitor", "--as", "office"},
		{"messages", "inbox", "--as", "office"},
		{"messages", "who"},
	}
	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, err, stdout, _ := runCLI(t, project, args)
			if code != exitFailed || err == nil {
				t.Fatalf("Run(%q) = %d, %v; want %d, not-configured error", args, code, err, exitFailed)
			}
			if !strings.Contains(err.Error(), "messaging is not configured") {
				t.Fatalf("Run(%q) error = %q, want not configured", args, err)
			}
			if !strings.Contains(err.Error(), filepath.Join(state, "messages.json")) {
				t.Fatalf("Run(%q) error = %q, want temporary state path", args, err)
			}
			if strings.Contains(err.Error(), filepath.Join(os.Getenv("HOME"), ".landing")) {
				t.Fatalf("Run(%q) error = %q, used real ~/.landing", args, err)
			}
			if stdout.String() != "" {
				t.Fatalf("Run(%q) stdout = %q, want empty", args, stdout.String())
			}
		})
	}
	if _, err := os.Stat(filepath.Join(state, "messages.json")); !os.IsNotExist(err) {
		t.Fatalf("messages.json under test state = %v, want not created", err)
	}
}

func configureMessages(t *testing.T) (string, string) {
	t.Helper()
	state := t.TempDir()
	providerDir := t.TempDir()
	project := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", state)
	t.Setenv("LANDING_FAKE_PROVIDER_DIR", providerDir)
	command := fakeMessagesProvider(t)
	payload, err := json.Marshal(messageProviderFile{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "messages.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}

	return project, providerDir
}

func fakeMessagesProvider(t *testing.T) string {
	t.Helper()
	fakeMessagesProviderOnce.Do(func() {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			fakeMessagesProviderErr = fmt.Errorf("could not locate fake messages provider source")
			return
		}
		dir, err := os.MkdirTemp("", "landing-fake-messages-provider-")
		if err != nil {
			fakeMessagesProviderErr = err
			return
		}
		fakeMessagesProviderBin = filepath.Join(dir, "fake-messages-provider")
		if runtime.GOOS == "windows" {
			fakeMessagesProviderBin += ".exe"
		}
		source := filepath.Join(filepath.Dir(file), "testdata", "fake-messages-provider", "main.go")
		command := exec.Command("go", "build", "-o", fakeMessagesProviderBin, source)
		output, err := command.CombinedOutput()
		if err != nil {
			fakeMessagesProviderErr = fmt.Errorf("go build: %w\n%s", err, output)
		}
	})
	if fakeMessagesProviderErr != nil {
		t.Fatalf("build fake messages provider: %v", fakeMessagesProviderErr)
	}

	return fakeMessagesProviderBin
}

func recordedRequests(t *testing.T, providerDir string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(providerDir, "requests.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadFile(requests.jsonl) returned unexpected error: %v", err)
	}
	requests := make([]map[string]any, 0)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var request map[string]any
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			t.Fatalf("recorded request %q: %v", line, err)
		}
		requests = append(requests, request)
	}

	return requests
}

func writeProviderFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) returned unexpected error: %v", name, err)
	}
}

func testChat(t *testing.T, name string) chatSession {
	t.Helper()
	chat, err := identifyChat(context.Background(), parsedOption{Value: name, Set: true})
	if err != nil {
		t.Fatalf("identifyChat(%q) returned unexpected error: %v", name, err)
	}

	return chat
}

func startPinnedProcess(t *testing.T) int {
	t.Helper()
	command := exec.Command(fakeMessagesProvider(t))
	command.Env = append(os.Environ(), "LANDING_FAKE_PROVIDER_SLEEP=1")
	if err := command.Start(); err != nil {
		t.Fatalf("start pinned process: %v", err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})

	return command.Process.Pid
}

func deadPID(t *testing.T) int {
	t.Helper()
	command := exec.Command(fakeMessagesProvider(t))
	command.Env = append(os.Environ(), "LANDING_FAKE_PROVIDER_SLEEP=1")
	if err := command.Start(); err != nil {
		t.Fatalf("start process for dead pid: %v", err)
	}
	pid := command.Process.Pid
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill process for dead pid: %v", err)
	}
	_ = command.Wait()

	return pid
}

func requestString(request map[string]any, key string) string {
	value, _ := request[key].(string)

	return value
}
