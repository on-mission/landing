// Package hooks installs and removes Landing's project-scoped harness hooks.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Operation describes how a harness configuration file would change.
type Operation string

const (
	OperationCreate    Operation = "create"
	OperationMerge     Operation = "merge"
	OperationRemove    Operation = "remove"
	OperationUnchanged Operation = "unchanged"
	OperationBlocked   Operation = "cannot modify"
	OperationSkipped   Operation = "skipped"
)

// Hook identifies one configured harness lifecycle hook.
type Hook struct {
	Event   string
	Purpose string
	Command string
}

// FileChange is one exact configuration-file outcome.
type FileChange struct {
	Harness   string
	Path      string
	Operation Operation
	Hooks     []Hook
	Reason    string
}

// PlanResult describes every configuration file Landing would change.
//
// It is named PlanResult because Go has one package-level namespace: a type named
// Plan cannot coexist with the required Plan function.
type PlanResult struct {
	ProjectDir string
	Files      []FileChange
}

// Render returns a stable, reviewable description of the proposed changes.
func (plan PlanResult) Render() string {
	lines := make([]string, 0, len(plan.Files)*3)
	for _, file := range plan.Files {
		lines = append(lines, changeLine(file.Harness, string(file.Operation), file.Path, file.Reason))
		for _, hook := range file.Hooks {
			lines = append(lines, fmt.Sprintf("  %s: %s (%s)", hook.Event, hook.Command, hook.Purpose))
		}
	}

	return strings.Join(lines, "\n")
}

// Result reports the changes made by Install or Uninstall.
type Result struct {
	Files []FileChange
}

// Render returns a stable description of what happened to each configuration
// file, distinct from PlanResult.Render: it reports outcome, not intent, and
// does not repeat the per-hook detail the plan already gave.
func (result Result) Render() string {
	lines := make([]string, 0, len(result.Files))
	for _, file := range result.Files {
		lines = append(lines, changeLine(file.Harness, outcomeVerb(file.Operation), file.Path, file.Reason))
	}

	return strings.Join(lines, "\n")
}

func changeLine(harness string, verb string, path string, reason string) string {
	line := harness + ": " + verb
	if path != "" {
		line += " " + path
	}
	if reason != "" {
		line += "; " + reason
	}

	return line
}

func outcomeVerb(operation Operation) string {
	switch operation {
	case OperationCreate:
		return "created"
	case OperationMerge:
		return "merged Landing hooks into"
	case OperationRemove:
		return "removed Landing hooks from"
	case OperationUnchanged:
		return "did not change"
	case OperationBlocked:
		return "could not modify"
	case OperationSkipped:
		return "not installed"
	default:
		return string(operation)
	}
}

type harnessSpec struct {
	name             string
	relative         string
	hooks            []hookSpec
	availability     string
	trustRequirement string
}

type hookSpec struct {
	event   string
	purpose string
	eventID string
}

func harnesses() []harnessSpec {
	return []harnessSpec{
		{
			name:     "Claude Code",
			relative: filepath.Join(".claude", "settings.local.json"),
			hooks: []hookSpec{
				{event: "SessionStart", eventID: "session-start", purpose: "register the session and provide capability and recent history"},
				{event: "PreToolUse", eventID: "pre-tool-use", purpose: "deliver pending messages before a tool call"},
				{event: "PostToolUse", eventID: "post-tool-use", purpose: "deliver pending messages after a tool call"},
				{event: "Stop", eventID: "turn-end", purpose: "deliver anything still pending at turn end"},
				{event: "SessionEnd", eventID: "session-end", purpose: "deregister the session"},
			},
			availability:     "claude",
			trustRequirement: "Claude Code records workspace-trust acceptance for project configuration; Landing does not create that acceptance",
		},
		{
			name:     "Codex",
			relative: filepath.Join(".codex", "hooks.json"),
			hooks: []hookSpec{
				{event: "SessionStart", eventID: "session-start", purpose: "register the session and provide capability and recent history"},
				{event: "PreToolUse", eventID: "pre-tool-use", purpose: "deliver pending messages before a tool call"},
				{event: "PostToolUse", eventID: "post-tool-use", purpose: "deliver pending messages after a tool call"},
				{event: "Stop", eventID: "turn-end", purpose: "deliver anything still pending at turn end"},
				{event: "SessionEnd", eventID: "session-end", purpose: "deregister the session"},
			},
			availability:     "codex",
			trustRequirement: "Codex runs project hooks only after Codex has persisted trust for their source; Landing cannot create that trust",
		},
		{
			name:     "Grok",
			relative: filepath.Join(".grok", "hooks", "landing.json"),
			hooks: []hookSpec{
				{event: "SessionStart", eventID: "session-start", purpose: "register the session and provide capability and recent history"},
				{event: "Stop", eventID: "turn-end", purpose: "deliver anything still pending at turn end"},
				{event: "SessionEnd", eventID: "session-end", purpose: "deregister the session"},
			},
			availability:     "grok",
			trustRequirement: "Grok runs project hooks only after persisted folder trust includes the project; Landing does not create that trust",
		},
		{
			name:         "Cline",
			relative:     "",
			availability: "cline",
		},
	}
}

// Plan reports the precise changes Install would make without writing files.
func Plan(ctx context.Context, projectDir string) (PlanResult, error) {
	landing, err := landingPath()
	if err != nil {
		return PlanResult{}, err
	}

	return inspect(ctx, projectDir, landing)
}

// Install merges Landing's hooks into supported installed harness configurations.
func Install(ctx context.Context, projectDir string) (Result, error) {
	landing, err := landingPath()
	if err != nil {
		return Result{}, err
	}
	root, err := cleanProjectDir(projectDir)
	if err != nil {
		return Result{}, err
	}
	changes := make([]FileChange, 0, len(harnesses()))
	for _, spec := range harnesses() {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if spec.relative == "" {
			changes = append(changes, fileChange(spec, "", OperationSkipped, nil, "Cline has no context-injection hook; sessions call Landing directly"))
			continue
		}
		path := filepath.Join(root, spec.relative)
		if !available(spec.availability) {
			changes = append(changes, fileChange(spec, path, OperationSkipped, nil, "harness is absent; this is what Landing would configure if installed"))
			continue
		}
		change, err := installFile(path, spec, landing)
		if err != nil {
			return Result{}, err
		}
		changes = append(changes, change)
	}

	return Result{Files: changes}, nil
}

// Uninstall removes only hooks whose command was added by Landing.
func Uninstall(ctx context.Context, projectDir string) (Result, error) {
	root, err := cleanProjectDir(projectDir)
	if err != nil {
		return Result{}, err
	}
	changes := make([]FileChange, 0, len(harnesses()))
	for _, spec := range harnesses() {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if spec.relative == "" {
			changes = append(changes, FileChange{Harness: spec.name, Operation: OperationSkipped, Reason: "Cline cannot inject text into a running agent"})
			continue
		}
		path := filepath.Join(root, spec.relative)
		change, err := uninstallFile(path, spec)
		if err != nil {
			return Result{}, err
		}
		changes = append(changes, change)
	}

	return Result{Files: changes}, nil
}

func inspect(ctx context.Context, projectDir string, landing string) (PlanResult, error) {
	root, err := cleanProjectDir(projectDir)
	if err != nil {
		return PlanResult{}, err
	}
	changes := make([]FileChange, 0, len(harnesses()))
	for _, spec := range harnesses() {
		if err := ctx.Err(); err != nil {
			return PlanResult{}, err
		}
		if spec.relative == "" {
			changes = append(changes, fileChange(spec, "", OperationSkipped, nil, "Cline has no context-injection hook; sessions call Landing directly"))
			continue
		}
		path := filepath.Join(root, spec.relative)
		hooks := plannedHooks(spec, landing)
		if !available(spec.availability) {
			changes = append(changes, fileChange(spec, path, OperationSkipped, hooks, "harness is absent; this is what Landing would configure if installed"))
			continue
		}
		contents, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			changes = append(changes, fileChange(spec, path, OperationCreate, hooks, ""))
			continue
		}
		if err != nil {
			return PlanResult{}, fmt.Errorf("reading %s: %w", path, err)
		}
		if err := validateHooksDocument(contents); err != nil {
			changes = append(changes, fileChange(spec, path, OperationBlocked, nil, err.Error()))
			continue
		}
		if containsLandingHook(contents) {
			changes = append(changes, fileChange(spec, path, OperationUnchanged, hooks, "Landing hooks already present"))
			continue
		}
		changes = append(changes, fileChange(spec, path, OperationMerge, hooks, ""))
	}

	return PlanResult{ProjectDir: root, Files: changes}, nil
}

func fileChange(spec harnessSpec, path string, operation Operation, hooks []Hook, reason string) FileChange {
	if spec.trustRequirement != "" {
		if reason != "" {
			reason += "; "
		}
		reason += spec.trustRequirement
	}

	return FileChange{
		Harness:   spec.name,
		Path:      path,
		Operation: operation,
		Hooks:     hooks,
		Reason:    reason,
	}
}

func cleanProjectDir(projectDir string) (string, error) {
	if projectDir == "" {
		return "", errors.New("project directory is required")
	}
	root, err := filepath.Abs(projectDir)
	if err != nil {
		return "", fmt.Errorf("resolving project directory: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("stating project directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project directory %q is not a directory", root)
	}

	return root, nil
}

func landingPath() (string, error) {
	path, err := exec.LookPath("landing")
	if err != nil {
		return "", errors.New("finding landing executable in PATH")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving landing executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolving landing executable symlinks: %w", err)
	}

	return resolved, nil
}

func available(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func plannedHooks(spec harnessSpec, landing string) []Hook {
	hooks := make([]Hook, 0, len(spec.hooks))
	for _, hook := range spec.hooks {
		command := "landing path resolved at installation"
		if landing != "" {
			command = hookCommand(landing, spec.name, hook.eventID)
		}
		hooks = append(hooks, Hook{Event: hook.event, Purpose: hook.purpose, Command: command})
	}

	return hooks
}

func hookCommand(landing, harness, event string) string {
	return shellQuote(landing) + " comms hook --harness " + shellQuote(strings.ToLower(strings.ReplaceAll(harness, " Code", ""))) + " --event " + shellQuote(event)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func installFile(path string, spec harnessSpec, landing string) (FileChange, error) {
	hooksList := plannedHooks(spec, landing)
	contents, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := writeFile(path, documentFor(spec, landing)); err != nil {
			return FileChange{}, err
		}
		return fileChange(spec, path, OperationCreate, hooksList, ""), nil
	}
	if err != nil {
		return FileChange{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := validateHooksDocument(contents); err != nil {
		return fileChange(spec, path, OperationBlocked, nil, err.Error()), nil
	}
	if containsLandingHook(contents) {
		return fileChange(spec, path, OperationUnchanged, hooksList, "Landing hooks already present"), nil
	}
	updated, err := appendHooks(contents, spec, landing)
	if err != nil {
		return FileChange{}, fmt.Errorf("merging %s: %w", path, err)
	}
	if err := writeFile(path, updated); err != nil {
		return FileChange{}, err
	}

	return fileChange(spec, path, OperationMerge, hooksList, ""), nil
}

func uninstallFile(path string, spec harnessSpec) (FileChange, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return FileChange{Harness: spec.name, Path: path, Operation: OperationUnchanged, Reason: "configuration file does not exist"}, nil
	}
	if err != nil {
		return FileChange{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := validateHooksDocument(contents); err != nil {
		return FileChange{Harness: spec.name, Path: path, Operation: OperationBlocked, Reason: err.Error()}, nil
	}
	if onlyLandingHooks(contents) {
		if err := os.Remove(path); err != nil {
			return FileChange{}, fmt.Errorf("removing %s: %w", path, err)
		}
		return FileChange{Harness: spec.name, Path: path, Operation: OperationRemove}, nil
	}
	updated, changed := removeLandingHooks(contents)
	if !changed {
		return FileChange{Harness: spec.name, Path: path, Operation: OperationUnchanged, Reason: "Landing hooks are not present"}, nil
	}
	if bytes.Equal(updated, []byte("{}\n")) {
		if err := os.Remove(path); err != nil {
			return FileChange{}, fmt.Errorf("removing %s: %w", path, err)
		}
		return FileChange{Harness: spec.name, Path: path, Operation: OperationRemove}, nil
	}
	if err := writeFile(path, updated); err != nil {
		return FileChange{}, err
	}

	return FileChange{Harness: spec.name, Path: path, Operation: OperationRemove}, nil
}

func writeFile(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating configuration directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

func documentFor(spec harnessSpec, landing string) []byte {
	events := make(map[string][]json.RawMessage, len(spec.hooks))
	for _, hook := range spec.hooks {
		events[hook.event] = []json.RawMessage{hookGroup(spec.name, hook, landing)}
	}
	keys := sortedKeys(events)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%q:[%s]", key, events[key][0]))
	}

	return []byte("{\"hooks\":{" + strings.Join(parts, ",") + "}}\n")
}

func hookGroup(harness string, hook hookSpec, landing string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf("{\"hooks\":[{\"type\":\"command\",\"command\":%q,\"timeout\":5}]}", hookCommand(landing, harness, hook.eventID)))
}

func validateHooksDocument(contents []byte) error {
	if !json.Valid(contents) {
		return errors.New("configuration is not valid JSON")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(contents, &root); err != nil {
		return errors.New("configuration root must be an object")
	}
	hooks, ok := root["hooks"]
	if !ok {
		return nil
	}
	var events map[string]json.RawMessage
	if err := json.Unmarshal(hooks, &events); err != nil {
		return errors.New("hooks must be an object")
	}
	for event, rawGroups := range events {
		var groups []json.RawMessage
		if err := json.Unmarshal(rawGroups, &groups); err != nil {
			return fmt.Errorf("hooks.%s must be an array", event)
		}
		for _, rawGroup := range groups {
			var group map[string]json.RawMessage
			if err := json.Unmarshal(rawGroup, &group); err != nil {
				return fmt.Errorf("hooks.%s contains a non-object group", event)
			}
			handlers, ok := group["hooks"]
			if !ok {
				return fmt.Errorf("hooks.%s group has no hooks array", event)
			}
			var values []json.RawMessage
			if err := json.Unmarshal(handlers, &values); err != nil {
				return fmt.Errorf("hooks.%s group hooks must be an array", event)
			}
		}
	}

	return nil
}

func appendHooks(contents []byte, spec harnessSpec, landing string) ([]byte, error) {
	root, err := objectAt(contents, 0)
	if err != nil {
		return nil, err
	}
	hooksProperty, hasHooks, err := root.property("hooks")
	if err != nil {
		return nil, err
	}
	if !hasHooks {
		prefix := []byte(",")
		if root.empty() {
			prefix = nil
		}
		var generated map[string]json.RawMessage
		if err := json.Unmarshal(documentFor(spec, landing), &generated); err != nil {
			return nil, err
		}
		addition := append(prefix, []byte("\"hooks\":"+string(generated["hooks"]))...)
		return insertBefore(contents, root.end-1, addition), nil
	}
	hooksObject, err := objectAt(contents, hooksProperty.valueStart)
	if err != nil {
		return nil, errors.New("hooks must be an object")
	}
	updated := contents
	for _, hook := range spec.hooks {
		updated, err = appendEvent(updated, hooksObject, spec.name, hook, landing)
		if err != nil {
			return nil, err
		}
		hooksObject, err = objectAt(updated, hooksProperty.valueStart)
		if err != nil {
			return nil, err
		}
	}

	return updated, nil
}

func appendEvent(contents []byte, hooks object, harness string, hook hookSpec, landing string) ([]byte, error) {
	property, found, err := hooks.property(hook.event)
	if err != nil {
		return nil, err
	}
	group := hookGroup(harness, hook, landing)
	if !found {
		addition := []byte(fmt.Sprintf(",%q:[%s]", hook.event, group))
		return insertBefore(contents, hooks.end-1, addition), nil
	}
	array, err := arrayAt(contents, property.valueStart)
	if err != nil {
		return nil, fmt.Errorf("hooks.%s must be an array", hook.event)
	}
	separator := []byte(",")
	if array.empty() {
		separator = nil
	}
	return insertBefore(contents, array.end-1, append(separator, group...)), nil
}

func removeLandingHooks(contents []byte) ([]byte, bool) {
	updated := contents
	changed := false
	for containsLandingHook(updated) {
		root, err := objectAt(updated, 0)
		if err != nil {
			return contents, false
		}
		hooksProperty, found, err := root.property("hooks")
		if err != nil || !found {
			return contents, false
		}
		hooks, err := objectAt(updated, hooksProperty.valueStart)
		if err != nil {
			return contents, false
		}
		removed := false
		for _, event := range hooks.properties() {
			array, err := arrayAt(updated, event.valueStart)
			if err != nil {
				return contents, false
			}
			for _, group := range array.elements() {
				if !bytes.Contains(updated[group.start:group.end], []byte(" comms hook --harness ")) {
					continue
				}
				if len(array.elements()) == 1 {
					updated = removeObjectProperty(updated, hooks, event)
				} else {
					updated = removeArrayValue(updated, array, group)
				}
				changed = true
				removed = true
				break
			}
			if removed {
				break
			}
		}
		if !removed {
			return contents, false
		}
	}

	return updated, changed
}

func onlyLandingHooks(contents []byte) bool {
	var root map[string]json.RawMessage
	if json.Unmarshal(contents, &root) != nil || len(root) != 1 {
		return false
	}
	rawHooks, ok := root["hooks"]
	if !ok {
		return false
	}
	var events map[string][]json.RawMessage
	if json.Unmarshal(rawHooks, &events) != nil || len(events) == 0 {
		return false
	}
	for _, groups := range events {
		if len(groups) == 0 {
			return false
		}
		for _, group := range groups {
			if !bytes.Contains(group, []byte(" comms hook --harness ")) {
				return false
			}
		}
	}

	return true
}

func containsLandingHook(contents []byte) bool {
	return bytes.Contains(contents, []byte(" comms hook --harness "))
}

func sortedKeys(values map[string][]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

type object struct {
	start int
	end   int
	data  []byte
}

type property struct {
	keyStart   int
	valueStart int
	valueEnd   int
}

func objectAt(data []byte, start int) (object, error) {
	position := skipSpace(data, start)
	if position >= len(data) || data[position] != '{' {
		return object{}, errors.New("expected JSON object")
	}
	end, err := valueEnd(data, position)
	if err != nil {
		return object{}, err
	}

	return object{start: position, end: end, data: data}, nil
}

func arrayAt(data []byte, start int) (object, error) {
	position := skipSpace(data, start)
	if position >= len(data) || data[position] != '[' {
		return object{}, errors.New("expected JSON array")
	}
	end, err := valueEnd(data, position)
	if err != nil {
		return object{}, err
	}

	return object{start: position, end: end, data: data}, nil
}

func (value object) property(name string) (property, bool, error) {
	position := skipSpace(value.data, value.start+1)
	if position < value.end && value.data[position] == '}' {
		return property{}, false, nil
	}
	for position < value.end-1 {
		if value.data[position] != '"' {
			return property{}, false, errors.New("unexpected JSON object shape")
		}
		keyEnd, err := stringEnd(value.data, position)
		if err != nil {
			return property{}, false, err
		}
		var key string
		if err := json.Unmarshal(value.data[position:keyEnd], &key); err != nil {
			return property{}, false, err
		}
		colon := skipSpace(value.data, keyEnd)
		if colon >= value.end || value.data[colon] != ':' {
			return property{}, false, errors.New("expected colon after JSON object key")
		}
		valueStart := skipSpace(value.data, colon+1)
		valueEndIndex, err := valueEnd(value.data, valueStart)
		if err != nil {
			return property{}, false, err
		}
		if key == name {
			return property{keyStart: position, valueStart: valueStart, valueEnd: valueEndIndex}, true, nil
		}
		position = skipSpace(value.data, valueEndIndex)
		if position >= value.end || value.data[position] == '}' {
			break
		}
		if value.data[position] != ',' {
			return property{}, false, errors.New("expected comma between JSON object properties")
		}
		position = skipSpace(value.data, position+1)
	}

	return property{}, false, nil
}

func (value object) properties() []property {
	properties := make([]property, 0)
	position := skipSpace(value.data, value.start+1)
	for position < value.end-1 && value.data[position] == '"' {
		keyEnd, err := stringEnd(value.data, position)
		if err != nil {
			return nil
		}
		colon := skipSpace(value.data, keyEnd)
		if colon >= value.end || value.data[colon] != ':' {
			return nil
		}
		valueStart := skipSpace(value.data, colon+1)
		valueEndIndex, err := valueEnd(value.data, valueStart)
		if err != nil {
			return nil
		}
		properties = append(properties, property{keyStart: position, valueStart: valueStart, valueEnd: valueEndIndex})
		position = skipSpace(value.data, valueEndIndex)
		if position >= value.end-1 || value.data[position] != ',' {
			break
		}
		position = skipSpace(value.data, position+1)
	}

	return properties
}

func (value object) elements() []object {
	elements := make([]object, 0)
	position := skipSpace(value.data, value.start+1)
	for position < value.end-1 {
		end, err := valueEnd(value.data, position)
		if err != nil {
			return nil
		}
		elements = append(elements, object{start: position, end: end, data: value.data})
		position = skipSpace(value.data, end)
		if position >= value.end-1 || value.data[position] != ',' {
			break
		}
		position = skipSpace(value.data, position+1)
	}

	return elements
}

func (value object) empty() bool {
	return skipSpace(value.data, value.start+1) == value.end-1
}

func skipSpace(data []byte, position int) int {
	for position < len(data) && (data[position] == ' ' || data[position] == '\n' || data[position] == '\r' || data[position] == '\t') {
		position++
	}

	return position
}

func valueEnd(data []byte, start int) (int, error) {
	if start >= len(data) {
		return 0, errors.New("unexpected end of JSON")
	}
	switch data[start] {
	case '"':
		return stringEnd(data, start)
	case '{', '[':
		open := data[start]
		close := byte('}')
		if open == '[' {
			close = ']'
		}
		depth := 1
		for position := start + 1; position < len(data); position++ {
			if data[position] == '"' {
				end, err := stringEnd(data, position)
				if err != nil {
					return 0, err
				}
				position = end - 1
				continue
			}
			if data[position] == open {
				depth++
			}
			if data[position] == close {
				depth--
				if depth == 0 {
					return position + 1, nil
				}
			}
		}
		return 0, errors.New("unterminated JSON value")
	default:
		position := start
		for position < len(data) && !bytes.ContainsRune([]byte(" \n\r\t,]}"), rune(data[position])) {
			position++
		}
		return position, nil
	}
}

func stringEnd(data []byte, start int) (int, error) {
	for position := start + 1; position < len(data); position++ {
		if data[position] == '\\' {
			position++
			continue
		}
		if data[position] == '"' {
			return position + 1, nil
		}
	}

	return 0, errors.New("unterminated JSON string")
}

func insertBefore(data []byte, position int, addition []byte) []byte {
	result := make([]byte, 0, len(data)+len(addition))
	result = append(result, data[:position]...)
	result = append(result, addition...)
	result = append(result, data[position:]...)

	return result
}

func removeObjectProperty(data []byte, object object, property property) []byte {
	start := property.keyStart
	end := property.valueEnd
	before := skipSpaceBackward(data, start)
	if before > object.start && data[before-1] == ',' {
		start = before - 1
	} else {
		after := skipSpace(data, end)
		if after < object.end-1 && data[after] == ',' {
			end = after + 1
		}
	}

	return append(append([]byte{}, data[:start]...), data[end:]...)
}

func removeArrayValue(data []byte, array object, value object) []byte {
	start := value.start
	end := value.end
	before := skipSpaceBackward(data, start)
	if before > array.start && data[before-1] == ',' {
		start = before - 1
	} else {
		after := skipSpace(data, end)
		if after < array.end-1 && data[after] == ',' {
			end = after + 1
		}
	}

	return append(append([]byte{}, data[:start]...), data[end:]...)
}

func skipSpaceBackward(data []byte, position int) int {
	for position > 0 && (data[position-1] == ' ' || data[position-1] == '\n' || data[position-1] == '\r' || data[position-1] == '\t') {
		position--
	}

	return position
}
