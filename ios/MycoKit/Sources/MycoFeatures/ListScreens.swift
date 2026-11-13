import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

/// Chats: everything I talk to directly, most recent first.
struct ChatsScreen: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @State private var query = ""

    var body: some View {
        let rows = session.chatRows.filter { query.isEmpty || $0.title.localizedCaseInsensitiveContains(query) }
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
                    tone: StatusTone.of(row.state), away: row.away)
            }
            .buttonStyle(RowButtonStyle())
            .listRowInsets(EdgeInsets(top: 9, leading: 16, bottom: 9, trailing: 16))
            .listRowBackground(MycoColor.bg)
            .listRowSeparator(.hidden)
            .contextMenu {
                if case .agent(let name) = row.target {
                    Button(L("看资料", "About"), systemImage: "person.crop.circle") { router.sheet = .memberCard(name) }
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
        .workspaceChrome(title: L("聊天", "Chats"), add: true)
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .automatic), prompt: L("搜索名字", "Search names"))
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

/// Members: everyone here, like a phone's contacts. Those at work come
/// first; those away come last.
struct MembersScreen: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @State private var query = ""

    var body: some View {
        let all = session.members.filter { status in
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
        .workspaceChrome(title: L("成员", "Members"), add: true)
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .automatic), prompt: L("搜索名字和分工", "Search names and roles"))
    }
}

extension View {
    /// What every tab's list has: its title, the workspace at the top left,
    /// "add an agent" at the top right, and the strip that says the
    /// workspace cannot be reached.
    func workspaceChrome(title: String, add: Bool) -> some View {
        modifier(WorkspaceChrome(title: title, add: add))
    }
}

private struct WorkspaceChrome: ViewModifier {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(\.isSearching) private var isSearching
    let title: String
    let add: Bool

    func body(content: Content) -> some View {
        content
            // Rows are compact: more of the list fits on a screen.
            .environment(\.defaultMinListRowHeight, 0)
            // Waiting for the first answer: rows in the shape of what comes.
            .overlay {
                if session.loadedAt == nil, session.problem == nil { SkeletonRows() }
            }
            .safeAreaInset(edge: .top, spacing: 0) {
                if let problem = session.problem { StaleBanner(staleText(problem)) }
            }
            // The app's own tab bar, one row across; the keyboard covers it
            // while searching.
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if !isSearching { AppTabBar() }
            }
            .toolbar(.hidden, for: .tabBar)
            // Pulling down asks the workspace again, now.
            .refreshable { await session.refresh() }
            .navigationTitle(title)
            // A small title in the bar: the list starts right under it.
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) { WorkspaceButton() }
                if add {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button {
                            router.sheet = .addAgent
                        } label: {
                            Image(systemName: "plus")
                        }
                        .accessibilityLabel(L("添加 agent", "Add an agent"))
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
        MycoTabBar(
            items: [
                .init(id: AppTab.chats, title: L("聊天", "Chats"), symbol: "bubble.left.and.bubble.right", accessibilityID: "tab.chats"),
                .init(id: AppTab.projects, title: L("项目", "Projects"), symbol: "square.stack.3d.up", tone: projectsTone, accessibilityID: "tab.projects"),
                .init(id: AppTab.members, title: L("成员", "Members"), symbol: "person.2", accessibilityID: "tab.members"),
            ],
            selection: router.tab
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
