import SwiftUI
import MycoAPI
import MycoCore
import MycoStore
import MycoUI

/// A member's card: who it is first, then what it has been doing.
struct MemberCardSheet: View {
    let name: String
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        ScrollView {
            if let status = session.status(of: name) {
                let agent = status.agent
                VStack(spacing: 18) {
                    VStack(spacing: 8) {
                        AvatarView(.of(agent), size: 84, away: status.state == "offline")
                        Text(session.displayName(of: name)).font(.title2.weight(.semibold)).foregroundStyle(MycoColor.fg)
                        if !agent.description.isEmpty {
                            Text(agent.description).font(.subheadline).foregroundStyle(MycoColor.muted).lineLimit(1)
                        }
                        HStack(spacing: 6) {
                            if let tone = StatusTone.of(member: status) { StatusDot(tone, size: 7) }
                            Text(Words.member(status)).font(.footnote).foregroundStyle(MycoColor.fg2)
                        }
                    }
                    .padding(.top, 28)

                    Button(L("单聊", "Message")) { router.chat(with: name) }
                        .buttonStyle(PrimaryButtonStyle())
                        .accessibilityIdentifier("member.chat")

                    CardSection(title: L("它是谁", "Who it is")) {
                        KeyValueRow(L("在哪", "Where"), place(of: agent))
                        KeyValueRow(L("用的 AI", "AI"), Words.ai(agent))
                        if let owner = agent.owner, !owner.isEmpty { KeyValueRow(L("主人", "Owner"), owner) }
                        KeyValueRow(L("加入", "Joined"), RelativeTime.short(agent.createdAt))
                    }

                    let projects = session.projects(of: name)
                    if !projects.isEmpty {
                        CardSection(title: L("所在项目", "Projects")) {
                            ForEach(projects) { project in
                                Button {
                                    router.show([.project(project.name)], in: .projects)
                                } label: {
                                    HStack {
                                        Text(project.title.isEmpty ? project.name : project.title).foregroundStyle(MycoColor.fg)
                                        Spacer()
                                        Image(systemName: "chevron.right").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.faint)
                                    }
                                    .font(.subheadline)
                                    .padding(.vertical, 10)
                                    .contentShape(Rectangle())
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 24)
            } else {
                EmptyState(symbol: "person.crop.circle.badge.questionmark", title: L("找不到这个成员", "This member is gone"))
                    .padding(.top, 60)
            }
        }
        .background(MycoColor.bg)
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }

    private func place(of agent: Agent) -> String {
        let dir = agent.binding.workdir ?? ""
        if Naming.cli(of: agent).hasSuffix("-cloud") { return L("云端", "In the cloud") }
        let machine = session.machines.first { $0.id == agent.machineId }?.name
        let parts = [machine, dir.isEmpty ? nil : dir].compactMap { $0 }
        return parts.isEmpty ? L("未上报", "Not reported") : parts.joined(separator: " · ")
    }
}

/// A titled card of rows.
struct CardSection<Content: View>: View {
    let title: String
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title).font(.footnote).foregroundStyle(MycoColor.muted).padding(.leading, 4)
            VStack(spacing: 0) { content }
                .padding(.horizontal, 14)
                .padding(.vertical, 4)
                .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// A name on the left, its value on the right.
struct KeyValueRow: View {
    let key: String
    let value: String

    init(_ key: String, _ value: String) {
        self.key = key
        self.value = value
    }

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(key).foregroundStyle(MycoColor.muted)
            Spacer(minLength: 12)
            Text(value).foregroundStyle(MycoColor.fg).multilineTextAlignment(.trailing)
        }
        .font(.subheadline)
        .padding(.vertical, 10)
    }
}

/// What a project is about: its goal, brief, links, files and members.
struct ProjectInfoScreen: View {
    let name: String
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(\.openURL) private var openURL

    var body: some View {
        ScrollView {
            if let project = session.project(name) {
                VStack(spacing: 18) {
                    if let goal = project.goal, !goal.isEmpty {
                        CardSection(title: L("目标", "Goal")) {
                            Text(goal).font(.subheadline).foregroundStyle(MycoColor.fg)
                                .frame(maxWidth: .infinity, alignment: .leading).padding(.vertical, 10)
                        }
                    }
                    if let brief = project.brief, !brief.isEmpty {
                        CardSection(title: L("说明", "Brief")) {
                            ReplyText(brief).font(.subheadline).padding(.vertical, 10)
                        }
                    }
                    CardSection(title: L("成员", "Members")) {
                        ForEach(session.memberStatuses(of: project)) { status in
                            Button {
                                router.sheet = .memberCard(status.agent.name)
                            } label: {
                                HStack(spacing: 10) {
                                    AvatarView(.of(status.agent), size: 28)
                                    Text(session.displayName(of: status.agent.name)).foregroundStyle(MycoColor.fg)
                                    Spacer()
                                    Text(Words.member(status)).foregroundStyle(MycoColor.muted).lineLimit(1)
                                }
                                .font(.subheadline)
                                .padding(.vertical, 8)
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    if !project.links.isEmpty {
                        CardSection(title: L("链接", "Links")) {
                            ForEach(project.links, id: \.url) { link in
                                Button {
                                    if let url = URL(string: link.url) { openURL(url) }
                                } label: {
                                    KeyValueRow(link.title ?? link.url, "↗")
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                    if !project.docs.isEmpty {
                        CardSection(title: L("文档", "Files")) {
                            ForEach(project.docs, id: \.name) { doc in
                                KeyValueRow(doc.name, RelativeTime.short(doc.updatedAt))
                            }
                        }
                    }
                }
                .padding(20)
            }
        }
        .background(MycoColor.bg)
        .navigationTitle(L("项目资料", "About this project"))
        .navigationBarTitleDisplayMode(.inline)
    }
}

/// The workspace panel: which workspace this is, joining another, settings.
struct WorkspaceSheet: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @State private var joining = false
    /// The workspace being renamed, and the name being typed.
    @State private var renaming: WorkspaceRecord?
    @State private var newName = ""
    /// The workspace about to be left: asked once more first.
    @State private var leaving: WorkspaceRecord?

    private var inDemo: Bool { session.id == AppModel.demoID }

    var body: some View {
        NavigationStack {
            List {
                Section(L("工作区", "Workspaces")) {
                    if inDemo { row(id: session.id, name: session.name, detail: status(of: session.id), current: true) }
                    ForEach(app.workspaces) { record in
                        let current = record.id == session.id
                        Button {
                            guard !current else { return }
                            dismiss()
                            Task { await app.switchTo(record.id) }
                        } label: {
                            row(id: record.id, name: app.name(of: record), detail: current ? status(of: record.id) : host(of: record), current: current)
                        }
                        .buttonStyle(.plain)
                        .accessibilityIdentifier("workspace.row")
                        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                            Button(L("离开", "Leave"), role: .destructive) { leaving = record }
                            Button(L("改名", "Rename")) { rename(record) }.tint(MycoColor.muted)
                        }
                        .contextMenu {
                            Button(L("改名", "Rename"), systemImage: "pencil") { rename(record) }
                            Button(L("离开", "Leave"), systemImage: "rectangle.portrait.and.arrow.right", role: .destructive) { leaving = record }
                        }
                    }
                    Button(L("加入另一个工作区", "Join another workspace"), systemImage: "plus") { joining = true }
                        .accessibilityIdentifier("workspace.join")
                }
                Section {
                    NavigationLink {
                        SettingsScreen()
                    } label: {
                        Label(L("设置", "Settings"), systemImage: "gearshape")
                    }
                    if let current = app.workspaces.first(where: { $0.id == session.id }) {
                        Button(L("给这个工作区改名", "Rename this workspace"), systemImage: "pencil") { rename(current) }
                        Button(L("离开这个工作区", "Leave this workspace"), systemImage: "rectangle.portrait.and.arrow.right") { leaving = current }
                    } else {
                        Button(L("离开演示工作区", "Leave the demo workspace"), systemImage: "rectangle.portrait.and.arrow.right") {
                            dismiss()
                            Task { await app.leaveWorkspace() }
                        }
                    }
                }
            }
            .tint(MycoColor.fg)
            .navigationTitle(L("工作区", "Workspaces"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("完成", "Done")) { dismiss() }
                }
            }
            // What is new in the others is asked for when the list opens,
            // not only once a minute.
            .task { await app.refreshOthers() }
            .sheet(isPresented: $joining) { JoinSheet() }
            .alert(L("给工作区改名", "Rename the workspace"), isPresented: Binding(get: { renaming != nil }, set: { if !$0 { renaming = nil } })) {
                TextField(L("名字", "Name"), text: $newName)
                Button(L("取消", "Cancel"), role: .cancel) { renaming = nil }
                Button(L("好", "OK")) {
                    if let record = renaming { Task { await app.rename(record.id, to: newName) } }
                    renaming = nil
                }
            } message: {
                Text(L("只改这台设备上显示的名字。", "Only what it is called on this device."))
            }
            .confirmationDialog(
                leaving.map { L("离开「\(app.name(of: $0))」？", "Leave \"\(app.name(of: $0))\"?") } ?? "",
                isPresented: Binding(get: { leaving != nil }, set: { if !$0 { leaving = nil } }), titleVisibility: .visible
            ) {
                Button(L("离开", "Leave"), role: .destructive) {
                    guard let record = leaving else { return }
                    leaving = nil
                    // Leaving the one on screen puts another there: this list goes with it.
                    if record.id == session.id { dismiss() }
                    Task { await app.leave(record.id) }
                }
                Button(L("取消", "Cancel"), role: .cancel) { leaving = nil }
            } message: {
                Text(L("这台设备会从它的设备列表里移除，要再用得重新加入。别的工作区不受影响。", "This device is removed from its list of devices; to use it again, join again. Your other workspaces stay."))
            }
        }
        .presentationDetents([.medium, .large])
    }

    private func rename(_ record: WorkspaceRecord) {
        newName = app.name(of: record)
        renaming = record
    }

    /// Where a workspace is: its host, and the workspace's name in the
    /// address when the hub serves several.
    private func host(of record: WorkspaceRecord) -> String {
        let host = record.url.host() ?? record.url.absoluteString
        let path = record.url.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        return path.isEmpty ? host : host + "/" + path
    }

    private func status(of id: String) -> String {
        if session.problem != nil { return L("连不上", "Unreachable") }
        return session.loadedAt == nil ? L("正在连接…", "Connecting…") : L("已连接", "Connected")
    }

    /// One workspace: its face, its name, where it is or how it is doing,
    /// a dot when something new came there, a tick on the one on screen.
    private func row(id: String, name: String, detail: String, current: Bool) -> some View {
        HStack(spacing: 12) {
            WorkspaceBadge(name: name, size: 34)
            VStack(alignment: .leading, spacing: 2) {
                Text(name).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg).lineLimit(1)
                Text(!current && app.problem(in: id) != nil ? L("连不上", "Unreachable") : detail)
                    .font(.footnote).lineLimit(1)
                    .foregroundStyle((current ? session.problem : app.problem(in: id)) == nil ? MycoColor.muted : MycoColor.red)
            }
            Spacer(minLength: 8)
            if current {
                Image(systemName: "checkmark").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.fg2)
            } else if app.unread(in: id) > 0 {
                Circle().fill(MycoColor.unread).frame(width: 9, height: 9)
                    .accessibilityLabel(L("有新消息", "Something new"))
                    .accessibilityIdentifier("workspace.unread")
            }
        }
        .contentShape(Rectangle())
    }
}

struct SettingsScreen: View {
    @AppStorage("myco-appearance") private var appearance = "system"
    @AppStorage(AppLanguage.defaultsKey) private var language = "system"

    var body: some View {
        Form {
            Section(L("外观", "Appearance")) {
                Picker(L("主题", "Theme"), selection: $appearance) {
                    Text(L("跟随系统", "System")).tag("system")
                    Text(L("浅色", "Light")).tag("light")
                    Text(L("深色", "Dark")).tag("dark")
                }
                Picker(L("语言", "Language"), selection: $language) {
                    Text(L("跟随系统", "System")).tag("system")
                    Text("中文").tag("zh")
                    Text("English").tag("en")
                }
            }
            Section(L("关于", "About")) {
                LabeledContent(L("版本", "Version"), value: Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev")
            }
        }
        .navigationTitle(L("设置", "Settings"))
        .navigationBarTitleDisplayMode(.inline)
    }
}

/// Adding an agent from the phone: one sentence to send to any AI session.
struct AddAgentSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session
    @State private var copied = false
    @State private var link: JoinLink?
    @State private var problem: String?

    /// What to tell an agent so it joins: one command when the workspace
    /// guides agents itself, the steps spelled out for older ones.
    private var sentence: String {
        guard let link else { return "" }
        let command = "curl -fsSL \(link.joinUrl) | bash"
        if link.agentGuide == true {
            return L("加入 myco：在终端运行 \(command) ，再照它最后的输出做完。",
                     "Join myco: run \(command) in a terminal, then do what its last lines say.")
        }
        return L("加入 myco：在终端运行 \(command) ，然后在你这个会话里运行 myco join --display-name \"你的分工\" --description \"一句话说你负责什么\"，再照它的输出接收消息。",
                 "Join myco: run \(command) in a terminal, then in this session run myco join --display-name \"your role\" --description \"one line on what you do\", and receive messages as its output says.")
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack {
                Text(L("添加 agent", "Add an agent")).font(.title3.weight(.semibold)).foregroundStyle(MycoColor.fg)
                Spacer()
                Button {
                    dismiss()
                } label: {
                    Image(systemName: "xmark")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundStyle(MycoColor.muted)
                        .frame(width: 30, height: 30)
                        .background(MycoColor.pill, in: Circle())
                }
                .buttonStyle(PressScaleStyle())
                .accessibilityLabel(L("关闭", "Close"))
            }
            Text(L("把这句话发给任何一个 AI 会话（Claude Code、Codex、claude.ai……），它会自己装好 myco 并加入。",
                   "Send this sentence to any AI session (Claude Code, Codex, claude.ai…). It installs myco and joins by itself."))
                .font(.subheadline).lineSpacing(3).foregroundStyle(MycoColor.fg2)
            Group {
                if link != nil {
                    Text(sentence).textSelection(.enabled)
                } else if let problem {
                    Text(problem).foregroundStyle(MycoColor.red)
                } else {
                    // The sentence's shape while the workspace makes the link.
                    Text(String(repeating: "加入 myco 的一句话 ", count: 6)).redacted(reason: .placeholder)
                }
            }
            .font(.callout.monospaced())
            .lineSpacing(3)
            .foregroundStyle(MycoColor.fg)
            .padding(14)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
            .accessibilityIdentifier("add.sentence")
            Text(L("链接能用 10 次，一周内有效。", "The link works ten times, for a week."))
                .font(.footnote).foregroundStyle(MycoColor.muted)
            HStack(spacing: 10) {
                Button(copied ? L("已复制", "Copied") : L("复制", "Copy")) {
                    UIPasteboard.general.string = sentence
                    copied = true
                }
                .buttonStyle(PrimaryButtonStyle())
                ShareLink(item: sentence) {
                    Text(L("分享", "Share")).font(.body.weight(.semibold)).foregroundStyle(MycoColor.fg)
                        .frame(maxWidth: .infinity).padding(.vertical, 13)
                        .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                }
            }
            .padding(.top, Space.xs)
            .disabled(link == nil)
            .opacity(link == nil ? 0.4 : 1)
        }
        .padding(.horizontal, 20)
        .padding(.top, 26)
        .padding(.bottom, 12)
        .background(MycoColor.bg)
        // As tall as what is in it: the buttons sit near the bottom edge.
        .fittedSheet()
        .sensoryFeedback(.success, trigger: copied) { _, now in now }
        .task {
            do {
                link = try await session.hub.joinLink()
            } catch {
                problem = L("没能拿到加入链接：", "Could not make a join link: ") + ((error as? HubError)?.message ?? error.localizedDescription)
            }
        }
    }
}

/// The first page: join a workspace. The phone follows and directs agents;
/// a workspace is set up on a computer.
struct WelcomeScreen: View {
    @Environment(AppModel.self) private var app
    @State private var joining = false

    var body: some View {
        VStack(spacing: 14) {
            Spacer()
            AvatarView(.mushroom(MushroomLook(cap: 0, pattern: 0, face: 0, sprout: true)), size: 96)
            Text("myco").font(.largeTitle.weight(.bold)).foregroundStyle(MycoColor.fg)
            Text(L("在手机上跟进和指挥你的 agent。", "Follow and direct your agents from your phone."))
                .font(.body).foregroundStyle(MycoColor.muted).multilineTextAlignment(.center)
            Spacer()
            Button(L("用链接加入工作区", "Join a workspace with a link")) { joining = true }
                .buttonStyle(PrimaryButtonStyle())
                .accessibilityIdentifier("welcome.join")
            Text(L("链接在电脑上的 myco 里：设置 → 成员 → 添加设备。", "The link is in myco on your computer: Settings → Members → Add a device."))
                .font(.footnote).foregroundStyle(MycoColor.muted).multilineTextAlignment(.center)
            Button(L("先看看演示工作区", "Look around a demo workspace")) { app.enterDemo() }
                .font(.subheadline.weight(.medium))
                .tint(MycoColor.accent)
                .padding(.top, 6)
                .accessibilityIdentifier("welcome.demo")
        }
        .padding(28)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(MycoColor.bg)
        .sheet(isPresented: $joining) { JoinSheet() }
    }
}

/// Joining with a link: paste it, and this device signs in as you.
struct JoinSheet: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @State private var link = ""
    @State private var working = false
    @State private var problem: String?

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 14) {
                Text(L("把「添加设备」给的链接粘贴到这里。链接只能用一次，24 小时内有效。", "Paste the link that \"Add a device\" gave you. It works once, within 24 hours."))
                    .font(.subheadline).foregroundStyle(MycoColor.fg2)
                HStack(spacing: 8) {
                    TextField("https://…/c/myco_pair_…", text: $link, axis: .vertical)
                        .lineLimit(1...4)
                        .font(.callout.monospaced())
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                        .padding(12)
                        .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                        .accessibilityIdentifier("join.link")
                    PasteButton(payloadType: String.self) { strings in
                        if let first = strings.first { link = first }
                    }
                    .labelStyle(.iconOnly)
                    .tint(MycoColor.button)
                }
                if let problem {
                    Text(problem).font(.footnote).foregroundStyle(MycoColor.red).accessibilityIdentifier("join.problem")
                }
                Button(working ? L("正在加入…", "Joining…") : L("加入", "Join")) { join() }
                    .buttonStyle(PrimaryButtonStyle())
                    .disabled(working || link.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    .opacity(link.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? 0.4 : 1)
                    .accessibilityIdentifier("join.submit")
                Spacer()
            }
            .padding(20)
            .background(MycoColor.bg)
            .navigationTitle(L("加入工作区", "Join a workspace"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L("取消", "Cancel")) { dismiss() }
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    private func join() {
        working = true
        problem = nil
        Task {
            do {
                try await app.join(link: link)
                dismiss()
            } catch let failure as AppModel.JoinFailure {
                problem = failure.message
            } catch {
                problem = L("没能加入，请再试一次。", "Could not join. Try again.")
            }
            working = false
        }
    }
}
