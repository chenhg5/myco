# myco for iOS

The phone app: follow and direct your agents. Native SwiftUI, iOS 17 and later, iPhone in portrait. No third-party dependencies.

Status: the pages and the conversation, and joining a real workspace with a link (read and send). Checked in a simulator only, against a demo workspace and a throwaway local hub. Not built yet: scanning a code, end-to-end encryption, attachments, push, more than one workspace at a time.

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
| `-myco-start` | nothing: the workspace joined earlier, else the welcome page. `demo`, `empty`, `long` (the demo plus a 400-message conversation): a demo workspace. `welcome`: the welcome page |
| `-myco-join` | an "add a device" link to join with at launch |
| `-myco-route` | `tab:<chats\|projects\|members>`, `project:<name>`, `info:<project>`, `agent:<name>`, `member:<name>`, `sheet:<workspace\|add>` |
| `-myco-appearance` | `system`, `light`, `dark` |
| `-myco-lang` | `system`, `zh`, `en` |

In the demo, what you send is taken on after a moment and answered about nine seconds later, so adding to a running task and stopping it can be tried by hand.

## Rules

- Everything tappable answers on the press (`RowButtonStyle`, `PressScaleStyle`), bars are translucent layers that content scrolls under, and motion is quick, bounce-free and off under Reduce Motion.
- The tab bar is the app's own (`MycoTabBar`): one row across, on each tab's first page. Faces carry no AI mark on the phone.
- Follow `docs/design/app-ui.md` for words, colours and states: the same state has the same colour and the same words as on the desktop.
- Text is `L("中文", "English")`, next to its use, as in `web/`.
- An agent's name (`Naming`) and face (`MushroomLook`) are ports of `web/src/app/nodes.ts` and `persona.tsx`. Change them together.
