import Foundation
import MycoAPI

/// One thing the agent did with a tool: run a command, read or edit a file.
public struct ToolStep: Identifiable, Hashable, Sendable {
    public var id: Int
    public var tool: String
    public var input: String
    public var failed: Bool
    /// Its result has come back.
    public var finished: Bool
}

/// One item of a plan the agent is working through.
public struct PlanItem: Hashable, Sendable {
    public enum State: Hashable, Sendable {
        case done, doing, todo
    }

    public var text: String
    public var state: State
}

/// A stretch of a run, in the order it happened.
public enum ProcessSegment: Identifiable, Hashable, Sendable {
    /// What it said along the way (Markdown).
    case said(id: Int, text: String)
    /// What it thought.
    case thought(id: Int, text: String)
    /// A run of tool steps between two things it said: shown as one line.
    case steps(id: Int, [ToolStep])
    case failure(id: Int, text: String)

    public var id: Int {
        switch self {
        case .said(let id, _), .thought(let id, _), .steps(let id, _), .failure(let id, _): return id
        }
    }
}

/// How a task was worked on: what the agent said, thought and did, and the
/// plan it ticked off. A port of the rules in web/src/app/TraceView.tsx
/// (docs/design/app-ui.md §6.11, §6.12).
public struct RunProcess: Hashable, Sendable {
    public var segments: [ProcessSegment] = []
    /// The latest plan, when it keeps one.
    public var plan: [PlanItem]?
    /// When that plan was written, when the steps say.
    public var planAt: Date?
    /// What the run took, once it reported it: "12 秒 · 1500 tokens · $0.02".
    public var usage: String?

    public var isEmpty: Bool { segments.isEmpty && plan == nil }
    /// It has something to show under its task besides a plan.
    public var hasSteps: Bool { !segments.isEmpty }

    public init(events: [TraceEvent]) {
        var calls: [String: (segment: Int, step: Int)] = [:]
        for event in events.sorted(by: { $0.seq < $1.seq }) {
            let step = event.data
            switch event.kind {
            case "tool":
                // A plan is shown as a checklist, not as a step.
                if step.tool == "TodoWrite" {
                    if let items = Self.parsePlan(step.input ?? "") {
                        plan = items
                        planAt = event.at
                    }
                    continue
                }
                let tool = ToolStep(id: event.seq, tool: step.tool ?? "", input: step.input ?? "", failed: false, finished: false)
                if case .steps(let id, var steps) = segments.last {
                    steps.append(tool)
                    segments[segments.count - 1] = .steps(id: id, steps)
                } else {
                    segments.append(.steps(id: event.seq, [tool]))
                }
                if let callID = step.id, case .steps(_, let steps) = segments.last {
                    calls[callID] = (segments.count - 1, steps.count - 1)
                }
            case "tool_result":
                guard let callID = step.id, let at = calls[callID], case .steps(let id, var steps) = segments[at.segment] else { continue }
                steps[at.step].finished = true
                steps[at.step].failed = step.failed ?? false
                segments[at.segment] = .steps(id: id, steps)
            case "text":
                if let text = step.text, !text.isEmpty { segments.append(.said(id: event.seq, text: text)) }
            case "thinking":
                if let text = step.text, !text.isEmpty { segments.append(.thought(id: event.seq, text: text)) }
            case "error":
                segments.append(.failure(id: event.seq, text: step.text ?? step.output ?? ""))
            case "usage":
                usage = Self.usageLine(step)
            default:
                break // output
            }
        }
    }

    /// A run's usage in one line, as the desktop app writes it.
    static func usageLine(_ step: TraceStep) -> String? {
        var parts: [String] = []
        if let ms = step.durationMs, ms > 0 {
            let s = Int((ms / 1000).rounded())
            parts.append(s >= 60 ? L("\(s / 60) 分 \(s % 60) 秒", "\(s / 60)m \(s % 60)s") : L("\(s) 秒", "\(s)s"))
        }
        let tokens = (step.inputTokens ?? 0) + (step.outputTokens ?? 0)
        if tokens >= 10_000 {
            parts.append(L(String(format: "%.1f 万 token", Double(tokens) / 10_000), String(format: "%.1fk tokens", Double(tokens) / 1_000)))
        } else if tokens > 0 {
            parts.append("\(tokens) token\(tokens > 1 ? "s" : "")")
        }
        if let cost = step.costUsd, cost > 0 { parts.append(String(format: "$%.2f", cost)) }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    static let readTools: Set<String> = ["Read", "NotebookRead"]
    static let editTools: Set<String> = ["Edit", "Write", "MultiEdit", "NotebookEdit"]
    static let searchTools: Set<String> = ["Grep", "Glob", "LS"]

    /// "读了 3 个文件、跑了 2 条命令、改了 1 个文件".
    public static func summary(of steps: [ToolStep]) -> String {
        var read = 0, run = 0, edit = 0, other = 0, failed = 0
        var files: Set<String> = []
        for step in steps {
            if step.failed { failed += 1 }
            if readTools.contains(step.tool) {
                read += 1
            } else if step.tool == "Bash" {
                run += 1
            } else if editTools.contains(step.tool) {
                edit += 1
                if !step.input.isEmpty { files.insert(step.input) }
            } else {
                other += 1
            }
        }
        func s(_ n: Int) -> String { n > 1 ? "s" : "" }
        var parts: [String] = []
        if read > 0 { parts.append(L("读了 \(read) 个文件", "read \(read) file\(s(read))")) }
        if run > 0 { parts.append(L("跑了 \(run) 条命令", "ran \(run) command\(s(run))")) }
        if edit > 0 {
            let n = files.isEmpty ? edit : files.count
            parts.append(L("改了 \(n) 个文件", "edited \(n) file\(s(n))"))
        }
        if other > 0 { parts.append(L("另外 \(other) 次操作", "\(other) other step\(s(other))")) }
        if failed > 0 { parts.append(L("\(failed) 次失败", "\(failed) failed")) }
        return parts.joined(separator: L("、", ", "))
    }

    /// What a tool step is called: "运行", "读取", "修改", ...
    public static func label(for tool: String) -> String {
        if tool == "Bash" { return L("运行", "Run") }
        if readTools.contains(tool) { return L("读取", "Read") }
        if editTools.contains(tool) { return L("修改", "Edit") }
        if searchTools.contains(tool) { return L("搜索", "Search") }
        if tool == "WebFetch" { return L("打开网页", "Fetch") }
        if tool == "WebSearch" { return L("搜索网页", "Web search") }
        if tool == "Task" || tool == "Agent" { return L("子任务", "Subtask") }
        return tool
    }

    /// One line of a checklist: "[x] done", "[~] doing", "[ ] to do", or
    /// a Markdown task ("- [x] …").
    static func planItem(_ line: String) -> PlanItem? {
        let trimmed = line.trimmingCharacters(in: .whitespaces)
        guard let match = trimmed.wholeMatch(of: /(?:[-*+]\s+)?\[( |x|X|~|-)\]\s+(.+)/) else { return nil }
        let mark = String(match.1)
        let state: PlanItem.State = mark == " " ? .todo : (mark == "~" || mark == "-" ? .doing : .done)
        return PlanItem(text: match.2.trimmingCharacters(in: .whitespaces), state: state)
    }

    /// A checklist (two items or more) and what was said around it: an
    /// agent often writes a line before its plan ("好，计划：") or after
    /// it. The longest checklist of the text is the plan (the desktop
    /// app's `splitPlan`).
    public static func splitPlan(_ text: String) -> (items: [PlanItem], rest: String)? {
        let lines = text.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n")
        func blank(_ index: Int) -> Bool { lines[index].trimmingCharacters(in: .whitespaces).isEmpty }
        var best: (from: Int, to: Int)?
        var i = 0
        while i < lines.count {
            guard planItem(lines[i]) != nil else {
                i += 1
                continue
            }
            // A run of items, with blank lines between them allowed.
            var j = i
            while j + 1 < lines.count, planItem(lines[j + 1]) != nil || blank(j + 1) { j += 1 }
            while j > i, blank(j) { j -= 1 }
            if best.map({ j - i > $0.to - $0.from }) ?? true { best = (i, j) }
            i = j + 1
        }
        guard let best else { return nil }
        let items = lines[best.from...best.to].compactMap(planItem)
        guard items.count >= 2 else { return nil }
        let rest = (lines[..<best.from] + lines[(best.to + 1)...]).joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
        return (items, rest)
    }

    /// A text that is only a checklist.
    public static func parsePlan(_ text: String) -> [PlanItem]? {
        guard let split = splitPlan(text), split.rest.isEmpty else { return nil }
        return split.items
    }
}
