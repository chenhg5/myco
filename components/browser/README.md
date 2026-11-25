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

The component adds `--disable-blink-features=AutomationControlled` to its existing
Chrome launch flags. Chrome supplies its own user agent. The collector stays in
an isolated world and does not install a stealth script.

Run `go test -race ./...` and `go vet ./...` in this directory. On a machine with
an installed Chrome or Edge, enable the existing native fixture flag:

```sh
go test -race -timeout 5m -run TestNativeChrome -args -browser-smoke
```

`TestNativeChromeNestedIframeRecording` checks a cross-site iframe, its nested
iframe and workers, and trusted input capture in both iframe levels. These
fixtures only use local pages and fresh dedicated profiles.

The desktop pins component 0.1.1 and also accepts an already installed 0.1.0
until the owner chooses **Update component**. A successful update verifies and
installs 0.1.1 before stopping the old process; the next browser operation starts
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
