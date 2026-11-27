# Managed agent copies

A coordinator can copy a bound Claude Code or Codex agent on a machine that permits that
coordinator to spawn sessions. The original agent keeps its session. Each copy has immutable
Hub provenance (`fork_of`, `forked_by`, optional `for_task`), independent of editable labels.
The machine runner must be upgraded and use `runner start --all`.

```sh
myco agent fork --from coder --count 2 --worktree --for-task msg_example
myco agent retire --fork-of coder
```

Output is JSON when piped. `fork` returns a batch `request_id` and compact copy records with
IDs, names, state and working directories. A batch contains at most 32 copies. Save the
request ID and pass it as `--idempotency-key` when retrying an interrupted batch: retries
return the same reservations. Use `--name-prefix` to choose a shorter naming prefix.

The source needs a session ID, working directory and machine binding. A copy cannot create
more copies, and a copy cannot be used as another fork's source. Without `--force`, the
source's quota status must be `ok`, its runner slot report must be fresh, and enough slots
must be free. Pending and live managed copies reserve capacity conservatively. `--force`
overrides only these resource checks, never permissions, runner support or directory policy.
Private source visibility and sender restrictions are preserved. The source identity and
permissions are checked again before a copy first reads its inherited context.

`--worktree` gives each Codex copy a separate sibling checkout and `fork/<copy-name>` branch
based on the source repository's current `HEAD`. Uncommitted source changes are not copied.
Both the source directory and the sibling's parent must be allowed by the machine's
`spawn.dirs`. Local Git checkout hooks and fsmonitor programs are disabled for provisioning.
Claude Code 2.1.314 looks up resumed sessions by directory; managed Claude forks therefore
support the original directory only and reject `--worktree` with `unsupported`.

```sh
myco agent fork --from reviewer --count 2            # same directory
myco agent fork --from coder --worktree --dry-run    # Hub-side preflight, no reservation
myco agent retire --name coder-fork-example --dry-run
myco agent retire --name coder-fork-example --now
```

Dry-run validates Hub-side permissions and resources; the machine independently validates
its local paths and current profile when the operation actually runs.

Retirement is allowed only for a registered managed copy, by its original creator identity,
the machine's owner, or a Hub administrator. A label that resembles fork provenance grants
no permission. The Hub first stops new work from being claimed. Work already claimed may
finish, including after a runner restart. `--now` explicitly cancels it instead. The machine
waits for the copy's worker and owned processes to stop, then removes its own recorded
worktree and profile before the Hub deletes that exact agent identity. The Git branch is
retained for review and merging. A changed identity, branch, path or machine owner causes a
refusal. Uncommitted worktree changes are preserved; resolve them and retry retirement.

Copies without queued or open work retire automatically after 30 minutes without a message
exchange. Runner presence polls do not reset that timer. Cleanup still applies the same
ownership, directory and dirty-worktree checks. A failed cleanup records an error and waits
for an explicit retry, rather than discarding work. A CLI timeout does not cancel an already
recorded retirement; repeat the retire command to observe or retry it.

An old Hub or machine runner returns a clear `unsupported` error. Ordinary agents and their
existing spawn, messaging and removal commands keep their behavior.
