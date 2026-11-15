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
design/app-icon.svg   the app icon's source: the desktop icon's mark, on a full square (iOS cuts the corners)
scripts/              gen-look-fixtures.mjs: avatar fixtures made from web/src/app/persona.tsx
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

## Trying it

The app opens the workspace joined earlier, or the welcome page: join with a link from the desktop app (Settings → Members → Add a device), or look around the demo workspace. Launch arguments pick what to show:

| Argument | Values |
|---|---|
| `-myco-start` | nothing: the workspace joined earlier, else the welcome page. `demo`, `empty`, `long` (the demo plus a 400-message conversation), `waiting` (the demo plus runs that are open but not at work): a demo workspace. `two`: two demo workspaces joined side by side (any well-formed link joins a third). `welcome`: the welcome page |
| `-myco-join` | an "add a device" link to join with at launch |
| `-myco-route` | `tab:<chats\|projects\|members>`, `project:<name>`, `info:<project>`, `agent:<name>`, `member:<name>`, `sheet:<workspace\|add>` |
| `-myco-attach` | an agent's name: the demo starts with a picture and a file picked for that agent, to see the input with files without the system's pickers |
| `-myco-dictation` | `scripted`: the microphone hears a script (nobody speaks to a simulator); `denied`: it is not allowed to listen |
| `-myco-voice` | `scripted`: a voice chat is a script (the microphone hears a question and a request, a scripted model answers, the voice is silent), with no key and nobody speaking; `denied`: the microphone may not open |
| `-myco-latency` | milliseconds every answer of the demo takes: a slow connection, to see what loading looks like |
| `-myco-notifier` | `recorded`: notifications are kept in memory and the system is never asked for permission |
| `-myco-appearance` | `system`, `light`, `dark` |
| `-myco-lang` | `system`, `zh`, `en` |

In the demo, what you send is taken on after a moment and answered about nine seconds later, so adding to a running task and stopping it can be tried by hand.

## Pictures and files

The input's plus button offers the photo library, the camera and Files. What is picked is uploaded at once (`POST /v1/console/files`, as the desktop app does) and waits above the words as a tile; it is sent with them, or alone. A picture goes as something every agent reads: a PNG, JPEG, GIF or WebP of sensible size as it is, anything else (a phone's HEIC, a very large photo) as a JPEG no longer than 3072 pixels (`Pictures`). A file picked in Files goes as it is, up to 50 MB.

Messages show their pictures as thumbnails and other files as rows; a tap opens the system's preview (Quick Look), which zooms, shares and saves. Files are fetched once and kept in the caches folder (`FileShelf`), and forgotten when the workspace is left.

## The input

One card, two rows: the words get the whole width, and the controls sit under them. What adds to a message is on the left (the plus for pictures and files, the microphone); the one button that sends it, adds it to the running task, or stops the run is alone at the lower right, under the thumb. With nothing typed and nothing running, that button starts a voice chat with the agent (below), as on the desktop. The microphone and Send are a whole card apart, so one is never hit for the other, and the microphone stays while there are words, so speaking can go on where it stopped (an input that swaps the microphone for Send, as Messages does, cannot do that). Every control is at least 44 points to the finger. A tap anywhere else on the card is a tap in the text.

Return breaks the line. What is said to an agent often takes several lines, sending wakes it or interrupts what it is doing, and a phone's keyboard has no Shift+Return, so sending stays on a button; "Return sends" in Settings is there for those who want a chat app's way, off by default. With a keyboard attached, ⌘Return sends.

## Speaking instead of typing

The microphone is in the input's lower left, beside the plus. It listens with Apple's speech recognition (`AppleSpeechListener`), on the phone when the language allows, in the language chosen for the app, and the words grow into the box after what was typed. A tap stops it; so does typing or sending, and nothing more is written after that. A pause does not end it: the recognizer ends its session at each pause, and the next one starts at once on the running microphone, its words after what was said (`Utterances`, the desktop's rule in `internal/console/dictation.go`); six silent sessions in a row end the listening. The system asks for the microphone and for speech recognition the first time; without them the button turns amber and a line says where to allow them.

## A voice chat

Talking with an agent out loud, in its conversation: the desktop app's voice call ([docs/design/voice.md](../docs/design/voice.md)), with the same rules. With nothing typed and nothing running, the input's round button (a waveform) starts it; the call is a bar docked above the input with how it stands (listening, thinking, speaking, muted), the last few lines of the talk, mute and hang up. The conversation scrolls and the input types as always.

- **What is said goes to the agent as if typed.** A fast model hears the person and passes on what they want with `tell_agent`: added to the agent's open task, else a new one, with no confirmation (`ConversationModel.tell`, the rule of the send button). What it passes on ends with the desktop's trailer (`ViaVoice`), which the page shows as a small "By voice" mark; a tap shows the words as heard. Questions about progress it answers itself from the conversation (`VoiceContext`); `stop_agent` stops the open task, `check_agent` reads the conversation again.
- **It can be cut off.** Real words while it talks stop it at once (not an "嗯", not its own voice coming back); what the person heard of its answer is kept as said, marked as cut off.
- **News is told in a pause.** When the agent answers or asks something, the voice says the gist once nobody has made a sound for a second and a half. A task typed in during the call is watched like one spoken.

A phone has no local backend, so the loop the desktop runs in Go (`internal/console/voice_call.go`) runs in the app (`VoiceCall`), in three parts behind protocols:

| Part | In the app | In the demo and tests |
|---|---|---|
| Hearing and playing (`VoiceAudio`) | `AppleVoiceAudio`: one audio engine with the system's voice processing (echo cancellation, noise suppression), Apple's speech recognition on the phone when the language allows, one session after another; a pause of 0.9 s ends what is said | `ScriptedVoiceAudio` |
| Thinking (`VoiceModel`) | `OpenAIChat`: any OpenAI-compatible chat API, streamed, with tools; the desktop's prompt | `ScriptedVoiceModel` |
| Speaking (`VoiceSpeech`) | `DoubaoSpeech` (the desktop's API and default voice); with no Doubao key, `SystemSpeech`: the phone's own voice, played through the same engine | `SilentSpeech` |

Settings › Voice chat takes the model's address, name and key, and optionally a Doubao key, voice and speed. Keys go to the keychain (`voice:llm`, `voice:tts`), are never shown again and never reach the workspace; the desktop's keys are in the Mac's keychain and cannot be read from here, so they are typed once more. Until the model is set, the round button opens that page.

What differs from the desktop: the call hangs up when the conversation is left, when the app goes to the background, or when a phone call or another app takes the microphone (the screen stays on during a call); and the talk's lines are not kept after the call.

## A conversation, as the desktop app shows it

The rules are the desktop's (`web/src/app/ConversationDetail.tsx`, `TraceView.tsx`); when one changes there, it changes here.

- What is added to a running task is a plain bubble: no label.
- A message put behind another and taken back before its turn (edited or withdrawn) was never sent: it is not in the conversation. Editing one puts its words and its files back in the box; the files are not uploaded again.
- A task the agent set aside (held) does not take what is typed: that is a new task, unless it was set aside to ask a question.
- A run that is open but not at work does not look at work: its turn ended, it was set aside, or it said something and has been silent for two minutes. The mushroom stands and the line says what it waits for ("Waiting for the tester", "Set aside; it answers later", "Waiting; no answer yet").
- Thinking is part of the steps ("thought, ran 2 commands"), there on a tap.
- A long press on a message offers Delete: it leaves this person's view on all their devices (`/v1/console/hidden`; on this device only with a workspace before v0.19.9), with Undo for a few seconds. Nothing is deleted for anyone else.

## Several workspaces

A link adds a workspace; the ones joined before stay (`Workspaces`, behind `WorkspaceBook`). Each keeps its own token in the keychain and its own cache; joining one already joined signs this device in again in the same row. The app opens on the one that was on screen last. The workspace panel (the badge at the top left) lists them: a tap switches, a swipe or a long press renames or leaves one, and leaving asks first. The ones off screen are looked at once a minute and when the panel opens: something new there is a blue dot on the badge and on its row. A workspace is called what its address says (the name after `/w/` on a hub that serves several, else its host; an address reads "My workspace"), numbered when two would read the same, until the person renames it; the name is only for this device.

## Notifications

Local ones, for now: the phone has to learn of a message itself, so they are prompt while the app is open and late while it is not. A message from the other side of a conversation is announced once (`NotificationDesk`): an answer, a failure, a question from an agent, something it says on its own; interim updates only on request; never the conversation on screen while the app is in front, never a chat the person muted (a long press on it), never during quiet hours. A tap leads to the conversation, switching workspace if it must. The icon's number is the count of conversations with something unread. While the app is not in front, the system gives it a moment now and then (`BGAppRefreshTask`, `MycoApp.refreshTask`) to look at every workspace; when is the system's choice, minutes to hours.

Push that arrives the moment a task is done needs Apple's push service: the hub has to know the phone's push token and send through APNs. That is not built.

## Loading

Nothing pops in. A list that waits for its first answer shows rows in its shape (`SkeletonRows`), breathing, and the list fades in over them. A conversation is laid out and scrolled to where it opens before it is shown, then fades in whole; one that takes longer than a moment shows `ConversationSkeleton` meanwhile, and one kept on the device opens before that. Its parts (what was asked, how it was worked on, the agent's other work) are asked for together and shown together; only after 1.5 seconds is the part that is here shown first. What comes once a page is open (a new message, an older page) arrives with `Arrival`: it fades in and rises a little, and the page glides to follow it.

## Rules

- Everything tappable answers on the press (`RowButtonStyle`, `PressScaleStyle`), bars are translucent layers that content scrolls under, and motion is quick, bounce-free and off under Reduce Motion.
- The tab bar is the app's own (`MycoTabBar`): one row across, on each tab's first page. Faces carry no AI mark on the phone.
- Follow `docs/design/app-ui.md` for words, colours and states: the same state has the same colour and the same words as on the desktop.
- Text is `L("中文", "English")`, next to its use, as in `web/`.
- An agent's name (`Naming`) and face (`MushroomLook`) are ports of `web/src/app/nodes.ts` and `persona.tsx`. Change them together.
