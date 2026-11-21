import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

/// Chats: everything I talk to directly, most recent first.
struct ChatsScreen: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(AppModel.self) private var app
    @State private var query = ""

    var body: some View {
        // (my superagent is not one of the chats: it is the orb in the tab bar)
        let rows = session.chatRows.filter { $0.party != session.superagent.agent }
            .filter { query.isEmpty || $0.title.localizedCaseInsensitiveContains(query) }
        Group {
            if query.trimmingCharacters(in: .whitespaces).isEmpty {
                list(rows)
            } else {
                SearchResults(query: query, chats: rows)
            }
        }
        .workspaceChrome(title: L("聊天", "Chats"), add: true, members: true)
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .automatic), prompt: L("搜索聊天、成员、项目和消息", "Search chats, members, projects and messages"))
        // (the workspace is asked a moment after typing stops)
        .task(id: query) {
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            await session.search.search(query)
        }
    }

    private func list(_ rows: [ChatRow]) -> some View {
        List(rows) { row in
            Button {
                switch row.target {
                case .project(let name): router.push(.project(name))
                case .agent(let name): router.push(.agent(name, focus: nil))
                }
            } label: {
                PersonRow(
                    face: row.face.map(AvatarView.Face.of) ?? .computer,
                    title: row.title, subtitle: row.subtitle,
                    trailing: row.updatedAt.map { RelativeTime.short($0) },
                    tone: StatusTone.of(row.state), away: row.away, unread: row.unread,
                    muted: app.isMuted(row.party, in: session.id))
            }
            .buttonStyle(RowButtonStyle())
            .listRowInsets(EdgeInsets(top: 9, leading: 16, bottom: 9, trailing: 16))
            .listRowBackground(MycoColor.bg)
            .listRowSeparator(.hidden)
            .contextMenu {
                if case .agent(let name) = row.target {
                    Button(L("看资料", "About"), systemImage: "person.crop.circle") { router.sheet = .memberCard(name) }
                }
                // Nothing from this one is announced; its dot still shows.
                if app.isMuted(row.party, in: session.id) {
                    Button(L("恢复通知", "Unmute"), systemImage: "bell") { app.setMuted(false, row.party, in: session.id) }
                } else {
                    Button(L("免打扰", "Mute"), systemImage: "bell.slash") { app.setMuted(true, row.party, in: session.id) }
                }
                Button(L("复制名字", "Copy name"), systemImage: "doc.on.doc") { UIPasteboard.general.string = row.title }
            }
            .accessibilityIdentifier("chat." + row.id)
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .background(MycoColor.bg)
        .overlay {
            if session.loadedAt != nil, rows.isEmpty {
                if query.isEmpty {
                    EmptyState(
                        face: .mushroom(MushroomLook(seed: session.name)), title: L("加一个 agent，直接聊", "Add an agent and start talking"),
                        note: L("把一句话发给任何一个 AI 会话，它就会加入。", "Send one sentence to any AI session and it joins."),
                        buttonTitle: L("添加 agent", "Add an agent")) { router.sheet = .addAgent }
                } else {
                    EmptyState(symbol: "magnifyingglass", title: L("没有匹配的聊天", "No matching chats"))
                }
            }
        }
    }
}

/// What the chats page's search finds (the desktop app's ⌘K): the chats
/// whose names say it, members, projects, and the messages of my
/// conversations that say it, each opening where it was said.
private struct SearchResults: View {
    let query: String
    let chats: [ChatRow]
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        let search = session.search
        let names = Set(chats.compactMap { row -> String? in if case .agent(let name) = row.target { return name } else { return nil } })
        let members = search.members(query).filter { !names.contains($0.agent.name) }.prefix(6)
        let projects = search.projects(query).prefix(6)
        let found = search.query == query.trimmingCharacters(in: .whitespacesAndNewlines) ? search.messages : []
        List {
            if !chats.isEmpty {
                Section {
                    ForEach(chats.prefix(6)) { row in
                        Button {
                            switch row.target {
                            case .project(let name): router.push(.project(name))
                            case .agent(let name): router.push(.agent(name, focus: nil))
                            }
                        } label: {
                            PersonRow(face: row.face.map(AvatarView.Face.of) ?? .computer, title: row.title, subtitle: row.subtitle)
                        }
                        .buttonStyle(RowButtonStyle()).modifier(ResultRow())
                    }
                } header: { header(L("聊天", "Chats")) }
            }
            if !members.isEmpty {
                Section {
                    ForEach(members) { status in
                        Button {
                            router.push(.agent(status.agent.name, focus: nil))
                        } label: {
                            PersonRow(face: AvatarView.Face.of(status.agent), title: session.displayName(of: status.agent.name), subtitle: status.agent.description)
                        }
                        .buttonStyle(RowButtonStyle()).modifier(ResultRow())
                        .accessibilityIdentifier("search.member")
                    }
                } header: { header(L("成员", "Members")) }
            }
            if !projects.isEmpty {
                Section {
                    ForEach(projects) { project in
                        Button {
                            router.push(.project(project.name))
                        } label: {
                            PersonRow(face: .mushroom(MushroomLook(seed: project.name)), title: project.title.isEmpty ? project.name : project.title, subtitle: project.goal ?? "")
                        }
                        .buttonStyle(RowButtonStyle()).modifier(ResultRow())
                        .accessibilityIdentifier("search.project")
                    }
                } header: { header(L("项目", "Projects")) }
            }
            Section {
                if query.trimmingCharacters(in: .whitespaces).count < 2 {
                    Text(L("再多打一个字，就会搜消息", "One more letter and messages are searched too"))
                        .font(.footnote).foregroundStyle(MycoColor.muted).modifier(ResultRow())
                } else if search.searching && found.isEmpty {
                    HStack(spacing: 8) {
                        ProgressView().controlSize(.small)
                        Text(L("正在搜消息…", "Searching messages…")).font(.footnote).foregroundStyle(MycoColor.muted)
                    }
                    .modifier(ResultRow())
                } else if let failure = search.failure {
                    FailureLine(failure: failure, onRetry: { Task { await search.search(query) } }).modifier(ResultRow())
                } else if found.isEmpty {
                    Text(L("消息里没有搜到", "No messages say it")).font(.footnote).foregroundStyle(MycoColor.muted).modifier(ResultRow())
                }
                ForEach(found) { hit in
                    Button {
                        router.findOnOpen = (message: hit.id, query: query.trimmingCharacters(in: .whitespacesAndNewlines))
                        router.push(.agent(hit.party, focus: hit.id))
                    } label: {
                        HStack(alignment: .top, spacing: 12) {
                            AvatarView(session.agent(hit.party).map(AvatarView.Face.of) ?? .computer, size: 36)
                            VStack(alignment: .leading, spacing: 3) {
                                HStack {
                                    Text(session.displayName(of: hit.party)).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg)
                                    Spacer(minLength: 0)
                                    Text(RelativeTime.short(hit.at)).font(.caption).foregroundStyle(MycoColor.faint)
                                }
                                Highlighted(text: (hit.from == session.me ? L("我：", "Me: ") : "") + hit.snippet, query: query)
                                    .font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(3)
                            }
                        }
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(RowButtonStyle()).modifier(ResultRow())
                    .accessibilityIdentifier("search.message")
                }
            } header: { header(L("消息", "Messages")) }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .background(MycoColor.bg)
        .accessibilityIdentifier("search.results")
    }

    private func header(_ text: String) -> some View {
        Text(text).font(.footnote).foregroundStyle(MycoColor.muted)
    }
}

private struct ResultRow: ViewModifier {
    func body(content: Content) -> some View {
        content
            .listRowInsets(EdgeInsets(top: 8, leading: 16, bottom: 8, trailing: 16))
            .listRowBackground(MycoColor.bg)
            .listRowSeparator(.hidden)
    }
}

/// Text with every place it says what was looked for in bold, in the
/// accent colour.
struct Highlighted: View {
    let text: String
    let query: String

    var body: some View {
        Text(attributed)
    }

    private var attributed: AttributedString {
        var string = AttributedString(text)
        let wanted = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !wanted.isEmpty else { return string }
        var from = string.startIndex
        while let range = string[from...].range(of: wanted, options: [.caseInsensitive, .diacriticInsensitive]) {
            string[range].font = .footnote.weight(.semibold)
            string[range].foregroundColor = MycoColor.accent
            from = range.upperBound
        }
        return string
    }
}

/// Projects: each thing being done, and who is at work on it.
struct ProjectsScreen: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        List(session.projects) { project in
            let members = session.memberStatuses(of: project)
            let working = members.filter { $0.state == "working" }.count
            Button {
                router.push(.project(project.name))
            } label: {
                HStack(spacing: 12) {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(project.title.isEmpty ? project.name : project.title)
                            .font(.body.weight(.medium)).foregroundStyle(MycoColor.fg).lineLimit(1)
                        if let goal = project.goal, !goal.isEmpty {
                            Text(goal).font(.subheadline).foregroundStyle(MycoColor.muted).lineLimit(1)
                        }
                        HStack(spacing: 8) {
                            AvatarStack(members.map { AvatarView.Face.of($0.agent) }, size: 22)
                            if working > 0 {
                                StatusDot(.working, size: 6)
                                Text(L("\(working) 个在干活", "\(working) at work")).font(.footnote).foregroundStyle(MycoColor.muted)
                            }
                        }
                        .padding(.top, 2)
                    }
                    Spacer(minLength: 8)
                    Image(systemName: "chevron.right").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.faint)
                }
                .padding(.vertical, 6)
                .contentShape(Rectangle())
            }
            .buttonStyle(RowButtonStyle())
            .listRowInsets(EdgeInsets(top: 9, leading: 16, bottom: 9, trailing: 16))
            .listRowBackground(MycoColor.bg)
            .listRowSeparator(.hidden)
            .accessibilityIdentifier("project." + project.name)
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .background(MycoColor.bg)
        .overlay {
            if session.loadedAt != nil, session.projects.isEmpty {
                EmptyState(
                    symbol: "square.stack.3d.up", title: L("还没有项目", "No projects yet"),
                    note: L("要几个 agent 一起做成一件事时，在电脑上的 myco 里建一个项目。", "When several agents should get one thing done together, create a project in myco on your computer."))
            }
        }
        .workspaceChrome(title: L("项目", "Projects"), add: false)
    }
}

/// 工作台: what agents do with me besides talking: apps to use together,
/// automations that run on their own, workflows that hand work along (and
/// what waits for my decision). Members are on the chats page.
struct WorkbenchScreen: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        List {
            // What waits for me first: a decision is a tap here.
            let decisions = session.workflows.decisions
            if !decisions.isEmpty {
                Section {
                    ForEach(decisions.prefix(3)) { decision in
                        DecisionCard(decision: decision, model: session.workflows, showRun: true)
                            .listRowInsets(EdgeInsets(top: 6, leading: 16, bottom: 6, trailing: 16))
                            .listRowBackground(MycoColor.bg)
                            .listRowSeparator(.hidden)
                    }
                } header: {
                    Text(L("等你决定", "Waiting for you")).font(.footnote).foregroundStyle(MycoColor.muted)
                }
            }
            Section {
            // What agents do by themselves, on a schedule or on a call:
            // one row above the people (the desktop app has it under
            // Members too).
                Button {
                    router.push(.automations)
                } label: {
                    HStack(spacing: 12) {
                        AutomationIcon(symbol: "clock", hook: false).frame(width: 40)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(L("自动化", "Automations")).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg)
                            Text(automationsNote).font(.footnote).foregroundStyle(MycoColor.muted).lineLimit(1)
                        }
                        Spacer(minLength: 0)
                        Image(systemName: "chevron.right").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.faint)
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(RowButtonStyle())
                .listRowInsets(EdgeInsets(top: 9, leading: 16, bottom: 9, trailing: 16))
                .listRowBackground(MycoColor.bg)
                .listRowSeparator(.hidden)
                .accessibilityIdentifier("workbench.automations")
                Button {
                    router.push(.apps)
                } label: {
                    HStack(spacing: 12) {
                        AutomationIcon(symbol: "square.grid.2x2", hook: nil).frame(width: 40)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(L("应用", "Apps")).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg)
                            Text(appsNote).font(.footnote).foregroundStyle(MycoColor.muted).lineLimit(1)
                        }
                        Spacer(minLength: 0)
                        Image(systemName: "chevron.right").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.faint)
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(RowButtonStyle())
                .listRowInsets(EdgeInsets(top: 9, leading: 16, bottom: 9, trailing: 16))
                .listRowBackground(MycoColor.bg)
                .listRowSeparator(.hidden)
                .accessibilityIdentifier("workbench.apps")
                if !session.workflows.unsupported {
                    Button {
                        router.push(.workflows)
                    } label: {
                        HStack(spacing: 12) {
                            AutomationIcon(symbol: "point.3.connected.trianglepath.dotted", hook: nil).frame(width: 40)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(L("工作流", "Workflows")).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg)
                                Text(workflowsNote).font(.footnote)
                                    .foregroundStyle(session.workflows.decisions.isEmpty ? MycoColor.muted : MycoColor.amber).lineLimit(1)
                            }
                            Spacer(minLength: 0)
                            if !session.workflows.decisions.isEmpty {
                                Text("\(session.workflows.decisions.count)").font(.caption.weight(.semibold)).foregroundStyle(.white)
                                    .padding(.horizontal, 7).frame(minHeight: 20).background(MycoColor.amber, in: Capsule())
                            }
                            Image(systemName: "chevron.right").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.faint)
                        }
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(RowButtonStyle())
                    .listRowInsets(EdgeInsets(top: 9, leading: 16, bottom: 9, trailing: 16))
                    .listRowBackground(MycoColor.bg)
                    .listRowSeparator(.hidden)
                    .accessibilityIdentifier("workbench.workflows")
                }
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .background(MycoColor.bg)
        .workspaceChrome(title: L("工作台", "Workbench"), add: false)
        // (how many there are, for the rows)
        .task {
            await session.automations.load()
            await session.apps.load()
            await session.workflows.load()
        }
    }
}

/// Members: everyone here, like a phone's contacts. Those at work come
/// first; those away come last.
struct MembersScreen: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @State private var query = ""

    var body: some View {
        let all = session.members.filter { status in
            status.agent.name != session.superagent.agent
        }.filter { status in
            query.isEmpty || session.displayName(of: status.agent.name).localizedCaseInsensitiveContains(query)
                || status.agent.description.localizedCaseInsensitiveContains(query)
        }
        let groups: [(String, [AgentStatus])] = [
            (L("在干活", "At work"), all.filter { $0.state == "working" || $0.state == "pending" }),
            (L("空闲", "Free"), all.filter { $0.state == "idle" }),
            (L("不在线", "Away"), all.filter { $0.state == "offline" }),
        ]
        List {
            ForEach(groups.filter { !$0.1.isEmpty }, id: \.0) { title, members in
                Section {
                    ForEach(members) { status in
                        Button {
                            router.sheet = .memberCard(status.agent.name)
                        } label: {
                            PersonRow(
                                face: AvatarView.Face.of(status.agent),
                                title: session.displayName(of: status.agent.name),
                                subtitle: status.state == "working" ? Words.member(status) : status.agent.description,
                                tone: StatusTone.of(member: status), away: status.state == "offline")
                        }
                        .buttonStyle(RowButtonStyle())
                        .listRowInsets(EdgeInsets(top: 9, leading: 16, bottom: 9, trailing: 16))
                        .listRowBackground(MycoColor.bg)
                        .listRowSeparator(.hidden)
                        .accessibilityIdentifier("member." + status.agent.name)
                    }
                } header: {
                    Text(title).font(.footnote).foregroundStyle(MycoColor.muted)
                }
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .background(MycoColor.bg)
        .overlay {
            if session.loadedAt != nil, all.isEmpty {
                EmptyState(symbol: "person.2", title: query.isEmpty ? L("还没有成员", "No members yet") : L("没有匹配的成员", "No matching members"))
            }
        }
        .navigationTitle(L("成员", "Members"))
        .navigationBarTitleDisplayMode(.inline)
        .solidTopBar()
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button { router.sheet = .addAgent } label: { Image(systemName: "person.badge.plus") }
                    .accessibilityLabel(L("添加 agent", "Add an agent"))
                    .accessibilityIdentifier("members.add")
            }
        }
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .automatic), prompt: L("搜索名字和分工", "Search names and roles"))
    }
}

extension WorkbenchScreen {
    /// Under "工作流": what waits for me, or what they are for.
    fileprivate var workflowsNote: String {
        let model = session.workflows
        if !model.decisions.isEmpty {
            return L("\(model.decisions.count) 件事等你决定", model.decisions.count == 1 ? "1 decision waits for you" : "\(model.decisions.count) decisions wait for you")
        }
        let going = model.runs.filter(\.isGoing).count
        if going > 0 { return L("\(going) 个在跑", "\(going) running") }
        return L("让几个 agent 按步骤接力，要紧处问你", "Agents take turns, step by step; you decide where it matters")
    }

    /// Under "应用": the pages made from them, or what they are for.
    fileprivate var appsNote: String {
        let made = session.apps.made.count
        return made > 0
            ? L("表格、文档、白板 · 建了 \(made) 个", "Tables, documents, boards · \(made) made")
            : L("和 agent 一起用的表格、文档、白板", "Tables, documents and boards with agents")
    }

    /// Under "自动化": how many there are and how many are on, or what they are for.
    fileprivate var automationsNote: String {
        let model = session.automations
        guard let list = model.list, !list.isEmpty, !model.unsupported else {
            return L("让 agent 按时自己干活", "Let agents work on their own schedule")
        }
        let on = list.filter(\.enabled).count
        return on == list.count
            ? L("\(list.count) 个，都开着", list.count == 1 ? "1, on" : "\(list.count), all on")
            : L("\(list.count) 个，\(on) 个开着", "\(list.count), \(on) on")
    }
}

extension View {
    /// What every tab's list has: its title, the workspace at the top left,
    /// "add an agent" at the top right, and the strip that says the
    /// workspace cannot be reached.
    func workspaceChrome(title: String, add: Bool, members: Bool = false) -> some View {
        modifier(WorkspaceChrome(title: title, add: add, members: members))
    }
}

private struct WorkspaceChrome: ViewModifier {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(\.isSearching) private var isSearching
    let title: String
    let add: Bool
    /// Everyone here, one tap away (the chats page).
    var members = false

    func body(content: Content) -> some View {
        content
            // Rows are compact: more of the list fits on a screen.
            .environment(\.defaultMinListRowHeight, 0)
            // Waiting for the first answer: rows in the shape of what comes.
            .overlay {
                if session.loadedAt == nil, session.problem == nil { SkeletonRows().transition(.opacity) }
            }
            // The list fades in over them when it comes.
            .animation(.easeOut(duration: 0.22), value: session.loadedAt == nil)
            .safeAreaInset(edge: .top, spacing: 0) {
                if let problem = session.problem { StaleBanner(staleText(problem), details: .of(session.problemDetail)) }
            }
            // The app's own tab bar, one row across; the keyboard covers it
            // while searching.
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if !isSearching { AppTabBar() }
            }
            .toolbar(.hidden, for: .tabBar)
            // Pulling down asks the workspace again, now. The spinner stays
            // long enough to be seen, however quick the answer.
            .refreshable {
                // In a task of its own: SwiftUI gives this one up as soon
                // as the list is drawn again, which the first answer does;
                // the refresh then ended half done and the system's
                // spinner never showed.
                let session = session
                await Task { await session.refreshByHand() }.value
            }
            .navigationTitle(title)
            // A small title in the bar: the list starts right under it.
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) { WorkspaceButton() }
                if members {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button { router.push(.members) } label: { Image(systemName: "person.2") }
                            .accessibilityLabel(L("成员", "Members"))
                            .accessibilityIdentifier("chrome.members")
                    }
                }
                if add {
                    ToolbarItem(placement: .topBarTrailing) {
                        // An agent to talk to, or a computer for agents to work on.
                        Menu {
                            Button(L("添加 agent", "Add an agent"), systemImage: "person.badge.plus") { router.sheet = .addAgent }
                            Button(L("加一台电脑", "Add a computer"), systemImage: "desktopcomputer") { router.sheet = .addComputer(project: nil) }
                        } label: {
                            Image(systemName: "plus")
                        }
                        .accessibilityLabel(L("添加", "Add"))
                        .accessibilityIdentifier("chrome.add")
                    }
                }
            }
    }

    private func staleText(_ problem: String) -> String {
        guard let at = session.loadedAt else { return L("连不上工作区", "Cannot reach the workspace") }
        return L("连不上工作区，这是 \(RelativeTime.short(at)) 的内容", "Cannot reach the workspace. This is from \(RelativeTime.short(at)).")
    }
}

/// The three tabs, one row across the bottom of each tab's first page.
struct AppTabBar: View {
    @Environment(Router.self) private var router
    @Environment(WorkspaceSession.self) private var session

    var body: some View {
        let assistant = session.superagent.agent
        MycoTabBar(
            items: [
                .init(id: AppTab.chats, title: L("聊天", "Chats"), symbol: "bubble.left.and.bubble.right",
                      unread: session.chatRows.contains { $0.unread && $0.party != assistant }, accessibilityID: "tab.chats"),
                .init(id: AppTab.projects, title: L("项目", "Projects"), symbol: "square.stack.3d.up", tone: projectsTone, accessibilityID: "tab.projects"),
                .init(id: AppTab.workbench, title: L("工作台", "Workbench"), symbol: "square.grid.2x2",
                      tone: session.workflows.decisions.isEmpty ? nil : .attention, accessibilityID: "tab.workbench"),
            ],
            selection: router.tab,
            // (on workspaces from before superagents, no orb)
            center: session.superagent.unsupported || !session.superagent.loaded ? nil : AnyView(SuperagentOrb())
        ) { router.select($0) }
    }

    /// Someone in a project is at work (green), or all who are may be stuck (amber).
    private var projectsTone: StatusTone? {
        let inProjects = Set(session.projects.flatMap { $0.members.map(\.name) })
        let states = session.members.filter { inProjects.contains($0.agent.name) }.map { MemberState.of($0) }
        if states.contains(where: { if case .working = $0 { return true } else { return false } }) { return .working }
        return states.contains(.possiblyStuck) ? .attention : nil
    }
}
