# Optional browser automation component

The desktop App can record and replay a linear workflow in a dedicated local Chrome profile. The official **Web automation** application lives in [myco-ai/apps](https://github.com/myco-ai/apps). Its manifest declares `permissions.os: [browser]`.

## Install and use

Open Web automation in the Mac or Windows App, then choose **Install browser component**. The App asks for confirmation in its own dialog, downloads the pinned `browser-v0.1.0` release, checks `SHA256SUMS`, and installs it under `~/.myco/components/browser/0.1.0/`. Installation is optional and never happens on a status request. An unpublished component release produces an installation error; it is not replaced with an arbitrary version.

Install Google Chrome yourself. On Windows, Microsoft Edge is used if Chrome is absent. No Chromium download, user-agent override, extension, system service, or everyday browser profile is used. Recording opens a normal visible browser. Log in there yourself when a workflow needs authentication.

Create a profile, record actions, stop recording, and review the resulting sequence before running it. Add a **Confirm** or breakpoint before any submission or other consequential action. Playback executes clicks exactly as recorded: it does not infer that a button submits a form. A full run with `start_url` opens a fresh tab so the previous draft is preserved; **Start here** uses the current tab and its current DOM. Close unused tabs if the 32-tab limit is reached.

## Isolation and local data

- Only generated profiles under `~/.myco/browser/profiles/<id>/` may be opened. The data directory is passed by the App, never by a page. Profile paths and symlinks are checked and profiles have an OS file lock.
- The component receives a random secret through stdin and returns `{address, protocol:1}`. Its HTTP listener binds only IPv4 loopback. Every request needs Bearer authentication and a page/instance scope; browser `Origin` headers and non-loopback Host values are rejected. Hub credentials are not passed to Chrome.
- Closing a page or revoking its browser permission stops its sessions. Closing stdin stops the component; it exits after two idle minutes without sessions. The App owns a private process group/job; only processes it started are terminated. The component never sends `Browser.close` or terminates processes by name.
- Profiles, cookies, passwords, secret parameters, screenshots, structural DOM snapshots, and downloaded files stay on this machine. Shared scripts contain locator metadata and opaque artifact IDs, not artifact bytes or local file paths.
- Password fields record `${password}`, not their value. Accepted prompt input records `${dialog_input}`. Declare those parameters as `secret` and supply them for each run. Secret-bearing runs omit automatic screenshots and redact secret values from returned output. Sensitive recording steps omit screenshots; pages with password/OTP/sensitive fields or inaccessible frames also omit snapshots. Structural DOM evidence contains bounded tag/role/parent relationships, not text, attributes, values or HTML.
- Artifacts are private files, limited to 4 MiB each and 192 MiB/1,024 entries per component store. Where capture succeeds, images have local index references to a page thumbnail and structural DOM evidence. The page bridge retrieves images only. Downloads remain under the browser data directory; run output contains the filename only.
- Upload replay accepts only files previously approved through the host's file picker (`myco.os`), up to 64 MiB. A page-supplied path or private `uploads` allowlist cannot grant access.
- `allowAgents` is a local-owner-confirmed preference only. M0/M1 advertises no agent actions and adds no Hub provider or cross-machine access.

## Page bridge (protocol 1)

Supported Mac arm64/x64 and Windows x64 desktop hosts advertise `myco.features.includes("browser")` so an uninstalled component can be discovered. Check `await myco.browser.status()` for `installed`, `supported`, `reason`, `protocol`, `version`, browser names, machine name and profiles. Missing installations return an empty version and empty browser/profile arrays. Other platforms and the web-only console do not advertise the feature.

The bridge provides `install`, `profiles.create/open/allowAgents`, `record.start/events/stop`, `runs.start/get/control`, `locate`, `pick`, and `snapshot`. `install` and enabling `allowAgents` require a host-owned confirmation, bound to the page instance and action with a one-use, 60-second nonce. The child never receives that nonce. The browser capability also needs the App's per-page, once-per-App consent.

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

Recorded locator lists prefer test IDs, semantic role/name, label/placeholder, text, CSS, then XPath. Open shadow roots and frame chains are retained. The collector runs in an isolated CDP world and uses bindings/events, not Go polling. Renderer navigation is a `wait:url`, popup creation a `tab:popup`, downloads a `wait:download`, and dialogs explicit `dialog:accept/dismiss` nodes. Playback arms the next dialog before the triggering action. Tab indices are 0-based in each run. Navigation URLs omit queries to avoid persisting credentials; review any workflow requiring query parameters and supply an explicitly safe URL yourself.

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

`browser-component.yml` runs Go tests and native Chrome/Edge fixtures on macOS arm64/x64 and Windows x64, and builds three zip archives with dependency notices. `browser-component-release.yml` takes a successful main build run, checks its provenance, signs/notarizes Mac binaries, and stages a **draft** `browser-v*` prerelease with `SHA256SUMS`. Published releases are immutable. A release owner must verify acceptance and publish the draft before installation works. This process does not change the main `VERSION`.
