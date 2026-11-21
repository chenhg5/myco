import BackgroundTasks
import SwiftUI
import UIKit
import MycoAPI
import MycoCore
import MycoFake
import MycoFeatures
import MycoStore

/// The app's entry: the only place where the pieces are put together.
///
/// It opens the workspace joined earlier, or the welcome page. Launch
/// arguments change that, for tests and for trying things:
/// `-myco-start demo|empty|long|waiting` shows a demo workspace (an
/// in-memory hub), `-myco-start two` two of them joined side by side,
/// `-myco-start welcome` the welcome page, `-myco-join <link>`
/// joins with a link at launch, `-myco-attach <agent>` starts the demo
/// with files picked for that agent, `-myco-dictation scripted|denied`
/// replaces the microphone with a script, `-myco-latency <ms>` makes the
/// demo answer as slowly as a bad connection, `-myco-notifier
/// recorded` keeps notifications from reaching the system, `-myco-trouble
/// links` makes the demo's first join link fail as a gateway would, and
/// `-myco-voice scripted|denied` makes a voice call a script (a microphone
/// that hears two sentences, a model that answers them, a silent voice);
/// `-myco-voice probe` opens a call's real microphone and speaker alone
/// for a few seconds and says in the system's log what happened.
@main
struct MycoApp: App {
    @UIApplicationDelegateAdaptor(MycoOrientationDelegate.self) private var orientationDelegate
    @State private var model: AppModel
    private let start: String

    init() {
        start = UserDefaults.standard.string(forKey: "myco-start") ?? ""
        let snapshots = FileSnapshotStore()
        // `-myco-start two`: two demo workspaces already joined (and any
        // well-formed link joins another), to see and test switching.
        let demos: DemoWorkspaces? = start == "two" ? DemoWorkspaces() : nil
        let joined: any WorkspaceBook
        if let demos {
            joined = demos
        } else {
            joined = Workspaces(records: FileWorkspaceStore(), secrets: KeychainStore(), deviceName: Self.deviceName())
        }
        let kept = demos == nil
        let doors = AppModel.Doors(
            demo: { Self.demo(.team) },
            book: joined,
            session: { record, hub in
                // What a real workspace showed last is kept for its next opening.
                WorkspaceSession(id: record.id, name: Workspaces.displayName(of: record), hub: hub, snapshots: kept ? snapshots : nil)
            },
            forget: { id in
                await snapshots.removeSnapshots(of: id)
                // What its messages carried goes with it.
                FileShelf.forget(id)
            },
            speech: Self.speech,
            // `-myco-notifier recorded`: nothing is shown and the system
            // is not asked, to see the settings in a test.
            notifier: {
                if UserDefaults.standard.string(forKey: "myco-notifier") == "recorded" { return RecordingNotifier() }
                return SystemNotifier()
            },
            leftFront: { Self.askForBackgroundRefresh() },
            // The voice's keys live in the keychain, beside the device tokens.
            secrets: KeychainStore(),
            voice: Self.voice)
        let scenarios: [String: Scenario] = ["demo": .team, "empty": .empty, "long": .long, "waiting": .waiting, "session": .session, "remote": .remote]
        _model = State(initialValue: AppModel(session: scenarios[start].map(Self.demo), doors: doors))
    }

    /// What hears speech: Apple's recognition, or with `-myco-dictation
    /// scripted|denied` a script, to see and test the microphone where
    /// there is nobody to speak.
    private static let speech: @MainActor @Sendable () -> any SpeechListener = {
        switch UserDefaults.standard.string(forKey: "myco-dictation") {
        case "scripted":
            return ScriptedSpeechListener(["帮", "帮我看", "帮我看一下下载页", "然后", "然后改一下标题"], pause: .milliseconds(450))
        case "denied":
            return ScriptedSpeechListener([], problem: .microphoneDenied)
        default:
            return AppleSpeechListener()
        }
    }

    /// What a voice call is made of: nil is the real thing (the phone's
    /// microphone, the model and the voice set in Settings). With
    /// `-myco-voice scripted|denied` a script, to see and test a call
    /// where there is nobody to speak and no key.
    private static let voice: (@MainActor @Sendable () -> AppModel.VoiceStandIn)? = {
        switch UserDefaults.standard.string(forKey: "myco-voice") {
        case "scripted":
            return { AppModel.VoiceStandIn(audio: ScriptedVoiceAudio.demo(), model: ScriptedVoiceModel.demo(), speech: SilentSpeech()) }
        case "denied":
            return {
                AppModel.VoiceStandIn(
                    audio: ScriptedVoiceAudio(problem: .microphoneDenied), model: ScriptedVoiceModel.demo(), speech: SilentSpeech())
            }
        default:
            return nil
        }
    }()

    private static func demo(_ scenario: Scenario) -> WorkspaceSession {
        // `-myco-latency <ms>`: every answer of the demo takes this long,
        // to see what loading looks like on a slow connection.
        let latency = Duration.milliseconds(UserDefaults.standard.integer(forKey: "myco-latency"))
        let hub = FakeHub(scenario: scenario, latency: latency)
        // (the demo's superagent asks for something once it is named)
        Task { await hub.setDemoApprovals(true) }
        // `-myco-trouble links`: the demo's first join link fails as a
        // gateway in front of the workspace would, to see what is said.
        if UserDefaults.standard.string(forKey: "myco-trouble") == "links" {
            Task { await hub.setJoinLinkFailures(1) }
        }
        let session = WorkspaceSession(id: AppModel.demoID, name: "演示工作区", hub: hub)
        // `-myco-attach <agent>`: the demo starts with a picture and a
        // file picked for that agent, to see and test the input with files
        // without going through the system's pickers.
        if let agent = UserDefaults.standard.string(forKey: "myco-attach"), !agent.isEmpty {
            let conversation = session.conversation(with: agent)
            Task {
                await conversation.attach(SamplePicture.png(hue: 0.45, width: 800, height: 1100), name: "照片-101500.png", contentType: "image/png")
                await conversation.attach(Data("# 需求\n\n- 发图片\n- 发文件\n".utf8), name: "需求说明.md", contentType: "text/markdown")
            }
        }
        return session
    }

    var body: some Scene {
        WindowGroup {
            AppRootView(
                model: model, initialRoute: UserDefaults.standard.string(forKey: "myco-route"),
                restoring: start != "welcome", joinLink: UserDefaults.standard.string(forKey: "myco-join"))
                .task {
                    // `-myco-voice probe`: a call's microphone and speaker
                    // alone, for a few seconds, into the system's log.
                    if UserDefaults.standard.string(forKey: "myco-voice") == "probe" { await VoiceProbe.run() }
                }
        }
        // Now and then the system gives the app a moment while it is not in
        // front: it looks at every workspace and announces what is new.
        .backgroundTask(.appRefresh(Self.refreshTask)) { [model] in
            await Self.askForBackgroundRefresh()
            await model.refreshInBackground()
        }
    }

    /// The background refresh's name (also in Info.plist).
    static let refreshTask = "dev.myco.ios.refresh"

    /// Asks the system for a moment in the background, a quarter of an
    /// hour from now at the earliest. When it comes is the system's choice.
    @MainActor
    static func askForBackgroundRefresh() {
        let request = BGAppRefreshTaskRequest(identifier: refreshTask)
        request.earliestBeginDate = Date(timeIntervalSinceNow: 15 * 60)
        try? BGTaskScheduler.shared.submit(request)
    }

    /// This device's name in the member's device list: "iphone-17-pro".
    private static func deviceName() -> String {
        let name = UIDevice.current.name.lowercased()
        let slug = name.map { $0.isASCII && ($0.isLetter || $0.isNumber) ? $0 : "-" }
        let joined = String(slug).split(separator: "-").joined(separator: "-")
        return joined.isEmpty ? "iphone" : joined
    }
}

/// Two demo workspaces, joined: for seeing and testing what the app does
/// with more than one (switching, the dot when something new comes in the
/// other, joining a third, renaming, leaving) without any hub. A few
/// seconds after launch something new arrives in the second one.
actor DemoWorkspaces: WorkspaceBook {
    private var records: [WorkspaceRecord]
    private var hubs: [String: FakeHub]
    private var chosen: String?

    init() {
        let team = WorkspaceRecord(id: "demo-team", name: "团队", url: URL(string: "https://team.myco.example")!)
        let side = WorkspaceRecord(id: "demo-side", name: "个人项目", url: URL(string: "https://side.myco.example")!)
        let sideHub = FakeHub(scenario: .waiting)
        records = [team, side]
        hubs = [team.id: FakeHub(scenario: .team), side.id: sideHub]
        Task {
            try? await Task.sleep(for: .seconds(5))
            await sideHub.deliver(from: "studio-claude-c41f", body: "录音收到了，我开始整理第二场会议。")
        }
    }

    func all() -> [WorkspaceRecord] { records }

    func selected() -> String? {
        chosen.flatMap { id in records.contains { $0.id == id } ? id : nil } ?? records.first?.id
    }

    func select(_ id: String?) { chosen = id }

    func join(link: String) throws -> WorkspaceRecord {
        guard let link = AppLink(link) else { throw JoinError.notALink }
        if let known = records.first(where: { $0.url == link.hub }) { return known }
        let record = WorkspaceRecord(id: "demo-\(records.count + 1)", name: link.hub.host() ?? "workspace", url: link.hub)
        records.append(record)
        hubs[record.id] = FakeHub(scenario: .empty)
        return record
    }

    func hub(of id: String) -> (any HubAPI)? { hubs[id] }

    func rename(_ id: String, to name: String) {
        guard let index = records.firstIndex(where: { $0.id == id }) else { return }
        records[index].name = name
    }

    func leave(_ id: String) {
        records.removeAll { $0.id == id }
        hubs[id] = nil
        if chosen == id { chosen = nil }
    }
}

@MainActor
final class MycoOrientationDelegate: NSObject, UIApplicationDelegate {
    func application(_ application: UIApplication, supportedInterfaceOrientationsFor window: UIWindow?) -> UIInterfaceOrientationMask {
        RemotePresentation.orientations
    }
}
