import SwiftUI
import Observation
import MycoCore
import MycoUI

/// The app as a whole: which workspace is on screen, and where in it.
@MainActor
@Observable
public final class AppModel {
    public private(set) var session: WorkspaceSession?
    /// False until the app has looked for a workspace joined earlier.
    public private(set) var started = false
    public let router = Router()

    /// What the app needs from outside, put together at its entry point.
    public struct Doors: Sendable {
        /// Makes the demo workspace (no hub behind it).
        public var demo: @MainActor @Sendable () -> WorkspaceSession
        /// The workspace joined earlier, if any.
        public var restore: @MainActor @Sendable () async -> WorkspaceSession?
        /// Joins the workspace a link leads to; throws `JoinFailure`.
        public var join: @MainActor @Sendable (String) async throws -> WorkspaceSession
        /// Leaves a workspace joined with a link, by its id.
        public var leave: @MainActor @Sendable (String) async -> Void

        public init(
            demo: @escaping @MainActor @Sendable () -> WorkspaceSession,
            restore: @escaping @MainActor @Sendable () async -> WorkspaceSession? = { nil },
            join: @escaping @MainActor @Sendable (String) async throws -> WorkspaceSession = { _ in throw JoinFailure(message: "") },
            leave: @escaping @MainActor @Sendable (String) async -> Void = { _ in }
        ) {
            self.demo = demo
            self.restore = restore
            self.join = join
            self.leave = leave
        }
    }

    /// Why joining did not work, in words for the person.
    public struct JoinFailure: Error, Sendable {
        public var message: String

        public init(message: String) {
            self.message = message
        }
    }

    private let doors: Doors
    /// The id the demo workspace goes by: it is never "left" on a hub.
    public static let demoID = "demo"

    /// - Parameter session: a workspace to show at once (the demo, in tests);
    ///   nil looks for the one joined earlier.
    public init(session: WorkspaceSession? = nil, doors: Doors) {
        self.session = session
        self.doors = doors
        started = session != nil
    }

    /// Opens the workspace joined earlier, or leaves the welcome page up.
    public func start(restoring: Bool) async {
        guard !started else { return }
        if restoring { session = await doors.restore() }
        started = true
    }

    public func enterDemo() {
        router.reset()
        session = doors.demo()
    }

    /// Joins the workspace a link leads to and shows it.
    public func join(link: String) async throws {
        let joined = try await doors.join(link)
        router.reset()
        session = joined
    }

    public func leaveWorkspace() async {
        if let session, session.id != Self.demoID { await doors.leave(session.id) }
        router.reset()
        session = nil
    }
}

/// The top of the app: the workspace on screen, or the welcome page.
public struct AppRootView: View {
    let model: AppModel
    /// A place to open at launch (see `Router.open(spec:)`).
    let initialRoute: String?
    /// Whether to open the workspace joined earlier.
    let restoring: Bool
    /// A link to join with at launch (for trying the app from a script).
    let joinLink: String?

    @AppStorage("myco-appearance") private var appearance = "system"
    @AppStorage(AppLanguage.defaultsKey) private var language = "system"

    public init(model: AppModel, initialRoute: String? = nil, restoring: Bool = true, joinLink: String? = nil) {
        self.model = model
        self.initialRoute = initialRoute
        self.restoring = restoring
        self.joinLink = joinLink
    }

    public var body: some View {
        Group {
            if let session = model.session {
                // A new workspace is a new tree: nothing of the last one stays.
                RootView().environment(session).id(session.id)
            } else if model.started {
                WelcomeScreen()
            } else {
                MycoColor.bg.ignoresSafeArea()
            }
        }
        .environment(model)
        .environment(model.router)
        .preferredColorScheme(appearance == "dark" ? .dark : appearance == "light" ? .light : nil)
        // Strings are picked when a view is built: rebuild on a new language.
        .id(language)
        .task {
            await model.start(restoring: restoring)
            if let joinLink, !joinLink.isEmpty { try? await model.join(link: joinLink) }
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
            // The system's tab bar is hidden: each tab's first page carries
            // the app's own, one row across (`AppTabBar`).
            NavigationStack(path: $router.chatsPath) {
                ChatsScreen().routed()
            }
            .tag(AppTab.chats)

            NavigationStack(path: $router.projectsPath) {
                ProjectsScreen().routed()
            }
            .tag(AppTab.projects)

            NavigationStack(path: $router.membersPath) {
                MembersScreen().routed()
            }
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
                case .agent(let name, let focus): AgentScreen(name: name, focus: focus)
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
