# Browser component compatibility

Page and popup attachment pauses their renderer until the recorder binding and
new-document script have been installed. An iframe session's **nested**
auto-attachment uses `waitForDebuggerOnStart:false`: verification frames must be
able to start their child frames and workers without waiting for our collector.
The initial iframe attachment is still resumed after its own installation.

The collector is installed with `Page.addScriptToEvaluateOnNewDocument` and
`runImmediately`, then injected into existing isolated contexts. Existing
contexts also receive the current recording state. A newly created iframe can
run before these hooks finish, so its earliest events can be missed; recording
continues once installation completes. Pages/popups retain their original
installation-before-resume ordering.

Cross-process iframe frame trees are read from each attached renderer and merged
using `parentId` before recording a selector path. Rod 0.116.2 does not issue its
own `Target.setAutoAttach`; the unit test also checks the calls made during its
page initialization.

Default launch flags omit experimental Blink switches such as
`--disable-blink-features=AutomationControlled`, which display Chrome’s unsupported
command-line warning. The component does not set `--enable-automation`. Chrome
supplies its own user agent. The collector stays in an isolated world and does
not install a stealth script.

Named form controls retain their semantic/ID fallback locators. An unnamed
document element may have only a structural CSS locator; this remains a valid
recording target. The native fixture verifies both kinds and replays them.

Native navigation assertions wait for the new document's readiness, then verify
the complete recording after stop has flushed the collector and drained its
evidence queue. A navigation commit alone does not mean screenshots and panel
updates for preceding steps have finished.

Run `go test -race ./...` and `go vet ./...` in this directory. On a machine with
an installed Chrome or Edge, enable the existing native fixture flag:

```sh
go test -race -timeout 5m -run TestNativeChrome -args -browser-smoke
```

`TestNativeChromeNestedIframeRecording` checks a cross-site iframe, its nested
iframe and workers, and trusted input capture in both iframe levels. These
fixtures only use local pages and fresh dedicated profiles.

The desktop pins component 0.3.0 and also accepts an already installed 0.2.0, 0.1.1 or 0.1.0
until the owner chooses **Update component**. A successful update verifies and
installs 0.3.0 before stopping the old process; the next browser operation starts
the new process. A failed download or checksum check leaves the old process and
profiles intact. Updating interrupts any browser sessions still using that
process, so finish recording or playback before updating.

Profile proxy/UA/locale/launch settings are owner-confirmed, local-only and take
effect when a new browser session starts. See
[the settings contract](../../docs/browser-component.md#per-profile-network-and-launch-settings).
`TestNativeChromeProfileSettingsAndBypass` verifies real Chrome's UA, language,
Accept-Language, explicit bypass and saving settings without restarting.

Proxy passwords currently live as plaintext in the private 0600 profile index,
not an OS keychain. The random loopback forwarder has no additional
authentication: other local processes can use its upstream access while the
browser session is open. A future keychain integration is separate work.

## Live agent control

Builds advertising `status.features: ["agent_control", ...]` implement protocol-1
`POST /control/open`, `/control/snapshot`, `/control/act`, `/control/extract`,
`/control/screenshot`, `/control/close`, and the App-only
`GET /control/events?session=<id>&after=0`. These are private loopback endpoints,
protected by the component bearer and `X-Myco-Scope`. They are not a public
browser-control server. The trusted desktop verifies the agent's grant and
binds its scope to the workspace, owner, profile and grant.

Every POST requires `request_id` (the desktop passes the Hub job ID). Repeating
an ID with the same operation and parameters reuses its immutable outcome;
changing parameters returns `conflict`. Outcomes are never replayed to a closed
session, except the voluntary close response. Requests arriving while another
operation runs return `busy`; a forced close bypasses that operation lock.
Closed browser sessions and ordinary observations expire after a two-minute retry
window. Open request tombstones and close outcomes remain until the grant expires
(at most thirty minutes), so a lost close can retrieve its draft and a repeated
open cannot recreate a closed browser. Cache limits remain bounded.

Open takes `profile`, optional HTTP(S) `url`, and desktop-injected `grant_id`,
`actor: {id, name}`, `expires_at` (RFC 3339, within thirty minutes), and optional
`hosts`. A profile may
have `kind: "agent"` to identify a clean, dedicated agent profile. Agent access
still requires the owner's explicit profile preference and grant. Sessions are
headed and exclusive; opening a profile already in use returns `busy`.

The six operation bodies and their additional fields are:

| Endpoint | Fields |
| --- | --- |
| `open` | `profile`, `url?`, `grant_id`, `actor`, `expires_at`, `hosts?` |
| `snapshot` | `session`, `tab?`, `mode?: "a11y" or "text"`, `max_nodes?`, `include_screenshot?` |
| `act` | `session`, `tab?`, `op`, operation-specific fields below, `timeout_ms?` |
| `extract` | `session`, `tab?`, `ref`, `what: "text", "html" or "attribute"`, `attr?` |
| `screenshot` | `session`, `tab?`, at most one of `ref`, `clip`, `full` |
| `close` | `session`, desktop-only `reason?` |

Unknown fields are rejected, including fields irrelevant to the selected
operation. The `act` whitelist is:

| Operation | Additional fields |
| --- | --- |
| `navigate` | HTTP(S) `url` |
| `click`, `dblclick`, `hover` | `ref` |
| `type` | `ref`, `value`, optional `clear` (defaults to true) |
| `select` | `ref`, `value` (native option value) |
| `press` | `key`, optional `ref` |
| `scroll` | finite CSS-pixel `dy`, optional `ref` |
| `wait` | `wait: "element"` plus `ref`, `"url"` plus `value`, or `"network"` |
| `back`, `forward`, `reload` | none |
| `switch_tab`, `close_tab` | `tab` |
| `dialog` | `choice: "accept" or "dismiss"` |

Timeouts default to ten seconds and cannot exceed thirty seconds. Live control
does not accept JavaScript execution, uploads or clipboard paste. Parameter
expressions such as `${name}` are rejected in live inputs: enter an example,
then parameterize the returned draft before review.

Snapshots contain `snapshot_id`, `tab`, `url`, `title`, `frames`, `nodes` (or
`text`), `node_count` and `truncated`. Nodes have `id`, optional `parent` and
`ref`, `frame`, `role`, `name`, optional `text`, `value` and `rect`, plus
`interactive`, `disabled`, `redacted` and `in_viewport`. Rectangles use CSS pixels
in the identified frame's viewport. Accessibility information is supplemented
with pointer-cursor and clickable custom controls; visible nodes are prioritized.
The default and maximum are 400 nodes and 64 KiB of serialized JSON. Action
responses contain a brief snapshot capped at 80 nodes and 16 KiB. Truncation
preserves valid JSON. Unavailable child frames are reported explicitly.

Refs are monotonically allocated within a session, bound to the original tab,
frame, document epoch and backend node. Navigation, frame replacement or node
detachment produces `stale_ref`; the component never substitutes a similar
locator. Draft targets use the recorder's multiple locators, frame paths and
shadow paths instead. Password, one-time-code and `data-sensitive` values are
redacted; typing into a sensitive field is refused. HTML and text extraction
sanitize sensitive descendants. Screenshots redact unreadable iframe rectangles to opaque black before storing
artifacts. Readable documents containing sensitive elements, including closed
shadow roots, still refuse screenshots. A frame that cannot be located, or a
layout/document change during capture, also refuses the screenshot.
The owner's control panel is excluded from snapshots, screenshots and drafts.

Screenshot `clip` coordinates are relative to the top viewport in CSS pixels.
`full` captures the actual document bounds. Native results contain
`screenshot: {id, type, size}` and `image: {width, height, scale}`. The desktop
retrieves the artifact through the existing scoped `/artifacts/<id>` endpoint
and uploads a private action artifact; local paths and image bytes are not
returned to agents. The artifact size limit is 4 MiB.

A successful action returns `ok`, `active_tab`, `tabs` (each with `id`, `url`,
`title`, `active`), `new_tab`, `step_ids` and `snapshot`. An observation failure
after a successful action is reported as `snapshot_error`, not a failed action.
Errors have `code`, `error`, optional `reason`, `host`, `message` and `applied`.
An ambiguous dispatched action returns `outcome_unknown` and must not be retried
with a new request ID. Normal results include the observed URL's `host` for audit;
request parameters, full URLs and page contents do not belong in audit records.

`hosts` accepts exact hostnames or subdomain patterns such as `*.example.com`.
A wildcard excludes its bare parent domain. Ports do not change hostname matching.
An omitted list means unrestricted domains; an explicit empty list is rejected.
Explicit navigation is
checked before dispatch. A top-level redirect or link crossing the scope is
rolled back; a new disallowed tab is closed. The response and event use
`host_not_allowed` on the originating action, before it enters the draft.
Input, scroll and hover return immediately after the event barrier when no
navigation is pending. Operations likely to navigate observe an additional
500 ms, then wait for any navigation already started to commit, including slow
redirects and popup documents. Downloads, HTTP 204 and cancelled navigations
finish without a new document. Later jumps (including a 600 ms script timer)
are corrected in the background and reported on the next call; the completed
action remains in the draft. The two-second recording window does not make
every action wait two seconds. A tab gets one rollback attempt per deliberate
action; repeated automatic crossings go to `about:blank` to stop reload loops. Derived URL
waits use a literal prefix capped at 8 KiB of JSON, with parameter-like fragments
removed. Iframes and subresources are outside this top-level scope.

The draft budget is 200 steps and 192 KiB for serialized `steps`; operations
reserve their logical step and any derived navigation or popup before dispatch.
Successful extraction and screenshots allocate shared `data1`, `data2`, ...
variables. A voluntary close returns `steps`, `removed`, `next`, `done: true`,
`reason: "closed"`, `profile` and `start_url`, and closes the browser process.
New replay node types are `back`, `forward`, `reload` and `tab: "close"`.

Only voluntary close exports a draft. The native panel shows the agent's name
and last operation without input contents, with an owner stop button. Stopping
immediately cancels work, closes the browser and discards the draft. Later calls
return `session_closed` with `reason: "stopped_by_owner"`. Five minutes of
inactivity, grant expiry or revocation, profile disablement and desktop shutdown
also discard the draft. Trusted desktop close reasons `grant_ended`,
`host_closed`, `profile_disabled` and `idle_timeout` have the same discard behavior.

The events response is `{session, events, next, done, reason}`. Each event has
`time`, `done`, and optional `action`, `host`, `code` and `reason`; it contains no
steps or page contents. `after` is a replayable cursor. Polling does not renew
session inactivity. A terminal event lets the desktop revoke its grant promptly.
