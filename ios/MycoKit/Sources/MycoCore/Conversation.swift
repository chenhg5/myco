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
        /// An open task of mine: where it stands now.
        case status(RunState, taskID: String)
        /// Work someone else gave, as one line: who, to whom, what, state.
        case dispatch(Dispatch)
    }

    public var id: String
    public var at: Date
    public var kind: Kind
    /// The files that came with it (what was asked, or an answer).
    public var files: [Attachment] = []
}

/// A task between two others, shown as a one-line capsule.
public struct Dispatch: Hashable, Sendable {
    public var taskID: String
    public var from: String
    public var to: String
    public var firstLine: String
    public var state: RunState
}

/// Builds what a conversation page shows. Pure: same input, same list.
public enum ConversationBuilder {
    /// - Parameters:
    ///   - agent: whose page this is.
    ///   - messages: every message to show, any order: mine with the agent
    ///     and, on a member's page, the agent's with its colleagues.
    ///   - others: tasks the agent gave out, to show as one-line capsules
    ///     (a coordinator's hand-outs on its project's page).
    ///   - processes: tasks whose steps are known and worth a row.
    ///   - tracedPlans: the plan each task's steps hold, and when it was written.
    ///   - status: looks up a node's status, to tell working from stuck.
    public static func items(
        me: String, agent: String, messages all: [Message], others: [Message], processes: Set<String> = [],
        tracedPlans: [String: (items: [PlanItem], at: Date?)] = [:],
        status: (String) -> AgentStatus?, now: Date
    ) -> [ConversationItem] {
        var items: [ConversationItem] = []
        // What waits behind an open task is not part of the conversation
        // yet: it shows above the input until its turn comes.
        let waiting = Set(queuedBehind(all).map(\.id))
        let messages = all.filter { !waiting.contains($0.id) }
        let byID = Dictionary(messages.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        /// Updates of tasks that have ended, by task, oldest first.
        var past: [String: [(message: Message, said: String)]] = [:]
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
                items.append(.init(id: m.id, at: m.createdAt, kind: .mine(text: text(of: m), steer: false, by: by), files: files(of: m)))
                guard m.type == "task" else { continue }
                let state = RunState.of(m, recipient: status(m.to), now: now)
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
                if asked {
                    items.append(.init(id: m.id, at: m.createdAt, kind: .mine(text: text(of: m), steer: true, by: by), files: files(of: m)))
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
                // A task that is not on this page counts as open: when in
                // doubt, an update is shown, not tucked away.
                if byID[taskID].map(\.isOpen) ?? true {
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
        // Where a colleague's open task stands comes under its work: after
        // its newest update, never above it.
        for open in theirOpen {
            let under = max(open.task.createdAt.addingTimeInterval(0.002), (newestUpdate[open.task.id]?.at ?? .distantPast).addingTimeInterval(0.001))
            items.append(.init(id: open.task.id + ".status", at: under, kind: .status(open.state, taskID: open.task.id)))
        }
        // One line says where my work with the agent stands, never two:
        // the task being worked on if there is one, else one that may be
        // stuck, else the newest still waiting.
        func rank(_ state: RunState) -> Int {
            switch state {
            case .working: return 0
            case .maybeStuck: return 1
            default: return 2
            }
        }
        let leading = myOpen.min { a, b in
            (rank(a.state), b.task.createdAt) < (rank(b.state), a.task.createdAt)
        }
        if let leading {
            items.append(.init(id: leading.task.id + ".status", at: .distantFuture, kind: .status(leading.state, taskID: leading.task.id)))
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
        for task in others where task.type == "task" {
            let dispatch = Dispatch(
                taskID: task.id, from: task.from, to: task.to, firstLine: firstLine(of: task.body),
                state: RunState.of(task, recipient: status(task.to), now: now))
            items.append(.init(id: task.id + ".dispatch", at: task.createdAt, kind: .dispatch(dispatch)))
        }
        return withTimeBreaks(items.sorted { ($0.at, $0.id) < ($1.at, $1.id) })
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
    /// to. Not one that waits behind another: that one has not begun.
    public static func openTask(me: String, messages: [Message]) -> Message? {
        let waiting = Set(queuedBehind(messages).map(\.id))
        return messages.filter { $0.from == me && $0.isOpen && !waiting.contains($0.id) }.max { $0.createdAt < $1.createdAt }
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
