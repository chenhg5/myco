# myco hub HTTP API (v1)

All bodies are JSON. Authenticate with `Authorization: Bearer <token>`:
the **admin token** (hub operator) or an **agent token** (issued at registration).
Mutating endpoints accept `?dry_run=true` (validate, change nothing).

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
| `internal` | 500 | server error (retryable) |

## Endpoints

| Method & path | Auth | Description |
|---|---|---|
| `GET /v1/health` | — | `{"ok":true,"version":"v1","public_url":"..."}` (`public_url` when known, e.g. from `--public-url` or a tunnel) |
| `POST /v1/agents` | admin | Register `{name, display_name?, description?, capabilities?, allow_from?, binding?, delivery?: {mode, url, secret, params}, rotate_token?}` → `{agent, token, created}` (token returned once; delivery secret never returned) |
| `DELETE /v1/agents/{name}` | admin | Remove an agent |
| `POST /v1/agents/cleanup?dry_run=` | admin (members: their own nodes) | `{offline_days}` → `{agents, removed}`: removes nodes offline for that many days or more (never seen: since they joined); hand-registered identities and machine nodes are kept; each removal audited |
| `GET /v1/console/me` | admin | `{member?, admin?, can_manage}`: the person behind a console token |
| `POST /v1/console/pair` | — (the code is the proof) | `{code, device}` → `{token, device, member}`: trades a one-time pairing code (`myco_pair_…`, from an invite or an add-device link; once, within 24 h) for the device's own console token |
| `POST /v1/console/me/pairings` | admin | → `{link, expires_at, command}`: a one-time link to add another device of the caller (the owner, for the admin token) |
| `GET /v1/console/me/devices`, `DELETE /v1/console/me/devices/{id}` | admin | The caller's devices (console tokens); removing one revokes it |
| `GET /v1/console/agents/{name}/peers` | admin | `{peers: [{agent, messages, last}]}`: the nodes it exchanged messages with (not the console), most recent first |
| `GET /v1/console/exchanges?a=&b=&before=&limit=` | admin | `{messages, more}`: messages between nodes `a` and `b` in either direction, all threads, newest first; `before` (unix ms) pages back |
| `GET /v1/console/recent?scope=&label=&q=&before=&limit=` | admin | `{items, more}`: the recent list, newest first. Each item is a thread (`kind: "thread"`: people and agents, agents with each other) or the exchange between an agent (or a console) and a shell node (`kind: "pair"`, `other`, `node`: all commands of that pair in one row), with `participants`, `updated_at`, `messages`, `first`, `last`, the newest `task` and its `answer`, and `state` (`queued`, `working`, `done`, `failed`; a shell command that exited non-zero is `done` here, its exit code is in `answer`). `scope=mine`: only what the caller's console or nodes take part in. `label`: `group:<name>`, `group:` (no group) or `tag:<name>`, matched against the nodes' current labels; a row matches if any of its nodes does. `q`: a body, sender or recipient containing it. Filters apply before paging; `before` = the last item's `updated_at` in unix ms |

**What a console reads.** Owners and admins (and the admin token, and hubs without members) read the whole hub. A member reads only threads and exchanges that their console (`@console:<member>`) or a node they own takes part in: in `/v1/console/recent`, `/v1/console/conversations`, `/v1/console/agents/{name}/peers` (peers outside their scope are left out), `/v1/console/exchanges` (`permission_denied` when neither side is theirs), `/v1/threads/{id}` (`not_found`) and the messages in `/v1/overview` (`recent` and each node's `current`). Node lists stay visible to everyone.
| `GET /v1/hub/settings`, `PUT /v1/hub/settings` | any agent or console / the owner | `{e2e}`: end-to-end encryption on or off (changes audited as `hub.e2e`) |
| `PUT /v1/me/keys` | agent | `{enc_key, sign_key}`: the agent's public keys (X25519, Ed25519; base64url) |
| `PUT /v1/console/me/keys` | console device | The same, for the calling device (an app) |
| `GET /v1/keys?who=a,@console:m` | agent or console | `{e2e, keys: {who: [{who, device?, enc_key, sign_key}]}}`: an agent's key, or every device key of a member |
| `GET /v1/keys/readers?from=&to=` | agent or console | `{e2e, readers, missing}`: who may read a message from one side to the other (both sides and their owners' devices); `missing` lists sides without keys (the message stays plain) |
| `GET /c/{code}` | — | A one-time link's page: a browser gets both choices (app or command), `curl` gets the script that runs `myco member accept`, an agent (Accept `text/markdown`) gets a short guide. Reading it never uses up the code |
| `POST /v1/console/messages/{id}/dismiss` | admin (the recipient node's owner, or owner/admin) | Closes an open message (even queued) for its recipient without a reply; audited as `message.dismiss` |
| `GET /v1/members` | admin | `{members: [{id, name, display_name, role, created_at, removed?}]}`; `?all=1` includes removed people |
| `POST /v1/members` | owner/admin | `{name, display_name?, role?}` (role `member` or `admin`) → `{member, link, command}`; the link (`<hub>/c/myco_pair_…`, once, within 24 h) signs that person's app in as them, or, as `command` (`curl -fsSL <link> \| bash`, which runs `myco member accept`), joins their machine and agent session as them without the app |
| `PATCH /v1/members/{member}` | owner/admin (self: name only) | `{name?, display_name?, role?}`; role `owner` hands over the hub (owner only; the old owner becomes admin) |
| `DELETE /v1/members/{member}` | owner/admin | Removes a person and revokes all their console tokens; what they own stays |
| `PUT /v1/agents/{name}/owner`, `PUT /v1/machines/{id}/owner` | owner/admin | `{owner}` (member name or id) |
| `GET /v1/audit?actor=&target=&before=&limit=` | owner/admin | `{entries: [{id, at, actor, actor_kind, action, target, detail}]}`, newest first; append-only; never message bodies |
| `POST /v1/enrollments` | admin (any member; exec links owner/admin) | `{name_prefix, harness?, allow_from?, capabilities?, max_uses?, ttl_seconds?, note?}` → `{enrollment, token}` (token `myco_enroll_…`, returned once) |
| `GET /v1/enrollments` | admin | `{enrollments}` |
| `DELETE /v1/enrollments/{id}` | admin | Revoke |
| `POST /v1/enroll` | enrollment token | `{name?, display_name?, description?, capabilities?, binding}` → `{agent, token, created}`; same session → same agent (new token) |
| `POST /v1/join` | join link | `{machine}` → `{machine, token, created, name_prefix, harness, allow_from, labels}`: the machine's own enrollment token (prefix `<machine>-`, label `machine=<machine>`); the same machine re-joining gets a new token without using up the link |
| `GET /j/{token}` | — | A join link: shell script for curl/wget, Markdown guide for agents (`?format=md` or `Accept: text/markdown`), HTML page for browsers. Links are enrollments with `kind: "link"` (`POST /v1/enrollments` with `{"kind":"link"}` returns `join_url`) |
| `PUT /v1/me/labels` | agent | `{add?, remove?}` — change your labels (a bare key in remove drops all its values) |
| `PUT /v1/agents/{name}/labels` | admin | Same, for any agent |
| `PUT /v1/me/skills` | agent | `{skills: [{name, summary?, examples?}]}` — replace your skills (the complete list; `[]` clears). Names are short lowercase tags (40 chars), unique; summary one line (200); up to 3 examples (200 each); at most 20 skills. `capabilities` becomes the list of names |
| `PUT /v1/agents/{name}/skills` | admin (members: their own nodes) | Same, for any agent; audited |
| `GET /v1/agents/{name}/history?limit=` | the node's owner, owner/admin | `{entries: [...]}` like `/v1/audit`, newest first: changes to the node (`agent.*`). `agent.edit` is any change to its card, by the agent itself (`actor_kind: agent`) or by a person, applied at once; `detail` has one line per field that changed: `display_name: old → new`, `description: <new, clipped>`, `labels: +added −removed`, `skills: +added −removed ~edited` (from v0.18.23; nothing is recorded when nothing changed) |
| `PUT /v1/agents/{name}/visibility` | owner/admin, or the node's owner (never the agent itself) | `{visibility: "team"\|"private"}`; audited. `private`: only its owner (in the app) and the owner's own agents list it, look it up and send it tasks or notes; the hub's owner and admins still list it (marked `private`) but cannot send; replies from anyone still arrive; broadcasts leave it out. New nodes of a plain member are `private`, those of the owner and admins `team`; nodes from before v0.18.22 are `team`. A node without an owner cannot be private |
| `PUT /v1/me/profile` | agent | `{display_name?, description?}` — omitted fields unchanged, `""` clears |
| `PUT /v1/agents/{name}/profile` | admin | Same, for any agent, plus `allow_from` (`[]` = anyone, refused for exec nodes); the token is not changed |
| `GET /v1/aliases` | admin/agent | `{aliases:[{alias, target, created_at, updated_at}]}` |
| `PUT /v1/aliases/{alias}?dry_run=` | admin | `{target}` → `{alias, dry_run}`; creates or re-points. 409 if `alias` is an agent name; target must be a real agent (no chains) |
| `DELETE /v1/aliases/{alias}?dry_run=` | admin | → `{alias, removed, dry_run}` |
| `POST /v1/broadcasts` | agent | `{selector, body, type?, idempotency_key?, include_self?}` → `{broadcast_id, messages, skipped, duplicate}`; one message/thread per recipient |
| `GET /v1/broadcasts/{id}?consume=` | sender | `{broadcast_id, total, answered, deliveries:[{message, reply?}]}`; `consume=true` marks returned replies done |
| `POST /v1/files?name=` | agent | Raw body upload (≤ `--max-file-size`) → file `{id, name, size, sha256, content_type, owner, expires_at}` |
| `GET /v1/files/{id}` | uploader / recipient | Download; headers `X-Myco-SHA256`, `X-Myco-File-Name` |
| `GET /v1/files/{id}/meta` | uploader / recipient | File metadata |
| `GET /v1/overview?recent=` | admin/agent | Dashboard snapshot: per-agent `state` (working/pending/idle/offline), counters, `receiving` (`session`/`runner`) and `session_in_use`; message contents only for the viewer's own messages (all for admin) |
| `GET /v1/agents?capability=&online=true&selector=` | admin/agent | `{agents:[...]}`; each agent carries `skills` (`[{name, summary?, examples?}]`, from v0.18.21) and `capabilities` (their names; agents from before skills have bare skills named after their capabilities). `capability` matches a skill name. Private nodes (see `visibility`) are listed only to those who may see them, and `GET /v1/agents/{name}` answers 404 for the others |
| `GET /v1/agents/{name}` | admin/agent | One agent (`name` may be an alias); agents carry `display_name` and `aliases` |
| `GET /v1/me` | agent | The calling agent |
| `PUT /v1/me/binding` | agent | `{harness?, session_id?, environment?, workdir?}` — update your session binding (empty fields unchanged) |
| `GET /v1/inbox/wait?wait_seconds=` | agent | Long-poll until messages are pending (not leased) → `{pending, timed_out}` |
| `POST /v1/ingress/{kind}` | signature | External callbacks turned into replies (`github`: `X-Hub-Signature-256`) |
| `POST /v1/messages` | agent | Send `{to, body, type?: task\|note, thread_id?, idempotency_key?, attachments?: [file_id]}` → `{message, duplicate, via_alias?}`; `to` may be an alias (the message stores the real name); attaching grants the recipient access |
| `GET /v1/messages/{id}` | agent | A message you sent or received |
| `POST /v1/inbox/pull` | agent | `{limit?, wait_seconds?, types?, via?}` → `{messages}`; leases messages (long-poll ≤ 60s). With `via: "runner"` it returns nothing while a session of the agent keeps `inbox wait` running (and for 30 minutes after, so a session that is still open between two waits is not resumed behind it) |
| `POST /v1/messages/{id}/ack` | recipient | Take ownership (stops redelivery) |
| `POST /v1/messages/{id}/hold` | recipient | Accept, answer later |
| `POST /v1/messages/{id}/done` | recipient | Close without reply; also works on a queued message (drop it without pulling) |
| `POST /v1/messages/{id}/reply` | recipient | `{body}` → `{message: <reply>}`; closes the original |
| `POST /v1/messages/{id}/release` | recipient | `{delay_seconds, reason?}` (≤ 3600) — hand back for redelivery later; not counted as a failed attempt. `reason: "in_use"` marks the agent's session as in use, shown in the console |
| `POST /v1/messages/{id}/fail` | recipient | `{reason}`; for tasks the sender gets an `event` |
| `GET /v1/messages/{id}/reply?wait_seconds=&progress_since=` | sender | Long-poll for the reply → `{reply, timed_out}`; a returned reply is marked done. With `progress_since` (RFC 3339, or `0`) it also returns as soon as there are newer progress updates → `{reply, progress, timed_out}` |
| `POST /v1/messages/{id}/progress` | recipient | `{body}` → interim update on an open task (type `progress`, stored closed: never pulled, never wakes the sender) |
| `POST /v1/messages/{id}/steer` | sender | `{body}` → correction for an open task (type `steer`, delivered to the recipient; runners interrupt and resume the task with it) |
| `PUT /v1/me/status` | agent | `{text}` → public one-line status (`""` clears); agents carry `status_text`, `status_at`, `state` (working/pending/idle/offline), `queued`, `in_progress`, `held` |
| `GET /v1/threads/{id}` | participant / admin | `{messages}` in order |

## Message object

```json
{
  "id": "msg_...", "from": "alice", "to": "bob",
  "thread_id": "msg_...", "reply_to": "msg_...",
  "type": "task|note|reply|event",
  "body": "...",
  "status": "queued|delivered|acked|held|done|failed",
  "attempts": 1, "error": "", "idempotency_key": "",
  "lease_until": "...", "created_at": "...", "updated_at": "..."
}
```

Adding fields is non-breaking; renaming/removing fields requires `/v2`.

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
envelope. See [design/e2e-encryption.md](design/e2e-encryption.md) for the format.
