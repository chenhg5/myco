---
name: myco-preview
description: Build a preview of the Mac app on the maintainer's Mac and open it there as myco-preview, beside the released app. Use when a maintainer asks for a Mac preview ("打个 mac 预览包", "预览版", "在 Mac 上看看"), in the chenhg5/myco repository. Not a release: VERSION stays as it is.
---

# A Mac preview (myco-preview)

A preview is the latest main built **on the maintainer's Mac** into `~/Applications/myco-preview.app` and opened there, next to the released `/Applications/myco.app`. It takes a few minutes. It is not a release: nothing is published and `VERSION` is not touched.

If you were asked through myco: `myco message ack` first, then `myco message progress` while it builds, and `myco message reply` when it is open.

## Do not

- **Do not replace `/Applications/myco.app`.** That is the released app the maintainer uses every day; the preview is a separate app, `myco-preview`.
- **Do not send a DMG through myco** (`--attach` a 45 MB file, then `myco file get` on the Mac): it crawls and a 60-second exec times out. Build on the Mac instead.
- Do not use GitHub's sign-only release run for this: it is slower (10+ minutes) and gets to the Mac only as a download.
- Do not print tokens or the signing identity; do not change the Mac's proxy or network settings (the script uses the local proxy as it is).

## Steps

1. **Push** what should be in it to `main` (after `make vet`, `make test`; for `web/` changes `make web` and the UI checks in AGENTS.md). The Mac builds `origin/main`.
2. **Start the build on the Mac** through its exec node `mac-exec`, in the background (an exec stops after 60 seconds):

   ```bash
   myco exec --to mac-exec --cmd '(nohup ~/code/.myco-preview.sh > ~/code/.myco-preview.log 2>&1 &); echo started'
   ```

   `~/code/.myco-preview.sh` is the Mac's copy of `scripts/mac-preview.sh` (it builds in `~/code/myco-app-build`). If it is missing or older than the repo's, put the repo's there first:

   ```bash
   myco exec --to mac-exec --cmd "echo $(base64 -w0 scripts/mac-preview.sh) | base64 -D > ~/code/.myco-preview.sh && chmod +x ~/code/.myco-preview.sh && echo ok"
   ```

3. **Wait** (a few minutes), checking every 15–20 seconds:

   ```bash
   myco exec --to mac-exec --cmd 'tail -3 ~/code/.myco-preview.log; pgrep -f myco-preview.sh >/dev/null && echo RUNNING || echo FINISHED'
   ```

   Done when the log ends with `PREVIEW-OK` (the line before says `built bin/myco.app (…)`); the second line of the log is the commit it built. Anything else: read the log's last lines and fix or report them.
4. **Check** it is open, and that the released app is still there:

   ```bash
   myco exec --to mac-exec --cmd 'pgrep -lf "myco-preview.app/Contents/MacOS|/Applications/myco.app/Contents/MacOS"; defaults read /Applications/myco.app/Contents/Info.plist CFBundleShortVersionString'
   ```

5. **Reply**: the commit it built, what to try in it, and that it runs beside the released app.

## Good to know

- Both apps register the same global shortcuts (⌘⇧Space for the superagent's quick panel). The one opened first keeps them, so to try a shortcut in the preview, quit the released myco and reopen myco-preview. Say so when the change is about a shortcut.
- Both apps have the same bundle id, so never quit or open one by name (`osascript -e 'quit app "myco-preview"'` quits the released app too): stop the preview by its path (`pkill -f ~/Applications/myco-preview.app/Contents/MacOS`), as the script does.
- Both apps share `~/.myco` and the keychain: what the preview changes in settings, the released app sees too.
- The preview is signed only on that Mac (ad hoc), not notarized; it is for that Mac only. For a build other people install, release (`myco-release`).
- To see what the preview does (the App writes no log file of its own), start it with its log kept: `MYCO_PREVIEW_LOG=~/code/myco-preview-app.log ~/code/.myco-preview.sh`, then read that file through `mac-exec`.
- If `mac-exec` is offline, ask the maintainer to wake and unlock the Mac.
