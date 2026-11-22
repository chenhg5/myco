# Agent actions SDK

Applications have two ways to collaborate with an agent:

- **Shared data:** use page records and document their schema in `agent.md`.
  This works today through `myco page data`; it needs no running application.
- **Runtime actions:** execute work on an available, explicitly authorized
  provider, such as observing a desktop. These need framework grants and jobs.

The initial action SDK is in `templates/src/_lib/actions.ts` and
`actions-http.ts`. The source includes Hub grants/jobs, private artifacts, a
trusted App worker and native component RPCs. A desktop provider is available
only with a published compatible component installed, a registered workspace
machine and the desktop App running. Older components do not advertise agent
actions. Adding these source libraries does **not** enable agent remote control
in existing installations. New desktop sources also include a restricted shared-
page renderer and an optional `page-actions` bridge for application handler
registration; applications must still register and validate their own exports.
The existing `myco.os`, `myco.net` and `myco.remote` page bridges are not agent tools.

## Layer boundaries

Shared data can include bounded, derived content without introducing an action
queue. Whiteboard 1.4.0 publishes `exports/main` while its page is open: a shared
SVG with a fingerprint of committed source record IDs, versions and data. Agents
read it with existing page data permissions and validate that fingerprint before
using the file. A missing or mismatched export is stale; a large board reports
`too_large` rather than truncating its output. The application owns its renderer,
schema and size limit. This contains no desktop screenshot or local assets and
does not advertise a new grant, SDK action or offline rendering service. See the
[Whiteboard guide](../templates/src/whiteboard/public/agent.md) for extraction.

The producer observes synchronized stores only after writes settle and reads
its singleton output with `data.get`. It does not repeatedly call `data.list`:
the current App advances the page's shared change cursor on a list response,
which could skip changes another editor store has not yet consumed.

| Layer | Responsibility |
| --- | --- |
| Framework | Authenticated identities, owner approvals, scoped expiring grants, target availability, dispatch, job deduplication/cancellation, private artifacts and audit |
| Generic SDK | Types, transport adaptation, action/version discovery, structured errors, job observation and explicit cancellation |
| Application/provider | Business action names, argument validation, domain helpers and execution, application UI |

The generic SDK imports no application code, React, OS implementation or browser
bridge. The application imports the SDK. The framework validates permissions
without understanding application arguments. Client-side guards help callers;
the Hub and trusted host must enforce access again for every operation.

Remote desktop's application adapter is
`templates/src/remote-desktop/agent-actions.ts`. Its namespace is
`remote.desktop`, with separate `remote.desktop.view` and
`remote.desktop.control` permissions. Screen capture and native input remain in
the optional native component; installing the SDK does not install that component.

## Using the SDK with a supported backend

Supply an existing authenticated **agent** request adapter. The SDK does not
read credentials, sign in, assume an App identity or fall back to OS/exec access.
The adapter receives a relative Hub route and a standard `RequestInit`, including
an optional `AbortSignal`, and returns a `Response`.

```ts
import { ActionClient } from "./actions";
import { ActionHTTPTransport } from "./actions-http";

const client = new ActionClient(new ActionHTTPTransport(authenticatedAgentRequest));
const requested = await client.request({
  target: { kind: "machine", id: machineNameOrID },
  permissions: ["remote.desktop.view"],
  reason: "Inspect the reported display problem",
  message_id: taskID,
  ttl_seconds: 300,
});
// An owner approves in the trusted App. A pending request grants no access.
const session = await client.session(requested.id, { wait_seconds: 30 });
// Long polling is bounded: if it is still pending, wait again or return to
// other work. Denial/expiry/end must also be handled before calling actions().
if (session.grant.state === "active") {
  const actions = await session.actions();
}
```

The backend binds a grant to its authenticated requesting agent, target and
optional task. A grant ID alone gives no authority. Grants can be denied,
expired, revoked or ended; callers must handle those states. Artifacts remain
private, expire and become inaccessible when the grant ends. They do not belong
in page data or messages.

The SDK retains grant metadata such as `requested`, `target_name`, `end_reason`
and `ended_at`. A pending grant's `permissions` may be `null`: nothing has been
approved yet. List the agent's grants with `client.grants()`. The current Hub
holds grants, jobs and artifacts in memory; restarting it ends those sessions.

## Remote application example

Once the owner has approved a view-and-control grant and the installed provider
advertises version 1 of both actions:

```ts
import { RemoteDesktopActions, desktopClick, desktopText, desktopShortcut } from "../remote-desktop/agent-actions";

const desktop = new RemoteDesktopActions(session);
const before = await (await desktop.observe("inspect-before-1")).wait();
const image = await desktop.read(before); // private bytes, not a public URL

// Decide the coordinates from this image; dimensions are delivered image pixels.
await (await desktop.input(before, desktopClick(before, x, y), "click-1")).wait();
const after = await (await desktop.observe("inspect-after-1")).wait();
// Inspect the new frame before deciding whether the intended UI action worked.
await desktop.end();
```

The remote application's `desktopText("Hello\n第二行")` turns line breaks and
tabs into physical key events alongside Unicode text. `desktopShortcut(["MetaLeft",
"KeyS"])` creates balanced key-down and reverse key-up events (use `ControlLeft`
for that shortcut on Windows). Pass these arrays to `desktop.input()` after
observing the intended target. They do not use the clipboard. Oversized text,
invalid keys and batches exceeding 64 events fail before dispatch. Keep each
shortcut or drag in one batch: the native provider releases held input after
every batch. These helpers belong to the remote application, not the generic SDK.

Agents using the CLI should read the [Remote Desktop agent guide](../templates/src/remote-desktop/public/agent.md),
which documents discovery, screenshot files, normalized coordinates, `--key`,
input events and stopping. `myco action run` prints `{job, saved}`; the frame
is `job.result` and `saved` contains local image paths. Inspect `job.state` and
`job.error` before treating a successful CLI invocation as a successful action.

Input references its observation's frame ID, not arbitrary native coordinates.
The host must reject stale or resized frames and batches outside the grant.
The application adapter validates the entire batch before dispatching it; it
never truncates text or clamps invalid click coordinates. Text events exclude
control characters; use physical key events for Enter or Tab. A successful input
result acknowledges native injection, not completion of a business operation.

## Jobs, failures and cancellation

`session.invoke()` requires an explicit idempotency key. Persist the key when
recovering a lost response: the backend returns the same job for the same body,
and rejects a changed body under that key. The SDK never automatically retries a
mutation. A lost POST response is `outcome_unknown`; input might already have
been applied. Observe before deciding what to do next.

`task.wait({signal, onUpdate})` observes state changes. Aborting stops waiting;
it does not undo native input or imply that the job stopped. Use `task.cancel()`
to request cancellation. A running job with `cancel_requested` is not yet
terminal; updates include changes to that flag. Terminal results stay terminal.
Cancelled jobs preserve the host's error, such as `outcome_unknown` with its
`applied` count, or `grant_ended` after revocation, rather than erasing that
diagnostic. Errors expose `code`,
`retryable` and, for partial native execution, `applied` where known.

The first protocol uses job state and results with HTTP long polling; it does
not claim an event/progress stream. Unsupported hosts, action versions or native
components must fail explicitly. Never reuse a human pairing code or App token
to bypass missing agent support.

`wait_seconds` accepts 0–60 seconds for grant/job queries (job queries default
to 30 seconds). `ActionError.status` preserves the HTTP response status alongside
the server's code: an inaccessible or expired artifact returns status `410`
with code `not_found`. Request a new observation rather than retrying that ID.
Jobs also expose `artifacts`, `cancel_requested` and their version/timestamps.

Run the SDK contract tests with `node templates/actions.test.mjs` and typecheck
with `cd templates && npm run typecheck`. These tests exercise the SDK and a fake
transport. With Node.js and template dependencies installed, run
`go test -race ./internal/integration -run '^TestAgentActionsSDK$' -count=1`
to exercise the actual SDK against an isolated real Hub and a simulated App
provider. Native capture/input still requires separate component/device tests.
