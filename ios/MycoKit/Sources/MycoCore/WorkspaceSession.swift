import Foundation
import Observation
import MycoAPI

/// The workspace on screen: its members, projects and recent conversations,
/// kept fresh while the app is in front. Pages read it; they do not call
/// the hub for any of this themselves.
@MainActor
@Observable
public final class WorkspaceSession {
    public let id: String
    public let name: String
    public let hub: any HubAPI

    /// How messages from this app are signed (`@console:<member>`).
    public private(set) var me = ""
    public private(set) var statuses: [AgentStatus] = []
    public private(set) var machines: [Machine] = []
    public private(set) var projects: [Project] = []
    public private(set) var recent: [RecentItem] = []
    /// When the workspace last answered.
    public private(set) var loadedAt: Date?
    /// Why it cannot be reached now; what is shown is from `loadedAt`.
    public private(set) var problem: String?

    /// How often the lists refresh while the app is in front.
    public static let pollInterval: Duration = .seconds(5)

    public init(id: String, name: String, hub: any HubAPI) {
        self.id = id
        self.name = name
        self.hub = hub
    }

    /// Refreshes until the surrounding task is cancelled (the page left the
    /// screen, the app went to the background).
    public func run() async {
        while !Task.isCancelled {
            await refresh()
            try? await Task.sleep(for: Self.pollInterval)
        }
    }

    public func refresh() async {
        do {
            if me.isEmpty { me = try await hub.whoami().consoleSender }
            async let overview = hub.overview()
            async let projects = hub.projects()
            async let recent = hub.recent(project: nil)
            let (o, p, r) = try await (overview, projects, recent)
            statuses = o.agents
            machines = o.machines
            self.projects = p.filter { $0.archived != true }
            self.recent = r
            loadedAt = Date()
            problem = nil
        } catch let error as HubError {
            problem = error.message
        } catch is CancellationError {
            // leaving the page is not a problem to report
        } catch {
            problem = error.localizedDescription
        }
    }

    // MARK: Lookups

    public func status(of name: String) -> AgentStatus? {
        statuses.first { $0.agent.name == name }
    }

    public func agent(_ name: String) -> Agent? {
        status(of: name)?.agent
    }

    /// What a node is called here. A person's console is "你" when it is me.
    public func displayName(of name: String) -> String {
        if name == me { return L("你", "You") }
        if name.hasPrefix("@console:") { return String(name.dropFirst("@console:".count)) }
        guard let agent = agent(name) else { return name }
        return Naming.name(of: agent, among: statuses.map(\.agent), machines: machines)
    }

    /// The AI members: not computers, machines or people.
    public var members: [AgentStatus] {
        statuses.filter { Naming.kind(of: $0.agent) == .agent }
    }

    public func project(_ name: String) -> Project? {
        projects.first { $0.name == name }
    }

    /// The one people talk to in a project: its coordinator, else the member
    /// that joined first.
    public func coordinator(of project: Project) -> String? {
        let agents = project.members.filter { m in agent(m.name).map { Naming.kind(of: $0) == .agent } ?? false }
        return (agents.first { $0.role == "coordinator" } ?? agents.min { $0.addedAt < $1.addedAt })?.name
    }

    /// A project's members as the page lists them: the coordinator first,
    /// then those at work, those waiting, the free ones, and those away.
    public func memberStatuses(of project: Project) -> [AgentStatus] {
        let lead = coordinator(of: project)
        func rank(_ s: AgentStatus) -> Int {
            if s.agent.name == lead { return 0 }
            switch s.state {
            case "working": return 1
            case "pending": return 2
            case "offline": return 4
            default: return 3
            }
        }
        return project.members.compactMap { status(of: $0.name) }
            .filter { Naming.kind(of: $0.agent) == .agent }
            .sorted { (rank($0), $0.agent.name) < (rank($1), $1.agent.name) }
    }

    public func projects(of agentName: String) -> [Project] {
        projects.filter { p in p.members.contains { $0.name == agentName } }
    }
}
