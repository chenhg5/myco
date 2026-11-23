# Server Console

Manage Linux servers through a person's local OpenSSH profiles. This app has no shared command queue. Never write commands or credentials into page data and claim they ran. There is no arbitrary shell or sudo action.

The desktop App must advertise `ssh`. SSH profiles, config, trust and credentials stay on the person's controller computer. A human first adds an existing trusted SSH target, then explicitly enables Agent access for the page's current workspace. ProxyJump and encrypted-workspace action execution are unsupported in this version. Mobile and old desktop builds may show an unsupported message.

## Discover and request permission

Run `myco action targets` using your own agent identity. Select a resource with `provider: "server.ssh"`. Preserve its controller machine ID, profile ID and exact binding. Request a limited grant:

```
myco action grant request --machine mch_... --ssh-profile ssh_... --binding <sha256> --permission server.ssh.read --reason "Inspect the server"
```

The person sees the trusted profile name and resolved SSH destination in the approval. `server.ssh.read` permits overview, service/container lists, status and bounded logs. `server.ssh.manage` additionally permits start/stop/restart. Grants are target-bound and expiring; profile/config/trust changes invalidate them. Do not fall back to a grant without a resource, another computer, person's App credentials or raw local SSH access if permission fails.

## Fixed actions (v1)

- `server.ssh.overview`
- `server.ssh.services.list`, `.status`, `.logs`, `.start`, `.stop`, `.restart`
- `server.ssh.containers.list`, `.logs`, `.start`, `.stop`, `.restart`

Arguments are only `name` and `lines` where relevant. Service names must end in `.service`; container IDs/names are restricted tokens. Logs accept 1–200 lines. Request actions through the framework CLI/SDK with a stable idempotency key. Results contain stdout, stderr, exit_code, truncated and duration_ms; large results become private JSON artifacts. Use the authenticated artifact API, not public links.

A succeeded action job means the command returned, not that its exit code was zero. Check exit_code and reread resource state before describing a management operation as successful. Keep the same key when recovering a lost response. Cancellation or disconnection during a mutation may mean outcome_unknown; never retry with a new key automatically. Human UI activity is session-local; framework jobs and audit are authoritative for agent operations.

The application-domain SDK is `templates/src/server-console/agent-actions.ts`. Pure output adapters live in `protocol.ts`. Native human calls use `ssh.profiles` and `ssh.run` separately; they do not approve an agent grant.
