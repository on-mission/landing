# Deep review

Run an end-of-phase review of the exact changes named by the user and return one
evidence-backed synthesis. Review the diff, not the ticket description or the
current conversation's confidence.

## Resolve scope

Interpret the requested target precisely: uncommitted changes, a commit, a
range, a branch, or explicit paths. If the target is ambiguous, stop rather than
reviewing the wrong changes.

For an uncommitted review, include tracked changes and untracked files. Record
one canonical command for changed content and one matching names-only command so
every reviewer sees the same scope.

Protect concurrent work. Review and fix only files inside the resolved scope;
never stash, reset, restore, clean, or discard unrelated changes.

## Choose review lanes

Read the changed-file list and state which lanes run and which are skipped:

- `founding-engineer` — correctness, durability, and cross-cutting design
- `domain-engineer` — structural movement, naming, ownership, and abstraction
- `go-engineer` — Go surfaces: goroutine lifecycles, context and cancellation,
  error modeling, package and interface boundaries
- `typescript-engineer` — TypeScript or TypeScript-migration surfaces
- `testing-engineer` — always when behavior or tests change
- `security-engineer` — credentials, config, subprocesses, external input,
  installation, path or permission boundaries
- `systems-engineer` — external calls, async work, retries, timeouts,
  cancellation, state growth, concurrency, or capacity
- `frontend-engineer` — frontend component, state, and data-flow changes
- `ui-engineer` — rendered UI, interaction, accessibility, or visual-system
  changes
- pattern review — every changed code file against the matching leaves under
  `docs/patterns/`

A lane runs only when the diff contains its material. When close, run it; a miss
ships, while an extra review only costs a dispatch. A full-lane review is the
exception and must be justified by the actual seams touched.

## Review protocol

1. Dispatch independent lanes in parallel using fresh isolated specialist
   contexts. Require file:line evidence, a concrete failure or violated
   invariant, severity, and the smallest fix.
2. Reviewers perform two passes: permissive candidate recall, then explicit
   promotion or clearance. Silent candidate drops are not allowed.
3. Treat reports as evidence, not verdicts. Reject findings based only on grep,
   taste, or unsupported severity.
4. Reconcile genuine technical conflict with `council-runner`; do not average
   incompatible recommendations.
5. If the user requested review-and-fix, dispatch fixes to the owning implementer:
   frontend/UI to their engineer, tests to `testing-engineer`, and general code
   to `founding-engineer`. Security, systems, domain, and TypeScript roles remain
   review-focused.
6. Verify fixes using the project's documented composite command. If none exists,
   report that limitation and use the narrowest non-mutating checks available.
7. Re-review only the affected lane after a fix. Stop when the remaining signal
   is clean or requires a human decision; repeated subjective churn is not
   convergence.

Never edit author-owned docs during a review. Route a documentation finding to
its author and mark it deferred.

## Output

Return:

- severity totals;
- lanes run and skipped with reasons;
- findings with evidence, status, and verification;
- fixes applied only when authorized; and
- unresolved decisions or verification gaps.

ARGUMENTS: $ARGUMENTS
