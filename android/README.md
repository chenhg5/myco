# myco for Android

Native Kotlin / Jetpack Compose phone client, Android 8.0 (API 26) and later.
This client is under development; it does **not yet have full iOS feature parity**.
The iOS app and `docs/api.md` are the behavior and protocol references.

## Build

Install JDK 17 and the Android SDK (`platform-tools`, `platforms;android-36`,
`build-tools;35.0.0`). Set `ANDROID_HOME` or put `sdk.dir` in an untracked
`local.properties`, then:

```sh
cd android
./gradlew :core:conversation:test :core:network:test :app:lintDebug :app:assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

All dependency versions are pinned in `gradle/libs.versions.toml`. The Gradle
wrapper checks its distribution checksum. There is no dependency on the desktop
Web UI or Go runtime in the APK. An offline demo is available on the welcome page.
It never connects to a real workspace.

On Linux, `tools/setup-sdk-linux.sh /absolute/path/to/android-sdk` installs
the required SDK using Google’s command-line archive with a pinned checksum.
CI uses the same helper and does not assume a preinstalled SDK.

Open **Settings → My devices → Add another device** on an existing myco client,
then paste its link into **Join workspace**. Each workspace has its own device
credential, conversation cache and drafts. HTTP is supported for self-hosted
workspaces; use HTTPS when accessing a workspace over the internet.

See [workspace page links](../docs/android-links.md) for opening pages from a
conversation and passing a fragment to an app.

## Tests

```sh
# Unit tests: task/held/answered state, pending sends, network cancellation,
# error decoding, file messages, credential isolation and remote revocation.
./gradlew :core:conversation:test :core:network:test

# Real Hub contract test, in an isolated temporary workspace. Needs a myco
# binary on PATH (or --binary /absolute/path/to/myco). No personal config is used.
python3 tools/hub-fixture.py -- ./gradlew :core:network:test

# Native UI journeys on a connected Android device or emulator (English locale).
./gradlew :app:connectedDebugAndroidTest
```

The real Hub test pairs a device, sends a file and a message, acknowledges the
task, adds a correction, replies, downloads the file, and hides/restores a
message. Its credentials and database are temporary and are never printed.
Without the fixture, this one integration test is explicitly skipped.
Native tests cover multiline sending/background drafts, a readable join failure,
and a real WebView bridge with credential-free networking and cookie isolation
between pages. CI builds the APK and runs these checks on an API 35 emulator.

A real sharing host is still needed to validate remote video and input. A mocked
signaling response only tests consent and lifecycle rules. Physical devices are
also needed for speech, keyboard, photo formats, notification delivery and
performance acceptance; an emulator alone cannot establish those results.

## Modules

```
app/                 Android entry point and composition root; offline demo
core/model/          Hub contract and replaceable transport/storage ports (JVM)
core/conversation/   Immutable state, task rules, outbox and synchronization (JVM)
core/network/        HTTP, page networking and remote-session lifecycle (JVM)
core/storage/        Device Keystore + encrypted workspace/draft/cache storage
core/ui/             Theme, shared native components and localization
feature/chat/        Conversation, composer, files, steps and export
feature/pages/       Sandboxed WebView and scoped native page bridge
feature/workspace/   Workspace navigation, projects, members and workbench
```

The core conversation module has no Android, HTTP or persistence implementation
dependency. Its ports can be replaced independently. UI pages render state and
send actions; message delivery decisions belong in `core/conversation`.

A send is saved before it reaches the network. An interrupted or ambiguous send
is restored for the person to inspect; it is never automatically retried. This
avoids creating a second task after an uncertain network result. Cached messages,
drafts and workspace credentials are encrypted with a non-exportable device key.

Page resources are fetched by the native client and served through a synthetic
origin with a content policy. A main-frame-only bridge provides page state,
records, storage, preferences, bounded credential-free network requests and
controller-only remote signaling. A page never receives the workspace token.
Sending as the person and connecting to a computer require native consent.
Backgrounding or closing revokes remote sessions and cancels page requests.

## Current coverage

| Area | Status |
| --- | --- |
| Pairing, multiple workspaces, switching and leaving | Implemented, including device-local workspace names; device management still to follow |
| Chat, Markdown, multiline input, send/steer/stop, queued edits | Implemented; conversation presentation still being aligned with iOS |
| Drafts, cached messages, interrupted sends | Implemented |
| Files | Picker, upload, download/open, queued attachment reuse; camera and image normalization/preview still to follow |
| Projects and members | Browse and converse; model/effort settings; full management still to follow |
| Automations and workflows | Initial list/actions/editor; workflow inputs, runs and decisions still to follow |
| Apps and pages | App library, page creation, scoped bridge and native document saving; pins and additional lifecycle tests still to follow |
| Remote Desktop | Signaling/consent/revocation implemented; real-host video/input acceptance pending |
| Speech input and voice calls | Pending |
| Notifications, quiet hours and off-screen workspace refresh | Pending |
| Appearance/language controls | System, light/dark and Chinese/English choices persist on this device and apply to embedded pages |
| Proxy settings and chat backgrounds | Pending; system proxy settings are used |
| E2E messages, QR scan and server push | Not implemented in the iOS reference either; sealed messages remain unreadable |

## Packages

`assembleDebug` produces an installable debug APK at the path above. It is a
preview, not a signed public release. `assembleRelease` / `bundleRelease` build
optimized release artifacts; release signing is deliberately supplied externally
and no signing key belongs in this repository. Keep an APK's signing identity
stable before distributing it, so later builds can update it in place.

Do not change the repository `VERSION` to publish Android previews.

## Public media

Pages can check `myco.features.includes("net-media")` and use
`myco.media.url(publicHttpUrl)` as an audio or video source. Android streams through
its system proxy settings with Range support, without changing the page CSP or
sharing workspace credentials, cookies or browser headers. Initial hosts, every
redirect and addresses used for direct connections must be public. HTML and other
nonmedia responses are refused. Closing or backgrounding the page cancels active
streams; returning to the page requires starting playback again.

See [the shared media contract](../docs/app-os.md#playing-public-audio-and-video).
Playback formats depend on Android System WebView and the device's codecs. A new
APK is required to add this native capability to an older Android installation.

## Appearance and local workspace names

Open **Appearance and language** on the welcome page, or **Settings** inside a
workspace. Choose System, Light or Dark and System, 中文 or English. Choices take
effect immediately and survive restarts. Embedded pages receive the resolved
preferences before their scripts run and receive later changes without reloading
the document. User content is not translated.

**Settings → Rename this workspace** changes only this device's name for it.
The workspace address, selected workspace and sign-in stay the same. Other
members' workspace names are unaffected. A failed save remains visible and can
be retried.

## Saving files from apps

`myco.download(name, content, type)` opens Android's **Save as** document picker.
Choose the destination there; a page cannot supply a local path or destination
URI. Documents, pictures, audio and video can be up to 20 MiB. Programs and
unknown file extensions are refused. Large desktop media-export capability
(`download-media`) is not advertised by this Android bridge.

The promise resolves to `true` only after writing completes, or `false` when the
picker is cancelled; a failure rejects and is also shown in the native page.
Closing the page cancels a pending export. The one save request survives the
Activity pause caused by the system picker; network and remote requests continue
to cancel when the page leaves the foreground. Typed-array views export only
the selected bytes, and text is UTF-8. No broad storage permission is requested.
