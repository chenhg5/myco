# myco for iOS

The phone app: follow and direct your agents. Native SwiftUI, iOS 17 and later, iPhone in portrait. No third-party dependencies.

Status: phase 1 of the plan (skeleton on a demo workspace). It does not talk to a real hub yet: pairing, sending to a real workspace, end-to-end encryption and push come in later phases.

## Layout

```
Myco/                 the app target: only the entry point (puts the pieces together)
MycoAppUITests/       walks the key paths in a simulator, on the demo workspace
MycoKit/              a local Swift package with everything else
  Sources/
    MycoAPI/          the hub's JSON contract (mirrors internal/api) and the HubAPI protocol
    MycoCrypto/       end-to-end encryption envelopes
    MycoStore/        keychain, drafts, the list of workspaces (protocols; in-memory for now)
    MycoCore/         run state, the conversation and its input, sync, naming rules
    MycoFake/         an in-memory workspace behind HubAPI, for previews, tests and the demo
    MycoUI/           reusable views: plain values in, no network, no storage
    MycoFeatures/     pages and navigation (Router)
  Tests/
project.yml           the Xcode project's source (XcodeGen); Myco.xcodeproj is generated and committed
scripts/              gen-look-fixtures.mjs: avatar fixtures made from web/src/app/persona.tsx
```

Dependencies point down only: `MycoFeatures` → `MycoUI`, `MycoCore` → `MycoAPI`, `MycoCrypto`, `MycoStore`. A page reads its model and draws; what a task's state is (`RunState`), what a member is up to (`MemberState`) and what the send button does (`ComposerAction`) are each worked out in one place in `MycoCore`. Navigation state lives only in `Router`.

## Commands

On a Mac with Xcode 26 or later:

```bash
cd ios/MycoKit && xcodebuild test -scheme MycoKit-Package -destination 'platform=iOS Simulator,name=iPhone 17'   # unit tests
cd ios && xcodebuild test -project Myco.xcodeproj -scheme Myco -destination 'platform=iOS Simulator,name=iPhone 17'   # UI tests
cd ios && xcodegen generate            # after changing project.yml; commit the result
node ios/scripts/gen-look-fixtures.mjs # after changing how faces are drawn in web/src/app/persona.tsx
```

## Trying it

The app opens on a demo workspace. Launch arguments pick what to show:

| Argument | Values |
|---|---|
| `-myco-start` | `demo` (default), `welcome`, `empty` |
| `-myco-route` | `tab:<chats\|projects\|members>`, `project:<name>`, `info:<project>`, `agent:<name>`, `member:<name>`, `sheet:<workspace\|add>` |
| `-myco-appearance` | `system`, `light`, `dark` |
| `-myco-lang` | `system`, `zh`, `en` |

In the demo, what you send is taken on after a moment and answered about nine seconds later, so adding to a running task and stopping it can be tried by hand.

## Rules

- Follow `docs/design/app-ui.md` for words, colours and states: the same state has the same colour and the same words as on the desktop.
- Text is `L("中文", "English")`, next to its use, as in `web/`.
- An agent's name (`Naming`) and face (`MushroomLook`) are ports of `web/src/app/nodes.ts` and `persona.tsx`. Change them together.
