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
/// `-myco-start demo|empty|long` shows a demo workspace (an in-memory
/// hub), `-myco-start welcome` the welcome page, `-myco-join <link>`
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
        let workspaces = Workspaces(records: FileWorkspaceStore(), secrets: KeychainStore(), deviceName: Self.deviceName())
        let snapshots = FileSnapshotStore()
        let session: @MainActor @Sendable ((record: WorkspaceRecord, hub: HubClient)) -> WorkspaceSession = { current in
            WorkspaceSession(id: current.record.id, name: Workspaces.displayName(of: current.record), hub: current.hub, snapshots: snapshots)
        }
        let doors = AppModel.Doors(
            demo: { Self.demo(.team) },
            restore: {
                guard let current = await workspaces.current() else { return nil }
                return session(current)
            },
            join: { link in
                do {
                    _ = try await workspaces.join(link: link)
                } catch Workspaces.JoinError.notALink {
                    throw AppModel.JoinFailure(message: L("这不是添加设备的链接。它应该像 https://你的工作区/c/myco_pair_…", "This is not an \"add a device\" link. It looks like https://your-workspace/c/myco_pair_…"))
                } catch Workspaces.JoinError.refused(let reason) {
                    throw AppModel.JoinFailure(message: L("工作区没有接受这个链接：\(reason)", "The workspace did not accept this link: \(reason)"))
                }
                guard let current = await workspaces.current() else {
                    throw AppModel.JoinFailure(message: L("加入了，但没能保存在这台设备上。", "Joined, but it could not be saved on this device."))
                }
                return session(current)
            },
            leave: { id in
                await workspaces.leave(id)
                await snapshots.removeSnapshots(of: id)
                // What its messages carried goes with it.
                FileShelf.forget(id)
            },
            speech: Self.speech)
        let scenarios: [String: Scenario] = ["demo": .team, "empty": .empty, "long": .long]
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
