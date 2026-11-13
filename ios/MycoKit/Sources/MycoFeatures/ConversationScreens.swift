import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

/// An agent's conversation, as its own page.
struct AgentScreen: View {
    let name: String
    /// A task to land on (from a capsule on a project's page).
    var focus: String?
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        ConversationHost(agent: name, project: nil, focus: focus)
            .navigationBarTitleDisplayMode(.inline)
            .solidTopBar()
            .toolbar {
                ToolbarItem(placement: .principal) {
                    Button {
                        router.sheet = .memberCard(name)
                    } label: {
                        VStack(spacing: 1) {
                            Text(session.displayName(of: name)).font(.headline).foregroundStyle(MycoColor.fg)
                            if let status = session.status(of: name) {
                                Text(Words.member(status)).font(.caption).foregroundStyle(MycoColor.muted).lineLimit(1)
                            }
                        }
                    }
                    .accessibilityIdentifier("conversation.title")
                }
            }
    }
}

/// A project: the conversation with its coordinator, and above it a strip
/// of the members with what each is up to.
struct ProjectScreen: View {
    let name: String
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        Group {
            if let project = session.project(name) {
                if let lead = session.coordinator(of: project) {
                    VStack(spacing: 0) {
                        MemberStrip(project: project, lead: lead)
                        ConversationHost(agent: lead, project: name)
                    }
                } else {
                    EmptyState(
                        face: .mushroom(MushroomLook(seed: name)), title: L("项目建好了，拉几个 agent 进来吧", "The project is ready. Bring some agents in."),
                        buttonTitle: L("添加 agent", "Add an agent")) { router.sheet = .addAgent }
                }
            } else if session.loadedAt != nil {
                EmptyState(symbol: "questionmark.folder", title: L("找不到这个项目", "This project is gone"))
            }
        }
        .background(MycoColor.bg)
        .navigationBarTitleDisplayMode(.inline)
        .solidTopBar()
        .toolbar {
            ToolbarItem(placement: .principal) {
                VStack(spacing: 1) {
                    let project = session.project(name)
                    Text(project.map { $0.title.isEmpty ? $0.name : $0.title } ?? name).font(.headline).foregroundStyle(MycoColor.fg)
                    if let goal = project?.goal, !goal.isEmpty {
                        Text(goal).font(.caption).foregroundStyle(MycoColor.muted).lineLimit(1)
                    }
                }
            }
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button(L("项目资料", "About this project"), systemImage: "doc.text") { router.push(.projectInfo(name)) }
                } label: {
                    Image(systemName: "ellipsis")
                }
                .accessibilityIdentifier("project.more")
            }
        }
    }
}

/// A project's members in a row, the coordinator first: tap one to see
/// its work.
private struct MemberStrip: View {
    let project: Project
    let lead: String
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(alignment: .top, spacing: 4) {
                ForEach(session.memberStatuses(of: project)) { status in
                    Button {
                        router.push(.agent(status.agent.name, focus: nil))
                    } label: {
                        VStack(spacing: 4) {
                            AvatarView(.of(status.agent), size: 40, status: StatusTone.of(member: status), away: status.state == "offline")
                            Text(status.agent.name == lead ? L("协调者", "Coordinator") : session.displayName(of: status.agent.name))
                                .font(.caption2)
                                .foregroundStyle(MycoColor.muted)
                                .lineLimit(1)
                        }
                        .frame(width: 64)
                    }
                    .buttonStyle(PressScaleStyle())
                    .accessibilityIdentifier("strip." + status.agent.name)
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
        }
        .background(MycoColor.bg)
    }
}

/// Shows one conversation. Its model lives with the workspace, so coming
/// back to a conversation finds it as it was.
struct ConversationHost: View {
    let agent: String
    let project: String?
    var focus: String?
    @Environment(WorkspaceSession.self) private var session

    var body: some View {
        let model = session.conversation(with: agent, project: project)
        ZStack {
            MycoColor.bg.ignoresSafeArea()
            ConversationBody(model: model, focus: focus)
        }
        // Starts once the workspace has said who this device is.
        .task(id: session.me) { await model.run() }
        // On screen, and whenever something new arrives while it is: read.
        .onChange(of: model.items.last?.id, initial: true) { session.markRead(agent) }
        .onDisappear { session.markRead(agent) }
    }
}

/// What a conversation looks like: what was said, where the open task
/// stands, and the input. It draws the model and passes taps on.
struct ConversationBody: View {
    @Bindable var model: ConversationModel
    /// A task to land on once the conversation has loaded.
    var focus: String?
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @State private var attachNotice = false
    @State private var landed = false
    /// The end of the conversation is on screen.
    @State private var atBottom = true
    /// It was when the keyboard started to come up: keep it there.
    @State private var followKeyboard = false
    /// The conversation stays at its end as it grows. Only the person
    /// scrolling away lets go of it; content getting taller does not.
    @State private var pinned = true
    /// A finger moved the conversation a moment ago.
    @State private var touched = false
    /// How tall the part of the conversation on screen is.
    @State private var viewport: CGFloat = 800

    private static let space = "conversation"
    /// Counts what was sent or stopped from here: a light tap answers each.
    @State private var commits = 0
    /// The conversation has opened where it should and settled: from now
    /// on, reaching its top asks for older messages.
    @State private var settled = false

    private static let bottomID = "conversation.bottom"

    private var name: String { session.displayName(of: model.agent) }

    /// The newest thing I said: saying something new brings it into view.
    private var newestMine: String? {
        model.items.last { if case .mine(_, _, nil) = $0.kind { return true } else { return false } }?.id
    }

    /// The newest answer: the one that carries the row of actions.
    private var newestReply: String? {
        model.items.last { if case .reply = $0.kind { return true } else { return false } }?.id
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                // Not lazy, on purpose. A conversation comes a page at a
                // time, so the rows are few enough to lay out all at once,
                // and then every one has its real height: where the page
                // is scrolled to is exact, which a lazy stack's estimates
                // (with rows as tall as an agent's run) could not promise.
                VStack(alignment: .leading, spacing: 0) {
                    // Older messages exist: coming near this row loads them.
                    if model.hasEarlier {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                            .padding(.bottom, Space.l)
                            .onGeometryChange(for: Bool.self) { proxy in
                                proxy.frame(in: .named(Self.space)).maxY > -240
                            } action: { near in
                                if near { loadEarlier(proxy) }
                            }
                            .accessibilityIdentifier("conversation.earlier")
                    }
                    ForEach(Array(model.items.enumerated()), id: \.element.id) { index, item in
                        row(item, at: index)
                            .padding(.top, index == 0 ? 0 : gap(from: model.items[index - 1], to: item))
                    }
                    // Marks the end. It is also the room between the last
                    // thing said and the input, so "the end" includes it.
                    // On screen (or nearly) means "at the bottom".
                    Color.clear.frame(height: Space.xl)
                        .id(Self.bottomID)
                        .onGeometryChange(for: Bool.self) { proxy in
                            proxy.frame(in: .named(Self.space)).minY <= viewport + 40
                        } action: { there in
                            atBottom = there
                            if there {
                                pinned = true
                            } else if touched {
                                // Gone because a finger scrolled away: let
                                // go. Gone because the page grew: pinned.
                                pinned = false
                            }
                        }
                }
                .padding(.horizontal, Space.page)
                .padding(.top, Space.l)
            }
            .coordinateSpace(.named(Self.space))
            .onGeometryChange(for: CGFloat.self) { proxy in
                proxy.size.height
            } action: { height in
                viewport = height
            }
            .simultaneousGesture(
                DragGesture(minimumDistance: 8)
                    .onChanged { _ in touched = true }
                    .onEnded { _ in
                        // The glide after a flick still counts as the person's.
                        Task {
                            try? await Task.sleep(for: .seconds(1.2))
                            touched = false
                        }
                    })
            // Opens at the bottom, and stays there as content grows while
            // the newest message is in view.
            .defaultScrollAnchor(.bottom)
            .scrollDismissesKeyboard(.interactively)
            .modifier(SolidTopEdge())
            // A tap anywhere on the conversation puts the keyboard away.
            .simultaneousGesture(TapGesture().onEnded { Self.dismissKeyboard() })
            // Away from the end: one tap goes back to it.
            .overlay(alignment: .bottom) {
                if !atBottom, !model.items.isEmpty {
                    ScrollToEndButton(label: L("回到底部", "Back to the newest")) {
                        pinned = true
                        withAnimation(.snappy(duration: 0.3)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
                    }
                    .padding(.bottom, Space.m)
                    .transition(.scale(scale: 0.8).combined(with: .opacity))
                }
            }
            .animation(.snappy(duration: 0.2), value: atBottom)
            // What the page shows changed (an answer came, a task's steps
            // loaded, a colleague's exchange was merged in): a pinned
            // conversation goes to its end again. A few times, a moment
            // apart, as rows take their real heights; never in answer to
            // the layout itself, which would chase its own tail.
            .onChange(of: model.revision) {
                guard landed, pinned else { return }
                Task {
                    for pause in [0, 80, 300] {
                        try? await Task.sleep(for: .milliseconds(pause))
                        guard pinned else { return }
                        proxy.scrollTo(Self.bottomID, anchor: .bottom)
                    }
                }
            }
            .onChange(of: newestMine) { old, new in
                // What I just said comes into view even if I had scrolled up.
                guard old != nil, new != nil else { return }
                pinned = true
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
            }
            // The keyboard takes the lower part of the screen. A
            // conversation that was at its end stays at its end; one being
            // read further up stays where it is.
            .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillShowNotification)) { _ in
                followKeyboard = pinned || atBottom
                guard followKeyboard else { return }
                withAnimation(.easeOut(duration: 0.25)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
            }
            .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardDidShowNotification)) { _ in
                if followKeyboard { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
            }
            .onChange(of: model.loaded, initial: true) { _, loaded in
                // Where a conversation opens: on the task it was opened
                // for, else at its end. The bottom anchor alone does not get
                // a long, lazily laid out conversation there, so it is
                // sent there, and once more when the first rows have their
                // real heights.
                guard loaded, !landed else { return }
                landed = true
                let target = focus.flatMap { focus in model.items.first { $0.id == focus }?.id }
                if let target {
                    proxy.scrollTo(target, anchor: .center)
                    settled = true
                    return
                }
                proxy.scrollTo(Self.bottomID, anchor: .bottom)
                Task {
                    try? await Task.sleep(for: .milliseconds(60))
                    proxy.scrollTo(Self.bottomID, anchor: .bottom)
                    try? await Task.sleep(for: .milliseconds(400))
                    settled = true
                }
            }
        }
        .overlay {
            if model.loaded, model.items.isEmpty {
                EmptyState(
                    face: session.agent(model.agent).map(AvatarView.Face.of),
                    title: model.project == nil
                        ? L("想让 \(name) 帮你做点什么？", "What should \(name) do for you?")
                        : L("你想在这个项目里做成什么？", "What do you want to get done here?"),
                    note: L("一句话就行，它会自己想办法。", "One sentence is enough. It will work out the rest."))
            }
        }
        .safeAreaInset(edge: .bottom, spacing: 0) { bottom }
        .alert(L("发文件还没做好", "Attachments are not built yet"), isPresented: $attachNotice) {
            Button(L("好", "OK"), role: .cancel) {}
        }
    }

    /// Loads the page of older messages, and keeps what was at the top of
    /// the screen where it was: the new rows go in above it.
    private func loadEarlier(_ proxy: ScrollViewProxy) {
        guard settled else { return }
        let anchor = model.items.first?.id
        Task {
            guard await model.loadEarlier(), let anchor else { return }
            proxy.scrollTo(anchor, anchor: .top)
        }
    }

    private static func dismissKeyboard() {
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    }

    /// How far a thing keeps from the one above it: close inside a turn,
    /// further between its parts, furthest where a new turn begins.
    private func gap(from above: ConversationItem, to item: ConversationItem) -> CGFloat {
        switch (above.kind, item.kind) {
        case (.time, _): return Space.m
        case (_, .time): return Space.xl
        case (.mine, .mine): return Space.s
        case (_, .mine): return Space.xl
        case (.dispatch, .dispatch): return Space.s
        case (.progress, .progress): return 0
        case (.process, .reply), (.pastProgress, .reply), (.progress, .reply): return Space.m
        case (.mine, .plan), (.plan, .process), (.plan, .progress): return Space.m
        default: return Space.l
        }
    }

    private func isProgress(_ index: Int) -> Bool {
        guard model.items.indices.contains(index), case .progress = model.items[index].kind else { return false }
        return true
    }

    /// Where an update sits in the run of updates around it: they are
    /// drawn as one timeline, joined by a line.
    private func timelinePosition(at index: Int) -> ProgressLine.Position {
        switch (isProgress(index - 1), isProgress(index + 1)) {
        case (false, false): return .only
        case (false, true): return .first
        case (true, true): return .middle
        case (true, false): return .last
        }
    }

    @ViewBuilder private func row(_ item: ConversationItem, at index: Int) -> some View {
        switch item.kind {
        case .time(let date):
            TimeBreak(Self.timeText(date))
        case .mine(let text, let steer, let by):
            // A colleague's task carries their name; mine needs none.
            MyBubble(text, label: steer ? L("补充", "Added") : nil, name: by.map { session.displayName(of: $0) })
                .contextMenu { copyButton(text) }
        case .reply(let text, let taskID, let to):
            VStack(alignment: .leading, spacing: Space.s) {
                if let to {
                    // An answer to a colleague says whom it answers.
                    Text(L("回复 \(session.displayName(of: to))", "To \(session.displayName(of: to))"))
                        .font(.footnote.weight(.medium)).foregroundStyle(MycoColor.muted)
                }
                ReplyText(text)
                // The newest answer carries what can be done with it; the
                // others keep the page quiet (a long press copies any).
                if item.id == newestReply {
                    let took = taskID.flatMap { model.process(of: $0)?.usage }
                    ReplyActions(
                        copying: text,
                        detail: [item.at.formatted(date: .omitted, time: .shortened), took].compactMap { $0 }.joined(separator: " · "),
                        copyLabel: L("复制", "Copy"))
                }
            }
            .contextMenu { copyButton(text) }
        case .progress(let text, let latest):
            ProgressLine(
                text, time: item.at.formatted(date: .omitted, time: .shortened),
                position: timelinePosition(at: index), latest: latest)
        case .plan(let plan, _):
            let done = plan.filter { $0.state == .done }.count
            PlanCard(
                title: L("计划", "Plan"), count: L("\(done) / \(plan.count) 完成", "\(done) of \(plan.count) done"),
                lines: plan.map { .init(text: $0.text, mark: $0.state == .done ? .done : ($0.state == .doing ? .doing : .todo)) })
        case .pastProgress(let texts):
            PastProgress(title: L("\(texts.count) 条进展", texts.count == 1 ? "1 update" : "\(texts.count) updates"), texts: texts)
        case .failure(let reason):
            NoticeLine(.failed, reason.isEmpty ? L("没做成", "Did not finish") : L("没做成：\(reason)", "Did not finish: \(reason)"))
        case .stopped:
            NoticeLine(.idle, L("已停止", "Stopped"))
        case .status(let state, _):
            status(state)
        case .process(let taskID, let live):
            if let process = model.process(of: taskID) { ProcessView(process: process, live: live) }
        case .dispatch(let dispatch):
            // Work the coordinator gave a member: one line, leading to
            // that member's page.
            let name = session.displayName(of: dispatch.to)
            let stateText = Words.state(dispatch.state)
            DispatchCapsule(
                face: session.agent(dispatch.to).map(AvatarView.Face.of) ?? .computer,
                name: name, text: dispatch.firstLine, stateText: stateText,
                tone: StatusTone.of(dispatch.state) ?? .idle,
                spoken: L("派给 \(name)：\(dispatch.firstLine)，\(stateText)", "To \(name): \(dispatch.firstLine), \(stateText)"),
                action: { router.push(.agent(dispatch.to, focus: dispatch.taskID)) })
        }
    }

    private func copyButton(_ text: String) -> some View {
        Button(L("复制", "Copy"), systemImage: "square.on.square") { UIPasteboard.general.string = text }
    }

    /// "14:32" today, "昨天 14:32", else "11月12日 14:32".
    private static func timeText(_ date: Date) -> String {
        let time = date.formatted(date: .omitted, time: .shortened)
        let calendar = Calendar.current
        if calendar.isDateInToday(date) { return time }
        if calendar.isDateInYesterday(date) { return L("昨天 \(time)", "Yesterday \(time)") }
        let locale = Locale(identifier: AppLanguage.current == .zh ? "zh-Hans" : "en")
        return date.formatted(.dateTime.month().day().locale(locale)) + " " + time
    }

    @ViewBuilder private func status(_ state: RunState) -> some View {
        let look = MushroomLook.of(session.agent(model.agent))
        switch state {
        case .queued:
            WorkingLine(look: look, text: L("排队中，轮到它时开始", "Waiting its turn"), since: nil)
        case .working(let since):
            // What it says it is doing; when it has not said, a playful
            // word that changes every few seconds, as on the desktop.
            let text = session.agent(model.agent)?.statusText
            TimelineView(.periodic(from: .now, by: 3.6)) { context in
                WorkingLine(
                    look: look, text: (text?.isEmpty == false ? text : nil) ?? RelativeTime.workingVerb(now: context.date),
                    since: since, elapsed: { RelativeTime.working(since: $0) })
            }
        case .maybeStuck(let quietSince):
            NoticeLine(
                .attention,
                L("\(RelativeTime.elapsed(since: quietSince))没有动静，可能卡住了", "Quiet for \(RelativeTime.elapsed(since: quietSince)). It may be stuck."),
                actions: [.init(L("停止", "Stop")) { Task { await model.submitStop() } }])
        case .done, .stopped, .failed:
            EmptyView()
        }
    }

    private var bottom: some View {
        VStack(spacing: 0) {
            if let problem = model.problem {
                Text(problem).font(.footnote).foregroundStyle(MycoColor.red)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, Space.page).padding(.top, Space.s)
            }
            // What waits behind the task being worked on: not part of the
            // conversation yet, shown here until its turn comes.
            ForEach(model.waiting) { item in
                HStack(spacing: 10) {
                    Text(L("排队中：", "Queued: ") + ConversationBuilder.firstLine(of: item.body))
                        .font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(1)
                    Spacer(minLength: 4)
                    Button(L("编辑", "Edit")) { Task { await model.withdraw(item, edit: true) } }
                    Button(L("撤回", "Withdraw")) { Task { await model.withdraw(item, edit: false) } }
                }
                .font(.footnote)
                .tint(MycoColor.accent)
                .padding(.horizontal, 14)
                .padding(.vertical, 9)
                .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                .padding(.horizontal, 14)
                .padding(.top, Space.s)
                .accessibilityIdentifier("queue.row")
            }
            ComposerView(
                text: $model.draft,
                placeholder: model.openTask == nil ? L("发给 \(name)", "Message \(name)") : L("补充给 \(name)", "Add to what \(name) is doing"),
                mode: mode, enabled: model.canSubmit, submitLabel: submitLabel,
                queueTitle: L("排到下一条", "Send after this one"),
                onSubmit: {
                    commits += 1
                    Task { await model.submit() }
                },
                onQueue: {
                    commits += 1
                    Task { await model.queueNext() }
                },
                onAttach: { attachNotice = true })
        }
        .background(.bar)
        .sensoryFeedback(.impact(weight: .light), trigger: commits)
    }

    private var submitLabel: String {
        switch model.action {
        case .send: return L("发送", "Send")
        case .steer: return L("补充", "Add")
        case .stop: return L("停止", "Stop")
        }
    }

    private var mode: ComposerView.Mode {
        switch model.action {
        case .send: return .send
        case .steer: return .steer
        case .stop: return .stop
        }
    }
}

/// How a task was worked on: the plan on top, then what the agent said,
/// thought and did, with each run of steps folded into one line.
struct ProcessView: View {
    let process: RunProcess
    /// Still being worked on: a step without a result is still going.
    let live: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            ForEach(process.segments) { segment in
                switch segment {
                case .said(_, let text):
                    ReplyText(text)
                case .thought(_, let text):
                    ThoughtText(text)
                case .steps(_, let steps):
                    StepGroupView(summary: RunProcess.summary(of: steps), steps: steps.map(line))
                case .failure(_, let text):
                    NoticeLine(.failed, text)
                }
            }
        }
    }

    private func line(_ step: ToolStep) -> StepLine {
        StepLine(
            id: step.id, symbol: symbol(for: step.tool), label: RunProcess.label(for: step.tool),
            detail: step.input, failed: step.failed, running: live && !step.finished)
    }

    private func symbol(for tool: String) -> String {
        switch tool {
        case "Bash": return "terminal"
        case "Read", "NotebookRead": return "doc.text"
        case "Edit", "Write", "MultiEdit", "NotebookEdit": return "pencil"
        case "Grep", "Glob", "LS": return "magnifyingglass"
        case "WebFetch", "WebSearch": return "globe"
        default: return "wrench.and.screwdriver"
        }
    }
}

/// Where text scrolls under the top bar, the bar stays solid enough to
/// read its title: a conversation's words do not show through it.
private struct SolidTopEdge: ViewModifier {
    func body(content: Content) -> some View {
        if #available(iOS 26.0, *) {
            content.scrollEdgeEffectStyle(.hard, for: .top)
        } else {
            content
        }
    }
}

extension View {
    /// A top bar with a background of its own on pages of running text: a
    /// conversation's words pass under it without showing through the title.
    func solidTopBar() -> some View {
        toolbarBackground(MycoColor.bg, for: .navigationBar)
            .toolbarBackground(.visible, for: .navigationBar)
    }
}
