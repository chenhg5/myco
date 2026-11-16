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
    /// When it was made, when the steps say.
    public var at: Date?

    public init(id: Int, tool: String, input: String, failed: Bool, finished: Bool, at: Date? = nil) {
        self.id = id
        self.tool = tool
        self.input = input
        self.failed = failed
        self.finished = finished
        self.at = at
    }
}

/// Something that belongs in a run at the time it was said: an update
/// the agent reported (`myco message progress`), or what its sender added
/// to the task.
public struct RunReport: Identifiable, Hashable, Sendable {
    public var id: String
    public var text: String
    public var at: Date
    public var files: [Attachment]

    public init(id: String, text: String, at: Date, files: [Attachment] = []) {
        self.id = id
        self.text = text
        self.at = at
        self.files = files
    }
}

/// A stretch of a run as a page shows it, in the order it happened: what
/// the agent said and did, with what it reported and what was added to
/// the task where each came.
public enum RunPart: Identifiable, Hashable, Sendable {
    /// What it said along the way (Markdown).
    case said(id: String, text: String)
    /// What it did and thought between two things said: one line that opens.
    case steps(id: String, [ToolStep])
    case failure(id: String, text: String)
    /// Updates it reported one after another: a small timeline.
    case reports(id: String, [RunReport])
    /// Something its sender added to the task while it ran.
    case added(RunReport)

    public var id: String {
        switch self {
        case .said(let id, _), .steps(let id, _), .failure(let id, _), .reports(let id, _): return id
        case .added(let report): return "added-" + report.id
        }
    }
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
    /// What it did and thought between two things it said: shown as one
    /// line ("thought, ran 2 commands") that opens.
    case steps(id: Int, [ToolStep])
    case failure(id: Int, text: String)

    public var id: Int {
        switch self {
        case .said(let id, _), .steps(let id, _), .failure(let id, _): return id
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
    /// When each thing said (or failed) happened, by its segment's id.
    public var times: [Int: Date] = [:]

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
                let tool = ToolStep(id: event.seq, tool: step.tool ?? "", input: step.input ?? "", failed: false, finished: false, at: event.at)
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
                if let text = step.text, !text.isEmpty {
                    segments.append(.said(id: event.seq, text: text))
                    times[event.seq] = event.at
                }
            case "thinking":
                // Thinking is part of the process: folded into the line of
                // steps, there on a tap, not grey prose between what it says.
                guard let text = step.text, !text.isEmpty else { continue }
                let thought = ToolStep(id: event.seq, tool: Self.thinking, input: text, failed: false, finished: true, at: event.at)
                if case .steps(let id, var steps) = segments.last {
                    steps.append(thought)
                    segments[segments.count - 1] = .steps(id: id, steps)
                } else {
                    segments.append(.steps(id: event.seq, [thought]))
                }
            case "error":
                segments.append(.failure(id: event.seq, text: step.text ?? step.output ?? ""))
                times[event.seq] = event.at
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

    /// The run as a page shows it: its words and steps in order, with what
    /// it reported and what was added to the task put in where each came
    /// (after the last step made before it), as the desktop app does.
    ///
    /// What it wrote after its last step is its answer: once the answer is
    /// in (`live` false) that text is the message under the run, and is not
    /// said twice. While it works, it shows: it may be the answer's start.
    /// When some steps carry no time, reports go at the end.
    public func parts(reports: [RunReport], added: [RunReport] = [], live: Bool) -> [RunPart] {
        enum Row {
            case said(Int, String), step(ToolStep), failure(Int, String), report(RunReport), added(RunReport)
        }
        var rows: [(at: Date?, row: Row)] = []
        for segment in segments {
            switch segment {
            case .said(let id, let text): rows.append((times[id], .said(id, text)))
            case .steps(_, let steps):
                for step in steps { rows.append((step.at, .step(step))) }
            case .failure(let id, let text): rows.append((times[id], .failure(id, text)))
            }
        }
        if !live {
            while let last = rows.last, case .said = last.row { rows.removeLast() }
        }
        let timed = rows.allSatisfy { $0.at != nil }
        let extras = (reports.map { ($0.at, Row.report($0)) } + added.map { ($0.at, Row.added($0)) }).sorted { $0.0 < $1.0 }
        for (at, row) in extras {
            var index = rows.count
            if timed, let later = rows.firstIndex(where: { ($0.at ?? .distantPast) > at }) { index = later }
            rows.insert((at, row), at: index)
        }
        var parts: [RunPart] = []
        for (_, row) in rows {
            switch row {
            case .said(let id, let text):
                parts.append(.said(id: "said-\(id)", text: text))
            case .failure(let id, let text):
                parts.append(.failure(id: "failure-\(id)", text: text))
            case .added(let report):
                parts.append(.added(report))
            case .step(let step):
                if case .steps(let id, var steps) = parts.last {
                    steps.append(step)
                    parts[parts.count - 1] = .steps(id: id, steps)
                } else {
                    parts.append(.steps(id: "steps-\(step.id)", [step]))
                }
            case .report(let report):
                if case .reports(let id, var group) = parts.last {
                    group.append(report)
                    parts[parts.count - 1] = .reports(id: id, group)
                } else {
                    parts.append(.reports(id: "reports-" + report.id, [report]))
                }
            }
        }
        return parts
    }

    /// How a thought stands among the steps: a step of this "tool", its
    /// input what was thought.
    public static let thinking = "thinking"

    /// How long a run may be silent after it said something before it
    /// counts as stopped (a session with no `turn_end`: an older myco
    /// following it).
    static let quietAfterSaying: TimeInterval = 120

    /// A command that only keeps a session listening (`myco inbox
    /// wait`/`pull`, `myco message ack`/`done`/`release`, a `cd` before
    /// them): not work on a task. The CLI's `isChore` (trace_chores.go)
    /// and the desktop app's.
    public static func isChore(_ input: String) -> Bool {
        var command = input.trimmingCharacters(in: .whitespacesAndNewlines)
        if let shell = command.range(of: #"^(?:/\S*/)?(?:ba|z)?sh\s+-l?c\s+"#, options: .regularExpression) {
            command.removeSubrange(shell)
        }
        command = command.trimmingCharacters(in: .whitespacesAndNewlines).trimmingCharacters(in: CharacterSet(charactersIn: "\"'"))
        guard !command.isEmpty else { return false }
        var seen = false
        let parts = command.replacingOccurrences(of: #"&&|\|\||;|\n"#, with: "\u{1}", options: .regularExpression).split(separator: "\u{1}")
        for part in parts {
            let first = (part.split(separator: "|", omittingEmptySubsequences: false).first ?? "").trimmingCharacters(in: .whitespaces)
            if first.isEmpty || first.range(of: #"^cd(?:\s|$)"#, options: .regularExpression) != nil { continue }
            guard first.range(of: #"^myco\s+(?:inbox\s+(?:wait|pull)|message\s+(?:ack|done|release))(?:\s|$)"#, options: .regularExpression) != nil
            else { return false }
            seen = true
        }
        return seen
    }

    /// Leaves out the turns a session that receives in its conversation
    /// woke for only to keep listening (its wait timed out and it started
    /// it again): newer CLIs do not upload them, older ones do, and they
    /// read as the agent at work again under an answer already given. The
    /// first turn (the one that took the task) is always kept, and what a
    /// run cost stays. The desktop app's `withoutChores`.
    public static func withoutChores(_ events: [TraceEvent]) -> [TraceEvent] {
        var out: [TraceEvent] = []
        var turn: [TraceEvent] = []
        var first = true
        func worked(_ turn: [TraceEvent]) -> Bool {
            turn.contains { $0.kind == "tool" && !isChore($0.data.input ?? "") }
        }
        for event in events {
            turn.append(event)
            guard event.kind == "turn_end" else { continue }
            let tools = turn.filter { $0.kind == "tool" }
            let chores = !tools.isEmpty && tools.allSatisfy { isChore($0.data.input ?? "") }
            out += first || !chores ? turn : turn.filter { $0.kind == "usage" }
            first = false
            turn = []
        }
        // The turn still going, after one ended: shown once it does
        // something (until then it may be only that).
        out += first || worked(turn) ? turn : turn.filter { $0.kind == "usage" }
        return out
    }

    /// Whether a run still open has stopped, so is not at work: its session
    /// ended its turn and it has done nothing since (a chore is nothing) or
    /// its run (`usage`), it was set aside (held) and nothing came after, or
    /// the last thing it did was say something and it has been silent
    /// since. A command still running is at work however long. The desktop
    /// app's `pausedRun`.
    /// - Parameters:
    ///   - events: the task's steps, oldest first.
    ///   - reports: its progress updates, oldest first.
    public static func isPaused(events: [TraceEvent], task: Message, reports: [Message], now: Date) -> Bool {
        let last = events.last
        let stepAt = last?.at ?? .distantPast
        let reportAt = reports.last?.createdAt ?? .distantPast
        let activeAt = max(stepAt, reportAt)
        // Its run over (usage ends a runner's run: the answer is on its way).
        if let last, last.kind == "usage", reportAt <= stepAt { return true }
        // Its turn over, and not at work since: after a turn ends, only a
        // step that does something (not a chore, not only words) or a new
        // report says it is at it again.
        if let end = events.lastIndex(where: { $0.kind == "turn_end" }), reportAt <= (events[end].at ?? .distantPast),
           !events[(end + 1)...].contains(where: { $0.kind == "tool" && !isChore($0.data.input ?? "") }) {
            return true
        }
        if task.status == "held" { return activeAt <= task.updatedAt }
        guard activeAt > .distantPast else { return false }
        let said = last == nil || last?.kind == "text" || last?.kind == "thinking" || reportAt > stepAt
        return said && now.timeIntervalSince(activeAt) > quietAfterSaying
    }

    static let readTools: Set<String> = ["Read", "NotebookRead"]
    static let editTools: Set<String> = ["Edit", "Write", "MultiEdit", "NotebookEdit"]
    static let searchTools: Set<String> = ["Grep", "Glob", "LS"]

    /// "读了 3 个文件、跑了 2 条命令、改了 1 个文件".
    public static func summary(of steps: [ToolStep]) -> String {
        var read = 0, run = 0, edit = 0, other = 0, failed = 0, thought = 0
        var files: Set<String> = []
        for step in steps {
            if step.tool == thinking {
                thought += 1
                continue
            }
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
        if thought > 0 { parts.append(L("想了一下", "thought")) }
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
        if tool == thinking { return L("想", "Thought") }
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
