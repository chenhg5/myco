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

/// Why a task failed, in words. A reason the hub is known to give is
/// explained; anything else is shown as it came. The desktop app's
/// `failureWords`.
public enum FailureWords {
    public enum Kind: Sendable, Hashable {
        /// The hub gave up: taken several times, never started on.
        case notDelivered
        case timedOut
        case failed
    }

    public static func kind(of reason: String) -> Kind {
        let reason = reason.lowercased()
        if reason.contains("delivery attempts exhausted") { return .notDelivered }
        if reason.contains("lease expired") || reason.contains("timed out") || reason.contains("timeout") { return .timedOut }
        return .failed
    }

    /// A word or two ("Not delivered").
    public static func short(_ reason: String) -> String {
        switch kind(of: reason) {
        case .notDelivered: return L("没送到", "Not delivered")
        case .timedOut: return L("超时了", "Timed out")
        case .failed: return L("失败了", "Failed")
        }
    }

    /// One line for a notification: what happened, without the advice.
    public static func line(_ reason: String) -> String {
        if kind(of: reason) == .notDelivered {
            return L("没送到：它取走了几次，都没开始处理。", "Not delivered: it picked this up several times but never started on it.")
        }
        return long(reason)
    }

    /// What happened and what can be done about it.
    public static func long(_ reason: String) -> String {
        switch kind(of: reason) {
        case .notDelivered:
            return L("没送到：它取走了几次，都没开始处理。它那边的会话可能没在运行，或者卡住了。可以重新发送，或者先看看它在不在线。",
                     "Not delivered: it picked this up several times but never started on it. Its session may not be running, or may be stuck. Send it again, or check that it is online first.")
        case .timedOut:
            return L("超时了：", "Timed out: ") + reason
        case .failed:
            return reason.isEmpty ? L("失败了", "Failed") : L("失败了：", "Failed: ") + reason
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

    /// - Parameter hasText: something is there to send: words, or files.
    public static func resolve(hasOpenTask: Bool, hasText: Bool) -> ComposerAction {
        guard hasOpenTask else { return .send }
        return hasText ? .steer : .stop
    }
}
