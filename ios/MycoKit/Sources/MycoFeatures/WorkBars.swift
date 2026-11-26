import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

/// Above a chat, on a surface of its own (the desktop's bars): the goal the
/// agent works on and what it waits for in the background, each with what
/// can be done about it; and the line one of them left when it ended.
struct WorkBars: View {
    let model: WorkStateModel
    @State private var goalOpen = false
    @State private var watchesOpen = false
    @State private var stopping: Watch?
    @State private var stoppingGoal = false

    var body: some View {
        let ended = latestEnded
        if model.goal != nil || !model.watches.isEmpty || ended != nil {
            TimelineView(.periodic(from: .now, by: 1)) { context in
                VStack(alignment: .leading, spacing: 0) {
                    if let goal = model.goal { goalBar(goal, now: context.date) }
                    if !model.watches.isEmpty {
                        if model.goal != nil { Divider().overlay(MycoColor.line) }
                        watchBars(now: context.date)
                    }
                    if let ended {
                        if model.goal != nil || !model.watches.isEmpty { Divider().overlay(MycoColor.line) }
                        Text(ended).font(.caption).foregroundStyle(MycoColor.muted).lineLimit(1)
                            .padding(.vertical, 8)
                            .accessibilityIdentifier("work.ended")
                    }
                }
                .padding(.horizontal, Space.page)
            }
            .background(MycoColor.card)
            .overlay(alignment: .bottom) { Rectangle().fill(MycoColor.line).frame(height: 0.5) }
            .shadow(color: .black.opacity(0.04), radius: 4, y: 2)
            .confirmationDialog(L("停止盯这件事？", "Stop waiting for this?"), isPresented: SwiftUI.Binding(get: { stopping != nil }, set: { if !$0 { stopping = nil } }), titleVisibility: .visible) {
                Button(L("停止", "Stop"), role: .destructive) {
                    if let watch = stopping { Task { await model.stop(watch) } }
                }
            } message: {
                Text(stopping?.what ?? "")
            }
            .confirmationDialog(L("停止这个目标？", "Stop this goal?"), isPresented: $stoppingGoal, titleVisibility: .visible) {
                Button(L("停止", "Stop"), role: .destructive) { Task { await model.control("stop") } }
            } message: {
                Text(L("它不会再被唤醒去做这件事。", "It will not be woken for this again."))
            }
        }
    }

    /// The newest that ended in the last hour, as a grey line.
    private var latestEnded: String? {
        let hour = Date().addingTimeInterval(-3_600)
        let watch = model.endedWatches.filter { ($0.endedAt ?? .distantPast) > hour }.max { ($0.endedAt ?? .distantPast) < ($1.endedAt ?? .distantPast) }
        let goal = model.endedGoals.filter { ($0.endedAt ?? .distantPast) > hour }.max { ($0.endedAt ?? .distantPast) < ($1.endedAt ?? .distantPast) }
        switch (watch, goal) {
        case (let w?, let g?): return (w.endedAt ?? .distantPast) > (g.endedAt ?? .distantPast) ? WorkStateModel.watchEnded(w) : WorkStateModel.goalEnded(g)
        case (let w?, nil): return WorkStateModel.watchEnded(w)
        case (nil, let g?): return WorkStateModel.goalEnded(g)
        default: return nil
        }
    }

    // MARK: Goal

    @ViewBuilder private func goalBar(_ goal: Goal, now: Date) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .top, spacing: 8) {
                Image(systemName: "target").font(.subheadline).foregroundStyle(goal.state == "paused" ? MycoColor.amber : MycoColor.accent)
                    .padding(.top, 1)
                Button {
                    withAnimation(.easeOut(duration: 0.18)) { goalOpen.toggle() }
                } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(L("目标：", "Goal: ") + goal.objective).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg).lineLimit(goalOpen ? nil : 1)
                        Text(WorkStateModel.goalLine(goal, now: now)).font(.caption.monospacedDigit()).foregroundStyle(MycoColor.muted).lineLimit(1)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("work.goal")
                Menu {
                    if goal.state == "paused" {
                        Button(L("继续", "Resume"), systemImage: "play") { Task { await model.control("resume") } }
                    } else {
                        Button(L("暂停", "Pause"), systemImage: "pause") { Task { await model.control("pause") } }
                    }
                    Menu(L("改间隔", "Change how often")) {
                        ForEach([900, 1_800, 3_600], id: \.self) { seconds in
                            Button(WorkStateModel.span(TimeInterval(seconds))) { Task { await model.setInterval(seconds) } }
                        }
                    }
                    Button(L("停止", "Stop"), systemImage: "stop", role: .destructive) { stoppingGoal = true }
                } label: {
                    Image(systemName: "ellipsis").font(.subheadline).foregroundStyle(MycoColor.muted).frame(width: 32, height: 28)
                }
                .accessibilityLabel(L("目标操作", "Goal actions"))
                .accessibilityIdentifier("work.goal.more")
            }
            if let note = WorkStateModel.goalNote(goal) {
                Text(note).font(.caption).foregroundStyle(goal.state == "paused" ? MycoColor.amber : MycoColor.muted)
                    .padding(.leading, 24)
            }
            if goalOpen { goalDetail(goal) }
        }
        .padding(.vertical, 8)
    }

    private func goalDetail(_ goal: Goal) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            detail(L("算完成：", "Done when: "), goal.doneWhen)
            if let summary = goal.lastSummary { detail(L("上一轮：", "Last round: "), summary) }
            if let next = goal.nextStep { detail(L("下一步：", "Next: "), next) }
            ForEach(model.rounds.reversed(), id: \.round) { round in
                HStack(alignment: .firstTextBaseline, spacing: 6) {
                    Text("\(round.round)").font(.caption.monospacedDigit()).foregroundStyle(MycoColor.faint).frame(width: 18, alignment: .trailing)
                    Image(systemName: round.blocked == true ? "exclamationmark.circle" : round.progressed == true ? "checkmark.circle" : "minus.circle")
                        .font(.caption).foregroundStyle(round.progressed == true ? MycoColor.green : MycoColor.muted)
                    Text(round.summary ?? "").font(.caption).foregroundStyle(MycoColor.fg2)
                }
            }
        }
        .padding(.leading, 24)
        .accessibilityIdentifier("work.goal.detail")
    }

    private func detail(_ label: String, _ text: String) -> some View {
        (Text(label).foregroundStyle(MycoColor.muted) + Text(text).foregroundStyle(MycoColor.fg2)).font(.caption)
    }

    // MARK: Watches

    @ViewBuilder private func watchBars(now: Date) -> some View {
        if model.watches.count == 1, let watch = model.watches.first {
            watchRow(watch, now: now, title: L("正在盯：", "Waiting for: ") + watch.what)
        } else {
            VStack(alignment: .leading, spacing: 0) {
                Button {
                    withAnimation(.easeOut(duration: 0.18)) { watchesOpen.toggle() }
                } label: {
                    HStack(spacing: 8) {
                        Image(systemName: "eye").font(.subheadline).foregroundStyle(MycoColor.accent)
                        Text(L("正在盯 \(model.watches.count) 件事", "Waiting for \(model.watches.count) things")).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg)
                        Spacer(minLength: 0)
                        Image(systemName: watchesOpen ? "chevron.up" : "chevron.down").font(.caption.weight(.semibold)).foregroundStyle(MycoColor.faint)
                    }
                    .padding(.vertical, 10)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("work.watches")
                if watchesOpen {
                    ForEach(model.watches) { watch in watchRow(watch, now: now, title: watch.what) }
                }
            }
        }
    }

    private func watchRow(_ watch: Watch, now: Date, title: String) -> some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: "eye").font(.subheadline).foregroundStyle(MycoColor.accent).padding(.top, 1)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg).lineLimit(1)
                    .accessibilityIdentifier("work.watch")
                Text(WorkStateModel.watchLine(watch, now: now)).font(.caption.monospacedDigit()).foregroundStyle(MycoColor.muted).lineLimit(1)
                if let result = watch.lastResult, !result.isEmpty {
                    Text(result).font(.caption).foregroundStyle(MycoColor.fg2).lineLimit(1)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Menu {
                Menu(L("延长", "Give it more time")) {
                    Button(L("30 分钟", "30 minutes")) { Task { await model.extend(watch, seconds: 1_800) } }
                    Button(L("1 小时", "1 hour")) { Task { await model.extend(watch, seconds: 3_600) } }
                    Button(L("2 小时", "2 hours")) { Task { await model.extend(watch, seconds: 7_200) } }
                }
                Button(L("停止", "Stop"), systemImage: "stop", role: .destructive) { stopping = watch }
            } label: {
                Image(systemName: "ellipsis").font(.subheadline).foregroundStyle(MycoColor.muted).frame(width: 32, height: 28)
            }
            .accessibilityLabel(L("盯着的操作", "Wait actions"))
            .accessibilityIdentifier("work.watch.more")
        }
        .padding(.vertical, 8)
    }
}

/// Beside the send button: the account's tightest window ("5h 剩 38%"),
/// grey, amber under 20% left, red under 5%; a tap shows every window.
struct QuotaPill: View {
    let quota: Quota

    var body: some View {
        if QuotaWords.hasNumbers(quota), let worst = QuotaWords.worst(quota) {
            let level = QuotaWords.level(worst)
            Menu {
                Section(QuotaWords.source(quota)) {
                    ForEach(quota.windows, id: \.self) { window in
                        Text(QuotaWords.label(window) + "：" + QuotaWords.row(window))
                    }
                }
                if let at = quota.at {
                    Text(L("\(RelativeTime.short(at))取到的数据", "Read \(RelativeTime.short(at))"))
                }
            } label: {
                Text(QuotaWords.pill(worst))
                    .font(.caption.weight(.medium).monospacedDigit())
                    .foregroundStyle(level == .out ? MycoColor.red : level == .low ? MycoColor.amber : MycoColor.muted)
                    .padding(.horizontal, 8).frame(minHeight: 28)
                    .background(level == .ok ? MycoColor.pill : (level == .out ? MycoColor.redSoft : MycoColor.amberSoft), in: Capsule())
            }
            .accessibilityLabel(L("额度：", "Usage: ") + QuotaWords.pill(worst))
            .accessibilityIdentifier("composer.quota")
        }
    }
}

/// An account's usage limits on a member's card: whose, then each window.
struct QuotaSection: View {
    let quota: Quota

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(L("来源：", "Source: ") + QuotaWords.source(quota)).font(.footnote).foregroundStyle(MycoColor.muted)
            if QuotaWords.hasNumbers(quota) {
                ForEach(quota.windows, id: \.self) { window in
                    HStack {
                        Text(QuotaWords.label(window)).font(.subheadline)
                        Spacer()
                        Text(QuotaWords.row(window)).font(.subheadline.monospacedDigit())
                            .foregroundStyle(QuotaWords.level(window) == .out ? MycoColor.red : QuotaWords.level(window) == .low ? MycoColor.amber : MycoColor.fg2)
                    }
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("member.quota.window")
                }
            }
            if let status = QuotaWords.status(quota) {
                Text(status).font(.footnote).foregroundStyle(MycoColor.amber)
            }
        }
    }
}

/// Setting a goal on an agent: what, when it counts as done, how often it
/// is woken and for how long (the desktop's 设定目标…).
struct GoalSheet: View {
    let agent: String
    var project: String?
    @Environment(WorkspaceSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var objective = ""
    @State private var doneWhen = ""
    @State private var every = 1_800
    @State private var rounds = 16
    @State private var hours = 8
    @State private var problem: String?

    var body: some View {
        let model = session.workState(of: agent)
        NavigationStack {
            Form {
                Section {
                    TextField(L("比如：把推送接口做完并合进 main", "For example: finish the push API and merge it"), text: $objective, axis: .vertical)
                        .lineLimit(2...4)
                        .accessibilityIdentifier("goal.objective")
                } header: { Text(L("目标", "Goal")) }
                Section {
                    TextField(L("比如：有测试、CI 绿、PR 合并", "For example: tests, CI green, PR merged"), text: $doneWhen, axis: .vertical)
                        .lineLimit(2...4)
                        .accessibilityIdentifier("goal.done")
                } header: { Text(L("怎样算完成", "When it counts as done")) }
                Section {
                    Picker(L("多久唤醒一次", "How often"), selection: $every) {
                        Text(L("15 分钟", "15 minutes")).tag(900)
                        Text(L("30 分钟", "30 minutes")).tag(1_800)
                        Text(L("1 小时", "1 hour")).tag(3_600)
                    }
                    Stepper(L("最多 \(rounds) 轮", "At most \(rounds) rounds"), value: $rounds, in: 1...64)
                    Stepper(L("最长 \(hours) 小时", "At most \(hours) hours"), value: $hours, in: 1...8)
                } footer: {
                    Text(L("到时间它会被唤醒，接着做这件事，直到完成、轮数或时间用完。几轮没有进展会自动拉长间隔，卡住会暂停并告诉你。",
                           "It is woken every so often to go on with it, until it is done or the rounds or time run out. Rounds without progress space the wakes out; when it is stuck it pauses and tells you."))
                }
                if let problem {
                    Section { Text(problem).foregroundStyle(MycoColor.red).font(.footnote) }
                }
            }
            .navigationTitle(L("设定目标", "Set a goal"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button(L("取消", "Cancel")) { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("开始", "Start")) {
                        Task {
                            if await model.start(objective: objective, doneWhen: doneWhen, every: every, rounds: rounds, hours: hours, project: project) {
                                dismiss()
                            } else {
                                problem = model.failure?.words ?? L("没设上，再试一次。", "It was not set; try again.")
                            }
                        }
                    }
                    .disabled(objective.trimmingCharacters(in: .whitespaces).isEmpty || doneWhen.trimmingCharacters(in: .whitespaces).isEmpty || model.busy != nil)
                    .accessibilityIdentifier("goal.start")
                }
            }
        }
    }
}

/// The plan pinned above the chat: how far along it is and the next thing
/// left ("下一步：…"); a tap shows all of it; closed, it stays closed until
/// a new plan comes.
struct PlanBar: View {
    let items: [PlanItem]
    let close: () -> Void
    @State private var open = false

    var body: some View {
        let done = items.filter { $0.state == .done }.count
        let next = items.first { $0.state == .doing } ?? items.first { $0.state != .done }
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .top, spacing: 8) {
                Image(systemName: "checklist").font(.subheadline).foregroundStyle(MycoColor.accent).padding(.top, 1)
                Button {
                    withAnimation(.easeOut(duration: 0.18)) { open.toggle() }
                } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(L("下一步：", "Next: ") + (next?.text ?? "")).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg).lineLimit(open ? nil : 1)
                        Text(L("计划 \(done)/\(items.count) 完成", "Plan: \(done) of \(items.count) done")).font(.caption.monospacedDigit()).foregroundStyle(MycoColor.muted)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("work.plan")
                Button(action: close) {
                    Image(systemName: "xmark").font(.caption.weight(.semibold)).foregroundStyle(MycoColor.faint).frame(width: 32, height: 28)
                }
                .accessibilityLabel(L("收起计划", "Close the plan"))
                .accessibilityIdentifier("work.plan.close")
            }
            if open {
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                        HStack(alignment: .firstTextBaseline, spacing: 6) {
                            Image(systemName: item.state == .done ? "checkmark.square.fill" : "square")
                                .font(.caption).foregroundStyle(item.state == .done ? MycoColor.muted : (item.state == .doing ? MycoColor.fg : MycoColor.faint))
                            Text(item.text).font(.caption).foregroundStyle(item.state == .done ? MycoColor.muted : MycoColor.fg2)
                                .strikethrough(item.state == .done)
                        }
                    }
                }
                .padding(.leading, 24)
            }
        }
        .padding(.vertical, 8)
        .padding(.horizontal, Space.page)
        .background(MycoColor.card)
        .overlay(alignment: .bottom) { Rectangle().fill(MycoColor.line).frame(height: 0.5) }
    }
}
