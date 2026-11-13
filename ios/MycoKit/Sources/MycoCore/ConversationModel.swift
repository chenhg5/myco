import Foundation
import Observation
import MycoAPI

/// One conversation on screen: what was said, where the open task stands,
/// and what the input does. The page binds to it and draws; it does not
/// decide any of this itself.
@MainActor
@Observable
public final class ConversationModel {
    public let agent: String
    /// Set on a project's page: the conversation is with its coordinator,
    /// and the coordinator's dispatches show as capsules.
    public let project: String?

    public private(set) var items: [ConversationItem] = []
    public private(set) var openTask: Message?
    public private(set) var loaded = false
    /// Why the last send, steer or stop did not go through.
    public private(set) var problem: String?
    /// A message waiting for the open task to finish.
    public struct Waiting: Identifiable, Hashable, Sendable {
        public var id: String
        public var body: String
        /// Held by the workspace (true), or kept here because the
        /// workspace is too old to hold it.
        public var onHub: Bool
    }

    /// Messages waiting behind the open task, in the order they will go:
    /// those the workspace holds, then those kept here.
    public private(set) var waiting: [Waiting] = []
    /// Kept here: the workspace could not hold them.
    private var queued: [String] = []
    /// Waiting messages taken back here (a stopped one may still come back
    /// once more from the workspace).
    private var withdrawn: Set<String> = []
    /// A project's page starts a thread in the project with its first
    /// message of this visit, then continues it.
    private var startedProjectThread = false
    public var draft = ""

    /// Older messages exist than the ones shown: scrolling up loads them.
    public private(set) var hasEarlier = false
    /// Goes up whenever what the page shows changed: a message, or a
    /// task's steps (which make a row taller without adding one).
    public private(set) var revision = 0

    private let session: WorkspaceSession
    private var sending = false
    /// Every message of this conversation seen so far, by id: the newest
    /// page, refreshed all the time, and the older pages scrolled back to.
    private var known: [String: Message] = [:]
    private var others: [Message] = []
    /// On a member's page: its exchanges with its colleagues, by id.
    private var colleagueMessages: [String: Message] = [:]
    private var reloads = 0

    /// A member's colleagues' exchanges are asked for again every this many
    /// refreshes (mine with it: every time).
    static let colleaguesEvery = 5
    /// At most this many colleagues' exchanges make up a member's page.
    static let maxColleagues = 6
    private var loadingEarlier = false
    /// Older pages were loaded: the newest page no longer says whether
    /// there is more.
    private var wentBack = false
    /// The steps of my tasks, by task id.
    private var traces: [String: [TraceEvent]] = [:]
    /// Tasks that are closed and whose steps are all here.
    private var settled: Set<String> = []
    private var processes: [String: RunProcess] = [:]

    /// How many of my newest tasks show their steps.
    static let tracedTasks = 6

    /// How often an open conversation refreshes.
    public static let pollInterval: Duration = .seconds(2)

    public init(session: WorkspaceSession, agent: String, project: String? = nil) {
        self.session = session
        self.agent = agent
        self.project = project
    }

    /// How a task of mine was worked on, when its steps are known.
    public func process(of taskID: String) -> RunProcess? {
        processes[taskID]
    }

    public var action: ComposerAction {
        ComposerAction.resolve(hasOpenTask: openTask != nil, hasText: !trimmedDraft.isEmpty)
    }

    /// Sending is off while a send is under way, or with nothing to send.
    public var canSubmit: Bool {
        !sending && (action == .stop || !trimmedDraft.isEmpty)
    }

    private var trimmedDraft: String {
        draft.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// What is kept of a conversation between launches.
    struct Snapshot: Codable {
        var messages: [Message]
        var others: [Message]
        var traces: [String: [TraceEvent]]
        var colleagues: [Message]?
    }

    private var snapshotName: String { "conversation-" + agent + "-" + (project ?? "") }
    private var restored = false
    private var lastSaved: Data?

    /// Shows what this conversation showed last, until the workspace answers.
    public func restore() async {
        guard !restored, !loaded else { return }
        restored = true
        guard let data = await session.snapshots?.snapshot(of: session.id, name: snapshotName),
              let snapshot = try? HubJSON.decoder().decode(Snapshot.self, from: data), !loaded
        else { return }
        for message in snapshot.messages { known[message.id] = message }
        for message in snapshot.colleagues ?? [] { colleagueMessages[message.id] = message }
        traces = snapshot.traces
        processes = snapshot.traces.mapValues { RunProcess(events: $0) }
        // Closed tasks' steps are complete: no need to fetch them again.
        settled = Set((snapshot.messages + (snapshot.colleagues ?? [])).filter { !$0.isOpen }.map(\.id)).intersection(snapshot.traces.keys)
        show(others: snapshot.others)
    }

    public func run() async {
        await restore()
        while !Task.isCancelled {
            await reload()
            try? await Task.sleep(for: Self.pollInterval)
        }
    }

    public func reload() async {
        await load()
        // The open task closed: what waited behind it goes out now.
        if await sendNextQueued() { await load() }
    }

    private func load() async {
        let me = session.me
        guard !me.isEmpty else { return }
        do {
            let page = try await session.hub.exchanges(between: me, and: agent, before: nil)
            for message in page.messages { known[message.id] = message }
            if !wentBack { hasEarlier = page.more }
            // Mine with it comes first and shows at once; the rest of its
            // work (asked for less often) is merged in when it comes.
            if !loaded { show(others: others) }
            var others = self.others
            if reloads % Self.colleaguesEvery == 0 { others = await loadSurroundings() }
            reloads += 1
            let everything = Array(known.values) + Array(colleagueMessages.values)
            await loadSteps(of: everything.filter { $0.to == agent && $0.type == "task" && $0.status != "queued" })
            show(others: others)
            // Only the newest pages are kept between launches.
            await save(Snapshot(messages: page.messages, others: others, traces: traces, colleagues: Array(colleagueMessages.values)))
        } catch is CancellationError {
            return
        } catch {
            // The workspace's own banner says it cannot be reached.
            loaded = true
        }
    }

    /// Loads the page before the oldest message shown. True if it brought any.
    @discardableResult
    public func loadEarlier() async -> Bool {
        guard hasEarlier, !loadingEarlier, let oldest = known.values.map(\.createdAt).min() else { return false }
        loadingEarlier = true
        defer { loadingEarlier = false }
        guard let page = try? await session.hub.exchanges(between: session.me, and: agent, before: oldest) else { return false }
        wentBack = true
        hasEarlier = page.more && !page.messages.isEmpty
        for message in page.messages { known[message.id] = message }
        show(others: others)
        return !page.messages.isEmpty
    }

    private func show(others: [Message]) {
        self.others = others
        let me = session.me
        // Mine with the agent, and on a member's page the agent's with its
        // colleagues, as one conversation.
        var all = known.filter { !withdrawn.contains($0.key) }
        for (id, message) in colleagueMessages where all[id] == nil { all[id] = message }
        var tracedPlans: [String: (items: [PlanItem], at: Date?)] = [:]
        for (taskID, process) in processes {
            if let plan = process.plan { tracedPlans[taskID] = (plan, process.planAt) }
        }
        let built = ConversationBuilder.items(
            me: me, agent: agent, messages: Array(all.values), others: others,
            processes: Set(processes.filter { $0.value.hasSteps }.keys), tracedPlans: tracedPlans,
            status: { self.session.status(of: $0) }, now: Date())
        let mine = Array(known.values).filter { !withdrawn.contains($0.id) }
        let held = ConversationBuilder.queuedBehind(mine).filter { $0.from == me }
            .map { Waiting(id: $0.id, body: $0.body, onHub: true) }
        let local = queued.enumerated().map { Waiting(id: "local-\($0.offset)", body: $0.element, onHub: false) }
        if waiting != held + local { waiting = held + local }
        // Nothing new: leave the list alone, so the page does not redraw.
        if built != items {
            items = built
            revision += 1
        }
        openTask = ConversationBuilder.openTask(me: me, messages: mine)
        loaded = true
    }

    /// A new message of mine: in the thread my last exchange was in. On a
    /// project's page, the first one of this visit starts a thread in the
    /// project (as the desktop app does).
    private func request(_ body: String, after: String? = nil) -> SendRequest {
        if project != nil, !startedProjectThread {
            startedProjectThread = true
            return SendRequest(to: agent, body: body, project: project, after: after)
        }
        let thread = ConversationBuilder.threadToContinue(me: session.me, messages: Array(known.values))
        return SendRequest(to: agent, body: body, threadId: thread, project: thread == nil ? project : nil, after: after)
    }

    private func save(_ snapshot: Snapshot) async {
        guard let store = session.snapshots, let data = try? HubJSON.encoder().encode(snapshot), data != lastSaved else { return }
        lastSaved = data
        await store.setSnapshot(data, of: session.id, name: snapshotName)
    }

    /// What surrounds my own exchange with the agent, as the desktop app
    /// gathers it. On a project's page (the agent is its coordinator): the
    /// work it gave the other members, returned to show as capsules. On a
    /// member's own page: its exchanges with its colleagues in its
    /// projects, merged into the conversation.
    private func loadSurroundings() async -> [Message] {
        let everything = (try? await session.hub.recent(.everything)) ?? []
        func feed(_ project: Project) async -> [RecentItem] {
            let own = (try? await session.hub.recent(.project(project.name))) ?? []
            return ProjectFeed.items(of: project, own: own, everything: everything)
        }
        if let project, let found = session.project(project) {
            let tasks = await feed(found).compactMap(\.task)
            return tasks.filter { $0.from == agent && $0.to != session.me }
        }
        var colleagues: [String] = []
        for project in session.projects(of: agent).prefix(Self.projectsScanned) {
            let names = ProjectFeed.colleagues(of: agent, in: await feed(project), coordinator: session.coordinator(of: project))
            for name in names where !colleagues.contains(name) { colleagues.append(name) }
        }
        for colleague in colleagues.prefix(Self.maxColleagues) {
            guard let page = try? await session.hub.exchanges(between: colleague, and: agent, before: nil) else { continue }
            for message in page.messages { colleagueMessages[message.id] = message }
        }
        return []
    }

    /// How many of an agent's projects are looked through for its colleagues.
    static let projectsScanned = 3

    /// Fetches the steps of the agent's newest tasks: all of them once for a
    /// closed task, the new ones each time for one still being worked on.
    private func loadSteps(of tasks: [Message]) async {
        let newest = tasks.sorted { $0.createdAt > $1.createdAt }.prefix(Self.tracedTasks)
        for task in newest where !settled.contains(task.id) {
            let known = traces[task.id] ?? []
            guard let trace = try? await session.hub.trace(of: task.id, after: known.last?.seq ?? 0) else { continue }
            if !trace.events.isEmpty || processes[task.id] == nil {
                let events = known + trace.events
                traces[task.id] = events
                let process = RunProcess(events: events)
                if processes[task.id] != process {
                    processes[task.id] = process
                    revision += 1
                }
            }
            if !task.isOpen { settled.insert(task.id) }
        }
    }

    /// The send button: a new message, an addition to the open task, or stop.
    public func submit() async {
        guard canSubmit else { return }
        let action = action
        let text = trimmedDraft
        if action != .stop { draft = "" }
        sending = true
        do {
            switch action {
            case .send:
                _ = try await session.hub.send(request(text))
            case .steer:
                if let task = openTask { try await session.hub.steer(task.id, body: text) }
            case .stop:
                if let task = openTask { try await session.hub.stop(task.id) }
            }
            problem = nil
        } catch {
            // Give the words back: nothing typed is lost to a failed send.
            if action != .stop, draft.isEmpty { draft = text }
            problem = (error as? HubError)?.message ?? error.localizedDescription
        }
        sending = false
        await reload()
    }

    /// Stops the open task whatever is typed (the action on a stuck task).
    public func submitStop() async {
        guard let task = openTask, !sending else { return }
        sending = true
        do {
            try await session.hub.stop(task.id)
            problem = nil
        } catch {
            problem = (error as? HubError)?.message ?? error.localizedDescription
        }
        sending = false
        await reload()
    }

    /// Puts the typed text behind the open task: the workspace holds it
    /// and delivers it when that task ends. A workspace too old to hold it
    /// refuses; then it is kept here and sent when the task ends.
    public func queueNext() async {
        let text = trimmedDraft
        guard !text.isEmpty, !sending else { return }
        guard let task = openTask else {
            await submit()
            return
        }
        draft = ""
        sending = true
        do {
            _ = try await session.hub.send(request(text, after: task.id))
            problem = nil
        } catch let error as HubError where error.error == "invalid_argument" {
            queued.append(text)
        } catch {
            draft = text
            problem = (error as? HubError)?.message ?? error.localizedDescription
        }
        sending = false
        await reload()
    }

    /// Takes a waiting message back: it will not be sent. `edit` puts its
    /// words back in the input.
    public func withdraw(_ item: Waiting, edit: Bool) async {
        if item.onHub {
            do {
                try await session.hub.stop(item.id)
                withdrawn.insert(item.id)
                problem = nil
            } catch {
                problem = (error as? HubError)?.message ?? error.localizedDescription
                return
            }
        } else if let index = waiting.filter({ !$0.onHub }).firstIndex(of: item), queued.indices.contains(index) {
            queued.remove(at: index)
        }
        if edit { draft = item.body }
        show(others: others)
    }

    /// Sends the first waiting message once nothing is open. True if it went.
    private func sendNextQueued() async -> Bool {
        guard openTask == nil, !sending, let next = queued.first else { return false }
        sending = true
        defer { sending = false }
        do {
            _ = try await session.hub.send(request(next))
            queued.removeFirst()
            return true
        } catch {
            problem = (error as? HubError)?.message ?? error.localizedDescription
            return false
        }
    }
}
