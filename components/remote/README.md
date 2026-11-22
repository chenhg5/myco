# Optional Remote Desktop component

This separate Go module keeps Pion, capture, encoding and input outside the main
myco binary. The App only imports `internal/remotedesktop`: its protocol, small
component manager and short-lived signaling broker. An App-only host reuses its paired console device identity; no CLI installation,
machine node or runner service is needed. No component is downloaded,
started or registered for incoming connections until the person opts in.

## First version

- Apple silicon/Intel Mac and x64 Windows unlocked desktops; H.264 video up to 720p30, keyboard and mouse.
- Direct WebRTC on a LAN or over Tailscale. ICE has no external STUN/TURN servers
  by default; this version does not promise public-internet NAT traversal.
- No audio, clipboard synchronization, unattended service, virtual display,
  lock-screen access, Windows UAC/secure-desktop control or automatic public relay
  in this version.
- Video and input use WebRTC DTLS/SRTP/SCTP; Hub HTTPS carries signaling only.
  The Hub is trusted for signaling. This does not provide independently verified
  identity against a malicious Hub.
- One session per host. View-only by default. A fresh eight-hex-character pairing
  code is generated on every sharing start; five failures pause pairing for a
  minute. Only people who own/manage the computer may signal through its Hub.
- Closing the hosting app/page, disabling sharing or losing the Hub heartbeat
  stops the process. The parent contains it in a process group / Windows job,
  including its encoder. Inputs are released on blur and disconnect.

## Architecture

The parent sends one JSON line on stdin, containing a fresh local RPC secret,
pairing code and whether control was approved. The component responds once on
its private stdout pipe with `{address:"127.0.0.1:<port>",protocol:1}`. It exposes
only authenticated loopback `/status`, `POST /sessions/<id>`,
`DELETE /sessions/<id>` and the agent calls below; browser Origins are refused. No Hub token enters the
component. EOF on stdin shuts it down. Its stdout is never copied to user logs.

For agent actions (`remote.desktop.observe` and `.input`, see `docs/api.md`),
the App also calls `POST /agent/observe` `{max_width, format}` → `{frame_id,
captured_at, display_id, width, height, media_type, image}` (one still frame of
the display input maps to, scaled down only; 1440 wide by default), and `POST /agent/input`
`{frame_id, events}` → `{applied}`. Input needs control, names a frame observed
in the last 30 seconds of the display as it still is (same origin and size), takes 1–64 events checked all together before any
runs, applies them one by one, stops when the request ends (the App drops it
when the grant is revoked) and releases every key and button afterwards. While a person's remote control session is connected, agent input is refused
(`computer_busy`): one controller per computer. A
refusal is `{error, applied?}`. `/status` lists `agent_actions` from 0.3.0.
The App checks each job's grant with its Hub; the component trusts the App.

An offer/answer uses `{type,sdp}`. H.264 is sent through Pion; the adjacent FFmpeg
captures the display and encodes with VideoToolbox (Mac) or Media Foundation
(Windows). The input data channel accepts only validated normalized pointer
coordinates, bounded scroll amounts and allowlisted physical key codes. It
cannot execute commands or read arbitrary files.

Mac capture uses AVFoundation in this initial encoder adapter. A future
ScreenCaptureKit backend can replace it without changing the App or Hub API.
Mac requires Screen Recording and, for control, Accessibility consent. The
component asks for Accessibility only after the person enables control; it never
changes system settings itself. Some system shortcuts remain local to the App.

## Build and test

```
cd components/remote
go test -race ./...
go vet ./...
go build -trimpath -ldflags='-s -w' -o myco-remote .
```

`main_test.go` connects two actual Pion peers, checks video RTP delivery and
keyboard input, and covers pairing, rate limiting and view-only permissions.
This exercises transport, not real desktop capture or end-to-end display latency.
Actual Mac/Windows captures require interactive machines with system permissions.
The native package jobs additionally encode synthetic NV12 with the packaged
platform encoder and check that browser-required SPS/PPS/IDR are sent in-band.
An opt-in local `go test -run TestBrowserDecodesVideo -args -browser-smoke` also
checks actual H.264 pixels decoded by Chromium (requires system FFmpeg with
libx264, Node, Playwright, and `UI_SMOKE_EXECUTABLE` when its default browser is
not installed). It never captures a real screen.

Run the FFmpeg build script from the repository root.
`.github/workflows/remote-component.yml` tests the module and builds packages for
Mac arm64/x64 and Windows x64. `scripts/build-ffmpeg.sh` pins FFmpeg 7.1.2 source,
uses only built-in platform encoders and includes its LGPL license and source
reference. No GPL codec or full third-party desktop shell is bundled.

Before publishing, sign the Mac executables, notarize their distribution and run
interactive Mac/Windows acceptance checks. The workflow uploads build artifacts
only, never releases automatically. A maintainer publishes the reviewed archives
as `remote-v0.2.0` (a prerelease, never latest) with one combined `SHA256SUMS`.
The App downloads the matching archive from the myco repository over HTTPS,
checks its SHA-256, validates every archive entry and installs atomically into
`~/.myco/components/remote/0.2.0`. Release archives contain only `myco-remote`,
`ffmpeg` (`.exe` on Windows) and LICENSE/NOTICE text files. Until these assets are
published, the UI reports installation unavailable; it does not silently download
an arbitrary external program or fall back to an insecure connection.

## Interactive acceptance before distributing a build

On both Mac and Windows, verify screen capture, pointer mapping at display edges,
scrolling, double-click, dragging, modified keys and reconnecting. Test both
view-only and control, including refusal before permission is granted. On Mac,
grant Screen Recording and Accessibility explicitly and retry; no prompt should
claim these permissions were granted automatically. Check that closing the page,
quitting/crashing the parent, stopping sharing and losing the Hub all terminate
capture and release held input. Use LAN and Tailscale direct connections; a failed
route must report a failure rather than falling back to an unencrypted stream.

## Controller keyboard capabilities

Protocol 1 remains compatible. On opening an approved control channel, a new
component sends `{"type":"capabilities","input_features":["text"]}`. Controllers
that recognize it may send `{"kind":"text","text":"…"}` (up to 256 Unicode
scalars / 1024 UTF-8 bytes, no control characters). Text is entered using native
Unicode keyboard events, never the clipboard. Older components omit this event;
controllers keep physical key support and explain when non-ASCII text needs a
component update. View-only sessions have no input channel.
