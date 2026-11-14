import Foundation
import Observation
import MycoAPI
import MycoStore

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

    /// Messages this person took out of their own view (删除): kept by
    /// the workspace, so all their devices agree; by this device alone
    /// when the workspace is too old to keep them.
    public private(set) var hidden: Set<String> = []
    private var hiddenKeptHere = false

    /// How often the lists refresh while the app is in front.
    public static let pollInterval: Duration = .seconds(5)

    /// When each conversation was last looked at, by the agent on the other
    /// side. Kept on this device only.
    public private(set) var readAt: [String: Date] = [:]
    /// When this device first saw the workspace: what was there before
    /// counts as read.
    public private(set) var readBaseline: Date?

    struct ReadState: Codable {
        var baseline: Date
        var readAt: [String: Date]
    }

    /// The conversation with this agent is on screen: nothing in it is unread.
    public func markRead(_ agent: String) {
        readAt[agent] = Date()
        Task { await saveRead() }
    }

    /// Conversations with something unread.
    public var unreadCount: Int {
        chatRows.filter(\.unread).count
    }

    private func saveRead() async {
        guard let snapshots, let readBaseline else { return }
        let state = ReadState(baseline: readBaseline, readAt: readAt)
        await snapshots.setSnapshot(try? HubJSON.encoder().encode(state), of: id, name: "read")
    }

    let snapshots: (any SnapshotStore)?
    /// The conversations opened this launch, kept so reopening one is instant.
    private var conversations: [String: ConversationModel] = [:]

    /// The conversation with an agent (in a project: with its coordinator).
    /// One model per conversation for as long as the workspace is on screen.
    public func conversation(with agent: String, project: String? = nil) -> ConversationModel {
        let key = agent + "|" + (project ?? "")
        if let known = conversations[key] { return known }
        let model = ConversationModel(session: self, agent: agent, project: project)
        conversations[key] = model
        return model
    }

    /// - Parameter snapshots: where what it showed last is kept, so the
    ///   next launch opens on it at once; nil keeps nothing.
    ///   - files: the folder its messages' files are kept in once
    ///     fetched; nil is its own folder in the caches.
    public init(id: String, name: String, hub: any HubAPI, snapshots: (any SnapshotStore)? = nil, files: URL? = nil) {
        self.id = id
        self.name = name
        self.hub = hub
        self.snapshots = snapshots
        shelf = FileShelf(folder: files ?? FileShelf.folder(of: id)) { [hub] id in try await hub.file(id) }
    }

    /// The files of this workspace's messages, on this device once fetched.
    public let shelf: FileShelf

    /// What is kept between launches: the lists, never a token.
    struct Snapshot: Codable {
        var me: String
        var statuses: [AgentStatus]
        var machines: [Machine]
        var projects: [Project]
        var recent: [RecentItem]
        var savedAt: Date
    }

    /// Shows what the workspace showed last, until it answers.
    public func restore() async {
        if readBaseline == nil, let data = await snapshots?.snapshot(of: id, name: "read"),
           let state = try? HubJSON.decoder().decode(ReadState.self, from: data) {
            readBaseline = state.baseline
            readAt = state.readAt
        }
        guard loadedAt == nil, let data = await snapshots?.snapshot(of: id, name: "lists"),
              let snapshot = try? HubJSON.decoder().decode(Snapshot.self, from: data)
        else { return }
        me = snapshot.me
        statuses = snapshot.statuses
        machines = snapshot.machines
        projects = snapshot.projects
        recent = snapshot.recent
        loadedAt = snapshot.savedAt
    }

    private func save() async {
        guard let snapshots else { return }
        let snapshot = Snapshot(me: me, statuses: statuses, machines: machines, projects: projects, recent: recent, savedAt: Date())
        await snapshots.setSnapshot(try? HubJSON.encoder().encode(snapshot), of: id, name: "lists")
    }

    /// Refreshes until the surrounding task is cancelled (the page left the
    /// screen, the app went to the background).
    public func run() async {
        await restore()
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
            async let recent = hub.recent(.myPairs)
            let (o, p, r) = try await (overview, projects, recent)
            statuses = o.agents
            machines = o.machines
            self.projects = p.filter { $0.archived != true }
            self.recent = r
            loadedAt = Date()
            problem = nil
            await loadHidden()
            if readBaseline == nil {
                // The first look at this workspace: all of it counts as seen.
                readBaseline = Date()
                await saveRead()
            }
            await save()
        } catch let error as HubError {
            problem = error.message
        } catch is CancellationError {
            // leaving the page is not a problem to report
        } catch {
            problem = error.localizedDescription
        }
    }

    // MARK: Hidden messages

    private func loadHidden() async {
        if hiddenKeptHere { return }
        do {
            let ids = Set(try await hub.hidden())
            if ids != hidden { hidden = ids }
        } catch let error as HubError where error.error == "not_found" {
            // A workspace from before this: the list lives on this device.
            hiddenKeptHere = true
            if let data = await snapshots?.snapshot(of: id, name: "hidden"), let ids = try? JSONDecoder().decode([String].self, from: data) {
                hidden = Set(ids)
            }
        } catch {
            // Not reachable now: what is known stays.
        }
    }

    /// Takes messages out of my view of their conversation, or (with
    /// `shown`) puts them back. Nothing is deleted: the agents and other
    /// people still see them, and a task goes on. It shows at once; the
    /// workspace is told after. Throws when the workspace refused.
    public func setHidden(_ ids: [String], hidden hide: Bool = true) async throws {
        let before = hidden
        if hide { hidden.formUnion(ids) } else { hidden.subtract(ids) }
        if !hiddenKeptHere {
            do {
                hidden = Set(try await hub.setHidden(hide: hide ? ids : [], show: hide ? [] : ids))
                return
            } catch let error as HubError where error.error == "not_found" {
                hiddenKeptHere = true
            } catch {
                hidden = before
                throw error
            }
        }
        await snapshots?.setSnapshot(try? JSONEncoder().encode(hidden.sorted()), of: id, name: "hidden")
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

/// A project's activity, as the desktop app gathers it: its own threads,
/// and the threads in no project between its members. An agent that is not
/// woken by a runner (it waits on its inbox) starts threads without a
/// project, a coordinator's hand-outs among them.
public enum ProjectFeed {
    public static func items(of project: Project, own: [RecentItem], everything: [RecentItem]) -> [RecentItem] {
        let members = Set(project.members.map(\.name))
        let have = Set(own.map(\.threadId))
        let among = everything.filter { item in
            guard item.project == nil, item.kind == "thread", !have.contains(item.threadId) else { return false }
            let agents = item.participants.filter { !$0.hasPrefix("@") }
            return !agents.isEmpty && agents.allSatisfy(members.contains)
        }
        return own + among
    }

    /// Everyone a member has worked with in the project (from its feed),
    /// and its coordinator: whose exchanges with it make up its page.
    public static func colleagues(of agent: String, in feed: [RecentItem], coordinator: String?) -> [String] {
        var names: Set<String> = []
        for item in feed where item.kind == "thread" && item.participants.contains(agent) {
            for other in item.participants where other != agent && !other.hasPrefix("@") { names.insert(other) }
        }
        if let coordinator, coordinator != agent { names.insert(coordinator) }
        return names.sorted()
    }
}
