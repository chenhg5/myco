import SwiftUI
import MycoCore
import MycoFake
import MycoFeatures

/// The app's entry: the only place where the pieces are put together.
/// Until pairing exists, the workspace on screen is the demo one (an
/// in-memory hub); `-myco-start welcome` shows the welcome page instead,
/// `-myco-start empty` a workspace with nobody in it, and `-myco-start
/// long` one with a very long conversation.
@main
struct MycoApp: App {
    @State private var model: AppModel

    init() {
        let start = UserDefaults.standard.string(forKey: "myco-start") ?? "demo"
        func demo(_ scenario: Scenario) -> WorkspaceSession {
            WorkspaceSession(id: "demo", name: "演示工作区", hub: FakeHub(scenario: scenario))
        }
        let scenarios: [String: Scenario] = ["empty": .empty, "long": .long]
        let session: WorkspaceSession? = start == "welcome" ? nil : demo(scenarios[start] ?? .team)
        _model = State(initialValue: AppModel(session: session, demo: { demo(.team) }))
    }

    var body: some Scene {
        WindowGroup {
            AppRootView(model: model, initialRoute: UserDefaults.standard.string(forKey: "myco-route"))
        }
    }
}
