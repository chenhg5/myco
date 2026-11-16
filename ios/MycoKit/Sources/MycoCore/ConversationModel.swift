import Foundation
import Observation
import MycoAPI

/// A file about to go with the next message. It is uploaded as soon as it
/// is added (to the workspace, as mine), so sending is only the message.
public struct PendingFile: Identifiable, Hashable, Sendable {
    public enum State: Hashable, Sendable {
        case uploading
        /// In the workspace, under this id.
        case ready(fileID: String)
        /// Why it did not go up.
        case failed(String)
    }

    public var id: String
    public var name: String
    public var size: Int
    public var contentType: String
    /// A small picture of it, when it is one.
    public var preview: Data?
    public var state: State
}

/// One conversation on screen: what was said, where the open task stands,
/// and what the input does. The page binds to it and draws; it does not
/// decide any of this itself.
@MainActor
@Observable
public final class ConversationModel {
    public let agent: String
    /// Set on a project's page: the conversation is with its coordinator,
    /// and what it and the members handed each other shows as cards.
    public let project: String?
    /// Set on a look at what two agents said to each other (opened from a
    /// handoff card): the other one. The page then shows their exchange
    /// as the other one's conversation with the agent, and nothing can be
    /// sent from it.
    public let observed: String?

    /// Whose conversation with the agent this is: mine, or the other agent's.
    private var me: String { observed ?? session.me }

    /// A look at two agents' exchange: nothing is sent from here.
    public var readOnly: Bool { observed != nil }

    public private(set) var items: [ConversationItem] = []
    public private(set) var openTask: Message?
    /// What the agent offers me to say next (see
    /// `ConversationBuilder.suggestion`); the page shows it above an empty
    /// input, and a tap writes it there to edit or send.
    public private(set) var suggestion: String?
    public private(set) var loaded = false
    /// Why the last look at the workspace failed; nil while it answers.
    /// What is on the page is then from before.
    public private(set) var unreachable: String?
    /// A refresh the person asked for is under way.
    public private(set) var refreshing = false

    /// Something I sent that the workspace has not handed back yet: it
    /// shows at the end of the conversation at once, marked as being sent.
    public struct Outgoing: Identifiable, Hashable, Sendable {
        public var id: String
        /// What I typed (nothing, when only files go).
        public var text: String
        public var files: [Attachment]
        /// The message as it goes, and the messages known before it went:
        /// how its arrival on the page is told.
        var body: String
        var before: Set<String>
    }

    public private(set) var outgoing: [Outgoing] = []
    /// Messages that took the place of something shown as being sent:
    /// they are simply there, they do not arrive.
    public private(set) var placed: Set<String> = []
    private var sends = 0
    /// Why the last send, steer or stop did not go through.
    public private(set) var problem: String?
    /// A message waiting for the open task to finish.
    public struct Waiting: Identifiable, Hashable, Sendable {
        public var id: String
        public var body: String
        /// Held by the workspace (true), or kept here because the
        /// workspace is too old to hold it.
        public var onHub: Bool
        /// The files it carries.
        public var files: [Attachment] = []
    }

    /// Messages waiting behind the open task, in the order they will go:
    /// those the workspace holds, then those kept here.
    public private(set) var waiting: [Waiting] = []
    /// Kept here: the workspace could not hold them.
    private var queued: [Queued] = []

    private struct Queued {
        var body: String
        var files: [PendingFile]
    }

    /// The files that go with the next message, in the order they were added.
    public private(set) var pending: [PendingFile] = []
    /// The bytes of those not in the workspace yet (to try again).
    private var payloads: [String: Data] = [:]
    private var pendingCount = 0
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
    /// On a project's page: what the coordinator and the members handed
    /// each other, one card per exchange.
    private var cards: [Handoff] = []
    /// On a member's page: its exchanges with its colleagues, by id (they
    /// show as handoff cards, not as messages).
    private var colleagueMessages: [String: Message] = [:]
    private var reloads = 0
    /// The steps and the surroundings being fetched, while they are.
    private var details: Task<Void, Never>?
    /// The newest page of messages, as last read.
    private var newestPage: [Message] = []
    /// Looks at the workspace that failed, in a row.
    private var failures = 0

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

    public init(session: WorkspaceSession, agent: String, project: String? = nil, observed: String? = nil) {
        self.session = session
        self.agent = agent
        self.project = project
        self.observed = observed
    }

    /// How a task of mine was worked on, when its steps are known.
    public func process(of taskID: String) -> RunProcess? {
        processes[taskID]
    }

    /// A task's run as the page shows it: its words and steps, with what
    /// it reported and what was added to it where each came.
    public func run(of taskID: String) -> [RunPart] {
        runs[taskID] ?? []
    }

    private var runs: [String: [RunPart]] = [:]

    public var action: ComposerAction {
        ComposerAction.resolve(hasOpenTask: openTask != nil, hasText: !trimmedDraft.isEmpty || !pending.isEmpty)
    }

    /// Sending is off while a send is under way, while a file is still
    /// going up, or with nothing to send. Files can go without words.
    /// Takes what the agent offered: it is in the input now.
    public func takeSuggestion() {
        guard let suggestion, draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        draft = suggestion
    }

    public var canSubmit: Bool {
        guard !sending else { return false }
        if action == .stop { return true }
        return !pending.contains { $0.state == .uploading } && (!trimmedDraft.isEmpty || !readyFiles.isEmpty)
    }

    /// The files that made it to the workspace.
    private var readyFiles: [PendingFile] {
        pending.filter { if case .ready = $0.state { return true } else { return false } }
    }

    private static func ids(of files: [PendingFile]) -> [String] {
        files.compactMap { if case .ready(let id) = $0.state { return id } else { return nil } }
    }

    /// What a message says: what was typed, or (files alone) what they
    /// are, in the desktop app's words.
    static func body(_ text: String, files: [PendingFile]) -> String {
        if !text.isEmpty || files.isEmpty { return text }
        return ConversationBuilder.filesOnlyBodies(files.map(\.name))[AppLanguage.current] ?? text
    }

    // MARK: Files

    /// Adds a file to the next message and starts sending it to the
    /// workspace.
    public func attach(_ data: Data, name: String, contentType: String) async {
        pendingCount += 1
        let id = "pending-\(pendingCount)"
        var preview: Data?
        if contentType.hasPrefix("image/") {
            preview = await Task.detached { Pictures.thumbnail(data) }.value
        }
        var file = PendingFile(id: id, name: name, size: data.count, contentType: contentType, preview: preview, state: .uploading)
        guard data.count <= HubFile.maxSize else {
            file.state = .failed(L("超过 50 MB，发不了", "Larger than 50 MB: it cannot be sent"))
            pending.append(file)
            return
        }
        payloads[id] = data
        pending.append(file)
        await upload(id)
    }

    /// Something picked could not be read on this device: said under the input.
    public func couldNotRead(_ name: String) {
        problem = L("读不了「\(name)」", "Could not read \"\(name)\"")
    }

    /// Takes a file off the next message.
    public func removePending(_ id: String) {
        pending.removeAll { $0.id == id }
        payloads[id] = nil
    }

    /// Tries again to send a file that did not go up.
    public func retryPending(_ id: String) async {
        guard let index = pending.firstIndex(where: { $0.id == id }), case .failed = pending[index].state, payloads[id] != nil else { return }
        pending[index].state = .uploading
        await upload(id)
    }

    private func upload(_ id: String) async {
        guard let data = payloads[id], let file = pending.first(where: { $0.id == id }) else { return }
        let state: PendingFile.State
        do {
            let stored = try await session.hub.upload(data, name: file.name, contentType: file.contentType)
            // Already here: showing it once sent needs no download.
            await session.shelf.keep(data, as: Attachment(fileId: stored.id, name: stored.name, size: stored.size, contentType: stored.contentType))
            payloads[id] = nil
            state = .ready(fileID: stored.id)
        } catch let error as HubError where error.error == "not_found" {
            state = .failed(L("这个工作区还不能从 App 发文件，升级它的 myco 就可以", "This workspace can't take files from the app yet; update its myco"))
        } catch {
            state = .failed((error as? HubError)?.message ?? L("没传上去", "It did not go up"))
        }
        // Taken off while it was going up: nothing to mark.
        guard let index = pending.firstIndex(where: { $0.id == id }) else { return }
        pending[index].state = state
    }

    private var trimmedDraft: String {
        draft.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// What is kept of a conversation between launches.
    struct Snapshot: Codable {
        var messages: [Message]
        var traces: [String: [TraceEvent]]
        var colleagues: [Message]?
        var cards: [Handoff]?
    }

    private var snapshotName: String { "conversation-" + agent + "-" + (project ?? "") + (observed.map { "-with-" + $0 } ?? "") }
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
        processes = snapshot.traces.mapValues { RunProcess(events: RunProcess.withoutChores($0)) }
        // Closed tasks' steps are complete: no need to fetch them again.
        settled = Set((snapshot.messages + (snapshot.colleagues ?? [])).filter { !$0.isOpen }.map(\.id)).intersection(snapshot.traces.keys)
        show(cards: snapshot.cards ?? [])
    }

    /// Keeps the page fresh until the surrounding task is cancelled (the
    /// page left the screen, the app left the front). What was said is
    /// asked for every two seconds and shown as soon as it comes; how it
    /// was worked on follows by itself.
    public func run() async {
        await restore()
        while !Task.isCancelled {
            await load(whole: false)
            if await sendNextQueued() { await load(whole: false) }
            try? await Task.sleep(for: Self.pollInterval)
        }
        // Nobody is looking: what was still on its way is let go, so a
        // request that hangs does not hold up the next look.
        details?.cancel()
    }

    /// Reads the page again, all of it: what was said, and how it was
    /// worked on.
    public func reload() async {
        await load(whole: true)
        // The open task closed: what waited behind it goes out now.
        if await sendNextQueued() { await load(whole: true) }
    }

    /// A refresh the person asked for: the conversation and the
    /// workspace's lists, now.
    public func refresh() async {
        guard !refreshing else { return }
        refreshing = true
        async let lists: Void = session.refresh()
        await reload()
        await lists
        refreshing = false
    }

    /// - Parameter whole: wait for the steps and the surroundings too.
    ///   Without it they are fetched alongside: one slow answer among
    ///   them must not keep back a message that is already here.
    private func load(whole: Bool) async {
        let me = self.me
        guard !me.isEmpty else { return }
        do {
            let page = try await session.hub.exchanges(between: me, and: agent, before: nil)
            failures = 0
            unreachable = nil
            for message in page.messages { known[message.id] = message }
            if !wentBack { hasEarlier = page.more }
            newestPage = page.messages
            // What was said shows at once. (The first look shows the page
            // whole: see below.)
            if loaded { show(cards: cards) }
            // One fetch of the steps at a time. Asked to read it all now,
            // an earlier one that may be hanging is given up, not waited for.
            if whole {
                details?.cancel()
                _ = await details?.value
            }
            startDetails()
            if whole {
                _ = await details?.value
            } else if !loaded {
                // The first look shows the page whole: what was asked, how
                // it was worked on and the rest of the agent's work arrive
                // together, not in three jumps. Only when that takes long
                // is what is already here shown meanwhile.
                let patience = ContinuousClock.now + Self.patience
                while details != nil, ContinuousClock.now < patience, !Task.isCancelled {
                    try? await Task.sleep(for: .milliseconds(30))
                }
            }
            if !loaded, !Task.isCancelled { show(cards: cards) }
        } catch {
            // Leaving the page is not a failure to report.
            guard !Task.isCancelled, !(error is CancellationError) else { return }
            // One look that failed is a blip; the page says so when the
            // next fails too, or when the person asked for this one.
            failures += 1
            if failures >= 2 || refreshing {
                unreachable = (error as? HubError)?.message ?? error.localizedDescription
            }
            loaded = true
        }
    }

    /// Fetches how the tasks were worked on and what surrounds the
    /// conversation, unless that is under way already, and shows it.
    private func startDetails() {
        guard details == nil else { return }
        let around = reloads % Self.colleaguesEvery == 0
        reloads += 1
        details = Task { [weak self] in
            await self?.loadDetails(around: around)
            self?.details = nil
        }
    }

    private func loadDetails(around: Bool) async {
        var cards = self.cards
        // My tasks' steps come while the agent's other work is asked for.
        let myTasks = Self.traceable(Array(known.values), for: agent)
        async let mine: Void = loadSteps(of: myTasks)
        // (A look at two agents' exchange is only that exchange.)
        if around, observed == nil { cards = await loadSurroundings() }
        await mine
        guard !Task.isCancelled else { return }
        show(cards: cards)
        // Only the newest pages are kept between launches.
        await save(Snapshot(messages: newestPage, traces: traces, colleagues: Array(colleagueMessages.values), cards: cards))
    }

    /// How long the first look at a conversation waits for all of it
    /// before showing the part that is here.
    static let patience: Duration = .milliseconds(1500)

    /// The agent's tasks whose steps can be asked for.
    private static func traceable(_ messages: [Message], for agent: String) -> [Message] {
        messages.filter { $0.to == agent && $0.type == "task" && $0.status != "queued" }
    }

    /// Asks the workspace for several things at once: the answers by key,
    /// without those that failed.
    private static func gather<Key: Hashable & Sendable, Value: Sendable>(
        _ keys: [Key], _ fetch: @escaping @Sendable (Key) async -> Value?
    ) async -> [Key: Value] {
        await withTaskGroup(of: (Key, Value?).self) { group in
            for key in keys { group.addTask { (key, await fetch(key)) } }
            var answers: [Key: Value] = [:]
            for await (key, value) in group {
                if let value { answers[key] = value }
            }
            return answers
        }
    }

    /// Loads the page before the oldest message shown. True if it brought any.
    @discardableResult
    public func loadEarlier() async -> Bool {
        guard hasEarlier, !loadingEarlier, let oldest = known.values.map(\.createdAt).min() else { return false }
        loadingEarlier = true
        defer { loadingEarlier = false }
        guard let page = try? await session.hub.exchanges(between: me, and: agent, before: oldest) else { return false }
        wentBack = true
        hasEarlier = page.more && !page.messages.isEmpty
        for message in page.messages { known[message.id] = message }
        show(cards: cards)
        return !page.messages.isEmpty
    }

    /// Someone who is not an agent: what such a one says is a message,
    /// never a handoff card.
    private func isPerson(_ name: String) -> Bool {
        name.hasPrefix("@") || (session.agent(name).map { Naming.kind(of: $0) == .human } ?? false)
    }

    private func show(cards: [Handoff]) {
        self.cards = cards
        let me = self.me
        // What was shown as being sent is on the page for real now.
        if !outgoing.isEmpty {
            outgoing.removeAll { entry in
                guard let arrived = known.values.first(where: { $0.from == me && $0.body == entry.body && !entry.before.contains($0.id) }) else {
                    return false
                }
                placed.insert(arrived.id)
                return true
            }
        }
        // Mine with the agent. What the agent and its colleagues said to
        // each other is not part of it: one card per exchange instead.
        // Not what I took back, and not what I took out of my view (on any
        // of my devices): a task taken out takes its updates and what was
        // added to it along; its answer stays until taken out too.
        let hidden = session.hidden
        func shown(_ message: Message) -> Bool {
            if withdrawn.contains(message.id) || hidden.contains(message.id) { return false }
            if message.type == "progress" || message.type == "steer", let task = message.replyTo, hidden.contains(task) { return false }
            return true
        }
        let all = known.filter { shown($0.value) }
        var handoffs: [Handoff] = []
        if observed == nil {
            handoffs = project != nil ? cards : ConversationBuilder.handoffs(
                between: colleagueMessages.values.filter { message in
                    known[message.id] == nil && shown(message) && message.from != me && message.to != me
                        && !isPerson(message.from) && !isPerson(message.to)
                }, and: agent)
        }
        // Which open runs have stopped for now: not at work.
        let reports = Dictionary(grouping: all.values.filter { $0.type == "progress" }, by: { $0.replyTo ?? $0.threadId })
        var paused: Set<String> = []
        for task in all.values where task.to == agent && task.isOpen {
            let said = (reports[task.id] ?? []).sorted { $0.createdAt < $1.createdAt }
            if RunProcess.isPaused(events: traces[task.id] ?? [], task: task, reports: said, now: Date()) { paused.insert(task.id) }
        }
        var tracedPlans: [String: (items: [PlanItem], at: Date?)] = [:]
        for (taskID, process) in processes {
            if let plan = process.plan { tracedPlans[taskID] = (plan, process.planAt) }
        }
        // Each run with its updates and additions put in where they came.
        var runs: [String: [RunPart]] = [:]
        for (taskID, process) in processes where process.hasSteps {
            let extras = ConversationBuilder.runExtras(of: taskID, agent: agent, messages: Array(all.values))
            runs[taskID] = process.parts(reports: extras.reports, added: extras.added, live: all[taskID]?.isOpen ?? false)
        }
        if runs != self.runs {
            self.runs = runs
            revision += 1
        }
        let built = ConversationBuilder.items(
            me: me, agent: agent, messages: Array(all.values), handoffs: handoffs,
            processes: Set(processes.filter { $0.value.hasSteps }.keys), tracedPlans: tracedPlans, paused: paused,
            status: { self.session.status(of: $0) }, now: Date())
        let mine = Array(known.values).filter { shown($0) }
        let held = ConversationBuilder.queuedBehind(mine).filter { $0.from == me }
            .map { Waiting(id: $0.id, body: $0.body, onHub: true, files: ConversationBuilder.files(of: $0)) }
        let local = queued.enumerated().map { Waiting(id: "local-\($0.offset)", body: $0.element.body, onHub: false) }
        if waiting != held + local { waiting = held + local }
        // Nothing new: leave the list alone, so the page does not redraw.
        if built != items {
            items = built
            revision += 1
        }
        // One set aside takes what I type only once its steps say it is at
        // it again (the same rule as its line in the conversation).
        let moving = Set(mine.filter { $0.status == "held" && $0.to == agent && $0.isOpen && !paused.contains($0.id) }.map(\.id))
        openTask = ConversationBuilder.openTask(me: me, messages: mine, paused: paused, moving: moving)
        let offered = ConversationBuilder.suggestion(me: me, messages: mine, open: openTask)
        if offered != suggestion { suggestion = offered }
        loaded = true
    }

    /// A new message of mine: in the thread my last exchange was in. On a
    /// project's page, the first one of this visit starts a thread in the
    /// project (as the desktop app does).
    private func request(_ body: String, after: String? = nil, attachments: [String] = []) -> SendRequest {
        if project != nil, !startedProjectThread {
            startedProjectThread = true
            return SendRequest(to: agent, body: body, project: project, after: after, attachments: attachments)
        }
        let thread = ConversationBuilder.threadToContinue(me: me, messages: Array(known.values))
        return SendRequest(
            to: agent, body: body, threadId: thread, project: thread == nil ? project : nil, after: after, attachments: attachments)
    }

    private func save(_ snapshot: Snapshot) async {
        guard let store = session.snapshots, let data = try? HubJSON.encoder().encode(snapshot), data != lastSaved else { return }
        lastSaved = data
        await store.setSnapshot(data, of: session.id, name: snapshotName)
    }

    /// What surrounds my own exchange with the agent, as the desktop app
    /// gathers it. On a project's page (the agent is its coordinator): what
    /// it and the members handed each other, returned as cards. On a
    /// member's own page: its exchanges with its colleagues in its
    /// projects, kept to make its cards from.
    private func loadSurroundings() async -> [Handoff] {
        let hub = session.hub
        let agent = agent
        // Which projects to look through: this page's, or the agent's.
        var scanned: [Project] = []
        if let project {
            if let found = session.project(project) { scanned = [found] }
        } else {
            scanned = Array(session.projects(of: agent).prefix(Self.projectsScanned))
        }
        // Everything recent and each project's threads are asked for together.
        async let recent = try? await hub.recent(.everything)
        let own = await Self.gather(scanned.map(\.name)) { name in try? await hub.recent(.project(name)) }
        let everything = await recent ?? []
        func feed(_ project: Project) -> [RecentItem] {
            ProjectFeed.items(of: project, own: own[project.name] ?? [], everything: everything)
        }
        if project != nil {
            guard let found = scanned.first else { return [] }
            let mine = session.me
            return ConversationBuilder.handoffs(ofHome: feed(found), coordinator: agent) { $0 == mine || isPerson($0) }
        }
        var colleagues: [String] = []
        for project in scanned {
            let names = ProjectFeed.colleagues(of: agent, in: feed(project), coordinator: session.coordinator(of: project))
            for name in names where !colleagues.contains(name) { colleagues.append(name) }
        }
        // And its exchanges with each colleague, together too.
        let pages = await Self.gather(Array(colleagues.prefix(Self.maxColleagues))) { colleague in
            try? await hub.exchanges(between: colleague, and: agent, before: nil)
        }
        for page in pages.values {
            for message in page.messages { colleagueMessages[message.id] = message }
        }
        return []
    }

    /// How many of an agent's projects are looked through for its colleagues.
    static let projectsScanned = 3

    /// Fetches the steps of the agent's newest tasks: all of them once for a
    /// closed task, the new ones each time for one still being worked on.
    private func loadSteps(of tasks: [Message]) async {
        let newest = tasks.sorted { $0.createdAt > $1.createdAt }.prefix(Self.tracedTasks).filter { !settled.contains($0.id) }
        guard !newest.isEmpty else { return }
        let hub = session.hub
        let from = Dictionary(newest.map { ($0.id, traces[$0.id]?.last?.seq ?? 0) }, uniquingKeysWith: { first, _ in first })
        // All of them at once: a page of six tasks is one wait, not six.
        let fetched = await Self.gather(Array(from.keys)) { id in try? await hub.trace(of: id, after: from[id] ?? 0) }
        for task in newest {
            guard let trace = fetched[task.id] else { continue }
            // What is known now, not when asked: another refresh may have
            // brought some of these steps meanwhile.
            let known = traces[task.id] ?? []
            let fresh = trace.events.filter { $0.seq > (known.last?.seq ?? 0) }
            if !fresh.isEmpty || processes[task.id] == nil {
                let events = known + fresh
                traces[task.id] = events
                // (without the turns it woke for only to keep listening)
                let process = RunProcess(events: RunProcess.withoutChores(events))
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
        let files = pending
        let attachments = Self.ids(of: files)
        let body = Self.body(text, files: readyFiles)
        var going: String?
        if action != .stop {
            // On the page at once, as being sent: the person does not wait
            // for the workspace to see what they said.
            sends += 1
            let entry = Outgoing(
                id: "sending-\(sends)", text: text, files: readyFiles.compactMap(Self.attachment),
                body: body, before: Set(known.keys))
            going = entry.id
            outgoing.append(entry)
            takeDraft()
        }
        sending = true
        do {
            var sent: Message?
            switch action {
            case .send:
                sent = try await session.hub.send(request(body, attachments: attachments))
            case .steer:
                guard let task = openTask else { break }
                do {
                    try await session.hub.steer(task.id, body: body, attachments: attachments)
                } catch let error as HubError where error.error == "invalid_state" {
                    // The task ended just now: this is a new message.
                    sent = try await session.hub.send(request(body, attachments: attachments))
                }
            case .stop:
                if let task = openTask { try await session.hub.stop(task.id) }
            }
            problem = nil
            if action != .stop { dropSentPayloads() }
            // The workspace has it: the real message takes the place of
            // the one shown as being sent (`show`), with no wait between.
            if let sent {
                known[sent.id] = sent
                show(cards: cards)
            }
        } catch {
            // Give it all back: nothing typed or picked is lost to a failed send.
            outgoing.removeAll { $0.id == going }
            if action != .stop { giveBack(text, files) }
            problem = (error as? HubError)?.message ?? error.localizedDescription
        }
        sending = false
        await reload()
        // An addition has no message handed back: it is on the page by
        // now. (When the page could not be read just now, it stays shown
        // as sent until the next look finds it.)
        if failures == 0 { outgoing.removeAll { $0.id == going } }
    }

    /// A file that went up, as the message will carry it.
    private static func attachment(_ file: PendingFile) -> Attachment? {
        guard case .ready(let id) = file.state else { return nil }
        return Attachment(fileId: id, name: file.name, size: Int64(file.size), contentType: file.contentType)
    }

    /// Where a file waiting above the input is on this device, to look at
    /// it before it goes: one still going up (or that did not) is written
    /// out from memory, one that went up is on the shelf already.
    public func preview(ofPending id: String) async -> URL? {
        guard let file = pending.first(where: { $0.id == id }) else { return nil }
        if let attachment = Self.attachment(file) {
            return try? await session.shelf.url(of: attachment)
        }
        guard let data = payloads[id] else { return nil }
        let folder = URL.temporaryDirectory.appending(path: "myco-pending/\(id)")
        let place = folder.appending(path: FileShelf.fileName(file.name))
        do {
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            try data.write(to: place, options: .atomic)
            return place
        } catch {
            return nil
        }
    }

    /// Empties the input: its words and files are on their way.
    private func takeDraft() {
        draft = ""
        pending = []
    }

    /// The bytes kept to try an upload again are only those of files
    /// still waiting to go.
    private func dropSentPayloads() {
        let waiting = Set(pending.map(\.id))
        payloads = payloads.filter { waiting.contains($0.key) }
    }

    /// Puts back what a failed send took, before anything added since.
    private func giveBack(_ text: String, _ files: [PendingFile]) {
        if draft.isEmpty { draft = text }
        pending = files + pending
    }

    // MARK: For a voice call (ConversationVoice)

    /// Every message between me and the agent seen so far, oldest first.
    public var exchanged: [Message] {
        known.values.sorted { $0.createdAt < $1.createdAt }
    }

    /// Says something to the agent as if it were typed and sent: added to
    /// the open task, else a new message. What is in the input stays.
    /// Answers the task it went to.
    public func tell(_ body: String) async throws -> (task: String, steered: Bool) {
        if let task = openTask {
            do {
                try await session.hub.steer(task.id, body: body, attachments: [])
                // The page catches up by itself: the call does not wait for it.
                Task { await reload() }
                return (task.id, true)
            } catch let error as HubError where error.error == "invalid_state" {
                // The task ended just now: this is a new message.
            }
        }
        let sent = try await session.hub.send(request(body))
        // It is the open task from now on, before the page has read it
        // back: what is said next is added to it.
        known[sent.id] = sent
        show(cards: cards)
        Task { await reload() }
        return (sent.id, false)
    }

    /// Stops the open task; false when there is none.
    public func stopOpenTask() async throws -> Bool {
        guard let task = openTask else { return false }
        try await session.hub.stop(task.id)
        await reload()
        return true
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
        guard canSubmit, action != .stop else { return }
        guard let task = openTask else {
            await submit()
            return
        }
        let text = trimmedDraft
        let files = pending
        let attachments = Self.ids(of: files)
        let body = Self.body(text, files: readyFiles)
        takeDraft()
        sending = true
        do {
            _ = try await session.hub.send(request(body, after: task.id, attachments: attachments))
            problem = nil
            dropSentPayloads()
        } catch let error as HubError where error.error == "invalid_argument" {
            queued.append(Queued(body: body, files: files.filter { if case .ready = $0.state { return true } else { return false } }))
            dropSentPayloads()
        } catch {
            giveBack(text, files)
            problem = (error as? HubError)?.message ?? error.localizedDescription
        }
        sending = false
        await reload()
    }

    /// Takes a waiting message back: it will not be sent. `edit` puts its
    /// words back in the input.
    public func withdraw(_ item: Waiting, edit: Bool) async {
        var names = item.files.map(\.name)
        if item.onHub {
            do {
                try await session.hub.stop(item.id)
                withdrawn.insert(item.id)
                problem = nil
            } catch {
                problem = (error as? HubError)?.message ?? error.localizedDescription
                return
            }
            if edit { restore(item.files) }
        } else if let index = waiting.filter({ !$0.onHub }).firstIndex(of: item), queued.indices.contains(index) {
            let taken = queued.remove(at: index)
            names = taken.files.map(\.name)
            if edit { pending = taken.files + pending }
        }
        // Its words go back in the box; the sentence written for files
        // sent alone does not, the files say it.
        if edit {
            let madeUp = ConversationBuilder.filesOnlyBodies(names).values.contains(item.body)
            draft = madeUp ? "" : item.body
        }
        show(cards: cards)
    }

    /// Puts the files of a message taken back into the box again. They are
    /// in the workspace already: nothing is uploaded a second time.
    private func restore(_ files: [Attachment]) {
        for file in files {
            pendingCount += 1
            let id = "pending-\(pendingCount)"
            pending.append(PendingFile(
                id: id, name: file.name, size: Int(file.size), contentType: file.contentType ?? "", preview: nil,
                state: .ready(fileID: file.fileId)))
            guard file.isPicture else { continue }
            // A picture gets its small preview back once it is here.
            Task {
                guard let url = try? await session.shelf.url(of: file) else { return }
                let preview = await Task.detached { (try? Data(contentsOf: url)).flatMap { Pictures.thumbnail($0) } }.value
                if let index = pending.firstIndex(where: { $0.id == id }) { pending[index].preview = preview }
            }
        }
    }

    /// Takes a message out of my view of this conversation (nothing is
    /// deleted), or with `hidden: false` puts it back.
    public func setHidden(_ ids: [String], hidden: Bool = true) async {
        do {
            try await session.setHidden(ids, hidden: hidden)
            problem = nil
        } catch {
            problem = (error as? HubError)?.message ?? error.localizedDescription
        }
        show(cards: cards)
    }

    /// Sends the first waiting message once nothing is open. True if it went.
    private func sendNextQueued() async -> Bool {
        guard openTask == nil, !sending, let next = queued.first else { return false }
        sending = true
        defer { sending = false }
        do {
            _ = try await session.hub.send(request(next.body, attachments: Self.ids(of: next.files)))
            queued.removeFirst()
            return true
        } catch {
            problem = (error as? HubError)?.message ?? error.localizedDescription
            return false
        }
    }
}
