import Foundation
import MycoAPI

/// 工作流 (the desktop app's Workflows page): decisions waiting for me,
/// the workflows (run one, turn one off) and their runs. Making and
/// editing workflows is for the desktop app and agents.
@MainActor
@Observable
public final class WorkflowsModel {
    private let hub: any HubAPI

    /// nil until the workspace has answered once.
    public private(set) var list: [Workflow]?
    public private(set) var decisions: [WorkflowDecision] = []
    /// The latest runs, newest first.
    public private(set) var runs: [WorkflowRun] = []
    /// The workspace is from before workflows (v0.21).
    public private(set) var unsupported = false
    public private(set) var failure: Failure?
    /// What something is being done to: a workflow, a run, a decision.
    public private(set) var busy: String?

    public init(hub: any HubAPI) {
        self.hub = hub
    }

    public func load() async {
        do {
            async let workflows = hub.workflows()
            async let decisions = hub.workflowDecisions()
            async let runs = hub.workflowRuns(workflow: nil, limit: 30)
            let (w, d, r) = try await (workflows, decisions, runs)
            list = w.sorted { ($0.lastRun?.updatedAt ?? $0.updatedAt) > ($1.lastRun?.updatedAt ?? $1.updatedAt) }
            self.decisions = d.sorted { ($0.step.dueAt ?? .distantFuture) < ($1.step.dueAt ?? .distantFuture) }
            self.runs = r
            unsupported = false
            failure = nil
        } catch let error as HubError where error.status == 404 || Failure.isTooOld(error) {
            unsupported = true
            list = []
            decisions = []
            runs = []
        } catch is CancellationError {
            return
        } catch {
            failure = Failure.of(error, canRetry: true)
            if list == nil { list = [] }
        }
    }

    /// Reads it all until the surrounding task is cancelled.
    public func run() async {
        while !Task.isCancelled {
            await load()
            try? await Task.sleep(for: .seconds(15))
        }
    }

    /// Answers a decision a run waits on me for.
    public func decide(_ decision: WorkflowDecision, choice: String, note: String) async -> Bool {
        await act(decision.id) {
            let note = note.trimmingCharacters(in: .whitespacesAndNewlines)
            _ = try await self.hub.decide(run: decision.run.id, step: decision.step.step, choice: choice, note: note.isEmpty ? nil : note)
        }
    }

    public func setEnabled(_ workflow: Workflow, _ enabled: Bool) async {
        _ = await act(workflow.id) { try await self.hub.setWorkflow(workflow.id, enabled: enabled) }
    }

    public func cancel(_ run: WorkflowRun) async -> WorkflowRun? {
        var after: WorkflowRun?
        _ = await act(run.id) { after = try await self.hub.cancelRun(run.id) }
        return after
    }

    public func retry(_ run: WorkflowRun) async -> WorkflowRun? {
        var after: WorkflowRun?
        _ = await act(run.id) { after = try await self.hub.retryRun(run.id) }
        return after
    }

    /// Starts a run with what was filled in: the run, or nil and `failure`.
    public func start(_ workflow: Workflow, values: [String: String], definition: WorkflowDefinition?) async -> WorkflowRun? {
        var started: WorkflowRun?
        _ = await act(workflow.id) {
            started = try await self.hub.startWorkflow(workflow.id, inputs: Self.inputs(values, definition: definition))
        }
        return started
    }

    /// A workflow with its definition (what a run asks for, who does what).
    public func detail(_ workflow: Workflow) async -> Workflow? {
        try? await hub.workflow(workflow.id)
    }

    /// A run with its steps.
    public func detail(_ run: WorkflowRun) async -> WorkflowRun? {
        do {
            return try await hub.workflowRun(run.id)
        } catch {
            failure = Failure.of(error, canRetry: true)
            return nil
        }
    }

    private func act(_ id: String, _ work: @escaping @MainActor () async throws -> Void) async -> Bool {
        guard busy == nil else { return false }
        busy = id
        defer { busy = nil }
        do {
            try await work()
            failure = nil
        } catch {
            failure = Failure.of(error)
            return false
        }
        await load()
        return true
    }

    // MARK: What a run asks for

    /// One thing a run asks for, as a form shows it.
    public struct Field: Identifiable, Hashable, Sendable {
        public enum Kind: Hashable, Sendable { case text, longText, number, toggle, choice([String]) }
        public var id: String
        public var title: String
        public var note: String?
        public var kind: Kind
        public var required: Bool
        /// What it starts with.
        public var start: String
    }

    /// The fields a run of it asks for, required ones first.
    public static func fields(of definition: WorkflowDefinition?) -> [Field] {
        let required = Set(definition?.inputs?.required ?? [])
        let fields = (definition?.inputs?.properties ?? [:]).map { key, field -> Field in
            let kind: Field.Kind
            if let options = field.enum, !options.isEmpty {
                kind = .choice(options.map(\.text))
            } else {
                switch field.type ?? "string" {
                case "boolean": kind = .toggle
                case "number", "integer": kind = .number
                default: kind = field.format == "textarea" ? .longText : .text
                }
            }
            return Field(
                id: key, title: (field.title?.isEmpty == false ? field.title : nil) ?? key, note: field.description, kind: kind,
                required: required.contains(key), start: field.default?.text ?? (kind == .toggle ? "false" : ""))
        }
        return fields.sorted { ($0.required ? 0 : 1, $0.id) < ($1.required ? 0 : 1, $1.id) }
    }

    /// What is filled in, as the inputs a run takes: numbers and switches
    /// as such, empty ones left out (the workspace fills defaults).
    public static func inputs(_ values: [String: String], definition: WorkflowDefinition?) -> [String: JSONValue] {
        var inputs: [String: JSONValue] = [:]
        for field in fields(of: definition) {
            let raw = (values[field.id] ?? field.start).trimmingCharacters(in: .whitespacesAndNewlines)
            switch field.kind {
            case .toggle: inputs[field.id] = .bool(raw == "true")
            case .number:
                if let number = Double(raw) { inputs[field.id] = .number(number) } else if !raw.isEmpty { inputs[field.id] = .string(raw) }
            default:
                if !raw.isEmpty { inputs[field.id] = .string(raw) }
            }
        }
        // (inputs a definition the phone cannot read asks for, as typed)
        for (key, value) in values where inputs[key] == nil && !value.isEmpty { inputs[key] = .string(value) }
        return inputs
    }

    /// Whether every required field has something.
    public static func ready(_ values: [String: String], definition: WorkflowDefinition?) -> Bool {
        fields(of: definition).allSatisfy { !$0.required || $0.kind == .toggle || !(values[$0.id] ?? $0.start).trimmingCharacters(in: .whitespaces).isEmpty }
    }

    // MARK: Words

    /// A run's state, in words.
    public static func words(of status: String) -> String {
        switch status {
        case "running": return L("在跑", "Running")
        case "waiting": return L("等人决定", "Waiting for a decision")
        case "succeeded": return L("成功", "Succeeded")
        case "failed": return L("失败", "Failed")
        case "cancelled": return L("已取消", "Cancelled")
        default: return status
        }
    }

    /// A step's state, in words.
    public static func words(ofStep status: String) -> String {
        switch status {
        case "running": return L("在做", "At work")
        case "waiting": return L("等决定", "Waiting")
        case "done": return L("完成", "Done")
        case "failed": return L("失败", "Failed")
        case "cancelled": return L("已取消", "Cancelled")
        default: return status
        }
    }

    /// How long a decision may still wait, in words.
    public static func due(_ step: RunStep, now: Date = Date()) -> String? {
        guard let due = step.dueAt else { return nil }
        let seconds = due.timeIntervalSince(now)
        if seconds <= 0 { return L("已过期", "Expired") }
        let hours = Int(seconds / 3_600)
        if hours >= 24 { return L("\(hours / 24) 天内要决定", "Decide within \(hours / 24)d") }
        if hours >= 1 { return L("\(hours) 小时内要决定", "Decide within \(hours)h") }
        return L("\(max(1, Int(seconds / 60))) 分钟内要决定", "Decide within \(max(1, Int(seconds / 60)))m")
    }

    /// How long something took, short.
    public static func duration(_ ms: Int64?) -> String? {
        guard let ms, ms > 0 else { return nil }
        let seconds = Int(ms / 1_000)
        if seconds < 60 { return L("\(seconds) 秒", "\(seconds)s") }
        if seconds < 3_600 { return L("\(seconds / 60) 分钟", "\(seconds / 60)m") }
        return L("\(seconds / 3_600) 小时 \(seconds % 3_600 / 60) 分", "\(seconds / 3_600)h \(seconds % 3_600 / 60)m")
    }
}
