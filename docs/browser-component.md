# Optional browser automation component

The desktop App can record and replay a linear workflow in a dedicated local Chrome profile. The official **Web automation** application lives in [myco-ai/apps](https://github.com/myco-ai/apps). Its manifest declares `permissions.os: [browser]`.

## Install and use

Open Web automation in the Mac or Windows App, then choose **Install browser component**. The App asks for confirmation in its own dialog, downloads the pinned `browser-v0.6.0` release, checks `SHA256SUMS`, and installs it under `~/.myco/components/browser/0.6.0/`. Installation is optional and never happens on a status request. An unpublished component release produces an installation error; it is not replaced with an arbitrary version.

Existing 0.1.0 through 0.5.0 installations remain usable and report `update_available`.
Choose **Update component** to install 0.6.0. The App verifies and installs the
new package before stopping the old process; download or checksum failures leave
the old process available. The next browser operation starts the new version.
Finish recording or playback before updating, because the old browser sessions
will close. Profiles and login data are retained.

Install Google Chrome yourself. On Windows, Microsoft Edge is used if Chrome is absent. No Chromium download, user-agent override, extension, system service, or everyday browser profile is used. Recording opens a normal visible browser. Log in there yourself when a workflow needs authentication.

Create a profile, record actions, stop recording, and review the resulting sequence before running it. Add a **Confirm** or breakpoint before any submission or other consequential action. Playback executes clicks exactly as recorded: it does not infer that a button submits a form. A full run with `start_url` opens a fresh tab so the previous draft is preserved; **Start here** uses the current tab and its current DOM. Close unused tabs if the 32-tab limit is reached.

## Isolation and local data

- Only generated profiles under `~/.myco/browser/profiles/<id>/` may be opened. The data directory is passed by the App, never by a page. Profile paths and symlinks are checked and profiles have an OS file lock.
- The component receives a random secret through stdin and returns `{address, protocol:1}`. Its HTTP listener binds only IPv4 loopback. Every request needs Bearer authentication and a page/instance scope; browser `Origin` headers and non-loopback Host values are rejected. Hub credentials are not passed to Chrome.
- Closing a page or revoking its browser permission stops its sessions. Closing stdin stops the component; it exits after two idle minutes without sessions. The App owns the component process group/job; Unix browsers use separately recorded groups recovered by the component. Only verified owned processes are terminated. The component never sends `Browser.close` or terminates processes by name.
- Profiles, cookies, passwords, secret parameters, screenshots, structural DOM snapshots, and downloaded files stay on this machine. Shared scripts contain locator metadata and opaque artifact IDs, not artifact bytes or local file paths.
- Password fields record `${password}`, not their value. Accepted prompt input records `${dialog_input}`. Declare those parameters as `secret` and supply them for each run. Secret-bearing runs omit automatic screenshots and redact secret values from returned output. Sensitive recording steps omit screenshots; pages with password/OTP/sensitive fields or inaccessible frames also omit snapshots. Structural DOM evidence contains bounded tag/role/parent relationships, not text, attributes, values or HTML.
- Artifacts are private files, limited to 4 MiB each and 192 MiB/1,024 entries per component store. Where capture succeeds, images have local index references to a page thumbnail and structural DOM evidence. The page bridge retrieves images only. Downloads remain under the browser data directory; run output contains the filename only.
- Upload replay accepts only files previously approved through the host's file picker (`myco.os`), up to 64 MiB. A page-supplied path or private `uploads` allowlist cannot grant access.
- `allowAgents` is a local-owner-confirmed preference only. M0/M1 advertises no agent actions and adds no Hub provider or cross-machine access.

## Page bridge (protocol 1)

Supported Mac arm64/x64 and Windows x64 desktop hosts advertise `myco.features.includes("browser")` so an uninstalled component can be discovered. Check `await myco.browser.status()` for `update_available` (an older compatible component is installed), `installed`, `supported`, `reason`, `protocol`, `version`, browser names, machine name and profiles. Missing installations return an empty version and empty browser/profile arrays. Other platforms and the web-only console do not advertise the feature.

The bridge provides `install`, `profiles.create/open/allowAgents`, `record.active/start/events/stop`, `runs.start/get/control`, `locate`, `pick`, and `snapshot`. `install` and enabling `allowAgents` require a host-owned confirmation, bound to the page instance and action with a one-use, 60-second nonce. The child never receives that nonce. The browser capability also needs the App's per-page, once-per-App consent.

Long polls use cursor deltas (20 seconds maximum); keep the returned cursor. `snapshot(id)` returns an image Blob. Methods follow the [application SDK](https://github.com/myco-ai/apps/tree/main/templates/src/web-automation).

The private component API is authenticated and not a public Hub API:

| Method and path | Purpose |
| --- | --- |
| `GET /status`, `POST /profiles`, `POST /profiles/{id}/agents` | Local status and profile preferences |
| `POST /sessions`, `DELETE /sessions`, `DELETE /sessions/{profile}` | Open and stop owned sessions |
| `POST /record/start`, `POST /record/stop`, `GET /record/events?id=&after=` | Record and stream nodes |
| `POST /runs`, `GET /runs/{id}?after=`, `POST /runs/{id}/{action}` | Linear playback, pause/resume/step/cancel |
| `POST /locate`, `POST /pick` | Locate/highlight or inspect a target |
| `POST /page/{action}` | Local debugging actions (not exposed as agent tools) |
| `GET /artifacts/{id}` | Scope-checked local artifacts |

Recorded locator lists prefer test IDs, semantic role/name, label/placeholder, text, CSS, then XPath. Open shadow roots and frame chains are retained. The collector runs in an isolated CDP world and uses bindings/events, not Go polling. Only top-level page navigations are recorded; same-process iframes and out-of-process iframe roots are ignored. A renderer-requested navigation within two seconds of a recorded click or Enter key becomes a `wait:url` (the intent is captured when navigation is requested, so slow loads still work). Automatic redirects and history/hash changes outside that action window are ignored. Browser/address-bar navigation remains an explicit `navigate` step. Popup creation a `tab:popup`, downloads a `wait:download`, and dialogs explicit `dialog:accept/dismiss` nodes. Playback arms the next dialog before the triggering action. Tab indices are 0-based in each run. Navigation URLs omit queries to avoid persisting credentials; review any workflow requiring query parameters and supply an explicitly safe URL yourself.

Iframe sessions attach their descendants without pausing them, so verification
widgets can initialize normally. Pages and popups retain their paused collector
bootstrap. New-document hooks and existing-context injection capture iframe
events after installation; the earliest events before installation may be missed.
Default flags omit experimental Blink switches that display Chrome’s unsupported
command-line warning, including `--disable-blink-features=AutomationControlled`.
Chrome keeps its native user agent, with no stealth scripts. Website challenges
can still require a person’s interaction.

M0/M1 accepts a single connected, acyclic sequence, up to 128 nodes. Branches, loops and subflows fail validation before the browser opens. Element actions wait for visibility/interactability, not inserted sleeps. Explicit `wait:time` is supported when a person deliberately adds it. `stop`, `skip`, and `retry:1..5` failure policies are supported. Unknown or ambiguous targets fail instead of clicking an arbitrary match.

## Build, verify and release

The independent Go module keeps CDP dependencies out of the Hub/App binary:

```sh
cd components/browser
go test -race ./...
go vet ./...
# Opt in only on a machine allowed to start its installed Chrome/Edge:
go test -timeout 5m -run TestNativeChrome -args -browser-smoke
go build -trimpath -o myco-browser .
```

The optional native fixture records a local form with an iframe and shadow root, replays it ten times, and asserts that its submit handler was never called. Ordinary tests do not start a browser or download one. A fixture passing does not replace website acceptance.

For website acceptance, use the official application with a fresh dedicated visible profile. On both the CSDN article editor and V2EX post form: record once, omit submission, replay ten times, and report successful/attempted runs separately with screenshots and failure details. Also exercise single-step, a breakpoint, and Start here. Do not publish a post. Authentication, CAPTCHA, editor differences and site changes may require manual handling. If the browser cannot start, record **not run**, not a zero-percent trial result.

A preview build of the App (version ending in `-preview`) prefers a component in `~/.myco/components/browser-preview/`, built from the same commit, so a fix can be tried before a component release. Release builds never read that directory, and removing the component leaves it alone. The preview component must report the App's `ComponentVersion`; the start handshake still checks it.

`browser-component.yml` runs Go tests and native Chrome/Edge fixtures on macOS arm64/x64 and Windows x64, and builds three zip archives with dependency notices. `browser-component-release.yml` takes a successful main build run, checks its provenance, signs/notarizes Mac binaries, and stages a **draft** `browser-v*` prerelease with `SHA256SUMS`. Published releases are immutable. A release owner must verify acceptance and publish the draft before installation works. This process does not change the main `VERSION`.


## Owner-approved agent runs (M2)

In the Web Automation application, enable **Allow agents** for a dedicated
profile in the current workspace, then save and approve a script version under
**Agent runs**. The desktop confirmation shows the profile, version, all visited
URLs, executable code, child scripts and complete snapshot. Approval is local to
this App configuration, workspace URL, signed-in account, page and profile. It
never grants authority through a shared page record, and the component's legacy
global agent preference does not enable other workspaces.

The native host stores immutable private snapshots and a SHA256 digest. Adding
or revoking an approval, disabling access or changing accounts invalidates old
bindings and stops their native runs. The agent separately requests a short grant
for `browser.view` and/or `browser.run`; neither permission implies the other.
There is no live page control or arbitrary script submission in M2. See the
[bound browser action API](api.md#bound-browser-profiles) for CLI examples.

Run jobs wait until completion or confirmation. A paused result keeps `run_id`
under the same grant and agent; resumption creates another job. Paused native
sessions survive at most 15 minutes and never past grant expiry. Ending the grant
prevents resumption; already applied website actions cannot be undone by ending a
job. Results and screenshots are private artifacts, not shared page records.

All code fields must be fixed text without `${...}`, including human runs and
code consuming extracted website values. Use declared parameters only for text,
URL and selector fields. The installed 0.1.x component still rejects branch,
loop and subflow nodes before opening a browser. Agent runs cannot use secret
parameters, uploads, Start here or breakpoints. A profile already controlled by a
person or another run is not taken over. No automatic restart or replay bypasses
a native error or run limit.

## Per-profile network and launch settings

On desktop hosts that expose `myco.browser.profiles.configure`, Web Automation
can ask to change a profile's proxy, user agent, language and launch arguments.
The App shows its own confirmation listing the proxy address and bypass rules,
whether credentials are present, the user agent, language and every argument.
The review scrolls independently of the buttons. When its contents overflow,
Allow stays disabled until the owner scrolls to the end; each new request requires
a fresh review. Passwords are never displayed. Declining changes nothing. The confirmation is
single-use and bound to the exact profile and settings, including a write-only
password change. This is a local human capability, not an agent or Hub action.
Older Apps omit the optional method; older components omit profile `settings`.

`GET /status` profiles include this public object:

```json
{"settings":{"proxy":{"server":"https://proxy.example:8443","bypass":["*.local","<local>"],"username":"proxy-user","has_password":true},"user_agent":"","lang":"zh-CN","args":["--window-size=1280,900"]}}
```

The authenticated private component endpoint `PATCH /profiles/{id}` accepts
`{"settings":{...}}`. This replaces all settings. Its `proxy` input omits the
read-only `has_password` and may include a write-only `password`: omission keeps
the saved password only when both server and username are unchanged; otherwise
it clears the old secret. Explicit `""` clears it, and `proxy:null` clears the proxy and password.
Responses contain `has_password` only. The private index
`~/.myco/browser/profiles.json` is written atomically with mode 0600; credentials
are never stored in shared page records or passed on Chrome's command line.
The index currently stores passwords in plaintext inside this private file;
system-keychain storage is a possible later improvement. The loopback forwarder
has no separate authentication: other local processes can use its random port
while the browser session is open.

A configured proxy starts an IPv4-loopback forwarding proxy on a random port
for that browser session. HTTP/HTTPS upstreams receive proxy authentication;
SOCKS5 upstreams use username/password authentication. HTTPS upstreams require a
valid TLS certificate. CONNECT and upgraded connections close with the session.
Chrome receives only the local proxy address and its bypass rules, so bypass
matching follows Chrome's rules (including its default loopback exemptions).
When a proxy is configured, Chrome also receives
`--force-webrtc-ip-handling-policy=disable_non_proxied_udp` to prevent direct
WebRTC UDP traffic from bypassing it. This flag is automatic and cannot be
supplied through the user argument allowlist. No CDP Fetch interception is installed. Custom UA uses `--user-agent`; language
uses `--lang` and `--accept-lang`. A custom UA can disagree with Chrome's version
and client hints; retaining the default is recommended.

Allowed launch arguments match the application editor:

- `--start-maximized`, `--disable-gpu`, `--disable-background-timer-throttling`,
  `--disable-backgrounding-occluded-windows`, `--disable-renderer-backgrounding`.
- `--window-size=W,H` (2–5 decimal digits per dimension),
  `--force-device-scale-factor=0..4` (up to two decimals below 4).
- `--disable-features=` / `--enable-features=` with comma-separated names
  containing only ASCII letters, digits and underscores.
- `--blink-settings=imagesEnabled=true` or `false`.

Other flags, malformed proxy addresses and control characters return
`invalid_argument`. Proxy addresses must be `http://host:port`,
`https://host:port` or `socks5://host:port`, without embedded credentials, paths,
queries or fragments. Username/password are separate fields. Language is an
optional language tag such as `en-US` or `zh-CN`; a UA is at most 512 printable
ASCII characters. A profile can save settings while running without restarting
or changing that session. Close its browser before starting a session with the
new settings. Updating settings does not revoke script approvals; settings
fingerprints and approval-change indicators are reserved for a later change.

## Recording inside Chrome

Every headed profile browser opened by myco has a small recorder at the top
right of its top-level pages. Headless sessions never inject panel code. While
a run or native debugging action owns the session, the entire panel host is
removed before execution and restored after release, so it cannot intercept
playback clicks, screenshots or hit testing. Drag its header to move it or collapse it. It follows the
browser's language (Chinese/English) and light/dark appearance. Its controls
run in the isolated collector world inside a closed shadow root, outside the
website's script context. Frames collect actions but never display another panel.
The panel's controls are excluded from recorded actions.

Choose **Start recording** on the current page, interact with the site, then
**Stop recording**. Page-started recordings use the same panel. The list shows
actions in recording order, with type, name and target; deleting a step asks
for confirmation inside the panel. Hovering highlights top-level elements;
iframe contents are represented by their frame boundary. A brief status line
shows the latest recorded action.
**Extract data** offers text, HTML or attribute selection: clicking an element is intercepted rather
than activating it or recording a click. Choose text, an attribute (with its
name), or HTML and save an `extract` node with `what`, `attr` when needed, and
`into` (default `data1`, `data2`, ...). Right-click a top-level element for the same extraction choices without recording
the context-menu click. Attribute names are entered inside the panel, never in
a website prompt. These are suggested output names only;
the application may rename them to avoid existing script outputs. The component
does not generate references to these names in other recorded steps. Esc cancels selection. Frames retain their
locator paths. Password values are never shown in the panel's step summaries.

The **Region screenshot** control is shown only when `status.features` includes
`screenshot_clip`. Drag a rectangle in the top-level viewport; Esc cancels. The
panel, menus and highlights are hidden during selection, and these gestures never
become click, drag or key steps. A completed selection adds
`{type:"screenshot",clip:{x,y,width,height}}`. Coordinates are finite, nonnegative
CSS viewport pixels with positive dimensions; `target` and `full` cannot coexist
with `clip`. Playback validates viewport bounds and converts CSS coordinates to
the current document offset; device scaling affects image resolution, not the
saved rectangle. A changed layout does not make the region follow an element.

Clip results are private PNG artifacts containing only the chosen pixels. No
full-page thumbnail or DOM snapshot is attached, including on capture failure.
Secret-bearing runs and sensitive pages refuse region screenshots. The native
host negotiates `screenshot_clip` before posting a run to a legacy component;
unsupported clips return `unsupported_node`, never a full-page screenshot.

Protocol 1 adds optional `myco.browser.record.active()` on newer hosts. It calls
component `GET /status` and returns its scope-filtered `recordings` list:

```json
[{"id":"rec_example","profile":"p_work","url":"https://example.com/","started_at":"2026-11-25T00:00:00Z","origin":"browser"}]
```

`origin` is `browser` for panel starts or `page` for `record.start`. Unclaimed
browser recordings remain discoverable even after they stop. The first valid
`record.events(id, after)` request claims a recording and removes it from browser
discovery; known IDs remain readable, including replay from `after=0`. Page-origin
recordings can appear while running. Scope is the existing workspace/page/instance
session scope; another scope cannot discover or read these recordings. Buffers
keep the existing limits (128 added steps, 48 KiB, 256 recordings per component).

`record.events` now returns `{steps, removed, next, done}`. `next` is an opaque,
monotonic cursor that also advances for deletions. Remove the IDs in `removed`
from the script, then continue with `next`. Already-deleted steps that were not
sent before the supplied cursor are omitted from `steps`; they need no tombstone.
Deletion never renumbers or shrinks cursors; retry from the last saved cursor
to replay pending changes. Ending from either surface
flushes pending inputs and returns `done:true` once the buffer is drained. Older
components may omit `recordings`/`removed`; newer hosts return an empty active
list for components that do not advertise recordings.

### Recorded scrolling

The recorder captures page scrolling only after a trusted wheel gesture, page-navigation
key, or scrollbar drag on that scroll container. It waits for a 400 ms quiet interval
after other recording actions. Focus, typing, clicks, layout changes, and programmatic
scrolling do not independently create scroll steps. Recorder-panel controls and their
scrolling are excluded. Scroll positions remain absolute replay destinations.

### Browser process groups and recovery (Unix)

Each managed browser starts in its own process group. A private launcher writes a
0600 record in that profile before executing the locally selected Chrome/Edge;
no arbitrary executable is accepted. Records bind the profile ID, generation, component
identity, and process start fingerprints, never browser credentials. Copied
profile records cannot authorize closing another profile's browser. Member
snapshots refresh every ten seconds and immediately before shutdown.

Shutdown sends SIGTERM to the verified group, waits up to five seconds, then
uses SIGKILL if needed. The profile lease and record remain until the kernel
reports that the entire group is gone. Linux adopts and reaps orphaned helpers.
Windows continues to use its existing job object.

After a component crash, the App invokes the same component's private recovery
entry point. The next component startup also checks records, taking each profile
lease before cleanup. Empty groups are discarded; surviving groups require a
matching process fingerprint or an exact `--user-data-dir` argument for that
profile. An unverified group is never signalled. A conflicting live lease is
left alone. Profiles with unresolved records cannot be opened with an older
component that lacks the `process_groups: 1` startup capability.

Private entry points `--launch-browser` and `--cleanup-browser-groups` accept
bounded JSON only on stdin; they are not page, Hub, or agent actions. The latter
accepts `data_dir` and an optional `owner_pid` filter. These are implementation
interfaces used by the App, not general-purpose process management commands.

Component errors may add a safe `reason` to `profile_locked`:
`leftover_unverified`, `cleanup_pending`, or `cleanup_failed`. The App preserves
this optional field as `error.reason` on the page bridge. Close the profile's
previous browser and retry, or restart the computer. Error responses never
expose process IDs, filesystem paths, or command lines.

### Removing a local profile

On App/component versions exposing `profiles.remove` and `profile_delete`, the
Profile dialog offers deletion of an idle profile. The trusted App confirms
which profile and warns that sign-ins, local downloads and local script
approvals in every workspace are removed permanently. Close its browser window
and finish any recording, run or agent control first. Older Apps/components
keep the dialog usable without showing deletion.

The dedicated agent profile is shared on the computer and reused when another
workspace connects; its grants and approved scripts remain separate for each
workspace/account. Older duplicate dedicated profiles are disabled as action
targets and can be removed after their browser windows are closed.

A URL wait recorded after a click keeps a distinguishing routing query (such
as `tab=tech`) when available, while omitting authentication/tracking parameters.
For `busy` with `owner_window_open`, close the profile's browser window before
asking an agent to open control. Live grants with a host restriction name the
allowed sites in the approval card.

Deleting the last dedicated agent profile also disables its automatic recreation
on this App configuration. Create a new personal profile and explicitly allow
agent access if you want to use browser actions again.
