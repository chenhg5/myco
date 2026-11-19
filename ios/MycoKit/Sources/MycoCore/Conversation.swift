import Foundation
import MycoAPI
import MycoCrypto

/// One thing shown in a conversation, in the order people read it.
public struct ConversationItem: Identifiable, Hashable, Sendable {
    public enum Kind: Hashable, Sendable {
        /// Something asked of the agent whose page this is. `steer`: it was
        /// added to a task already running. `by`: who asked, when it was
        /// not me (a coordinator, another member).
        case mine(text: String, steer: Bool, by: String?)
        /// The agent's answer (Markdown). `taskID`: the task it answers.
        /// `to`: whom it answers, when it is not me.
        case reply(text: String, taskID: String?, to: String?)
        /// Half an hour or more passed since the message above.
        case time(Date)
        /// An interim update from the agent, on a task still being worked on.
        /// `latest`: the newest one of its task (its dot is green).
        case progress(text: String, latest: Bool)
        /// The updates of a task that has ended, folded into one line that
        /// opens ("3 条进展"): the answer is what matters now.
        case pastProgress(texts: [String])
        /// A task that did not get done, and why.
        case failure(reason: String)
        /// A task I stopped.
        case stopped
        /// The plan the agent is working through for a task: the latest one,
        /// from its steps or from a checklist it reported.
        case plan(items: [PlanItem], live: Bool)
        /// How a task was worked on: what the agent said and did.
        /// `live`: it is still being worked on.
        case process(taskID: String, live: Bool)
        /// An open task that is being worked on, or that may be stuck:
        /// where it stands now. One whose run has stopped, or that has not
        /// begun, gets no line: a task reads as a message, not as a state.
        case status(RunState, taskID: String)
        /// What the agent and another agent said to each other, as one
        /// card: who gave what to whom, how it stands, what came back.
        case handoff(Handoff)
        /// Another session took the agent over here: a thin line, not a
        /// message ("换成 Studio Mac · Codex 接着做（你换的）").
        case takenOver(text: String)
    }

    public var id: String
    public var at: Date
    public var kind: Kind
    /// The files that came with it (what was asked, or an answer).
    public var files: [Attachment] = []
    /// On a task of mine that has not started: a few words under it.
    public var receipt: TaskReceipt?
    /// An automation sent it for me (marked 自动).
    public var automated = false
    /// What it answers or adds to, when that is not right above it: the
    /// task's first words, quoted on top.
    public var quote: String?
    /// The loading at the foot: what the agent says it is doing now, when
    /// it has said so since it last answered.
    public var saying: String?
}

/// What is said under a task of mine that the agent has not started on,
/// like a receipt under a message: gone once it starts, and nothing is
/// said while it works or after it stops (the desktop app's rule: a task
/// reads as a message, not as a state).
public enum TaskReceipt: Hashable, Sendable {
    /// Not looked at yet.
    case notOpened
    /// Another task of mine is at work before it.
    case behind
    /// The agent is offline: it reads it when it is back.
    case offline
    /// The agent's session is open somewhere else.
    case sessionBusy
    /// The agent set it aside: it answers later.
    case later
    /// In the conversation of an agent that receives in its own session:
    /// it is there, not looked at yet.
    case delivered
}

/// What two agents said to each other, in a conversation that is mine with
/// one of them: never as messages (they would read as if it were talking
/// to me), one card per exchange instead. A line says who gave what to
/// whom and how it is going; once something came back, a second line holds
/// the first line of it: the handoff, not the back and forth. A tap opens
/// their whole exchange. The desktop app's `Handoff.tsx`.
public struct Handoff: Hashable, Sendable, Codable, Identifiable {
    public enum State: String, Hashable, Sendable, Codable {
        case queued, working, done, failed, note
    }

    /// The first message of the exchange: its key, and where it opens.
    public var id: String
    /// When it started: where the card goes in the conversation.
    public var at: Date
    /// When it last moved.
    public var updated: Date
    /// The agent whose page this is sent it; else it was sent to it.
    public var outgoing: Bool
    /// The other agent.
    public var other: String
    /// A task (work to do), or only a note.
    public var isTask: Bool
    /// The first line of what was sent.
    public var text: String
    public var state: State
    /// The first line of what came back.
    public var result: String?

    public init(
        id: String, at: Date, updated: Date, outgoing: Bool, other: String, isTask: Bool, text: String, state: State,
        result: String? = nil
    ) {
        self.id = id
        self.at = at
        self.updated = updated
        self.outgoing = outgoing
        self.other = other
        self.isTask = isTask
        self.text = text
        self.state = state
        self.result = result
    }
}

/// Builds what a conversation page shows. Pure: same input, same list.
public enum ConversationBuilder {
    /// - Parameters:
    ///   - agent: whose page this is.
    ///   - messages: every message to show, any order: mine with the agent.
    ///   - handoffs: what the agent and other agents said to each other,
    ///     one card per exchange (`handoffs(between:and:)`, `handoffs(ofHome:)`).
    ///   - processes: tasks whose steps are known and worth a row. What
    ///     such a task reported, and what was added to it, are drawn inside
    ///     its run where each came, not as rows of their own.
    ///   - tracedPlans: the plan each task's steps hold, and when it was written.
    ///   - paused: open tasks whose run has stopped for now (`RunProcess.isPaused`):
    ///     they get no line saying where they stand.
    ///   - activity: when each open task's steps last moved. With what the
    ///     task itself says and what it reported, the latest is its last
    ///     sign of life: how long it has been quiet counts from there.
    ///   - ended: open tasks whose session ended its turn. Such a task
    ///     waits (for me, or for others); it is not stuck, unless its node
    ///     is offline.
    ///   - status: looks up a node's status, to tell working from stuck.
    public static func items(
        me: String, agent: String, messages all: [Message], handoffs: [Handoff] = [], processes: Set<String> = [],
        tracedPlans: [String: (items: [PlanItem], at: Date?)] = [:], paused: Set<String> = [],
        activity: [String: Date] = [:], ended: Set<String> = [], windows: RunWindows = RunWindows(),
        takeovers: [(id: String, at: Date, text: String)] = [],
        status: (String) -> AgentStatus?, now: Date
    ) -> [ConversationItem] {
        var items: [ConversationItem] = []
        // What waits behind an open task is not part of the conversation
        // yet: it shows above the input until its turn comes. What was
        // taken back before its turn came was never sent: not here at all.
        let waiting = Set(queuedBehind(all).map(\.id)).union(neverSent(all, me: me))
        let messages = all.filter { !waiting.contains($0.id) }
        let byID = Dictionary(messages.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let answered = Set(messages.filter { $0.type == "reply" }.compactMap(\.replyTo))
        let reported = Set(messages.filter { $0.type == "progress" }.map { $0.replyTo ?? $0.threadId })
        /// When each task last reported.
        var saidAt: [String: Date] = [:]
        for report in messages where report.type == "progress" {
            guard let task = report.replyTo else { continue }
            saidAt[task] = max(saidAt[task] ?? .distantPast, report.createdAt)
        }
        /// Where a task stands. Possibly stuck is judged from its last
        /// sign of life (the task, what it reported, its steps: the
        /// latest), and a task whose turn is over waits, it is not stuck.
        /// The desktop app's rule.
        func state(of task: Message) -> RunState {
            var task = task
            let last = max(task.activeAt ?? task.updatedAt, saidAt[task.id] ?? .distantPast, activity[task.id] ?? .distantPast)
            task.activeAt = last
            let recipient = status(task.to)
            let state = RunState.of(task, recipient: recipient, now: now)
            if case .maybeStuck = state, ended.contains(task.id) {
                let offline = recipient.map { $0.state == "offline" || !$0.agent.online } ?? false
                if !offline { return .working(since: last) }
            }
            return state
        }
        /// The task of mine the agent is at work on, if any.
        let atWork = messages
            .filter { $0.from == me && $0.type == "task" && $0.status == "acked" && !answered.contains($0.id) }
            .max { $0.createdAt < $1.createdAt }?.id
        /// What is said under a task of mine that has not started.
        func receipt(of task: Message) -> TaskReceipt? {
            guard task.from == me, task.type == "task", task.isOpen, task.e2e != true, !answered.contains(task.id),
                  (task.asking ?? "").isEmpty
            else { return nil }
            if case .maybeStuck = state(of: task) { return nil }
            if task.status == "held" { return .later }
            if task.status == "acked" || reported.contains(task.id) { return nil }
            if let recipient = status(task.to) {
                if recipient.state == "offline" { return .offline }
                if recipient.sessionInUse == true, recipient.receiving != "session" { return .sessionBusy }
            }
            if atWork != nil && atWork != task.id { return .behind }
            if task.status == "delivered", status(task.to)?.receiving == "session" { return .delivered }
            return .notOpened
        }
        /// Updates of tasks that have ended, by task, oldest first.
        var past: [String: [(message: Message, said: String)]] = [:]
        /// The task an item belongs to, when that is not plain from the
        /// item (an update, an addition): what a reply below it answers
        /// is right above it, or quoted.
        var owner: [String: String] = [:]
        /// My tasks that are still open, with where each stands.
        var myOpen: [(task: Message, state: RunState)] = []
        /// Colleagues' tasks that are still open, with where each stands.
        var theirOpen: [(task: Message, state: RunState)] = []
        /// The newest update shown for each open task, by task.
        var newestUpdate: [String: (id: String, at: Date)] = [:]
        /// The newest plan of each task: from its steps, or a checklist it reported.
        var plans: [String: (items: [PlanItem], at: Date)] = [:]
        for (taskID, plan) in tracedPlans {
            plans[taskID] = (plan.items, plan.at ?? byID[taskID]?.createdAt ?? .distantPast)
        }
        for m in messages {
            let mine = m.from == me
            // Whoever gives the agent work is on the asking side: me, or a
            // colleague (named). What the agent says is on the other side.
            let asked = m.from != agent
            let by: String? = mine ? nil : m.from
            switch m.type {
            case "task", "note":
                guard asked else {
                    // The agent speaking first, or giving work to someone.
                    items.append(.init(
                        id: m.id, at: m.createdAt, kind: .reply(text: text(of: m), taskID: nil, to: m.to == me ? nil : m.to),
                        files: files(of: m)))
                    continue
                }
                items.append(.init(
                    id: m.id, at: m.createdAt, kind: .mine(text: text(of: m), steer: false, by: by), files: files(of: m),
                    receipt: receipt(of: m), automated: !(m.automation ?? "").isEmpty))
                guard m.type == "task" else { continue }
                let state = state(of: m)
                if processes.contains(m.id) {
                    // Right under what was asked, before anything said later.
                    items.append(.init(
                        id: m.id + ".process", at: m.createdAt.addingTimeInterval(0.001),
                        kind: .process(taskID: m.id, live: state.isOpen)))
                }
                switch state {
                case .queued, .working, .maybeStuck:
                    // My open task's status closes the page (one line,
                    // added below, however many are open); a colleague's
                    // stays with its task.
                    // Its answer is in the conversation: for whoever
                    // reads it, it is answered, whatever its status says.
                    if answered.contains(m.id) { break }
                    if mine {
                        myOpen.append((m, state))
                    } else {
                        theirOpen.append((m, state))
                    }
                case .failed(let reason):
                    items.append(.init(id: m.id + ".failed", at: m.updatedAt, kind: .failure(reason: reason)))
                case .stopped:
                    items.append(.init(id: m.id + ".stopped", at: m.updatedAt, kind: .stopped))
                case .done:
                    break
                }
            case "steer":
                // Added to a task whose run is shown, with nothing said in
                // between: it is in the run, where it was sent. With other
                // messages since, it is drawn where it was sent, quoting
                // the task it adds to.
                let close = Self.steerIsInItsRun(m, among: messages)
                if let task = m.replyTo, processes.contains(task), close { continue }
                if asked {
                    var item = ConversationItem(
                        id: m.id, at: m.createdAt, kind: .mine(text: text(of: m), steer: true, by: by), files: files(of: m))
                    if !close, let task = m.replyTo.flatMap({ byID[$0] }) { item.quote = RunWindows.title(of: task) }
                    if let task = m.replyTo { owner[m.id] = task }
                    items.append(item)
                }
            case "reply":
                if !asked {
                    items.append(.init(
                        id: m.id, at: m.createdAt,
                        kind: .reply(text: text(of: m), taskID: m.replyTo, to: m.to == me ? nil : m.to),
                        files: files(of: m)))
                }
            case "progress":
                guard !asked else { continue }
                // Its task: the one it answers, else the one its thread began with.
                let taskID = m.replyTo ?? m.threadId
                // A report with a checklist carries the task's plan, ticked
                // off at the top of its work. The checklist is not said
                // again; what was said around it ("好，计划：") still is.
                var said = text(of: m)
                if let split = RunProcess.splitPlan(m.body) {
                    if plans[taskID].map({ m.createdAt >= $0.at }) ?? true { plans[taskID] = (split.items, m.createdAt) }
                    if split.rest.isEmpty { continue }
                    said = split.rest
                }
                // Its task's run is shown: the update is in the run, after
                // the step it came after. (In a long session that is the
                // run under my newer message, when it came after that.)
                if processes.contains(windows.home(of: taskID, at: m.createdAt)) { continue }
                // A task that is not on this page counts as open: when in
                // doubt, an update is shown, not tucked away.
                if byID[taskID].map(\.isOpen) ?? true {
                    owner[m.id] = windows.home(of: taskID, at: m.createdAt)
                    items.append(.init(id: m.id, at: m.createdAt, kind: .progress(text: said, latest: false)))
                    if newestUpdate[taskID].map({ m.createdAt >= $0.at }) ?? true { newestUpdate[taskID] = (m.id, m.createdAt) }
                } else {
                    past[taskID, default: []].append((m, said))
                }
            default:
                break // events repeat what a failed task already says
            }
        }
        // The newest update of each task still being worked on is marked.
        let newest = Set(newestUpdate.values.map(\.id))
        items = items.map { item in
            guard newest.contains(item.id), case .progress(let text, _) = item.kind else { return item }
            return ConversationItem(id: item.id, at: item.at, kind: .progress(text: text, latest: true))
        }
        /// The line for an open task, when there is something to say: it
        /// is at work, or it may be stuck. One whose run has stopped (its
        /// turn ended, it handed work out, it was set aside) gets none:
        /// what it said last stands, as in any chat. One that has not
        /// begun has a receipt under the message instead.
        func standing(_ open: (task: Message, state: RunState)) -> ConversationItem.Kind? {
            // One that may be stuck says so: that needs a look.
            if case .maybeStuck = open.state, open.task.status != "held" { return .status(open.state, taskID: open.task.id) }
            if paused.contains(open.task.id) { return nil }
            // Set aside, then at it again (new steps since): at work.
            if open.task.status == "held" {
                return .status(.working(since: open.task.activeAt ?? open.task.updatedAt), taskID: open.task.id)
            }
            if case .queued = open.state { return nil }
            return .status(open.state, taskID: open.task.id)
        }
        // Where a colleague's open task stands comes under its work: after
        // its newest update, never above it.
        for open in theirOpen {
            guard let line = standing(open) else { continue }
            let under = max(open.task.createdAt.addingTimeInterval(0.002), (newestUpdate[open.task.id]?.at ?? .distantPast).addingTimeInterval(0.001))
            items.append(.init(id: open.task.id + ".status", at: under, kind: line))
        }
        // One line says where my work with the agent stands, never two:
        // the task being worked on if there is one, else one that may be
        // stuck.
        func rank(_ kind: ConversationItem.Kind) -> Int {
            if case .status(.working, _) = kind { return 0 }
            return 1
        }
        let lines = myOpen.compactMap { open in standing(open).map { (task: open.task, kind: $0) } }
        let leading = lines.min { a, b in
            (rank(a.kind), b.task.createdAt) < (rank(b.kind), a.task.createdAt)
        }
        if let leading {
            var line = ConversationItem(id: leading.task.id + ".status", at: .distantFuture, kind: leading.kind)
            if case .status(.working, let taskID) = leading.kind {
                // What it is doing now, whatever task it is on: its newest
                // update, unless it has answered since; its clock from the
                // latest message here (a coordinator in one long session
                // keeps an early task open while it answers newer ones).
                let fromAgent = messages.filter { $0.from == agent }
                let update = fromAgent.filter { $0.type == "progress" }.max { $0.createdAt < $1.createdAt }
                let answer = fromAgent.filter { $0.type == "reply" }.map(\.createdAt).max() ?? .distantPast
                if let update, update.createdAt > answer {
                    let said = Self.firstLine(of: RunProcess.splitPlan(update.body).map { $0.rest } ?? update.body)
                    if !said.isEmpty { line.saying = said }
                }
                let latest = messages.filter { ["task", "note", "steer", "reply"].contains($0.type) }.map(\.createdAt).max()
                if let latest, case .status(.working(let since), _) = leading.kind, latest > since {
                    line.kind = .status(.working(since: latest), taskID: taskID)
                }
            }
            items.append(line)
        }
        for (taskID, plan) in plans {
            guard let task = byID[taskID] else { continue }
            // Right under what was asked, above how it was worked on.
            items.append(.init(
                id: taskID + ".plan", at: task.createdAt.addingTimeInterval(0.0005),
                kind: .plan(items: plan.items, live: task.isOpen)))
        }
        for (taskID, updates) in past {
            let ordered = updates.sorted { $0.message.createdAt < $1.message.createdAt }
            guard let last = ordered.last else { continue }
            // Where the last update was: right above the answer.
            items.append(.init(id: taskID + ".progress", at: last.message.createdAt, kind: .pastProgress(texts: ordered.map(\.said))))
        }
        for handoff in handoffs {
            items.append(.init(id: handoff.id + ".handoff", at: handoff.at, kind: .handoff(handoff)))
        }
        // Where another session took over: only within what is on the
        // page (a line above everything would hang there alone).
        if let first = items.map(\.at).min() {
            for takeover in takeovers where takeover.at >= first {
                items.append(.init(id: "handover." + takeover.id, at: takeover.at, kind: .takenOver(text: takeover.text)))
            }
        }
        var sorted = items.sorted { ($0.at, $0.id) < ($1.at, $1.id) }
        // An answer that does not come right under what it answers (other
        // messages came between) quotes it on top.
        func belongs(_ item: ConversationItem) -> String? {
            if let known = owner[item.id] { return known }
            switch item.kind {
            case .mine: return item.id
            case .process(let taskID, _): return taskID
            case .reply(_, let taskID, _): return taskID
            case .plan: return String(item.id.dropLast(".plan".count))
            case .failure, .stopped, .pastProgress: return item.id.split(separator: ".").first.map(String.init)
            default: return nil
            }
        }
        for index in sorted.indices {
            guard case .reply(_, let taskID?, _) = sorted[index].kind, let task = byID[taskID] else { continue }
            let above = sorted[..<index].last { item in
                if case .time = item.kind { return false }
                if case .status = item.kind { return false }
                return true
            }
            if let above, belongs(above) == taskID { continue }
            sorted[index].quote = RunWindows.title(of: task)
        }
        return withTimeBreaks(sorted)
    }

    /// An addition goes into its task's run when nothing else was said
    /// between them (another task, an answer, the task's own answer).
    public static func steerIsInItsRun(_ steer: Message, among messages: [Message]) -> Bool {
        guard let taskID = steer.replyTo, let task = messages.first(where: { $0.id == taskID }) else { return false }
        return !messages.contains { other in
            other.id != taskID && ["task", "note", "reply"].contains(other.type)
                && other.createdAt > task.createdAt && other.createdAt < steer.createdAt
        }
    }

    private static func handoffState(of task: Message) -> Handoff.State {
        switch task.status {
        case "done": return .done
        case "failed": return .failed
        case "acked", "held": return .working
        default: return .queued
        }
    }

    /// The cards for what `agent` and the others said to each other (the
    /// messages between them, as loaded): one per task, the notes around
    /// it folded into it, a note on its own its own card. Updates and
    /// additions do not show: the handoff, not the work. The desktop app's
    /// `handoffsFrom`.
    public static func handoffs(between messages: [Message], and agent: String) -> [Handoff] {
        var threads: [String: [Message]] = [:]
        for message in messages where message.type != "progress" && message.type != "steer" {
            threads[message.threadId, default: []].append(message)
        }
        var out: [Handoff] = []
        for list in threads.values {
            // (who began the card, and its task)
            var cards: [(card: Handoff, from: String, task: String?)] = []
            for message in list.sorted(by: { ($0.createdAt, $0.id) < ($1.createdAt, $1.id) }) {
                let isTask = message.type == "task"
                if isTask || cards.isEmpty {
                    cards.append((
                        Handoff(
                            id: message.id, at: message.createdAt, updated: max(message.updatedAt, message.createdAt),
                            outgoing: message.from == agent, other: message.from == agent ? message.to : message.from,
                            isTask: isTask, text: firstLine(of: message.body), state: isTask ? handoffState(of: message) : .note),
                        message.from, isTask ? message.id : nil))
                    continue
                }
                // What came back: the answer to the task, or a note from the other side.
                var current = cards[cards.count - 1]
                if message.type == "event", message.replyTo == nil || message.replyTo == current.task {
                    current.card.state = .failed
                } else if message.from != current.from, message.type == "reply" || message.type == "note" {
                    let line = firstLine(of: message.body)
                    if !line.isEmpty { current.card.result = line }
                }
                current.card.updated = max(current.card.updated, message.updatedAt, message.createdAt)
                cards[cards.count - 1] = current
            }
            out += cards.map(\.card)
        }
        return out.sorted { ($0.at, $0.id) < ($1.at, $1.id) }
    }

    /// The cards on a project's page, from its threads: the work the
    /// coordinator handed out, and what the members sent it. Only between
    /// agents: what a person writes is a message. The desktop app's
    /// `handoffsOfHome`.
    public static func handoffs(ofHome items: [RecentItem], coordinator: String, isPerson: (String) -> Bool) -> [Handoff] {
        var out: [Handoff] = []
        for item in items where item.kind == "thread" {
            guard let first = item.task ?? item.first, !isPerson(first.from), !isPerson(first.to),
                  first.from == coordinator || first.to == coordinator
            else { continue }
            let outgoing = first.from == coordinator
            let isTask = first.type == "task"
            var back: Message?
            if let answer = item.answer, answer.from != first.from {
                back = answer
            } else if item.last.id != first.id, item.last.from != first.from, item.last.type == "reply" || item.last.type == "note" {
                back = item.last
            }
            let state: Handoff.State
            if !isTask {
                state = .note
            } else {
                state = Handoff.State(rawValue: item.state ?? "").flatMap { $0 == .note ? nil : $0 } ?? handoffState(of: first)
            }
            out.append(Handoff(
                id: first.id, at: first.createdAt, updated: item.updatedAt, outgoing: outgoing,
                other: outgoing ? first.to : first.from, isTask: isTask, text: firstLine(of: first.body), state: state,
                result: back.map { firstLine(of: $0.body) }.flatMap { $0.isEmpty ? nil : $0 }))
        }
        return out.sorted { ($0.at, $0.id) < ($1.at, $1.id) }
    }

    /// A long quiet stretch gets the time written before what follows it.
    public static let timeBreakAfter: TimeInterval = 30 * 60

    static func withTimeBreaks(_ sorted: [ConversationItem]) -> [ConversationItem] {
        var result: [ConversationItem] = []
        var last: Date?
        for item in sorted {
            // An open task's status sorts last on purpose: it has no time of its own.
            if item.at != .distantFuture {
                if let last, item.at.timeIntervalSince(last) >= timeBreakAfter {
                    result.append(.init(id: item.id + ".time", at: item.at, kind: .time(item.at)))
                } else if last == nil {
                    result.append(.init(id: item.id + ".time", at: item.at, kind: .time(item.at)))
                }
                last = item.at
            }
            result.append(item)
        }
        return result
    }

    /// Messages put after a task that is still open: the hub holds them
    /// until that task ends (hubs from v0.19.2). Oldest first.
    public static func queuedBehind(_ messages: [Message]) -> [Message] {
        let byID = Dictionary(messages.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        return messages.filter { m in
            guard let after = m.after, !after.isEmpty, ["queued", "delivered", "held", "acked"].contains(m.status) else { return false }
            return byID[after]?.isOpen ?? false
        }
        .sorted { $0.createdAt < $1.createdAt }
    }

    /// The newest task of mine that is still open: what the input speaks
    /// to. Not one that waits behind another: that one has not begun. And
    /// not one the agent set aside (held), nor one it has stopped working
    /// on (its turn is over: `paused`): what I type then is a new message,
    /// as in a chat, and there is nothing to stop. Unless it asks me
    /// something, when what I type is the answer. The desktop app's rule.
    /// `moving`: tasks set aside that the agent is at again (new steps
    /// since): those take what I type like any at work. One rule for the
    /// input and for the line that says it is working.
    public static func openTask(me: String, messages: [Message], paused: Set<String> = [], moving: Set<String> = []) -> Message? {
        let waiting = Set(queuedBehind(messages).map(\.id))
        // A task whose answer is in the conversation is not open to the
        // person, even while its status lags (the desktop app's hasAnswer).
        let answered = Set(messages.filter { $0.type == "reply" || $0.type == "event" }.compactMap(\.replyTo))
        func atWork(_ task: Message) -> Bool {
            task.status == "held" ? moving.contains(task.id) : !paused.contains(task.id)
        }
        return messages
            .filter { $0.from == me && $0.isOpen && !waiting.contains($0.id) && !answered.contains($0.id) }
            .filter { atWork($0) || !($0.asking ?? "").isEmpty }
            .max { $0.createdAt < $1.createdAt }
    }

    /// What the agent offers me to say next: the suggestion on my newest
    /// task, once the agent is not at work on anything of mine (`open`).
    /// One that is sealed, or on a task this device cannot read, is left
    /// out: it is only an offer. The desktop app's rule (there the empty
    /// input shows it and Tab takes it).
    public static func suggestion(me: String, messages: [Message], open: Message?) -> String? {
        guard open == nil,
              let last = messages.filter({ $0.type == "task" && $0.from == me }).max(by: { $0.createdAt < $1.createdAt })
        else { return nil }
        let text = (last.suggest ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, (last.unreadable ?? "").isEmpty, !Envelope.isSealed(text) else { return nil }
        return text
    }

    /// Messages put behind another and stopped before their turn came
    /// (edited or withdrawn): never sent, so not part of the conversation
    /// (not "stopped" there). The desktop app's rule.
    public static func neverSent(_ messages: [Message], me: String) -> Set<String> {
        let answered = Set(messages.compactMap(\.replyTo))
        return Set(messages.filter { m in
            m.type == "task" && !(m.after ?? "").isEmpty && m.status == "failed" && m.error == RunState.stoppedBySender
                && m.from == me && !answered.contains(m.id)
        }.map(\.id))
    }

    /// The thread a new message of mine continues: the one my last exchange
    /// with the agent was in.
    public static func threadToContinue(me: String, messages: [Message]) -> String? {
        messages.filter { $0.from == me || $0.to == me }.max { $0.createdAt < $1.createdAt }?.threadId
    }

    /// What a message says, as shown: its body, or a note that it is
    /// encrypted and this device cannot open it (opening comes later).
    public static func text(of message: Message) -> String {
        if !(message.unreadable ?? "").isEmpty || Envelope.isSealed(message.body) {
            return L("🔒 已加密，这台设备还读不了", "🔒 Encrypted: this device cannot read it yet")
        }
        return message.body
    }

    /// What a task's run takes in besides its steps: the updates the agent
    /// reported for it (without the checklist that is its plan), and what
    /// was added to it, each with when. Oldest first.
    public static func runExtras(
        of taskID: String, agent: String, messages: [Message], windows: RunWindows = RunWindows()
    ) -> (reports: [RunReport], added: [RunReport]) {
        var reports: [RunReport] = []
        var added: [RunReport] = []
        for m in messages.sorted(by: { $0.createdAt < $1.createdAt }) {
            switch m.type {
            case "progress" where m.from == agent && windows.home(of: m.replyTo ?? m.threadId, at: m.createdAt) == taskID:
                var said = text(of: m)
                if let split = RunProcess.splitPlan(m.body) {
                    if split.rest.isEmpty { continue }
                    said = split.rest
                }
                reports.append(RunReport(id: m.id, text: said, at: m.createdAt, files: files(of: m)))
            case "steer" where m.from != agent && m.replyTo == taskID && steerIsInItsRun(m, among: messages):
                added.append(RunReport(id: m.id, text: text(of: m), at: m.createdAt, files: files(of: m)))
            default:
                break
            }
        }
        return (reports, added)
    }

    /// The files a message shows. A sealed message's files are sealed
    /// too: they are opened when it can be.
    public static func files(of message: Message) -> [Attachment] {
        if !(message.unreadable ?? "").isEmpty || message.e2e == true || Envelope.isSealed(message.body) { return [] }
        return message.attachments
    }

    /// What a message of files alone says, in each language the app
    /// speaks: only what they are, in the desktop app's words.
    static func filesOnlyBodies(_ names: [String]) -> [AppLanguage: String] {
        guard let first = names.first else { return [:] }
        if names.count == 1 { return [.zh: "发来一个文件：\(first)", .en: "Sent a file: \(first)"] }
        return [
            .zh: "发来 \(names.count) 个文件：\(names.joined(separator: "、"))",
            .en: "Sent \(names.count) files: \(names.joined(separator: ", "))",
        ]
    }

    /// The words of this message only name its files (it was sent with
    /// none of the sender's own): the files say it better, so a page
    /// shows them alone.
    public static func saysOnlyItsFiles(_ text: String, files: [Attachment]) -> Bool {
        filesOnlyBodies(files.map(\.name)).values.contains(text.trimmingCharacters(in: .whitespacesAndNewlines))
    }

    /// A message's first line as plain text, for lists and capsules:
    /// Markdown marks are dropped, as the desktop app's summaries do.
    public static func firstLine(of body: String) -> String {
        if Envelope.isSealed(body) { return L("🔒 已加密", "🔒 Encrypted") }
        let line = body.split(whereSeparator: \.isNewline).first.map(String.init) ?? ""
        return line
            .replacingOccurrences(of: "[#>*`_~]", with: "", options: .regularExpression)
            .replacingOccurrences(of: "\\s+", with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespaces)
    }
}
