import Foundation
import MycoAPI
import MycoCrypto

/// A conversation as a Markdown file, to read, share or keep: who said
/// what to whom and when, the attachments by name, and, when asked for,
/// each task's tool calls folded under it. The same shape as the desktop
/// app's export and `myco thread export`, so a file from the phone and
/// one from a computer read alike.
public enum ConversationExport {
    /// One tool call of a task's run.
    public struct Step: Sendable, Hashable {
        public var tool: String
        public var input: String
        public var failed: Bool
    }

    static let kinds: [String: String] = [
        "task": "任务", "reply": "回复", "progress": "进展", "steer": "补充", "note": "留言", "event": "事件",
    ]

    /// The tool calls in a task's steps, in order; one whose result failed is marked.
    public static func steps(of events: [TraceEvent]) -> [Step] {
        var steps: [Step] = []
        var place: [String: Int] = [:]
        for event in events.sorted(by: { $0.seq < $1.seq }) {
            let step = event.data
            if event.kind == "tool", let tool = step.tool, !tool.isEmpty {
                if let id = step.id, !id.isEmpty { place[id] = steps.count }
                steps.append(Step(tool: tool, input: oneLine(step.input ?? "", limit: 160), failed: false))
            } else if event.kind == "tool_result", step.failed == true, let id = step.id, let index = place[id] {
                steps[index].failed = true
            }
        }
        return steps
    }

    static func oneLine(_ text: String, limit: Int) -> String {
        let line = text.split(whereSeparator: \.isWhitespace).joined(separator: " ")
        return line.count > limit ? String(line.prefix(limit - 1)) + "…" : line
    }

    static func size(_ bytes: Int64) -> String {
        if bytes < 1024 { return "\(bytes) B" }
        if bytes < 1024 * 1024 { return "\(Int((Double(bytes) / 1024).rounded())) KB" }
        return String(format: "%.1f MB", Double(bytes) / 1024 / 1024)
    }

    private static func formatter(_ format: String, _ zone: TimeZone) -> DateFormatter {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = zone
        formatter.dateFormat = format
        return formatter
    }

    /// - Parameters:
    ///   - messages: what the conversation shows, in any order.
    ///   - name: a person's or agent's name as the chat shows it.
    ///   - steps: each task's tool calls, by task; left out when not asked for.
    public static func markdown(
        _ messages: [Message], title: String, name: (String) -> String, steps: [String: [Step]] = [:],
        now: Date = Date(), timeZone: TimeZone = .current
    ) -> String {
        let stamp = formatter("MM-dd HH:mm", timeZone)
        var lines = ["# \(title)", "", "导出时间：\(formatter("yyyy-MM-dd HH:mm", timeZone).string(from: now)) · 共 \(messages.count) 条消息"]
        for message in messages.sorted(by: { ($0.createdAt, $0.id) < ($1.createdAt, $1.id) }) {
            var head = "**\(name(message.from))** → \(name(message.to)) · \(stamp.string(from: message.createdAt)) · \(kinds[message.type] ?? message.type)"
            if message.status == "failed" {
                let error = message.error ?? ""
                head += error == RunState.stoppedBySender ? " · 已停止" : error.isEmpty ? " · 失败" : " · 失败：\(error)"
            }
            lines += ["", "---", "", head, ""]
            if let unreadable = message.unreadable, !unreadable.isEmpty {
                lines.append("（读不出：\(unreadable)）")
            } else if Envelope.isSealed(message.body) {
                lines.append("（已加密，这台设备读不了）")
            } else {
                lines.append(message.body)
            }
            if !message.attachments.isEmpty {
                lines += ["", "附件：" + message.attachments.map { "\($0.name)（\(size($0.size))）" }.joined(separator: "、")]
            }
            if let taken = steps[message.id], !taken.isEmpty {
                lines += ["", "<details><summary>过程：\(taken.count) 步</summary>", ""]
                lines += taken.map { step in
                    "- " + step.tool + (step.input.isEmpty ? "" : " `" + step.input.replacingOccurrences(of: "`", with: "'") + "`") + (step.failed ? "（失败）" : "")
                }
                lines += ["", "</details>"]
            }
        }
        return lines.joined(separator: "\n") + "\n"
    }

    /// A file name for the export: who it was with and the day, safe on every system.
    public static func fileName(with whom: String, now: Date = Date(), timeZone: TimeZone = .current) -> String {
        let unsafe = CharacterSet(charactersIn: "\\/:*?\"<>|").union(.controlCharacters)
        let parts = whom.components(separatedBy: unsafe).filter { !$0.isEmpty }
        let who = String(parts.joined(separator: " ").trimmingCharacters(in: .whitespaces).prefix(60))
        return "和 \(who.isEmpty ? "myco" : who) 的对话 \(formatter("yyyy-MM-dd", timeZone).string(from: now)).md"
    }
}
