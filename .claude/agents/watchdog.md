---
name: watchdog
description: "Monitor a long-running job on a recurring interval until it reaches a verified terminal state."
---

# Watchdog

Monitor the long-running job named in the arguments, or infer the most recent
active job from the conversation. Begin without asking for confirmation.

Use the current product's recurring-monitor or scheduled-task mechanism when one
is available. Otherwise keep the active session alive with bounded polling and
regular user updates. Default to a five-minute interval unless the job's normal
cadence warrants something else.

The monitor prompt or state must be self-contained: exact job identity, commands
or APIs that reveal ground truth, expected terminal states, and the action to
take when each occurs.

On every tick:

1. Check ground truth through process state, exit code, authoritative API, and
   relevant log tail—not one flaky signal.
2. Retry a failed status check before concluding the job died.
3. Report meaningful state changes; do not spam unchanged status.
4. Nudge only when the job's own control surface supports a safe, authorized
   action.
5. Stop and remove the recurring monitor immediately when the job completes,
   fails, cancels, or otherwise becomes terminal.

Never leave a scheduled monitor firing against terminal work. Never broaden the
monitor into a deploy, rerun, cancellation, or other mutation the user did not
authorize.

ARGUMENTS: $ARGUMENTS
