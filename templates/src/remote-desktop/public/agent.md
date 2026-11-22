# Remote Desktop

A private remote desktop viewer/controller for computers in this workspace.
No shared collections. Never save pairing codes, SDP or native
component credentials in page state, records, messages or logs.

The human viewer uses the optional `myco.remote` page bridge. This is not an
agent tool. The App asks the person before installation,
hosting or connecting. Hosting ends when its page closes. Mac and Windows hosts
need the optional component; only unlocked desktops are supported. The first
version uses LAN/Tailscale direct WebRTC (H.264); no audio or public relay.

## Agent workflow

With a compatible CLI, Hub and desktop App, agents use `myco action`, with their own
agent identity. The computer's owner approves an expiring grant in the App.
The owner can allow view only, allow control, deny, or revoke it at any time.
Never use an App credential, pairing code, `myco.os`, exec node or a new page to
bypass an unavailable action or a denied grant.

This needs a registered workspace machine, its running myco desktop App and a
compatible optional component advertising agent actions (0.3.0 or later).
Installation of the currently published older component does not add agent
actions. If `myco action` is unknown, the CLI also needs updating. An App-only computer without a machine registration is not supported
by the first agent-action transport. iPhone can view/control as a human but
cannot host this desktop provider. If actions are absent or `unsupported`, tell
the person which part needs updating; do not install unreleased binaries.

1. Ask for only what the task needs. Use the computer name the person supplied;
   the Hub resolves it to a machine ID. For viewing:

   ```sh
   myco action grant request --machine studio --permission remote.desktop.view \
     --reason "Inspect the reported window" --ttl 5m --wait 30s
   ```

   For a task that needs keyboard/mouse input, request
   `--permission remote.desktop.control` instead (it includes view). Add
   `--task <current-msg-id>` when this is for an open task: it ends the grant
   with the task. Read the returned `state`; a pending request grants no access.
   `myco action grant get --id <grant-id> --wait 30s` waits for its decision.
   The owner may narrow control to view. Check the actual granted permissions.

2. Discover what this target offers:

   ```sh
   myco action list --grant <grant-id>
   ```

   The current provider has `remote.desktop.observe` and, when separately
   approved, `remote.desktop.input`, both version 1. Empty discovery is not
   permission to use a different transport.

3. Observe, then **look at the saved image** before choosing an action:

   ```sh
   myco action run --grant <grant-id> --action remote.desktop.observe \
     --args '{"display_id":"main","max_width":1440,"format":"jpeg"}' \
     --key inspect-before-1 --save before.jpg
   ```

   JSON output has `job.result` with `frame_id`, `captured_at`,
   `display_id`, `width`, `height` and a private `artifact`; `saved` lists local
   image paths. Only `main` is supported now. Coordinates refer to the delivered
   image, not App CSS pixels or the computer's Retina/native resolution.

4. Input uses that observation's `frame_id`. For a point `(px, py)` in the
   delivered image, use `x=px/width`, `y=py/height` in [0,1]. Example normalized
   click (replace both frame and coordinates with ones you actually observed):

   ```sh
   myco action run --grant <grant-id> --action remote.desktop.input \
     --key click-1 --args '{"frame_id":"<observed-frame-id>","events":[{"kind":"button","x":0.5,"y":0.5,"button":0,"clicks":1,"down":true},{"kind":"button","x":0.5,"y":0.5,"button":0,"clicks":1,"down":false}]}'
   ```

   `kind` is required (not `type`). Events are `move` (`x,y`), `button`
   (`x,y,button:0|1|2,down,clicks?`), `key` (`code,down`), `text` (`text`),
   `scroll` (`delta`, bounded ±1200), or `release`. A batch contains 1–64 events
   and is validated before any execute. Text is 1–256 Unicode characters / up to
   1024 UTF-8 bytes without control characters. Use physical key events for
   Enter/Tab. Keep an entire shortcut, drag or key press/release in one batch:
   all held keys/buttons are released when a batch ends. Use `MetaLeft` on Mac
   or `ControlLeft` on Windows when the intended shortcut requires it; release
   modifier keys in reverse order. Text does not read or modify the clipboard.

5. Observe again with a new key and image filename; verify the actual UI result.
   `{applied:n}` acknowledges input injection, not completion of the task.
   A frame is scoped to its grant and expires quickly (30 seconds); a changed
   display or restarted component requires a new observation. Do not reuse
   frames from another grant.

6. End as soon as the task is done:

   ```sh
   myco action grant end --id <grant-id>
   ```

## Failures and stopping

- Pass a stable explicit `--key` for input. Same key/body recovers the original
  job; a changed body under that key is a conflict. The CLI otherwise generates
  a new key: blindly repeating a command can type or click twice.
- Inspect `job.state` and `job.error` even when the CLI itself exits successfully.
  `myco action job --id <job-id> --wait 30s` reads an existing job; do not submit
  a fresh input just because a wait timed out.
- `myco action cancel --id <job-id>` stops queued work or requests that running
  work stop. `cancel_requested` is an intent until the host acknowledges it.
  Cancellation, revocation and `outcome_unknown` do not undo prior input.
  `job.error.applied`, when present, is how many events were applied before a
  failure; missing is unknown, not zero. Observe before deciding the next step.
- `stale_frame` means observe again. `control_not_allowed` means the provider
  cannot inject input (for example missing Accessibility permission). Do not
  retry a denied action through another route. Host restarts or network loss
  require checking the current grant and observing again.
- Screenshots are private, short-lived artifacts; they become inaccessible when
  the grant ends. Keep local task files private, and do not publish them as page
  data or attachments unless the person explicitly requests an export.

Do not start sharing, install system permissions or connect to a computer on the
person's behalf without their explicit request. The App and native OS permissions
remain the authority; this guide cannot approve access.
