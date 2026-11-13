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
/// hub), `-myco-start welcome` the welcome page, and `-myco-join <link>`
/// joins with a link at launch.
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
            })
        let scenarios: [String: Scenario] = ["demo": .team, "empty": .empty, "long": .long]
        _model = State(initialValue: AppModel(session: scenarios[start].map(Self.demo), doors: doors))
    }

    private static func demo(_ scenario: Scenario) -> WorkspaceSession {
        WorkspaceSession(id: AppModel.demoID, name: "演示工作区", hub: FakeHub(scenario: scenario))
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
