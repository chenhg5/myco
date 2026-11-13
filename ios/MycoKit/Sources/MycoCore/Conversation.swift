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
        case progress(text: String)
        /// The updates of a task that has ended, folded into one line that
        /// opens ("3 条进展"): the answer is what matters now.
        case pastProgress(texts: [String])
        /// A task that did not get done, and why.
        case failure(reason: String)
        /// A task I stopped.
        case stopped
        /// How a task of mine was worked on: what the agent said and did.
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
    ///   - status: looks up a node's status, to tell working from stuck.
    public static func items(
        me: String, agent: String, messages: [Message], others: [Message], processes: Set<String> = [],
        status: (String) -> AgentStatus?, now: Date
    ) -> [ConversationItem] {
        var items: [ConversationItem] = []
        let byID = Dictionary(messages.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        /// Updates of tasks that have ended, by task, oldest first.
        var past: [String: [Message]] = [:]
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
                    items.append(.init(id: m.id, at: m.createdAt, kind: .reply(text: text(of: m), taskID: nil, to: m.to == me ? nil : m.to)))
                    continue
                }
                items.append(.init(id: m.id, at: m.createdAt, kind: .mine(text: text(of: m), steer: false, by: by)))
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
                    // My open task's status closes the page; a colleague's
                    // stays with its task, so two never stack at the end.
                    items.append(.init(
                        id: m.id + ".status", at: mine ? .distantFuture : m.createdAt.addingTimeInterval(0.002),
                        kind: .status(state, taskID: m.id)))
                case .failed(let reason):
                    items.append(.init(id: m.id + ".failed", at: m.updatedAt, kind: .failure(reason: reason)))
                case .stopped:
                    items.append(.init(id: m.id + ".stopped", at: m.updatedAt, kind: .stopped))
                case .done:
                    break
                }
            case "steer":
                if asked { items.append(.init(id: m.id, at: m.createdAt, kind: .mine(text: text(of: m), steer: true, by: by))) }
            case "reply":
                if !asked {
                    items.append(.init(
                        id: m.id, at: m.createdAt,
                        kind: .reply(text: text(of: m), taskID: m.replyTo, to: m.to == me ? nil : m.to)))
                }
            case "progress":
                guard !asked else { continue }
                // Its task: the one it answers, else the one its thread began with.
                let taskID = m.replyTo ?? m.threadId
                // A task that is not on this page counts as open: when in
                // doubt, an update is shown, not tucked away.
                if byID[taskID].map(\.isOpen) ?? true {
                    items.append(.init(id: m.id, at: m.createdAt, kind: .progress(text: text(of: m))))
                } else {
                    past[taskID, default: []].append(m)
                }
            default:
                break // events repeat what a failed task already says
            }
        }
        for (taskID, updates) in past {
            let ordered = updates.sorted { $0.createdAt < $1.createdAt }
            guard let last = ordered.last else { continue }
            // Where the last update was: right above the answer.
            items.append(.init(id: taskID + ".progress", at: last.createdAt, kind: .pastProgress(texts: ordered.map { text(of: $0) })))
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

    /// The newest task of mine that is still open: what the input speaks to.
    public static func openTask(me: String, messages: [Message]) -> Message? {
        messages.filter { $0.from == me && $0.isOpen }.max { $0.createdAt < $1.createdAt }
    }

    /// What a message says, as shown: its body, or a note that it is
    /// encrypted and this device cannot open it (opening comes later).
    public static func text(of message: Message) -> String {
        if !(message.unreadable ?? "").isEmpty || Envelope.isSealed(message.body) {
            return L("🔒 已加密，这台设备还读不了", "🔒 Encrypted: this device cannot read it yet")
        }
        return message.body
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
