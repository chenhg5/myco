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

/// Owns one conversation's model for as long as its page is on screen.
struct ConversationHost: View {
    let agent: String
    let project: String?
    var focus: String?
    @Environment(WorkspaceSession.self) private var session
    @State private var model: ConversationModel?

    var body: some View {
        ZStack {
            MycoColor.bg.ignoresSafeArea()
            if let model { ConversationBody(model: model, focus: focus) }
        }
        // Starts once the workspace has said who this device is.
        .task(id: session.me) {
            if model == nil { model = ConversationModel(session: session, agent: agent, project: project) }
            await model?.run()
        }
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

    private static let bottomID = "conversation.bottom"
    /// Counts what was sent or stopped from here: a light tap answers each.
    @State private var commits = 0

    private var name: String { session.displayName(of: model.agent) }

    /// The newest thing I said: saying something new brings it into view.
    private var newestMine: String? {
        model.items.last { if case .mine = $0.kind { return true } else { return false } }?.id
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 14) {
                    ForEach(model.items) { item in
                        row(item)
                    }
                    // Marks the end: on screen means "at the bottom".
                    Color.clear.frame(height: 1)
                        .id(Self.bottomID)
                        .onAppear { atBottom = true }
                        .onDisappear { atBottom = false }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 14)
            }
            // Opens at the bottom, and stays there as content grows while
            // the newest message is in view.
            .defaultScrollAnchor(.bottom)
            .scrollDismissesKeyboard(.interactively)
            .modifier(SolidTopEdge())
            .onChange(of: newestMine) { old, new in
                // What I just said comes into view even if I had scrolled up.
                guard old != nil, let new else { return }
                _ = new
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
            }
            // The keyboard takes the lower part of the screen. A
            // conversation that was at its end stays at its end; one being
            // read further up stays where it is.
            .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillShowNotification)) { _ in
                followKeyboard = atBottom
                guard followKeyboard else { return }
                withAnimation(.easeOut(duration: 0.25)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
            }
            .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardDidShowNotification)) { _ in
                if followKeyboard { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
            }
            .onChange(of: model.loaded, initial: true) { _, loaded in
                guard loaded, !landed, let focus else { return }
                landed = true
                let target = model.items.first { $0.id == focus || $0.id == focus + ".dispatch" }?.id
                if let target { proxy.scrollTo(target, anchor: .center) }
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

    @ViewBuilder private func row(_ item: ConversationItem) -> some View {
        switch item.kind {
        case .mine(let text, let steer):
            MyBubble(text, label: steer ? L("补充", "Added") : nil)
        case .reply(let text):
            ReplyText(text)
        case .progress(let text):
            ProgressLine(text)
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
            // Whose face: who gave it (on an agent's own page), or who it
            // was given to (on a project's page).
            let other = dispatch.incoming ? dispatch.from : dispatch.to
            let name = session.displayName(of: other)
            let stateText = Words.state(dispatch.state)
            DispatchCapsule(
                face: session.agent(other).map(AvatarView.Face.of) ?? .computer,
                name: name, text: dispatch.firstLine, stateText: stateText,
                tone: StatusTone.of(dispatch.state) ?? .idle,
                spoken: dispatch.incoming
                    ? L("\(name) 派的：\(dispatch.firstLine)，\(stateText)", "From \(name): \(dispatch.firstLine), \(stateText)")
                    : L("派给 \(name)：\(dispatch.firstLine)，\(stateText)", "To \(name): \(dispatch.firstLine), \(stateText)"),
                action: dispatch.incoming ? nil : { router.push(.agent(dispatch.to, focus: dispatch.taskID)) })
        }
    }

    @ViewBuilder private func status(_ state: RunState) -> some View {
        let look = MushroomLook.of(session.agent(model.agent))
        switch state {
        case .queued:
            WorkingLine(look: look, text: L("排队中，轮到它时开始", "Waiting its turn"), since: nil)
        case .working(let since):
            let text = session.agent(model.agent)?.statusText
            WorkingLine(look: look, text: (text?.isEmpty == false ? text : nil) ?? L("在干活", "Working"), since: since)
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
                    .padding(.horizontal, 16).padding(.top, 8)
            }
            ForEach(Array(model.queued.enumerated()), id: \.offset) { index, text in
                HStack(spacing: 10) {
                    Text(L("排队中：", "Queued: ") + ConversationBuilder.firstLine(of: text))
                        .font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(1)
                    Spacer(minLength: 4)
                    Button(L("编辑", "Edit")) { model.unqueue(at: index, edit: true) }
                    Button(L("撤回", "Remove")) { model.unqueue(at: index, edit: false) }
                }
                .font(.footnote)
                .tint(MycoColor.accent)
                .padding(.horizontal, 16)
                .padding(.top, 8)
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
                onQueue: { model.queueNext() },
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
        VStack(alignment: .leading, spacing: 12) {
            if let plan = process.plan {
                let done = plan.filter { $0.state == .done }.count
                PlanCard(
                    title: L("计划", "Plan"), count: L("\(done) / \(plan.count) 完成", "\(done) of \(plan.count) done"),
                    lines: plan.map { .init(text: $0.text, mark: $0.state == .done ? .done : ($0.state == .doing ? .doing : .todo)) })
            }
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
