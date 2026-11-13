import SwiftUI
import Observation
import MycoCore
import MycoUI

/// The app as a whole: which workspace is on screen, and where in it.
@MainActor
@Observable
public final class AppModel {
    public private(set) var session: WorkspaceSession?
    public let router = Router()

    private let makeDemo: @MainActor () -> WorkspaceSession

    /// - Parameter demo: makes the demo workspace (no hub behind it).
    public init(session: WorkspaceSession? = nil, demo: @escaping @MainActor () -> WorkspaceSession) {
        self.session = session
        self.makeDemo = demo
    }

    public func enterDemo() {
        router.reset()
        session = makeDemo()
    }

    public func leaveWorkspace() {
        router.reset()
        session = nil
    }
}

/// The top of the app: the workspace on screen, or the welcome page.
public struct AppRootView: View {
    let model: AppModel
    /// A place to open at launch (see `Router.open(spec:)`).
    let initialRoute: String?

    @AppStorage("myco-appearance") private var appearance = "system"
    @AppStorage(AppLanguage.defaultsKey) private var language = "system"

    public init(model: AppModel, initialRoute: String? = nil) {
        self.model = model
        self.initialRoute = initialRoute
    }

    public var body: some View {
        Group {
            if let session = model.session {
                RootView().environment(session)
            } else {
                WelcomeScreen()
            }
        }
        .environment(model)
        .environment(model.router)
        .preferredColorScheme(appearance == "dark" ? .dark : appearance == "light" ? .light : nil)
        // Strings are picked when a view is built: rebuild on a new language.
        .id(language)
        .task {
            if let initialRoute { model.router.open(spec: initialRoute) }
        }
    }
}

/// The three tabs, each with its own stack of pages.
struct RootView: View {
    @Environment(Router.self) private var router
    @Environment(WorkspaceSession.self) private var session
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        @Bindable var router = router
        TabView(selection: Binding(get: { router.tab }, set: { router.select($0) })) {
            NavigationStack(path: $router.chatsPath) {
                ChatsScreen().routed()
            }
            .tabItem { Label(L("聊天", "Chats"), systemImage: "bubble.left.and.bubble.right") }
            .tag(AppTab.chats)

            NavigationStack(path: $router.projectsPath) {
                ProjectsScreen().routed()
            }
            .tabItem { Label(L("项目", "Projects"), systemImage: "square.stack.3d.up") }
            .tag(AppTab.projects)

            NavigationStack(path: $router.membersPath) {
                MembersScreen().routed()
            }
            .tabItem { Label(L("成员", "Members"), systemImage: "person.2") }
            .tag(AppTab.members)
        }
        .tint(MycoColor.fg)
        .sheet(item: $router.sheet) { sheet in
            switch sheet {
            case .workspace: WorkspaceSheet()
            case .memberCard(let name): MemberCardSheet(name: name)
            case .addAgent: AddAgentSheet()
            }
        }
        // Refresh only while the app is in front.
        .task(id: scenePhase) {
            if scenePhase == .active { await session.run() }
        }
    }
}

extension View {
    /// The pages a tab's stack can show.
    func routed() -> some View {
        navigationDestination(for: Route.self) { route in
            Group {
                switch route {
                case .project(let name): ProjectScreen(name: name)
                case .agent(let name, _): AgentScreen(name: name)
                case .projectInfo(let name): ProjectInfoScreen(name: name)
                }
            }
            // A pushed page takes the whole screen: no tab bar under its input.
            .toolbar(.hidden, for: .tabBar)
        }
    }
}

/// The current workspace's face at the top left of every tab: tap to switch
/// workspaces, join another, or open settings.
struct WorkspaceButton: View {
    @Environment(Router.self) private var router
    @Environment(WorkspaceSession.self) private var session

    var body: some View {
        Button {
            router.sheet = .workspace
        } label: {
            WorkspaceBadge(name: session.name, size: 28)
        }
        .accessibilityLabel(L("工作区：\(session.name)", "Workspace: \(session.name)"))
        .accessibilityIdentifier("workspace.button")
    }
}

/// A workspace's face: the first letter of its name in a coloured square.
struct WorkspaceBadge: View {
    let name: String
    let size: CGFloat

    var body: some View {
        let look = MushroomLook(seed: name)
        RoundedRectangle(cornerRadius: size * 0.28, style: .continuous)
            .fill(Color(hex: look.capColor))
            .frame(width: size, height: size)
            .overlay {
                Text(name.trimmingCharacters(in: .whitespaces).prefix(1).uppercased())
                    .font(.system(size: size * 0.5, weight: .semibold))
                    .foregroundStyle(.white)
            }
    }
}
