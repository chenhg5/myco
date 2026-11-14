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
/// replaces the microphone with a script, and `-myco-latency <ms>` makes
/// the demo answer as slowly as a bad connection.
@main
struct MycoApp: App {
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
            speech: Self.speech)
        let scenarios: [String: Scenario] = ["demo": .team, "empty": .empty, "long": .long, "waiting": .waiting]
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

    private static func demo(_ scenario: Scenario) -> WorkspaceSession {
        // `-myco-latency <ms>`: every answer of the demo takes this long,
        // to see what loading looks like on a slow connection.
        let latency = Duration.milliseconds(UserDefaults.standard.integer(forKey: "myco-latency"))
        let session = WorkspaceSession(id: AppModel.demoID, name: "演示工作区", hub: FakeHub(scenario: scenario, latency: latency))
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
        }
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
