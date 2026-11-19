# myco for iOS

The phone app: follow and direct your agents. Native SwiftUI, iOS 17 and later, iPhone in portrait. No third-party dependencies.

Status: the pages and the conversation, and joining a real workspace with a link (read and send). Checked in a simulator only, against a demo workspace and a throwaway local hub. Pictures and files go with a message and show in the ones that carry them; the microphone turns speech into text. Several workspaces can be joined at once. New messages are announced by local notifications (late while the app is not open). Not built yet: scanning a code, end-to-end encryption (sealed messages and their files show as encrypted), push through Apple's service.

## Layout

```
Myco/                 the app target: only the entry point (puts the pieces together)
MycoAppUITests/       walks the key paths in a simulator, on the demo workspace
MycoKit/              a local Swift package with everything else
  Sources/
    MycoAPI/          the hub's JSON contract (mirrors internal/api) and the HubAPI protocol
    MycoCrypto/       end-to-end encryption envelopes
    MycoStore/        keychain, drafts, the list of workspaces
    MycoCore/         run state, the conversation and its input, sync, naming rules
    MycoFake/         an in-memory workspace behind HubAPI, for previews, tests and the demo
    MycoUI/           reusable views: plain values in, no network, no storage
    MycoFeatures/     pages and navigation (Router)
  Tests/
project.yml           the Xcode project's source (XcodeGen); Myco.xcodeproj is generated and committed
design/app-icon.svg   the app icon's source: the logo's ink mushroom on cream, on a full square (iOS cuts the corners)
scripts/              gen-look-fixtures.mjs: avatar fixtures made from web/src/app/persona.tsx;
                      testflight.sh: archives the app and uploads it to TestFlight
```

Dependencies point down only: `MycoFeatures` → `MycoUI`, `MycoCore` → `MycoAPI`, `MycoCrypto`, `MycoStore`. A page reads its model and draws; what a task's state is (`RunState`), what a member is up to (`MemberState`) and what the send button does (`ComposerAction`) are each worked out in one place in `MycoCore`. Navigation state lives only in `Router`.

## Commands

On a Mac with Xcode 26 or later:

```bash
cd ios/MycoKit && xcodebuild test -scheme MycoKit-Package -destination 'platform=iOS Simulator,name=iPhone 17'   # unit tests
cd ios && xcodebuild test -project Myco.xcodeproj -scheme Myco -destination 'platform=iOS Simulator,name=iPhone 17'   # UI tests
cd ios && xcodegen generate            # after changing project.yml or adding a file to Myco/ or MycoAppUITests/; commit the result
node ios/scripts/gen-look-fixtures.mjs # after changing how faces are drawn in web/src/app/persona.tsx
```

## Releasing to TestFlight

What is installed from Xcode is a development build: it runs only on devices registered in the developer account. Other people get the app through TestFlight.

Once, in the developer account: register the bundle id (`PRODUCT_BUNDLE_IDENTIFIER` in `project.yml`) as an explicit App ID, and create the app in App Store Connect with it. The name there must be one no other app has; the name under the icon is `CFBundleDisplayName` and stays "myco".

Then, for every build:

```bash
TEAM=<Apple team id> ios/scripts/testflight.sh            # archive (Release), export, upload
TEAM=<Apple team id> UPLOAD=0 ios/scripts/testflight.sh   # archive only: does a release build work?
```

The upload signs in as the account Xcode is signed in to on the Mac, or with an App Store Connect API key (`ASC_KEY_PATH`, `ASC_KEY_ID`, `ASC_ISSUER_ID`). Xcode's account must have App Store Connect access for the team ("Failed to Use Accounts" says it has not). A key must have the Admin role: the export asks Apple for the team's distribution certificate (cloud signing), which an App Manager key may not ("Cloud signing permission error", then "No profiles for … were found"). The build number is the time (UTC) unless `BUILD` is given, since it must grow with every upload; the version is the project's `MARKETING_VERSION` unless `VERSION` is given (a new version means a new beta review for outside testers, so it changes only on purpose). After some minutes the build shows in TestFlight: testers who are members of the team can install it at once; outside testers need the first build of a version to pass beta review. Outside testers join through the external group "Public" and its public link (https://testflight.apple.com/join/XDKvU8Jp, in the READMEs' download table); a new build reaches them once it is added to that group (and, for a new version, has passed beta review).

What an upload is checked for is in the project: the privacy manifest (`Myco/PrivacyInfo.xcprivacy`: no tracking, nothing collected, `UserDefaults` for the app's own settings), and the export compliance answer (`ITSAppUsesNonExemptEncryption` is false: the app encrypts nothing itself, it uses the system's HTTPS and keychain). Both must be kept true: change them the day the app collects something or encrypts messages itself. The privacy policy TestFlight and the App Store link to is [docs/privacy.md](../docs/privacy.md) (its GitHub address); keep it true in the same way.

## Trying it

The app opens the workspace joined earlier, or the welcome page: join with a link from the desktop app (Settings → Members → Add a device), or look around the demo workspace. Launch arguments pick what to show:

| Argument | Values |
|---|---|
| `-myco-start` | nothing: the workspace joined earlier, else the welcome page. `demo`, `empty`, `long` (the demo plus a 400-message conversation), `waiting` (the demo plus runs that are open but not at work, and no automations), `session` (the demo plus an agent in one long session, a message that was never delivered, a note that links to a web page, and an earlier handover): a demo workspace. `two`: two demo workspaces joined side by side (any well-formed link joins a third). `welcome`: the welcome page |
| `-myco-join` | an "add a device" link to join with at launch |
| `-myco-route` | `tab:<chats\|projects\|members>`, `project:<name>`, `info:<project>`, `agent:<name>`, `member:<name>`, `sheet:<workspace\|add\|computer>`, `page:automations` |
| `-myco-attach` | an agent's name: the demo starts with a picture and a file picked for that agent, to see the input with files without the system's pickers |
| `-myco-dictation` | `scripted`: the microphone hears a script (nobody speaks to a simulator); `denied`: it is not allowed to listen |
| `-myco-proxy-target` | where Settings › Proxy's "Try it" sends its request (default: a workspace that is not on the local network, else GitHub); the proxy UI test sets it. That test needs `ios/scripts/test-proxies.py` running and is skipped without (see the script) |
| `-myco-voice` | `scripted`: a voice chat is a script (the microphone hears a question and a request, a scripted model answers, the voice is silent), with no key and nobody speaking; `denied`: the microphone may not open; `probe`: opens a call's real microphone and speaker alone for a few seconds, plays a short tone, and writes what happened to the system's log (subsystem `dev.myco.ios`, category `voice`; never what was said), to see on a phone why a call would not start |
| `-myco-trouble` | `links`: the demo's first join link fails as a gateway in front of the workspace would (502), to see what a failed request says |
| `-myco-latency` | milliseconds every answer of the demo takes: a slow connection, to see what loading looks like |
| `-myco-notifier` | `recorded`: notifications are kept in memory and the system is never asked for permission |
| `-myco-appearance` | `system`, `light`, `dark` |
| `-myco-lang` | `system`, `zh`, `en` |

In the demo, what you send is taken on after a moment and answered about nine seconds later, so adding to a running task and stopping it can be tried by hand.

## Pictures and files

The input's plus button offers the photo library, the camera and Files. What is picked is uploaded at once (`POST /v1/console/files`, as the desktop app does) and waits above the words as a tile; it is sent with them, or alone. A picture goes as something every agent reads: a PNG, JPEG, GIF or WebP of sensible size as it is, anything else (a phone's HEIC, a very large photo) as a JPEG no longer than 3072 pixels (`Pictures`). A file picked in Files goes as it is, up to 50 MB.

Messages show their pictures as thumbnails and other files as rows; a tap opens the system's preview (Quick Look), which zooms, shares and saves. Files are fetched once and kept in the caches folder (`FileShelf`), and forgotten when the workspace is left.

## The input

One card, two rows: the words get the whole width, and the controls sit under them. What adds to a message is on the left (the plus for pictures and files, the microphone); the one button that sends it, adds it to the running task, or stops the run is alone at the lower right, under the thumb. Once voice is set up, with nothing typed and nothing running that button starts a voice chat with the agent (below), as on the desktop. The microphone and Send are a whole card apart, so one is never hit for the other, and the microphone stays while there are words, so speaking can go on where it stopped (an input that swaps the microphone for Send, as Messages does, cannot do that). Every control is at least 44 points to the finger. A tap anywhere else on the card is a tap in the text.

Return breaks the line. What is said to an agent often takes several lines, sending wakes it or interrupts what it is doing, and a phone's keyboard has no Shift+Return, so sending stays on a button; "Return sends" in Settings is there for those who want a chat app's way, off by default. With a keyboard attached, ⌘Return sends.

## Speaking instead of typing

The microphone is in the input's lower left, beside the plus. It listens with Apple's speech recognition (`AppleSpeechListener`), on the phone when the language allows, in the language chosen for the app, and the words grow into the box after what was typed. A tap stops it; so does typing or sending, and nothing more is written after that. A pause does not end it: the recognizer ends its session at each pause, and the next one starts at once on the running microphone, its words after what was said (`Utterances`, the desktop's rule in `internal/console/dictation.go`); six silent sessions in a row end the listening. The system asks for the microphone and for speech recognition the first time; without them the button turns amber and a line says where to allow them.

## A voice chat

Talking with an agent out loud, in its conversation: the desktop app's voice call ([docs/design/voice.md](../docs/design/voice.md)), with the same rules. Once it is set up in Settings, with nothing typed and nothing running the input's round button (a waveform) starts it; the call is a bar docked above the input with how it stands (listening, thinking, speaking, muted), the last few lines of the talk, mute and hang up. The conversation scrolls and the input types as always.

- **What is said goes to the agent as if typed.** A fast model hears the person and passes on what they want with `tell_agent`: added to the agent's open task, else a new one, with no confirmation (`ConversationModel.tell`, the rule of the send button). What it passes on ends with the desktop's trailer (`ViaVoice`), which the page shows as a small "By voice" mark; a tap shows the words as heard. Questions about progress it answers itself from the conversation (`VoiceContext`); `stop_agent` stops the open task, `check_agent` reads the conversation again.
- **It can be cut off.** Real words while it talks stop it at once (not an "嗯", not its own voice coming back); what the person heard of its answer is kept as said, marked as cut off.
- **News is told in a pause.** When the agent answers or asks something, the voice says the gist once nobody has made a sound for a second and a half. A task typed in during the call is watched like one spoken.

A phone has no local backend, so the loop the desktop runs in Go (`internal/console/voice_call.go`) runs in the app (`VoiceCall`), in three parts behind protocols:

| Part | In the app | In the demo and tests |
|---|---|---|
| Hearing and playing (`VoiceAudio`) | `AppleVoiceAudio`: one audio engine with the system's voice processing (echo cancellation, noise suppression), Apple's speech recognition on the phone when the language allows, one session after another; a pause of 0.9 s ends what is said | `ScriptedVoiceAudio` |
| Thinking (`VoiceModel`) | `OpenAIChat`: any OpenAI-compatible chat API, streamed, with tools; the desktop's prompt | `ScriptedVoiceModel` |
| Speaking (`VoiceSpeech`) | `DoubaoSpeech` (the desktop's API and default voice); with no Doubao key, `SystemSpeech`: the phone's own voice, played through the same engine | `SilentSpeech` |

Settings › Voice chat takes the model's address, name and key, and the speech service: one of `SpeechService.all` (Doubao today; another is added to that list and nowhere else), with its key, voice and speed. On this phone the keys are in the keychain (`voice:llm`, and `voice:tts` or `voice:tts:<service>`) and are never shown again.

The settings are the person's, not the phone's (`VoiceSharing`): unless "Keep on the workspaces" is turned off, they are kept, keys and all, on every workspace this device joined (`GET`/`PUT /v1/console/voice`), so what was set in the desktop app is here without typing it again, and the other way round. The newest change wins, by the time the person made it. The app looks at the workspaces when it starts, when the settings page opens, and before a call that is not set up; a change goes out when the page is left. The page says where the settings are kept, which workspace's hub is too old to keep them (they stay on this device then), and which could not be reached. The keys are handed only to the person's own devices, but they are in the hub's database as they are: whoever administers the hub's machine can read them, which the page says. Until the model and its key are set there is no voice button: the round button is Send, as always (the desktop's rule; there Doubao's key is needed too).

The call lives in the app (`AppModel.ongoing`), not in the page it was started from: going to another chat, a project or the settings does not hang up. In its own chat it is the bar above the input; anywhere else a strip across the top says how it goes and with whom (`CallStrip`: a tap goes back to that chat, also in another workspace), mutes and hangs up. There is one call at a time: starting one with another agent hangs up the call before first (its button says so), and the microphone for dictation is out of the way meanwhile.

What differs from the desktop: the call hangs up when the app goes to the background, or when a phone call or another app takes the microphone (the screen stays on during a call); and the talk's lines are not kept after the call.

## A conversation, as the desktop app shows it

The rules are the desktop's (`web/src/app/ConversationDetail.tsx`, `TraceView.tsx`); when one changes there, it changes here.

- What is added to a running task is a plain bubble: no label.
- A message put behind another and taken back before its turn (edited or withdrawn) was never sent: it is not in the conversation. Editing one puts its words and its files back in the box; the files are not uploaded again.
- A task the agent set aside (held) does not take what is typed: that is a new task, unless it was set aside to ask a question.
- A task reads as a message, not as a state (the desktop's rule, `docs/design/app-ui.md`). A run that is open but not at work (its turn ended, it handed work out, it was set aside) gets no line saying so: what it said last stands, as in any chat. A task of mine that has not started has a few words under the message instead, like a receipt (`TaskReceipt`): "Not opened yet", "Queued behind the current one", "Offline; it reads it when back", "Its session is busy elsewhere", or, set aside, "Will answer later"; gone once it starts. Only a run really at work shows the mushroom; one that may be stuck still has its own line, because it carries something to do (Stop). And when the run has stopped or the task was set aside, the input is a plain message box again ("Message X", no stop button): what is typed is a new message, until the agent is at work again; one that asks me something still takes the answer (`ConversationBuilder.openTask`).
- Thinking is part of the steps ("thought, ran 2 commands"), there on a tap.
- A long press on a message offers Delete: it leaves this person's view on all their devices (`/v1/console/hidden`; on this device only with a workspace before v0.19.9), with Undo for a few seconds. Nothing is deleted for anyone else.

- A task that failed says why in words (`FailureWords`): one the workspace gave up delivering reads "没送到…", with "重新发送" under it (the same words and files, as a new task; the failed one stays).
- An agent in one long session files its steps under the task it took first. The page goes by time (`RunWindows`): a task shows its steps up to my next message that was taken up, and that message shows what came since.
- Where another session took the agent over, a thin line says so (`HandoverWords.line`).
- A message an automation sent is marked 自动.
- A link to a page of this workspace has a card under its message and opens in the app (below).

## What else the desktop app has, here too

Each follows the desktop app's page of the same name; `docs/design/app-ui.md` says how the phone's differs.

- **The model beside the send button** (`RunChoice`): what the agent runs with, small and quiet; a menu to switch model and effort for an agent a runner wakes that I may manage (`PUT /v1/agents/{name}/run-settings`), a note saying who decides for any other.
- **A request that failed** (`Failure`, `FailureLine`): a workspace that is down or slow is said in words, with what to do, "重试" where the page can try again, and the raw reason (HTTP status, code, message) one tap away with a copy button.
- **Export** (`ConversationExport`, a conversation's "…"): the whole conversation as Markdown, in the shape `myco thread export` writes, with or without each task's tool calls, handed to the share sheet.
- **Automations** (`AutomationsScreen`, the first row of Members): the list with a switch each, run now (swipe right), delete (swipe left), and an editor that reads as an automation does: to whom, what, when (`Every` is the schedule as people say it and the cron the workspace keeps). A webhook's secret is shown once, after saving.
- **Add a computer** (`AddComputerSheet`, the plus): one command to run on that computer, in the folder agents should use; the page waits for the folder and puts it in the project chosen.
- **Web pages agents publish** (`PageCard`, `PageScreen`): the page's files are fetched by the app with its own token and handed to a web view through a scheme of its own (`myco-page://<page>/…`), each file with the desktop's content policy: a page loads only its own files, sends nothing elsewhere, and never sees the token. The page bridge and live reload are not here yet.
- **Hand over to another session** (`HandoverSheet`, at the foot of an agent's card): a new session on a computer, or a one-time command for any session; what will happen is said first; the card lists the handovers, each with a way back where there is one.

## Several workspaces

A link adds a workspace; the ones joined before stay (`Workspaces`, behind `WorkspaceBook`). Each keeps its own token in the keychain and its own cache; joining one already joined signs this device in again in the same row. The app opens on the one that was on screen last. The workspace panel (the badge at the top left) lists them: a tap switches, a swipe or a long press renames or leaves one, and leaving asks first. The ones off screen are looked at once a minute and when the panel opens: something new there is a blue dot on the badge and on its row. A workspace is called what its address says (the name after `/w/` on a hub that serves several, else its host; an address reads "My workspace"), numbered when two would read the same, until the person renames it; the name is only for this device.

## Notifications

Local ones, for now: the phone has to learn of a message itself, so they are prompt while the app is open and late while it is not. A message from the other side of a conversation is announced once (`NotificationDesk`): an answer, a failure, a question from an agent, something it says on its own; interim updates only on request; never the conversation on screen while the app is in front, never a chat the person muted (a long press on it), never during quiet hours. A tap leads to the conversation, switching workspace if it must. The icon's number is the count of conversations with something unread. While the app is not in front, the system gives it a moment now and then (`BGAppRefreshTask`, `MycoApp.refreshTask`) to look at every workspace; when is the system's choice, minutes to hours.

Push that arrives the moment a task is done needs Apple's push service: the hub has to know the phone's push token and send through APNs. That is not built.

## Loading

Nothing pops in. A list that waits for its first answer shows rows in its shape (`SkeletonRows`), breathing, and the list fades in over them. A conversation is laid out and scrolled to where it opens before it is shown, then fades in whole; one that takes longer than a moment shows `ConversationSkeleton` meanwhile, and one kept on the device opens before that. Its parts (what was asked, how it was worked on, the agent's other work) are asked for together and shown together; only after 1.5 seconds is the part that is here shown first. What comes once a page is open (a new message, an older page) arrives with `Arrival`: it fades in and rises a little, and the page glides to follow it.

## Staying fresh

An open conversation asks for its messages every two seconds and shows them as soon as they come. How the tasks were worked on (their steps) and what surrounds the conversation (the agent's work with colleagues) are fetched alongside, one round at a time, and shown when they come: one slow answer among them never keeps back a message that is already here (`ConversationModel.load`). The lists ask every five seconds. What is asked again and again is given up after 12 seconds, since the next asking is seconds away.

A look that fails is a blip: the lists, like a conversation, say the workspace cannot be reached only when the next look fails too, when the person pulled to refresh, or when nothing was ever shown (`WorkspaceSession.refresh`). A look may go 20 seconds without a byte before it is given up (`HubClient.pollTimeout`): through a slow proxy a look takes several seconds and stalls now and then for more than ten. With a proxy set here, a request that cannot get through makes its session again, so the next opens a new tunnel; each such failure is in the system's log (subsystem `dev.myco.ios`, category `network`: the error code, and whether it went through the proxy).

Leaving the app stops the asking; coming back starts it over at once, for the lists and for the conversation on screen alike, so the page never waits for an answer that was cut off when the app left the front.

The person can ask too: pulling a list down (the system's refresh; it runs in a task of its own, since SwiftUI gives up a `refreshable` task as soon as the list is drawn again, which its first answer does, and the spinner then never showed), or pulling a conversation up past its end (new messages come at the end, so that is where one pulls): a spinner turns in the room under the last message while the conversation and the lists are read again. When the workspace does not answer twice in a row, or when a refresh that was asked for fails, a line across the top of the conversation says that what is shown may not be the latest, as the lists do.

## Rules

- Everything tappable answers on the press (`RowButtonStyle`, `PressScaleStyle`), bars are translucent layers that content scrolls under, and motion is quick, bounce-free and off under Reduce Motion.
- The tab bar is the app's own (`MycoTabBar`): one row across, on each tab's first page. Faces carry no AI mark on the phone.
- Follow `docs/design/app-ui.md` for words, colours and states: the same state has the same colour and the same words as on the desktop.
- Text is `L("中文", "English")`, next to its use, as in `web/`.
- An agent's name (`Naming`) and face (`MushroomLook`) are ports of `web/src/app/nodes.ts` and `persona.tsx`. Change them together.
