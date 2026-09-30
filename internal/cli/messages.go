package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type chatSession struct {
	name    string
	session string
}

type harnessAncestor struct {
	pid int
	id  string
}

func runMessages(ctx context.Context, values options, positionals []string, invocationDir string, stdout io.Writer) (int, error) {
	if len(positionals) == 0 {
		return exitUsage, &usageError{message: "messages requires a command"}
	}
	switch positionals[0] {
	case "send":
		return runMessagesSend(ctx, values, positionals[1:], stdout)
	case "monitor":
		return runMessagesMonitor(ctx, values, positionals[1:], stdout)
	case "who":
		return runMessagesWho(ctx, values, positionals[1:], stdout)
	case "inbox":
		return runMessagesInbox(ctx, values, positionals[1:], stdout)
	case "install":
		return runMessagesContextEdit(values, positionals[1:], invocationDir, stdout, true)
	case "uninstall":
		return runMessagesContextEdit(values, positionals[1:], invocationDir, stdout, false)
	default:
		return exitUsage, &usageError{message: fmt.Sprintf("messages has unknown command %q", positionals[0])}
	}
}

func runMessagesSend(ctx context.Context, values options, positionals []string, stdout io.Writer) (int, error) {
	if len(positionals) != 0 {
		return exitUsage, &usageError{message: "messages send has unexpected arguments"}
	}
	if err := unexpectedOptions(values, "messages send", "to", "message", "as", "json"); err != nil {
		return exitUsage, err
	}
	if !values.To.Set {
		return exitUsage, &usageError{message: "--to is required"}
	}
	if !values.Message.Set {
		return exitUsage, &usageError{message: "--message is required"}
	}
	if len(values.Message.Value) > maxMessageBody {
		return exitUsage, &usageError{message: "message body exceeds 64 KiB"}
	}
	if err := validateRecipient(values.To.Value); err != nil {
		return exitUsage, err
	}
	chat, err := identifyChat(ctx, values.As)
	if err != nil {
		return messageExit(err)
	}
	provider, err := loadMessageProvider()
	if err != nil {
		return exitFailed, err
	}
	now := time.Now().UTC()
	id, err := newMessageID(now)
	if err != nil {
		return exitFailed, err
	}
	message := wireMessage{ID: id, From: chat.name, To: values.To.Value, Body: values.Message.Value, SentAt: formatSentAt(now)}
	if err := provider.send(ctx, message); err != nil {
		return exitFailed, err
	}
	if values.JSON {
		if err := writeStoredMessage(stdout, message); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if _, err := fmt.Fprintf(stdout, "sent %s to %s\n", message.ID, message.To); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func runMessagesMonitor(ctx context.Context, values options, positionals []string, stdout io.Writer) (int, error) {
	if len(positionals) != 0 {
		return exitUsage, &usageError{message: "messages monitor has unexpected arguments"}
	}
	if err := unexpectedOptions(values, "messages monitor", "as", "json"); err != nil {
		return exitUsage, err
	}
	chat, err := identifyChat(ctx, values.As)
	if err != nil {
		return messageExit(err)
	}
	provider, err := loadMessageProvider()
	if err != nil {
		return exitFailed, err
	}
	if err := claimMonitor(chat.session, chat.name); err != nil {
		return exitFailed, err
	}
	if err := retryMonitorCommand(ctx, func() error {
		return acknowledgePresented(ctx, provider, chat.session)
	}); err != nil {
		return exitFailed, err
	}
	var messages []wireMessage
	if err := retryMonitorCommand(ctx, func() error {
		var err error
		messages, err = provider.wait(ctx, chat.name, true)
		return err
	}); err != nil {
		return exitFailed, err
	}
	if err := publishMessages(stdout, chat.session, chat.name, messages, values.JSON); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func runMessagesWho(ctx context.Context, values options, positionals []string, stdout io.Writer) (int, error) {
	if len(positionals) > 1 {
		return exitUsage, &usageError{message: "messages who has unexpected arguments"}
	}
	if err := unexpectedOptions(values, "messages who"); err != nil {
		return exitUsage, err
	}
	provider, err := loadMessageProvider()
	if err != nil {
		return exitFailed, err
	}
	recipients, err := provider.list(ctx)
	if err != nil {
		return exitFailed, err
	}
	if len(positionals) == 0 {
		for _, recipient := range recipients {
			if _, err := fmt.Fprintln(stdout, recipient.Name); err != nil {
				return exitFailed, err
			}
		}
		return exitOK, nil
	}
	name := positionals[0]
	var found *listedRecipient
	for index := range recipients {
		if recipients[index].Name == name {
			found = &recipients[index]
			break
		}
	}
	if found == nil {
		return exitUsage, &usageError{message: fmt.Sprintf("name %q is not registered", name)}
	}
	since, err := displayProviderTime(found.Since)
	if err != nil {
		return exitFailed, err
	}
	if _, err := fmt.Fprintf(stdout, "%s\n  registered: %s\n", found.Name, since); err != nil {
		return exitFailed, err
	}
	pids, err := liveMonitorPIDs(name)
	if err != nil {
		return exitFailed, err
	}
	for _, pid := range pids {
		if _, err := fmt.Fprintf(stdout, "  pid: %d\n", pid); err != nil {
			return exitFailed, err
		}
	}

	return exitOK, nil
}

func runMessagesInbox(ctx context.Context, values options, positionals []string, stdout io.Writer) (int, error) {
	if len(positionals) != 0 {
		return exitUsage, &usageError{message: "messages inbox has unexpected arguments"}
	}
	if err := unexpectedOptions(values, "messages inbox", "as"); err != nil {
		return exitUsage, err
	}
	chat, err := identifyChat(ctx, values.As)
	if err != nil {
		return messageExit(err)
	}
	provider, err := loadMessageProvider()
	if err != nil {
		return exitFailed, err
	}
	messages, err := provider.wait(ctx, chat.name, false)
	if err != nil {
		return exitFailed, err
	}
	if len(messages) == 0 {
		if _, err := fmt.Fprintln(stdout, "inbox empty"); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if err := writeMessages(stdout, messages, false); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func runMessagesContextEdit(values options, positionals []string, invocationDir string, stdout io.Writer, install bool) (int, error) {
	command := "messages uninstall"
	if install {
		command = "messages install"
	}
	if len(positionals) != 0 {
		return exitUsage, &usageError{message: command + " has unexpected arguments"}
	}
	if err := unexpectedOptions(values, command); err != nil {
		return exitUsage, err
	}

	return editContextFiles(invocationDir, stdout, install)
}

func messageExit(err error) (int, error) {
	var usage *usageError
	if errors.As(err, &usage) {
		return exitUsage, err
	}

	return exitFailed, err
}

func identifyChat(ctx context.Context, as parsedOption) (chatSession, error) {
	if as.Set {
		if err := validateChatName(as.Value); err != nil {
			return chatSession{}, err
		}
	}
	ancestor, found, err := closestHarnessAncestor(ctx)
	if err != nil {
		return chatSession{}, err
	}
	if !found {
		if !as.Set {
			return chatSession{}, &usageError{message: "no harness parent and no --as"}
		}

		return chatSession{name: as.Value, session: as.Value}, nil
	}
	name := ancestor.id + "-" + strconv.Itoa(ancestor.pid)
	if as.Set {
		name = as.Value
	}

	return chatSession{name: name, session: strconv.Itoa(ancestor.pid)}, nil
}

func closestHarnessAncestor(ctx context.Context) (harnessAncestor, bool, error) {
	ancestors, err := ancestorProcesses(ctx)
	if err != nil {
		return harnessAncestor{}, false, fmt.Errorf("find harness parent: %w", err)
	}
	known := harnessIDSet()
	for _, ancestor := range ancestors {
		id, ok := harnessIDFromCommand(ancestor.command, known)
		if !ok {
			continue
		}

		return harnessAncestor{pid: ancestor.pid, id: id}, true, nil
	}

	return harnessAncestor{}, false, nil
}

func harnessIDSet() map[string]struct{} {
	set := make(map[string]struct{})
	for _, id := range newRegistry().IDs() {
		set[id] = struct{}{}
	}
	delete(set, "landing")

	return set
}

func harnessIDFromCommand(command string, known map[string]struct{}) (string, bool) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", false
	}
	base := filepath.Base(fields[0])
	if len(base) > 4 && strings.EqualFold(base[len(base)-4:], ".exe") {
		base = base[:len(base)-4]
	}
	// The landing process is an ancestor of a nested command, not a chat.
	if strings.EqualFold(base, "landing") {
		return "", false
	}
	if _, ok := known[base]; ok {
		return base, true
	}
	lower := strings.ToLower(base)
	if _, ok := known[lower]; ok {
		return lower, true
	}

	return "", false
}

func validateChatName(name string) error {
	if name == "all" {
		return &usageError{message: `--as name "all" is reserved`}
	}
	if !chatName(name) {
		return &usageError{message: fmt.Sprintf("--as name %q must be letters, digits, and hyphens", name)}
	}

	return nil
}

func validateRecipient(name string) error {
	if name == "all" || chatName(name) {
		return nil
	}

	return &usageError{message: fmt.Sprintf("--to name %q must be letters, digits, and hyphens", name)}
}

func chatName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '-':
		default:
			return false
		}
	}

	return true
}

func claimMonitor(session, name string) error {
	existing, ok, err := readMonitor(session)
	if err != nil {
		return err
	}
	if ok && existing.PID != os.Getpid() && processAlive(existing.PID) {
		return fmt.Errorf("a monitor is already running (pid %d)", existing.PID)
	}

	return writeMonitor(session, monitorRecord{PID: os.Getpid(), Name: name})
}

func acknowledgePresented(ctx context.Context, provider messageProvider, session string) error {
	record, ok, err := readPresented(session)
	if err != nil || !ok {
		return err
	}
	if len(record.IDs) == 0 {
		return removePresented(session)
	}
	if record.Recipient == "" {
		return fmt.Errorf("presented messages for session %s have no recipient", session)
	}
	if err := provider.ack(ctx, record.Recipient, record.IDs); err != nil {
		return err
	}

	return removePresented(session)
}

// retryMonitorCommand waits 3 seconds and tries a provider call again. The
// failure stays off stdout: that stream is the batch a chat treats as mail.
// A local record error is not a provider failure and is returned as-is.
func retryMonitorCommand(ctx context.Context, op func() error) error {
	for {
		err := op()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !retryableMonitorError(err) {
			return err
		}
		timer := time.NewTimer(monitorRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func retryableMonitorError(err error) bool {
	var failure *providerFailure
	if errors.As(err, &failure) {
		return true
	}
	text := err.Error()

	return text == "wait returned no messages" || strings.HasPrefix(text, "message command returned")
}

func publishMessages(stdout io.Writer, session, recipient string, messages []wireMessage, asJSON bool) error {
	if err := writeMessages(stdout, messages, asJSON); err != nil {
		return err
	}
	if err := flushWriter(stdout); err != nil {
		return err
	}
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}

	return writePresented(session, presentedRecord{Recipient: recipient, IDs: ids})
}

func writeMessages(stdout io.Writer, messages []wireMessage, asJSON bool) error {
	if asJSON {
		encoded, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
			return err
		}

		return nil
	}
	for _, message := range messages {
		sentAt, err := displayProviderTime(message.SentAt)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "message %s from %s at %s\n%s", message.ID, message.From, sentAt, message.Body); err != nil {
			return err
		}
		if strings.HasSuffix(message.Body, "\n") {
			continue
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
	}

	return nil
}

func writeStoredMessage(stdout io.Writer, message wireMessage) error {
	encoded, err := json.MarshalIndent(message, "", "  ")
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return err
	}

	return nil
}

func flushWriter(writer io.Writer) error {
	flusher, ok := writer.(interface{ Flush() error })
	if !ok {
		return nil
	}

	return flusher.Flush()
}
