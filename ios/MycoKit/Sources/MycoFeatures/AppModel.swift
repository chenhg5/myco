import SwiftUI
import Observation
import MycoCore
import MycoStore
import MycoUI

/// The app as a whole: the workspaces this device joined, which one is on
/// screen, and where in it.
@MainActor
@Observable
public final class AppModel {
    /// The workspace on screen.
    public private(set) var session: WorkspaceSession?
    /// Every workspace joined, in the order they were joined.
    public private(set) var workspaces: [WorkspaceRecord] = []
    /// False until the app has looked for workspaces joined earlier.
    public private(set) var started = false
    public let router = Router()

    /// A session per workspace joined, kept while the app runs: switching
    /// back to one finds it as it was, and the ones off screen are looked
    /// at now and then, to say when something new came there.
    private var sessions: [String: WorkspaceSession] = [:]

    /// What the app needs from outside, put together at its entry point.
    public struct Doors: Sendable {
        /// Makes the demo workspace (no hub behind it).
        public var demo: @MainActor @Sendable () -> WorkspaceSession
        /// The workspaces this device joined; nil when none can be (a preview).
        public var book: (any WorkspaceBook)?
        /// Makes the session of a workspace joined.
        public var session: @MainActor @Sendable (WorkspaceRecord, any HubAPI) -> WorkspaceSession
        /// Forgets what was kept on this device for a workspace that was left.
        public var forget: @MainActor @Sendable (String) async -> Void
        /// Something to hear speech with: the system's recognition, or a
        /// script in tests.
        public var speech: @MainActor @Sendable () -> any SpeechListener

        public init(
            demo: @escaping @MainActor @Sendable () -> WorkspaceSession,
            book: (any WorkspaceBook)? = nil,
            session: @escaping @MainActor @Sendable (WorkspaceRecord, any HubAPI) -> WorkspaceSession = { record, hub in
                WorkspaceSession(id: record.id, name: Workspaces.displayName(of: record), hub: hub)
            },
            forget: @escaping @MainActor @Sendable (String) async -> Void = { _ in },
            speech: @escaping @MainActor @Sendable () -> any SpeechListener = { AppleSpeechListener() }
        ) {
            self.demo = demo
            self.book = book
            self.session = session
            self.forget = forget
            self.speech = speech
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

    /// A listener for one input's microphone.
    public func speechListener() -> any SpeechListener {
        doors.speech()
    }

    /// - Parameter session: a workspace to show at once (the demo, in tests);
    ///   nil looks for the ones joined earlier.
    public init(session: WorkspaceSession? = nil, doors: Doors) {
        self.session = session
        self.doors = doors
        started = session != nil
    }

    /// Opens the workspace that was on screen last, or leaves the welcome
    /// page up.
    public func start(restoring: Bool) async {
        guard !started else { return }
        if restoring { await loadWorkspaces() }
        started = true
    }

    /// Reads the workspaces joined and shows the one chosen last.
    private func loadWorkspaces() async {
        guard let book = doors.book else { return }
        workspaces = await book.all()
        for record in workspaces where sessions[record.id] == nil {
            guard let hub = await book.hub(of: record.id) else { continue }
            sessions[record.id] = doors.session(record, hub)
        }
        // One whose token is gone cannot be opened: it is not listed.
        workspaces = workspaces.filter { sessions[$0.id] != nil }
        let chosen = await book.selected()
        session = chosen.flatMap { sessions[$0] } ?? workspaces.first.flatMap { sessions[$0.id] }
    }

    public func enterDemo() {
        router.reset()
        session = doors.demo()
    }

    /// Joins the workspace a link leads to and shows it. The ones joined
    /// before stay.
    public func join(link: String) async throws {
        guard let book = doors.book else {
            throw JoinFailure(message: L("这里不能加入工作区。", "A workspace cannot be joined here."))
        }
        let record: WorkspaceRecord
        do {
            record = try await book.join(link: link)
        } catch JoinError.notALink {
            throw JoinFailure(message: L("这不是添加设备的链接。它应该像 https://你的工作区/c/myco_pair_…", "This is not an \"add a device\" link. It looks like https://your-workspace/c/myco_pair_…"))
        } catch JoinError.refused(let reason) {
            throw JoinFailure(message: L("工作区没有接受这个链接：\(reason)", "The workspace did not accept this link: \(reason)"))
        }
        guard let hub = await book.hub(of: record.id) else {
            throw JoinFailure(message: L("加入了，但没能保存在这台设备上。", "Joined, but it could not be saved on this device."))
        }
        // Joined again: a new session, with the new sign-in.
        sessions[record.id] = doors.session(record, hub)
        workspaces = await book.all().filter { sessions[$0.id] != nil }
        await show(record.id)
    }

    /// Puts another workspace on screen.
    public func switchTo(_ id: String) async {
        guard id != session?.id, sessions[id] != nil else { return }
        await show(id)
    }

    private func show(_ id: String) async {
        guard let next = sessions[id] else { return }
        router.reset()
        session = next
        await doors.book?.select(id)
    }

    /// Gives a workspace another name on this device.
    public func rename(_ id: String, to name: String) async {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, let book = doors.book else { return }
        await book.rename(id, to: name)
        workspaces = await book.all().filter { sessions[$0.id] != nil }
        sessions[id]?.rename(name)
    }

    /// Leaves a workspace. When it was the one on screen, another takes
    /// its place; with none left, the welcome page.
    public func leave(_ id: String) async {
        if id == Self.demoID {
            router.reset()
            session = workspaces.first.flatMap { sessions[$0.id] }
            return
        }
        await doors.book?.leave(id)
        await doors.forget(id)
        sessions[id] = nil
        workspaces.removeAll { $0.id == id }
        guard session?.id == id else { return }
        router.reset()
        if let next = workspaces.first {
            await show(next.id)
        } else {
            session = nil
        }
    }

    public func leaveWorkspace() async {
        if let session { await leave(session.id) }
    }

    // MARK: The workspaces off screen

    /// What a workspace is called on this device.
    public func name(of record: WorkspaceRecord) -> String {
        sessions[record.id]?.name ?? Workspaces.displayName(of: record)
    }

    /// How many conversations of a workspace hold something unread.
    public func unread(in id: String) -> Int {
        sessions[id]?.unreadCount ?? 0
    }

    /// Whether a workspace could be reached the last time it was asked.
    public func problem(in id: String) -> String? {
        sessions[id]?.problem
    }

    /// Something new came in a workspace that is not on screen.
    public var unreadElsewhere: Bool {
        workspaces.contains { $0.id != session?.id && unread(in: $0.id) > 0 }
    }

    /// How often the workspaces off screen are looked at.
    public static let othersInterval: Duration = .seconds(60)

    /// Looks at the workspaces off screen once: what is new there shows as
    /// a dot on the way to them.
    public func refreshOthers() async {
        let others = workspaces.compactMap { $0.id == session?.id ? nil : sessions[$0.id] }
        await withTaskGroup(of: Void.self) { group in
            for other in others {
                group.addTask {
                    await other.restore()
                    await other.refresh()
                }
            }
        }
    }

    /// Keeps looking at them, while the app is in front.
    public func watchOthers() async {
        while !Task.isCancelled {
            await refreshOthers()
            try? await Task.sleep(for: Self.othersInterval)
        }
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
    @Environment(AppModel.self) private var app
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
        // The workspaces off screen are looked at now and then, to say
        // when something new came there.
        .task(id: scenePhase) {
            if scenePhase == .active { await app.watchOthers() }
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
    @Environment(AppModel.self) private var app
    @Environment(WorkspaceSession.self) private var session

    var body: some View {
        Button {
            router.sheet = .workspace
        } label: {
            WorkspaceBadge(name: session.name, size: 28)
                // Something new came in another workspace: a dot on the way there.
                .overlay(alignment: .topTrailing) {
                    if app.unreadElsewhere {
                        Circle().fill(MycoColor.unread).frame(width: 9, height: 9)
                            .overlay { Circle().stroke(MycoColor.bg, lineWidth: 1.5) }
                            .offset(x: 3, y: -3)
                            .accessibilityIdentifier("workspace.elsewhere")
                    }
                }
        }
        .accessibilityLabel(
            app.unreadElsewhere
                ? L("工作区：\(session.name)，别的工作区有新消息", "Workspace: \(session.name). Something new in another workspace")
                : L("工作区：\(session.name)", "Workspace: \(session.name)"))
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
