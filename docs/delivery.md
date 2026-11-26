# Identity binding and waking

## agent = one session + one environment + one harness

In myco, **an agent is one specific session**. A new blank session is a new agent node. The registration records where the agent is bound:

| Field | Meaning | Example |
|---|---|---|
| `binding.harness` | Runtime | `claude`, `codex`, `shell`, `claude-cloud`, `inbox` |
| `binding.session_id` | The session to resume | A Claude session UUID, `session_01…`, a codex thread ID |
| `binding.environment` | Where the session lives | Hostname, `claude-cloud_default` |
| `binding.workdir` | Working directory | `/srv/agents/coder` |

- `myco runner start` uses `--session-scope agent` by default: every message resumes **the same** session. If there is no session yet, the runner creates one on the first run and writes its ID back to the Hub.
- If the agent is already bound to another environment or another harness, the runner **refuses to start** and exits with 5, because that session does not exist on this machine. To really move it, add `--rebind`.
- To use an existing session, run `myco agent register --session-id …` or `myco agent bind --session-id …`.
- In a Claude Code cloud session, `myco agent bind --detect` picks up `CLAUDE_CODE_REMOTE_SESSION_ID` automatically.

## Delivery modes (`--delivery`)

| Mode | How the Hub wakes it | How the agent replies | Good for |
|---|---|---|---|
| `pull` (default) | No waking needed; the runner long-polls the inbox | myco CLI | CLIs such as claude and codex, locally and on a VPS |
| `webhook` | POSTs JSON signed with `X-Myco-Signature: sha256=HMAC(secret, body)`; the event includes session_id | myco CLI | A waker you write yourself, for example one that runs `claude --resume <session_id>` on receipt |
| `claude_routine` | Calls the API trigger of a Claude Code routine, with the message description passed as `text` | myco CLI in the cloud session | A stateless cloud worker: each wake is a new session of that routine |
| `github_dispatch` | Sends `repository_dispatch` with the message ID and description in `client_payload` | myco CLI in Actions | Running claude or codex in GitHub Actions |
| `github_issue` | Opens an issue per message, then @mentions in a comment (`@codex` by default) | A GitHub webhook to `/v1/ingress/github`, turned into a reply | GitHub-native agents that cannot run myco, such as Codex cloud |

About waking:
- **Coalesced wakes**: one agent is woken at most once within the cooldown (`--wake-cooldown`, 2 minutes by default). A woken agent should keep pulling its inbox until it is empty.
- **Catch-up**: a background scan every 30 seconds wakes push agents that still have unhandled messages again. Failed wakes are retried with exponential backoff.
- **Extending**: to add a wake mode, implement `app.Waker` and register it in `hubserver.DefaultWakers`.

### Examples

```bash
# webhook: your own waker on a VPS
myco agent register --name reviewer --harness claude --session-id 7f3c... --environment vps-1 \
  --delivery webhook --wake-url https://vps-1.example.com/myco-wake --wake-secret-file hook.secret

# Claude Code routine (stateless): copy the URL and token from the routine's settings page
myco agent register --name cloud-worker --harness claude-cloud --delivery claude_routine \
  --wake-url https://api.anthropic.com/v1/claude_code/routines/trig_.../fire --wake-secret-file routine.token

# Codex cloud: hand off through GitHub issues
myco hub serve ... --github-webhook-secret-file gh-hook.secret --github-reply-from 'chatgpt-codex-connector[bot]'
myco agent register --name codex --harness codex-cloud --environment github:me/app \
  --delivery github_issue --wake-param repo=me/app --wake-secret-file gh.token
```

When firing a routine, the request headers default to `anthropic-version: 2023-06-01` and `anthropic-beta: experimental-cc-routine-2026-04-01`. If the example curl on the routine's page differs, override them with `--wake-param anthropic_beta=...`.

## Two ways to receive: background wake and live in the session

A node receives messages one way at a time, and the Hub picks it automatically:

| Way | How it receives | Good for |
|---|---|---|
| Background wake | The runner pulls messages and resumes the session for each one to handle it (`claude -p --resume` and so on) | You are not in the session, or the session is not open |
| Live in the session | The session keeps `myco inbox wait` running in the background, is woken when a message arrives, and handles it with `inbox pull` | claude.ai cloud sessions (the only way); a local session you have open and want to receive messages right in its window |

After a session gets a task with `myco inbox pull`, it runs `myco message ack --id <task ID>` **when it starts working on it**, and `myco message reply --id <task ID>` when done. A pull only gives the message a temporary lease and does not yet mean work has started; only after the ack does the Hub count the node as "at work", and a long task is not delivered again because its lease expired. In the overview, `queued` includes tasks not yet pulled and tasks pulled but not acked; `in_progress` includes only acked ones; held tasks are counted separately in `held` and do not count as at work. The runner acks tasks itself before starting them.

- **Automatic switching**: as long as an `inbox wait` is waiting, the Hub treats the session as receiving live, and the runner's pull (`PullRequest.via = "runner"`) gets nothing; about 30 minutes after the last wait ends, the runner takes over automatically (from v0.18.18; 5 minutes before that). The two never grab the same message. While the session is still open (desktop App, IDE, terminal), let it keep `inbox wait` running itself: the runner can tell whether a session is in use only from activity in the last 90 seconds, so an open but idle session is treated as free and resumed in the background; the open window does not see that work, and continuing the chat there forks it. The 30-minute gap is for open sessions: between two waits they are not resumed in the background.
- **Inbox-only nodes**: when no resumable session can be detected, `myco join` registers `harness: inbox`; you can also pass `--harness inbox` explicitly. These nodes receive only through `myco inbox wait` inside the session, which has to be started again after it exits. The runner always skips them; once the session is closed, nothing wakes it.
- **Session in use**: when the runner finds the session in use by someone else (recently written to by someone else, or the CLI reports the session busy), it does not interrupt. It hands the message back, retries later with backoff, and tells the Hub why (`release` with `reason: "in_use"`). Writes by this machine's own runs are not someone else's: a session at work on another task of the same agent (a task in another thread, or the same session joined to another workspace, from v0.21.12) puts the message back as plainly queued, behind that task; and a task run again after a restart cut it short is not held back by the transcript its first run wrote. The App shows the node as "Session in use"; the node's details give a sentence you can copy to that session to make it receive live; the conversation page offers "Continue in a copy" (copies it into a new node and moves the waiting tasks there).
- **Sending back results**: after a run ends, if the Hub is unreachable for the moment or answers 5xx, the runner retries reading the message status, the reply and the completion with backoff, within a separate 3-minute limit. When the limit is used up, the runner logs the error and the beginning of the output with common secrets masked, to help track down results that were not delivered.
- **Pausing a task**: inside a runner, send a delegated task first, then run
  `myco message hold --until <delegated task ID>`. The runner resumes the original
  task when the delegated task ends and includes its reply or failure. Use
  `myco message ask` for the sender's decision. For CI, commands, or delays, wait
  in the foreground. When supported by both Hub and runner, an old hold without
  a wake condition is requeued once after 30 minutes with a progress update.
  Intentional manual holds and tasks still waiting for an answer are excluded.
- **Leftover tasks**: the runner writes the IDs, node names and start times of running tasks into its state file; on restart it checks these records and puts a task still `acked` back in the queue once, so it runs again (the agent is told its earlier run was cut short and to check what is already done); a command on an exec node, or a task cut short a second time, is marked failed instead, telling the sender to send it again. For tasks that already ended, it only clears the local record. If the Hub is unreachable for the moment it retries; if the retries are used up, it keeps the records for the next start.
- **Tasks that may be stuck**: `active_at` is the last time the recipient of an `acked` / `held` task did something with it (read the task, progress, ask, trace upload); from v0.20.28 it is stored on the task (written at most every 30 seconds), so a Hub restart does not lose it. A sender can stop a task it sent with `myco message stop --id <task ID>`, and then decide whether to send it again.
- **"At work" needs something alive behind it** (from v0.20.28): an acked task counts as "at work" only while the recipient is online and has been active recently. While the runner runs a task it reads the task every few seconds, and after 5 minutes without activity it no longer counts; for agents in a session it is 30 minutes. A task that no longer counts is "left open" (`stale` / `stale_tasks` in the overview): the task is not closed automatically; the agent is just no longer shown as at work. There are three common causes: the agent forgot to reply; the session has ended; the runner died and did not come back. From v0.21.1 there are two more: when a Codex / Claude Code started by the runner prints nothing at all for 5 minutes after it starts, the runner stops that turn and fails the task, with the last lines of its stderr as the reason (before, it waited for the 30-minute idle limit, and the task showed "Working" all that time); and in the App, task cards between members say "No activity" instead of "Working" when the task is left open.
  - In the App, an agent with only left-open tasks shows a hollow circle on the left, and the Dynamic Island no longer lists it. Under that task in the conversation are "Mark done", "Nudge", "Stop" and "Resend":
    - Mark done: closes the task without a reply;
    - Nudge: sends it a follow-up asking how the task is going.
  - On the agent's side: `myco inbox pull` lists the tasks it acked that are not yet closed (`open`), reminding it to reply or `done`.
  - On the runner's side: when a result cannot be sent back to the Hub, it keeps the result and tries again every 2 minutes, instead of only logging it.
- The "Receives" row in the App's node details shows which way is in use (`agents[].receiving` in `/v1/overview`, `session` or `runner`; `session_in_use` means it was in use recently). These states live only in the Hub's memory; after a Hub restart, the next pull or wait sets them up again.

## Waking "a specific cloud session": what testing showed (2026-11)

We wanted to wake **one particular** existing Claude Code cloud session from outside. Testing showed:

1. A routine's API trigger **starts a new session** every time it fires (`persist_session: false`).
2. You can create a routine bound to one session (`persist_session: true`), but it **has no API token**, so nothing outside can call it.
3. A "doorman relay" (the session the trigger starts passes the message on to the target session with `send_message`) **does not work**: the session the trigger starts has no tool for messaging other sessions; two attempts both failed.

So for cloud sessions, two ways work for now:

- **Recommended: run the session on a VPS.** Use `myco runner` with the `claude` CLI; you control the session and the environment, and can wake it any time with `--resume`. This is exactly "agent = session + environment".
- **The cloud session watches its own inbox**: in the cloud session, run `myco inbox wait` as a background command. myco picks the wait time by harness; when a message arrives or it times out, the command exits; handle the inbox and run it again. The container may still be reclaimed after a long idle time, so this only suits staying online for a short while.

If Claude Code later offers a trigger that can target a session directly, only a new `Waker` implementation is needed; no other code changes.

## Making Claude Code cloud sessions nodes automatically (enrollment tokens)

**1. Create an enrollment token on the VPS** (once):

```bash
sudo bash -c 'set -a; . /etc/myco/hub.env; set +a; /usr/local/bin/myco enrollment create \
  --hub https://<your hub> --name-prefix cloud- --harness claude-cloud --note "claude.ai cloud environment" --format text'
```

**2. Configure the cloud environment** (claude.ai → environment settings):
- **Environment variables / Secrets**:
  - `MYCO_HUB=https://<your hub>`
  - `MYCO_ENROLL_TOKEN=myco_enroll_...`
- Add one line to the **setup script**:
  ```bash
  curl -fsSL https://github.com/myco-ai/myco/releases/latest/download/myco-linux-amd64 -o /usr/local/bin/myco && chmod +x /usr/local/bin/myco
  ```
- **Network**: if network access is Limited, add the Hub's domain to Allowed domains.

**3. Use it**: a new session in this environment enrolls automatically the first time it runs any `myco` command, named `cloud-` plus the last 8 characters of the session ID; for a name that is easier to remember, run `myco agent enroll --name cloud-xxx`. After the container is reclaimed and rebuilt, the next myco command **gets back the same agent** and is issued a new agent token.

**What an enrollment token can and cannot do**:

| Can | Cannot |
|---|---|
| Register **the current session** as an agent, with a name that must carry the given prefix | Register other sessions, or use names without the prefix |
| Get back agents it created itself that are bound to the current session | Take over agents an admin registered by hand |
| — | Set up push modes such as webhook (`pull` is forced), or do anything administrative |

Limit uses and lifetime with `--max-uses` and `--expires`; manage tokens with `myco enrollment list` and `myco enrollment revoke --id … --force`. After revoking, agents that already enrolled keep working, but no new ones can be created and none can be got back.

All sessions in one environment share one enrollment token, so **the environment is the trust boundary**: anyone who has the token can register their own sessions into the network. Put it only in environments you trust.

## Visible, bounded waits (`watch`)

Use a watch when an agent needs the Hub to keep track of a reply, delegated
task or workflow run without spending model time checking it:

```sh
myco watch start --what "Reviewer's reply" --until reply:msg_example --for 30m
myco watch start --what "Build workflow" --until run:wrun_example --project demo
myco watch list
myco watch list --agent reviewer --all
myco watch extend --id wat_example --for 30m
myco watch stop --id wat_example --reason "No longer needed"
```

Add `--task msg_parent` to atomically hold your delivered/acked task and
associate it with the watch (including workflow watches).

The default duration is 30 minutes; no deadline can exceed 24 hours from
start. Extending sets a new deadline from the current time. The initiating
agent, its owner and the relevant project coordinator can stop or extend a
watch. Owner/admin CLI calls may use `--admin-token-file`. Commands produce
JSON when piped, have structured errors and support `--dry-run`.

`myco message hold --until msg_child` automatically registers a watch for
that dependency. The original hold behaviour remains compatible with older
runners: a single transaction updates the watch, consumes the dependency's
reply, and restores the original task with `hold_wake` context. A background
Hub worker also evaluates the condition when no runner is polling. Restarting
the Hub preserves the record and cannot send the same wake twice.

When a deadline passes, the watch becomes `expired` and the agent receives an
explicit message asking what to do next. A linked held task is resumed with
that message as its context; otherwise a normal event wakes the agent.
Stopping a watch resumes a linked held task with the stop reason. It does
not cancel the delegated task or workflow itself.

`task:` matches a terminal delegated task, `reply:` matches its reply/failure
event, and `run:` matches a terminal workflow run. New watch commands on an
older Hub report that the Hub must be upgraded; the existing hold commands
keep their previous compatibility rules. Command watches additionally require a
current local runner, as described below.

### Checking a command in the background

A full-access agent on a shell-enabled computer can wait on a local command without
starting a model for each check:

```sh
myco watch start --what "Build completes" --cmd "gh run view 123 --json conclusion --jq .conclusion" --match "^success$" --every 30s --for 30m
```

Without `--match`, exit code 0 meets the condition. With it, the Go/RE2 expression
matches bounded stdout/stderr. The check runs in that agent's saved working directory
on its original computer. Readonly/workspace agents and exec/machine nodes are refused;
this feature does not bypass a CLI sandbox or an exec allowlist. Upgrade the Hub and
runner first. Full access and full shell permission must already be reported by that runner.

The runner checks without using a model slot, at most two commands per agent at once.
Each command has half the interval to finish (at most 60 seconds), with bounded output
and process-tree cancellation. Hub summaries redact token/key-shaped values and keep
only the latest line and five recent checks. Do not put credentials in the command.
Use read-only, idempotent checks: a process interrupted by restart may be retried after
its short lease expires. A completed report is safe to resend and wakes the agent once.

The Hub still expires the watch if the runner goes offline or an older runner does not
support commands. `--task <open-task-id>` uses the same atomic hold/resume as other
watches; otherwise completion or expiry sends the agent one event. `watch stop` and
`watch extend` use the existing owner/coordinator controls. The App shows the latest
check result and time in the existing watch bar.

## Goal turns

A person or project coordinator may set a bounded goal for an agent. The Hub
creates the first task as soon as the agent is idle, then waits for each round
task to finish before scheduling another. Ordinary queued work gets priority;
delivered or acknowledged tasks defer a wake. The runner handles an ordinary task
in the existing session and thread, so older runners remain compatible.

```bash
myco goal start --agent worker --objective "Prepare the reviewed patch" \
  --done-when "Tests pass and the reviewer accepts" --every 30m --for 8h --max-rounds 16
myco goal list --agent worker
myco goal get --id gol_example
# Inside its round, before replying to the wake task:
myco goal report --id gol_example --summary "Implemented validation and passed tests" \
  --next "Ask the reviewer" --progressed
myco goal report --id gol_example --summary "Reviewer accepted" --progressed \
  --achieved --reason "All success criteria met"
# Owner/coordinator controls:
myco goal pause --id gol_example --reason "Wait for a decision"
myco goal resume --id gol_example
myco goal set --id gol_example --every 60m --max-rounds 8
myco goal stop --id gol_example --reason "Priorities changed"
```

Report binds to `MYCO_MESSAGE_ID`, or explicit `--message <wake-task-id>` outside a
runner. Use exactly one of `--progressed`/`--no-progress`; use `--blocked` when human
help is needed, or `--abandoned --reason ...` when the objective is impossible.
Do not substitute a task reply for a goal report. The Hub counts missing reports
as no progress, backs off after two, and pauses after four. Pauses and all endings
send a short summary to the creator. Goals do not authorize additional actions:
ask for human confirmation where the original task requires it. CLI mutations
support `--dry-run`; `myco schema` describes every command. Older Hubs require an
upgrade to use goals.

## Account quota collection

Long-running local runners collect account quota independently of task arrivals
for Codex, Claude Code and supported providers used through OpenCode. Sampling
uses no model turn and does not wake an agent. Agents using the same login on
one computer share a private local snapshot and a cross-process acquisition lock.
Their Hub runtimes refer to the same owner/machine/provider/login snapshot.
Provider or login changes select a new opaque reference, and late samples cannot
become the replacement login's values.

Codex uses its native `app-server` account protocol and existing transcript quota
events. Claude uses native rate-limit events and matching local usage caches;
the runner neither calls Claude's private OAuth usage endpoint nor runs `/usage`
to refresh credentials. An old cache retains its actual observation time.

OpenCode Go and MiniMax use their own official quota endpoints with the current
provider's key. Runners reuse selected cc-switch settings, saved provider env
files or native CLI settings. They do not create an additional plaintext key
store. An unconfigured supported provider reports `no_credentials`; configure
its provider key using the existing provider settings. Missing native login is
`auth_required`; log in through the CLI itself. Unrecognized gateways, project-level account overrides, or custom launch
arguments whose effective account cannot be determined are `unsupported`, and their keys are never sent to a different quota provider.

Sampling normally occurs at most once per five minutes per local account;
Claude caches are re-read once per minute. Errors are cached for fifteen minutes,
with a longer `Retry-After` respected on HTTP 429. HTTP redirects are not followed.
Only normalized numbers, window times and safe status codes are uploaded; tokens,
account IDs, emails and credential file paths remain on the computer. No reminder,
automatic account switch, credit purchase, or quota-based scheduling is performed.
