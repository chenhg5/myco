import Foundation
import MycoAPI

/// One thing shown in a conversation, in the order people read it.
public struct ConversationItem: Identifiable, Hashable, Sendable {
    public enum Kind: Hashable, Sendable {
        /// What I said. `steer`: it was added to a task already running.
        case mine(text: String, steer: Bool)
        /// The agent's answer (Markdown).
        case reply(text: String)
        /// An interim update from the agent.
        case progress(text: String)
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
    /// Given to the agent whose page this is (by someone else), not given
    /// out by it.
    public var incoming = false
}

/// Builds what a conversation page shows. Pure: same input, same list.
public enum ConversationBuilder {
    /// - Parameters:
    ///   - messages: every message between me and the agent, any order.
    ///   - others: tasks between other parties to show as capsules (a
    ///     coordinator's dispatches, or work others gave this agent).
    ///   - incomingTo: the agent whose page this is, when `others` holds
    ///     work given to it rather than given out by it.
    ///   - processes: tasks whose steps are known and worth a row.
    ///   - status: looks up a node's status, to tell working from stuck.
    public static func items(
        me: String, messages: [Message], others: [Message], incomingTo: String? = nil, processes: Set<String> = [],
        status: (String) -> AgentStatus?, now: Date
    ) -> [ConversationItem] {
        var items: [ConversationItem] = []
        for m in messages {
            let mine = m.from == me
            switch m.type {
            case "task", "note":
                guard mine else {
                    items.append(.init(id: m.id, at: m.createdAt, kind: .reply(text: m.body)))
                    continue
                }
                items.append(.init(id: m.id, at: m.createdAt, kind: .mine(text: m.body, steer: false)))
                guard m.type == "task" else { continue }
                let state = RunState.of(m, recipient: status(m.to), now: now)
                if processes.contains(m.id) {
                    // Right under what I asked, before anything said later.
                    items.append(.init(
                        id: m.id + ".process", at: m.createdAt.addingTimeInterval(0.001),
                        kind: .process(taskID: m.id, live: state.isOpen)))
                }
                switch state {
                case .queued, .working, .maybeStuck:
                    // Open tasks sort after everything said so far.
                    items.append(.init(id: m.id + ".status", at: .distantFuture, kind: .status(state, taskID: m.id)))
                case .failed(let reason):
                    items.append(.init(id: m.id + ".failed", at: m.updatedAt, kind: .failure(reason: reason)))
                case .stopped:
                    items.append(.init(id: m.id + ".stopped", at: m.updatedAt, kind: .stopped))
                case .done:
                    break
                }
            case "steer":
                if mine { items.append(.init(id: m.id, at: m.createdAt, kind: .mine(text: m.body, steer: true))) }
            case "reply":
                if !mine { items.append(.init(id: m.id, at: m.createdAt, kind: .reply(text: m.body))) }
            case "progress":
                if !mine { items.append(.init(id: m.id, at: m.createdAt, kind: .progress(text: m.body))) }
            default:
                break // events repeat what a failed task already says
            }
        }
        for task in others where task.type == "task" {
            let dispatch = Dispatch(
                taskID: task.id, from: task.from, to: task.to, firstLine: firstLine(of: task.body),
                state: RunState.of(task, recipient: status(task.to), now: now), incoming: task.to == incomingTo)
            items.append(.init(id: task.id + ".dispatch", at: task.createdAt, kind: .dispatch(dispatch)))
        }
        return items.sorted { ($0.at, $0.id) < ($1.at, $1.id) }
    }

    /// The newest task of mine that is still open: what the input speaks to.
    public static func openTask(me: String, messages: [Message]) -> Message? {
        messages.filter { $0.from == me && $0.isOpen }.max { $0.createdAt < $1.createdAt }
    }

    public static func firstLine(of body: String) -> String {
        let line = body.split(whereSeparator: \.isNewline).first.map(String.init) ?? ""
        return line.trimmingCharacters(in: .whitespaces)
    }
}
