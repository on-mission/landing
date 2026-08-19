// Package adapters translates Landing's execution contract into the behavior of
// each supported harness. Provider-specific authentication, requests, capacity
// interpretation, and failure classification stay behind this boundary.
package adapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// How long a capacity probe may take before it is abandoned as unreadable. A
// probe is an optimization: it never gates execution, so it must never be the
// reason a dispatch feels hung.
const rpcTimeout = 10 * time.Second

// rpcStep is one message in a scripted JSON-RPC exchange over a harness's stdio
// transport. A step with no ID is a notification: it is written and the script
// moves on immediately. A step with an ID is a request, and the script blocks
// until a response carrying that ID comes back.
type rpcStep struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Method  string `json:"method"`
	// Params is the one place an untyped value is correct: these are
	// provider-defined handshake payloads being encoded outward at the process
	// boundary, not external data travelling inward into Landing's domain.
	Params any `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func request(id int, method string, params any) rpcStep {
	return rpcStep{JSONRPC: "2.0", ID: &id, Method: method, Params: params}
}

func notification(method string, params any) rpcStep {
	return rpcStep{JSONRPC: "2.0", Method: method, Params: params}
}

// jsonRPC runs a scripted request/response exchange against a harness spawned on
// its stdio transport and returns the result of the final request.
//
// The CLI door into these harnesses is lossy — it formats for humans and drops
// the numbers a capacity read needs. The stdio JSON-RPC door returns them
// structurally, which is why probing goes through here rather than through the
// harness's own subcommands.
func jsonRPC(ctx context.Context, command string, args []string, env []string, steps []rpcStep) (json.RawMessage, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("jsonRPC requires at least one step")
	}

	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()

	process := exec.CommandContext(ctx, command, args...)
	process.Env = env

	stdin, err := process.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open %s stdin: %w", command, err)
	}

	stdout, err := process.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open %s stdout: %w", command, err)
	}
	var stderr bytes.Buffer
	process.Stderr = &stderr

	if err := process.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	// The child is killed by CommandContext when ctx ends; Wait releases its
	// pipes and reaps it either way.
	defer func() {
		_ = stdin.Close()
		cancel()
		_ = process.Wait()
	}()

	// Send everything up to and including the first request, then alternate:
	// each response advances the script to the next request.
	next, err := sendThroughNextRequest(stdin, steps, 0)
	if err != nil {
		return nil, err
	}

	awaiting := *steps[next-1].ID
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var observed strings.Builder

	for scanner.Scan() {
		if observed.Len() > 0 {
			observed.WriteByte('\n')
		}
		observed.Write(scanner.Bytes())
		var message rpcResponse
		// A harness may interleave non-JSON banners with its JSONL responses.
		// A line that does not parse, or that answers a different request, is
		// not ours to act on.
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}
		if message.ID == nil || *message.ID != awaiting {
			continue
		}
		if message.Error != nil {
			return nil, fmt.Errorf("%s JSON-RPC error %d: %s", command, message.Error.Code, message.Error.Message)
		}
		if next == len(steps) {
			return message.Result, nil
		}

		next, err = sendThroughNextRequest(stdin, steps, next)
		if err != nil {
			return nil, err
		}
		awaiting = *steps[next-1].ID
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s JSON-RPC: %w", command, err)
	}

	detail := strings.TrimSpace(observed.String() + "\n" + stderr.String())
	if detail == "" {
		return nil, fmt.Errorf("%s exited before answering its JSON-RPC request", command)
	}

	return nil, fmt.Errorf("%s exited before answering its JSON-RPC request: %s", command, detail)
}

// sendThroughNextRequest writes steps starting at from, stopping once it has
// written a step that expects a response. It returns the index after that step.
func sendThroughNextRequest(stdin interface{ Write([]byte) (int, error) }, steps []rpcStep, from int) (int, error) {
	for index := from; index < len(steps); index++ {
		encoded, err := json.Marshal(steps[index])
		if err != nil {
			return 0, fmt.Errorf("encode JSON-RPC step %d: %w", index, err)
		}
		if _, err := stdin.Write(append(encoded, '\n')); err != nil {
			return 0, fmt.Errorf("write JSON-RPC step %d: %w", index, err)
		}
		if steps[index].ID != nil {
			return index + 1, nil
		}
	}

	return 0, fmt.Errorf("JSON-RPC script ends with no request awaiting a response")
}
