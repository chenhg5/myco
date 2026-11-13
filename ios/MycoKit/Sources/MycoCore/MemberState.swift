import Foundation
import MycoAPI

/// What a member is up to, as people see it. Like `RunState` for a task,
/// this is the one place it is worked out.
public enum MemberState: Sendable, Hashable {
    /// At work; `text` is the status line it reported, if any.
    case working(text: String?)
    /// Everything it took on has been quiet too long.
    case possiblyStuck
    /// Has work waiting that it has not started.
    case waiting
    case free
    case away

    public static func of(_ status: AgentStatus, now: Date = Date()) -> MemberState {
        switch status.state {
        case "offline":
            return .away
        case "working":
            let takenOn = status.current.filter { $0.status == "acked" }
            let stuck = takenOn.filter {
                if case .maybeStuck = RunState.of($0, recipient: status, now: now) { return true }
                return false
            }
            if !takenOn.isEmpty, stuck.count == takenOn.count { return .possiblyStuck }
            let text = status.agent.statusText ?? ""
            return .working(text: text.isEmpty ? nil : text)
        case "pending":
            return .waiting
        default:
            return .free
        }
    }
}
