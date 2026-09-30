package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/on-mission/landing/internal/paths"
)

const (
	maxMessageBody    = 64 * 1024
	monitorRetryDelay = 3 * time.Second
)

// wireMessage is the message object Landing sends and the provider returns.
type wireMessage struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Body   string `json:"body"`
	SentAt string `json:"sentAt"`
}

type sendRequest struct {
	Op     string `json:"op"`
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Body   string `json:"body"`
	SentAt string `json:"sentAt"`
}

type sendResult struct {
	ID string `json:"id"`
}

type waitRequest struct {
	Op        string `json:"op"`
	Recipient string `json:"recipient"`
	// Block is omitted for a monitor. Inbox sets it to false. A bool would
	// drop false under omitempty, so this stays a pointer.
	Block *bool `json:"block,omitempty"`
	// Harness is set only for a blocking wait, and only when the walk found
	// one. omitempty drops it when the walk found none.
	Harness string `json:"harness,omitempty"`
}

type waitResult struct {
	Messages []wireMessage `json:"messages"`
}

type ackRequest struct {
	Op        string   `json:"op"`
	Recipient string   `json:"recipient"`
	IDs       []string `json:"ids"`
}

type ackResult struct {
	Acked []string `json:"acked"`
}

type listRequest struct {
	Op string `json:"op"`
}

type listedRecipient struct {
	Name    string `json:"name"`
	Since   string `json:"since"`
	Harness string `json:"harness,omitempty"`
}

type listResult struct {
	Recipients []listedRecipient `json:"recipients"`
}

type messageProviderFile struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type messageProvider struct {
	command string
	args    []string
}

// providerFailure is a non-zero exit of the configured command. stderr is the
// error Landing shows; args are not included because they are the command's
// own configuration and Landing does not interpret them.
type providerFailure struct {
	command string
	stderr  string
	cause   error
}

func (failure *providerFailure) Error() string {
	if text := strings.TrimSpace(failure.stderr); text != "" {
		return text
	}
	if failure.cause != nil {
		return fmt.Sprintf("message command %s failed: %v", failure.command, failure.cause)
	}

	return fmt.Sprintf("message command %s failed", failure.command)
}

func (failure *providerFailure) Unwrap() error {
	return failure.cause
}

func loadMessageProvider() (messageProvider, error) {
	path, err := messagesConfigPath()
	if err != nil {
		return messageProvider{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return messageProvider{}, fmt.Errorf("messaging is not configured; %s does not exist", paths.Display(path))
	}
	if err != nil {
		return messageProvider{}, fmt.Errorf("read messaging configuration %s: %w", paths.Display(path), err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var file messageProviderFile
	if err := decoder.Decode(&file); err != nil {
		return messageProvider{}, fmt.Errorf("messaging configuration %s: %w", paths.Display(path), err)
	}
	if decoder.More() {
		return messageProvider{}, fmt.Errorf("messaging configuration %s has more than one JSON object", paths.Display(path))
	}
	command := strings.TrimSpace(file.Command)
	if command == "" {
		return messageProvider{}, fmt.Errorf("messaging is not configured; %s has no command", paths.Display(path))
	}

	return messageProvider{command: command, args: append([]string(nil), file.Args...)}, nil
}

func (provider messageProvider) send(ctx context.Context, message wireMessage) error {
	var result sendResult
	request := sendRequest{Op: "send", ID: message.ID, From: message.From, To: message.To, Body: message.Body, SentAt: message.SentAt}
	if err := provider.call(ctx, request, &result); err != nil {
		return err
	}
	if result.ID != message.ID {
		return fmt.Errorf("message command stored id %q for id %q", result.ID, message.ID)
	}

	return nil
}

func (provider messageProvider) wait(ctx context.Context, recipient string, blocking bool, harness string) ([]wireMessage, error) {
	request := waitRequest{Op: "wait", Recipient: recipient}
	if blocking {
		request.Harness = harness
	} else {
		value := false
		request.Block = &value
	}
	var result waitResult
	if err := provider.call(ctx, request, &result); err != nil {
		return nil, err
	}
	if err := validateMessages(result.Messages); err != nil {
		return nil, err
	}
	if blocking && len(result.Messages) == 0 {
		return nil, errors.New("wait returned no messages")
	}

	return result.Messages, nil
}

func (provider messageProvider) ack(ctx context.Context, recipient string, ids []string) error {
	var result ackResult
	request := ackRequest{Op: "ack", Recipient: recipient, IDs: append([]string(nil), ids...)}

	return provider.call(ctx, request, &result)
}

func (provider messageProvider) list(ctx context.Context) ([]listedRecipient, error) {
	var result listResult
	if err := provider.call(ctx, listRequest{Op: "list"}, &result); err != nil {
		return nil, err
	}
	for _, recipient := range result.Recipients {
		if recipient.Name == "" {
			return nil, errors.New("message command returned a recipient with no name")
		}
		if _, err := parseProviderTime(recipient.Since); err != nil {
			return nil, fmt.Errorf("message command returned recipient %q with since %q", recipient.Name, recipient.Since)
		}
	}

	return result.Recipients, nil
}

// call runs the configured command once. request and response are the
// operation structs encoded at this boundary. Env is left unset so the
// process inherits PATH and Landing does not inject LANDING_* variables.
func (provider messageProvider) call(ctx context.Context, request any, response any) error {
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(request); err != nil {
		return fmt.Errorf("encode message command request: %w", err)
	}
	command := exec.CommandContext(ctx, provider.command, provider.args...)
	command.Stdin = bytes.NewReader(payload.Bytes())
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return &providerFailure{command: provider.command, stderr: stderr.String(), cause: err}
	}
	if err := decodeProviderJSON(stdout.Bytes(), response); err != nil {
		return fmt.Errorf("message command returned invalid JSON: %w", err)
	}

	return nil
}

func decodeProviderJSON(payload []byte, response any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(response); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("message command returned more than one JSON object")
	}

	return nil
}

func validateMessages(messages []wireMessage) error {
	for _, message := range messages {
		if message.ID == "" || message.From == "" || message.To == "" {
			return errors.New("message command returned a message missing id, from, or to")
		}
		if _, err := parseProviderTime(message.SentAt); err != nil {
			return fmt.Errorf("message command returned message %q with sentAt %q", message.ID, message.SentAt)
		}
	}

	return nil
}

func parseProviderTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}

	return parsed.UTC(), nil
}

func displayProviderTime(value string) (string, error) {
	parsed, err := parseProviderTime(value)
	if err != nil {
		return "", err
	}

	return parsed.Format(time.RFC3339), nil
}

func formatSentAt(now time.Time) string {
	return now.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
}

const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newMessageID(now time.Time) (string, error) {
	var id [16]byte
	milliseconds := uint64(now.UTC().UnixMilli())
	id[0] = byte(milliseconds >> 40)
	id[1] = byte(milliseconds >> 32)
	id[2] = byte(milliseconds >> 24)
	id[3] = byte(milliseconds >> 16)
	id[4] = byte(milliseconds >> 8)
	id[5] = byte(milliseconds)
	if _, err := rand.Read(id[6:]); err != nil {
		return "", fmt.Errorf("generate message id: %w", err)
	}

	return encodeULID(id), nil
}

func encodeULID(id [16]byte) string {
	var encoded [26]byte
	encoded[0] = crockfordAlphabet[(id[0]&224)>>5]
	encoded[1] = crockfordAlphabet[id[0]&31]
	encoded[2] = crockfordAlphabet[(id[1]&248)>>3]
	encoded[3] = crockfordAlphabet[((id[1]&7)<<2)|((id[2]&192)>>6)]
	encoded[4] = crockfordAlphabet[(id[2]&62)>>1]
	encoded[5] = crockfordAlphabet[((id[2]&1)<<4)|((id[3]&240)>>4)]
	encoded[6] = crockfordAlphabet[((id[3]&15)<<1)|((id[4]&128)>>7)]
	encoded[7] = crockfordAlphabet[(id[4]&124)>>2]
	encoded[8] = crockfordAlphabet[((id[4]&3)<<3)|((id[5]&224)>>5)]
	encoded[9] = crockfordAlphabet[id[5]&31]
	encoded[10] = crockfordAlphabet[(id[6]&248)>>3]
	encoded[11] = crockfordAlphabet[((id[6]&7)<<2)|((id[7]&192)>>6)]
	encoded[12] = crockfordAlphabet[(id[7]&62)>>1]
	encoded[13] = crockfordAlphabet[((id[7]&1)<<4)|((id[8]&240)>>4)]
	encoded[14] = crockfordAlphabet[((id[8]&15)<<1)|((id[9]&128)>>7)]
	encoded[15] = crockfordAlphabet[(id[9]&124)>>2]
	encoded[16] = crockfordAlphabet[((id[9]&3)<<3)|((id[10]&224)>>5)]
	encoded[17] = crockfordAlphabet[id[10]&31]
	encoded[18] = crockfordAlphabet[(id[11]&248)>>3]
	encoded[19] = crockfordAlphabet[((id[11]&7)<<2)|((id[12]&192)>>6)]
	encoded[20] = crockfordAlphabet[(id[12]&62)>>1]
	encoded[21] = crockfordAlphabet[((id[12]&1)<<4)|((id[13]&240)>>4)]
	encoded[22] = crockfordAlphabet[((id[13]&15)<<1)|((id[14]&128)>>7)]
	encoded[23] = crockfordAlphabet[(id[14]&124)>>2]
	encoded[24] = crockfordAlphabet[((id[14]&3)<<3)|((id[15]&224)>>5)]
	encoded[25] = crockfordAlphabet[id[15]&31]

	return string(encoded[:])
}
