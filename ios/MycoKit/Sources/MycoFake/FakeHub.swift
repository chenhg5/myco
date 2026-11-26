import Foundation
import MycoAPI

/// A workspace in memory, behind the same protocol as a real hub: for
/// previews, UI tests and trying the app without a hub. It covers the
/// states pages must show: waiting, at work, done, failed, possibly stuck,
/// away. What is sent to it gets taken on after a moment and answered a
/// few seconds later, so adding to a running task and stopping it can be
/// tried by hand.
public actor FakeHub: HubAPI {
    private var remoteHosts: [RemoteComputer] = []
    private var remoteCalls: [String: RemoteConnection] = [:]
    public private(set) var remoteOffers: [RemoteOffer] = []
    public private(set) var remoteDisconnected: [String] = []
    private var holdRemote = false
    private var remoteGate: CheckedContinuation<Void, Never>?
    public func seedRemote(_ computers: [RemoteComputer], hold: Bool = false) { remoteHosts = computers; holdRemote = hold }
    public func releaseRemote() { remoteGate?.resume(); remoteGate = nil; holdRemote = false }
    public func remoteComputers() async throws -> [RemoteComputer] { remoteHosts }
    public func connectRemote(machine: String, offer: RemoteOffer) async throws -> RemoteConnection {
        remoteOffers.append(offer)
        let result = RemoteConnection(id: "rd_\(remoteOffers.count)", machine: machine, state: "pending")
        if holdRemote { await withCheckedContinuation { remoteGate = $0 } }
        remoteCalls[result.id] = result
        return result
    }
    public func remoteConnection(_ id: String) async throws -> RemoteConnection {
        guard let result = remoteCalls[id] else { throw HubError(error: "not_found", message: "Remote connection not found") }
        return result
    }
    public func disconnectRemote(_ id: String) async throws { remoteCalls[id] = nil; remoteDisconnected.append(id) }

    public static let me = "@console:alice"

    private var agents: [AgentStatus]
    private var machines: [Machine]
    private var projectList: [Project]
    private var messages: [Message]
    private var seq = 0
    /// Tasks sent from the app, by id: when each was sent.
    private var simulated: [String: Date] = [:]
    /// Makes every call fail, to see the app without its workspace.
    private var unreachable = false
    /// How long every answer takes: a slow connection, to see what
    /// loading looks like.
    private var latency: Duration
    /// The files kept here, by id.
    private var files: [String: (file: HubFile, data: Data)] = [:]
    /// The messages taken out of my view.
    private var hiddenIDs: [String] = []
    /// Tasks whose session ended its turn (open, but waiting).
    private var turnEnded: Set<String> = []
    /// Tasks with no steps of their own (see Scenario.World.silent).
    private var silent: Set<String> = []
    /// Acts like a hub from before messages could be taken out of view.
    private var tooOldForHidden = false
    /// Acts like a hub from before files could be sent from the app.
    private var tooOldForFiles = false

    public init(scenario: Scenario = .team, now: Date = Date(), latency: Duration = .zero) {
        self.latency = latency
        let world = scenario.build(now: now)
        agents = world.agents
        machines = world.machines
        projectList = world.projects
        messages = world.messages
        files = Dictionary(world.files.map { ($0.file.id, $0) }, uniquingKeysWith: { first, _ in first })
        turnEnded = world.turnEnded
        silent = world.silent
        automationList = world.automations
        pageList = world.pages.map(\.page)
        appList = world.apps.map(\.app)
        appFiles = Dictionary(world.apps.map { ($0.app.id, $0.files) }, uniquingKeysWith: { first, _ in first })
        handoverList = world.handovers
        pinList = world.pins
        pageFiles = Dictionary(world.pages.map { ($0.page.id, $0.files) }, uniquingKeysWith: { first, _ in first })
        automationRuns = Dictionary(grouping: world.automations.compactMap(\.lastRun), by: { $0.automationId ?? "" })
        workflowList = world.workflows
        runList = world.runs
        if scenario != .empty {
            (noticeList, usageRows) = Self.seeded(now: now)
            (watchList, goalList, goalRounds) = Self.seededWork(now: now)
            // The account limits its owner sees: the coordinator's Claude
            // account comfortable, 后端工程师's Codex nearly out.
            for index in agents.indices {
                switch agents[index].agent.name {
                case "cloud-lead":
                    agents[index].agent.runtime?.quota = Quota(provider: "claude", source: "claude", plan: "claude_subscription", at: now.addingTimeInterval(-120), windows: [
                        QuotaWindow(name: "five_hour", durationSec: 18_000, used: 0.38, resetsAt: now.addingTimeInterval(9_000)),
                        QuotaWindow(name: "seven_day", durationSec: 604_800, used: 0.41, resetsAt: now.addingTimeInterval(300_000)),
                    ])
                case "studio-codex-7f3a":
                    agents[index].agent.runtime?.quota = Quota(provider: "codex", source: "codex", plan: "chatgpt_subscription", at: now.addingTimeInterval(-300), windows: [
                        QuotaWindow(name: "primary", durationSec: 18_000, used: 0.86, resetsAt: now.addingTimeInterval(4_000)),
                        QuotaWindow(name: "secondary", durationSec: 604_800, used: 0.52, resetsAt: now.addingTimeInterval(400_000)),
                    ])
                default:
                    break
                }
            }
        }
        if scenario == .remote {
            remoteHosts = [RemoteComputer(machine: "app:fixture", name: "Fixture Mac", os: "darwin", control: true)]
            let page = Page(id: "pg_remote", name: "Remote controller probe", publisher: FakeHub.me, url: FakeHub.demoAddress.absoluteString + "/pages/pg_remote", createdAt: now, updatedAt: now)
            pageList.insert(page, at: 0)
            pageFiles[page.id] = ["index.html": ("text/html; charset=utf-8", RemotePageFixture.html)]
        }
        if scenario == .media {
            let page = Page(id: MediaPageFixture.id, name: "Media probe", publisher: FakeHub.me, url: FakeHub.demoAddress.absoluteString + "/pages/" + MediaPageFixture.id, createdAt: now, updatedAt: now)
            pageList.insert(page, at: 0)
            pageFiles[page.id] = ["index.html": ("text/html; charset=utf-8", MediaPageFixture.html)]
        }
        if scenario == .links {
            let page = Page(id: LinkPageFixture.id, name: "影视", publisher: "vps-codex-90aa", url: FakeHub.demoAddress.absoluteString + "/pages/" + LinkPageFixture.id, createdAt: now, updatedAt: now)
            pageList.insert(page, at: 0)
            pageFiles[page.id] = ["index.html": ("text/html; charset=utf-8", LinkPageFixture.html)]
            messages.append(Message(
                id: "cinema-link", from: "vps-codex-90aa", to: FakeHub.me, type: "note",
                body: "第一集在这：\(LinkPageFixture.link(episode: "第1集"))\n\n第二集：\(LinkPageFixture.link(episode: "第2集"))", status: "done",
                createdAt: now.addingTimeInterval(-30)))
        }
    }

    // MARK: Watches and goals

    private var watchList: [Watch] = []
    private var goalList: [Goal] = []
    private var goalRounds: [String: [GoalRound]] = [:]
    /// The demo's workspace is from before watches and goals.
    private var withoutWorkState = false

    public func setWithoutWorkState(_ without: Bool) { withoutWorkState = without }

    /// 运维 waits for CI in the background; 后端工程师 works on a goal.
    private static func seededWork(now: Date) -> ([Watch], [Goal], [String: [GoalRound]]) {
        let watch = Watch(
            id: "wat_demo1", agent: "vps-codex-90aa", what: "等 CI 跑完再发版", kind: "workflow",
            startedAt: now.addingTimeInterval(-720), deadline: now.addingTimeInterval(1_080), lastCheckAt: now.addingTimeInterval(-30),
            lastResult: "还在跑：12/18 个任务", checks: 24)
        let goal = Goal(
            id: "gol_demo1", agent: "studio-codex-7f3a", project: "myco-ios", objective: "把推送接口做完并合进 main", doneWhen: "接口有测试、CI 绿、PR 合并",
            everySec: 1_800, maxRounds: 16, deadline: now.addingTimeInterval(24_000), round: 3, noProgress: 0,
            lastSummary: "设备登记的接口写完了，在补测试", nextStep: "补完测试，开 PR", nextWakeAt: now.addingTimeInterval(720),
            startedAt: now.addingTimeInterval(-4_800), createdBy: Self.me)
        let rounds = [
            GoalRound(round: 1, startedAt: now.addingTimeInterval(-4_800), endedAt: now.addingTimeInterval(-4_200), summary: "看了现有的唤醒方式", nextStep: "写设备登记", progressed: true),
            GoalRound(round: 2, startedAt: now.addingTimeInterval(-3_000), endedAt: now.addingTimeInterval(-2_300), summary: "设备登记的接口写完了", nextStep: "补测试", progressed: true),
            GoalRound(round: 3, startedAt: now.addingTimeInterval(-1_100), endedAt: now.addingTimeInterval(-1_080), summary: "设备登记的接口写完了，在补测试", nextStep: "补完测试，开 PR", progressed: true),
        ]
        return ([watch], [goal], [goal.id: rounds])
    }

    private func checkWork() async throws {
        try await check()
        if withoutWorkState { throw HubError(error: "not_found", message: "not found", status: 404) }
    }

    public func watches(agent: String, all: Bool) async throws -> [Watch] {
        try await checkWork()
        return watchList.filter { $0.agent == agent && (all || $0.isOn) }
    }

    public func stopWatch(_ id: String) async throws -> Watch {
        try await checkWork()
        guard let index = watchList.firstIndex(where: { $0.id == id }), watchList[index].isOn else {
            throw HubError(error: "conflict", message: "the watch has ended", status: 409)
        }
        watchList[index].state = "stopped"
        watchList[index].endedAt = Date()
        watchList[index].endedBy = Self.me
        return watchList[index]
    }

    public func extendWatch(_ id: String, seconds: Int) async throws -> Watch {
        try await checkWork()
        guard let index = watchList.firstIndex(where: { $0.id == id }), watchList[index].isOn else {
            throw HubError(error: "conflict", message: "the watch has ended", status: 409)
        }
        watchList[index].deadline = Date().addingTimeInterval(TimeInterval(seconds))
        return watchList[index]
    }

    public func goals(agent: String, all: Bool) async throws -> [Goal] {
        try await checkWork()
        return goalList.filter { $0.agent == agent && (all || $0.isOn) }
    }

    public func goal(_ id: String) async throws -> (goal: Goal, rounds: [GoalRound]) {
        try await checkWork()
        guard let goal = goalList.first(where: { $0.id == id }) else { throw HubError(error: "not_found", message: "no such goal", status: 404) }
        return (goal, goalRounds[id] ?? [])
    }

    public func startGoal(_ request: GoalRequest) async throws -> Goal {
        try await checkWork()
        if goalList.contains(where: { $0.agent == request.agent && $0.isOn }) {
            throw HubError(error: "conflict", message: "it already has a goal", status: 409)
        }
        seq += 1
        let now = Date()
        let every = request.everySec ?? 1_800
        let goal = Goal(
            id: "gol_fake\(seq)", agent: request.agent, project: request.project, thread: request.thread, objective: request.objective,
            doneWhen: request.doneWhen, everySec: every, baseEverySec: every, maxRounds: request.maxRounds ?? 16,
            deadline: now.addingTimeInterval(TimeInterval(request.forSec ?? 28_800)), nextWakeAt: now.addingTimeInterval(TimeInterval(every)),
            startedAt: now, createdBy: Self.me)
        goalList.append(goal)
        return goal
    }

    public func controlGoal(_ id: String, _ action: String) async throws -> Goal {
        try await checkWork()
        guard let index = goalList.firstIndex(where: { $0.id == id }), goalList[index].isOn else {
            throw HubError(error: "conflict", message: "the goal has ended", status: 409)
        }
        switch action {
        case "pause": goalList[index].state = "paused"; goalList[index].nextWakeAt = nil
        case "resume": goalList[index].state = "active"; goalList[index].nextWakeAt = Date().addingTimeInterval(TimeInterval(goalList[index].everySec))
        case "stop":
            goalList[index].state = "stopped"
            goalList[index].endedAt = Date()
            goalList[index].endedBy = Self.me
            goalList[index].outcome = "stopped"
            goalList[index].nextWakeAt = nil
        default: throw HubError(error: "invalid_argument", message: "unknown action", status: 400)
        }
        return goalList[index]
    }

    public func setGoalInterval(_ id: String, seconds: Int) async throws -> Goal {
        try await checkWork()
        guard let index = goalList.firstIndex(where: { $0.id == id }), goalList[index].isOn else {
            throw HubError(error: "conflict", message: "the goal has ended", status: 409)
        }
        goalList[index].everySec = seconds
        goalList[index].baseEverySec = seconds
        if goalList[index].state == "active" { goalList[index].nextWakeAt = Date().addingTimeInterval(TimeInterval(seconds)) }
        return goalList[index]
    }

    // MARK: Notices and usage

    private var noticeList: [HubNotice] = []
    private var usageRows: [UsageRow] = []

    /// Two things waiting for me (运维 asks; a task of 后端工程师's failed),
    /// one settled; two weeks of tokens.
    private static func seeded(now: Date) -> ([HubNotice], [UsageRow]) {
        let noticeList = [
            HubNotice(seq: 1, id: "ntc_1", kind: "answered", tier: "results", target: "agent:studio-claude-c41f", actor: "studio-claude-c41f",
                   createdAt: now.addingTimeInterval(-7_200), readAt: now.addingTimeInterval(-7_000), resolvedAt: now.addingTimeInterval(-7_000)),
            HubNotice(seq: 2, id: "ntc_2", kind: "failed", target: "agent:studio-codex-7f3a", project: "myco-ios", actor: "studio-codex-7f3a",
                   subject: "跑测试", createdAt: now.addingTimeInterval(-3_600)),
            HubNotice(seq: 3, id: "ntc_3", kind: "asking", target: "agent:vps-codex-90aa", actor: "vps-codex-90aa",
                   subject: "要不要现在重启 hub", createdAt: now.addingTimeInterval(-900)),
        ]
        let calendar = Calendar(identifier: .gregorian)
        var utc = calendar
        utc.timeZone = TimeZone(identifier: "UTC")!
        let format = DateFormatter()
        format.calendar = utc
        format.timeZone = utc.timeZone
        format.dateFormat = "yyyy-MM-dd"
        var usageRows: [UsageRow] = []
        let people: [(agent: String, project: String, model: String, scale: Int64)] = [
            ("cloud-lead", "myco-ios", "claude-opus-5-5", 9),
            ("studio-codex-7f3a", "myco-ios", "gpt-6-sol", 14),
            ("studio-claude-2b9c", "myco-ios", "claude-sonnet-5-5", 6),
            ("vps-codex-90aa", "", "gpt-6-sol", 3),
        ]
        for back in 0..<14 {
            guard let day = utc.date(byAdding: .day, value: -back, to: now) else { continue }
            for (index, one) in people.enumerated() {
                // (a little different each day, the same every launch)
                let wave = Int64((back * 7 + index * 3) % 5 + 3)
                usageRows.append(UsageRow(
                    day: format.string(from: day), agent: one.agent, project: one.project.isEmpty ? nil : one.project, model: one.model,
                    calls: wave * one.scale, inputTokens: wave * one.scale * 21_000, cacheWriteTokens: wave * one.scale * 4_000,
                    cacheReadTokens: wave * one.scale * 90_000, outputTokens: wave * one.scale * 6_500, costUsd: Double(wave * one.scale) * 0.41))
            }
        }
        return (noticeList, usageRows)
    }

    public func notices(after: Int64?, limit: Int) async throws -> HubNoticePage {
        try await check()
        let list = noticeList.filter { $0.seq > (after ?? 0) }.prefix(limit)
        return HubNoticePage(notices: Array(list), last: noticeList.last?.seq ?? 0, badge: noticeList.filter(\.open).count)
    }

    public func readNotices(target: String?) async throws {
        try await check()
        let now = Date()
        for index in noticeList.indices where target == nil || noticeList[index].target == target {
            noticeList[index].readAt = noticeList[index].readAt ?? now
            // (asking, decision and approval are settled by answering them)
            if !["asking", "decision", "approval"].contains(noticeList[index].kind) {
                noticeList[index].resolvedAt = noticeList[index].resolvedAt ?? now
            }
        }
    }

    /// Settles what was asking for an answer that came (as the hub does).
    func settleAsking(of agent: String) {
        for index in noticeList.indices where noticeList[index].kind == "asking" && noticeList[index].target == "agent:" + agent {
            noticeList[index].resolvedAt = noticeList[index].resolvedAt ?? Date()
        }
    }

    public func usage(from: String, to: String, project: String?) async throws -> UsageReport {
        try await check()
        let rows = usageRows.filter { $0.day >= from && $0.day <= to && (project == nil || ($0.project ?? "-") == project || (project == "-" && $0.project == nil)) }
        return UsageReport(from: from, to: to, rows: rows)
    }

    /// Adds a page made of these files (by path, with their types): to
    /// open a built app in the demo (`-myco-page-dir`).
    public func addPage(_ page: Page, files: [String: (type: String, text: String)]) {
        pageList.insert(page, at: 0)
        pageFiles[page.id] = files
    }

    public func setLatency(_ value: Duration) {
        latency = value
    }

    /// How long a task's steps take to come, on top of that: one slow
    /// answer among the quick ones.
    private var slowSteps: Duration = .zero

    public func setSlowSteps(_ value: Duration) {
        slowSteps = value
    }

    /// The voice settings kept here, and whether this acts like a hub from
    /// before they were kept.
    private var voice: VoiceDocument?
    private var tooOldForVoice = false

    public func setTooOldForVoice(_ value: Bool) {
        tooOldForVoice = value
    }

    /// What is kept here (for tests: as if another device had set it).
    public func setVoice(_ document: VoiceDocument?) {
        voice = document
    }

    public func keptVoice() -> VoiceDocument? {
        voice
    }

    public func voiceSettings() async throws -> VoiceDocument {
        try await check()
        if tooOldForVoice { throw HubError(error: "not_found", message: "not found") }
        return voice ?? VoiceDocument(updatedAt: Date(timeIntervalSince1970: -62_135_596_800))
    }

    public func putVoiceSettings(_ document: VoiceDocument) async throws -> VoiceDocument {
        try await check()
        if tooOldForVoice { throw HubError(error: "not_found", message: "not found") }
        if let voice, voice.updatedAt > document.updatedAt { return voice }
        voice = document
        return document
    }

    public func setTooOldForHidden(_ value: Bool) {
        tooOldForHidden = value
    }

    /// Changes a message in place (for tests: a task set aside, stopped, ...).
    public func change(_ id: String, _ change: @Sendable (inout Message) -> Void) {
        guard let index = messages.firstIndex(where: { $0.id == id }) else { return }
        change(&messages[index])
    }

    /// Adds a message as it is (for tests).
    public func add(_ message: Message) {
        messages.append(message)
    }

    public func setTooOldForFiles(_ value: Bool) {
        tooOldForFiles = value
    }

    /// What was sent to it, for tests.
    public func allMessages() -> [Message] {
        messages
    }

    public func setUnreachable(_ value: Bool) {
        unreachable = value
    }

    /// Every call: the wait of a slow connection, then whether the
    /// workspace answers at all.
    private func check() async throws {
        if latency > .zero { try? await Task.sleep(for: latency) }
        if unreachable { throw HubError.unreachable("The workspace did not answer.") }
    }

    public func whoami() async throws -> Whoami {
        try await check()
        return Whoami(member: Member(id: "mem_demo", name: "alice", displayName: "Alice"))
    }

    public func overview() async throws -> Overview {
        try await check()
        advance()
        return Overview(generatedAt: Date(), machines: machines, agents: agents.map(withLoad))
    }

    public func projects() async throws -> [Project] {
        try await check()
        return projectList
    }

    public func recent(_ scope: RecentScope) async throws -> [RecentItem] {
        try await check()
        advance()
        var project: String?
        if case .project(let name) = scope { project = name }
        // What I talk to whose messages say it: one row per party.
        if case .search(let text) = scope {
            let mine = messages.filter { ($0.from == Self.me || $0.to == Self.me) && $0.body.localizedCaseInsensitiveContains(text) }
            let byParty = Dictionary(grouping: mine, by: { $0.from == Self.me ? $0.to : $0.from })
            return byParty.compactMap { party, said -> RecentItem? in
                guard let last = said.max(by: { $0.createdAt < $1.createdAt }) else { return nil }
                return RecentItem(threadId: last.threadId, participants: [Self.me, party], updatedAt: last.updatedAt, last: last)
            }
            .sorted { $0.updatedAt > $1.updatedAt }
        }
        let threads = Dictionary(grouping: messages.filter { project == nil || $0.project == project }, by: \.threadId)
        return threads.compactMap { id, all -> RecentItem? in
            let sorted = all.sorted { $0.createdAt < $1.createdAt }
            guard let first = sorted.first, let last = sorted.last else { return nil }
            let tasks = sorted.filter { $0.type == "task" }
            let task = tasks.last
            let open = tasks.last { $0.isOpen }
            let answer = task.flatMap { t in sorted.last { $0.replyTo == t.id && $0.type == "reply" } }
            var state = ""
            if let task {
                switch task.status {
                case "done": state = "done"
                case "failed": state = "failed"
                case "acked": state = "working"
                default: state = "queued"
                }
            }
            var parties: [String] = []
            for m in sorted { for p in [m.from, m.to] where !parties.contains(p) { parties.append(p) } }
            var item = RecentItem(
                threadId: id, participants: parties, updatedAt: sorted.map(\.updatedAt).max() ?? first.createdAt,
                last: last, task: task, answer: answer, open: open, state: state, project: first.project)
            item.first = first
            return item
        }
        .sorted { $0.updatedAt > $1.updatedAt }
    }

    /// How many messages one page of a conversation holds here.
    public static let pageSize = 60

    public func exchanges(between a: String, and b: String, before: Date?) async throws -> MessagePage {
        try await check()
        advance()
        let all = messages
            .filter { ($0.from == a && $0.to == b) || ($0.from == b && $0.to == a) }
            .filter { before == nil || $0.createdAt < before! }
            .sorted { $0.createdAt > $1.createdAt }
        return MessagePage(messages: Array(all.prefix(Self.pageSize)), more: all.count > Self.pageSize)
    }

    /// Something an agent says to me out of the blue (for tests of what is unread).
    public func deliver(from agent: String, body: String) {
        messages.append(Message(id: nextID(), from: agent, to: Self.me, type: "note", body: body, status: "done", createdAt: Date()))
    }

    /// A task's steps as given, instead of the scripted run (for tests of
    /// how a real session's steps read).
    private var givenTraces: [String: [TraceEvent]] = [:]

    public func setTrace(of messageID: String, _ events: [TraceEvent]) {
        givenTraces[messageID] = events
    }

    public func trace(of messageID: String, after seq: Int) async throws -> Trace {
        try await check()
        if let given = givenTraces[messageID] {
            let status = messages.first { $0.id == messageID }?.status ?? "acked"
            return Trace(message: messageID, status: status, events: given.filter { $0.seq > seq })
        }
        if slowSteps > .zero { try? await Task.sleep(for: slowSteps) }
        advance()
        guard let task = messages.first(where: { $0.id == messageID }) else {
            throw HubError(error: "not_found", message: "message \(messageID) not found")
        }
        // A task still being worked on shows its steps one by one.
        var shown = Self.script.count
        if task.status == "queued" || silent.contains(messageID) {
            shown = 0
        } else if task.isOpen, let sentAt = simulated[messageID] {
            shown = min(shown, Int(max(0, Date().timeIntervalSince(sentAt) - Self.ackAfter) / 0.8))
        } else if task.isOpen {
            shown = 5
        }
        // Each step has its time, as a real run's do: one sent from the app
        // steps along as it is watched; one that was already there spread
        // its steps from when it was asked to when it last moved.
        let began = simulated[messageID].map { $0.addingTimeInterval(Self.ackAfter) } ?? task.createdAt
        let pace = simulated[messageID] != nil
            ? 0.8
            : max(0, (task.activeAt ?? task.updatedAt).timeIntervalSince(task.createdAt)) / Double(max(1, shown))
        var steps = Array(Self.script.prefix(shown))
        // Its session ended its turn after the last step: it waits.
        if turnEnded.contains(messageID), task.isOpen { steps.append(TraceEvent(seq: shown + 1, kind: "turn_end", data: TraceStep())) }
        let events = steps.filter { $0.seq > seq }.map { event -> TraceEvent in
            var timed = event
            timed.at = began.addingTimeInterval(Double(min(event.seq, shown)) * pace)
            return timed
        }
        return Trace(message: messageID, status: task.status, events: events)
    }

    /// The fake run, for tests that read a whole one.
    public static var scriptForTests: [TraceEvent] { script }

    /// What a run looks like: it says something, runs and reads, keeps a
    /// plan, edits, and says what it found.
    static let script: [TraceEvent] = [
        TraceEvent(seq: 1, kind: "thinking", data: TraceStep(text: "先看现在的代码怎么组织的，再决定改哪里。")),
        TraceEvent(seq: 2, kind: "text", data: TraceStep(text: "我先看一下现在的代码。")),
        TraceEvent(seq: 3, kind: "tool", data: TraceStep(id: "c1", tool: "Bash", input: "git status --short")),
        TraceEvent(seq: 4, kind: "tool_result", data: TraceStep(id: "c1", output: "")),
        TraceEvent(seq: 5, kind: "tool", data: TraceStep(id: "c2", tool: "Read", input: "internal/api/types.go")),
        TraceEvent(seq: 6, kind: "tool_result", data: TraceStep(id: "c2", output: "…")),
        TraceEvent(seq: 7, kind: "tool", data: TraceStep(id: "c3", tool: "TodoWrite", input: "[x] 看现有的接口\n[~] 写设备登记\n[ ] 补测试\n[ ] 更新文档")),
        TraceEvent(seq: 8, kind: "tool", data: TraceStep(id: "c4", tool: "Read", input: "docs/api.md")),
        TraceEvent(seq: 9, kind: "tool_result", data: TraceStep(id: "c4", output: "…")),
        TraceEvent(seq: 10, kind: "text", data: TraceStep(text: "接口的形状清楚了，开始改。")),
        TraceEvent(seq: 11, kind: "tool", data: TraceStep(id: "c5", tool: "Edit", input: "internal/api/types.go")),
        TraceEvent(seq: 12, kind: "tool_result", data: TraceStep(id: "c5", output: "ok")),
        TraceEvent(seq: 13, kind: "tool", data: TraceStep(id: "c6", tool: "Bash", input: "go test ./internal/api/...")),
        TraceEvent(seq: 14, kind: "tool_result", data: TraceStep(id: "c6", output: "FAIL", failed: true)),
        TraceEvent(seq: 15, kind: "tool", data: TraceStep(id: "c7", tool: "Bash", input: "go test ./internal/api/...")),
        TraceEvent(seq: 16, kind: "tool_result", data: TraceStep(id: "c7", output: "ok")),
        TraceEvent(seq: 17, kind: "usage", data: {
            var step = TraceStep()
            step.durationMs = 12_400
            step.inputTokens = 1_200
            step.outputTokens = 300
            step.costUsd = 0.02
            return step
        }()),
    ]

    public func send(_ request: SendRequest) async throws -> Message {
        try await check()
        let now = Date()
        let message = Message(
            id: nextID(), from: Self.me, to: request.to, threadId: request.threadId, after: request.after, body: request.body,
            status: "queued", project: request.project, createdAt: now, attachments: try attached(request.attachments ?? []))
        messages.append(message)
        settleAsking(of: request.to)
        simulated[message.id] = now
        return message
    }

    public func steer(_ messageID: String, body: String, attachments: [String]) async throws {
        try await check()
        guard let task = messages.first(where: { $0.id == messageID }), task.isOpen else {
            throw HubError(error: "invalid_state", message: "the task is not open")
        }
        messages.append(Message(
            id: nextID(), from: Self.me, to: task.to, threadId: task.threadId, type: "steer", body: body,
            status: "done", project: task.project, createdAt: Date(), attachments: try attached(attachments)))
        // More to do: the answer comes a little later.
        if simulated[messageID] != nil { simulated[messageID] = Date().addingTimeInterval(-Self.ackAfter) }
    }

    // MARK: Workflows

    private var workflowList: [Workflow] = []
    private var runList: [WorkflowRun] = []
    private var tooOldForWorkflows = false

    public func setTooOldForWorkflows(_ value: Bool) {
        tooOldForWorkflows = value
    }

    private func checkWorkflows() async throws {
        try await check()
        if tooOldForWorkflows { throw HubError(error: "not_found", message: "no route for GET /v1/workflows", status: 404) }
    }

    private func workflowIndex(_ ref: String) throws -> Int {
        guard let i = workflowList.firstIndex(where: { $0.id == ref || $0.name == ref }) else {
            throw HubError(error: "not_found", message: "workflow \(ref) not found", status: 404)
        }
        return i
    }

    private func runIndex(_ id: String) throws -> Int {
        guard let i = runList.firstIndex(where: { $0.id == id }) else {
            throw HubError(error: "not_found", message: "run \(id) not found", status: 404)
        }
        return i
    }

    /// A workflow as the list has it (no definition), with its last run.
    private func listed(_ workflow: Workflow) -> Workflow {
        var one = workflow
        one.definition = nil
        one.lastRun = runList.first { $0.workflowId == workflow.id }.map { var run = $0; run.steps = nil; return run }
        one.active = runList.filter { $0.workflowId == workflow.id && $0.isGoing }.count
        return one
    }

    public func workflows() async throws -> [Workflow] {
        try await checkWorkflows()
        return workflowList.map(listed)
    }

    public func workflow(_ ref: String) async throws -> Workflow {
        try await checkWorkflows()
        var one = listed(workflowList[try workflowIndex(ref)])
        one.definition = workflowList[try workflowIndex(ref)].definition
        return one
    }

    public func setWorkflow(_ ref: String, enabled: Bool) async throws {
        try await checkWorkflows()
        workflowList[try workflowIndex(ref)].enabled = enabled
    }

    public func startWorkflow(_ ref: String, inputs: [String: JSONValue]) async throws -> WorkflowRun {
        try await checkWorkflows()
        let workflow = workflowList[try workflowIndex(ref)]
        guard workflow.enabled else { throw HubError(error: "invalid_state", message: "the workflow is turned off", status: 409) }
        for key in workflow.definition?.inputs?.required ?? [] where (inputs[key]?.text ?? "").isEmpty {
            throw HubError(error: "invalid_argument", message: "inputs: missing \(key)", status: 400)
        }
        seq += 1
        let first = workflow.definition?.steps?.first
        let run = WorkflowRun(
            id: "run_\(seq)", workflowId: workflow.id, name: workflow.name, project: workflow.project, status: "running", inputs: inputs,
            by: Self.me, current: first.map { [$0.id] }, steps: first.map {
                [RunStep(id: "rs_new\(seq)", step: $0.id, title: $0.title, kind: "agent", status: "running", target: $0.agent)]
            })
        runList.insert(run, at: 0)
        return run
    }

    public func workflowRuns(workflow: String?, limit: Int) async throws -> [WorkflowRun] {
        try await checkWorkflows()
        return Array(runList.filter { workflow == nil || $0.workflowId == workflow || $0.name == workflow }
            .map { var run = $0; run.steps = nil; return run }.prefix(limit))
    }

    public func workflowRun(_ id: String) async throws -> WorkflowRun {
        try await checkWorkflows()
        return runList[try runIndex(id)]
    }

    public func cancelRun(_ id: String) async throws -> WorkflowRun {
        try await checkWorkflows()
        let i = try runIndex(id)
        guard runList[i].isGoing else { throw HubError(error: "invalid_state", message: "the run has ended", status: 409) }
        runList[i].status = "cancelled"
        runList[i].endedAt = Date()
        for j in (runList[i].steps ?? []).indices where ["running", "waiting"].contains(runList[i].steps![j].status) {
            runList[i].steps![j].status = "cancelled"
        }
        return runList[i]
    }

    public func retryRun(_ id: String) async throws -> WorkflowRun {
        try await checkWorkflows()
        let i = try runIndex(id)
        guard ["failed", "cancelled"].contains(runList[i].status) else {
            throw HubError(error: "invalid_state", message: "only a failed or cancelled run starts again", status: 409)
        }
        runList[i].status = "running"
        runList[i].endedAt = nil
        return runList[i]
    }

    public func decide(run id: String, step: String, choice: String, note: String?) async throws -> WorkflowRun {
        try await checkWorkflows()
        let i = try runIndex(id)
        guard let j = runList[i].steps?.firstIndex(where: { $0.step == step && $0.status == "waiting" }) else {
            throw HubError(error: "invalid_state", message: "step \(step) is not waiting for a decision", status: 409)
        }
        let options = runList[i].steps![j].options ?? []
        guard options.contains(choice) else {
            throw HubError(error: "invalid_argument", message: "choice must be one of \(options)", status: 400)
        }
        runList[i].steps![j].status = "done"
        runList[i].steps![j].summary = note.map { choice + "：" + $0 } ?? choice
        runList[i].steps![j].endedAt = Date()
        runList[i].status = choice == options.first ? "succeeded" : "failed"
        if choice != options.first { runList[i].error = "decided: \(choice)" }
        runList[i].current = []
        runList[i].endedAt = Date()
        return runList[i]
    }

    public func workflowDecisions() async throws -> [WorkflowDecision] {
        try await checkWorkflows()
        return runList.flatMap { run in
            (run.steps ?? []).filter { $0.kind == "human" && $0.status == "waiting" }.map { WorkflowDecision(run: run, step: $0) }
        }
    }

    // MARK: A project's computers

    private var projectMachineList: [String: Set<String>] = [:]

    public func projectMachines(_ project: String) async throws -> [ProjectMachine] {
        try await check()
        guard projectList.contains(where: { $0.name == project }) else {
            throw HubError(error: "not_found", message: "project \(project) not found", status: 404)
        }
        let allowed = projectMachineList[project] ?? []
        return machines.map { ProjectMachine(id: $0.id, name: $0.name, os: $0.os, online: $0.online, canSpawn: $0.canSpawn, allowed: allowed.contains($0.id)) }
    }

    public func setProjectMachine(_ project: String, machine: String, allowed: Bool) async throws {
        try await check()
        guard machines.contains(where: { $0.id == machine }) else { throw HubError(error: "not_found", message: "machine \(machine) not found", status: 404) }
        if allowed { projectMachineList[project, default: []].insert(machine) } else { projectMachineList[project]?.remove(machine) }
    }

    // MARK: Superagents

    private var superagentName: String?
    private var approvalList: [Approval] = []
    private var tooOldForSuperagents = false
    /// The demo's superagent asks for something as soon as it is named
    /// (tests that count requests turn it off).
    public var demoApprovals = false

    public func setDemoApprovals(_ value: Bool) {
        demoApprovals = value
    }

    public func setTooOldForSuperagents(_ value: Bool) {
        tooOldForSuperagents = value
    }

    /// A request of the superagent held for me (for tests and the demo).
    public func addApproval(what: String, reason: String? = nil, at: Date = Date()) {
        seq += 1
        approvalList.append(Approval(
            id: "apr_\(seq)", agent: superagentName ?? "", what: what, reason: reason, method: "DELETE", path: "/v1/projects/x",
            createdAt: at, expiresAt: at.addingTimeInterval(86_400)))
    }

    public func superagent() async throws -> SuperagentState {
        try await check()
        if tooOldForSuperagents { throw HubError(error: "not_found", message: "no route for GET /v1/console/superagent", status: 404) }
        let agent = superagentName.flatMap { name in agents.first { $0.agent.name == name }?.agent }
        return SuperagentState(agent: agent, pending: approvalList.filter { $0.status == "pending" }.count)
    }

    public func setSuperagent(_ agent: String?) async throws {
        try await check()
        if let agent, !agents.contains(where: { $0.agent.name == agent }) {
            throw HubError(error: "not_found", message: "agent \(agent) not found", status: 404)
        }
        if agent != superagentName {
            for i in approvalList.indices where approvalList[i].status == "pending" { approvalList[i].status = "withdrew" }
        }
        superagentName = agent
        // (the demo's: once there is one, it has asked for something)
        if agent != nil, approvalList.isEmpty, demoApprovals {
            addApproval(what: "删除项目「旧官网」", reason: "它的内容已经合并进 myco-web，留着容易让人找错地方。")
        }
    }

    public func approvals() async throws -> [Approval] {
        try await check()
        return approvalList.filter { $0.status == "pending" }
    }

    public func decide(approval id: String, approve: Bool) async throws -> Approval {
        try await check()
        guard let i = approvalList.firstIndex(where: { $0.id == id && $0.status == "pending" }) else {
            throw HubError(error: "invalid_state", message: "approval \(id) is not pending", status: 409)
        }
        approvalList[i].status = approve ? "done" : "denied"
        if approve { approvalList[i].result = 204 }
        return approvalList[i]
    }

    /// The computers it started sessions on, and what (for tests).
    public private(set) var spawned: [(machine: String, request: SpawnRequest)] = []

    public func spawn(on machine: String, _ request: SpawnRequest) async throws -> SpawnResult {
        try await check()
        guard let host = machines.first(where: { $0.id == machine }) else {
            throw HubError(error: "not_found", message: "machine \(machine) not found", status: 404)
        }
        spawned.append((machine, request))
        if request.cli == "upgrade" {
            return SpawnResult(upgrade: .init(from: "v0.20.0", to: "v0.21.2", restarting: true))
        }
        seq += 1
        let name = "\(host.name.lowercased())-\(request.cli)-\(String(format: "%04x", seq))"
        let agent = Agent(
            id: "agt_new\(seq)", name: name, displayName: request.displayName, description: "", binding: Binding(harness: request.cli, workdir: request.dir),
            createdAt: Date(), owner: "alice", machineId: machine, runtime: Runtime(cli: request.cli))
        agents.append(AgentStatus(agent: agent, state: "idle"))
        return SpawnResult(agent: name)
    }

    public func dismiss(_ messageID: String) async throws {
        try await check()
        guard let i = messages.firstIndex(where: { $0.id == messageID }), messages[i].isOpen else {
            throw HubError(error: "invalid_state", message: "the message is not open")
        }
        messages[i].status = "done"
        messages[i].updatedAt = Date()
        simulated[messageID] = nil
    }

    public func stop(_ messageID: String) async throws {
        try await check()
        guard let i = messages.firstIndex(where: { $0.id == messageID }), messages[i].isOpen else {
            throw HubError(error: "invalid_state", message: "the task is not open")
        }
        messages[i].status = "failed"
        messages[i].error = "stopped by the sender"
        messages[i].updatedAt = Date()
        simulated[messageID] = nil
    }

    public func upload(_ data: Data, name: String, contentType: String) async throws -> HubFile {
        try await check()
        if tooOldForFiles { throw HubError(error: "not_found", message: "not found") }
        // A moment on the way up, as on a real connection.
        try? await Task.sleep(for: .milliseconds(400))
        seq += 1
        let file = HubFile(id: "f_fake\(seq)", name: name.isEmpty ? "file" : name, size: Int64(data.count), contentType: contentType)
        files[file.id] = (file, data)
        return file
    }

    public func file(_ id: String) async throws -> Data {
        try await check()
        guard let stored = files[id] else { throw HubError(error: "not_found", message: "file \(id) not found") }
        return stored.data
    }

    /// The files a message names, as it carries them.
    private func attached(_ ids: [String]) throws -> [Attachment] {
        try ids.map { id in
            guard let file = files[id]?.file else { throw HubError(error: "not_found", message: "file \(id) not found") }
            return Attachment(fileId: file.id, name: file.name, size: file.size, contentType: file.contentType)
        }
    }

    public func hidden() async throws -> [String] {
        try await check()
        if tooOldForHidden { throw HubError(error: "not_found", message: "not found") }
        return hiddenIDs
    }

    public func setHidden(hide: [String], show: [String]) async throws -> [String] {
        try await check()
        if tooOldForHidden { throw HubError(error: "not_found", message: "not found") }
        hiddenIDs.removeAll { show.contains($0) }
        for id in hide where !hiddenIDs.contains(id) { hiddenIDs.append(id) }
        return hiddenIDs
    }

    /// Acts like a hub from before run settings.
    private var tooOldForRunSettings = false

    public func setTooOldForRunSettings(_ value: Bool) {
        tooOldForRunSettings = value
    }

    public func setRunSettings(of agent: String, model: String, effort: String, access: String?) async throws -> Agent {
        try await check()
        if tooOldForRunSettings { throw HubError(error: "not_found", message: "not found") }
        guard let index = agents.firstIndex(where: { $0.agent.name == agent }) else {
            throw HubError(error: "not_found", message: "agent \(agent) not found")
        }
        // Kept as chosen; what it runs with changes at its next run.
        let keep = access ?? agents[index].agent.runSettings?.access ?? ""
        agents[index].agent.runSettings = model.isEmpty && effort.isEmpty && keep.isEmpty
            ? nil : RunSettings(model: model, effort: effort, access: keep.isEmpty ? nil : keep)
        return agents[index].agent
    }

    // MARK: Handovers

    private var handoverList: [Handover] = []
    /// A command was made: "someone runs it" a moment later.
    private var takeoverComing: (at: Date, id: String)?
    /// Acts like a hub from before handovers.
    private var tooOldForHandovers = false

    public func setTooOldForHandovers(_ value: Bool) {
        tooOldForHandovers = value
    }

    private func checkHandovers(_ agent: String) async throws -> Int {
        try await check()
        if tooOldForHandovers { throw HubError(error: "not_found", message: "no route for /v1/agents/\(agent)/handovers", status: 404) }
        guard let index = agents.firstIndex(where: { $0.agent.name == agent }) else {
            throw HubError(error: "not_found", message: "agent \(agent) not found", status: 404)
        }
        return index
    }

    private func side(of agent: Agent) -> HandoverSide {
        HandoverSide(
            binding: agent.binding, machineId: agent.machineId,
            machineName: machines.first { $0.id == agent.machineId }?.name, runtime: agent.runtime)
    }

    public func handovers(of agent: String) async throws -> [Handover] {
        _ = try await checkHandovers(agent)
        advance()
        return handoverList.filter { $0.agent == agent }.sorted { $0.at > $1.at }
    }

    /// The new session is there: the agent answers from it now.
    private func take(over index: Int, with handover: inout Handover, cli: String, dir: String?, machine: String?) {
        let open = messages.filter { $0.to == agents[index].agent.name && $0.isOpen }.map(\.id)
        agents[index].agent.binding = Binding(harness: cli, sessionId: "sess_\(handover.id)", workdir: dir ?? agents[index].agent.binding.workdir)
        agents[index].agent.machineId = machine
        agents[index].agent.runtime = Runtime(cli: cli, model: cli.hasPrefix("codex") ? "gpt-6-sol" : "claude-opus-5-5")
        handover.to = side(of: agents[index].agent)
        handover.state = "done"
        handover.doneAt = Date()
        handover.openTasks = open
    }

    public func handOver(_ agent: String, _ request: HandoverRequest) async throws -> HandoverResult {
        let index = try await checkHandovers(agent)
        guard let machine = machines.first(where: { $0.id == request.machine || $0.name == request.machine }) else {
            throw HubError(error: "not_found", message: "machine \(request.machine) not found", status: 404)
        }
        // (the new session takes a moment to start and read the note)
        try? await Task.sleep(for: .milliseconds(1_500))
        seq += 1
        var handover = Handover(
            id: "hov_fake\(seq)", agent: agent, at: Date(), by: "alice", note: request.note, from: side(of: agents[index].agent),
            state: "waiting", back: request.back)
        if let back = request.back, let earlier = handoverList.first(where: { $0.id == back }) {
            take(
                over: index, with: &handover, cli: earlier.from?.binding?.harness ?? "claude", dir: earlier.from?.binding?.workdir,
                machine: earlier.from?.machineId)
        } else {
            take(over: index, with: &handover, cli: request.cli ?? "claude", dir: request.dir, machine: machine.id)
        }
        handoverList.append(handover)
        return HandoverResult(handover: handover, spawn: .init(reply: "读完交接说明了：我接着把推送接口的设计写完，先看上一位留下的草稿。"))
    }

    public func handoverLink(of agent: String, note: String?, back: String?) async throws -> HandoverLink {
        let index = try await checkHandovers(agent)
        seq += 1
        let handover = Handover(
            id: "hov_fake\(seq)", agent: agent, at: Date(), by: "alice", note: note, from: side(of: agents[index].agent),
            state: "waiting", back: back)
        handoverList.append(handover)
        takeoverComing = (Date().addingTimeInterval(4), handover.id)
        let url = "https://demo.myco.example/h/myco_takeover_demo\(seq)"
        return HandoverLink(handover: handover, command: "curl -fsSL '\(url)' | sh", commandPs1: "irm '\(url)?format=ps1' | iex")
    }

    /// The command "was run" in a cloud session: it has the agent now.
    private func arriveTakeover() {
        guard let coming = takeoverComing, Date() >= coming.at else { return }
        takeoverComing = nil
        guard let place = handoverList.firstIndex(where: { $0.id == coming.id }),
              let index = agents.firstIndex(where: { $0.agent.name == handoverList[place].agent })
        else { return }
        var handover = handoverList[place]
        take(over: index, with: &handover, cli: "claude-cloud", dir: "/home/user/myco", machine: nil)
        handoverList[place] = handover
    }

    // MARK: Apps, and a page's own data

    /// A page's shared state, its records, and its change feed.
    struct PageData {
        var state: Any?
        var stateVersion = 0
        var records: [String: [String: (data: Any, version: Int)]] = [:]
        var order: [String: [String]] = [:]
        var changes: [[String: Any]] = []
        var seq = 0
    }

    private var appList: [AppTemplate] = []
    private var appFiles: [String: [String: (type: String, text: String)]] = [:]
    private var pageData: [String: PageData] = [:]
    private var tooOldForApps = false

    public func setTooOldForApps(_ value: Bool) {
        tooOldForApps = value
    }

    public func apps() async throws -> [AppTemplate] {
        try await check()
        if tooOldForApps { throw HubError(error: "not_found", message: "no route for GET /v1/templates", status: 404) }
        return appList
    }

    public func useApp(_ id: String, name: String, project: String?, sample: Bool) async throws -> Page {
        try await check()
        guard let app = appList.first(where: { $0.id == id || $0.slug == id }) else {
            throw HubError(error: "not_found", message: "template \(id) not found", status: 404)
        }
        seq += 1
        let pageID = "pg_app\(seq)"
        let page = Page(
            id: pageID, name: name.isEmpty ? app.name : name, project: project, publisher: Self.me, version: 1,
            url: Self.demoAddress.absoluteString + "/pages/" + pageID, versions: [PageVersion(n: 1, by: Self.me, at: Date())],
            createdAt: Date(), updatedAt: Date(), template: PageTemplate(id: app.id, slug: app.slug, name: app.name, version: app.version))
        pageList.insert(page, at: 0)
        pageFiles[pageID] = appFiles[app.id] ?? [:]
        var data = PageData()
        if sample {
            for (index, text) in ["买牛奶", "回邮件"].enumerated() {
                let record = "r\(index + 1)"
                data.records["items", default: [:]][record] = (["text": text, "done": index == 0], 1)
                data.order["items", default: []].append(record)
            }
        }
        pageData[pageID] = data
        return page
    }

    public func pages() async throws -> [Page] {
        try await check()
        return pageList
    }

    private static func json(_ value: Any) -> Data {
        (try? JSONSerialization.data(withJSONObject: value, options: [.fragmentsAllowed])) ?? Data("null".utf8)
    }

    private func record(_ page: String, _ collection: String, _ id: String) -> [String: Any]? {
        guard let one = pageData[page]?.records[collection]?[id] else { return nil }
        return ["collection": collection, "id": id, "data": one.data, "version": one.version, "updated_by": Self.me,
                "updated_at": ISO8601DateFormatter().string(from: Date())]
    }

    public func pageRequest(_ method: String, page: String, path: String, query: [String: String], body: Data?) async throws -> PageAnswer {
        try await check()
        guard pageList.contains(where: { $0.id == page }) else { throw HubError(error: "not_found", message: "page \(page) not found", status: 404) }
        var data = pageData[page] ?? PageData()
        defer { pageData[page] = data }
        let sent = body.flatMap { try? JSONSerialization.jsonObject(with: $0, options: [.fragmentsAllowed]) } as? [String: Any] ?? [:]
        let parts = path.split(separator: "/").map(String.init)
        func change(_ collection: String, _ id: String, _ op: String, _ value: Any?, _ version: Int) {
            data.seq += 1
            var one: [String: Any] = ["seq": data.seq, "collection": collection, "record": id, "op": op, "version": version, "by": Self.me]
            if let value { one["data"] = value }
            data.changes.append(one)
        }
        switch (method, parts.first ?? "", parts.count) {
        case ("GET", "state", _):
            return PageAnswer(data: Self.json(["state": data.state ?? NSNull(), "version": data.stateVersion]), status: 200)
        case ("PUT", "state", _):
            let base = sent["if_version"] as? Int ?? 0
            guard base == data.stateVersion else {
                return PageAnswer(data: Self.json(["error": "conflict", "message": "the state changed", "latest": ["state": data.state ?? NSNull(), "version": data.stateVersion]]), status: 409)
            }
            data.state = sent["state"] ?? NSNull()
            data.stateVersion += 1
            return PageAnswer(data: Self.json(["state": data.state ?? NSNull(), "version": data.stateVersion]), status: 200)
        case ("GET", "changes", _):
            let after = Int(query["after"] ?? "0") ?? 0
            let changes = data.changes.filter { ($0["seq"] as? Int ?? 0) > after }
            return PageAnswer(data: Self.json(["changes": changes, "seq": data.seq]), status: 200)
        case ("GET", "data", 1):
            let wanted = query["collection"].flatMap { $0.isEmpty ? nil : $0 }
            let order = data.order
            let collections = wanted.map { [$0] } ?? Array(order.keys).sorted()
            var records: [[String: Any]] = []
            for collection in collections {
                for id in order[collection] ?? [] {
                    if let one = record(page, collection, id) { records.append(one) }
                }
            }
            return PageAnswer(data: Self.json(["records": records, "seq": data.seq]), status: 200)
        case ("GET", "data", 3):
            guard let one = record(page, parts[1], parts[2]) else { throw HubError(error: "not_found", message: "no such record", status: 404) }
            return PageAnswer(data: Self.json(one), status: 200)
        case ("POST", "data", 2), ("PUT", "data", 3):
            let collection = parts[1]
            var id = parts.count == 3 ? parts[2] : ""
            if id.isEmpty {
                seq += 1
                id = "rec\(seq)"
            }
            let current = data.records[collection]?[id]
            if let base = sent["if_version"] as? Int, base != (current?.version ?? 0) {
                return PageAnswer(data: Self.json(["error": "conflict", "message": "the record changed", "latest": record(page, collection, id).map { $0 as Any } ?? NSNull()]), status: 409)
            }
            let version = (current?.version ?? 0) + 1
            data.records[collection, default: [:]][id] = (sent["data"] ?? [:], version)
            if current == nil { data.order[collection, default: []].append(id) }
            change(collection, id, "put", sent["data"] ?? [:], version)
            pageData[page] = data
            return PageAnswer(data: Self.json(record(page, collection, id) ?? [:]), status: current == nil ? 201 : 200)
        case ("DELETE", "data", 3):
            let (collection, id) = (parts[1], parts[2])
            data.records[collection]?[id] = nil
            data.order[collection]?.removeAll { $0 == id }
            change(collection, id, "delete", nil, 0)
            return PageAnswer(data: Data(), status: 204)
        default:
            throw HubError(error: "not_found", message: "no route for \(method) \(path)", status: 404)
        }
    }

    /// Someone else (an agent) writes a record of a page: for tests of
    /// what comes to a page that listens.
    public func agentWrites(page: String, collection: String, id: String, data value: [String: Any]) {
        var data = pageData[page] ?? PageData()
        let version = (data.records[collection]?[id]?.version ?? 0) + 1
        if data.records[collection]?[id] == nil { data.order[collection, default: []].append(id) }
        data.records[collection, default: [:]][id] = (value, version)
        data.seq += 1
        data.changes.append(["seq": data.seq, "collection": collection, "record": id, "op": "put", "data": value, "version": version, "by": "studio-claude-55d0"])
        pageData[page] = data
    }

    /// The messages sent to an agent (for tests of a page sending as me).
    public func messages(to agent: String) -> [Message] {
        messages.filter { $0.to == agent }
    }

    // MARK: Pins

    private var pinList: [String: [Pin]] = [:]
    private var tooOldForPins = false

    public func setTooOldForPins(_ value: Bool) {
        tooOldForPins = value
    }

    private func checkPins() async throws {
        try await check()
        if tooOldForPins { throw HubError(error: "not_found", message: "no route for GET /v1/pins", status: 404) }
    }

    public func pins(with agent: String) async throws -> [Pin] {
        try await checkPins()
        return pinList[agent] ?? []
    }

    public func addPin(_ request: PinRequest) async throws -> Pin {
        try await checkPins()
        var list = pinList[request.with] ?? []
        // The same thing again is not added twice.
        if let same = list.first(where: { $0.kind == request.kind && $0.target == request.target && $0.file?.fileId == request.file?.fileId }) {
            return same
        }
        guard list.count < 20 else { throw HubError(error: "conflict", message: "at most 20 pins", status: 409) }
        seq += 1
        let title = request.title?.isEmpty == false ? request.title! : (request.target ?? request.file?.name ?? "")
        let pin = Pin(id: "pin_fake\(seq)", kind: request.kind, title: String(title.prefix(80)), target: request.target, file: request.file, addedBy: Self.me, at: Date())
        list.append(pin)
        pinList[request.with] = list
        return pin
    }

    public func removePin(_ id: String, with agent: String) async throws {
        try await checkPins()
        pinList[agent]?.removeAll { $0.id == id }
    }

    // MARK: Pages

    public static let demoAddress = URL(string: "https://demo.myco.example")!

    public nonisolated var address: URL? { Self.demoAddress }

    private var pageList: [Page] = []
    private var pageFiles: [String: [String: (type: String, text: String)]] = [:]

    public func page(_ id: String) async throws -> Page {
        try await check()
        guard let page = pageList.first(where: { $0.id == id }) else {
            throw HubError(error: "not_found", message: "page \(id) not found", status: 404)
        }
        return page
    }

    public func pageFile(_ id: String, version: Int?, path: String) async throws -> PageFile {
        try await check()
        guard let files = pageFiles[id], let file = files[path.isEmpty ? "index.html" : path] else {
            throw HubError(error: "not_found", message: "no such file in page \(id)", status: 404)
        }
        // (each version says which it is, to tell them apart)
        let text = file.text.replacingOccurrences(of: "{{version}}", with: String(version ?? 0))
        return PageFile(data: Data(text.utf8), contentType: file.type)
    }

    /// The agent publishes the page again (for tests of "a newer version").
    public func publishAgain(_ id: String) {
        guard let index = pageList.firstIndex(where: { $0.id == id }) else { return }
        pageList[index].version += 1
        pageList[index].versions.insert(PageVersion(n: pageList[index].version, by: pageList[index].publisher, at: Date()), at: 0)
        pageList[index].updatedAt = Date()
    }

    // MARK: A computer added with one command

    /// A link was made: the folder it is run in shows up a moment later
    /// (as if someone ran the command), usable by these agents.
    private var computerComing: (at: Date, allowFrom: [String])?
    /// Acts as for someone who is not the workspace's owner or an admin.
    private var mayNotAddComputers = false

    public func setMayNotAddComputers(_ value: Bool) {
        mayNotAddComputers = value
    }

    public func computerLink(allowFrom: [String]) async throws -> JoinLink {
        try await check()
        if mayNotAddComputers {
            throw HubError(error: "permission_denied", message: "only the hub's owner or an admin can make a link for a shell node", status: 403)
        }
        guard !allowFrom.isEmpty else {
            throw HubError(error: "invalid_argument", message: "a shell node needs at least one agent allowed to send it commands", status: 400)
        }
        computerComing = (Date().addingTimeInterval(4), allowFrom)
        return JoinLink(joinUrl: "https://demo.myco.example/j/myco_enroll_computer")
    }

    public func addProjectMember(_ agent: String, to project: String) async throws {
        try await check()
        guard let index = projectList.firstIndex(where: { $0.name == project }) else {
            throw HubError(error: "not_found", message: "project \(project) not found", status: 404)
        }
        guard agents.contains(where: { $0.agent.name == agent }) else {
            throw HubError(error: "not_found", message: "agent \(agent) not found", status: 404)
        }
        if !projectList[index].members.contains(where: { $0.name == agent }) {
            projectList[index].members.append(ProjectMember(name: agent, addedAt: Date()))
        }
    }

    /// The command "was run": the folder is a node now.
    private func arriveComputer() {
        guard let coming = computerComing, Date() >= coming.at else { return }
        computerComing = nil
        seq += 1
        let node = Agent(
            id: "agt_shell\(seq)", name: "studio-site-shell\(seq)", binding: Binding(harness: "exec", workdir: "~/code/site"),
            kind: "shell", machineId: "mach_studio")
        agents.append(AgentStatus(agent: node, state: "idle"))
    }

    // MARK: Automations

    private var automationList: [Automation] = []
    private var automationRuns: [String: [AutomationRun]] = [:]
    /// Acts like a hub from before automations.
    private var tooOldForAutomations = false

    public func setTooOldForAutomations(_ value: Bool) {
        tooOldForAutomations = value
    }

    private func checkAutomations() async throws {
        try await check()
        if tooOldForAutomations { throw HubError(error: "not_found", message: "no route for GET /v1/console/automations", status: 404) }
    }

    private func place(of id: String) throws -> Int {
        guard let index = automationList.firstIndex(where: { $0.id == id }) else {
            throw HubError(error: "not_found", message: "automation \(id) not found", status: 404)
        }
        return index
    }

    public func automations() async throws -> [Automation] {
        try await checkAutomations()
        return automationList
    }

    public func automation(_ id: String) async throws -> AutomationDetail {
        try await checkAutomations()
        return AutomationDetail(automation: automationList[try place(of: id)], runs: automationRuns[id] ?? [])
    }

    public func createAutomation(_ request: AutomationRequest) async throws -> AutomationSaved {
        try await checkAutomations()
        let name = (request.name ?? "").trimmingCharacters(in: .whitespaces), to = request.to ?? "", body = request.body ?? ""
        guard !name.isEmpty, !to.isEmpty, !body.isEmpty else {
            throw HubError(error: "invalid_argument", message: "name, to and body are required", status: 400)
        }
        guard request.schedule != nil || request.webhook == true else {
            throw HubError(error: "invalid_argument", message: "an automation needs a schedule or a webhook", status: 400)
        }
        seq += 1
        let id = "aut_fake\(seq)"
        let hook = request.webhook == true
        let made = Automation(
            id: id, name: name, owner: "alice", to: to, body: body, schedule: request.schedule, webhook: hook,
            webhookUrl: hook ? "https://demo.myco.example/hooks/\(id)" : nil, skipIfBusy: request.skipIfBusy ?? true,
            enabled: request.enabled ?? true, nextRunAt: request.schedule == nil ? nil : Date().addingTimeInterval(3_600))
        automationList.insert(made, at: 0)
        return AutomationSaved(automation: made, webhookSecret: hook ? "whk_demo_\(seq)" : nil)
    }

    public func updateAutomation(_ id: String, _ request: AutomationRequest) async throws -> AutomationSaved {
        try await checkAutomations()
        let index = try place(of: id)
        var one = automationList[index]
        var secret: String?
        if let name = request.name { one.name = name }
        if let to = request.to { one.to = to }
        if let body = request.body { one.body = body }
        if let schedule = request.schedule { one.schedule = schedule }
        if request.clearSchedule == true { one.schedule = nil }
        if let hook = request.webhook, hook != one.webhook {
            one.webhook = hook
            one.webhookUrl = hook ? "https://demo.myco.example/hooks/\(id)" : nil
            if hook {
                seq += 1
                secret = "whk_demo_\(seq)"
            }
        }
        if request.rotateSecret == true, one.webhook {
            seq += 1
            secret = "whk_demo_\(seq)"
        }
        if let skip = request.skipIfBusy { one.skipIfBusy = skip }
        if let enabled = request.enabled { one.enabled = enabled }
        guard one.schedule != nil || one.webhook else {
            throw HubError(error: "invalid_argument", message: "an automation needs a schedule or a webhook", status: 400)
        }
        one.nextRunAt = one.schedule != nil && one.enabled ? Date().addingTimeInterval(3_600) : nil
        automationList[index] = one
        return AutomationSaved(automation: one, webhookSecret: secret)
    }

    public func deleteAutomation(_ id: String) async throws {
        try await checkAutomations()
        automationList.remove(at: try place(of: id))
        automationRuns[id] = nil
    }

    public func runAutomation(_ id: String) async throws -> AutomationRun {
        try await checkAutomations()
        let index = try place(of: id)
        let one = automationList[index]
        // An ordinary task from me, marked as the automation's.
        var sent = Message(id: nextID(), from: Self.me, to: one.to, body: one.body, status: "queued", createdAt: Date())
        sent.automation = id
        messages.append(sent)
        simulated[sent.id] = Date()
        seq += 1
        let run = AutomationRun(id: "arun_fake\(seq)", automationId: id, at: Date(), trigger: "manual", outcome: "sent", messageId: sent.id)
        automationRuns[id, default: []].insert(run, at: 0)
        automationList[index].lastRun = run
        return run
    }

    /// How many times a join link is refused as a gateway in front of the
    /// hub would (502), before one is made.
    private var linkFailures = 0

    public func setJoinLinkFailures(_ count: Int) {
        linkFailures = count
    }

    public func joinLink() async throws -> JoinLink {
        try await check()
        if linkFailures > 0 {
            linkFailures -= 1
            throw HubError(error: "http_502", message: "502 Bad Gateway nginx", status: 502)
        }
        return JoinLink(joinUrl: "https://demo.myco.example/j/myco_enroll_demo")
    }

    // MARK: Simulation

    static let ackAfter: TimeInterval = 1.5
    static let replyAfter: TimeInterval = 9

    /// Moves what the app sent along: taken on, then answered.
    private func advance() {
        let now = Date()
        arriveComputer()
        arriveTakeover()
        for (id, sentAt) in simulated {
            guard let i = messages.firstIndex(where: { $0.id == id }) else { continue }
            // Put after a task that is still open: it waits, as on a hub.
            if let after = messages[i].after, messages.first(where: { $0.id == after })?.isOpen == true {
                simulated[id] = now
                continue
            }
            let age = now.timeIntervalSince(sentAt)
            if age >= Self.replyAfter {
                messages[i].status = "done"
                messages[i].updatedAt = now
                messages.append(Message(
                    id: nextID(), from: messages[i].to, to: Self.me, threadId: messages[i].threadId,
                    replyTo: id, type: "reply", body: "收到，已经处理好了：\n\n- 看了相关的代码\n- 改动很小，测试都过了",
                    status: "done", project: messages[i].project, createdAt: now))
                simulated[id] = nil
            } else if age >= Self.ackAfter, messages[i].status == "queued" {
                messages[i].status = "acked"
                messages[i].updatedAt = now
                messages[i].activeAt = now
            }
        }
    }

    /// An agent's status with what the simulation added on top.
    private func withLoad(_ status: AgentStatus) -> AgentStatus {
        var s = status
        // What it waits for in the background, and its goal.
        let name = status.agent.name
        let watching = watchList.filter { $0.agent == name && $0.isOn }.count
        s.watching = watchList.isEmpty ? nil : watching
        s.goal = goalList.first { $0.agent == name && $0.isOn }.map {
            GoalBrief(id: $0.id, objective: $0.objective, round: $0.round, maxRounds: $0.maxRounds, nextWakeAt: $0.nextWakeAt, state: $0.state)
        }
        guard status.state != "offline" else { return s }
        let open = messages.filter { $0.to == status.agent.name && $0.isOpen }
        s.inProgress = open.filter { $0.status == "acked" }.count
        s.queued = open.count - s.inProgress
        s.state = s.inProgress > 0 ? "working" : (s.queued > 0 ? "pending" : "idle")
        s.current = open
        return s
    }

    private func nextID() -> String {
        seq += 1
        return "msg_fake\(seq)"
    }
}
