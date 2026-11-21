# Windows Remote Desktop preview

This is an unsigned, experimental test build from PR #38, not a production
release. Obtain it only from the myco repository's preview workflow. Do not
replace installed executables or disable Windows security protections to run it.

1. Extract the entire preview archive into its own folder. Do not run executables
   inside the ZIP or copy them over the installed App.
2. Double-click `Start-preview.cmd`. Keep `myco.exe`, `myco-app.exe` and the
   `component` folder together. Use the launcher, not the App executable directly.
3. Join the isolated preview Hub supplied by the project coordinator. The current
   public Hub does not contain these preview API endpoints. Do not create agents,
   install services, update the App or change your normal workspace for this test.
4. On the hosting computer, open Apps → Remote Desktop → Share this computer.
   Start with view-only. The preview component is already included; no public
   component release or installer download is needed. Read the App's consent
   dialog and enable sharing. On Mac, explicitly grant Screen Recording;
   Accessibility is needed only when enabling keyboard/mouse control.
5. On the other computer, open the same app in the same preview workspace,
   select the host and enter its displayed temporary pairing code. First check
   the picture, then enable control on the host and reconnect with control.
6. Check typing, scrolling, double-click, dragging and screen edges. Stop sharing
   and verify the controller loses its picture and control. Closing the hosting
   page must also end capture. Never test on a sensitive screen.
7. Use `Stop-preview.cmd` to quit only the executable in this preview folder.
   It never stops an App by name alone. The component process is contained by
   the parent App and ends when the preview quits.

The launcher uses `%LOCALAPPDATA%\myco-remote-preview\desktop\profile` as a
separate home, config and browser-data directory, clears inherited `MYCO_*`
settings, and puts the bundled CLI first on this child process's PATH. Your
normal profile and services are not imported. Windows credentials for the new
preview Hub are kept in Windows Credential Manager under that Hub's distinct ID.
Preview settings persist so reopening the launcher keeps the test workspace.

The first version needs direct connectivity: use the same LAN (or an existing
Tailscale network). A custom TURN relay has not been implemented yet. There is
no sound, locked-desktop/UAC control or unattended service in this preview.
A second test over different public networks will be required when the relay
is added; passing a LAN test does not demonstrate public-internet fallback.

Report which computer was hosting, OS, App version, view-only/control, network,
what failed, and a screenshot with personal information hidden. Share no pairing
code, login link or token in a public issue.
