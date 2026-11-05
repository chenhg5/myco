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
| `unauthenticated` | 401 | missing/invalid token |
| `permission_denied` | 403 | not allowed (admin only, allow-from, not your message) |
| `not_found` | 404 | no such agent/message/thread |
| `conflict` / `invalid_state` | 409 | already exists / illegal state transition |
| `internal` | 500 | server error (retryable) |

## Endpoints

| Method & path | Auth | Description |
|---|---|---|
| `GET /v1/health` | — | `{"ok":true,"version":"v1"}` |
| `POST /v1/agents` | admin | Register `{name, description?, capabilities?, allow_from?, binding?, delivery?: {mode, url, secret, params}, rotate_token?}` → `{agent, token, created}` (token returned once; delivery secret never returned) |
| `DELETE /v1/agents/{name}` | admin | Remove an agent |
| `POST /v1/enrollments` | admin | `{name_prefix, harness?, allow_from?, capabilities?, max_uses?, ttl_seconds?, note?}` → `{enrollment, token}` (token `myco_enroll_…`, returned once) |
| `GET /v1/enrollments` | admin | `{enrollments}` |
| `DELETE /v1/enrollments/{id}` | admin | Revoke |
| `POST /v1/enroll` | enrollment token | `{name?, description?, capabilities?, binding}` → `{agent, token, created}`; same session → same agent (new token) |
| `GET /v1/agents?capability=&online=true` | admin/agent | `{agents:[...]}` |
| `GET /v1/agents/{name}` | admin/agent | One agent |
| `GET /v1/me` | agent | The calling agent |
| `PUT /v1/me/binding` | agent | `{harness?, session_id?, environment?, workdir?}` — update your session binding (empty fields unchanged) |
| `GET /v1/inbox/wait?wait_seconds=` | agent | Long-poll until messages are pending (not leased) → `{pending, timed_out}` |
| `POST /v1/ingress/{kind}` | signature | External callbacks turned into replies (`github`: `X-Hub-Signature-256`) |
| `POST /v1/messages` | agent | Send `{to, body, type?: task\|note, thread_id?, idempotency_key?}` → `{message, duplicate}` |
| `GET /v1/messages/{id}` | agent | A message you sent or received |
| `POST /v1/inbox/pull` | agent | `{limit?, wait_seconds?}` → `{messages}`; leases messages (long-poll ≤ 60s) |
| `POST /v1/messages/{id}/ack` | recipient | Take ownership (stops redelivery) |
| `POST /v1/messages/{id}/hold` | recipient | Accept, answer later |
| `POST /v1/messages/{id}/done` | recipient | Close without reply |
| `POST /v1/messages/{id}/reply` | recipient | `{body}` → `{message: <reply>}`; closes the original |
| `POST /v1/messages/{id}/fail` | recipient | `{reason}`; for tasks the sender gets an `event` |
| `GET /v1/messages/{id}/reply?wait_seconds=` | sender | Long-poll for the reply → `{reply, timed_out}`; a returned reply is marked done |
| `GET /v1/threads/{id}` | participant | `{messages}` in order |

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
