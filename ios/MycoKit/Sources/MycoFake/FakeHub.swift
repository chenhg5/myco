import Foundation
import MycoAPI

/// A workspace in memory, behind the same protocol as a real hub: for
/// previews, UI tests and trying the app without a hub. It covers the
/// states pages must show: waiting, at work, done, failed, possibly stuck,
/// away. What is sent to it gets taken on after a moment and answered a
/// few seconds later, so adding to a running task and stopping it can be
/// tried by hand.
public actor FakeHub: HubAPI {
    public static let me = "@console:glenn"

    private var agents: [AgentStatus]
    private var machines: [Machine]
    private var projectList: [Project]
    private var messages: [Message]
    private var seq = 0
    /// Tasks sent from the app, by id: when each was sent.
    private var simulated: [String: Date] = [:]
    /// Makes every call fail, to see the app without its workspace.
    private var unreachable = false

    public init(scenario: Scenario = .team, now: Date = Date()) {
        let world = scenario.build(now: now)
        agents = world.agents
        machines = world.machines
        projectList = world.projects
        messages = world.messages
    }

    public func setUnreachable(_ value: Bool) {
        unreachable = value
    }

    private func check() throws {
        if unreachable { throw HubError.unreachable("The workspace did not answer.") }
    }

    public func whoami() throws -> Whoami {
        try check()
        return Whoami(member: Member(id: "mem_demo", name: "glenn", displayName: "Glenn"))
    }

    public func overview() throws -> Overview {
        try check()
        advance()
        return Overview(generatedAt: Date(), machines: machines, agents: agents.map(withLoad))
    }

    public func projects() throws -> [Project] {
        try check()
        return projectList
    }

    public func recent(_ scope: RecentScope) throws -> [RecentItem] {
        try check()
        advance()
        var project: String?
        if case .project(let name) = scope { project = name }
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
            return RecentItem(
                threadId: id, participants: parties, updatedAt: sorted.map(\.updatedAt).max() ?? first.createdAt,
                last: last, task: task, answer: answer, open: open, state: state, project: first.project)
        }
        .sorted { $0.updatedAt > $1.updatedAt }
    }

    /// How many messages one page of a conversation holds here.
    public static let pageSize = 60

    public func exchanges(between a: String, and b: String, before: Date?) throws -> MessagePage {
        try check()
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

    public func trace(of messageID: String, after seq: Int) throws -> Trace {
        try check()
        advance()
        guard let task = messages.first(where: { $0.id == messageID }) else {
            throw HubError(error: "not_found", message: "message \(messageID) not found")
        }
        // A task still being worked on shows its steps one by one.
        var shown = Self.script.count
        if task.status == "queued" {
            shown = 0
        } else if task.isOpen, let sentAt = simulated[messageID] {
            shown = min(shown, Int(max(0, Date().timeIntervalSince(sentAt) - Self.ackAfter) / 0.8))
        } else if task.isOpen {
            shown = 5
        }
        return Trace(message: messageID, status: task.status, events: Self.script.prefix(shown).filter { $0.seq > seq })
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

    public func send(_ request: SendRequest) throws -> Message {
        try check()
        let now = Date()
        let message = Message(
            id: nextID(), from: Self.me, to: request.to, after: request.after, body: request.body,
            status: "queued", project: request.project, createdAt: now)
        messages.append(message)
        simulated[message.id] = now
        return message
    }

    public func steer(_ messageID: String, body: String) throws {
        try check()
        guard let task = messages.first(where: { $0.id == messageID }), task.isOpen else {
            throw HubError(error: "invalid_state", message: "the task is not open")
        }
        messages.append(Message(
            id: nextID(), from: Self.me, to: task.to, threadId: task.threadId, type: "steer", body: body,
            status: "done", project: task.project, createdAt: Date()))
        // More to do: the answer comes a little later.
        if simulated[messageID] != nil { simulated[messageID] = Date().addingTimeInterval(-Self.ackAfter) }
    }

    public func stop(_ messageID: String) throws {
        try check()
        guard let i = messages.firstIndex(where: { $0.id == messageID }), messages[i].isOpen else {
            throw HubError(error: "invalid_state", message: "the task is not open")
        }
        messages[i].status = "failed"
        messages[i].error = "stopped by the sender"
        messages[i].updatedAt = Date()
        simulated[messageID] = nil
    }

    public func joinLink() throws -> JoinLink {
        try check()
        return JoinLink(joinUrl: "https://demo.myco.example/j/myco_enroll_demo")
    }

    // MARK: Simulation

    static let ackAfter: TimeInterval = 1.5
    static let replyAfter: TimeInterval = 9

    /// Moves what the app sent along: taken on, then answered.
    private func advance() {
        let now = Date()
        for (id, sentAt) in simulated {
            guard let i = messages.firstIndex(where: { $0.id == id }) else { continue }
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
        guard status.state != "offline" else { return status }
        var s = status
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
