import SwiftUI
import UIKit
import MycoAPI
import MycoCore
import MycoUI

/// Automations: a task sent for me to one agent, on a schedule or when an
/// outside system calls its webhook. They run in the workspace, so they go
/// on while every computer is off; what an agent answers is in my chat
/// with it, the task marked 自动.
///
/// The page is a list of them, each saying what it does in a line: when,
/// to whom, when next, how the last one went. A switch turns one off; a
/// swipe sends it now or deletes it. With none yet, a few common ones to
/// start from. The desktop app's `Automations.tsx`.
struct AutomationsScreen: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(\.scenePhase) private var scenePhase

    /// The one being written: a new one, or one that exists.
    @State private var editing: Editing?
    @State private var removing: Automation?
    @State private var now = Date()

    enum Editing: Identifiable, Hashable {
        case new(AutomationDraft.Template?)
        case existing(Automation)

        var id: String {
            switch self {
            case .new(let template): return "new:" + (template?.name ?? "")
            case .existing(let automation): return automation.id
            }
        }
    }

    var body: some View {
        let model = session.automations
        let front = scenePhase != .background
        Group {
            if model.unsupported {
                EmptyState(
                    symbol: "clock", title: L("这个工作区还不能用自动化", "No automations in this workspace yet"),
                    note: L("它的版本太旧。在服务器上运行 sudo myco upgrade，升级到 v0.19.22 或更新。", "It is too old: run sudo myco upgrade on the server (v0.19.22 or later)."))
                    .accessibilityIdentifier("automations.unsupported")
            } else if let list = model.list {
                if list.isEmpty, model.failure == nil {
                    starting
                } else {
                    rows(list, model)
                }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .background(MycoColor.bg)
        .navigationTitle(L("自动化", "Automations"))
        .navigationBarTitleDisplayMode(.inline)
        .solidTopBar()
        .toolbar {
            if !model.unsupported, !(model.list ?? []).isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        editing = .new(nil)
                    } label: {
                        Image(systemName: "plus")
                    }
                    .accessibilityLabel(L("新建自动化", "New automation"))
                    .accessibilityIdentifier("automations.new")
                }
            }
        }
        .task(id: front) {
            if front { await model.run() }
        }
        .sheet(item: $editing) { editing in
            AutomationEditor(editing: editing)
        }
        .confirmationDialog(
            L("删除这个自动化？", "Delete this automation?"), isPresented: SwiftUI.Binding(get: { removing != nil }, set: { if !$0 { removing = nil } }),
            titleVisibility: .visible, presenting: removing
        ) { automation in
            Button(L("删除", "Delete"), role: .destructive) { Task { await model.remove(automation) } }
            Button(L("取消", "Cancel"), role: .cancel) {}
        } message: { _ in
            Text(Self.deleteNote)
        }
    }

    static var deleteNote: String {
        L("它不会再发消息了，Webhook 地址也失效。已经发出去的消息和 agent 的回复都保留。", "It sends nothing more, and its webhook address stops working. What it sent and the answers stay.")
    }

    /// None yet: what they are for, and a few common ones to start from.
    private var starting: some View {
        ScrollView {
            VStack(spacing: Space.m) {
                VStack(spacing: 8) {
                    Image(systemName: "clock").font(.system(size: 30, weight: .regular)).foregroundStyle(MycoColor.muted)
                        .padding(.bottom, 4)
                    Text(L("让 agent 按时自己干活", "Let agents work on their own schedule")).font(.headline).foregroundStyle(MycoColor.fg)
                    Text(L("比如每天早上汇总 issue、CI 失败时看日志。它的回复在你和它的对话里，标着「自动」。跑在工作区里，电脑关着也照常。",
                           "Say, a morning summary of issues, or a look at the log when CI fails. Its answers are in your chat with it, marked Auto. They run in the workspace, with every computer off."))
                        .font(.subheadline).foregroundStyle(MycoColor.fg2).multilineTextAlignment(.center)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .padding(.horizontal, 12).padding(.top, 36).padding(.bottom, 10)
                ForEach(AutomationDraft.templates) { template in
                    Button {
                        editing = .new(template)
                    } label: {
                        card(symbol: template.symbol, hook: template.hook, title: template.name, note: template.hint)
                    }
                    .buttonStyle(PressScaleStyle())
                    .accessibilityIdentifier("automations.template")
                }
                Button {
                    editing = .new(nil)
                } label: {
                    card(symbol: "plus", hook: nil, title: L("从空白开始", "Start from blank"), note: L("自己写要它做什么、什么时候做", "Write what it does, and when"))
                }
                .buttonStyle(PressScaleStyle())
                .accessibilityIdentifier("automations.blank")
            }
            .padding(.horizontal, Space.page)
            .padding(.bottom, 24)
        }
        .accessibilityIdentifier("automations.empty")
    }

    private func card(symbol: String, hook: Bool?, title: String, note: String) -> some View {
        HStack(alignment: .top, spacing: 12) {
            AutomationIcon(symbol: symbol, hook: hook)
            VStack(alignment: .leading, spacing: 3) {
                Text(title).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg)
                Text(note).font(.footnote).foregroundStyle(MycoColor.muted).multilineTextAlignment(.leading)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 0)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).strokeBorder(MycoColor.line, lineWidth: 1))
    }

    private func rows(_ list: [Automation], _ model: AutomationsModel) -> some View {
        List {
            Section {
                if let failure = model.failure {
                    FailureLine(failure: failure, onRetry: failure.kind == .transient ? { Task { await model.load() } } : nil)
                        .listRowBackground(MycoColor.bg)
                        .listRowSeparator(.hidden)
                }
                ForEach(list) { automation in
                    AutomationRow(
                        automation: automation, to: session.displayName(of: automation.to),
                        face: session.agent(automation.to).map(AvatarView.Face.of), now: now, busy: model.busy.contains(automation.id),
                        onOpen: { editing = .existing(automation) },
                        onToggle: { on in Task { await model.setEnabled(automation, on) } })
                        .listRowInsets(EdgeInsets(top: 10, leading: 16, bottom: 10, trailing: 16))
                        .listRowBackground(MycoColor.bg)
                        .swipeActions(edge: .leading, allowsFullSwipe: true) {
                            Button {
                                Task { await model.runNow(automation) }
                            } label: {
                                Label(L("立即运行", "Run now"), systemImage: "play.fill")
                            }
                            .tint(MycoColor.green)
                        }
                        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                            Button {
                                removing = automation
                            } label: {
                                Label(L("删除", "Delete"), systemImage: "trash")
                            }
                            .tint(MycoColor.red)
                        }
                        .contextMenu {
                            Button(L("编辑", "Edit"), systemImage: "pencil") { editing = .existing(automation) }
                            Button(L("立即运行", "Run now"), systemImage: "play") { Task { await model.runNow(automation) } }
                            Button(automation.enabled ? L("暂停", "Pause") : L("启用", "Resume"), systemImage: automation.enabled ? "pause" : "play.circle") {
                                Task { await model.setEnabled(automation, !automation.enabled) }
                            }
                            Divider()
                            Button(L("删除", "Delete"), systemImage: "trash", role: .destructive) { removing = automation }
                        }
                }
                // (a row, not the section's footer: a plain list draws a
                // band behind that one)
                Text(L("到点或者外部系统通知时，替你给 agent 派一个任务。往右滑立即运行一次。", "On a schedule, or when an outside system calls, a task goes to an agent for you. Swipe right to run one now."))
                    .font(.footnote).foregroundStyle(MycoColor.muted)
                    .listRowInsets(EdgeInsets(top: 14, leading: 16, bottom: 14, trailing: 16))
                    .listRowBackground(MycoColor.bg)
                    .listRowSeparator(.hidden)
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .refreshable { await model.load() }
        // (so "next" and "last" stay true while the page is open)
        .task {
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(30))
                now = Date()
            }
        }
        .accessibilityIdentifier("automations.list")
    }
}

/// The small tile that says what kind it is: on a schedule (green) or by
/// webhook (amber); neither for the blank one.
struct AutomationIcon: View {
    let symbol: String
    /// nil: neither kind.
    let hook: Bool?

    var body: some View {
        Image(systemName: symbol)
            .font(.system(size: 15, weight: .medium))
            .foregroundStyle(hook == nil ? MycoColor.muted : hook == true ? MycoColor.amber : MycoColor.green)
            .frame(width: 34, height: 34)
            .background(hook == nil ? MycoColor.pill : hook == true ? MycoColor.amberSoft : MycoColor.greenSoft, in: RoundedRectangle(cornerRadius: 9, style: .continuous))
    }
}

/// One automation in the list: what it does in three lines, and its switch.
private struct AutomationRow: View {
    let automation: Automation
    let to: String
    let face: AvatarView.Face?
    let now: Date
    let busy: Bool
    let onOpen: () -> Void
    let onToggle: (Bool) -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Button(action: onOpen) {
                HStack(alignment: .top, spacing: 12) {
                    AutomationIcon(symbol: automation.schedule != nil ? "clock" : "point.3.connected.trianglepath.dotted", hook: automation.schedule == nil)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(automation.name).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg).lineLimit(1)
                        HStack(spacing: 5) {
                            Text(AutomationWords.when(automation)).lineLimit(1)
                            Text(L("发给", "to")).foregroundStyle(MycoColor.muted)
                            if let face { AvatarView(face, size: 16) }
                            Text(to).fontWeight(.medium).lineLimit(1)
                        }
                        .font(.footnote).foregroundStyle(MycoColor.fg2)
                        HStack(spacing: 8) {
                            if let standing = AutomationWords.standing(automation, now: now) {
                                Text(standing)
                            }
                            if let last = automation.lastRun {
                                HStack(spacing: 4) {
                                    StatusDot(last.outcome == "sent" ? .working : last.outcome == "failed" ? .failed : .idle, size: 6)
                                    Text(L("上次 \(RelativeTime.short(last.at, now: now)) · ", "Last \(RelativeTime.short(last.at, now: now)) · ") + AutomationWords.outcome(last))
                                        .lineLimit(1)
                                }
                            }
                        }
                        .font(.caption).foregroundStyle(MycoColor.muted)
                    }
                    Spacer(minLength: 0)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .opacity(automation.enabled ? 1 : 0.55)
            .accessibilityIdentifier("automation.row")
            Toggle("", isOn: SwiftUI.Binding(get: { automation.enabled }, set: onToggle))
                .labelsHidden()
                .tint(MycoColor.green)
                .disabled(busy)
                .accessibilityLabel(automation.enabled ? L("暂停「\(automation.name)」", "Pause \(automation.name)") : L("启用「\(automation.name)」", "Resume \(automation.name)"))
                .accessibilityIdentifier("automation.switch")
        }
    }
}

/// Writing an automation, top to bottom as it works: its name, to whom,
/// what, when. Saving one with a webhook ends on a page with its address
/// and secret, shown that once.
struct AutomationEditor: View {
    let editing: AutomationsScreen.Editing
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session

    @State private var draft = AutomationDraft()
    @State private var loaded = false
    @State private var busy = false
    @State private var failure: Failure?
    @State private var runs: [AutomationRun] = []
    @State private var secret: Secret?
    @State private var deleting = false

    struct Secret: Hashable {
        var url: String
        var value: String
    }

    private var existing: Automation? {
        if case .existing(let automation) = editing { return automation }
        return nil
    }

    var body: some View {
        NavigationStack {
            Group {
                if let secret {
                    WebhookSecretPage(secret: secret) { dismiss() }
                } else {
                    form
                }
            }
            .background(MycoColor.bg)
            .navigationBarTitleDisplayMode(.inline)
        }
        .interactiveDismissDisabled(busy || secret != nil)
        .task {
            guard !loaded else { return }
            loaded = true
            switch editing {
            case .new(let template):
                draft = AutomationDraft(template: template, to: session.agentsByName.first?.name ?? "")
            case .existing(let automation):
                draft = AutomationDraft(automation)
                runs = await session.automations.runs(of: automation.id)
            }
        }
    }

    private var form: some View {
        Form {
            Section {
                TextField(L("名字，比如：每天早上汇总 issue", "Name, e.g. Morning issue summary"), text: $draft.name)
                    .accessibilityIdentifier("automation.name")
                Picker(L("发给谁", "To whom"), selection: $draft.to) {
                    ForEach(session.agentsByName) { agent in
                        Text(session.displayName(of: agent.name)).tag(agent.name)
                    }
                    // (one that left the workspace still shows what was chosen)
                    if !draft.to.isEmpty, session.agent(draft.to) == nil { Text(draft.to).tag(draft.to) }
                }
                .accessibilityIdentifier("automation.to")
            }
            Section {
                TextField(
                    L("比如：看一下昨天新开的 issue，挑出要紧的，在这里回我一份清单。", "e.g. Look at yesterday's new issues, pick the urgent ones, and reply with a list."),
                    text: $draft.body, axis: .vertical)
                    .lineLimit(3...10)
                    .accessibilityIdentifier("automation.body")
            } header: {
                Text(L("让它做什么", "What to ask"))
            } footer: {
                if draft.hook {
                    VStack(alignment: .leading, spacing: 6) {
                        Text(L("外部系统调用时带来的内容，会放在 {{payload}} 的位置；没写就接在最后。", "What a call sends goes where {{payload}} is; without it, at the end."))
                        if !draft.body.contains("{{payload}}") {
                            Button(L("在最后插入 {{payload}}", "Insert {{payload}} at the end")) { draft.insertPayload() }
                                .font(.footnote.weight(.medium))
                        }
                    }
                }
            }
            Section {
                Toggle(isOn: $draft.timed) {
                    Label {
                        VStack(alignment: .leading, spacing: 1) {
                            Text(L("定时", "On a schedule"))
                            Text(L("到点就发", "Sent when it is time")).font(.footnote).foregroundStyle(MycoColor.muted)
                        }
                    } icon: {
                        Image(systemName: "clock").foregroundStyle(MycoColor.green)
                    }
                }
                .tint(MycoColor.green)
                .accessibilityIdentifier("automation.timed")
                if draft.timed { schedule }
                Toggle(isOn: $draft.hook) {
                    Label {
                        VStack(alignment: .leading, spacing: 1) {
                            Text("Webhook")
                            Text(L("CI、监控等调用时", "When CI, monitoring… call")).font(.footnote).foregroundStyle(MycoColor.muted)
                        }
                    } icon: {
                        Image(systemName: "point.3.connected.trianglepath.dotted").foregroundStyle(MycoColor.amber)
                    }
                }
                .tint(MycoColor.green)
                .accessibilityIdentifier("automation.hook")
            } header: {
                Text(L("什么时候", "When"))
            } footer: {
                if draft.hook, existing?.webhook != true {
                    Text(L("保存后给出调用地址和密钥。", "The address and secret come after saving."))
                }
            }
            Section {
                Toggle(isOn: $draft.skip) {
                    VStack(alignment: .leading, spacing: 1) {
                        Text(L("上一次的任务还没做完，就跳过这次", "Skip a run while the last task is not finished"))
                        Text(L("免得 agent 那边越堆越多。", "So that work does not pile up at the agent.")).font(.footnote).foregroundStyle(MycoColor.muted)
                    }
                }
                .tint(MycoColor.green)
            } footer: {
                Text(L("自动化的内容存在工作区里，不是端到端加密的。", "An automation's text is kept in the workspace, not end-to-end encrypted."))
            }
            if !runs.isEmpty {
                Section(L("最近运行", "Recent runs")) {
                    ForEach(runs.prefix(8)) { run in
                        HStack(alignment: .firstTextBaseline, spacing: 8) {
                            StatusDot(run.outcome == "sent" ? .working : run.outcome == "failed" ? .failed : .idle, size: 6)
                            Text(RelativeTime.short(run.at) + " · " + AutomationWords.trigger(run)).foregroundStyle(MycoColor.muted)
                            Text(AutomationWords.outcome(run)).foregroundStyle(MycoColor.fg2)
                            Spacer(minLength: 0)
                        }
                        .font(.footnote)
                    }
                }
                .accessibilityIdentifier("automation.runs")
            }
            if let failure {
                Section {
                    FailureLine(failure: failure)
                }
            }
            if let existing {
                Section {
                    if existing.webhook {
                        Button(L("重新生成 Webhook 密钥", "New webhook secret")) { rotate(existing) }
                            .disabled(busy)
                    }
                    Button(L("删除这个自动化", "Delete this automation"), role: .destructive) { deleting = true }
                        .disabled(busy)
                        .accessibilityIdentifier("automation.delete")
                }
            }
        }
        .disabled(busy)
        .navigationTitle(existing == nil ? L("新建自动化", "New automation") : L("编辑自动化", "Edit automation"))
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button(L("取消", "Cancel")) { dismiss() }.disabled(busy)
            }
            ToolbarItem(placement: .confirmationAction) {
                Button(existing == nil ? L("创建", "Create") : L("保存", "Save")) { save() }
                    .fontWeight(.semibold)
                    .disabled(busy || draft.missing != nil)
                    .accessibilityIdentifier("automation.save")
            }
        }
        // What is still missing, above the keyboard and the home bar.
        .safeAreaInset(edge: .bottom, spacing: 0) {
            if let missing = draft.missing, loaded {
                Text(missing).font(.footnote).foregroundStyle(MycoColor.muted)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 8)
                    .background(.bar)
                    .accessibilityIdentifier("automation.missing")
            }
        }
        .confirmationDialog(L("删除这个自动化？", "Delete this automation?"), isPresented: $deleting, titleVisibility: .visible) {
            Button(L("删除", "Delete"), role: .destructive) { remove() }
            Button(L("取消", "Cancel"), role: .cancel) {}
        } message: {
            Text(AutomationsScreen.deleteNote)
        }
    }

    /// How often, which day or how many hours, and at what time; then the
    /// whole of it in words.
    @ViewBuilder private var schedule: some View {
        Picker(L("多久一次", "How often"), selection: $draft.mode) {
            Text(L("每天", "Daily")).tag(AutomationDraft.Mode.daily)
            Text(L("工作日", "Weekdays")).tag(AutomationDraft.Mode.weekdays)
            Text(L("每周", "Weekly")).tag(AutomationDraft.Mode.weekly)
            Text(L("每隔几小时", "Every few hours")).tag(AutomationDraft.Mode.hours)
            Text(L("自定义（cron）", "Custom (cron)")).tag(AutomationDraft.Mode.cron)
        }
        .accessibilityIdentifier("automation.mode")
        if draft.mode == .weekly {
            Picker(L("星期几", "Which day"), selection: $draft.day) {
                ForEach(Every.weekChips, id: \.day) { chip in Text(chip.label).tag(chip.day) }
            }
            .pickerStyle(.segmented)
        }
        if draft.mode == .hours {
            Picker(L("隔几小时", "How many hours"), selection: $draft.hours) {
                ForEach([1, 2, 3, 4, 6, 8, 12], id: \.self) { count in Text("\(count)").tag(count) }
            }
            .pickerStyle(.segmented)
        }
        if draft.mode == .cron {
            TextField("0 9 * * 1-5", text: $draft.cron)
                .font(.body.monospaced())
                .textInputAutocapitalization(.never).autocorrectionDisabled()
                .keyboardType(.numbersAndPunctuation)
                .accessibilityIdentifier("automation.cron")
        } else if draft.mode == .hours {
            Picker(L("在每个整点后第几分", "At minute"), selection: $draft.minute) {
                ForEach(Self.minutes(with: draft.minute), id: \.self) { minute in Text(String(format: "%02d", minute)).tag(minute) }
            }
        } else {
            DatePicker(L("几点", "At"), selection: time, displayedComponents: .hourAndMinute)
                .accessibilityIdentifier("automation.time")
        }
        VStack(alignment: .leading, spacing: 2) {
            Text(draft.every.words).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg)
            Text([
                L("\(draft.zone) 时间", "\(draft.zone) time"),
                draft.next(after: Date()).map { L("下次 \(Every.soon($0, now: Date()))", "next \(Every.soon($0, now: Date()))") } ?? "",
            ].filter { !$0.isEmpty }.joined(separator: " · "))
                .font(.footnote).foregroundStyle(MycoColor.muted)
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("automation.summary")
    }

    static func minutes(with chosen: Int) -> [Int] {
        Array(Set(stride(from: 0, to: 60, by: 5)).union([chosen])).sorted()
    }

    /// The hour and minute as a time of day, for the system's picker.
    private var time: SwiftUI.Binding<Date> {
        SwiftUI.Binding(
            get: { Calendar.current.date(bySettingHour: draft.hour, minute: draft.minute, second: 0, of: Date()) ?? Date() },
            set: { picked in
                let parts = Calendar.current.dateComponents([.hour, .minute], from: picked)
                draft.hour = parts.hour ?? 9
                draft.minute = parts.minute ?? 0
            })
    }

    private func save() {
        guard draft.missing == nil, !busy else { return }
        busy = true
        failure = nil
        Task {
            defer { busy = false }
            do {
                let saved = try await session.automations.save(draft, changing: existing)
                if let value = saved.webhookSecret, !value.isEmpty {
                    secret = Secret(url: saved.automation.webhookUrl ?? "", value: value)
                } else {
                    dismiss()
                }
            } catch {
                failure = Failure.of(error)
            }
        }
    }

    private func rotate(_ automation: Automation) {
        busy = true
        failure = nil
        Task {
            defer { busy = false }
            do {
                let saved = try await session.automations.newSecret(of: automation)
                if let value = saved.webhookSecret, !value.isEmpty { secret = Secret(url: saved.automation.webhookUrl ?? "", value: value) }
            } catch {
                failure = Failure.of(error)
            }
        }
    }

    private func remove() {
        guard let existing else { return }
        busy = true
        Task {
            defer { busy = false }
            if await session.automations.remove(existing) {
                dismiss()
            } else {
                failure = session.automations.failure
            }
        }
    }
}

/// A webhook's address and secret, right after it was made: the secret is
/// shown this once.
private struct WebhookSecretPage: View {
    let secret: AutomationEditor.Secret
    let onDone: () -> Void

    var body: some View {
        Form {
            Section {
                copyable(L("地址", "Address"), secret.url, id: "automation.secret.url")
                copyable(L("密钥", "Secret"), secret.value, id: "automation.secret.value")
                copyable(L("试一下", "Try it"), AutomationWords.curl(url: secret.url, secret: secret.value), id: "automation.secret.curl")
            } header: {
                Text(L("密钥只显示这一次，请现在复制保存。外部系统带着它 POST 这个地址，就会触发一次；请求内容会替换消息里的 {{payload}}。",
                       "The secret is shown only now: copy it and keep it. An outside system POSTs to the address with it to run this once; what it sends replaces {{payload}} in the message."))
                    .textCase(nil)
                    .font(.footnote)
                    .padding(.bottom, 6)
            }
        }
        .navigationTitle(L("Webhook 地址和密钥", "Webhook address and secret"))
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button(L("好了", "Done"), action: onDone).fontWeight(.semibold)
                    .accessibilityIdentifier("automation.secret.done")
            }
        }
        .accessibilityIdentifier("automation.secret")
    }

    private func copyable(_ title: String, _ value: String, id: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title).font(.footnote).foregroundStyle(MycoColor.muted)
            RawReason(value, copyLabel: L("复制", "Copy"))
        }
        .accessibilityIdentifier(id)
    }
}
