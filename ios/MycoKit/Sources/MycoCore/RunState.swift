import Foundation
import MycoAPI

/// Where a task stands, as people see it. The one place this is worked out
/// (docs/design/app-ui.md §6.11): only a task the node has taken on counts
/// as being worked on; one pulled but not acked, or held, is still waiting.
public enum RunState: Sendable, Hashable {
    /// Waiting its turn.
    case queued
    /// Taken on; `since` is when it last showed activity.
    case working(since: Date)
    /// Taken on, then quiet for too long: the result may be lost.
    case maybeStuck(quietSince: Date)
    case done
    case stopped
    case failed(reason: String)

    /// Quiet this long after a runner woke the node: possibly stuck.
    public static let stuckAfter: TimeInterval = 10 * 60
    /// The same for a node that only receives while its own session waits.
    public static let stuckAfterInSession: TimeInterval = 30 * 60
    /// The error the hub records when the sender stops a task.
    public static let stoppedBySender = "stopped by the sender"

    public static func of(_ task: Message, recipient: AgentStatus?, now: Date) -> RunState {
        switch task.status {
        case "done":
            return .done
        case "failed":
            let reason = task.error ?? ""
            return reason == stoppedBySender ? .stopped : .failed(reason: reason)
        case "acked":
            let last = task.activeAt ?? task.updatedAt
            let offline = recipient.map { $0.state == "offline" || !$0.agent.online } ?? false
            let limit = recipient?.receiving == "session" ? stuckAfterInSession : stuckAfter
            if offline || now.timeIntervalSince(last) > limit {
                return .maybeStuck(quietSince: last)
            }
            return .working(since: task.updatedAt)
        default:
            return .queued
        }
    }

    /// Still open: the input speaks to this task.
    public var isOpen: Bool {
        switch self {
        case .queued, .working, .maybeStuck: return true
        case .done, .stopped, .failed: return false
        }
    }
}

/// What the send button does right now. Worked out here, read by the page.
public enum ComposerAction: Sendable, Hashable {
    /// A new message.
    case send
    /// Adds to the task being worked on.
    case steer
    /// Stops the task being worked on.
    case stop

    public static func resolve(hasOpenTask: Bool, hasText: Bool) -> ComposerAction {
        guard hasOpenTask else { return .send }
        return hasText ? .steer : .stop
    }
}
