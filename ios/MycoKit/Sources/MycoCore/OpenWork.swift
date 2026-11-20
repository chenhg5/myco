import Foundation
import MycoAPI

/// An agent's open work, as the desktop app shows it in the profile (当前
/// 工作) and in its chat: each task says what it waits on, and the tasks
/// from elsewhere it still holds are said in its chat.
public enum OpenWork {
    /// What a task of its open work waits on, in words.
    public static func line(of task: Message, in status: AgentStatus, name: (String) -> String) -> String {
        if task.status == "held", let asking = task.asking, !asking.isEmpty {
            return L("在等 \(name(task.from)) 回答", "Waiting on \(name(task.from))'s answer")
        }
        if (status.staleTasks ?? []).contains(task.id) { return L("没收尾", "Left open") }
        switch task.status {
        case "queued", "delivered": return L("待处理", "Queued")
        case "held": return L("已挂起", "Held")
        default: return L("处理中", "In progress")
        }
    }

    /// The tasks it holds from elsewhere: given by others, taken up, not
    /// in this conversation, and not just given (a minute and a half),
    /// unless they are left open.
    public static func elsewhere(_ status: AgentStatus?, agent: String, shown: Set<String>, now: Date = Date()) -> [Message] {
        guard let status else { return [] }
        let stale = Set(status.staleTasks ?? [])
        return status.current.filter { task in
            task.to == agent && task.type == "task" && (task.status == "acked" || task.status == "held")
                && !shown.contains(task.id)
                && (stale.contains(task.id) || now.timeIntervalSince(task.createdAt) >= 90)
        }
    }

    /// The line in its chat about them.
    public static func words(_ elsewhere: [Message], status: AgentStatus?, me: String, name: (String) -> String) -> String {
        let stale = Set(status?.staleTasks ?? [])
        let quiet = elsewhere.filter { stale.contains($0.id) }.count
        let n = elsewhere.count
        var text: String
        if quiet == n {
            text = L("它有 \(n) 条别处来的任务好一阵没有动静", n == 1 ? "It has 1 open task from elsewhere with no activity for a while" : "It has \(n) open tasks from elsewhere with no activity for a while")
        } else if quiet > 0 {
            text = L("它还有 \(n) 条别处来的任务没做完，其中 \(quiet) 条好一阵没有动静", "It has \(n) more open tasks from elsewhere, \(quiet) with no activity for a while")
        } else {
            text = L("它还有 \(n) 条别处来的任务没做完", n == 1 ? "It has 1 more open task from elsewhere" : "It has \(n) more open tasks from elsewhere")
        }
        let asking = elsewhere.filter { $0.status == "held" && !($0.asking ?? "").isEmpty && $0.from != me }
        if !asking.isEmpty {
            var who: [String] = []
            for task in asking where !who.contains(name(task.from)) { who.append(name(task.from)) }
            text += L("，其中 \(asking.count) 条在等 \(who.joined(separator: "、")) 回答它的问题", "; \(asking.count) wait on \(who.joined(separator: ", ")) to answer its question")
        }
        return text + L("。", ".")
    }
}
