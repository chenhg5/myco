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
    /// What it is called on this device; the person can rename it.
    public private(set) var name: String

    public func rename(_ name: String) {
        self.name = name
    }
    public let hub: any HubAPI

    /// How messages from this app are signed (`@console:<member>`).
    public private(set) var me = ""
    /// Who I am here, once the workspace has said (each launch).
    public private(set) var whoami: Whoami?
    public private(set) var statuses: [AgentStatus] = []
    public private(set) var machines: [Machine] = []
    public private(set) var projects: [Project] = []
    public private(set) var recent: [RecentItem] = []
    /// When the workspace last answered.
    public private(set) var loadedAt: Date?
    /// Why it cannot be reached now; what is shown is from `loadedAt`.
    public private(set) var problem: String?
    /// The raw reason (status, code, message), for whoever wants to see it.
    public private(set) var problemDetail: String?

    /// Messages this person took out of their own view (删除): kept by
    /// the workspace, so all their devices agree; by this device alone
    /// when the workspace is too old to keep them.
    public private(set) var hidden: Set<String> = []
    private var hiddenKeptHere = false

    /// Told each time the workspace answered a refresh (to announce what is new).
    @ObservationIgnored public var refreshed: (@MainActor (WorkspaceSession) -> Void)?
    /// Told whenever a workspace's agents are loaded (from the hub, or from
    /// what it showed last): the app spreads their faces from it.
    public static var agentsLoaded: (@MainActor ([Agent]) -> Void)?

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

    @ObservationIgnored private var automationsModel: AutomationsModel?

    /// What is sent for me on a schedule or on a call: one model for as
    /// long as the workspace is on screen.
    public var automations: AutomationsModel {
        if let automationsModel { return automationsModel }
        let model = AutomationsModel(hub: hub)
        automationsModel = model
        return model
    }

    @ObservationIgnored private var searchModel: SearchModel?

    /// What the chats page's search box finds.
    public var search: SearchModel {
        if let searchModel { return searchModel }
        let model = SearchModel(session: self)
        searchModel = model
        return model
    }

    @ObservationIgnored private var workStates: [String: WorkStateModel] = [:]

    /// An agent's goal and background waits (the bars above its chat).
    public func workState(of agent: String) -> WorkStateModel {
        if let model = workStates[agent] { return model }
        let model = WorkStateModel(hub: hub, agent: agent)
        workStates[agent] = model
        return model
    }

    @ObservationIgnored private var waitingModel: NoticesModel?
    @ObservationIgnored private var usageModel: UsageModel?

    /// 待处理: what waits for me (an agent asking, a task that failed).
    public var waiting: NoticesModel {
        if let waitingModel { return waitingModel }
        let model = NoticesModel(hub: hub)
        waitingModel = model
        return model
    }

    /// 用量: where the workspace's tokens go.
    public var usage: UsageModel {
        if let usageModel { return usageModel }
        let model = UsageModel(hub: hub)
        usageModel = model
        return model
    }

    @ObservationIgnored private var workflowsModel: WorkflowsModel?

    /// 工作流: decisions waiting for me, the workflows and their runs.
    public var workflows: WorkflowsModel {
        if let workflowsModel { return workflowsModel }
        let model = WorkflowsModel(hub: hub)
        workflowsModel = model
        return model
    }

    @ObservationIgnored private var superagentModel: SuperagentModel?

    /// My superagent here and what it waits on me for.
    public var superagent: SuperagentModel {
        if let superagentModel { return superagentModel }
        let model = SuperagentModel(hub: hub)
        superagentModel = model
        return model
    }

    @ObservationIgnored private var appsModel: AppsModel?

    /// 应用: the apps of this workspace and the pages made from them.
    public var apps: AppsModel {
        if let appsModel { return appsModel }
        let model = AppsModel(hub: hub)
        appsModel = model
        return model
    }

    @ObservationIgnored private var handoverModels: [String: HandoversModel] = [:]
    @ObservationIgnored var pinModels: [String: PinsModel] = [:]

    /// An agent's handovers: the times another session took it over.
    public func handovers(of agent: String) -> HandoversModel {
        if let known = handoverModels[agent] { return known }
        let model = HandoversModel(hub: hub, agent: agent)
        handoverModels[agent] = model
        return model
    }

    @ObservationIgnored private var pagesModel: PagesModel?

    /// The web pages this workspace's messages link to.
    public var pages: PagesModel {
        if let pagesModel { return pagesModel }
        let model = PagesModel(hub: hub)
        pagesModel = model
        return model
    }

    /// The agents a task can be sent to, by the name the app shows.
    public var agentsByName: [Agent] {
        statuses.map(\.agent).filter { Naming.kind(of: $0) == .agent }
            .sorted { displayName(of: $0.name).localizedStandardCompare(displayName(of: $1.name)) == .orderedAscending }
    }

    /// The conversation with an agent (in a project: with its coordinator).
    /// One model per conversation for as long as the workspace is on screen.
    public func conversation(with agent: String, project: String? = nil) -> ConversationModel {
        let key = agent + "|" + (project ?? "")
        if let known = conversations[key] { return known }
        let model = ConversationModel(session: self, agent: agent, project: project)
        conversations[key] = model
        return model
    }

    /// What `agent` and `other` said to each other, to read (a handoff
    /// card opens it): `other`'s conversation with `agent`, from which
    /// nothing is sent.
    public func exchange(of agent: String, with other: String) -> ConversationModel {
        let key = agent + "|~|" + other
        if let known = conversations[key] { return known }
        let model = ConversationModel(session: self, agent: agent, observed: other)
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
        Self.agentsLoaded?(statuses.map(\.agent))
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

    /// A refresh the person asked for (a list pulled down) is under way.
    public private(set) var refreshingByHand = false

    /// Reads the lists again because the person asked. It takes long
    /// enough to be seen, however quick the answer.
    public func refreshByHand(atLeast shown: Duration = .milliseconds(500)) async {
        guard !refreshingByHand else { return }
        refreshingByHand = true
        async let seen: Void? = try? await Task.sleep(for: shown)
        await refresh()
        _ = await seen
        refreshingByHand = false
    }

    /// Looks in a row that failed.
    private var failures = 0

    /// The names a person says here: agents and members (as shown and by
    /// name), projects (title and name).
    var speechNames: [String] {
        statuses.flatMap { [$0.agent.displayName ?? "", $0.agent.name] }.filter { !$0.isEmpty }
            + projects.flatMap { [$0.title, $0.name] }
    }

    public func refresh() async {
        do {
            if me.isEmpty || whoami == nil {
                let who = try await hub.whoami()
                whoami = who
                me = who.consoleSender
            }
            async let overview = hub.overview()
            async let projects = hub.projects()
            async let recent = hub.recent(.myPairs)
            let (o, p, r) = try await (overview, projects, recent)
            statuses = o.agents
            Self.agentsLoaded?(statuses.map(\.agent))
            machines = o.machines
            self.projects = p.filter { $0.archived != true }
            self.recent = r
            // Speech recognition listens for this workspace's names.
            SpeechVocabulary.shared.setWorkspace(speechNames)
            loadedAt = Date()
            failures = 0
            problem = nil
            problemDetail = nil
            await loadHidden()
            refreshed?(self)
            if readBaseline == nil {
                // The first look at this workspace: all of it counts as seen.
                readBaseline = Date()
                await saveRead()
            }
            await save()
        } catch is CancellationError {
            // leaving the page is not a problem to report
        } catch {
            // One look that failed is a blip (through a slow proxy, a look
            // now and then does): the lists say so when the next fails too,
            // when the person asked for this one, or when nothing was ever
            // shown. The conversation's rule.
            failures += 1
            guard failures >= 2 || refreshingByHand || loadedAt == nil else { return }
            if let error = error as? HubError {
                problem = error.message
                problemDetail = Failure.of(error).detail ?? error.message
            } else {
                problem = error.localizedDescription
                problemDetail = problem
            }
        }
    }

    // MARK: Model and effort

    /// The workspace answered that it keeps no run settings (before v0.19.5).
    public private(set) var runSettingsTooOld = false

    /// I own this agent, or manage the workspace. Not known until the
    /// workspace has said who I am: then no.
    public func manages(_ agent: Agent) -> Bool {
        guard let whoami else { return false }
        if whoami.canManage { return true }
        guard let mine = whoami.member?.id, !mine.isEmpty else { return false }
        return agent.ownerId == mine
    }

    /// What an agent may touch when it runs, as its message box shows it.
    public func accessChoice(of name: String) -> AccessChoice? {
        guard let agent = agent(name) else { return nil }
        let mine = whoami?.member?.id ?? ""
        let owns = !mine.isEmpty && agent.ownerId == mine
        return AccessChoice.of(agent, owns: owns, tooOld: runSettingsTooOld)
    }

    /// The model and effort an agent runs with, as its message box shows them.
    public func runChoice(of name: String) -> RunChoice? {
        guard let agent = agent(name) else { return nil }
        return RunChoice.of(agent, manages: manages(agent), tooOld: runSettingsTooOld)
    }

    /// Switches an agent's model and effort, from its next run ("" follows
    /// its computer's setting). Answers why it could not, if it could not.
    public func setRun(of name: String, model: String, effort: String, access: String? = nil) async -> String? {
        do {
            let agent = try await hub.setRunSettings(of: name, model: model, effort: effort, access: access)
            if let index = statuses.firstIndex(where: { $0.agent.name == name }) {
                statuses[index].agent = agent
                Self.agentsLoaded?(statuses.map(\.agent))
            }
            return nil
        } catch let error as HubError where error.error == "not_found" || error.error == "http_404" || error.error == "http_405" {
            // A workspace from before this: shown, not switched.
            runSettingsTooOld = true
            return runChoice(of: name)?.note ?? error.message
        } catch let error as HubError {
            return error.message
        } catch {
            return error.localizedDescription
        }
    }

    /// Asks an agent's runner to list its provider's models again (it
    /// does within a minute; the list comes with its next report).
    /// Answers why it could not, if it could not.
    public func listModelsAgain(of name: String) async -> String? {
        do {
            let agent = try await hub.listModelsAgain(of: name)
            if let index = statuses.firstIndex(where: { $0.agent.name == name }) {
                statuses[index].agent = agent
                Self.agentsLoaded?(statuses.map(\.agent))
            }
            return nil
        } catch let error as HubError {
            return error.message
        } catch {
            return error.localizedDescription
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
    /// A project's members in its groups (the desktop's members column):
    /// each group in order with its members, then those in none. Without
    /// groups, one section without a title.
    public func memberSections(of project: Project) -> [(group: String?, members: [AgentStatus])] {
        MemberGroups.sections(groups: project.groups ?? [], members: project.members, statuses: memberStatuses(of: project))
    }

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

/// How a project's members are grouped.
public enum MemberGroups {
    public static func sections(groups: [String], members: [ProjectMember], statuses: [AgentStatus]) -> [(group: String?, members: [AgentStatus])] {
        let groupOf = Dictionary(members.map { ($0.name, $0.group ?? "") }, uniquingKeysWith: { first, _ in first })
        guard !groups.isEmpty else { return statuses.isEmpty ? [] : [(nil, statuses)] }
        var sections: [(group: String?, members: [AgentStatus])] = []
        for group in groups {
            let these = statuses.filter { groupOf[$0.agent.name] == group }
            if !these.isEmpty { sections.append((group, these)) }
        }
        // In none, or in a group the list no longer has.
        let rest = statuses.filter { !groups.contains(groupOf[$0.agent.name] ?? "") }
        if !rest.isEmpty { sections.append((sections.isEmpty ? nil : L("未分组", "Ungrouped"), rest)) }
        return sections
    }
}
