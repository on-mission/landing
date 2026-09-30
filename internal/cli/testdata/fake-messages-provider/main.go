// Test fake for Landing's messages provider protocol. It is not a product command.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	if os.Getenv("LANDING_FAKE_PROVIDER_SLEEP") == "1" {
		select {}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dir := os.Getenv("LANDING_FAKE_PROVIDER_DIR")
	if dir == "" {
		return errors.New("LANDING_FAKE_PROVIDER_DIR is not set")
	}
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var request map[string]json.RawMessage
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("stdin is not one JSON object: %w", err)
	}
	if decoder.More() {
		return errors.New("stdin has more than one JSON object")
	}
	if err := appendRequest(dir, payload); err != nil {
		return err
	}
	if failed, err := consumeFailure(dir); err != nil {
		return err
	} else if failed {
		message := "provider failed"
		if text, readErr := os.ReadFile(filepath.Join(dir, "stderr.txt")); readErr == nil && len(text) > 0 {
			message = strings.TrimSuffix(string(text), "\n")
		}
		return errors.New(message)
	}
	var op string
	if err := json.Unmarshal(request["op"], &op); err != nil {
		return fmt.Errorf("request op: %w", err)
	}
	switch op {
	case "send":
		var id string
		if err := json.Unmarshal(request["id"], &id); err != nil || id == "" {
			return fmt.Errorf("send id: %w", err)
		}
		return encode(map[string]string{"id": id})
	case "wait":
		return writeResponseFile(dir, "wait.json", `{"messages":[{"id":"01TESTMESSAGE000000000000","from":"laptop","to":"office","body":"hello from the bus","sentAt":"2026-09-29T22:14:03.000Z"}]}`)
	case "ack":
		var ids []string
		if raw, ok := request["ids"]; ok {
			if err := json.Unmarshal(raw, &ids); err != nil {
				return fmt.Errorf("ack ids: %w", err)
			}
		}
		return encode(struct {
			Acked []string `json:"acked"`
		}{Acked: ids})
	case "list":
		return writeResponseFile(dir, "list.json", `{"recipients":[{"name":"office","since":"2026-09-29T18:02:11Z"}]}`)
	default:
		return fmt.Errorf("unsupported op %q", op)
	}
}

func appendRequest(dir string, payload []byte) error {
	line := bytes.TrimSpace(payload)
	if len(line) == 0 {
		return errors.New("stdin is empty")
	}
	file, err := os.OpenFile(filepath.Join(dir, "requests.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		closeErr := file.Close()
		return errors.Join(err, closeErr)
	}

	return file.Close()
}

func consumeFailure(dir string) (bool, error) {
	path := filepath.Join(dir, "failures_left")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	remaining, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false, err
	}
	if remaining <= 0 {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(remaining-1)+"\n"), 0o600); err != nil {
		return false, err
	}

	return true, nil
}

func writeResponseFile(dir, name, fallback string) error {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		_, writeErr := os.Stdout.Write([]byte(fallback + "\n"))
		return writeErr
	}
	if err != nil {
		return err
	}
	if _, err := os.Stdout.Write(data); err != nil {
		return err
	}
	if bytes.HasSuffix(data, []byte("\n")) {
		return nil
	}
	_, err = os.Stdout.Write([]byte("\n"))

	return err
}

func encode(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)

	return encoder.Encode(value)
}
