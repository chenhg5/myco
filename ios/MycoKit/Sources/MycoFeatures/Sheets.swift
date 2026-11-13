import SwiftUI
import MycoAPI
import MycoCore
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
    @State private var joinNotice = false

    var body: some View {
        NavigationStack {
            List {
                Section(L("工作区", "Workspaces")) {
                    HStack(spacing: 12) {
                        WorkspaceBadge(name: session.name, size: 34)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(session.name).font(.body.weight(.medium))
                            Text(session.problem != nil ? L("连不上", "Unreachable") : (session.loadedAt == nil ? L("正在连接…", "Connecting…") : L("已连接", "Connected")))
                                .font(.footnote).foregroundStyle(session.problem == nil ? MycoColor.muted : MycoColor.red)
                        }
                        Spacer()
                        Image(systemName: "checkmark").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.fg2)
                    }
                    Button(L("加入另一个工作区", "Join another workspace"), systemImage: "plus") { joinNotice = true }
                }
                Section {
                    NavigationLink {
                        SettingsScreen()
                    } label: {
                        Label(L("设置", "Settings"), systemImage: "gearshape")
                    }
                    Button(L("离开这个工作区", "Leave this workspace"), systemImage: "rectangle.portrait.and.arrow.right") {
                        dismiss()
                        Task { await app.leaveWorkspace() }
                    }
                }
            }
            .tint(MycoColor.fg)
            .navigationTitle(session.name)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("完成", "Done")) { dismiss() }
                }
            }
            .alert(L("现在一次只能加入一个工作区。要换一个，先离开这个。", "One workspace at a time for now. To switch, leave this one first."), isPresented: $joinNotice) {
                Button(L("好", "OK"), role: .cancel) {}
            }
        }
        .presentationDetents([.medium, .large])
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
    @State private var copied = false

    // The real sentence carries a join link made by the workspace; this is
    // its shape until the app talks to one.
    private var sentence: String {
        L("加入 myco：在终端运行 curl -fsSL https://你的工作区/j/… | bash ，再照它最后的输出做完。",
          "Join myco: run curl -fsSL https://your-workspace/j/… | bash in a terminal, then do what its last lines say.")
    }

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 16) {
                Text(L("把这句话发给任何一个 AI 会话（Claude Code、Codex、claude.ai……），它会自己装好 myco 并加入。",
                       "Send this sentence to any AI session (Claude Code, Codex, claude.ai…). It installs myco and joins by itself."))
                    .font(.subheadline).foregroundStyle(MycoColor.fg2)
                Text(sentence)
                    .font(.callout.monospaced())
                    .foregroundStyle(MycoColor.fg)
                    .padding(14)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                    .textSelection(.enabled)
                Text(L("演示工作区没有真的链接，这里只是这句话的样子。", "The demo workspace has no real link. This only shows what the sentence looks like."))
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
                Spacer()
            }
            .padding(20)
            .background(MycoColor.bg)
            .navigationTitle(L("添加 agent", "Add an agent"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L("关闭", "Close")) { dismiss() }
                }
            }
        }
        .presentationDetents([.medium, .large])
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
