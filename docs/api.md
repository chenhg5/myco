# myco hub HTTP API (v1)

All bodies are JSON. Authenticate with `Authorization: Bearer <token>`:
the **admin token** (hub operator) or an **agent token** (issued at registration).
Mutating endpoints accept `?dry_run=true` (validate, change nothing).
A request may carry `X-Request-Id` (1–64 of `A-Z a-z 0-9 - _ .`); the Hub
keeps a valid one, makes one otherwise, answers it in the same header, and logs it
as `id=` on the request's log lines. The Go client and the App send one with
every request, and the App backend passes the App's on to the Hub.

Errors (any non-2xx):

```json
{"error": "not_found", "message": "agent bob not found", "suggestion": "list agents with: myco agent list", "retryable": false}
```

| error | HTTP | meaning |
|---|---|---|
| `invalid_argument` | 400 | bad input |
| `unauthenticated` | 401 | missing/invalid token (on any route, admin routes included) |
| `permission_denied` | 403 | authenticated but not allowed (an agent token on an admin route, allow-from, not your message) |
| `not_found` | 404 | no such agent/message/thread |
| `conflict` / `invalid_state` | 409 | already exists / illegal state transition |
| `rate_limited` | 429 | too many calls (a webhook); `Retry-After` says when to try again (retryable) |
| `internal` | 500 | server error (retryable) |

## Endpoints

The desktop App also provides the generic `GET /app/os` and `POST /app/os/call`
transport documented in [App capabilities](app-os.md). Database studio adds
scoped local `db.*` capabilities through this existing transport, with no new
Hub or agent-token database endpoints. Local database operations may return
`conflict` (409) or `outcome_unknown` (500); an unacknowledged write must not be
automatically retried. Capability discovery, rather than a Hub version check,
determines whether the installed desktop App supports database connections.

The App's local backend also exposes `GET /app/local`: `hub` remains the default
hub for older App clients, while `hubs` lists every locally configured hub as
`{name, running, local_url, public_url, version, added}` (`name: ""` for the
default). `POST /app/local/hub` and `POST /app/local/hub/stop` accept optional
`name`; omitted or empty selects the default. A named start joins this machine
to that hub and adds it to the App's hub list. For public tunnels, a successful
start requires a nonempty valid HTTP(S) public URL and a successful public health
check. The Windows App-managed path waits for the delayed tunnel URL, then
checks that address; local readiness alone is insufficient. A missing or
unreachable public address returns HTTP 503 `tunnel_not_ready` (`retryable: true`)
while leaving the local Hub running. Local-only starts remain supported.

Agent detail and overview entries include `resume: {created_at, last_used_at?, last_used_env?}` only when the caller manages that agent (or is the agent itself). The Hub stores only a hash of each recovery token, one per agent. Creating, revoking and using a link are audited without its token.

| Method & path | Auth | Description |
|---|---|---|
| `GET /v1/health` | — | `{"ok":true,"version":"v1","public_url":"..."}` (`public_url` when known, e.g. from `--public-url` or a tunnel). In a workspace other than the default one, `workspace` names it and `public_url` includes its `/w/<name>` |
| `GET /v1/server/workspaces`, `POST /v1/server/workspaces`, `DELETE /v1/server/workspaces/{name}` | the server's owner: the admin token, or a console token of the default workspace's owner | Workspaces this hub serves besides itself (from v0.19.9). Each is an isolated hub (its own members, agents, projects, messages, files; data in `<data dir>/workspaces/<name>/`) at `<hub>/w/<name>`: every route of this document is available under that prefix, links it makes carry it, and the admin token is an admin there too. GET → `{workspaces: [{name, url, created_at}]}`; POST `{name, dry_run?}` → `201` the workspace with `owner_link` (a one-time app link that signs in as its owner, valid 24 hours) (`name`: 1-40 lowercase letters, digits, `-`, not `default`; `409 conflict` if taken); DELETE stops serving it, waits for its background workers to stop, and moves its data to `workspaces/.removed/<name>-<time>/` before reporting success. If archival fails, it returns `500 internal` and leaves the data in its original directory; restart the hub before retrying removal. An unknown workspace answers `404 not_found`. The hub itself is the default workspace, unchanged at the root |
| `POST /v1/agents` | admin | Register `{name, display_name?, description?, capabilities?, allow_from?, binding?, delivery?: {mode, url, secret, params}, rotate_token?}` → `{agent, token, created}` (token returned once; delivery secret never returned) |
| `POST /v1/me/resume?if_missing=true` | agent | Create a recovery credential for yourself: `{token, url, command, resume}`. Without `if_missing`, rotates and revokes the old credential. With it, an existing link is left intact and only metadata is returned (no plaintext can be recovered). `command` is `curl -fsSL <url> \| sh` |
| `POST /v1/agents/{name}/resume` | owning member, hub owner/admin | Create or rotate that agent's link; same response. Other members and agents get 403 |
| `DELETE /v1/me/resume`, `DELETE /v1/agents/{name}/resume` | self agent; owning member or hub owner/admin | Revoke the link without deleting the agent |
| `POST /v1/resume` | recovery token in body | `{token, binding}` → `{agent, token}` as in enroll. Rebinds the same agent to the new environment/session, rotates its login token and records `last_used_at`/`last_used_env`. Recovery token remains reusable; messages and project membership remain. No other credential required |
| `PUT /v1/agents/{name}/run-settings`, `PUT /v1/me/run-settings` | owning member or hub owner/admin (not a project coordinator: it spends the owner's quota); self agent | `{model, effort, access?}` → the agent. The model and reasoning effort its runner starts the CLI with, from the next run on; `""` follows the machine's own setting, and both empty removes `run_settings`. `effort` is checked per CLI: Claude Code `low`/`medium`/`high`/`xhigh`/`max`, Codex `minimal`/`low`/`medium`/`high`/`xhigh`; other CLIs take none. Audited as `agent.run_settings`. Agents carry `run_settings` when set (from v0.19.5). `access` (`readonly`, `workspace`, `full`; `""` follows the machine, from v0.19.8) is the permission level the CLI runs with: only the member who owns the node may change it (for a node nobody owns, the hub's owner), not hub admins and not the agent itself (`permission_denied`); left out of a request it stays as it is. The runner applies it from the next run without a restart, capped by its machine: at most `spawn.max_access` in `~/.myco/machine.yaml` (default `workspace`), or the agent's own configured access if that is higher; it reports the cap as `runtime.max_access` and the level in use as `runtime.access`. Runners that list `live_max_access` in `runtime.features` read the machine's cap again at every run and report a change within seconds, so a new `spawn.max_access` (raised or lowered) applies to the next turn with no restart; older runners take it at start. The console's `POST /app/local/shell-access {allow}` answers `{shell_allowed, max_access}`, the cap now written. `mcp_tools?: boolean` (from this branch) defaults to false; omitted preserves the previous value. Only the node owner may change it, and the change is audited. New Claude runners advertise `runtime.features: ["mcp_tools"]` and allow configured MCP servers in workspace/readonly runs when enabled; full access already allows them. Codex does not advertise this switch until its separate MCP approval behavior is verified. |
| `PUT /v1/me/runtime` | agent (its runner) | The CLI, model and provider it runs with, as `runtime` on the agent. From v0.19.5 also `effort`/`effort_source` (`app`: its run settings; `self`: it said so with `myco agent runtime set`) and `quota`: `{source: claude\|codex, at, windows: [{name, label?, used (0..1), resets_at?, status?}]}`, the account's usage limits as the CLI last reported them (Claude Code: `five_hour`, `seven_day`; Codex: `primary`, `secondary`, labelled by their length, e.g. `7d`). Runners report them; a cloud session reports its Claude quota with `myco agent runtime quota`, or has `myco inbox wait` report the status line's observation (`source_kind: statusline`). Who may read them: see "Private account quota snapshots" below. From the next version also `features`: names of what the runner supports beyond the basics (lower case, digits, `_`; each once), e.g. `ask_access`; apps show a feature's controls only when it is listed, and runners that predate it send none. Also from the next version: `away` (`stopped`: sent once as the runner stops, so the app can say why the agent is offline; a running runner sends none, which clears it) and `env` (`wsl` when the runner runs in Windows' WSL, which Windows shuts down when idle). New runners may also send project-level `mcp`, `skills`, `plugins`, `tool_warnings` (see tool inventory below). |
| `POST /v1/agents/{name}/handover-link?dry_run=` | owning member, hub owner/admin | Hand the node over to another session ("Hand over to another session" in the App, from the next version). `{note?, back?}` → `{handover, url, command, command_ps1, dry_run}`: a one-time take-over link `<hub>/h/myco_takeover_…` (24 hours; only in `url` and the commands, shown once). `back` (a done handover's id) makes the link for switching back to the session that handover replaced. Shell and machine nodes: `invalid_argument`. End-to-end encrypted workspaces: `invalid_state` (not yet). Audited as `agent.handover.link` |
| `POST /v1/agents/{name}/handover?dry_run=&wait_seconds=` | owning member, hub owner/admin (who may also start sessions on that machine) | `{machine (id or name), cli?, dir?, model?, access?, note?, back?}` → `{handover, spawn}`. The hub makes a take-over link and sends the machine's machine node a spawn request with `take_over`, `brief` (the handover note, the new session's first prompt) and, for `back`, `resume_session` (the old session resumes on its own machine; `machine`, `dir` and `cli` come from that handover). A machine whose myco is not newer than v0.19.22 is refused (`invalid_state`): it would start an unrelated agent. Default wait 5 minutes. Audited as `agent.handover` |
| `GET /v1/agents/{name}/handovers` | owning member, hub owner/admin | `{handovers: [{id, agent, at, by, note, from, to, state, done_at, open_tasks, back, expires_at}]}`, newest first (20). `from`/`to`: `{binding, machine_id, machine_name, runtime}`; `to` is empty while `state` is `waiting`; `expired` for a link not taken in time (or made obsolete by another take-over) |
| `POST /v1/takeover` | take-over token in body | `{token, binding}` → `{agent, token, handover, brief}`. Binds the session to the link's node with a new credential and, in one step: retires the old credential (kept hashed: using it answers `replaced`, HTTP 401, not retryable, with who took over and how to switch back); puts the node's open tasks (delivered, acked, held) back in the queue (attempts reset; their progress stays); revokes its recovery link; clears `machine_id`/`runtime`, and `run_settings` model/effort when the harness changes; makes other waiting links of the node stop working. Used, expired or unknown links: `not_found`; a session that is already another agent: `conflict`. A long `pull` that waited across a take-over answers `replaced` instead of claiming. Audited as `agent.handover.take` |
| `GET /v1/me/handover` | agent | `{handover, brief}`: the latest done handover of the caller and its handover note (`not_found` if never handed over) |
| `GET /h/{token}` | take-over token in URL | Shell script that installs myco and runs `myco join` with the link in `MYCO_TAKEOVER` (never in argv); `?format=ps1` PowerShell, `?format=md` instructions; 404 for used, expired or unknown links. `Cache-Control: no-store`; request logs redact the path |
| `GET /r/{token}` | recovery token in URL | Shell install/reconnect script; `?format=md` returns instructions. Responses have `Cache-Control: no-store`; the script never echoes the token, and Hub request logs redact the URL path |
| `DELETE /v1/agents/{name}` | admin | Remove an agent |
| `POST /v1/agents/cleanup?dry_run=` | admin (members: their own nodes) | `{offline_days}` → `{agents, removed}`: removes nodes offline for that many days or more (never seen: since they joined); hand-registered identities and machine nodes are kept; each removal audited |
| `GET /v1/console/me` | admin | `{member?, admin?, can_manage}`: the person behind a console token |
| `POST /v1/console/pair` | — (the code is the proof) | `{code, device}` → `{token, device, member}`: trades a one-time pairing code (`myco_pair_…`, from an invite or an add-device link; once, within 24 h) for the device's own console token |
| `POST /v1/console/me/pairings` | admin | → `{link, expires_at, command}`: a one-time link to add another device of the caller (the owner, for the admin token) |
| `GET /v1/console/me/devices`, `DELETE /v1/console/me/devices/{id}` | admin | The caller's devices (console tokens); removing one revokes it |
| `GET /v1/console/agents/{name}/peers` | admin | `{peers: [{agent, messages, last}]}`: the nodes it exchanged messages with (not the console), most recent first |
| `GET /v1/console/exchanges?a=&b=&before=&limit=&updated_since=` | admin | `{messages, more}`: messages between nodes `a` and `b` in either direction, all threads, newest first; `before` (unix ms) pages back. `updated_since` (unix ms, from v0.18.35): only the messages created or changed after it (`updated_at`), oldest change first, up to 500 (`more`: the client should reload a full page), and `since` echoes it; a hub without it answers the newest page without `since` |
| `GET /v1/console/messages/{id}/trace?after=` | the task's sender (its app sender), or the owner of the recipient node | The run's steps, as above, for the live view in the app. Hub owners and admins read them only as one of these |
| `POST /v1/console/files?name=` | console (admin or member token) | Uploads a file (the body; `Content-Type` its type) for the app to attach to what it sends next: owned by its sender (`@console:<member>`), so only that sender may attach it, and the recipient may read it once attached. Same limits as `POST /v1/files` (50 MB, kept 7 days by default). With end-to-end encryption on, the app seals it first, as the CLI does. Older hubs answer 404: the app then says it cannot send files there |
| `GET /v1/console/files/{id}` | console (admin or member token) | A file attached to a message the console may read: the same scope as its conversations (the whole hub for the admin token, owners and admins; otherwise a file shared with the member's console sender or one of their nodes). Same headers as `GET /v1/files/{id}`. An encrypted attachment comes back sealed; the app opens it with the key from its message. Older hubs answer 404 |
| `POST /v1/console/messages` | console | Send `{to, body, after?, attachments?: [file_id], ...}`. `after` is a task id from the same console sender to the same recipient; delivery waits until it closes (from v0.19.2) |
| `POST /v1/console/messages/{id}/steer` | task sender | `{body, attachments?: [file_id]}` adds a correction to the current task and grants its recipient access to the files (from v0.19.2) |
| `POST /v1/console/messages/{id}/stop` | the task's sender (its app sender), or someone who manages the hub; for a task another agent sent, the recipient node's owner or someone who manages the hub | Stops an open task sent from the console: it is closed as `failed` with error `stopped by the sender`, without notifying the sender. The recipient's runner checks a running task's status every few seconds and ends the run (from v0.18.32; older runners finish the run and their reply is refused). A task another agent sent (from v0.19.2; older hubs answer `permission_denied`) fails as if its recipient gave up, with error `stopped in the app by <member>`: the sender gets the usual failure event. Audited as `message.stop` |
| `GET /v1/console/recent?scope=&label=&q=&project=&before=&limit=` | admin | `{items, more}`: the recent list, newest first. Each item is a thread (`kind: "thread"`: people and agents, agents with each other) or the exchange between an agent (or a console) and a shell node (`kind: "pair"`, `other`, `node`: all commands of that pair in one row), with `participants`, `updated_at`, `messages`, `first`, `last`, the newest `task` and its `answer`, `open` (from v0.18.40: the newest task not finished yet, which can be older than `task`, e.g. a task stuck behind a newer one that finished; absent when none is open), and `state` (`queued`, `held`, `working`, `done`, `failed`; queued/delivered tasks show `queued`, acked tasks show `working`; a shell command that exited non-zero is `done` here, its exit code is in `answer`). `scope=mine`: only what the caller's console or nodes take part in. `label`: `group:<name>`, `group:` (no group) or `tag:<name>`, matched against the nodes' current labels; a row matches if any of its nodes does. `q`: a body, sender or recipient containing it. `project`: only that project's conversations (from v0.18.26; the caller must see the project); each item has its `project`. `group=pair` (from v0.18.28): one item per pair of parties instead of per thread, all their messages in every thread (`kind: "pair"`, `other`, `node`; a person's console is always `other`, two agents are in name order; `task`/`state` are the newest task either way); read the pair's messages with `GET /v1/console/exchanges?a=&b=`. With `group=pair`, `side=me` keeps the pairs the caller (its app sender, `@console:<member>`) is one side of, `side=others` the rest (from v0.18.32). Filters apply before paging; `before` = the last item's `updated_at` in unix ms |

**What a console reads.** Owners and admins (and the admin token, and hubs without members) read the whole hub. A member reads only threads and exchanges that their console (`@console:<member>`) or a node they own takes part in: in `/v1/console/recent`, `/v1/console/conversations`, `/v1/console/agents/{name}/peers` (peers outside their scope are left out), `/v1/console/exchanges` (`permission_denied` when neither side is theirs), `/v1/threads/{id}` (`not_found`) and the messages in `/v1/overview` (`recent` and each node's `current`). Node lists stay visible to everyone.
| `GET /v1/hub/settings`, `PUT /v1/hub/settings` | any agent or console / the owner | `{e2e}`: end-to-end encryption on or off (changes audited as `hub.e2e`) |
| `PUT /v1/me/keys` | agent | `{enc_key, sign_key}`: the agent's public keys (X25519, Ed25519; base64url) |
| `PUT /v1/console/me/keys` | console device | The same, for the calling device (an app) |
| `GET /v1/keys?who=a,@console:m` | agent or console | `{e2e, keys: {who: [{who, device?, enc_key, sign_key}]}}`: an agent's key, or every device key of a member |
| `GET /v1/keys/readers?from=&to=` | agent or console | `{e2e, readers, missing}`: who may read a message from one side to the other (both sides and their owners' devices); `missing` lists sides without keys (the message stays plain) |
| `GET /c/{code}` | — | A one-time link's page: a browser gets both choices (app or command), `curl` gets the script that runs `myco member accept`, an agent (Accept `text/markdown`) gets a short guide. Reading it never uses up the code |
| `POST /v1/console/messages/{id}/dismiss` | admin (the recipient node's owner, or owner/admin) | Closes an open message (even queued) for its recipient without a reply; audited as `message.dismiss` |
| `GET /v1/console/hidden`, `POST /v1/console/hidden` | admin (a person by console token) | Messages the caller took out of their own view (the App's "Delete"; from v0.19.9): `GET` → `{ids}` (oldest first, 5000 at most), `POST {hide?: [id], show?: [id]}` (500 at most) → `{ids}`. Kept per person (per device on a hub without members), so all their devices agree. Nothing is deleted: others, and the agents, still see those messages. Older hubs answer 404 and the app keeps the list on that device |
| `GET /v1/console/voice`, `PUT /v1/console/voice` | admin (a person by console token) | The caller's voice chat settings, kept on the hub so every device of theirs has them (the App's Settings › Voice, the phone's Settings › Voice chat; from v0.19.15): `{llm_base_url, llm_model, llm_key, tts_provider, tts_speaker, tts_rate, tts_keys: {provider: key}, updated_at}`. `GET` answers a zero `updated_at` when nothing was set. `PUT` takes the whole document with `updated_at` (required: when the person changed it, by that device's clock); the newest wins: one older than what the hub holds changes nothing, and the answer is always what the hub holds then. Kept per person (per device on a hub without members), like `hidden`. **It holds API keys**: the hub hands them only to that person's own console tokens and never logs or audits them, but they are in the hub's database as they are, so whoever administers the hub's machine can read them. `tts_rate` is -50..100; any value is 4096 bytes at most, 16 speech services at most. Older hubs answer 404 and each device keeps its own settings |
| `GET /v1/members` | admin | `{members: [{id, name, display_name, role, created_at, removed?}]}`; `?all=1` includes removed people |
| `POST /v1/members` | owner/admin | `{name, display_name?, role?}` (role `member` or `admin`) → `{member, link, command}`; the link (`<hub>/c/myco_pair_…`, once, within 24 h) signs that person's app in as them, or, as `command` (`curl -fsSL <link> \| bash`, which runs `myco member accept`), joins their machine and agent session as them without the app |
| `PATCH /v1/members/{member}` | owner/admin (self: name only) | `{name?, display_name?, role?}`; role `owner` hands over the hub (owner only; the old owner becomes admin) |
| `DELETE /v1/members/{member}` | owner/admin | Removes a person and revokes all their console tokens; what they own stays |
| `PUT /v1/agents/{name}/owner`, `PUT /v1/machines/{id}/owner` | owner/admin | `{owner}` (member name or id) |
| `POST /v1/machines/{id}/spawn?wait=` | machine owner or owner/admin | A spawn request (`api.SpawnRequest`) to the machine's machine node, answered with its `SpawnResult` (default wait 5m). With `join_link` (from v0.19.9) it starts no session: the machine joins another workspace of the same hub server with that workspace's join link (`<hub>/w/<name>/j/<token>`), through the connection it already has, and answers `{joined: <workspace URL>, machine_node}`. The hub sends only `join_link` on; the machine refuses a link to any other server (scheme, host and port must match its hub), and a myco older than v0.19.9 refuses the request (no directory). With `cli: "info"` (from v0.20.14) it starts nothing: the machine answers with `info` (`api.MachineInfo`: `dirs` sessions may start in as configured, `["~"]` when not limited; `new_dir`, where a folder given to no conversation goes; `max_access`; `ais`, each agent CLI's `state` missing/signed_out/ready/unknown with `version`, `install` and `login` commands; `sessions`, its Claude Code and Codex sessions of the last 180 days (at most 200) with `joined` naming the agent one already is in this workspace and `auto` marking one a program ran; `home`). The same JSON is in `reply` for hubs older than `info`, which re-encode the result; the hub sends nothing else with it and does not audit it. Older machines refuse it as an unknown CLI. A recent session joins as a copy with `fork_from: <session_id>` and `dir: <its workdir>` |
| `POST /v1/machines/{id}/spawn` with `cli: "concurrency"` | owner/admin to read; the computer's owner to change | (machines newer than v0.23.9) How many agent CLIs the machine's runner (`runner start --all`) runs at once: answers `concurrency` (`api.MachineConcurrency`: `limit`, `in_use`, `waiting`, `source` (`setting`, `flag` or `default`), `flag`, `ceiling`; also as JSON in `reply`). With `max_concurrent` (1 to 64) it first sets `runner.max_concurrent` in the machine's `machine.yaml`, applied at once. Changing it is the machine's owner's call only (their device, or the admin token when they own the hub; the hub's owner for a machine nobody owns), never an administrator for them or an agent; the hub audits `machine.concurrency`. The hub passes only these fields on; older hubs drop `max_concurrent` (nothing changes). Older machines refuse it as an unknown CLI; `info` also carries `concurrency` from this version. The console: `GET`/`POST /app/hubs/{id}/machines/{mid}/concurrency` `{max_concurrent}` asks or changes another computer (`too_old`: upgrade myco there; `hub_too_old`: the hub dropped the number), and `GET`/`POST /app/local/concurrency` reads or writes this computer's setting directly: `{setting, runner, default, ceiling, joined}`, where `runner` is what the runner running here reports in `~/.myco/runner-slots.json` (null when none does: not running, or older) |
| `POST /v1/machines/{id}/spawn` with `cli: "upgrade"` | owner/admin | (from v0.20.29) The machine brings its myco to `version` (`""` = the latest release) with `myco upgrade --no-restart` and answers `upgrade` (`api.MachineUpgrade`: `from`, `to` (`""` when it already runs it), `restarting`, `note`); a few seconds later its runner service restarts onto the new binary (tasks running there are cut short). Only the console's administrator sender may ask; the hub passes only `version` on and audits `machine.upgrade`. Older machines refuse it as an unknown CLI |
| `GET /v1/admin/backups` | the hub's owner (or the admin token; not admins, not a superagent) | (from v0.21.12) `api.Backups`: `{backups: [{name, size, created_at}], keep, daily, restore}`, newest first. The hub keeps backups of its own data (a snapshot of its database, and its files) in `<db dir>/backups`, makes one a day by itself and keeps `keep` (`myco hub serve --backups`); each workspace of a server keeps its own. `restore` says how to put one back. A hub without backups: `invalid_state` |
| `POST /v1/admin/backups` | the hub's owner | Make a backup now → 201 `api.Backup`. One at a time (`conflict` while one is being made). Audited as `hub.backup` |
| `GET /v1/admin/backups/{name}` | the hub's owner | The backup file (`application/gzip`, a `.tar.gz` laid out like the data directory, with `BACKUP.json`: what it holds and the sha256 of every entry). Restore it on the server with `myco hub restore`. Audited as `hub.backup_download` |
| `POST /v1/admin/upgrade` | owner/admin | (from v0.20.29) `{version?}` → 202 `api.HubUpgrade` (`from`, `to`, `via`): the hub brings its own myco to `version` (`""` = the latest; already the latest: `invalid_state`) and restarts. `via: "service"`: a server set up by `install-hub.sh` (or upgraded once with `sudo myco upgrade`) has a root path unit `myco-upgrade.path`; the hub leaves `upgrade.request` in its state directory and the unit runs `myco upgrade`. `via: "self"`: a hub under the user's own service manager (`myco hub up`: launchd, systemd --user) replaces its binary and stops; the service manager starts the new one. Otherwise `invalid_state` with the command to run on the server. Watch `GET /v1/health`'s `server_version` |
| `GET /v1/admin/perf` | owner/admin | (from v0.22.21) `?minutes=` (1-60, default 15) `&limit=` (1-500, default 20) → `api.PerfReport` (`minutes`, `requests`, `statements`, optional `database`): how long the hub's requests and database statements took over the last minutes, slowest first by p95. Each row is `api.PerfStat`: `name`, `count`, `p50_ms`, `p95_ms`, `max_ms`, `total_ms`, and for statements `avg_rows`, `max_rows`. A request is named by its route pattern (`GET /v1/threads/{id}`; a request no route matched: `GET (no route)`); long polls and streams are left out. A statement is named by the repository method that ran it, its verb and table, and a short hash of its text (`MessageRepo.ListThread: SELECT messages #be87`). Names and numbers only: no ids, parameters or message text. Percentiles are estimates (at most 25% above the true value, never above the maximum). The hub keeps an hour, in memory: a restart starts over. CLI: `myco hub perf`. The hub also logs a statement taking `--slow-statement` (default 50 ms) or longer as `slow statement` with its name, duration and rows |
| `PATCH /v1/machines/{id}` | owner/admin | `{name}` → the machine. 1–64 characters on one line; a name another machine of the hub has (letter case aside) is refused with `conflict` (from v0.20.16). Machines joining with a name already taken get `-2`, `-3`… |
| `DELETE /v1/machines/{id}?dry_run=` | machine owner or owner/admin | `{machine, removed:[agent names], dry_run}`; revokes the machine enrollment, removes its nodes (including the machine node), aliases and project memberships, then the machine record. Message history stays. Dry-run only previews |
| `GET /v1/audit?actor=&target=&before=&limit=` | owner/admin | `{entries: [{id, at, actor, actor_kind, action, target, detail}]}`, newest first; append-only; never message bodies |
| `POST /v1/enrollments` | admin (any member; exec links owner/admin) | `{name_prefix, harness?, allow_from?, capabilities?, max_uses?, ttl_seconds?, note?, superagent?}` → `{enrollment, token}` (token `myco_enroll_…`, returned once) | `superagent` (v0.21.5+, a person's link only): the agent that joins through it (and the machine enrollment made from it) becomes the caller's superagent.
| `GET /v1/enrollments` | admin | `{enrollments}` |
| `DELETE /v1/enrollments/{id}` | admin | Revoke |
| `POST /v1/enroll` | enrollment token | `{name?, display_name?, description?, capabilities?, binding}` → `{agent, token, created}`; same session → same agent (new token) |
| `POST /v1/join` | join link | `{machine}` → `{machine, token, created, name_prefix, harness, allow_from, labels}`: the machine's own enrollment token (prefix `<machine>-`, label `machine=<machine>`); the same machine re-joining gets a new token without using up the link |
| `GET /j/{token}` | — | A join link: shell script for curl/wget, Markdown guide for agents (`?format=md` or `Accept: text/markdown`), HTML page for browsers. Links are enrollments with `kind: "link"` (`POST /v1/enrollments` with `{"kind":"link"}` returns `join_url`, and `agent_guide: true` on hubs that serve the form below). The script and the guide end with the agent's last step (`myco join --display-name … --description …`). With `?shell=1` (only that exact value; from the next version) the script runs `myco machine join --allow-shell`, so the computer that runs it also lets agents open a terminal (shell node) there (`spawn.max_access: full`); the guide and the page show commands that keep the query (`curl -fsSL '<link>?shell=1' \| bash`, `irm '<link>?format=ps1&shell=1' \| iex`) and say so. For an exec link (`harness: "exec"`, from the next version) the scripts run `myco machine join --workdir "$PWD"` (PowerShell: `--workdir (Get-Location).Path`): run in a folder, that folder becomes the shell node, named `<machine>-<folder>-shell` (`<machine>-shell` for the home directory) |
| `GET /j/{token}/{project}` | — | The same, and the last step joins that project too (`myco join --project <project>`). A name that is not a project name is ignored. Older hubs answer 404: use it only when `agent_guide` is true |
| `PUT /v1/me/labels` | agent | `{add?, remove?}` — change your labels (a bare key in remove drops all its values) |
| `PUT /v1/agents/{name}/labels` | admin | Same, for any agent |
| `PUT /v1/me/skills` | agent | `{skills: [{name, summary?, examples?}]}` — replace your skills (the complete list; `[]` clears). Names are short lowercase tags (40 chars), unique; summary one line (200); up to 3 examples (200 each); at most 20 skills. `capabilities` becomes the list of names |
| `PUT /v1/agents/{name}/skills` | admin (members: their own nodes), or a project coordinator (see Projects) | Same, for any agent; audited |
| `GET /v1/agents/{name}/history?limit=` | the node's owner, owner/admin | `{entries: [...]}` like `/v1/audit`, newest first: changes to the node (`agent.*`). `agent.edit` is any change to its card, by the agent itself (`actor_kind: agent`) or by a person, applied at once; `detail` has one line per field that changed: `display_name: old → new`, `description: <new, clipped>`, `labels: +added −removed`, `skills: +added −removed ~edited` (from v0.18.23; nothing is recorded when nothing changed) |
| `PUT /v1/agents/{name}/visibility` | owner/admin, or the node's owner (never the agent itself) | `{visibility: "team"\|"private"}`; audited. `private`: only its owner (in the app) and the owner's own agents list it, look it up and send it tasks or notes; the hub's owner and admins still list it (marked `private`) but cannot send; replies from anyone still arrive; broadcasts leave it out. New nodes of a plain member are `private`, those of the owner and admins `team`; nodes from before v0.18.22 are `team`. A node without an owner cannot be private |
| `PUT /v1/me/profile` | agent | `{display_name?, description?, avatar?}` — omitted fields unchanged, `""` clears. `avatar` is the face: `""` for the default one drawn from the id, `look:<parts>` (`[a-z0-9-]`, up to 48; drawn by clients, e.g. `look:c3-p1-f2-s1`), or a picture as a base64 `data:` URL (PNG, JPEG, WebP or GIF by its bytes, not SVG; at most 192 KiB). A picture is stored and the agent's `avatar` becomes `pic:<hash>`; hubs before avatars refuse the field (`invalid_argument`, unknown field) |
| `GET /v1/agents/{name}/avatar` | admin/agent (who can see the agent) | The picture the agent uses as its face (`Content-Type` from its bytes, `nosniff`); 404 when its avatar is not a picture |
| `PUT /v1/agents/{name}/profile` | admin | Same, plus either `allow_from` (whole-list replacement, legacy `[]` = anyone; exec needs `accept_exec_risk`), or `allow_add?: string[]` / `allow_remove?: string[]` (atomic edits). Replacement and incremental fields are mutually exclusive; overlapping add/remove names are rejected. Response remains an Agent with optional `allow_from_change: {added, removed, unchanged}` arrays showing the actual stored difference. Permission changes and their audit (`actor`, `node`, `added`, `removed`) commit together; the token is unchanged. |
| `GET /v1/aliases` | admin/agent | `{aliases:[{alias, target, created_at, updated_at}]}` |
| `PUT /v1/aliases/{alias}?dry_run=` | admin/member/agent | `{target}` → `{alias, dry_run}`. An agent can point an alias only at itself, cannot take an existing alias, and can own at most 3; a member can act only on their own nodes; an admin can change where an alias points. 409 if `alias` is an agent name; target must be a real agent (no chains) |
| `DELETE /v1/aliases/{alias}?dry_run=` | admin/member/agent | → `{alias, removed, dry_run}`; an agent can delete only aliases that point at itself, a member only aliases of their own nodes |

Incremental sender edits are idempotent: adding existing names and removing absent names do nothing. They preserve concurrent additions. Adding to an unrestricted list does not narrow it; removing individual names from an unrestricted list is rejected. Removing the final named sender is rejected, rather than changing the legacy empty-list meaning to anyone. The App's existing nobody placeholder can represent a deny-all selection. New clients must not fall back to read/merge/replace on an older Hub's exact unknown `allow_add` / `allow_remove` field error: show an upgrade requirement instead. Other errors retain their original meaning. Existing clients may continue replacing lists. The CLI requires `--replace` before `--allow-from` removes exec-node entries (exit 5, `allow_replace_required`); `--dry-run` reports before/after/added/removed/unchanged without writing (exit 10). On exec nodes, a CLI replacement without removal consent uses an atomic append after its preflight, preserving senders added during that preflight; older Hubs require an upgrade for this safe path or an explicit `--replace`. Sender permission approvals always append; a legacy positive `allow_from` approval is normalized to `allow_add`, including on replay. Removal and allowing everyone require the person's explicit editor/CLI action.

`api.PerfReport` also has an optional `database` snapshot for the addressed
workspace's primary SQL pool (older Hubs omit it): `at`, `started_at`,
`max_open_connections`, `open_connections`, `in_use`, `idle`, `wait_count`,
and `wait_duration_ms`. Reading it does not acquire a SQL connection, so it
remains available when the pool is saturated. The wait counters accumulate
since `started_at`, **not** over `minutes`; compare their deltas only while
`started_at` is unchanged. Pending waits have increased `wait_count` but add
their elapsed duration only when they finish or are canceled. Pool waiting
is separate from statement execution timing. Request/statement histograms
retain their existing process-wide aggregation; the pool snapshot belongs
to the current workspace URL. Only the existing owner/admin endpoint exposes
it; no database path or connection string is returned. `myco hub perf`
shows this snapshot when available and still works against older Hubs.

| `POST /v1/broadcasts` | agent | `{selector, body, type?, idempotency_key?, include_self?, parent_id?}` → `{broadcast_id, messages, skipped, duplicate}`; one message/thread per recipient |
| `GET /v1/broadcasts/{id}?consume=` | sender | `{broadcast_id, total, answered, deliveries:[{message, reply?}]}`; `consume=true` marks returned replies done |
| `POST /v1/files?name=` | agent | Raw body upload (≤ `--max-file-size`) → file `{id, name, size, sha256, content_type, owner, expires_at}` |
| `GET /v1/files/{id}` | uploader / recipient | Download; headers `X-Myco-SHA256`, `X-Myco-File-Name` |
| `GET /v1/files/{id}/meta` | uploader / recipient | File metadata |
| `GET /v1/overview?recent=` | admin/agent | Dashboard snapshot: per-agent `state` (working/pending/idle/offline), `queued` (queued + delivered, pulled but not acked), `in_progress` (acked tasks that show life), `stale` and `stale_tasks` (acked tasks that do not: the agent is offline, or none was touched for 5 minutes under a runner or 30 minutes in a session; still open, from v0.20.28), `held` (separate), `receiving` (`session`/`runner`) and `session_in_use`; `working` means at least one acked task that shows life, `pending` means queued/delivered/held without it; a task shows life when its recipient reads it, reports progress, asks or uploads its trace (kept with the task, so a restart keeps it); message contents only for the viewer's own messages (all for admin) |
| `GET /v1/agents?capability=&online=true&selector=` | admin/agent | `{agents:[...]}`; each agent carries `skills` (`[{name, summary?, examples?}]`, from v0.18.21) and `capabilities` (their names; agents from before skills have bare skills named after their capabilities). `capability` matches a skill name. Private nodes (see `visibility`) are listed only to those who may see them, and `GET /v1/agents/{name}` answers 404 for the others |
| `GET /v1/agents/{name}` | admin/agent | One agent (`name` may be an alias); agents carry `display_name`, `avatar` (when one was chosen) and `aliases`. Those who may manage it also get `resume` (`{created_at, last_used_at?, last_used_env?}`, from v0.19.5) once it has a resume line |
| `GET /v1/me` | agent | The calling agent, including Hub-resolved `owner` (member name) and `owner_id` when known. Exec runners may derive that owner's exact console identity from `owner`; it is not a client-supplied authorization field |
| `PUT /v1/me/binding` | agent | `{harness?, session_id?, environment?, workdir?}` — update your session binding (empty fields unchanged) |
| `GET /v1/inbox/wait?wait_seconds=` | agent | Long-poll until messages are pending (not leased) → `{pending, timed_out}` |
| `POST /v1/ingress/{kind}` | signature | External callbacks turned into replies (`github`: `X-Hub-Signature-256`) |
| `POST /v1/messages` | agent | Send `{to, body, type?: task\|note, thread_id?, idempotency_key?, attachments?: [file_id], project?, after?, parent_id?}` → `{message, duplicate, via_alias?}`; `to` may be an alias (the message stores the real name); attaching grants the recipient access. `project` puts a new thread in a project (you must be a member; it must not be archived); a message in an existing thread always belongs to the thread's project, and so do replies, progress, steers and events. Messages carry `project` (from v0.18.26). From v0.19.2, `after` must name a task by this sender to the same recipient. While that task is open, this message stays queued but cannot be claimed, does not wake the recipient and is excluded from overview `queued`; closing the task releases it. A closed `after` delivers immediately. The sender may stop the queued message |
| `POST /v1/messages/{id}/trace` | agent (the task's recipient) | `{events: [{seq, kind, data}]}`: steps of the run handling task `id`, uploaded by the recipient's runner while it runs (from v0.18.32). `kind`: `text`, `thinking`, `tool`, `tool_result`, `usage`, `error`, `output` (a shell node's command output as it runs, `stream` is `stdout` or `stderr`), or `delta` (text or thinking while it is being written, from v0.19.8: `stream` is `text` or `thinking`, `id` names the block, and pieces with the same `id` join in order; the finished block follows as a `text` or `thinking` step and replaces them), or `turn_end` (from `myco trace follow`, v0.19.8: the followed session ended its turn and waits, e.g. for agents it handed work to; steps after it mean it is at work again); `data` is the step (`text`, `stream`, `tool`, `id`, `input`, `output`, `failed`, `input_tokens`, `output_tokens`, `cost_usd`, `duration_ms`). `data.observed_at_ms` optionally gives the runner source observation time (Unix ms); outer `at` remains Hub ingestion time. Older runners omit it. Cross-host subtraction requires measured clock-skew bounds. A `seq` already stored is skipped; up to 500 steps per upload, 16 KB per step, 2 MB per task. Not for end-to-end encrypted tasks. Kept 7 days |
| `GET /v1/messages/{id}/trace?after=` | agent (the task's sender or recipient) | `{message, status, events}`: the steps after `after` |
| `GET /v1/messages/{id}` | agent | A message you sent or received. For `acked`/`held` tasks, `active_at` is the recipient's latest observed activity (recipient GET, progress, trace, steer receipt); Hub restart without in-memory activity falls back to `updated_at`. Sender reads do not advance it |
| `GET /v1/messages/{id}/tree?limit=200` | participant, project coordinator or admin | Metadata-only nested task tree → `{root, as_of, truncated}`. Nodes contain `id, from, agent, status, age_seconds, thread_id, project?, parent_id?, relation, reference?, waiting_on, children`. Limit 1–500, depth ≤32; private descendants are filtered. [Task coordination](message-coordination.md) describes parent, hold and inferred thread links. |
| `POST /v1/messages/{id}/reassign?dry_run=1` | sender, project coordinator or admin | `{to, note?}` → `{id, from, to, previous_to, thread_id, project?, status, dry_run?}`. Atomically moves an unclaimed queued task, its attachment grants and an audit record. Previously delivered tasks return `invalid_state` (stop + send); E2E tasks return `unsupported` (sender must re-encrypt via stop + send). Note ≤2,000 characters; agent-origin tasks retain the original sender’s recipient permissions; project tasks require a recipient in that unarchived project. [Details](message-coordination.md). |
| `POST /v1/inbox/pull` | agent | `{limit?, wait_seconds?, types?, via?}` → `{messages}`; leases messages (long-poll ≤ 60s); each message from a node carries `from_display`, that node's display name, when it has one (from v0.18.36). With `via: "runner"` it returns nothing while a session of the agent keeps `inbox wait` running (and for 30 minutes after, so a session that is still open between two waits is not resumed behind it) |
| `POST /v1/messages/{id}/ack` | recipient | Start work on a pulled task (stops redelivery and makes it `working` in overview); an inbox-wait session should ack before doing the work |
| `POST /v1/messages/{id}/hold` | recipient | Legacy hold. With `?manual=1`, mark an intentional hold outside a runner. |
| `POST /v1/messages/{id}/hold-until` | recipient | `{until}`: hold the task until the delegated task ends. The caller must have sent that task to another agent. The held message reports `held_for: "reply"`. Supports `dry_run`. |
| `POST /v1/messages/{id}/done` | recipient | Close without reply; also works on a queued message (drop it without pulling) |
| `POST /v1/messages/{id}/reply` | recipient | `{body, attachments?, result?}` → `{message: <reply>}`; closes the original. `result` (v0.21+): a JSON object, the structured result a workflow step reads (at most 64 KiB; not end-to-end encrypted, the hub reads it). Older hubs refuse the field: the CLI then puts it at the end of the body as a `` ```json `` block, which workflows read the same way |
| `POST /v1/messages/{id}/release` | recipient | `{delay_seconds, reason?}` (≤ 3600) — hand back for redelivery later; not counted as a failed attempt. `reason: "in_use"` marks the agent's session as in use, shown in the console |
| `POST /v1/messages/{id}/fail` | recipient | `{reason}`; for tasks the sender gets an `event` |
| `GET /v1/messages/{id}/reply?wait_seconds=&progress_since=` | sender | Long-poll for the reply → `{reply, timed_out}`; a returned reply is marked done. With `progress_since` (RFC 3339, or `0`) it also returns as soon as there are newer progress updates → `{reply, progress, timed_out}` |
| `POST /v1/messages/{id}/progress` | recipient | `{body, attachments?, ask?}` → interim update on an open task (type `progress`, stored closed: never pulled, never wakes the sender). With `ask: true` it is a question that holds the task until the sender answers (`myco message answer`); when the sender is an agent (not a person in the app) the question is stored `queued` instead, so it lands in that agent's inbox and wakes its session or runner (from v0.22.1). `attachments` are ids of files the recipient uploaded (a screenshot, a draft); the task's sender may read them. Hubs before this keep the text and drop the files (the answer has no `attachments`; the CLI says so) |
| `POST /v1/messages/{id}/suggest` | recipient | `{text}` → the task (`message`) with `suggest`: what its sender may want to say next, one short line (the CLI cuts it at 80 characters; `myco message suggest`, `myco message reply --suggest`); `""` clears it. The app offers it in the empty input and Tab fills it in. Sealed for the sender like a body when end-to-end encryption is on. Older hubs answer 404 (the CLI then replies without it) |
| `POST /v1/messages/{id}/progress` with `ask: true` | recipient | A question for the task's sender (`myco message ask`): the update has `ask: true`, its body is the question then the choices as a numbered list (`1. label — detail`); the task is held and its `asking` names the question. Whatever the sender says next (a steer) answers it: the steer has `answers` (the question's id) and the task's `asking` is cleared; the recipient's runner resumes the held task's session with it. Agents count such tasks as `asking` (also in `held`). Older hubs refuse the field (the CLI then sends plain progress and holds the task) |
| `POST /v1/messages/{id}/steer` | sender | `{body, attachments?: [file_id]}` → correction for an open task (type `steer`, delivered to the recipient; runners interrupt and resume the task with it) |
| `GET /v1/messages/{id}/steers?consume=` | task recipient | `{steers: [message...]}` lists unhandled corrections (from v0.19.2). `consume=true` atomically marks those corrections done; `consume=false` only reads. CLI ack/progress consume and show them, while CLI reply checks and refuses unread corrections unless `--anyway`. Older hubs return 404 and CLI skips the check |
| `POST /v1/messages/{id}/stop?dry_run=` | sender | Stop a task you sent, including queued or running tasks → `{message,dry_run}`; task becomes failed with `error: "stopped by the sender"`. Another agent gets 403 |
| `PUT /v1/me/status` | agent | `{text}` → public one-line status (`""` clears); agents carry `status_text`, `status_at`, `state` (working/pending/idle/offline), `queued`, `in_progress`, `held` |
| `GET /v1/threads/{id}` | participant / admin | `{messages}` in order. For a console, `updated_since` (unix ms, from v0.18.35): only the messages that changed after it, and `since` echoes it |

Runner pulls with `via: "runner"` and query `wake_held=1` requeue a held task
when its delegated task completes or its sender answers its question. The
pull response includes `hold_wake` (the triggering message ID, or `recovered`
for a legacy hold past 30 minutes). Older hubs ignore the query parameter.

## Projects

A project groups the nodes that work on one thing, with a shared goal, a brief, documents and links (from v0.18.25), and a memory its members add to (from v0.21.10). Agents read them with `myco project list/show/pull` and `myco project memory show`, and add to the memory with `myco project memory add`.

| Endpoint | Who | Notes |
|---|---|---|
| `GET /v1/projects?mine=true` | anyone | `{projects: [project]}`: the projects you can see (`mine`: only those you or your nodes are in). A project: `{name, title, goal, brief, links: [{title, url}], visibility, owner, owner_id, archived, created_at, updated_at, members: [{name, role, note, added_by, added_at, agent?}], docs: [{name, size, updated_by, updated_at}], can_edit, can_manage, machines, groups}`; `agent` is the node's card when you can see it; `machines` (v0.21.2+) are the computers (ids) its coordinators may start agents on; `groups` (v0.21.34+) are the groups its members are sorted into, in display order, always present (`[]` when none; absent from older hubs, which have no groups), and a member's `group` is its group in this project (absent: ungrouped) |
| `POST /v1/projects` | people (not agents) | `{name, title?, goal?, visibility?}`; `name` is a short lowercase address; you become its owner |
| `GET /v1/projects/{name}?docs=body` | who can see it | One project; `docs=body` includes the documents' text. From v0.21.10 it also has `memory`: the project's memory document with its body (`{name: "memory", body: ""}` while nobody has added to it; absent from older hubs) |
| `GET /v1/projects/{name}/storage` | who can see the project | Usage without file metadata: `files_used`, `files_max`, `bytes_used`, `bytes_max`, `docs_used`, `docs_max` (100), `files_max_override`, `bytes_max_override`, `can_manage_storage`. The project response includes the same fields. `files_used` counts non-context/Blob files; bytes count all logical content, including UTF-8 inline text. Zero overrides inherit Hub defaults; absent effective `files_max` identifies an older Hub |
| `PUT /v1/projects/{name}/storage` | workspace owner/admin (people or admin token; not project owners, agents, or delegated superagents) | `{files_max, bytes_max}` sets project overrides, 0 restores inheritance. Protection caps: 10,000 files and 21,474,836,480 bytes (20 GiB). Returns storage usage; audited as `project.storage`. Existing files remain readable/deletable if a new limit is below usage; uploads must fit both count and byte limits |
| `PATCH /v1/projects/{name}` | editors: its owner, the hub's owner/admins, its coordinators | `{title?, goal?, brief?, links?, groups?, member_groups?}`; the brief is up to 16,000 characters (8 KB before v0.21.10). `visibility` (`members`\|`team`) and `archived`: its owner and hub owner/admins only. From v0.21.34 (older hubs reject them): `groups` replaces the list of groups (up to 30 unique names of 1-40 characters, trimmed; members of a group left out become ungrouped, which is how a group is deleted); `member_groups` (`{agent: group}`, `""` = ungrouped) moves members of the project into groups of the resulting list; anything else is `invalid_argument`. Both are applied together in one transaction (rename: the new list plus moving the old group's members). Audited as `project.groups` |
| `DELETE /v1/projects/{name}` | its owner, hub owner/admins | Deletes members, documents and links; nodes stay |
| `PUT /v1/projects/{name}/members/{agent}` | see below | `{role?, note?, group?}`. Adding: a person adds nodes they own (admins any), an agent adds itself, a coordinator adds nodes of its own owner. `role` (`member`\|`coordinator`): its owner and hub owner/admins only. `note` (what it does here): editors or the node's owner. `group` (v0.21.34+): one of the project's `groups`, `""` = ungrouped (else `invalid_argument`; create a group with `PATCH` first): editors |
| `GET /v1/projects/{name}/machines` | anyone who sees it | `{project, machines: [{id, name, os, online, can_spawn, spawn_node?, allowed}]}`: the workspace's computers, and which the project's coordinators may start agents on (`allowed`; v0.21.2+) |
| `PUT /v1/projects/{name}/machines/{machine}` | who manages it | Let its coordinators start agents on a computer (`{machine}`: id, name or machine node). A spawn request from a coordinator to that computer's machine node then passes without being on the node's `allow_from` (the message carries `granted_by: "project:<name>"`); the computer's own folder and access rules still apply. Not related to the terminals (shell nodes) in the project. `DELETE` takes it back. Both answer the project, with `machines` (ids) |
| `POST /v1/projects/{name}/machines/{machine}/request` | a coordinator (agent) | `{reason?}` → `202` approval (`pending`): a card for the project's owner, shown in the coordinator's chat; approving carries out the `PUT` above as the owner, and the agent gets a note either way. Asking again while it waits gives the same approval; an allowed computer answers `200 {allowed: true}` |
| `DELETE /v1/projects/{name}/members/{agent}` | editors, the node's owner | A coordinator cannot remove another coordinator |
| `GET/PUT/DELETE /v1/projects/{name}/docs/{doc}` | read: who can see it; write: editors | `{body}` (Markdown, up to 64 KB) — documents written as text |
| `POST /v1/projects/{name}/memory` | member nodes, people who see it, editors (archived: who manages it) | `{text}` (one entry, up to 1000 characters; whitespace is folded into one line) → the memory document with its body (from v0.21.10). The entry is appended to the document `memory` as `- text (who, date)`; editors change or tidy it with `PUT …/docs/memory`. The runner gives member agents the memory with the project (the newest 8,000 characters) and gives the project again when it or its memory changed. Audited as `project.memory` |
| `PUT /v1/projects/{name}/files/{file}` | anyone involved (member agents, people whose nodes are in it, editors); replacing someone else's file: editors | Upload a file (from v0.18.29): the body is its content, `Content-Type` its type; up to 20 MiB per file. Inline context text documents are limited to 100; other files default to 1,000 per project, with 2 GiB total logical bytes (including documents). Text up to 64 KB (Markdown, txt, json, …) comes with the project like a document (`body`, read by agents); anything else has `binary: true`. Names are plain file names (no folders) |
| `GET /v1/projects/{name}/files/{file}` | who can see it | The file's content (any file, documents too) |
| `DELETE /v1/projects/{name}/files/{file}` | its uploader, or editors | Remove it |

Conversations: a thread belongs to the project of its first message (`project` on `POST /v1/messages` and `POST /v1/console/messages`; from the app the person must see the project). `GET /v1/console/recent?project=` lists a project's conversations; their task states are the project's progress. Deleting a project keeps its conversations, without the project.

Visibility: `members` (default) — its owner, the owners of its member nodes, the member nodes, and the hub's owner and admins; `team` — everyone on the hub. All changes are audited (`project.*`).

A **coordinator** of an active project may also change, with its own token, the card of that project's other member nodes: `PUT /v1/agents/{other}/profile` (display name, description and avatar; not `allow_from`), `/labels` and `/skills`; not their visibility, owner or permissions. Its changes are recorded as `agent.edit` under its own name. This replaces the per-node `coordinator` flag of v0.18.24.

## Pages

Updated clients support an application-owned fragment on public page links; see [App links](app-links.md). Fragments select an app view and do not change HTTP page identity, file paths or permissions. The page bridge may expose optional `myco.ready.page.url` (the canonical public Page URL) so apps can copy these links without exposing their private rendering URL. This is additive client context; older bridges omit it.

From v0.19.24: web pages agents (or people) publish for the App to open in its page panel. A page is a folder of static files kept as a zip; each publish is a version (the 10 newest are kept). Agent tokens and console tokens use the same routes. Who sees a page: its owner (the publishing agent's owner, or the person who published it), that owner's agents, the hub's owner and admins, and, for a page published to a project, whoever sees the project. Its owner, their agents and the hub's managers publish versions and delete it. Hubs with end-to-end encryption on refuse pages (`invalid_state`). Older hubs answer 404.

| Endpoint | Who | What |
| --- | --- | --- |
| `POST /v1/pages?name=&project=&entry=` | agent or person; with `project`, anyone who may add files to it | The body is a zip of the page's files (`Content-Type: application/zip`, up to 50 MB; at most 2000 files, 20 MB each; relative paths, no `..` or hidden parts) → `201` page `{id, name, project?, publisher, version, entry, url, versions: [{n, entry, files, size, sha256, by, at}], created_at, updated_at, can_manage}`. `entry` is the file it opens on: given, else `index.html`, else its only html file. `url` is `<hub>/pages/<id>`: the App opens such links in its page panel; in a browser it says to open it in the App. A workspace keeps up to 200 pages |
| `POST /v1/pages/{id}/versions?entry=&name=` | its managers | A new version (and, with `name`, a new name) → `200` the page |
| `GET /v1/pages?project=` | anyone | `{pages: [...]}` the caller may see, newest first |
| `GET /v1/pages/{id}` | who sees it | The page and its versions. For a page made from an official app, a hub whose last look for newer apps is over an hour old looks first (waiting up to 4 seconds), so `template.latest` shows an update published since |
| `GET /v1/pages/{id}/v/{n}/{path}` | who sees it | A file of version `n` (`latest` for the newest; an empty path is its entry file), with its type from its extension, `X-Content-Type-Options: nosniff`, `Content-Security-Policy: sandbox …` (opened straight from the hub, it runs in a sandbox of its own), `ETag`, `X-Myco-Page-Version`. A numbered version is cached for good; `latest` is revalidated |
| `PATCH /v1/pages/{id}` | its managers | `{editors: "everyone" \| "managers"}` → the page (from v0.22.2): who may change its records and shared state, everyone who sees it (the default) or only those who manage it. A page answers `editors` (`managers`, or absent) and, to whom it is read-only, `read_only: true`; their writes answer `403 permission_denied` |
| `DELETE /v1/pages/{id}` | its managers | `204`; all its versions go |
| `POST /v1/pages/live?name=&project=&port=&page=` | an agent | A **live page**: a server on the agent's computer (`port` there, e.g. a dev server), forwarded while its tunnel is open. Without `page` a new one (`name` required); with `page`, one of its own again (same address; `port` may change). The page has `live: true`, `port`, `online` (forwarded now) and no versions or files. Refused on hubs with end-to-end encryption |
| `GET /v1/pages/{id}/tunnel` | the agent that publishes it | A WebSocket (binary) carrying [yamux](https://github.com/hashicorp/yamux): the hub opens a stream for each connection to the page's server, and the agent joins it to `localhost:<port>`. A newer tunnel replaces an older one. (`myco page serve` does this.) A proxy in front of the hub must pass WebSockets |
| `/v1/pages/{id}/live/{path}` (any method) | who sees it | The page's server at `/{path}`, forwarded: requests and WebSocket upgrades (hot reload). Its `Host` is `localhost:<port>`; the caller's `Authorization` and cookies are not passed on, and its `Set-Cookie` is dropped. `503 offline` (with how to take it up again) when no tunnel is open; `502 unavailable` when nothing answers on the port |
| `GET /v1/pages/{id}/state` | who sees it | The page's shared state (the bridge's `myco.update`): `{state, version, updated_by?, updated_at?}`; `state` is `null` and `version` `0` before anyone wrote. Kept across the page's versions |
| `PUT /v1/pages/{id}/state` | who sees it | `{state, if_version}`: any JSON up to 256 KB, based on the version read → the new state (`version` + 1). When someone wrote since: `409 conflict` with `latest` (the current state) |

## Pins

From v0.20.3: what a conversation keeps at hand ("Pinned" in the App), shown as tabs above it in the App: a web address (`link`), a file (`file`), or one of its messages (`message`). A conversation is two parties (a person's console sender `@console:<name>` and an agent, or two agents); both see the same list, in one order, and either changes it. Kept in the hub's settings per conversation (no schema change), 20 at most; pinning the same thing again only changes its title. Agents act in their own conversations only (`a` is themselves); a console token in its own (`a` defaults to its sender) or, naming `a`, in one it may read. Older hubs answer 404 (the App then shows no tabs).

| Endpoint | Who | What |
| --- | --- | --- |
| `GET /v1/pins?with=&a=` | agent or person | `{a, with, pins: [{id, kind, title, target?, file?, added_by, at}], max}` in their order. `target` is the address (`link`) or the message id (`message`); `file` is an attachment `{file_id, name, size, sha256, content_type}` |
| `POST /v1/pins` | agent or person | `{a?, with, kind, title?, target?, file?}` → `201` the pin. A link is `http`/`https` (2048 characters at most); a title is cut to 80 characters and defaults to the address, the file's name or the message id. `409 conflict` past 20 |
| `PATCH /v1/pins/{id}` | agent or person | `{a?, with, title}` → the pin |
| `PUT /v1/pins/order` | agent or person | `{a?, with, ids}` → the list, those named first in that order |
| `DELETE /v1/pins/{id}?with=&a=` | agent or person | `204` |

## Notices

What each person should hear about (hub v0.21.21+): an agent asking them, a workflow waiting on their decision, their superagent asking for their OK, a task of theirs that failed, an answer, an update. The hub works them out from its own events and decides once, from the person's preferences, whether each is announced (a banner, a sound, a push), so every device agrees. They are a person's: a console credential reads the notices of the member behind it (the hub's admin token reads the owner's); agents and superagents cannot. Kept 30 days.

| Endpoint | Who | Description |
|---|---|---|
| `GET /v1/console/notices?after=&limit=&wait_seconds=` | person | `{notices: [Notice], last, badge}`, oldest first, those after `after` (a `seq`). With `wait_seconds` (at most 55) and nothing new, waits for the next one or for a change (read, settled). Read after `last` next time; `badge` counts open `needs_you` notices |
| `POST /v1/console/notices/read` | person | `{target?, through?}` → `{changed}`: marks as read what leads to `target` (`"agent:codex"`; `"workflow:w1"` covers its runs; omitted: all) up to `through` (a `seq`; omitted: all). Reading settles notices that are not actionable (`asking`, `decision`, `approval` are settled only by answering, deciding, approving) |
| `GET /v1/console/notice-prefs` | person | the person's preferences (below; `{}` when never set) |
| `PUT /v1/console/notice-prefs` | person | replaces them; returns them. `400` for quiet hours outside 0–1439, an unknown zone or tier, or more than 500 mutes |

A **Notice** is `{seq, id, kind, tier, key, ref?, target?, project?, actor?, subject?, message_id?, announce, quiet?, created_at, read_at?, resolved_at?, message?}`:

- `kind` and its `tier`: `needs_you` (`asking`, `decision`, `approval`, `failed`, `run_failed`, `stuck`), `results` (`answered`, `said`, `mention`), `activity` (`progress`).
- `target` is where it leads: `agent:<name>`, `project:<name>`, `workflow:<id>?run=<run>`, `superagent`. `actor` is who it is from.
- `message` is the message it is about, when there is one (sealed as stored on an end-to-end encrypted hub; the Go client opens it). `subject` is a few words when the hub knows them (a run's name, what an approval would do); a message's text is never copied into a notice.
- `announce` is whether devices announce it; when not, `quiet` says why: `tier`, `muted`, `paused`, `quiet_hours`, `off`. A device adds only what it alone knows (the person is looking at that conversation).
- `resolved_at`: it no longer waits for the person (answered, decided, read).

**Preferences** are `{off?, tiers?, quiet?, paused_until?, mutes?, updated_at?}`. `tiers` overrides the defaults (`needs_you` on; `results` and `activity` off). `quiet` is `{from, to, zone}`: minutes from midnight in an IANA zone, a `from` later than `to` spanning the night; nothing is announced then, whatever the tier. `mutes` maps a scope (a notice's target without its query, `project:<name>`, or `agent:<actor>`) to when the mute ends (the zero time: never).

### Desktop page origins and local recovery

The desktop console opens each static page on a stable loopback port in the local App profile, scoped by workspace, member and page. The console's `POST /app/hubs/{id}/pages/{page}/open` returns a current-process opening ticket in `lease`, its `url`, and `isolated: true` for the interactive panel. The panel releases the ticket with `DELETE /app/hubs/{id}/pages/{page}/open/{lease}`. Close the other app and retry if the saved address is occupied. The App releases idle listeners beyond the most recent 32, keeping the port reservations and allowing up to 128 static listeners. Hidden exports open a separate origin. The bridge exposes desktop interactive isolation in `myco.ready.isolation`. Apps that need it declare `requires.isolation: true` in `myco.json` and check the bridge before opening sensitive UI on older clients. When publishing to older Hubs, set `requires.hub` to the first Hub that accepts the new manifest field.

From App v0.22.11 the desktop App also opens a page in a window of its own: `POST /app/hubs/{id}/pages/{page}/window` with `{name}` (the window's title) opens or brings forward that page's window, which shows the page alone with the same bridge, consent and origin as the panel. `/app/info` says `page_windows: true` where the App can; elsewhere the call answers `501 unsupported`.

IndexedDB, Web Storage and CacheStorage are keyed by origin. Cookies are shared across ports on the loopback hostname. Page hosts and their bridge never use cookies for state or authority. Use IndexedDB for app data on this device or page records for data that belongs on the Hub. Keep secrets out of shared records. Hostname-level cookie separation is a future option after both desktop WebViews support the same loopback name resolution.

The App keeps the old shared origin's browser storage. The desktop panel checks the retired origin asynchronously with a host-owned frame. Only supported templates with pending items show **Recover older local files** in the page title menu. The viewer uses the same retired origin and opens on the current app. Export an original from Video Editor, Photo, Motion or Reader and import it in its app. After importing a saved copy, mark the item recovered. The App remembers confirmations in its own profile and hides the entry when that app has no pending items. Original records stay in place and downloads may be retried before confirmation. Legacy origin keys lacked authenticated ownership, so records are not copied into new pages by inference. Reader settings and earlier bridge preferences can be exported as JSON for reference. New bridge preferences for isolated pages use an account-specific key.

## Page records and templates

From v0.20.5. A page's data can be **records**: JSON objects in named collections (`[a-z][a-z0-9_-]*`), each written on its own with a version, at most 64 KB a record and 20000 a page. Every write goes into the page's **change feed**. Whoever may see the page reads its records and, unless its `editors` are `managers` or the collection's `write` is `managers` (from v0.22.2), writes them (agent and console tokens alike). A page made from a template checks each record against the template's fields (no unknown fields; each of its type; `null` clears one; a field its template dropped in an upgrade may be written back unchanged where the record already has it); a `single` collection holds one record, `main`. Older hubs answer 404.

| Endpoint | What |
| --- | --- |
| `GET /v1/pages/{id}/data?collection=` | `{records: [{collection, id, data, version, updated_by, updated_at}], seq}` in their order; `seq` is the latest change, to watch from |
| `GET /v1/pages/{id}/data/{collection}/{record}` | One record |
| `POST /v1/pages/{id}/data/{collection}` | `{data}` → `201` a new record (its id made up) |
| `PUT /v1/pages/{id}/data/{collection}/{record}` | `{data, if_version?}` → the record (`201` when new). `if_version` is the version read (0: it must be new); left out, it writes whatever it is. A conflict answers `409` with the record now in `latest` |
| `DELETE /v1/pages/{id}/data/{collection}/{record}?if_version=` | `204` |
| `GET /v1/pages/{id}/changes?after=&wait=` | `{changes: [{seq, collection, record, op: put\|delete, data?, version, by, at}], seq, reset?}` after `after`, oldest first; with `wait` (seconds, up to 60) it waits for one. The newest 5000 are kept: `reset` means list the records again (a page hears it as `myco.on("reset")`, bridge feature `data-reset`) |

### Record queries

The records GET endpoint accepts optional `fields`, repeatable `where`, `sort`,
`limit`, `after` and `count` parameters. Existing requests retain their unlimited
list behavior. Successful query-capable responses include `query_version: 1`;
clients may send `query_version=1` to negotiate. Older Hubs silently ignore these
GET parameters and omit the response marker: the current CLI and desktop detect
that and apply the same query locally to the returned records. The CLI warns to
upgrade the Hub. Authentication and validation failures never trigger fallback.

| Parameter | Behavior |
| --- | --- |
| `fields=n,title,status` | Projects only `data`. The record envelope, including `id` and `version`, remains. |
| `where=status:progress` | Equality; repeat conditions to AND them. |
| `where=status:ne:done` | Not equal; `!=` is an alias. Missing fields do not match. |
| `where=status:in:todo,progress` | Membership; a JSON scalar array (`n:in:[1,2,3]`) also works. 1–100 values. |
| `where=assignee:exists` | Field is present, including JSON null. `exists:false` finds missing fields. |
| `where=n:gte:10` | Numeric range. `gt`, `lt`, `gte`, `lte` and `>`, `<`, `>=`, `<=` work. |
| `where=created:>=:2026-11-25T00:00:00Z` | ISO date / RFC3339 instant range, millisecond precision; invalid dates do not match. |
| `sort=-n,created` | Ascending by default, `-` descending; missing fields always last. Stable ties use collection, record position, ID. |
| `limit=100` | 1–1000 records; unset or 0 is unlimited. A nonempty `next` response cursor means more records. |
| `after=<next>` | Repeat the same page, collection, fields, filters and sort. Page size may change. |
| `count=true` | Only `{count, seq, query_version:1}`; total matching records, ignores projection/sort/limit. Cannot combine with after. |

Field names address flat `data` keys: `[A-Za-z_][A-Za-z0-9_]{0,63}`. Nested paths
and SQL syntax are rejected. At most 64 projected fields, 8 sort fields and 32
conditions (4096 bytes each) are allowed. Values are bound SQL parameters, never
interpolated into SQL. Scalars use JSON types, or unquoted strings; quote strings
that look like JSON (`status:eq:"null"`). Equality is type-strict (numbers compare
numerically; booleans are distinct). Missing fields fail all comparisons except
`exists:false`. Ranges accept only numbers or dates; equivalent timezone-offset
instants compare chronologically in ranges, whereas equality remains literal.
Sorting uses SQLite scalar order: null, numbers/booleans, then text; array/object
values use compact JSON text. Use consistent field types and normalized UTC date
strings when sorting. Useful collection indexes cover position, issue number,
status and assignee/status; other fields remain queryable without fixed schemas.

Cursors are opaque, scoped to the query and the page's change sequence. Any page
record mutation invalidates them with `409 conflict`; restart without `after`.
Malformed or cross-query cursors return `400 invalid_argument`. A query reads its
sequence and records in one SQLite snapshot. Projection is not a complete record:
get the full record before a replacing `put`, and use its version for CAS writes.

CLI examples (text mode renders a table; JSON remains structured):

```sh
myco page data list --page pg_id --collection issues --fields n --sort -n --limit 1
myco page data list --page pg_id --collection issues --where assignee:alex --where status:progress
myco page data list --page pg_id --collection issues --where status:in:todo,progress --count
```

The desktop page bridge exposes the same options as
`myco.data.list(collection, {fields, where, sort, limit, after, count})`. Fields and
sort accept a comma-separated string or string array; where accepts a string or
string array. Check feature **`data-query`** first: older native hosts, including
mobile versions without this feature, may ignore extra arguments. Query reads do
not advance the editor's shared watch cursor; an ordinary unfiltered list keeps
its existing watch initialization behavior. SDK examples and types live in the
[apps query guide](https://github.com/myco-ai/apps/blob/main/docs/page-data-queries.md).

**Templates**: a folder with `myco.json` (its manifest: `id` as `author/name`, `name`, `version` as `x.y.z`, `kind`, `category` (from v0.20.6: `office`, `dev`, `tools`, `life`, `creative`, `game` or `other`; the App's shelves), `icon` (from v0.20.7: an `.svg` in the template, at most 16 KB; the App shows its shape), `requires` (from v0.20.7: `{"hub": "0.20.7"}`; an older hub refuses it, saying which version it needs), `description`, `entry`, `data` (collections → `{fields: {name: type}, single?, write?}` (`write: "managers"`, from v0.22.2: only those who manage the page change that collection); types `text, markdown, number, bool, date, datetime, person, select, multi, url, file, json`), `permissions` (`send`, `download`), `os` (from v0.22.1: the groups of `myco.os` capabilities it uses: `files`, `files.write`, `files.delete`, `exec`, `screen`, `microphone`, `speech`, `clipboard`, `notify`, `system`, `network`, `ssh`, `database`, `proxy`, `browser`, `codehost`; see docs/app-os.md), `net` (from v0.22.1: the hosts `net.fetch` may reach, as `host`, `host:port` or `*.domain`; needs `network` in `os`), `export`, `agent` (default `agent.md`), `sample` (default `sample.json`)), its page files, the agent's guide and sample data (`{collection: [records…]}`). Everyone in the workspace sees the library and uses its templates; whoever published one (a person, or agents of the same owner) and the workspace's managers publish its versions and delete it. A page made from a template keeps its own copy of the files and the manifest.

| Endpoint | What |
| --- | --- |
| `POST /v1/templates` | The body is a zip of the folder → the template (`201` new, `200` a new version; the version must be higher). 200 templates, 10 versions each |
| `GET /v1/templates` | `{templates: [...]}`: `{id, slug, name, description, kind, category, icon (SVG markup: show it as a mask or an image, never as HTML), version, publisher, data, permissions, export, versions, can_manage, created_at, updated_at}` |
| `GET /v1/templates/{id}` | One, by `tp_…` or its slug (URL-escaped), with `guide` (agent.md) and, from v0.21.5, `shots`: its screenshots, `[{src, dark?}]` (paths of pictures in its folder's `shots/`: `x.webp` and its dark `x.dark.webp`; png, jpg or webp, up to 8). The list (`GET /v1/templates`) has them for myco's own apps only |
| `GET /v1/templates/{id}/files/{path}` | From v0.21.5: one of its screenshots (`shots/…` pictures only; anything else is `404`), with its image type, `X-Content-Type-Options: nosniff` and `Cache-Control: private, max-age=3600`. Older hubs answer 404 |
| `DELETE /v1/templates/{id}` | `204`; pages made from it stay |
| `POST /v1/templates/sync` | From v0.20.7: fetch myco's official apps now (the hub does once a day; at most once a minute) → `{apps, updated?, source, fetched_at?, error?}` |
| `POST /v1/pages/{id}/upgrade` | From v0.20.7: a page made from an app takes the app's newest version (new files, new data structure; its records stay) → the page; `409` when it is already at it. A page's `template.latest` says when there is one |
| `POST /v1/templates/{id}/use` | `{name?, project?, version?, sample?}` → `201 {page, template}`: a page made from it (its `template` says which: `{id, slug, name, version, data, permissions, os, net}`; from v0.22.9 also `declares: true`, meaning `permissions` and `os` are all the app declares, so Apps refuse what they leave out, and `official` for one of myco's own apps), with the sample data when asked |

## Automations

An automation sends a task, as its owner, to one agent when its schedule comes round or its webhook is called. It runs on the hub (each workspace's own), so it fires while the owner's computers are off. The message is an ordinary task from the owner's console address (`@console:<member>`) with `automation` set: it wakes the agent as usual, and the answer lands in the owner's chat with that agent. Older hubs answer `404` on these routes.

| Endpoint | Who | Notes |
|---|---|---|
| `GET /v1/console/automations` | console | `{automations: [automation]}`: everyone's for the hub's owner, admins and the admin token; a member's own otherwise |
| `POST /v1/console/automations` | console | `{name, to, body, project?, schedule?: {cron, tz?}, webhook?, skip_if_busy?, enabled?}` → `201 {automation, webhook_secret?}`. Or, instead of `to` and `body` (v0.21+), `workflow` (a workflow's id or name the caller may run) and `inputs` (`{key: template}`; `{{payload}}` is a webhook call's body, `{{payload.issue.title}}` a field of its JSON): each run starts that workflow in the owner's name (`trigger: automation`), and the run records `workflow_run`; with `skip_if_busy`, a run is skipped while the last workflow run it started is going. `workflow: ""` on an update goes back to sending a task. `name`, `to`, `body` are required, and a schedule or a webhook (or both). The caller owns it and it sends from the caller's console address. `webhook_secret` (`whk_…`) is shown only here, when the webhook is turned on, and on `rotate_secret`; the hub keeps only its hash |
| `GET /v1/console/automations/{id}` | its owner, or who manages the hub | `{automation, runs}`: its runs (up to 200), newest first; a webhook run's `request` without its `body` (hubs before v0.20.18: the last 50) |
| `PATCH /v1/console/automations/{id}` | its owner, or who manages the hub | The same fields, each optional (absent: unchanged), plus `clear_schedule`, `webhook: false` (removes the webhook), `rotate_secret` (a new secret; the old one stops working) → `{automation, webhook_secret?}` |
| `DELETE /v1/console/automations/{id}` | its owner, or who manages the hub | `204`. Its runs go too; the messages it sent stay |
| `POST /v1/console/automations/{id}/run` | its owner, or who manages the hub | Runs it now (`trigger: manual`), also when turned off → `201` the run |
| `GET /v1/console/automations/{id}/runs/{run}` | its owner, or who manages the hub | One run, with what its webhook call carried (`request.body` included). Hubs from v0.20.18 |
| `POST /hooks/{id}` | the webhook secret | Public. The secret in `Authorization: Bearer <secret>` (or `?token=<secret>`). The body (text, JSON kept as it is; at most 16 KiB, else `413`) replaces `{{payload}}` in the message; a message without `{{payload}}` gets it appended after `\n\n---\n`. → `202 {run}`. A wrong or missing secret, an automation turned off or without a webhook all answer `404 not_found`. At most 30 calls an hour per automation: then `429 rate_limited` with `Retry-After` (seconds) |

Others get `404 not_found` for an automation that is not theirs. Creating, changing, removing and running one are audited (`automation.*`).

An automation:

```json
{
  "id": "aut_...", "name": "Daily report", "owner": "alice", "from": "@console:alice", "to": "pm",
  "project": "", "body": "Write today's daily report",
  "schedule": {"cron": "0 9 * * 1-5", "tz": "Asia/Shanghai"},
  "webhook": true, "webhook_url": "https://hub.example.com/hooks/aut_...",
  "skip_if_busy": true, "enabled": true,
  "created_at": "...", "updated_at": "...", "next_run_at": "...",
  "last_run": {"id": "arun_...", "automation_id": "aut_...", "at": "...", "trigger": "schedule", "outcome": "sent", "message_id": "msg_..."}
}
```

A run: `trigger` is `schedule`, `webhook` or `manual`; `outcome` is `sent` (with `message_id`), `skipped` or `failed` (with a short `error`). The last 200 runs of each are kept.

A webhook run also keeps what the call carried (hubs from v0.20.18), as `request`: `{"content_type": "application/json", "headers": {"User-Agent": "GitHub-Hookshot/…", "X-Github-Event": "workflow_run"}, "body": "…", "size": 1834}`. Only headers that say what sent the call are kept: `User-Agent`, `Content-Type`, `Content-Encoding`, and `X-…` headers whose names have nothing of auth, token, secret, signature, key, cookie, session, password, forwarded, real-ip, client-ip, connecting-ip, original or proxy (at most 24, each value at most 300 bytes). The secret, `?token=`, signatures and the caller's address are never kept. Calls answered `404`, `413` or `429` record no run.

Rules:

- `schedule.cron` has 5 fields: minute, hour, day of month, month, day of week (0-7, 0 and 7 are Sunday). Each takes numbers, `*`, `a-b`, `a,b`, `*/n` and `a-b/n`. When both day fields are restricted, either one matching is enough (as in cron). `tz` is an IANA zone (`Asia/Shanghai`); empty is UTC. Two runs in a row must be at least 10 minutes apart, otherwise `invalid_argument`. On a daylight-saving change a time that does not exist that day is skipped, and one that happens twice runs once.
- `next_run_at` is set while a schedule is on. A run missed while the hub was down runs once when it is back, then the schedule goes on.
- `skip_if_busy`: while the last task it sent is not `done` or `failed`, a run is recorded as `skipped` and sends nothing.
- A run whose send fails (the agent removed, the sender not allowed) is recorded as `failed`; the automation stays on.
- Limits: 200 automations per workspace, `name` 80 characters, `body` 8 KiB.
- End-to-end encryption: the hub writes the message, so it is sent in plain text even when the hub has encryption on.

## Workflows

A workflow is a graph the hub walks (v0.21+). Each agent step is an ordinary task sent from the run's owner's console address (`@console:<member>`) with `workflow` set to the run's id; the agent answers with `myco message reply --result` and the hub moves on. Computer steps are commands for an exec node; human steps wait for a decision (`POST …/decide`). The hub alone decides what comes next. Older hubs answer `404` on these routes.

Both people (a console or admin token) and agents (their own token, acting for the member who added them; on hubs without members, for the owner) may use them. Others' workflows and runs answer `404`; `…/decide` also takes the person the step names.

| Endpoint | Notes |
|---|---|
| `GET /v1/workflows` | `{workflows: [workflow]}` (without `definition`): the caller's; everyone's for those who manage the hub. Each workflow includes optional `run_counts: {total, running, waiting, succeeded, failed, cancelled}` across all its runs and versions (not a recent-run sample); `active` is running + waiting. Older Hubs omit `run_counts`; absence does not mean zero |
| `POST /v1/workflows/validate` | `{definition}` → `{valid, problems?, warnings?}`. Warnings (an agent that does not exist yet, a computer step to something that is not an exec node) do not block saving |
| `POST /v1/workflows` | `{definition}` → `201 {workflow, created, changed, warnings?}` (or `200` when it exists): creates the workflow named in the definition, or gives it a new `version` when the definition differs. Runs going on keep the version they started with. `?dry_run=true` checks only. Someone else's name: `permission_denied` |
| `GET /v1/workflows/{ref}` | `ref` is the id or the name → the workflow with `definition` |
| `PATCH /v1/workflows/{ref}` | `{enabled}`: a workflow turned off cannot be started |
| `DELETE /v1/workflows/{ref}` | `204`. Its runs go too (the ones going are cancelled first); the tasks they sent stay |
| `POST /v1/workflows/{ref}/runs` | `{inputs?}` → `201` the run (with steps). Inputs are checked against the definition's `inputs` schema (text is turned into the numbers, booleans and lists it asks for; defaults filled). `?dry_run=true` → `{inputs, step, target, body, dry_run}`: the first step's task, unsent |
| `GET /v1/workflow-runs?workflow=&active=true&status=&limit=` | `{runs: [run]}` (without steps), newest first; `status` (v0.21.1+) is one of `running waiting succeeded failed cancelled`; `limit` default 50, at most 200. Each run has `duration_ms` (to now while it goes; v0.21.1+) |
| `GET /v1/workflow-runs/{id}` | The run with `steps`, `events` (its log, the last 500 lines) and the `definition` it follows. From v0.21.1 also `metrics`, `artifacts` and `evaluations` (below); each step has `duration_ms`, `attempts` (tasks sent: the first, retries, send-backs), `bounces`, `tasks` (their ids) and `usage` |
| `GET /v1/workflow-runs/{id}/steps/{step}/log?trace=false` | v0.21.1+. `step` is a step start's id (`rs_…`) or a step id (its latest start) → `{run, step, entries: [{at, kind, sub?, try?, from?, text?, attachments?}], clipped?}`, in time order. `kind`: `task` (what was sent; each retry or send-back is another `try`), `progress`, `reply` (with its structured result), `failure`, `event` (the run's log for this step), `stdout` / `stderr` (a computer step), `trace` (what the agent did, from its trace: `sub` is `text`, `thinking`, `tool`, `tool_result`, `tool_error`, `error`, `usage`, `output:<stream>`; left out with `trace=false`). At most 3000 lines (the last) |
| `GET /v1/workflow-runs/{id}/evaluations` | v0.21.1+. `{evaluations: [evaluation]}` |
| `POST /v1/workflow-runs/{id}/evaluations` | v0.21.1+. `{name, value, unit?, direction?, step?, note?}` → `201` the evaluation (`{id, run, step?, name, value, unit?, direction?, note?, by, at}`). `name`: letters, digits, `_ . -`, starting with a letter; `value` a number; `direction` `higher` or `lower` (which is better; empty: neither); `step` a step id of the workflow. Those who may see the run, and the agents its steps went to (an agent step that checks the work), may score it. A run keeps its last 200. `?dry_run=true` checks only |
| `GET /v1/workflow-runs/compare?a=&b=` | v0.21.1+. Two runs side by side → `{a: {run, metrics}, b: {run, metrics}, same_version, steps: [{step, title?, a?, b?, faster_in?, delta_ms, delta_cost_usd?}], evaluations: [{name, unit?, direction?, a?, b?, delta, better?}]}`. A step's side (`{status, runs, attempts, duration_ms, usage?, summary?, error?}`) adds up all its starts in that run; deltas are B − A; a score is each run's latest of that name |
| `GET /v1/workflows/{ref}/stats?limit=&version=` | v0.21.1+. A workflow's figures (below) |
| `POST /v1/workflow-runs/{id}/cancel` | Stops it: open tasks are stopped, decisions withdrawn → the run |
| `POST /v1/workflow-runs/{id}/retry` | `{from?}`: a failed or cancelled run starts again at that step (default: the step that failed), keeping earlier results → the run |
| `POST /v1/workflow-runs/{id}/decide` | `{step, choice, note?}`: `choice` must be one of the step's `options` → the run |
| `GET /v1/workflow-decisions` | `{decisions: [{run, step}]}`: decisions waiting for the caller |

Saving and removing a workflow are audited (`workflow.*`).

**Observability (v0.21.1+).** What a run did, cost and produced, worked out by the hub:

- `metrics`: `{duration_ms, active_ms, waiting_ms, steps, done, failed, attempts, retries, bounces, usage?}`. `active_ms` is the time some agent or computer step was at work (parallel steps counted once); `waiting_ms` the time a decision waited while nothing else was at work.
- `usage` (on a run, a step, the figures): `{input_tokens?, output_tokens?, cost_usd?}`, the last `usage` step of each of the step's tasks' traces as the agent's CLI reports it (Claude Code reports tokens and cost; others what they have). Kept on the step once it is over, since traces are dropped after a week.
- `artifacts`: the files the step's agent attached to its answers and progress reports, oldest first, each once: `[{step, step_run, try, kind: reply|progress, message, from, at, file_id, name, size, content_type?}]`. Download with `GET /v1/files/{file_id}` (`myco file get`).
- Figures (`GET /v1/workflows/{ref}/stats`): `{workflow, name, total, by_status, success_rate, sample, version?, duration_ms, active_ms, waiting_ms, usage?, cost_per_run?, steps: [{step, title?, kind, runs, done, failed, retries, bounces, duration_ms, usage?, last_error?, last_error_run?, last_error_at?}], versions: [{version, runs, succeeded, failed, cancelled, duration_ms}], evaluations: [{name, unit?, direction?, values, last, last_run}], recent: [point], failures: [point]}`. `total`, `by_status` and `success_rate` (succeeded / ended) cover every run; the rest the last `limit` runs (default 200, at most 2000), of `version` only when given. Distributions are `{n, avg, min, p50, p90, p95, max}` (nearest rank; durations in ms, over ended runs and step starts). A score's figures take each run's latest of that name. `recent` (newest first, at most 60) and `failures` (at most 10) are `{id, status, version, created_at, duration_ms, error?, failed_step?, cost_usd?}`.

A run an agent started (`trigger: agent`) tells it when it ends: a note from the run's owner, with `workflow` set, `[工作流「<name>」完成了|失败了：…|取消了：…]` (the hub writes this line in Chinese: "workflow <name> succeeded | failed: … | cancelled: …"), each step's summary and how to see the rest (not when the agent cancelled it itself). Automations can start workflows too (see [Automations](#automations)).

A workflow definition (all of it is checked; unknown fields are refused):

```json
{
  "name": "write-review-fix", "description": "", "project": "myco",
  "inputs": {"required": ["task"], "properties": {"task": {"type": "string", "title": "What to do", "format": "textarea"}}},
  "steps": [
    {"id": "write", "agent": "coder", "prompt": "Do this: {{inputs.task}}",
     "output": {"required": ["branch"], "properties": {"branch": {"type": "string"}}}, "timeout": "2h", "retry": 1},
    {"id": "review", "agent": "reviewer", "prompt": "Review {{steps.write.output.branch}}",
     "output": {"required": ["approved"], "properties": {"approved": {"type": "boolean"}, "comments": {"type": "array"}}}},
    {"id": "test", "computer": "studio-mac", "run": "make test"},
    {"id": "ok", "human": "$starter", "prompt": "Merge?", "options": ["Merge", "Don't merge"], "timeout": "1d"}
  ],
  "edges": [
    {"from": "write", "to": "review"},
    {"from": "review", "to": "test", "when": "output.approved"},
    {"from": "review", "to": "write", "when": "!output.approved", "maxIterations": 3, "else": "$fail"},
    {"from": "test", "to": "ok"},
    {"from": "test", "to": "write", "onError": true, "maxIterations": 2},
    {"from": "ok", "to": "$end", "when": "output.choice == 'Merge'"},
    {"from": "ok", "to": "$fail", "when": "output.choice != 'Merge'"}
  ]
}
```

Rules:

- A step sets exactly one of `agent` (with `prompt`, optional `output` schema), `role` (the same, but the task goes to an agent whose role matches — a label `role=<x>` or `<x>`, a skill named `<x>`, or `<x>` in its name, display name, description, or its note in the run's project — preferring the run's project members, then the one with the fewest open tasks, then the one seen in the last 10 minutes; none: the step fails), `computer` (an exec node, with `run`; `parse: "json"` reads stdout into `output.json`) or `human` (a member name or `$starter`, with `prompt` and at least two `options`). Ids: letters, digits, `_`, `-`, at most 40. At most 50 steps and 200 edges.
- `timeout`: `30m`, `2h`, `1d`, `1d12h`. A step past it fails (its task is stopped). A decision with no timeout waits 3 days; one that expires counts as no answer (`output.choice` is `""`, `output.timed_out` is `true`), never as yes.
- `retry` (0-5): a failed agent or computer step is sent again that many times.
- Inputs and outputs: a run's inputs are the entry step's input (`entry`, default the first step). When a step ends, the edge taken passes its result to the next step as that step's input, or, with `map`, an object it builds (`{"branch": "{{output.branch}}", "task": "{{inputs.task}}"}`; a value that is one `{{…}}` alone keeps its type, a missing one is `""`). A join's input is an object keyed by the steps it waited for. The task an agent gets shows its input as JSON.
- `inputSchema` and `outputSchema` (`output` is the same; set one) are full JSON Schema draft 2020-12 (`oneOf`, `anyOf`, `$ref`/`$defs`, `pattern`, `minimum`, `minItems`, `additionalProperties`, …), compiled when the workflow is saved. An input that does not fit fails the step before it is sent. An agent step's result: the reply's `result`, else the last `` ```json `` block of its body; one that does not fit (or none) is sent back once with each place that is wrong (`/parts/1: 'API' does not match pattern '^[a-z]+$'`); a second miss fails the step. The output always has `text` (the reply's body). `summary` is the result's `summary`, else the reply's first line. The run's `inputs` are checked against `inputs` the same way.
- A computer step's output is the exec result (`exit_code`, `stdout`, `stderr`, …); a non-zero exit fails it. A human step's output is `{choice, note, by}`.
- Edges out of a step are tried in order and the first whose `when` holds is taken (all of them with `"dispatch": "all"` on the step). `onError` edges are tried only when the step failed, the others only when it succeeded; a failed step with no `onError` edge fails the run, and a step with edges none of which holds fails it too ("no way on"). A step with no edges ends its branch. With no `edges` at all, steps run in order.
- An edge to the same or an earlier step (in `steps` order) needs `maxIterations`; once taken that many times, `else` is taken instead (`$fail` when unset). `else` cannot go back.
- `"join": "all"` on a step: it starts once every step with an edge into it has arrived; `"join": {"from": ["a", "b"]}` names them (at least two, and every edge into it must come from one of them). Its input is `{"a": …, "b": …}`. The run fails if it ends with a join that never filled. `"dispatch": "first"` (the default) or `"all"`.
- `allowUnboundedCycles: true` lets an edge go back without `maxIterations` (a run stops after 2000 steps instead of 200).
- `title`, `tags` (at most 20) and `group` are for people; `name` stays the identifier.
- `$end` ends a branch; the run succeeds when nothing is open. `$fail` fails it. A run that has started 200 steps stops.
- `when`, `map`, and `{{…}}` in `prompt`, `run`, `title`, `agent`, `role` and `computer`, are expressions over `inputs` (the run's), `input` (this step's), `steps.<id>` or `nodes.<id>` `.output` (or `.result`), `.input`, `.summary`, `.status`, `.error`, `.attempt` (the latest finished time of that step), `run.id|name|by`, and on an edge (`when`, `map`) `output`, `status`, `error`, `attempt` of the step that ended: paths (`a.b`, `a[0]`, `a["k"]`), literals, `== != < <= > >= && || ! and or not in`, `+ - * / %` (`+` also joins text), `len contains startsWith endsWith matches lower upper trim`. Missing values are `null`; comparing different kinds is false. `{{#if expr}}…{{else}}…{{/if}}` keeps one part. Saving refuses a reference to a step that is not there, a field a step does not have, or an input `inputs` does not declare.

A run:

```json
{
  "id": "run_...", "workflow_id": "wf_...", "name": "write-review-fix", "version": 2, "project": "myco",
  "status": "running|waiting|succeeded|failed|cancelled", "inputs": {"task": "Add an export button"},
  "owner": "alice", "by": "alice", "trigger": "manual|agent|automation", "error": "",
  "current": ["review"],
  "steps": [{"id": "rs_...", "step": "write", "title": "", "kind": "agent", "attempt": 1, "status": "done",
             "target": "coder", "message_id": "msg_...", "input": "…the task as sent…", "payload": {"task": "Add an export button"},
             "output": {"branch": "feat/x", "summary": "Added", "text": "…"}, "summary": "Added",
             "started_at": "...", "ended_at": "..."}],
  "events": [{"at": "...", "kind": "started|step|done|failed|bounced|edge|decided|timeout|retry|finished", "step": "write", "text": "…"}],
  "created_at": "...", "updated_at": "...", "ended_at": "..."
}
```

`waiting`: only decisions are open. A step's `status` is `running`, `waiting` (a decision), `done`, `failed` or `cancelled`; `options` and `due_at` come with decisions.

## Superagents

A person's own agent with the App's powers in its workspace (v0.21.1+). Only the person names it; it uses its **own agent token** on the App's routes, and the hub acts as that person, marked as through it (the audit log names it: `alice（经 superagent shroom）`, the hub's Chinese for "alice (via superagent shroom)"). Each route the App uses is listed in `internal/transport/httpapi/superagent.go`:

- **allowed**: reading what its owner sees (overview, conversations, traces, machines, members, audit, projects), and things that harm nothing that exists: creating projects and putting agents in them, project docs and files, starting an agent on its owner's computers (`POST /v1/machines/{id}/spawn`), run settings, running an automation once, renaming a computer;
- **held for approval**: deleting or removing anything (projects, members of projects, docs, files, agents, computers), visibility and ownership, new or changed automations, stopping a task, upgrades and hub settings. The hub answers `409 {error: "approval_pending", message}` and keeps the request (with `X-Myco-Reason`, its why); the owner approves or denies it, and an approved one is carried out once, as it came. The agent gets a note from its owner either way. Pending ones lapse after 24 hours;
- **never** (not listed): tokens, enrollments, pairing and devices, keys, members, handovers, the superagent itself and approvals, and anything that speaks as its owner (console messages, steers, uploads): it always speaks as itself.

A route not listed treats it as the ordinary agent it also is. Nothing widens what its owner can do. One superagent per member in a workspace.

| Endpoint | Auth | Notes |
|---|---|---|
| `GET /v1/console/superagent` | console | `{agent?, pending}`: the caller's superagent and how many of its requests wait |
| `PUT /v1/console/superagent` | console (the person themselves) | `{agent}`: one of the caller's own agents (kind agent); replaces any before (whose pending requests are withdrawn). `?dry_run=true` |
| `DELETE /v1/console/superagent` | console (the person themselves) | `204`: an ordinary agent again |
| `GET /v1/console/approvals?status=pending` | console | `{approvals: [{id, agent, what, reason, method, path, body?, status, result?, result_body?, decided_by?, created_at, expires_at, decided_at?}]}`; `status` pending done denied expired withdrew |
| `POST /v1/console/approvals/{id}/approve` | console (its owner, not the superagent) | carries it out once → the approval with `result` (the HTTP status) and the start of `result_body` |
| `POST /v1/console/approvals/{id}/deny` | console (its owner) | → the approval |
| `GET /v1/me/superagent` | agent | `{superagent, owner?}` |
| `GET /v1/me/approvals` | agent | what it asked for |

Approvals also carry `kind` and `payload` (v0.21.21): an empty `kind` is a superagent's held request as above; `kind: "action_grant"` is any agent asking to act on a computer (see [Agent actions](#agent-actions)), with `payload: {grant_id}`. Approving one may send `{permissions?, ttl_seconds?, hosts?}` to allow less than was asked (never more); an empty body allows all of it.

## Agent actions

From v0.21.21, an agent can act on a computer through a provider that computer's own myco App runs (`remote.desktop`: screenshots, keyboard and mouse), with its owner's approval. The hub checks grants and relays jobs; it never reads their arguments or results, and it keeps grants, jobs and artifacts in memory only (a hub restart ends every session). In an end-to-end encrypted workspace these routes answer `unsupported`. The contract (fields, provider actions, lifecycle) is `context/design/app-agent-actions.md` in the maintainers' repository.

**Pages** (next version): a target can also be a shared page (`{kind: "page", id: "pg_..."}`) with the provider `page` and its one permission and action, `page.export` (v1): a file the page's own app makes of the page's shared content (SVG, PNG, or whatever the app declares in its action schema). An agent that may see the page gets the grant **active at once**, with no approval, because the file holds nothing it could not already read; the page's owner sees it among their grants and may revoke it. Page visibility is checked again when a job is run, handed to a renderer, completed, and when its artifact is downloaded: an agent that can no longer see the page loses the grant. A renderer is registered by a person, not a computer (`provider: "page"`, no `machine_id`), and gets only jobs for pages that person may see. The desktop App is the renderer: it loads the page in a sandboxed frame with that page's read-only data and calls the app's export handler.

| Route | Who | |
|---|---|---|
| `POST /v1/me/action-grants` | agent | `{target: {kind: "machine", id}, permissions, reason, message_id?, page_id?, ttl_seconds?}` → `201` a pending grant `{id, agent, owner, target, target_name, permissions, requested, reason, message_id?, approval_id, state, end_reason?, created_at, expires_at?, ended_at?}`; the target's owner gets an `action_grant` approval. `id` may be the computer's name. Permissions: `remote.desktop.view`, `remote.desktop.control` (control includes view). TTL 5 minutes by default, 30 at most. With `message_id`, the grant ends when that task ends. `?dry_run=1` checks only |
| `GET /v1/me/action-targets` | agent | `{targets: [{kind:"machine",id,resource:{provider:"server.ssh"|"browser",id,binding,name,summary}}]}`: enabled private profile resources belonging to the agent's own person in this workspace. Discovery is not permission to execute. Older hubs return 404 |
| `GET /v1/me/action-grants` | agent | `{grants: [...]}`, its own |
| `GET /v1/me/action-grants/{id}?wait_seconds=30` | agent | the grant; waits while it is pending (`wait=30s` works too; 60 seconds at most) |
| `POST /v1/me/action-grants/{id}/end` | agent | ends it → the grant |
| `GET /v1/me/action-grants/{id}/actions` | agent | `{actions: [{name, version, title, permission, input_schema?, output_schema?}]}`: what the computer offers now that the grant covers |
| `POST /v1/me/action-jobs` | agent | `{grant_id, action, version, args, idempotency_key}` → `201 {id, grant_id, action, version, state, result?, error?: {code, message, retryable}, artifacts?, cancel_requested?, created_at, finished_at?}`. The same key and body returns the first job; a different body is `409`. `unsupported` when the computer does not offer that action now |
| `GET /v1/me/action-jobs/{id}?wait_seconds=30` | agent | the job; waits until it ends. `state`: queued running succeeded failed cancelled |
| `POST /v1/me/action-jobs/{id}/cancel` | agent | cancels a queued job; asks a running one to stop (what it did stays done) |
| `GET /v1/me/action-artifacts/{id}` | agent (the grant's) | the file (a screenshot), `Cache-Control: no-store`; `410` when it expires (usually within 10 minutes or when the grant ends, except live browser screenshots below) |
| `GET /v1/console/action-grants?state=active` | console | `{grants}` the caller decides (all, for owners and admins) |
| `POST /v1/console/action-grants/{id}/revoke` | console | ends it at once: queued jobs are cancelled, a running job's late result is dropped, artifacts are gone |
| `POST /v1/console/action-hosts` | console (the computer's owner) | the trusted App offers a provider there: `{machine_id, provider, generation, actions}` (a page renderer: `provider: "page"`, no `machine_id`) → `201 {id, machine_id, provider, generation, last_seen_at}`. Registering a computer's provider again replaces its host (its running jobs fail as `host_restarted`). A person may run several page renderers (one per App); a new one replaces only theirs that stopped polling (45 s). A host not seen for 10 minutes is dropped. After `404`/`409` on its job poll, an App waits 30 s before registering again |
| `DELETE /v1/console/action-hosts/{id}?generation=...` | console | `204`; actual host owner only, including on an administrated Hub. Exact generation required (`409` if changed). Withdraws the host and revokes active and pending grants for the same machine/provider/profile id across bindings. Queued/running jobs are cancelled with `error.code: revoked`; a running action may already have side effects and must not be blindly retried. Existing outcome/error details are preserved. Local execution must stop/refuse first; this request uses a fresh bounded context. Missing/older Hub `404` never permits local execution or unbound fallback |
| `GET /v1/console/action-hosts/{id}/jobs?wait_seconds=30` | console | `{jobs: [{job, args, grant}]}`: queued jobs for that computer, now running. A host quiet for 45 seconds ends the grants it ran |
| `GET /v1/console/action-jobs/{id}?host_id=&generation=&wait_seconds=30` | console | `{job, grant_state, grant_expires_at?, stop}` for a job the host runs. `stop` turns true when the agent cancels it, its grant ends (revoked, expired, its task ended) or it is no longer running; with `wait_seconds` it answers as soon as it does. The host asks right before anything native happens and keeps asking while it runs |
| `POST /v1/console/action-jobs/{id}/artifacts?host_id=&generation=` | console | the raw file (`Content-Type` image/*, application/pdf, Office (OOXML), text/plain, text/csv, text/markdown, text/html or application/json; 4 MB at most; the agent downloads it as an attachment, nosniff and sandboxed) → `201 {id, media_type, bytes, expires_at}` |
| `POST /v1/console/action-jobs/{id}/complete` | console | `{host_id, generation, state, result?, error?, artifact_ids?}` → the job. A wrong generation is `409`; a finished job cannot change |

The CLI: `myco action grant request|list|get|end`, `myco action list`, `myco action run` (waits, and saves screenshots to files), `myco action job`, `myco action cancel`.

### Bound browser profiles

The trusted desktop App can host `browser` for a dedicated local browser profile
that its owner explicitly enabled for agents **in this workspace**. Discovery via
`GET /v1/me/action-targets` returns `{kind:"machine",id:machine_id,
resource:{provider:"browser",id:profile_id,binding,name,summary}}`. The opaque
`binding` identifies the local authorization policy generation. The owner must
approve a short-lived grant to this exact target; knowing its id grants no access.
Unopened profiles, stale bindings, mixed providers and machine-wide browser grants
are rejected. The host must belong to the computer's actual owner. These actions
are unavailable in end-to-end encrypted workspaces.

M2 offers five v1 actions. `browser.view` and `browser.run` are independent
permissions; request both when needed. The Hub fixes the action-to-permission
mapping, and the host registers only capabilities its installed component supports.

| Action | Permission | Arguments |
|---|---|---|
| `browser.profiles.list` | `browser.view` | `{}`; only the bound, enabled profile is disclosed |
| `browser.script.run` | `browser.run` | `{script_id,version,page_id?,params?,headless?}` |
| `browser.run.status` | `browser.view` | `{run_id}` |
| `browser.run.resume` | `browser.run` | `{run_id,choice?}` |
| `browser.run.cancel` | `browser.run` | `{run_id}` |

Live browser control adds six v1 actions when the component advertises
`agent_control`, plus replay when it also advertises `agent_replay`. They require the independently approved `browser.control`
permission on the exact machine and browser profile. Grants last at most 30
minutes. The browser opens visibly so the owner can watch and stop it.

A request without a target resource defaults to the selected machine’s
dedicated agent profile (`kind:"agent"`). The desktop creates this clean
profile when live control first becomes available. An explicit profile
resource must include its current binding. The approval identifies the
profile and warns about existing sign-ins when using a personal profile.

A control request can include `hosts` (CLI `--host`, repeat it for each domain).
Entries are exact hostnames or subdomain patterns such as `*.example.com`.
The wildcard does not include the parent hostname. Omitting the list permits
all domains. The owner can approve a subset by sending `hosts` in the approval
body. To allow none, deny the request. The approved list applies to top-level
navigation, including redirects and new tabs. A blocked navigation returns
`host_not_allowed` and is audited. Scoped requests require support in the Hub
and in the controller App.

| Action | Arguments | Result |
|---|---|---|
| `browser.page.open` | `{profile,url?}` | `session,active_tab,tabs,snapshot,expires_at,idle_expires_at,start_url` |
| `browser.page.snapshot` | `{session,tab?,mode?,max_nodes?,include_screenshot?}` | A snapshot with `snapshot_id,frames,node_count,truncated` and accessibility nodes or text |
| `browser.page.act` | `{session,op,tab?,ref?,value?,key?,url?,dy?,wait?,choice?,timeout_ms?,clear?}` | `active_tab,tabs,new_tab?,step_ids,snapshot?,snapshot_error?` |
| `browser.page.extract` | `{session,tab?,ref,what,attr?}` | Extracted text |
| `browser.page.screenshot` | `{session,tab?,ref?,clip?,full?}` | Screenshot metadata and a private job artifact |
| `browser.page.close` | `{session}` | `steps,removed,next,done:true,reason:"closed"` |
| `browser.page.run` | `{session,graph:{nodes,edges},tab?,timeout_ms?}` | `ok,state,error_node?,error?,steps,output,url,tabs` |

A screenshot denied with `sensitive_snapshot` can include a fixed `reason`:
`protected_content` (a sensitive field was found), `privacy_unavailable`
(the document could not be checked), `frame_unavailable` (an unreadable frame
could not be safely masked), or `privacy_changed` (the checked document or
frame state changed during capture). The same reason appears in a failed
`browser.page.run` screenshot node's `error`. No reason contains page content.

`browser.page.run` verifies a literal draft in the same live session, under the
same grant, profile, agent and allowed hosts. It does not need an approved M2
script version. Both the App and component must support replay; older components
keep the original six actions. Replaying does not add steps to the manual draft,
and invalidates earlier refs before and after execution. To test a closed draft,
open a new session, prepend a `navigate` node with the returned `start_url`, and
connect the unchanged returned steps in linear order.

A replay graph has 1–200 unique nodes, at most 192 KiB of JSON, one linear start,
and no disconnected nodes, cycles or branches. Node fields `pos`, `title`,
`timeout_ms` and `on_error` are accepted. `on_error` permits `stop`, `skip` or
`retry:1` through `retry:5`; ambiguous outcomes and scope/session failures always
stop. Retrying a website operation can repeat its effects: use it deliberately.
Allowed node types are `navigate`, `back`, `forward`, `reload`, `click`, `type`,
`select`, `press`, `hover`, `scroll`, `wait`, `assert`, `extract`, `screenshot`,
`tab`, `dialog` and `note`. Locators use `testid`, `role`, `label`, `placeholder`,
`text`, `css` or `xpath`, with the draft's frame/shadow paths. Clicks use the left
button with optional count 2. Assertions support visible/hidden/text/URL;
extraction supports text/attribute/HTML/table with sensitive content redacted.
Waits support element/network/URL/time (1–60000 milliseconds), not downloads.
Tabs support popup/new/opener/index/close. JavaScript, expression assertions,
uploads, child flows, loops, branches, confirms, parameters, `code` fields and
any decoded `${` in keys or values are refused before any node executes.

The default total replay deadline is 300000 milliseconds; `timeout_ms` may
shorten it, and the job/grant deadline always wins. Per-node timeouts and retries
remain inside that deadline. A running replay counts as activity for the
five-minute idle timer; owner stop and grant expiry still cancel it. Other
session operations return `busy` during replay. Sensitive typing, disallowed
navigation/popups/redirects and screenshot masking follow live control rules.

An executed replay returns `state:"succeeded"` or `state:"failed"`, with
`steps:[{node,state:"ok"|"failed"|"skipped",ms,error?}]`. A failed node includes
`error_node` and a safe `error:{code,message,retryable,reason?}`; earlier effects
may already have happened. The outer job succeeds when this confirmed envelope
is received. Preflight, authorization and session errors use the usual failed-job
path; validation can include optional `error_node` in the HTTP/CLI error too.
A transport loss becomes `outcome_unknown`: inspect the page, and do not submit
the same graph under a fresh idempotency key automatically. The native component
retains same-request outcomes while the session remains valid.

`output` contains values named by nodes' `into`, bounded to 16 KiB, with strings
clipped to 2000 characters and `truncated:true` when entries cannot fit. A screenshot
node with `into` returns `{artifact_id,image}` and adds that private artifact to
the job's `artifacts`; it never exposes a component artifact ID or public URL.
Screenshot nodes without `into` have no output value. The action argument envelope
allows 256 KiB for replay only; the graph's 192 KiB limit still applies.

Operations are limited to `navigate`, `click`, `dblclick`, `hover`, `type`, `select`,
`press`, `scroll`, `back`, `forward`, `reload`, `switch_tab`, `close_tab`,
`wait`, and `dialog`. Extra fields and URL schemes other than HTTP(S) are
rejected. A wait uses `wait:"element"` with `ref`, `wait:"url"` with `value`,
or `wait:"network"`. The maximum operation timeout is 30 seconds. Screenshot
targets `ref`, `clip` and `full` are mutually exclusive. Each image is kept
as a private job artifact up to 4 MiB. A successful live browser job retains
its screenshot for seven days. Only the initiating agent and the computer's
owner can download it, including after the grant ends. The Hub deletes
expired screenshots and excludes them from workspace backups and archives.

- The initiating agent downloads a screenshot at `GET /v1/me/action-artifacts/{id}`.
- The computer owner signs in directly and lists their 100 most recent
  available screenshots at `GET /v1/console/action-artifacts` (`{artifacts}`).
  The owner downloads one at `GET /v1/console/action-artifacts/{id}`.


Refs are scoped to a session, tab, frame and document. A stale ref returns
`stale_ref`. Take a fresh snapshot before retrying. A successful operation
with a failed follow-up snapshot still succeeds and returns `snapshot_error`.
An `outcome_unknown` error calls for observing the page before another action.
The component limits a draft to 200 steps and 192 KiB. Close explicitly to
retrieve the draft. Idle sessions end after five minutes. Grant expiry,
revocation, or owner stop ends the session and discards its draft. Subsequent
calls on an owner-stopped grant return `session_closed` with
`reason:"stopped_by_owner"`.

When completing a live browser job, the trusted desktop can send the optional
`browser_host` field containing only the observed hostname or IP address.
The Hub audits the action, host and time once per job. The detail records
`host_not_allowed` for a blocked host or `screenshot` when a screenshot
was retained. Request arguments, page contents and image bytes stay out of
the audit log.

For `browser.script.run`, shared page records are **not** approval. The desktop executes only the immutable
script version and child-script snapshots the computer's owner reviewed and
approved locally for that profile. An absent, revoked or mismatching version is
`permission_denied`; the host never fetches a newer shared script as a fallback.
Runs and their cancellation/resumption remain bound to the originating agent,
grant and profile. Revoking a script approval or disabling agent access changes
the policy binding, ends old grants and cancels their running jobs. Results and
screenshots use private job artifacts; they are not automatically published in
shared page data. A paused confirmation is reported to the caller, never approved
by merely changing a shared record.

`script.run` and `run.resume` keep their job running until the native run finishes
or pauses. A paused result is a successful action job with `result.state:"paused"`
and `run_id`; it is not a completed script. Results contain `run_id`, `state`,
`node`, masked `output`, optional `question` / error details, and an optional
`screenshot` private artifact descriptor. Supply `page_id` to disambiguate script
ids; omitting it only works for a unique locally approved id/version. Resume uses
the original grant, agent and profile, with `choice:"ok"` or `"cancel"`. Stop a
running script by cancelling its action job; `run.cancel` handles paused runs.
Paused native sessions are cancelled at the earlier of grant expiry and 15
minutes after pausing. A revoked grant cannot resume, even before local cleanup.

Agent arguments cannot carry executable scripts, child snapshots, nodes, uploads,
secrets, `start_at` or breakpoints. `params` accepts only declared non-secret
fields. All executable `code` fields must be fixed text: `${...}` is rejected as
`invalid_graph` with its node, for both human and agent runs. Dynamic text/URL
fields remain supported. Unsupported graph nodes fail before the browser opens;
the current 0.1.0 component supports connected acyclic sequences, not subflows.
A failed or cancelled run may already have changed a website. Inspect the result
before retrying; the host never automatically repeats an uncertain operation.

Live `browser.page.snapshot`, `browser.page.act` and `browser.control` are reserved
for M3 and are not offered in M2. Hard calls return `unsupported`. Future control
approval must explicitly warn that it can read all websites signed in through the
profile. An unsupported component capability or older Hub must not trigger an
unbound or more privileged fallback.

Example (copy the ids and binding from discovery):

```sh
myco action targets
myco action grant request --machine COMPUTER --browser-profile PROFILE --binding BINDING \
  --permission browser.view --permission browser.run --reason "Run the approved report" --wait 5m
myco action list --grant GRANT
myco action run --grant GRANT --action browser.script.run \
  --args '{"script_id":"report","version":3,"page_id":"pg_reports","params":{"month":"2026-11"}}'
myco action grant end --id GRANT
```

### Bound SSH resources

The desktop App can host `server.ssh` for a private, immutable OpenSSH profile.
A grant must supply `target: {kind:"machine", id:controller_machine_id,
resource:{provider:"server.ssh", id:profile_id, binding:sha256}}`. The trusted
host, not the agent, supplies the profile name and resolved `user@host:port`
summary shown in the approval. `server.ssh.read` allows overview, service and
container lists/status/logs; `server.ssh.manage` additionally allows fixed
start/stop/restart operations and implies read. Providers cannot be mixed in a
resource grant. There is no arbitrary agent shell action.

Host registration includes the same `resource` with `name` and `summary`.
Only the controller machine's actual owner may register or poll an SSH host;
hub management privileges do not substitute for ownership. Different profiles
on one machine have separate hosts. Matching requires the same provider, profile
ID and SHA256 binding at both the Hub and the native host. A changed binding
revokes existing grants, including pending requests; a restarted host with the
same binding can serve the same target. Old hubs/Apps are unsupported: clients
must never retry SSH grants without their resource binding.

Action names (v1): `server.ssh.overview`, `server.ssh.services.list`,
`server.ssh.services.status`, `server.ssh.services.logs`,
`server.ssh.services.start|stop|restart`, `server.ssh.containers.list`,
`server.ssh.containers.logs`, `server.ssh.containers.start|stop|restart`.
Arguments only contain `{name?,lines?}` (systemd `.service` unit/container name, log lines 1–200,
default 100); a profile or host override is rejected. Results are
`{stdout,stderr,exit_code,truncated,duration_ms}`. The SDK parses the fixed text
formats (`overview` uses `@@myco:` section markers, container lists use Docker
JSON lines). The optional `@@myco:identity` section contains `hostname=<value>`
and `cpus=<positive online CPU count>` lines, read with `uname -n` and
`getconf _NPROCESSORS_ONLN`. Empty values mean unavailable, never zero cores.
Older hosts omit this section; parsers must ignore unknown sections and preserve
unknown/missing fields. Nonzero remote exit codes are returned as results, not success
claims about the service. Output is capped at 256 KiB total and each operation
at 30 seconds. JSON results over 200 KiB use a private `application/json` action
artifact; the job result then contains `{artifact,exit_code,truncated,duration_ms}`.
Cancellation/disconnection during a mutation returns `outcome_unknown`, never an
automatic retry. Read current status before deciding to retry.

`myco action targets` discovers resources; request using
`myco action grant request --machine mch_... --ssh-profile ssh_... --binding
<sha256> --permission server.ssh.read --reason "inspect server"`. Grants,
expiry, private results and audit follow the same framework lifecycle as desktop
actions, including its current restriction in encrypted workspaces.

### Local SSH capabilities

Updated desktop bridges advertise `ssh` and offer `myco.os.ssh.profiles(args)` /
`myco.os.ssh.run(args)` (also `myco.os.call("ssh.profiles", args)` and
`myco.os.call("ssh.run", args)`). Detect capability availability before use;
missing OpenSSH or an old/mobile bridge is unsupported.

Profiles use `~/.myco/ssh/<stable-id>.json` (0600). `ssh.profiles` accepts
`{op:"list"}` → `{profiles:[...]}`;
`{op:"create",name,destination,user?,port?}` → a profile;
`{op:"delete",id}` → `{ok:true}`;
`{op:"agent_enabled",id,agent_enabled:boolean}` → updated profile.
Public profiles contain `{id,name,destination,host,user,port,binding,
agent_enabled,agent_hub?}`; no config, key paths, passwords or key contents.
Opt-in binds agent access to the page's current workspace (`agent_hub`), not all
workspaces on this computer. Enabling authenticates and verifies Linux first.
A profile's destination cannot be edited in place: recreate it and approve again.

`ssh.run` accepts `{profile_id,binding?,action,name?,lines?}` with the short
native action names (`overview`, `services.list`, etc.; `probe` returns `uname -s`).
These human calls do not create agent grants or execute shared page records.
Both native and agent calls use local OpenSSH/config/ssh-agent, BatchMode,
strict existing known_hosts, no PTY/password prompts, forwarding or automatic
host trust. Verify an unknown server's host fingerprint independently and connect
once with OpenSSH locally. SSH config and known_hosts are snapshotted and hashed;
changes cause `target_changed` rather than silently retargeting an approval.
The first version rejects `ProxyJump` configurations: a flattened single-host
snapshot cannot safely preserve a second host's settings. Use a directly
reachable address (including a VPN address) instead.
The original profile and trust remain local; only explicitly enabled resource
display metadata reaches the workspace. Execution uses the SSH account's normal
permissions; it never adds sudo, daemon privileges or remote software.

## Message object

```json
{
  "id": "msg_...", "from": "alice", "to": "bob",
  "thread_id": "msg_...", "reply_to": "msg_...", "after": "msg_...",
  "type": "task|note|reply|event",
  "body": "...",
  "status": "queued|delivered|acked|held|done|failed",
  "attempts": 1, "error": "", "idempotency_key": "", "automation": "aut_...", "workflow": "run_...",
  "result": {"approved": true},
  "lease_until": "...", "created_at": "...", "updated_at": "..."
}
```

`after` is optional and omitted when empty (from v0.19.2). `automation` names the automation that sent the message, omitted otherwise. `workflow` (v0.21+) is the workflow run that sent a task; `result` is the structured result on a reply that has one. `granted_by` (v0.21.2+), on a task, says why the hub let it past its recipient's `allow_from`: `project:<name>` for a spawn request (`myco agent spawn`) to a computer's machine node from a coordinator of an active project that allows that computer (see [Projects](#projects): `machines`); not for new terminals or sealed bodies. The machine node accepts such a request; its own folder and access rules still apply. Adding fields is non-breaking; renaming/removing fields requires `/v2`.

`body` is Markdown; what the App draws beyond plain Markdown (diagrams, pictures in the text, `myco-ui` blocks) is in [rich-messages.md](rich-messages.md).

## Exec nodes

An agent served with harness `exec` runs each **task** body as `sh -c <body>` and replies with
a JSON body:

```json
{"exit_code": 0, "stdout": "...", "stderr": "...", "duration_ms": 12,
 "truncated": false, "timed_out": false, "denied": false, "error": ""}
```

`exit_code` 124 = killed at the node's timeout, 126 = refused (sender not in `allow_from`,
command not in the node's allowlist) with the reason in `error`. Notes, replies and events are
never executed. `myco exec --to <node> --cmd "..."` wraps this and exits with `exit_code`.

Exec runners also accept the node owner's exact `@console:<owner>` identity,
derived from the authenticated `GET /v1/me` response, while retaining a nonempty
`allow_from` list. This includes a workflow computer step sent as that person.
Other members' consoles, including administrators, receive no exception. The
runner caches this owner and sender list together for up to 30 seconds; ownership
changes take effect on refresh. If refresh fails, only the original startup
`allow_from` is used, without an owner-console exception. Missing `owner` on older
Hubs and empty sender lists do not grant owner-console access. This identity is
never persisted in `allow_from`; command restrictions and the audit `from` remain
unchanged. Upgrade the node's runner to enable this behavior.

An exec node's answers (the result reply, or the failure event) are stored already `done`: they
matter only to whoever waits for them (`GET /v1/messages/{id}/reply` still returns them), so a
caller that gave up waiting does not find a stale result in its inbox.



## Members and permissions

"admin" in the tables above means the hub's admin token or a console token. Each console token belongs to a member (a person); the admin token acts as the owner. Members are local to the hub (no global accounts):

| Role | May |
|---|---|
| owner (one) | everything, including making someone else owner |
| admin | invite and remove people (not the owner, not other admins unless owner), manage every node, machine, link and alias, read the audit log |
| member | see every node and conversation, message any node, create join links (what joins is theirs), and manage only their own nodes and machines (edit, labels, allow-from, disconnect, delete, rename, start sessions) |

Agents, machines and links record their `owner`: what joins through a member's link is theirs; a machine's machine enrollment inherits the link's owner. Messages carry `from_owner` (the member behind the sender) and messages sent from the app are signed `@console:<member name>`; the runner tells the agent whether a message comes from its owner, a colleague, another agent of its owner, or someone else's agent. Hubs upgraded from before members get one owner (`owner`, rename it) who owns everything already there.

## End-to-end encryption

When the hub's owner turns it on (`PUT /v1/hub/settings {"e2e": true}`), clients seal message bodies:
`body` becomes `myco-e2e:v1:<base64url JSON envelope>` and messages carry `"e2e": true`. The hub
stores and relays envelopes without reading them; a sealed failure reason becomes the event body as
it is. Attachments are uploaded encrypted (named `encrypted`); their keys travel inside the
envelope.

## App alias proxy

The local App backend uses the selected Hub's console token. `GET /app/hubs/{id}/aliases` returns `{aliases:[{alias,target,created_at,updated_at}]}`. `PUT /app/hubs/{id}/aliases/{alias}` accepts `{target}` and returns `{alias:{...},dry_run}`. `DELETE /app/hubs/{id}/aliases/{alias}` returns `{alias:{...},removed,dry_run}`. PUT and DELETE support `?dry_run=true`; errors preserve the Hub's structured code and HTTP status.

`DELETE /app/hubs/{id}/machines/{mid}?dry_run=true` proxies machine removal and returns `{machine, removed, dry_run}` with the Hub's status and structured errors.

`PUT /app/hubs/{id}/agents/{name}/run-settings` forwards to the Hub's run settings route (above). `POST /app/hubs/{id}/agents/{name}/resume` makes (or rotates) that agent's resume line and returns `{url, command}`; `DELETE /app/hubs/{id}/agents/{name}/resume` revokes it. Both forward to the Hub routes above and keep its structured errors; a Hub before v0.19.5 answers 404, and the App then shows the values without controls.

### Read-only CLI tool inventory (from `codex/mcp-skills`)

`GET /app/hubs/{id}/machines/{mid}/info` returns `ais[]`; each Claude Code or Codex entry can include user-level `mcp`, `skills`, `plugins`, and `tool_warnings`. An agent's `runtime` includes the same fields for tools found in its work directory (project scope). Older machines and runners omit them. `mcp` entries contain `{name, transport, host?, scope, enabled?}`, `skills` contain `{name, description?, scope, managed_by?}`, and `plugins` contain `{name, marketplace?, enabled}`. `scope` is `user`, `project`, or `plugin` for MCP and `user` or `project` for skills. `tool_warnings` names files that could not be read, without file contents. The scanner never reports environment variables, HTTP headers, command arguments, URL paths or queries, or secret values. It reads CLI configuration only; it does not connect to or start MCP servers.

### Localized template display metadata

Template list/get objects can include optional `locales`, a map from language
tags to `{name?, description?}`. It comes from a template's optional
`locales.json` sidecar, not a new manifest field or database column. Values are
for display only: the base `name`/`description`, page names and record contents
stay unchanged. Select an exact language tag, then its base language, then the
base metadata. Publishing rejects malformed or oversized sidecars (32 KiB,
20 languages; name 80 characters, description 300). Old clients may ignore the
field, and old Hubs can still install apps containing the sidecar.

### Optional Remote Desktop signaling

Remote Desktop is an **Apps** application. New endpoints require a console/member
or Hub administrator credential; agent tokens are rejected. Every operation is
limited to a computer the actor owns/manages. The host must independently opt in
and validate a temporary pairing code; Hub authorization alone does not start
capture. Existing clients remain unchanged. No database migration is needed:
host registrations and signaling are in memory and disappear on Hub restart.

- `GET /v1/remote/machines` → `{machines:[{machine,name,os,control}]}`: enabled,
  authorized hosts whose last heartbeat was at most 30 seconds ago.
  `machine` may be an existing machine ID or `app:<console-device-id>`. App-only
  hosting creates no agent, machine record or runner service. Registering such an
  ID requires that exact App device credential (or the Hub master administrator);
  other operations use its registered member ownership plus the host capability.
- `PUT /v1/remote/machines/{machine}` body `{name,os,control}` → `{host_key}`.
  This replaces the previous registration and closes its negotiations.
- `GET /v1/remote/machines/{machine}/requests` → `{sessions:[...]}`: host heartbeat
  and pending offers / active or closed negotiations. Requires the registration's
  `X-Myco-Remote-Host` capability in addition to ordinary authentication.
- `DELETE /v1/remote/machines/{machine}`: stop advertising and close negotiations;
  requires the host capability. Returns 204.
- `POST /v1/remote/sessions` body `{machine,offer:{description:{type:"offer",sdp},
  pairing_code,control}}` → `{id,machine,state:"pending"}`. Only one pending/active
  negotiation per host. Controller responses never contain the pairing code.
- `PUT /v1/remote/machines/{machine}/sessions/{session}` body
  `{answer:{type:"answer",sdp}}` or `{error:"stable_error_code"}`: the host's result,
  requiring its capability. Returns 204 and discards the stored offer/code.
- `GET /v1/remote/sessions/{session}` → `{id,machine,state,answer?,error?}`:
  only its creator's member identity may poll it; polling renews its 90-second
  signaling lease. `state` is pending / connected / failed / closed. Connected
  means an answer was supplied, not that media has arrived; use peer state for UI.
- `DELETE /v1/remote/sessions/{session}`: cancel/stop the creator's session.

The iOS App uses these same controller endpoints; no phone-specific Hub
endpoint or schema change is required. Its native page bridge owns the session
handles and revokes them when the page closes or the App backgrounds. Text input
capabilities are negotiated on the encrypted peer data channel, not through the
Hub.

The Hub carries SDP only, never pixels, audio or keyboard/mouse events. SDP and
pairing codes are sensitive, short-lived signaling: never put them in page data,
logs, traces or audit details. Audit actions are `remote.enable`, `remote.disable`,
`remote.connect`, `remote.disconnect`. The Hub is trusted for signaling; this is
not independent peer identity verification against an untrusted Hub.

The local App proxies `POST /app/hubs/{id}/remote` with `{page,instance,action,args}`.
The trusted App assigns a fresh `instance` on each page load. A closed instance
is revoked, including requests which arrive out of order after `close`.
Actions: status, devices, install, remove, enable (`{control}`), disable, connect
(`{machine,offer}`), session / disconnect (`{id}`), close. Session handles and
host pairing codes are scoped to their originating page. Closing a page also
invalidates in-flight connection operations, preventing a late response from
starting unattended sharing. `close` releases that page's host/session resources.

The local App OS bridge additionally exposes the job `screen.record`; it is not
a Hub recording/streaming endpoint. Inputs are `{interactive?, microphone?,
clicks?, max_seconds?}`; the completed value is `{path, bytes}`. The path is local
only. Stop through job input `stop` to finalize the movie. Capability availability
and platform restrictions are documented in [app-os.md](app-os.md). Existing
`myco.os.mediaURL` supports MOV/MP4/WebM recordings with its directory confinement.

### Embedded iOS app networking

The iOS JavaScript page bridge advertises `net` and implements
`myco.net.fetch(args)`, answered natively as `net.fetch`. This is independent of
the desktop `os` feature and does not add a Hub HTTP endpoint. Requests and
responses use the existing desktop `net.fetch` shape documented in
[App OS capabilities](app-os.md#network-access-on-iphone). Old iOS clients lack
this feature; embedded apps must show an update hint instead of assuming that
all platforms provide desktop computer access.

### Local CLI readiness

`GET /app/local/agents` retains its existing `missing`, `signed_out`, `ready`
and `unknown` state values. `unknown` means installed with opaque sign-in;
it is not a signed-out result. CodeBuddy/Qoder local credentials and a matching
active local profile's successful CLI task within24 hours can establish
readiness. No model requests or credential values enter this response. See
[Local CLI sign-in detection](cli-auth-detection.md) for paths and limitations.

### Local model-provider selection

`GET/PUT /app/local/agents/{name}/provider?hub_id={id}` is an App-local
endpoint, not a Hub route. Only agents with a profile on this computer can be
changed. Existing `{"provider":"cc-switch:<id>"}` requests still pin a cc-switch
entry; `{"provider":""}` clears the pin and provider variables from its private
env file, retaining unrelated variables. Changes apply on the next run.

A custom provider is set with
`{"manual":{"base_url":"http://127.0.0.1:3425/v1","api_key":"<local-only>","model":"my-model","wire_api":"responses"}}`.
`wire_api` is Codex-only (`chat` or `responses`, default `responses`); omit it for
Claude Code. The URL must be HTTP(S) without user information, query or fragment.
The key is required on each update and is written only to a local 0600 env file.
It never enters argv, Hub state or responses. GET/PUT return
`{local, provider, harness?, env_host?, manual?: {base_url, model?, wire_api?, has_key}}`;
`provider: "env_file"` denotes the custom env settings (compatible with older Apps).
`manual` never includes `api_key`. No remote editing of another computer's secret
files is provided by this route.

`runtime.provider_host` optionally reports only the effective endpoint's host
and port, without path, credentials, query or fragment. Following this computer
observes Claude user settings and inherited variables, or Codex's active
`model_provider` in its user TOML configuration. `provider` identifies a detected
magpie loopback gateway (`127.0.0.1:3425`, `localhost:3425`, `[::1]:3425`), a matching
current cc-switch provider, or the host itself. These fields are observations of
user-level configuration, not an assertion about project/profile overrides or
service availability. Older runners omit `provider_host`; if an older Hub specifically rejects this
unknown field, the client retries once without it so other runtime reporting
continues to work. The Hub stores this field with runtime JSON; no schema migration
is needed.

### Remote provider settings

A runner supporting remote provider changes advertises the `remote_provider`
feature and `runtime.providers: [{ref, name, host?, model?}]`. This inventory is
from that computer's cc-switch configuration and never contains keys or config
arguments. A supported runner sends an empty array when it has no channels;
older runners omit it. Against a Hub that rejects these new runtime fields, the
client omits them and removes `remote_provider`, so Apps keep remote controls
read-only until both sides support them.

`PUT /v1/agents/{name}/run-settings` and its App proxy accept an optional
`provider`: `""` (follow that computer), `"cc-switch:<id>"` (an advertised channel),
`"clear"` (clear custom provider variables and follow), or `"custom"` with a
`provider_encrypted` envelope. Omitted provider fields preserve local settings
and older clients. Only the agent's human owner can change provider settings;
another administrator, project coordinator, the agent itself, and a superagent
acting for a person cannot. Omitting model/effort also preserves those values;
explicit empty strings still clear them. Existing local provider routes remain
available independently of remote settings.

**Remote custom keys are encrypted before contacting the Hub, or kept by it.**
The local App backend exposes `PUT /app/hubs/{id}/agents/{name}/provider` with
the same `{manual:{base_url, api_key, model?, wire_api?}}` input as local editing.
With workspace E2E on, it checks that the target agent has registered its public
key, then seals the whole configuration with `internal/e2e` for that agent alone,
and sends the Hub only `{provider:"custom", provider_encrypted:{id,from,body}}`
through run settings. With E2E off (i119) it sends `{provider:"custom",
provider_hub:{id, manual}}` instead: the Hub seals the settings with the
workspace's own key for that agent and request, keeps only the ciphertext in
`run_settings.provider_hub` (shown as `{id, from}` alone), and the agent's runner
fetches them (see Model providers › Without end-to-end encryption). A runner
without the `hub_provider` feature is refused (`unsupported`): enter the settings
on that computer, or update myco there. Never put `manual`, `api_key` or `key` in
an ordinary message.

The envelope uses header `{From:from, To:agentName, Type:"provider.settings",
ReplyTo:id}`, with `from` the authenticated owner's `@console:<member>` name.
`id` is a fresh 16–80 character request identifier. The Hub validates the owner,
E2E setting, device signature, envelope format and sole target recipient without
decrypting it. Neither audit entries nor logs include the ciphertext or key.
A pending `run_settings.provider_encrypted` (or `provider_hub`) means the runner has not acknowledged
installation. The runner opens it locally before its next invocation, validates
and writes a private 0600 env file, then `POST /v1/me/provider/ack {id}` consumes
only that matching pending envelope. A stale receipt returns conflict and cannot
delete a newer request. The `custom` marker remains while the actual settings live
only on the target computer. Private local receipts make lost-ack retries avoid
overwriting local changes with the same delivery again. Clearing or selecting a
channel cancels pending ciphertext. The existing run_settings/runtime JSON stores
are used; no database migration is required.

## Workspace usage

Usage is numeric CLI accounting, independent of trace storage and encryption.
No message content is reported. Older runners/sessions contribute no rows; the
Hub never infers usage from traces. Reports are self-reported, not billing records.

| Route | Who | Contract |
|---|---|---|
| `POST /v1/me/usage` | authenticated agent | `{source?:"runner"\|"session",reports:[{key?,day,model?,project?,message_id?,calls,input_tokens,cache_write_tokens?,cache_read_tokens?,output_tokens,cost_usd?}]}` → `{recorded:n}`. `source` defaults to `runner`; at most 100 reports. `day` is UTC YYYY-MM-DD. Numbers must be finite, nonnegative and at most 1e11 per field per report. An omitted project is taken from `message_id` when provided and visible to the agent. A supplied project takes precedence. |
| `GET /v1/console/usage?from=&to=&project=` | console/admin | `{from,to,rows:[{day,agent,project,model,source,calls,input_tokens,cache_write_tokens,cache_read_tokens,output_tokens,cost_usd}],agents:[{name,display_name,owner,harness,machine}]}`. Inclusive UTC days; defaults to the last seven days ending at `to` (today if omitted), at most 366 days. `project=-` selects unassigned rows. Owners/admins see all rows; ordinary members see only their currently owned nodes and metadata. |

Input tokens exclude cache reads and cache writes in this API. Existing trace
`input_tokens` retains its original total-input semantics. `owner` is the member
name (as in the public agent view) and `machine` is the machine ID; model/machine can be empty when not reported.
Costs are only included when reported by the CLI; zero can also mean unknown.
Claude Code estimates use Anthropic pricing and may not match a compatible proxy
or another provider's actual bill. No price lookup or trace backfill is performed.

Rows accumulate by `(day,agent,project,model,source)` in each workspace database.
A nonempty key is consumed once per agent, atomically with the increment. Retry
keys are retained for 30 days. `recorded` counts newly applied reports, excluding
duplicates. The Go client silently skips an old Hub's 404 without retrying.

The local desktop backend proxies `GET /app/hubs/{id}/usage` with the same query
and response. `myco usage show --from YYYY-MM-DD --to YYYY-MM-DD --project name
--by agent|project|day|model` uses console/admin credentials like `myco audit`;
JSON returns the raw response, text groups and sorts by total token consumption.

New runners advertise runtime feature `usage` and report usage events for every
invocation, including TraceOff and encrypted tasks. `MYCO_NO_USAGE=1` disables
reporting. Claude cache input is split explicitly; missing model/cost information
is left unknown rather than inferred from content.

For non-runner Claude sessions, `myco inbox wait` accounts assistant transcript
usage once per message ID (content-block lines repeat usage). Synthetic models
are ignored. On first read only timestamps in the preceding seven days count.
Complete lines are consumed incrementally; a partial tail waits for the next
invocation. Private 0600 `~/.myco/state/<agent>.usage` checkpoints retain the last
message ID, previous wait's project context, and an immutable pending numeric
batch. Cursor advancement follows successful reporting only; retries reuse keys
even if the transcript has grown. All reporting failures are best effort and do
not fail inbox waiting. Runner-owned subprocesses skip session accounting to
avoid duplication. For multi-workspace sessions, the previous receiving identity
is used once, not once per workspace. Codex rollout-session collection is not yet
supported (runner Codex usage events remain supported).


### Project coordinator invitations

`Project.coordinator_invites` is a boolean (default false; absent on older Hubs).
`PUT /v1/projects/{name}/agent-invites` takes `{ "allowed": true|false }` and
returns the updated Project. Only its human owner and workspace administrators
may change it; agent credentials cannot, including a coordinator's.

A project coordinator calls `POST /v1/projects/{name}/invite-agent` with
`{ "display_name"?: string, "description"?: string, "note"?: string }`.
Limits follow agent profiles (80 bytes for display name, 2000 for description)
and project notes (one line, 200 characters). `?dry_run=true` validates without
creating an invitation or approval. A normal member is refused. Archived projects
and projects without a human owner cannot issue invitations.

Response: `{id, approval_id?, link, sentence, expires_at, status}`. If enabled,
status is `ready` and `link`/`sentence` reveal a credential **once**. Otherwise HTTP
202/status `pending` returns an empty link and creates a `project_invite` typed
approval for the project owner. Its non-secret payload contains invitation_id,
project, project_title, display_name, description and note. Existing console
approval endpoints approve/deny it; ordinary approval notifications and events
apply. Approval does not enable the project setting.

The same coordinator polls `GET /v1/projects/{name}/invite-agent/{id}` for status
and, once `approved`, calls `POST /v1/projects/{name}/invite-agent/{id}/claim` to
receive the link once. Other agents cannot inspect or claim the request. Polling
never reveals credentials. Status values are pending, approved, ready, used,
denied and expired. Pending requests expire after 24 hours; issued links expire
24 hours after issuance. A lost issuance response requires a fresh invitation:
tokens are hashed, never stored in recoverable form. Request/approval IDs can be
stored or included in messages; links cannot be included in audit/events.

The existing `/j/{token}` page, Markdown guide and install scripts handle these
session-only enrollments. They install the CLI and use `MYCO_ENROLL_TOKEN` with
`myco join --invitation --hub ...`, ignoring existing computer enrollment defaults.
They never create a machine membership. `POST /v1/join` refuses these tokens.
`POST /v1/enroll` consumes one exactly once (even same-session reuse is refused),
creates an ordinary agent owned by the **project owner**, and adds it to that
project as `member` with the supplied note in one transaction. The inviter is
recorded in `Agent.invited_by`, and pre-filled display name/description are applied.
No superagent, shell or machine privileges are granted. Coordinator membership,
project owner, archive state, expiry and credential revocation are checked again
on use. Later recovery uses the agent's own resume link, not the invitation.

Audit actions: project.invitation.setting/create/issue/approve/deny/use. The use
entry identifies both the joined agent and inviter. All invitation responses and
join pages disable caching. The App proxies the setting through
`PUT /app/hubs/{id}/projects/{name}/agent-invites` with the selected Hub's console
credentials. The CLI translates an old Hub's HTTP 404 into an upgrade hint.

### Runner admission and queue visibility

`PUT /v1/me/runtime` and agent runtime responses optionally include
`runner_slots: {max_concurrent, limit?, in_use, waiting, at, queue?: [{message_id, position}]}`.
`limit` (runners newer than v0.23.9) is the limit in force; the runner reads it live
from `runner.max_concurrent` in `~/.myco/machine.yaml` or its flag, whichever was
chosen last. After it was lowered, running tasks finish, so `in_use` can be above
`limit` for a while: `max_concurrent` then says `in_use` (older hubs refuse
`in_use` above it). Without `limit`, `max_concurrent` is the limit.
The counts describe the reporting runner **process** across all its workspaces;
`queue` contains only the reporting agent's waiting task IDs, never another
workspace's IDs or address. `at` is the snapshot's RFC3339 time. Counts are
nonnegative, `max_concurrent` is positive, `in_use` and `limit` cannot exceed it, and queue
positions are positive and no greater than `waiting` (at most 256 entries).
Admissions and cancellations update snapshots; a running runner refreshes them
at least every 20 seconds. On admission the task disappears from `queue`.
Apps ignore stopped or older-than-90-second snapshots and ignore queue metadata
on finished tasks. Queue metadata remains visible with execution tracing off;
it contains no task body or execution details. A stopped task is rechecked
before admission starts a CLI.

The runtime JSON is stored with the existing agent runtime; no migration is
needed. Older clients ignore the optional property. Against a strict older Hub,
new clients retry without `runner_slots` **only** for that field's unknown-field
400 error; authentication and other validation errors are never downgraded.
Single-agent runners with no shared limit omit the field. Independent runner
processes are separate pools; one `runner start --all` is required for fairness
across that computer's workspaces.

### Project coordinator scheduling and automation date bounds

Projects expose `coordinator_automations: false` by default. A human project owner
or workspace administrator can change it with
`PUT /v1/projects/{name}/automation-permission` and `{ "allowed": true|false }`.
The App proxies this as `PUT /app/hubs/{id}/projects/{name}/automation-permission`.
The Members tab groups this switch, invitation access and computer access under
**Coordinator permissions**. An absent field means an older Hub.

Coordinator agents use their own agent credential on these routes:

| Method and route | Result |
| --- | --- |
| `GET /v1/projects/{name}/automations` | `{automations:[Automation]}` for the project owner's schedule-only tasks tagged with this project (webhooks and their request payloads are excluded); read access does not require a mutation approval |
| `POST /v1/projects/{name}/automations` | Create a scheduled task, using `AutomationRequest` |
| `PATCH /v1/projects/{name}/automations/{id}` | Change a scheduled task; `{enabled:false}` pauses it |
| `DELETE /v1/projects/{name}/automations/{id}` | Remove a task and its run history |

Only an active project's `coordinator` may call these routes. `project` is fixed by
the URL, `to` resolves to a current ordinary agent member, and workflow targets
must be enabled workflows belonging to the same project and project owner.
Targets are resolved before approval and checked again before dispatch. The
result belongs to the project owner, even if the coordinator belongs to someone
else. A changed/archived project, missing target membership, or a workflow moved
out of scope stops future dispatches with `disabled_reason: "project_scope_changed"`.
The project interface manages schedules only: it cannot create/rotate webhook
secrets or manage webhook-based configurations. Existing console endpoints retain
human credential behavior.

With permission enabled, a mutation returns `200` with
`{status:"applied",automation?:Automation,removed?:id}`. With permission off, it
returns `202 {status:"pending",approval_id}`. Nothing changes until the project
owner approves the `project_automation` card using the existing approval API.
The approval holds that exact request; an owner/target/configuration change
invalidates it. Approval permits only this change and leaves the switch off.
Approval reserves the action durably before executing it, preventing repeated
clicks from creating duplicate tasks. If a process dies during execution the
approval may remain `executing`; inspect the project's task list before asking
again, since that action is never automatically replayed. Approval outcomes name
the task ID in the coordinator's notification. Requests, approval/denial, settings
and applied changes are audited, including the acting agent and approving person.
`Automation.created_by` and `approved_by` provide attribution.

Both console and project automation requests additionally accept:

- `starts_at?: string`: RFC3339 timestamp, inclusive; empty string clears it.
- `ends_at?: string`: RFC3339 timestamp, exclusive; empty string clears it.
- `once?: boolean`: disable before the first execution attempt; skips due to a
  busy preceding task do not consume it. A failed attempt still consumes it.

When both dates are present, start must be earlier than end. Date bounds apply
to schedule, webhook and manual runs; they do not create a trigger themselves.
Date bounds stop future dispatches; they do not cancel tasks or workflows already sent.
Dates are persisted in UTC. `Automation` returns optional timestamp fields,
`once` (always present on supporting Hubs), and `disabled_reason` (`expired`,
`once`, or `project_scope_changed`). The scheduler disables expired tasks even
when they have no next cron run, and skips windows missed entirely during
Hub downtime. An explicit `{enabled:true}` resets the reason and allows a fresh
one-shot attempt, subject to date bounds. Repeated concurrent callbacks reload
the task under the dispatch lock and cannot dispatch a consumed one-shot again.

CLI agents use `myco automation create/update/list/remove --project <name>`.
`--starts/--ends` accept RFC3339 or `YYYY-MM-DD` in UTC; a date-only `--ends`
includes the whole named day. Example:

```sh
myco automation create --project release --name "Daily release check" \
  --to reviewer --body "Check the release status" --cron "0 9 * * *" \
  --tz Asia/Shanghai --starts 2026-11-26 --ends 2026-11-28
myco automation update --project release --id aut_example --once
myco automation update --project release --id aut_example --disabled
```

Without explicit human credentials the CLI uses the agent's own token and
requires `--project`; it never borrows App credentials for a coordinator action.
A missing project route or a strictly rejected new date field gives an upgrade
Hub hint. Older clients can continue using the unchanged console routes and
ignore the new response fields.

## Desktop browser automation bridge

The optional local browser component uses protocol 1 and its own loopback listener; it is not a Hub endpoint. Application manifests may request `permissions.os: [browser]`. Supported desktop hosts advertise the `browser` feature so pages can present the installer; `myco.browser.status()` distinguishes supported and installed states. The bridge provides local profiles, recording, linear runs, controls, locator inspection, and image snapshots. Installation and enabling an agent preference require a separate, one-use host confirmation. Agent actions and cross-machine browser providers are outside M0/M1. See [component and bridge contract](browser-component.md#page-bridge-protocol-1).

### Browser screenshot regions

The component's `GET /status` and `myco.browser.status()` advertise optional
`features:["screenshot_clip"]`. Absence means region screenshots are unsupported;
pages hide the region selector and refuse runs containing clip nodes. The host
also checks the same component process before dispatching a run, including child
script snapshots, and returns `unsupported_node` without posting to an old engine.

A `screenshot` node can contain `clip:{x,y,width,height}` in top-level viewport CSS
pixels. Every coordinate must be finite, x/y nonnegative, and width/height positive.
`clip` cannot be null or combined with `target` or `full`. Out-of-viewport regions
are rejected at capture time. The result and the step's screenshot both contain
only the selected pixels, in the private artifact store; there is no full-page
thumbnail fallback. Secret-bearing runs and sensitive pages refuse region capture.
See [component contract](browser-component.md#recording-inside-chrome).

### Local browser profile settings

`myco.browser.profiles.configure?(id, settings)` is an optional protocol-1
page-bridge method. It requires the native owner's one-use confirmation of the
exact profile and settings. Agents and Hub browser actions cannot configure
profiles. The local component receives authenticated
`PATCH /profiles/{id}` with `{"settings":settings}` and returns a public profile;
`GET /status` profiles include `settings` on components supporting this feature.

Settings input is `{proxy, user_agent, lang, args}`. `proxy` is `null` or
`{server,bypass,username,password?}`; `server` accepts only HTTP, HTTPS or SOCKS5
with an explicit host and port. An omitted password preserves the saved secret only if server and username are
unchanged; changing either clears the old secret unless a new password is
explicitly supplied. An empty password clears it, and a null proxy clears all
proxy credentials.
Output replaces `password` with `has_password`. The password is never returned
by a status, configuration response or confirmation. Only the local private
0600 profile index stores it; page data and Hub records do not.

Changes apply on the next browser-session start. No running browser is
restarted automatically. Arguments use the documented allowlist; invalid flags
or settings return `invalid_argument`. See
[network, UA and launch settings](browser-component.md#per-profile-network-and-launch-settings)
for validation and forwarding semantics. These methods do not change component
protocol 1 or broaden any agent grant.

### Local browser recording discovery

Newer desktop hosts optionally expose `myco.browser.record.active()`, returning
`[{id, profile, url, started_at, origin:"browser"|"page"}]` from the current
component session scope. Component `GET /status` includes the same `recordings`
metadata. Browser-started recordings remain discoverable until the first valid
`record.events` call claims them, including recordings already stopped. Known
IDs can be replayed from cursor 0. `record.events` adds `removed:string[]`; apply
these deletions by step ID, continue with the opaque `next` cursor, and finish on
`done:true`. Deleted steps not yet sent are omitted. The browser's isolated panel
can start/stop, delete, and capture text/attribute/HTML extracts; these are local
human controls, not Hub or agent actions. See [recorder contract](browser-component.md#recording-inside-chrome).

## Planned runner restarts

`PUT /v1/me/runtime` accepts optional `runner_instance` (one opaque ID per process),
`away: "updating"`, and `restarting_until` (RFC3339). The Hub caps a restart
lease at 90 seconds. Further reports from the stopping instance preserve the
lease; a healthy report from a new instance clears it. Late goodbyes from an
older instance cannot replace the new process's runtime. These fields live in
existing runtime JSON; no schema migration is needed.

Agent and machine responses include optional `updating: true` while the lease
is active. This is a temporary display state, not readiness: the existing
`state`, `online`, and `can_spawn` contract is unchanged. Apps display Updating
before other presence labels and expire cached leases locally too. A failed
restart therefore returns to ordinary offline status rather than appearing to
update forever. Runners clear the lease in their first runtime report, before
slow CLI version discovery.

Older Hubs may reject unknown fields. On only that specific 400 schema error,
the client retries without the new fields, retaining existing runtime metadata
and `away`; authentication and other validation errors are not retried. Apps
can bound this legacy updating indication using `last_seen_at + 90 seconds`.

The local optional browser component (not a Hub route) may return
`{ "error": "profile_locked", "reason": "leftover_unverified" }` when a profile's
persisted process group cannot be verified. Other safe recovery reasons are
`cleanup_pending` and `cleanup_failed`. The desktop page bridge preserves the
optional `reason` on its rejected Error; older Apps omit it. See
[the browser component contract](browser-component.md#browser-process-groups-and-recovery-unix).

### Page-owned files

Static pages advertise `attachments_version: 1` in their page DTO. Absence means
unsupported: clients must not substitute project-file uploads. Attachments inherit
the page's read permissions and the named collection's write permissions. They do
not appear in project files, project pull, or agents' project context. Deleting a
page removes its attachment metadata and schedules its independent blobs for cleanup;
application upgrades and version rollback leave attachments intact.

| Method | Path | Result |
| --- | --- | --- |
| GET | `/v1/pages/{id}/attachments?limit=100&after=...` | `{attachments_version:1, attachments:[...], usage:{files_used,files_max,bytes_used,bytes_max,max_file_bytes}, next?}` |
| POST | `/v1/pages/{id}/attachments?collection=assets&name=picture.png` | Upload raw body; `Content-Type` and `Idempotency-Key` required; returns attachment metadata |
| GET | `/v1/pages/{id}/attachments/{file_id}` | Attachment metadata |
| GET | `/v1/pages/{id}/attachments/{file_id}/content` | Original bytes; supports HTTP Range, 206 and 416 |
| DELETE | `/v1/pages/{id}/attachments/{file_id}` | 204; refuses while a record references the attachment |
| GET | `/v1/pages/{id}/attachments?name=old-name.png` | Resolve a legacy project-file name within this page |
| POST | `/v1/page-attachments/migrate` | Workspace owner/admin only; body `{project?,dry_run?,limit?}`; migration report |

Metadata contains `file_id`, `page`, `collection`, `name`, `size`, `sha256`,
`content_type`, `by`, `state`, `created_at`, and pending `expires_at`. Uploads count
against quota immediately. Save `{file_id: "paf_..."}` (optionally with the other
metadata) in a declared `file` field to associate the upload with a record. Unlinked
uploads expire after 24 hours. Associated files remain until explicitly removed or
the page is deleted. Remove all record references before removing a file. A repeated
upload key with identical inputs returns the same ID; different inputs conflict.
List limits are 1–1000; `next` is an attachment ID cursor. File responses are marked
as downloads, `nosniff`, and private/no-store.

Defaults per page: 1000 files, 2 GiB total, 20 MiB per file. Hub flags
`--page-files-max` / `MYCO_PAGE_FILES_MAX` and `--page-bytes-max` /
`MYCO_PAGE_BYTES_MAX` configure defaults (at most 10000 files / 20 GiB). The same
Hub disk-reserve settings as project uploads apply. Existing files remain readable
when limits are lowered; migration preserves pre-existing content even over quota.

The Hub automatically migrates project files referenced by declared page `file`
fields or a legacy text field named `file`, only within the page's own project.
It copies and verifies independent page blobs before transactionally saving aliases,
record associations, and removing project entries. Record data and versions do not
change. Shared originals get independent copies per page. Missing, ambiguous, changed,
or unreadable sources stay in place and appear in the migration report. The old
project-file GET route resolves migrated names subject to the current page ACL;
legacy aliases never reappear in project listings. Migration batch size is 1–100
(default 25). Administrators can preview or resume with:

```sh
myco page file migrate --project demo --dry-run
myco page file migrate --project demo
myco page file put --page pg_demo --collection assets --path picture.png
myco page file list --page pg_demo
myco page file get --page pg_demo --file paf_example --out picture.png
myco page file remove --page pg_demo --file paf_example --dry-run
```

### Durable watches

A watch is a durable, bounded wait for a reply, task completion, local command, or workflow
run. Checks do not run a model; command checks execute on the initiating agent's
permitted local runner, never on the Hub. Watch records survive Hub
restarts. Agent profiles and overview entries include `watching`, the number
of records in `watching` state; this is separate from `held` and `queued`.
Older Hubs omit this counter and do not provide the watch endpoints.

- `POST /v1/watches` (agent token): `{what, kind, target, project?, task?, for_sec?}`.
  `kind` is `reply`, `task`, `workflow`, or `command`. For reply/task/workflow,
  `target` is a task message ID or workflow run ID. The caller must be able to read the target. A reply watch targets a
  task the caller sent. When specified, `project` must contain the agent and
  match the target and associated task. `task` is a delivered/acked/held task addressed to
  the caller; starting with `task` atomically holds it and creates the watch.
  At most one active watch may be associated with a task.
  `what` is a single line of 1–200 characters. `for_sec` defaults to 1800
  and is 1–86400. Returns the Watch (201; 200 for a dry run).
- `GET /v1/watches?agent=&project=&state=active|all` returns `{watches: []}`,
  filtered to records the viewer may manage. The default state is `active`
  (`watching`); `all` includes terminal records.
- `GET /v1/watches/{id}` returns one Watch.
- `POST /v1/watches/{id}/stop` takes `{reason?}` (at most 200 characters).
- `POST /v1/watches/{id}/extend` takes `{for_sec}` (1–86400), setting the
  deadline **from now**, within 24 hours of the original start. Ended or
  already overdue watches cannot be extended.

Creation is agent-only. Reading, stopping and extending are allowed to the
initiating agent, its owner, Hub administrators, and the current coordinator
of the watch's project while the watched agent remains a project member.
A coordinator cannot manage unscoped watches or watches in another project.
Stop/extend accept `?dry_run=true`; they validate without persisting or
sending a wake. Successful changes record the actor and time in the audit log.
Concurrent changes can return `conflict`; read the record again before retrying.

A Watch contains `id` (`wat_…`), `agent`, optional `project` and `task`, `what`,
`kind`, `target`, `started_at`, `deadline`, optional `last_check_at`,
`last_result` (a status summary, never the target's content), `checks`, `state`,
and optional `ended_at`, `ended_by`, `outcome`. Dates are RFC3339 timestamps.
States are `watching`, `met`, `expired`, `stopped`; terminal states cannot be
reopened. Command watches also carry `command`, optional `match`, `every_sec`,
immutable execution `machine_id`, `environment`, `workdir`, and a `history` of at most
five `{at, result, matched, exit_code}` summaries. `revision` fences concurrent updates.

#### Command checks

Start with `{what, kind:"command", command, match?, every_sec?, project?, task?, for_sec?}`;
`target` must be absent. `command` is 1–8192 bytes without NUL, `match` is a Go/RE2
regular expression (max 2048 bytes), and `every_sec` defaults to 30 (1–86400).
The initiating agent must have a machine-bound, current runner advertising
`command_watch`, full agent access and a machine allowing full shell access.
Readonly/workspace agents, exec/machine nodes, and unbound or unsupported runners
receive `permission_denied`. This does not grant a sandboxed agent unrestricted shell access.

- `POST /v1/watches/{id}/claim` takes `{runner}` (the runtime runner instance).
  Only the initiating agent can claim a due command check on its original machine,
  environment and directory. The returned Watch contains `claim_id`, `claim_runner`,
  `claim_until`, and `next_check_at`. A live lease or a not-yet-due check returns `conflict`.
- `POST /v1/watches/{id}/checks` takes `{check_id, runner, result, matched, exit_code}`.
  A report must own the current lease; a repeated check ID is idempotent. For watches
  without `match`, exit code 0 determines success; with `match`, the runner evaluates
  the regex against bounded stdout/stderr. Timeout never counts as a match. Reports
  received after the deadline cannot meet the condition. Terminal watches stay terminal.

The runner executes without a model in the saved working directory, with a minimal
shell environment excluding myco credentials. Timeout is half the interval, capped
at 60 seconds; process-tree cleanup may take up to another seven seconds. Captured
stdout and stderr are capped at 32 KiB each; regex evaluation uses this bounded output
with surrounding whitespace removed. The latest printable line is reported, even
when captured output was truncated. Both runner and Hub redact credential-shaped
strings before retaining at most 1024 bytes of that line. History holds only the most
recent five summaries, never full output. Use existing CLI credential stores rather
than literal secrets in `command` or `what`.

A runner restart resumes the saved schedule. An abandoned lease can be claimed again;
a crash during a process can therefore cause that check to run again: use read-only,
idempotent commands. Only the final state and wake are committed once. An old or
absent runner never prevents Hub expiration. Stop/deadline/policy cancellation ends
an in-flight process when observed by the runner; it cannot undo command side effects.

The Hub evaluates conditions in the background. `task` means done or failed;
`reply` means a reply/failure event exists; `workflow` means succeeded, failed,
or cancelled. When evaluation happens after a restart, a target that finished
before the deadline still matches; later completion expires the watch.
Completion, expiration, or stop changes the watch and restores
its associated held task atomically. The wake explanation is provided through
existing `hold_wake`, rather than dispatching a second session. With no held
task, the agent receives a normal event. Expiration explicitly asks the agent
to decide what to do next. Stopping also resumes an associated held task with
the reason. An already completed associated task stops its watch without a
new wake. Automatic hold watches retain legacy cross-project hold semantics;
their label does not copy a delegated task's content across project boundaries.

### Local browser profile deletion

`myco.browser.profiles.remove?(id)` accepts `BrowserProfileRemoveInput` (`{id}`)
at the local browser-call endpoint and returns `true`. It is optional and
requires both the page's browser consent and a one-use, exact-profile owner
confirmation from the trusted App UI. Agents cannot request this confirmation.
The component advertises `profile_delete` and receives authenticated
`DELETE /profiles/{id}`. Older components return `unsupported` through the App.
Deletion removes that profile's local browser data, local downloads and proxy
credentials, and the App removes its local script approvals in every workspace.
A browser window, active recording/run/control, startup reservation or live
process lease makes deletion return `busy`; it never stops a browser to delete it.
Native dedicated agent profile identity is reused across workspaces. Each
workspace/account still has independent enablement, bindings and approvals.

`browser.control.open` can return `busy` with `reason: "owner_window_open"`:
ask the owner to close this profile's browser window first. `profile_in_use`
and `profile_starting` identify other live controllers and startup reservations.
The public bridge preserves these reasons without exposing process paths.

Deleting the last dedicated agent profile also disables its automatic recreation
on this App configuration. Create a new personal profile and explicitly allow
agent access if you want to use browser actions again.

## Goals: bounded work on an objective

Goals are plaintext objectives set by a person or project coordinator for an agent.
Only its owner, creator, Hub administrators, or its current project coordinator
may create/manage a goal; the target agent may read it and report its own turns,
but cannot grant itself more time, resume, or change the objective. An unrelated
agent and another agent sharing the target's owner do not inherit human authority.
Shell/exec and machine nodes cannot be goal targets.
One active or paused goal is allowed per agent. The objective and success criteria
are immutable. Existing Hubs return 404; existing runners need no new protocol.

| Method | Path | Body / response |
|---|---|---|
| POST | `/v1/goals` | `{agent,objective,done_when,project?,thread?,every_sec?,max_rounds?,for_sec?}` → Goal |
| GET | `/v1/goals?agent=&project=&state=active\|all` | `{goals: Goal[]}`; active includes paused goals |
| GET | `/v1/goals/{id}` | `{goal: Goal, rounds: GoalRound[]}` |
| POST | `/v1/goals/{id}/pause`, `/resume`, `/stop` | `{reason?}` → Goal |
| PATCH | `/v1/goals/{id}` | `{every_sec?,max_rounds?,for_sec?}` → Goal |
| POST | `/v1/goals/{id}/rounds` | `{message,summary,progressed,next_step?,blocked?,achieved?,abandoned?,reason?}` → Goal |

Mutations support `?dry_run=true` without persistence, wakes, or audit side effects.
Defaults: 1800 seconds, 16 turns, 28800 seconds. Base intervals are 900/1800/3600
seconds; turn limits are 1–256. Deadlines, including patches calculated from now,
must remain within eight hours of the original start. Resume preserves the deadline,
round count, and no-progress count. Paused goals still expire. `round_limit` ends
in state `stopped`. Other states are `active`, `paused`, `achieved`, `abandoned`,
`expired`, and `stopped`. All timestamps are RFC3339.

Goal fields are `id` (`gol_...`), `agent`, `project?`, `thread`, `objective`,
`done_when`, `every_sec`, `base_every_sec`, `max_rounds`, `deadline`, `round`,
`no_progress`, `last_summary?`, `next_step?`, `next_wake_at?`, `state`, `started_at`,
`ended_at?`, `ended_by?`, `outcome?`, and `created_by`. A round contains `goal`,
`round`, `started_at`, `ended_at?`, `summary`, `next_step`, `progressed`, `blocked`,
and its wake-task `message`. Dispatch increments `round`. A report's `ended_at`
marks report completion; scheduling additionally waits for its task to finish.

Reports require the current acknowledged round's `message` and explicit boolean
`progressed`. A duplicate identical report is idempotent; conflicting or stale
reports are rejected. Achieved/abandoned require a reason. Two consecutive turns
without progress double the interval, subsequent unsuccessful turns double it
again (maximum four hours); four unsuccessful turns or `blocked` pause and notify
the creator. Progress resets the interval. A terminal task without a report counts
as no progress. No next turn is dispatched while its predecessor is open.

Task creation, round state, pause/terminal notification are committed atomically.
Pausing/ending removes a wake which is only queued or delivered; an already
acknowledged/held task is not interrupted. Any later report cannot revive an ended
goal. Human notifications return to their console thread; agent coordinators get
a note in the goal thread. Goal creation/control/report operations are audited.
Agent lists and overview include optional `goal: {id,objective,round,max_rounds,
next_wake_at?,state}`; objective is truncated to 200 characters. This is independent
of `watching`, `held`, and `queued`.

### Live browser grant monitoring

The desktop monitors the session's grant by ID through the owner's grant list,
independently of host re-registration, host generations and completed job retention.
An explicit terminal grant stops the session immediately. A transient HTTP failure,
missing grant or malformed response starts a bounded 30-second verification grace
period; a confirmed active grant resets it. Continuous failures end the native
session with `reason:"grant_check_failed"`, which does not claim revocation. The
session's local grant deadline remains enforced throughout this grace period.
Expiry and confirmed terminal grants retain `grant_expired` and `grant_ended`
respectively.

### Private account quota snapshots

`PUT /v1/me/runtime` accepts an optional extended `quota`. Agent list, agent
get and overview responses resolve the agent's current account binding under
`runtime.quota`; clients do not need an additional request. Existing `source`,
`at` and `windows` remain supported. New optional fields are:

| Field | Meaning |
| --- | --- |
| `provider` | `codex`, `claude`, `opencode_go`, `minimax`, or `unknown` |
| `profile_ref` | Opaque machine-local login reference, not a path or account identifier |
| `machine_id` | Assigned by the Hub from the authenticated agent, never trusted from input |
| `plan` | `chatgpt_subscription`, `claude_subscription`, `go`, or `coding_plan` |
| `source_kind` | `native`, `event`, `cache`, `official_api`, `cloud_session`, or `statusline` |
| `status` | `ok`, `unsupported`, `no_credentials`, `auth_required`, `throttled`, `transient_error`, or `schema_changed`. Readers may also see `unknown`, which is never sent by a client: it means the last `ok` observation is over an hour old or none of its windows still applies (see below) |
| `checked_at` | Time of the latest actual acquisition attempt |
| `error_code` | Safe constant reason; never a raw HTTP body, token, or process error |

Windows add optional `duration_sec` and `model_scope`. `used` remains the share
used, in `[0,1]`; remaining is `1-used`. A missing window or reset is unknown,
not a full allowance. In particular, `primary` is not necessarily a five-hour
window. When native events update individual windows, `at` is the oldest included
observation, so an untouched weekly window is never made fresh by a five-hour
update. Missing numeric measurements are omitted, never reported as zero used.
An unsuccessful check
preserves the previous numbers and their original time. No successful sample
means an empty `windows` array and zero `at`; do not display numeric remaining
quota in that case. Show the original data time when a sample is old (for
example, over 15 minutes) or a subsequent check failed.

Responses never present an old `ok` as current use. A window whose `resets_at`
has passed is dropped (the numbers describe a window that no longer exists),
and when no window is left the status is `unknown`. A snapshot whose observation
is over an hour old is `unknown` too. Freshness uses `at` for a session report
(`profile_ref` empty, e.g. `cloud_session` or `statusline`) and `checked_at` for
a shared account snapshot (`profile_ref` set): a runner's `at` may deliberately
be the oldest included window's time. Clients should not offer a picker, a
warning or a "quota left" number for `unknown`.

The Hub stores one snapshot per owner, machine, provider and login reference.
Each agent's runtime stores its current reference. New bindings do not inherit
old profile values, and a pending association does not overwrite an existing
successful shared sample. Snapshot updates and binding updates are transactional.
The readers are the computer owner's console and, from the next version, any
agent that same member owns (for example the coordinator that plans how the
owner's quota is spent), while the agent whose quota it is still belongs to
that owner. Runtime write responses apply the same filtering as reads.
Transferring an agent does not transfer its former computer owner's account data.
Agents owned by another member (or by nobody), project coordinators that the
owner does not own, and administrative member roles do not inherit account
visibility. The explicitly configured Hub master credential retains
recovery access. Unauthorized responses omit the entire `quota`, including its
reference. Legacy runner quotas receive the same private response filtering.

New runners connecting to older Hubs with strict schemas omit the extended quota
on a schema rejection, rather than downgrade private values to legacy public
runtime fields. Other runtime fields continue to report normally.

## Provider model lists

`PUT /v1/me/runtime` accepts optional `models_from` and `models_at` next to
`models`. `models_from` says where `models` came from: `provider` (the vendor
of the API key the agent uses listed them, at `models_at`), `catalog` (myco's
built-in list for that vendor: it could not be asked yet; no `models_at`) or
`cli` (the CLI's own list, e.g. Codex's models cache). The Hub keeps only those
three values, and only with models; `models_at` in the future is dropped. Agent
responses carry both in `runtime`. Older runners send neither and older Hubs drop
them; `models` keeps its meaning. A runtime keeps at most 100 models (was 40).

Runners from this version list an API-key vendor's models on their own computer:
Claude Code from `ANTHROPIC_BASE_URL` with `ANTHROPIC_AUTH_TOKEN`/`ANTHROPIC_API_KEY`
(the environment, then `settings.json`, then a pinned provider), Codex from the
active `model_providers` entry (a pinned provider's `-c` options, then
`config.toml`). They `GET <base>/v1/models` (or `<base>/models` for a versioned
base), then the vendor's other interface on the same host when it is in the
built-in catalogue (OpenCode Go, DeepSeek, Kimi, MiniMax, Zhipu, Qwen, Doubao,
OpenRouter). The key goes only to that host (redirects elsewhere are refused);
errors name the host and status only. Lists are cached for 45 minutes in
`~/.myco/state/models/` without keys. Anthropic, ChatGPT and cloud-platform
logins are not asked.

`PUT /v1/agents/{name}/run-settings` (and the App proxy, as a patch) accepts an
optional `models_refresh`: a new value (the App sends the time, at most 40
characters, no line breaks) asks the runner to list the provider's models again;
it is stored in `run_settings.models_refresh` and nothing else changes. Runners
that read it report `runtime.features: ["models_refresh"]`; they read it within
a minute while their agent uses another vendor. Older Hubs ignore the field.

## Model providers (i115, i119)

A member's model providers: a vendor (a catalogue id from
`internal/provider` — `opencode_go`, `deepseek`, `kimi`, `minimax`, `zhipu`,
`qwen`, `doubao`, `openrouter` — or `custom`), its two base URLs
(`anthropic_base` for Claude Code, `openai_base` for Codex; at least one), an
optional `small_model`, the models last read with the key (`models`,
`models_at`), and the API key with `key_version`. Every computer the member adds
an agent on can use them. Who keeps the key is the provider's `custody`, set by
the workspace's end-to-end encryption: on, `"e2e"`, the key is an envelope only
the member's devices open (`key_sealed`); off, `"hub"`, the hub keeps it,
encrypted with the workspace's own key, and its administrator could read it
(see Without end-to-end encryption, below). Whether encryption is on is the
member's call; providers work either way.

| Route | Who | |
|---|---|---|
| `GET /v1/console/providers` | a member's device | `{e2e, providers: [...]}`: the caller's own providers only |
| `POST /v1/console/providers` | a member's device | `{id, name, vendor, anthropic_base?, openai_base?, small_model?, key_sealed, key_version: 1}` (encryption on) or `{id, name, vendor, …, api_key}` (off) → the provider. The client picks `id` (`prv_` and 8 to 40 lowercase letters or digits): the key is sealed for it |
| `PATCH /v1/console/providers/{id}` | its member | Only what is set changes. A new `key_sealed` comes with `key_version`: the current one plus one for a new key, or the current one when the same key is sealed again for the member's devices now (rewrap); anything else is `conflict` |
| `DELETE /v1/console/providers/{id}` | its member | Agents it was sent to keep what was written on their computers |
| `GET /v1/console/providers/{id}/key` | its member | `"hub"` custody only: `{api_key, key_version}`, for its owner's App to seal it for their devices or to list the vendor's models. Recorded, rate limited, `Cache-Control: no-store` |

**The key.** `key_sealed` is an e2e envelope (`e2e.Seal`) with header
`e2e.ProviderKeyHeader(member id, provider id, key_version)`; its payload is
`{"api_key", "vendor", "anthropic_base", "openai_base", "small_model"}`: the key
and everything that says where it is sent. The App uses only what it opens; the
plain fields on the hub are for showing, and the App says so when they differ
(`settings_changed`). A `PATCH` that changes the vendor, an address or the small
model must bring `key_sealed` at the next `key_version` (sealed again with them);
the name alone may change without it. An envelope from before the addresses were
sealed is not used until the provider is saved again with its key. The hub keeps it only when it is signed by one of the
member's device keys and readable by those devices alone
(`e2e.VerifySealedFor`); it cannot open it, and it never logs it (the audit log
says `provider.create`, `provider.key key_version=N`, `provider.delete`). A
`key_sealed` needs the workspace's end-to-end encryption on, and an `api_key`
needs it off (a provider names its `custody` in a request only to match). Agents, superagents and other members get `permission_denied`
or `not_found`, and so do the hub's administrator token and device tokens
that belong to no member: only a member's own devices manage their providers.

**A new device.** A device added after a key was sealed cannot open it. Any of
the member's devices that can (the App, at its next look at the list) sees that
the envelope's readers are not the member's devices now, opens it and seals it
again for them (`PATCH` with the same `key_version`). A removed device is left
out the same way. If none of the devices that could open it is left, the key is
entered again.

**To an agent.** The App opens the key and seals the custom provider for the
agent as before (`provider: "custom"` with `provider_encrypted`, see Remote
provider settings) and records `run_settings.provider_ref: {id, key_version}`
(only the agent's owner, and only to a provider of theirs; runners ignore it).
After a key changes, the App offers to send it again to every agent whose
`provider_ref.key_version` is older. The custom provider may carry
`small_model` (Claude Code's haiku tier and background work); older runners
ignore it.

**A new agent.** `POST /v1/machines/{id}/spawn` takes `provider_encrypted`
(sealed for that machine's node, by one of the person's devices; checked like an
agent's) and `provider_ref`, for `claude` and `codex` sessions only, from that
computer's owner only (as an agent's provider; not an administrator). A machine
node with the `sealed_spawn` feature opens it, runs the session's first turn
with it and writes it to the new agent's env file (with the machine node's own
variables besides, mode 0600); the hub then records the new agent's provider as
`custom` with that `provider_ref`, on the agent the machine names only when it
joined since the request, with the session the machine reported, and is the
computer owner's. A machine node takes a provider only when a person's device
sealed it (`@console:<member>`), that member owns the computer, and it was sent
by them or by the hub for them (`@admin`, after the checks above): one an agent
sealed, or sent on a person's behalf, is refused. A runner takes a custom
provider only when its agent's owner sealed it. `GET /v1/machines` names each machine's node
(`node`) for this. An older machine node (no `sealed_spawn`, or no key
registered yet) is sent the request without a provider; the App sends the
provider to the new agent once its runner has registered its key, before
anything is sent to it.

### Without end-to-end encryption: the hub keeps the key (i119)

**The workspace's key.** Each workspace (each `hub.db`) has its own random
32-byte key in `provider-secret.key` next to its database, mode 0600, made the
first time a key is to be kept (written whole: a temporary file, then a link).
The provider's key is sealed with it (AES-256-GCM, random nonce), bound to the
provider, its owner and `key_version`, as `key_hub` in the provider's row; the
response carries only `custody: "hub"` and `key_hint` (the last four
characters), never the key. A copy of the database alone does not open it. A
backup (`myco hub backup`, the App's, a workspace's) carries the key file with
the database, and a restore puts it back (0600; the one that was there is kept
aside with the rest): **keep backups as secrets**. If the key file is gone while
keys sealed with it are kept, the hub logs a warning when it starts, providers
say `key_lost: true`, and nothing makes a new key quietly (that would hide that
they cannot be opened): the owner entering a lost provider's key again does.

**Settings for one runner or node.** The key leaves the hub only through
`GET /v1/me/provider-material`, to:

- an agent's runner, `?id=<request>`, for the `run_settings.provider_hub` its
  owner set (`PUT /v1/agents/{name}/run-settings` with `provider: "custom"`
  and `provider_hub: {id, ref: {id, key_version}, model}` for one of their
  providers, or `{id, manual}` for settings of this agent alone; the owner
  only, at one of their devices, encryption off, to a runner with the
  `hub_provider` feature). The settings show only `provider_hub: {id, from}`;
  the runner checks `from` is its owner's, writes the env file (0600) and
  acknowledges with `POST /v1/me/provider/ack {id}`, which clears it;
- a machine node starting a session: `POST /v1/machines/{id}/spawn` with
  `provider_ref` and `provider_hub: true` (that computer's owner only, `claude`
  or `codex`, a node with the `hub_provider` feature) makes a grant the node
  is sent instead (`provider_grant`): for that node and that request only,
  used once, gone when the request ends and after ten minutes at most. The node
  fetches `?grant=<grant>`, takes it only when `from` is the computer's owner,
  runs the first turn with it and writes the new agent's env file; the hub then
  records the agent's provider as for a sealed one.

Every answer is recorded (`provider.material`, `provider.key.read`: who, which
provider, which version, never the key), refusals too. Each caller is held to
six a minute for reads by request id, owners' key reads and refusals; a grant
is its own limit (one spawn, once), so a computer may start many agents in a
minute. A node checks that a spawn request came from the hub before it uses
the request's grant. Nothing is cached or logged.

A new vendor, address or small model is a new `key_version` with the hub
keeping the key as well (the key is sealed again for it), so agents with the
old settings are offered the new ones; a new name is not. `key_hint` is
empty for a key shorter than twelve characters. Other members, other agents, the
administrator token and device tokens of no member get nothing.

**Turning encryption on or off.** On: the owner's App, at its next look at the
list, reads each key the hub keeps, seals it for their devices and saves it
(`PATCH` with `custody: "e2e"`, `key_sealed` at the same `key_version`); the hub
drops its copy. Off: a provider sealed for the member's devices says
`move_to_hub` and is not used until the owner agrees in the App, which opens it
and hands it to the hub (`PATCH` with `custody: "hub"`, `api_key`, the same
version). Agents already set keep what is on their computers either way; the
next key pushed goes the new way.

**Not shared.** A provider is its member's only; there is no workspace-wide
shared provider. A key sealed for every member's devices cannot be taken back
when one of them leaves (only changing the key at the vendor does that), whose
usage and quota it is would be unclear, and one person adding agents on their
computers needs only their own. Sharing (for a project, say) would be a change
of its own.

## Local App preferences

These routes belong to the local desktop App backend, not the workspace Hub.
They operate on the App's existing local profile and require no new Hub protocol.
Browser-only consoles / older backends may return 404 or 405; clients retain their
browser storage path in that case. See [Local App preferences](local-preferences.md)
for the migration and storage audit.

| Route | Request and response |
| --- | --- |
| `GET /app/preferences` | `{version: 1, values: {key: string \| null}}`; `Cache-Control: no-store` |
| `PATCH /app/preferences` | `{values: {key: string \| null}}`; updates only the supplied allowlisted keys and returns the complete document |
| `POST /app/preferences/import` | Same shape; imports only absent keys. Existing values and null deletion tombstones win |
| `PATCH /app/preferences/hubs/{id}` | `{name: string, look: {color?: 0..7, pic?: string}}`; atomically merges one workspace's alias/appearance without replacing other workspaces |
| `POST /app/preferences/images` | Raw PNG/JPEG/GIF/WebP bytes, at most 8 MiB; returns `{url: "/app/preferences/images/<sha256>.<ext>"}` |
| `GET /app/preferences/images/{name}` | Private immutable image bytes; only content-addressed names are accepted |

Metadata requests/files are bounded to 24 MiB. Workspace pictures supplied as
legacy `data:image/...;base64,...` are extracted into local image files before
metadata is committed; external or invalid picture references are rejected.
Preference errors are `{error: "local_preferences", message, retryable}`. Invalid
requests return 400, oversized metadata/images 413, a competing writer 503 and
profile I/O failures 500. Failed saves preserve previously committed metadata.
No local filesystem paths, image payloads or preference values are logged.

### Local App interface language

`GET /app/ui-lang` returns `{lang}` (empty before the web interface has selected a
language). `PUT /app/ui-lang` accepts `{lang}` with `zh`, `zh-Hant`, `en`, `ja`, `ko`,
`es` or `pt-BR`, saves the resolved language, and updates native menu/notification
labels. Unknown IDs return `400 invalid_argument`. The `myco-lang` local preference
stores the person's choice, including `system`; the web interface resolves System
before calling this route. No Hub/runner protocol changes are needed. Older desktop
backends can reject newer IDs without preventing the web interface from rendering.

The existing optional `POST /app/translate` accepts `{text, to}` with the same seven
resolved IDs and returns `{text, provider}`. It requires the configured translation
service's key; interface localization itself never calls it. See [Interface languages](i18n.md).

### Runtime account identity

`Runtime.account` is an optional email identity hint reported by newer runners
and `myco join` / session reporting. Claude cloud sessions use
`CLAUDE_CODE_USER_EMAIL`, falling back to `~/.claude.json`'s
`oauthAccount.emailAddress`; local Claude sessions use that file. Codex uses the
`email` claim in `tokens.id_token` in `auth.json` under `CODEX_HOME` (default
`~/.codex`). Only the JWT payload is decoded; it is not verified and never grants
authority. Tokens are neither transmitted nor logged. Missing/unreadable files,
malformed data and API-key-only configurations report no account.

The transport returns `account` only to the agent's owner and workspace
administrators, including the owner's authorized superagent. Ordinary members
and ordinary agent credentials cannot read it, including in runtime-report
responses. This rule applies to details, lists and overview. Older runners omit
the field; older Hubs ignore it. Existing databases store it in runtime JSON and
need no schema migration. The App shows it alongside the cloud/machine location
when present; provider names continue to describe the model provider separately.


### Local App model-provider connection test

`POST /app/hubs/{id}/providers/test` tests the unsaved form; the `{pid}/test`
variant uses a saved provider's sealed address and key. The response remains
`{models: [...]}` with an optional `models_source` of `provider` or `catalog`.
The latter means an explicit, bounded MiniMax completion verified the chosen
endpoint when model discovery was unavailable; it is not a discovered list.
Background discovery never generates completions. MiniMax HTTP 401 failures use
`provider_region_or_key` and a suggestion to check the selected region and key.
Neither errors nor responses contain the request key. No Hub API or vendor id
changes are required; region choice is stored in the existing address fields.
### Machine health and verified process termination

New runners sample once per minute per runner process and may include optional
`machine_health` in `PUT /v1/me/runtime`. It contains `at`, `runner_instance`, `owner_id` (bound to the reporting agent owner),
`complete`, `alert`, `load_ratio`, `load_metric` (`load1_per_core` on Unix,
`cpu_busy` on Windows), `cores`, `memory_total`, `memory_available`, and at most
20 `processes`. Process fields are `name` (basename only), `pid`, `created`
(an OS creation identity), `cpu` (percent of one core), `memory`, optional
`agent`, `suspicious`, and an optional short-lived `ref`. No argv, environment,
working directory or executable path is included. Samples older than two minutes
or incomplete samples are unknown and cannot authorize process termination.

Load above 70% of cores or available memory below 10% for ten continuous minutes
raises an alert. A verified agent descendant whose parent has exited or has been
replaced by an init/subreaper, consuming at least 80% of one core for thirty
continuous minutes, is a suspected orphan. Sampling gaps break continuity.
Unknown-source processes contribute to aggregate pressure but never receive a
termination reference. Alerts never automatically terminate a process.

`GET /v1/machines` and overview machine entries include optional `machine_health`
and `can_stop_process`. Reports from a former owner are refused after an ownership transfer. Only the machine owner and workspace administrators
receive process details or process controls. Per-agent runtime projections expose
aggregate health only. Coordinators of affected projects receive a deduplicated
note on an alert and another on recovery.

The machine owner or administrator can POST to `/v1/machines/{id}/spawn` with
`{"cli":"process-stop","process_ref":"<current ref>"}`. This is a typed control
operation, never a shell command. The machine runner checks the current machine
and agent ownership, reference freshness, and OS process creation identity again.
Unknown, expired, reused or unowned processes are refused. A successful response
has `stopped_process: {name, pid, created, ...}`. Audit action
`machine.process.stop` records only name, PID, creation identity and the owning agent in its detail,
plus the standard audit actor and machine target. It records no command line.
Older runners refuse this special CLI instead of executing an unrelated command;
older hubs reject the unknown report field, which new clients specifically omit
on retry without suppressing authentication failures.

Harness children live in Unix process groups or Windows Jobs. A normal turn keeps
background children and their ownership records. Explicit stop, session/profile
replacement, and runner shutdown terminate owned groups, with a bounded grace
period before forced termination. Windows launches suspended, assigns its Job,
then resumes; Job assignment failure aborts the launch. Windows Jobs disallow
breakaway and terminate members on final handle close. Unix children that escape
with setsid before their ancestry is observed cannot be reliably attributed by a
process group alone. Local ownership registries contain metadata only, with
owner-only permissions; unknown historical processes are never retroactively
claimed or offered a terminate button.

For a fresh execution-time ownership check, `GET /v1/me` includes optional
`machine_owner_id` read from the bound machine's current record. A present empty
string means unowned; absence (an older Hub or unreadable record) means unknown.
The process-stop runner refuses unknown ownership or a machine owner different
from the verified agent owner, including a machine transferred without its nodes.

### Coordination statistics (read-only)

Authenticated admin/member/agent credentials:

| Method | Route | Query | Response |
|---|---|---|---|
| GET | `/v1/stats/agents` | `name`, `project`, `since=7d` | `api.AgentStatsList` |
| GET | `/v1/stats/agent-pick` | `skill`, `cli`, `project`, `machine`, `count=3`, `strict=false` | `api.AgentPickList` |
| GET | `/v1/stats/coordinator` | `name` (default caller agent), `since=7d` | `api.CoordinatorStats` |

These routes return `version: 1`. Durations accept Go duration syntax plus `d`,
positive and at most 366 days; count is 1..100. Timings are seconds with explicit
sample counts and null for unavailable values. New clients report `unsupported`
against old Hubs; existing runners need no new fields or calls. No mutation,
reservation, token refresh or `dry_run` is involved.

Visible profiles are filtered separately from caller-readable task history.
Non-manager history is limited to participant messages, identified by `scope`;
project selection never expands it. Private account quota visibility is applied
before selection. No message bodies, raw error text or credential data are returned.
Cohorts, outcome definitions, queue coverage, resend/reassignment inference,
follow-up counts, percentile convention and deterministic ranking weights are
specified in [Coordination statistics](coordination-statistics.md). API structs
live in `internal/api/coordination_stats.go`; web contracts in
`web/src/lib/coordination-stats.ts`.

### Sent-task board and coordinator alerts

`GET /v1/tasks/board?project=<name>&stale=15m&all=true` accepts agent or
console/admin credentials. By default it returns every unfinished **task sent by
that caller**, across projects. `project` restricts the result to a visible project.
`all=true` requires `project` and either a coordinator agent in that project, its
human owner, or a workspace administrator; it returns all unfinished tasks in that
project. Ordinary members cannot enumerate other senders' tasks with this route.

The response is `{"observed_at":"<RFC3339>","tasks":[...]}`. Items contain only
`id`, `from`, `to`, `thread_id`, optional `project`, `status`, `created_at`, optional
`active_at`, `last_activity_at`, `age_ms`, `queued_for_ms`, `idle_ms`,
`stale_after_ms`, and `stale`. No body, result, attachment, credential, error text or
runtime/account data is read or returned. States are `queued`, `delivered`,
`acked` and `held` (delivered/acked are in progress). `queued_for_ms` is the age of
currently queued tasks since creation, zero otherwise; it does **not** reconstruct
historical queue latency or reset to the start of a requeue episode. Activity follows the existing `Message.LastActive` rule: the later of
`active_at` and the last state/metadata update (`updated_at`), falling back to
creation for a legacy task with neither timestamp. A new ack/hold is activity
even if its previous recipient heartbeat was older. Items sort stale first, then greatest idle time,
then ID. An empty result is `tasks: []`.

`stale` overrides only the board's display threshold, in whole seconds from `1s`
to `168h`. Without it, each task uses its project's configured alert threshold,
or 15 minutes for projects without an override and non-project tasks.

`GET /v1/projects/{name}/task-alerts` returns
`{"project":"acme","stale_seconds":900}`. `PUT` to the same route with
`{"stale_seconds":1200}` changes only this project's threshold (1–604800 seconds).
Both require the same project coordinator/owner/administrator permissions as
`all=true`. `PUT ?dry_run=true` validates permissions and returns the proposed
policy without persisting it. Concurrent ordinary project edits do not overwrite
this independent setting. No fields are added to old project requests/responses.

New console tasks are additionally bound to their authenticated member's ID in
internal storage; the existing display-name `from_owner` contract is unchanged.
Human boards, alert destinations and the cooldown follow that member across
renames. Missing/removed accounts never redirect notes to a reused old handle.
Historical console tasks without this attribution are not guessed from names;
use a project-authorized `all=true` view for those records. Agent-sent tasks,
including old runners' tasks, remain addressed by their existing agent identity.

Every 30 seconds the Hub evaluates metadata for three conditions:

- An open task has no recipient activity for its project's stale threshold.
- A delivered/acked task's recipient has a nonzero heartbeat whose online window
  has expired, after overlapping the task's lifetime. Held tasks and agents never
  seen online during that lifetime do not trigger this condition.
- Within a rolling 10 minutes, tasks have failed on at least three **distinct**
  agents on the same verified machine and owner. Failure time, agent and machine
  identity are captured in the failure transaction, starting after this migration;
  later agent moves, ownership changes and message metadata edits do not rewrite
  them. A quiet gap greater than 10 minutes starts a new machine incident. The
  incident is an observation, not an assertion of infrastructure failure or OOM.

The Hub sends the task sender a `note` with affected task metadata and suggested
`myco task board` / `myco message steer` commands. Machine incidents disclose only
that sender's task reference and an aggregate distinct-agent count, never other
senders' tasks, machine paths or task contents. One note batches up to 50 conditions
across the sender's projects. At most one alert note per sender is committed in
five minutes; remaining conditions wait for later batches. Stale/offline conditions
that resolve before delivery are discarded and may be detected again later.
Delivered stale/offline conditions are deduplicated once per task and condition;
machine alerts once per incident and affected sender. Deduplication, cooldown and
note insertion commit atomically and survive Hub restarts. Existing task replies
and failures retain their ordinary delivery; the alert budget does not suppress them.

New clients must treat missing task routes on an old Hub as `unsupported` with an
upgrade suggestion, not simulate a board from a truncated inbox/history listing.

### Managed agent fork and retirement

These endpoints require an agent or console/admin token. Older Hubs without these routes
are reported as `unsupported` by newer clients. Machine nodes must advertise
`fork_retire_v1`; ordinary agent spawn remains unchanged. See [managed copies](agent-forks.md).

- `POST /v1/agents/fork?dry_run=true`: `AgentForkRequest` (`from`, `count` 1–32,
  `worktree`, `name_prefix`, optional `for_task`, `force`, required `request_id`).
  Returns `AgentForkResult {forks, dry_run?}` with durable reservations and their spawn
  message IDs. An identical creator/request key returns the original copies; changing
  its source, count or options conflicts. Only the machine's normal spawn senders or
  permitted project coordinators may request copies. `force` affects capacity/quota only.
- `POST /v1/agents/retire?dry_run=true`: `AgentRetireRequest` with exactly one of `name`
  or `fork_of`, and optional `now`. Returns `AgentForkResult`. Retirement first records
  the existing work allowed to finish and blocks new claims. The original creator identity,
  current machine owner or administrator may retire copies; ordinary labels confer no rights.
- `GET /v1/agent-forks/{id}`: an `AgentFork` provenance/lifecycle record, visible only to
  its creator, the machine owner/admin, or that exact copy with unchanged ownership.
- `GET /v1/me/agent-forks/{id}`: the trusted execution plan, restricted to the exact
  registered machine node and current machine owner binding.
- `POST /v1/me/agent-forks/{id}`: `AgentForkCompletion {workdir, repository?, branch?,
  retired?, error?}`. Only that machine node may confirm registration or cleanup. The Hub
  checks the registered identity, enrollment, directory and owner; retirement deletes by
  immutable ID and fails remaining queued work in the same transaction.

`AgentFork` includes `id`, `batch`, `name`, `agent_id?`, `fork_of`, `source_id`, `cli`,
`source_session`, `source_dir`, `source_visibility?`, `source_allow_from?`, `forked_by`,
`for_task?`, `machine_id`, `node_id`, `node`, `owner_id`, `worktree`, `force?`, `access?`,
`model?`, `name_prefix?`, `state` (`planned`, `ready`, `retiring`, `retired`),
`message_id?`, `retire_message_id?`, `retire_attempt?`, `finishing?` (already claimed message
IDs), `workdir?`, `repository?`, `branch?`, `now?`, `created_at`, and `error?`.
Creator identity IDs are private storage metadata, never an editable permission field.
Copies retain source visibility and sender restrictions. Only the initial context-copy turn
requires the source's original identity, session and permissions still to match.


### App-local CLI upgrade receipts and idle restarts

These routes are local App endpoints under its existing origin/session protection;
they do not change the Hub schema. Contract: `internal/api/local_upgrade.go`.

| Route | Result |
| --- | --- |
| `GET /app/local/upgrade` | `LocalUpgradeStatus`: persisted `cli` receipt (`running`, `succeeded`, `failed`, JSON `result` or `error`), `installed` CLI version, fresh `runner` runtime, optional `restart`, `pending`, `expired`, `unsupported`, and optional error |
| `POST /app/local/upgrade` | 202 with status; retries detached CLI upgrade with `--restart-when-idle` and a separate result file per attempt |
| `POST /app/local/upgrade/restart` | 202 with status; requests another six-hour idle window; does not perform an immediate service restart |

The runner owns admission across all local profiles, including undelivered results
and command watches. Poll responses reserve messages before dropping their poll
lease. Existing work is never canceled for idle requests. Service handoff checks
request, instance and managed service PID; stale helpers cannot restart a replacement
runner. A failed handoff or unchanged version requires explicit retry. Missing/stale
runtime remains unknown; the installed file does not prove a new runner is running.
Older slot reports and configured local machines’ workspace reports supply version
display with `unsupported: true`, never restart
permission. Receipts are private and omit credential-bearing download URLs.

If a service restart may have been submitted but its result is unknown, runner
state is `restart_unknown`: new claims remain paused and automatic retries stop.
The App keeps the error and a manual-check action; it never reopens claims while
a delayed service stop could still interrupt them. CLI receipts capture setup
errors before the upgrade handler too. Each result file is bounded to 64 KiB.


### Desktop code-host owner confirmation

Local App endpoints `POST /app/codehost/prepare` and `POST /app/codehost/connect`
require the trusted owner's `X-Myco-Codehost-Owner: 1` header and the page's
`codehost` permission. Prepare takes `{page:{hub,id,name},provider,host,source,name}`
and returns `{confirmation,provider,host,source,private_network}`. Connect takes
`{confirmation,token,allow_private}` and returns only connected account metadata.
The one-use confirmation binds the workspace sign-in, page and immutable server
origin for two minutes. The token is entered in the host dialog; it is never a
page bridge argument. See [Code hosts](app-os.md#code-hosts-desktop) for the
read-only `forge.accounts` and `forge.query` page contracts and network policy.
