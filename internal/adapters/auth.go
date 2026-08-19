package adapters

import (
	"fmt"
	"strings"
)

// How much of a failed harness's output to quote back. Enough to see the real
// message, bounded so a runaway log never becomes the error.
const outputTailBytes = 8000

func tail(value string) string {
	if value == "" {
		return "(empty)"
	}
	if len(value) <= outputTailBytes {
		return value
	}

	return value[len(value)-outputTailBytes:]
}

// authenticationError describes an unauthenticated harness and identifies the
// official command through which that harness owns authentication.
//
// Landing never collects or stores provider credentials: each harness owns its
// own authentication, so the message names the unauthenticated harness and its
// official authentication command without directing the caller to act.
func authenticationError(harnessName, loginCommand, stdout, stderr string) string {
	return strings.Join([]string{
		fmt.Sprintf("%s is not authenticated on this machine.", harnessName),
		fmt.Sprintf("%s authentication is owned by `%s`.", harnessName, loginCommand),
		fmt.Sprintf("stdout tail: %s", tail(stdout)),
		fmt.Sprintf("stderr tail: %s", tail(stderr)),
	}, "\n")
}
