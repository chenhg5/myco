import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

/// 工作流 (the desktop app's Workflows page): what waits for my decision on
/// top, then the workflows, then the latest runs. Making and editing them
/// is for the desktop app and agents.
struct WorkflowsScreen: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        let model = session.workflows
        let front = scenePhase != .background
        Group {
            if model.unsupported {
                EmptyState(
                    symbol: "point.3.connected.trianglepath.dotted", title: L("这个工作区还不能用工作流", "No workflows in this workspace yet"),
                    note: L("它的版本太旧。在服务器上运行 sudo myco upgrade，升级到 v0.21 或更新。", "It is too old: run sudo myco upgrade on the server (v0.21 or later)."))
                    .accessibilityIdentifier("workflows.unsupported")
            } else if let list = model.list {
                List {
                    if let failure = model.failure {
                        FailureLine(failure: failure, onRetry: { Task { await model.load() } }).modifier(PlainRow())
                    }
                    if !model.decisions.isEmpty {
                        Section {
                            ForEach(model.decisions) { decision in
                                DecisionCard(decision: decision, model: model, showRun: true).modifier(PlainRow())
                            }
                        } header: {
                            SectionTitle(L("等你决定", "Waiting for you"))
                        }
                    }
                    Section {
                        if list.isEmpty {
                            Text(L("还没有工作流。在电脑上的「工作流」页挑一个现成的，或者跟 agent 说：「用 myco workflow 帮我做一个先写再审的工作流」。",
                                   "No workflows yet. Pick a ready-made one on the desktop's Workflows page, or ask an agent to make one with myco workflow."))
                                .font(.footnote).foregroundStyle(MycoColor.muted)
                                .fixedSize(horizontal: false, vertical: true)
                                .modifier(PlainRow())
                                .accessibilityIdentifier("workflows.empty")
                        }
                        ForEach(list) { workflow in
                            Button {
                                router.push(.workflow(workflow.id))
                            } label: {
                                WorkflowRow(workflow: workflow)
                            }
                            .buttonStyle(RowButtonStyle())
                            .modifier(PlainRow())
                            .accessibilityIdentifier("workflow.row")
                        }
                    } header: {
                        SectionTitle(L("工作流", "Workflows"))
                    }
                    if !model.runs.isEmpty {
                        Section {
                            ForEach(model.runs.prefix(10)) { run in
                                Button {
                                    router.push(.workflowRun(run.id))
                                } label: {
                                    RunRow(run: run)
                                }
                                .buttonStyle(RowButtonStyle())
                                .modifier(PlainRow())
                                .accessibilityIdentifier("workflow.run")
                            }
                        } header: {
                            SectionTitle(L("最近运行", "Latest runs"))
                        }
                    }
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
                .refreshable { await model.load() }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .background(MycoColor.bg)
        .navigationTitle(L("工作流", "Workflows"))
        .navigationBarTitleDisplayMode(.inline)
        .solidTopBar()
        .task(id: front) {
            if front { await model.run() }
        }
    }
}

private struct PlainRow: ViewModifier {
    func body(content: Content) -> some View {
        content
            .listRowInsets(EdgeInsets(top: 8, leading: Space.page, bottom: 8, trailing: Space.page))
            .listRowBackground(MycoColor.bg)
            .listRowSeparator(.hidden)
    }
}

private struct SectionTitle: View {
    let text: String

    init(_ text: String) {
        self.text = text
    }

    var body: some View {
        Text(text).font(.footnote).foregroundStyle(MycoColor.muted)
    }
}

/// A run's state as a dot: going green, waiting amber, failed red, the
/// rest grey.
private func tone(of status: String) -> StatusTone {
    switch status {
    case "running", "succeeded", "done": return .working
    case "waiting": return .attention
    case "failed": return .failed
    default: return .idle
    }
}

/// One workflow: its name, what it is for, how its last run went.
private struct WorkflowRow: View {
    let workflow: Workflow

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            AutomationIcon(symbol: "point.3.connected.trianglepath.dotted", hook: nil).opacity(workflow.enabled ? 1 : 0.5)
            VStack(alignment: .leading, spacing: 3) {
                Text(workflow.shownName).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg)
                if let description = workflow.description, !description.isEmpty {
                    Text(description).font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(2)
                }
                Text(meta).font(.caption).foregroundStyle(MycoColor.muted).lineLimit(1)
            }
            Spacer(minLength: 0)
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }

    private var meta: String {
        var parts: [String] = []
        if let steps = workflow.steps { parts.append(L("\(steps) 步", "\(steps) steps")) }
        if !workflow.enabled {
            parts.append(L("已关闭", "Off"))
        } else if let run = workflow.lastRun {
            parts.append(L("上次：", "Last: ") + WorkflowsModel.words(of: run.status) + " · " + RelativeTime.short(run.updatedAt))
        } else {
            parts.append(L("还没运行过", "Not run yet"))
        }
        return parts.joined(separator: " · ")
    }
}

/// One run: which workflow, how it is going, what it was asked, when.
private struct RunRow: View {
    let run: WorkflowRun

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            StatusDot(tone(of: run.status), size: 8).padding(.top, 6)
            VStack(alignment: .leading, spacing: 2) {
                Text(run.name).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg)
                if let asked = WorkflowRunScreen.asked(run) {
                    Text(asked).font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(1)
                }
                Text([WorkflowsModel.words(of: run.status), RelativeTime.short(run.createdAt)].joined(separator: " · "))
                    .font(.caption).foregroundStyle(MycoColor.muted)
            }
            Spacer(minLength: 0)
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }
}

/// A decision a run waits on me for: what it asks, the choices, a word to
/// go with it, and by when.
struct DecisionCard: View {
    let decision: WorkflowDecision
    let model: WorkflowsModel
    /// Which run it is from (on the list, not on the run's own page).
    var showRun = false
    @State private var note = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if showRun {
                Text(decision.run.name + " · " + decision.step.shownName).font(.caption.weight(.semibold)).foregroundStyle(MycoColor.amber)
            }
            Text(decision.step.input?.isEmpty == false ? decision.step.input! : decision.step.shownName)
                .font(.subheadline).foregroundStyle(MycoColor.fg)
                .fixedSize(horizontal: false, vertical: true)
            TextField(L("附一句话（可以不填）", "A word with it (optional)"), text: $note, axis: .vertical)
                .font(.footnote).lineLimit(1...3)
                .padding(.horizontal, 10).padding(.vertical, 7)
                .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
                .accessibilityIdentifier("decision.note")
            HStack(spacing: 8) {
                ForEach(Array((decision.step.options ?? []).enumerated()), id: \.offset) { index, option in
                    let button = Button(option) {
                        Task { _ = await model.decide(decision, choice: option, note: note) }
                    }
                    .font(.footnote.weight(.medium))
                    .accessibilityIdentifier("decision.option")
                    if index == 0 {
                        button.buttonStyle(.borderedProminent).tint(MycoColor.accent)
                    } else {
                        button.buttonStyle(.bordered).tint(MycoColor.fg2)
                    }
                }
                Spacer(minLength: 0)
                if model.busy == decision.id { ProgressView() }
            }
            .disabled(model.busy != nil)
            if let due = WorkflowsModel.due(decision.step) {
                Text(due).font(.caption).foregroundStyle(MycoColor.muted)
            }
        }
        .padding(12)
        .background(MycoColor.amberSoft, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("decision")
    }
}

/// A workflow: what it does, step by step, who does each; run it, turn it
/// off; its runs.
struct WorkflowScreen: View {
    let id: String
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @State private var workflow: Workflow?
    @State private var starting = false

    var body: some View {
        let model = session.workflows
        let listed = model.list?.first { $0.id == id }
        let shown = workflow ?? listed
        Group {
            if let shown {
                List {
                    Section {
                        VStack(alignment: .leading, spacing: 6) {
                            if let description = shown.description, !description.isEmpty {
                                Text(description).font(.subheadline).foregroundStyle(MycoColor.fg2)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                            Text(meta(shown)).font(.caption).foregroundStyle(MycoColor.muted)
                        }
                        .modifier(PlainRow())
                        Button {
                            starting = true
                        } label: {
                            Label(L("运行", "Run"), systemImage: "play.fill")
                        }
                        .buttonStyle(PrimaryButtonStyle())
                        .disabled(!(listed?.enabled ?? shown.enabled) || workflow == nil)
                        .modifier(PlainRow())
                        .accessibilityIdentifier("workflow.start")
                        Toggle(L("开着（关掉就不能运行）", "On (off: it cannot be run)"), isOn: SwiftUI.Binding(
                            get: { listed?.enabled ?? shown.enabled },
                            set: { on in Task { await model.setEnabled(shown, on) } }))
                            .tint(MycoColor.green)
                            .font(.subheadline)
                            .disabled(model.busy != nil)
                            .modifier(PlainRow())
                            .accessibilityIdentifier("workflow.enabled")
                        if let failure = model.failure { FailureLine(failure: failure).modifier(PlainRow()) }
                    }
                    if let steps = shown.definition?.steps, !steps.isEmpty {
                        Section {
                            ForEach(Array(steps.enumerated()), id: \.element.id) { index, step in
                                HStack(alignment: .firstTextBaseline, spacing: 10) {
                                    Text("\(index + 1)").font(.caption.monospacedDigit().weight(.semibold)).foregroundStyle(MycoColor.muted)
                                        .frame(width: 18)
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text((step.title?.isEmpty == false ? step.title : nil) ?? step.id).font(.subheadline).foregroundStyle(MycoColor.fg)
                                        Text(who(step)).font(.caption).foregroundStyle(MycoColor.muted)
                                    }
                                }
                                .modifier(PlainRow())
                            }
                        } header: {
                            SectionTitle(L("步骤", "Steps"))
                        }
                    }
                    let runs = model.runs.filter { $0.workflowId == id }
                    if !runs.isEmpty {
                        Section {
                            ForEach(runs) { run in
                                Button {
                                    router.push(.workflowRun(run.id))
                                } label: {
                                    RunRow(run: run)
                                }
                                .buttonStyle(RowButtonStyle())
                                .modifier(PlainRow())
                            }
                        } header: {
                            SectionTitle(L("运行记录", "Runs"))
                        }
                    }
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .background(MycoColor.bg)
        .navigationTitle(shown?.shownName ?? L("工作流", "Workflow"))
        .navigationBarTitleDisplayMode(.inline)
        .solidTopBar()
        .task {
            if let listed { workflow = await model.detail(listed) }
            if workflow == nil {
                await model.load()
                if let again = model.list?.first(where: { $0.id == id }) { workflow = await model.detail(again) }
            }
        }
        .sheet(isPresented: $starting) {
            if let workflow {
                StartRunSheet(workflow: workflow) { run in
                    starting = false
                    Task {
                        try? await Task.sleep(for: .milliseconds(400))
                        router.push(.workflowRun(run.id))
                    }
                }
            }
        }
    }

    private func meta(_ workflow: Workflow) -> String {
        var parts = [L("第 \(workflow.version) 版", "Version \(workflow.version)")]
        if let project = workflow.project, !project.isEmpty { parts.append(session.project(project)?.title ?? project) }
        if let owner = workflow.owner, !owner.isEmpty { parts.append(owner) }
        return parts.joined(separator: " · ")
    }

    /// Who does a step, in words.
    private func who(_ step: WorkflowDefinition.Step) -> String {
        if let agent = step.agent, !agent.isEmpty { return L("agent：", "Agent: ") + session.displayName(of: agent) }
        if let role = step.role, !role.isEmpty { return L("分工是「\(role)」的 agent", "An agent whose role is \(role)") }
        if let computer = step.computer, !computer.isEmpty { return L("电脑：", "Computer: ") + session.displayName(of: computer) }
        if let human = step.human, !human.isEmpty {
            return human == "$starter" ? L("运行它的人来决定", "Decided by whoever ran it") : L("由 \(human) 决定", "Decided by \(human)")
        }
        return ""
    }
}

/// What a run asks for, filled in, and go.
private struct StartRunSheet: View {
    let workflow: Workflow
    let onStarted: (WorkflowRun) -> Void
    @Environment(WorkspaceSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var values: [String: String] = [:]

    var body: some View {
        let model = session.workflows
        let fields = WorkflowsModel.fields(of: workflow.definition)
        NavigationStack {
            Form {
                if fields.isEmpty {
                    Section {
                        Text(L("它不用填什么，直接运行。", "It needs nothing: just run it.")).font(.footnote).foregroundStyle(MycoColor.muted)
                    }
                }
                ForEach(fields) { field in
                    Section {
                        input(field)
                    } header: {
                        Text(field.title + (field.required ? " *" : ""))
                    } footer: {
                        if let note = field.note, !note.isEmpty { Text(note) }
                    }
                }
                if let failure = model.failure {
                    Section { FailureLine(failure: failure) }
                }
            }
            .scrollContentBackground(.hidden)
            .background(MycoColor.bg)
            .navigationTitle(L("运行「\(workflow.shownName)」", "Run \(workflow.shownName)"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L("取消", "Cancel")) { dismiss() }.disabled(model.busy != nil)
                }
                ToolbarItem(placement: .confirmationAction) {
                    if model.busy == workflow.id {
                        ProgressView()
                    } else {
                        Button(L("运行", "Run")) {
                            Task {
                                if let run = await model.start(workflow, values: values, definition: workflow.definition) { onStarted(run) }
                            }
                        }
                        .fontWeight(.semibold)
                        .disabled(!WorkflowsModel.ready(values, definition: workflow.definition))
                        .accessibilityIdentifier("workflow.run.go")
                    }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .interactiveDismissDisabled(model.busy != nil)
    }

    private func binding(_ field: WorkflowsModel.Field) -> SwiftUI.Binding<String> {
        SwiftUI.Binding(get: { values[field.id] ?? field.start }, set: { values[field.id] = $0 })
    }

    @ViewBuilder private func input(_ field: WorkflowsModel.Field) -> some View {
        switch field.kind {
        case .text:
            TextField(field.title, text: binding(field)).accessibilityIdentifier("workflow.input." + field.id)
        case .longText:
            TextField(field.title, text: binding(field), axis: .vertical).lineLimit(3...8).accessibilityIdentifier("workflow.input." + field.id)
        case .number:
            TextField(field.title, text: binding(field)).keyboardType(.decimalPad).accessibilityIdentifier("workflow.input." + field.id)
        case .toggle:
            Toggle(field.title, isOn: SwiftUI.Binding(get: { (values[field.id] ?? field.start) == "true" }, set: { values[field.id] = $0 ? "true" : "false" }))
                .tint(MycoColor.green)
        case .choice(let options):
            Picker(field.title, selection: binding(field)) {
                Text(L("不选", "None")).tag("")
                ForEach(options, id: \.self) { Text($0).tag($0) }
            }
        }
    }
}

/// A run: how it is going, what it was asked, each step (who, how it went,
/// what it said), the decision it waits on; cancel it, or start a failed
/// one again.
struct WorkflowRunScreen: View {
    let id: String
    @Environment(WorkspaceSession.self) private var session
    @State private var run: WorkflowRun?
    @State private var cancelling = false

    /// What a run was asked to do, in one line (its first text input).
    static func asked(_ run: WorkflowRun) -> String? {
        guard let inputs = run.inputs, !inputs.isEmpty else { return nil }
        let text = inputs.sorted { $0.key < $1.key }.map(\.value.text).first { !$0.isEmpty && !["true", "false"].contains($0) }
        return text?.split(separator: "\n").first.map(String.init)
    }

    var body: some View {
        let model = session.workflows
        Group {
            if let run {
                List {
                    Section {
                        HStack(spacing: 8) {
                            StatusDot(tone(of: run.status), size: 9)
                            Text(WorkflowsModel.words(of: run.status)).font(.headline).foregroundStyle(MycoColor.fg)
                            Spacer(minLength: 0)
                            Text(RelativeTime.short(run.createdAt)).font(.caption).foregroundStyle(MycoColor.muted)
                        }
                        .modifier(PlainRow())
                        .accessibilityElement(children: .combine)
                        .accessibilityIdentifier("run.status")
                        if let error = run.error, !error.isEmpty {
                            Text(error).font(.footnote).foregroundStyle(MycoColor.red).modifier(PlainRow())
                        }
                        ForEach((run.inputs ?? [:]).sorted { $0.key < $1.key }, id: \.key) { key, value in
                            KeyValueRow(key, value.text).modifier(PlainRow())
                        }
                        if let failure = model.failure { FailureLine(failure: failure).modifier(PlainRow()) }
                        if run.isGoing {
                            Button(L("取消这次运行", "Cancel this run"), role: .destructive) { cancelling = true }
                                .font(.subheadline).modifier(PlainRow())
                                .disabled(model.busy != nil)
                                .accessibilityIdentifier("run.cancel")
                        } else if run.status == "failed" || run.status == "cancelled" {
                            Button(L("从出错的那一步重来", "Start again from the step that failed")) {
                                Task { if let after = await model.retry(run) { self.run = after } }
                            }
                            .font(.subheadline).tint(MycoColor.accent).modifier(PlainRow())
                            .disabled(model.busy != nil)
                            .accessibilityIdentifier("run.retry")
                        }
                    }
                    Section {
                        ForEach(run.steps ?? []) { step in
                            if step.kind == "human", step.status == "waiting", step.target == session.me || step.target == nil {
                                DecisionCard(decision: WorkflowDecision(run: run, step: step), model: model).modifier(PlainRow())
                            } else {
                                StepRow(step: step, who: who(step)).modifier(PlainRow())
                            }
                        }
                    } header: {
                        SectionTitle(L("步骤", "Steps"))
                    }
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
                .refreshable { await reload() }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .background(MycoColor.bg)
        .navigationTitle(run?.name ?? L("运行", "Run"))
        .navigationBarTitleDisplayMode(.inline)
        .solidTopBar()
        // (read again while it goes)
        .task {
            while !Task.isCancelled {
                await reload()
                guard run?.isGoing ?? true else { break }
                try? await Task.sleep(for: .seconds(5))
            }
        }
        .onChange(of: model.decisions.count) { Task { await reload() } }
        .confirmationDialog(L("取消这次运行？", "Cancel this run?"), isPresented: $cancelling, titleVisibility: .visible) {
            Button(L("取消运行", "Cancel the run"), role: .destructive) {
                Task { if let run, let after = await model.cancel(run) { self.run = after } }
            }
            Button(L("不了", "Keep it"), role: .cancel) {}
        } message: {
            Text(L("还开着的任务会停下，等的决定会撤回。", "Open tasks are stopped and waiting decisions withdrawn."))
        }
    }

    private func reload() async {
        if let fresh = await session.workflows.detail(WorkflowRun(id: id, workflowId: "", name: "", status: "")) { run = fresh }
    }

    private func who(_ step: RunStep) -> String {
        guard let target = step.target, !target.isEmpty else { return "" }
        return target == session.me ? L("你", "You") : session.displayName(of: target)
    }
}

/// One step of a run: its name, who, how it went, what it said, how long.
private struct StepRow: View {
    let step: RunStep
    let who: String

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            StatusDot(tone(of: step.status), size: 8).padding(.top, 6)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(step.shownName).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg)
                    if let attempt = step.attempt, attempt > 1 {
                        Text(L("第 \(attempt) 次", "Try \(attempt)")).font(.caption).foregroundStyle(MycoColor.muted)
                    }
                }
                Text([who, WorkflowsModel.words(ofStep: step.status), WorkflowsModel.duration(step.durationMs)].compactMap { $0 }.filter { !$0.isEmpty }
                    .joined(separator: " · "))
                    .font(.caption).foregroundStyle(MycoColor.muted)
                if let error = step.error, !error.isEmpty {
                    Text(error).font(.footnote).foregroundStyle(MycoColor.red).lineLimit(3)
                } else if let summary = step.summary, !summary.isEmpty {
                    Text(summary).font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(4)
                }
            }
            Spacer(minLength: 0)
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("run.step")
    }
}
