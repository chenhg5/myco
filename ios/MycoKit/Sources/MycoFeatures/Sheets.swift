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

                    // What it holds open, each saying what it waits on:
                    // above the rest, to be handled (the desktop's 当前工作).
                    if !status.current.isEmpty {
                        CurrentWork(status: status, manages: session.manages(agent))
                    }

                    // Offline: why, as far as can be told, and what to do.
                    if let why = OfflineWhy.of(status, machines: session.machines) {
                        CardSection(title: L("为什么离线", "Why it is offline")) {
                            VStack(alignment: .leading, spacing: 8) {
                                Text(why.words).font(.subheadline).foregroundStyle(MycoColor.fg)
                                    .fixedSize(horizontal: false, vertical: true)
                                    .accessibilityIdentifier("member.offline.why")
                                if why == .wsl {
                                    RawReason(OfflineWhy.keepWSL, copyLabel: L("复制", "Copy"))
                                }
                                Text(OfflineWhy.queued).font(.footnote).foregroundStyle(MycoColor.muted)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                            .padding(.vertical, 10)
                        }
                    }

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
                    // For whoever may manage it: hand it over to another
                    // session, and the times it was.
                    if Naming.kind(of: agent) == .agent, session.manages(agent) {
                        HandoverCardSections(agent: agent)
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
    @Environment(Router.self) private var router
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
                superagentSection
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
            .task { await session.superagent.load() }
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
        // Half the screen holds two workspaces and what can be done with
        // them; with more, the panel opens at its full height.
        .presentationDetents(app.workspaces.count > 2 ? [.large] : [.medium, .large])
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
    /// My superagent here: its chat (and what waits for me), or setting
    /// one up. Not on workspaces from before superagents.
    @ViewBuilder private var superagentSection: some View {
        let model = session.superagent
        if model.loaded, !model.unsupported {
            Section {
                if let agent = model.agent {
                    Button {
                        dismiss()
                        router.chat(with: agent)
                    } label: {
                        HStack(spacing: 12) {
                            Image(systemName: "sparkles").frame(width: 24).foregroundStyle(MycoColor.accent)
                            VStack(alignment: .leading, spacing: 2) {
                                Text("Superagent · " + session.displayName(of: agent)).foregroundStyle(MycoColor.fg)
                                Text(model.pending > 0
                                     ? L("\(model.pending) 件事等你批准", model.pending == 1 ? "1 request waits for you" : "\(model.pending) requests wait for you")
                                     : L("你在这个工作区的总管", "Runs this workspace for you"))
                                    .font(.footnote).foregroundStyle(model.pending > 0 ? MycoColor.amber : MycoColor.muted)
                            }
                            Spacer(minLength: 0)
                            if model.pending > 0 {
                                Text("\(model.pending)").font(.caption.weight(.semibold)).foregroundStyle(.white)
                                    .padding(.horizontal, 7).frame(minHeight: 20).background(MycoColor.amber, in: Capsule())
                            }
                        }
                    }
                    .accessibilityIdentifier("workspace.superagent")
                    .contextMenu {
                        Button(L("换一个 superagent", "Change your superagent"), systemImage: "arrow.triangle.2.circlepath") { router.sheet = .superagent }
                    }
                } else {
                    Button {
                        router.sheet = .superagent
                    } label: {
                        Label(L("配一个 superagent", "Set up your superagent"), systemImage: "sparkles")
                    }
                    .accessibilityIdentifier("workspace.superagent.setup")
                }
            }
        }
    }

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
    @AppStorage("myco-return-sends") private var returnSends = false
    @Environment(AppModel.self) private var app

    /// A time of day (minutes from midnight) as the date a picker edits.
    private func time(_ minutes: Int) -> Date {
        Calendar.current.date(bySettingHour: minutes / 60, minute: minutes % 60, second: 0, of: Date()) ?? Date()
    }

    private func minutes(_ date: Date) -> Int {
        let parts = Calendar.current.dateComponents([.hour, .minute], from: date)
        return (parts.hour ?? 0) * 60 + (parts.minute ?? 0)
    }

    var body: some View {
        Form {
            Section {
                Toggle(L("新消息通知", "Notify me of new messages"), isOn: Binding(
                    get: { app.notifications.enabled },
                    set: { on in Task { await app.setNotificationsEnabled(on) } }))
                    .tint(MycoColor.green)
                    .accessibilityIdentifier("settings.notifications")
                if app.notificationsRefused {
                    HStack(alignment: .firstTextBaseline) {
                        Text(L("系统没有允许 myco 发通知。", "The system does not let myco send notifications."))
                            .font(.footnote).foregroundStyle(MycoColor.amber)
                        Spacer(minLength: 8)
                        if let url = URL(string: UIApplication.openSettingsURLString) {
                            Button(L("去设置", "Settings")) { UIApplication.shared.open(url) }
                                .font(.footnote.weight(.medium)).tint(MycoColor.accent)
                        }
                    }
                }
                if app.notifications.enabled {
                    Toggle(L("进展也通知", "Interim updates too"), isOn: Binding(
                        get: { app.notifications.progressToo },
                        set: { on in app.setNotifications { $0.progressToo = on } }))
                        .tint(MycoColor.green)
                    Toggle(L("免打扰时段", "Quiet hours"), isOn: Binding(
                        get: { app.notifications.quietHours },
                        set: { on in app.setNotifications { $0.quietHours = on } }))
                        .tint(MycoColor.green)
                        .accessibilityIdentifier("settings.quietHours")
                    if app.notifications.quietHours {
                        DatePicker(L("从", "From"), selection: Binding(
                            get: { time(app.notifications.quietFrom) },
                            set: { date in app.setNotifications { $0.quietFrom = minutes(date) } }), displayedComponents: .hourAndMinute)
                        DatePicker(L("到", "To"), selection: Binding(
                            get: { time(app.notifications.quietTo) },
                            set: { date in app.setNotifications { $0.quietTo = minutes(date) } }), displayedComponents: .hourAndMinute)
                    }
                }
            } header: {
                Text(L("通知", "Notifications"))
            } footer: {
                Text(L("回复、没做成、agent 问你问题时通知；正在看的那个对话不通知。App 开着时马上通知；不在前台时由系统不定时唤醒它检查，可能迟到。单个对话可以在聊天列表里长按设为免打扰。",
                       "For answers, failures and questions from an agent; never for the conversation on screen. At once while the app is open; when it is not in front the system wakes it now and then to check, so it can be late. A long press on a chat mutes that one."))
            }
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
                // For the conversations without one of their own.
                NavigationLink {
                    ChatBackgroundSheet(key: nil)
                } label: {
                    Text(L("默认聊天背景", "Default chat background"))
                }
                .accessibilityIdentifier("settings.background")
            }
            Section {
                Toggle(L("回车键发送", "Return sends"), isOn: $returnSends)
                    .tint(MycoColor.green)
                    .accessibilityIdentifier("settings.returnSends")
            } header: {
                Text(L("输入", "Typing"))
            } footer: {
                Text(L("关着时回车是换行，用右下角的按钮发送：给 agent 的话常常要分段，发出去就会叫醒它或打断正在做的事。打开后回车直接发送，像聊天软件那样，但就不能用回车换行了。",
                       "Off: Return breaks the line and the button at the lower right sends. What you tell an agent often takes several lines, and sending wakes it or interrupts what it is doing. On: Return sends, as in a chat app, and can no longer break a line."))
            }
            Section {
                NavigationLink {
                    ProxySettingsScreen()
                } label: {
                    LabeledContent(L("代理", "Proxy"), value: ProxySettingsScreen.word(for: app.proxy))
                }
                .accessibilityIdentifier("settings.proxy")
            } header: {
                Text(L("网络", "Network"))
            } footer: {
                Text(L("连工作区和语音服务时走的代理。局域网里的地址总是直连。", "How the app reaches its workspaces and the voice services. Addresses on the local network always go direct."))
            }
            Section {
                NavigationLink {
                    VoiceSettingsScreen()
                } label: {
                    LabeledContent(L("语音聊天", "Voice chat"), value: app.voiceReady ? L("已设置", "Set up") : L("未设置", "Not set up"))
                }
                .accessibilityIdentifier("settings.voice")
            } header: {
                Text(L("语音", "Voice"))
            } footer: {
                Text(L("和 agent 像打电话一样说话。设置好以后，输入框空着时右下角的按钮就是语音聊天。",
                       "Talk with an agent as on a call. Once it is set up, with nothing typed the button at the lower right starts a voice chat."))
            }
            Section(L("关于", "About")) {
                LabeledContent(L("版本", "Version"), value: Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev")
            }
        }
        .navigationTitle(L("设置", "Settings"))
        .navigationBarTitleDisplayMode(.inline)
    }
}

/// Settings › 语音: the model a voice call talks with, and the speech
/// service it answers with (one of `SpeechService.all`). Keys go to the
/// keychain and are never shown again. The settings are the person's:
/// kept on the workspaces too, so what was set on the desktop is here
/// already, and the other way round.
struct VoiceSettingsScreen: View {
    /// Opened by itself from a conversation: it has a button to close it.
    var closes = false
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @State private var settings = VoiceSettings()
    /// What the fields started from: untouched, they follow what another
    /// device set meanwhile.
    @State private var loaded = VoiceSettings()
    @State private var modelKey = ""
    @State private var speechKey = ""
    @State private var forgetModelKey = false
    @State private var forgetSpeechKey = false
    @State private var read = false
    /// Trying the voice: while it is being made, and why it could not be.
    @State private var trying = false
    @State private var tryProblem: String?

    private var hasModelKey: Bool { app.voiceHasModelKey && !forgetModelKey }
    /// (The key on this device is the one of the service in use.)
    private var hasSpeechKey: Bool {
        app.voiceHasSpeechKey && !forgetSpeechKey && settings.service.id == app.voice.service.id
    }

    /// What is still missing before a call can start.
    private var missing: String? {
        if !settings.namesModel { return L("填上接口地址和模型，就可以开始语音聊天。", "Fill in the address and the model to start a voice chat.") }
        if !hasModelKey, modelKey.trimmingCharacters(in: .whitespaces).isEmpty { return L("还差模型的密钥。", "The model's key is still missing.") }
        return nil
    }

    private var service: SpeechService { settings.service }

    /// Where the settings are kept, in words.
    private var kept: String {
        let lines = app.voiceSync
        var said: [String] = []
        let have = lines.filter { $0.state == .synced }.map(\.name)
        let old = lines.filter { $0.state == .unsupported }.map(\.name)
        let away = lines.filter { $0.state == .unreachable }.map(\.name)
        if !have.isEmpty { said.append(L("已存到：\(have.joined(separator: "、"))。", "Kept on: \(have.joined(separator: ", ")).")) }
        if !old.isEmpty {
            said.append(L("\(old.joined(separator: "、")) 的 Hub 版本太旧，还不能存，升级它的 myco 就可以。",
                          "\(old.joined(separator: ", ")): the hub is too old to keep them; update its myco."))
        }
        if !away.isEmpty { said.append(L("\(away.joined(separator: "、")) 现在连不上，下次再试。", "\(away.joined(separator: ", ")): not reachable now; tried again next time.")) }
        return said.joined(separator: " ")
    }

    var body: some View {
        Form {
            Section {
                TextField(L("接口地址，如 https://api.deepseek.com/v1", "Address, e.g. https://api.openai.com/v1"), text: $settings.baseURL)
                    .keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled()
                    .accessibilityIdentifier("voice.baseURL")
                TextField(L("模型，如 deepseek-chat", "Model, e.g. gpt-4o-mini"), text: $settings.model)
                    .textInputAutocapitalization(.never).autocorrectionDisabled()
                    .accessibilityIdentifier("voice.model")
                key($modelKey, saved: hasModelKey, forget: $forgetModelKey, id: "voice.modelKey")
            } header: {
                Text(L("快模型（OpenAI 兼容接口）", "Fast model (OpenAI-compatible API)"))
            } footer: {
                Text(missing ?? L("它听你说话、把话转给 agent、再把进展说给你听，所以要选反应快的模型。",
                                  "It hears you, passes things on to the agent and tells you how it goes, so pick a fast one."))
                    .foregroundStyle(missing == nil ? MycoColor.muted : MycoColor.amber)
            }
            Section {
                // Which service speaks: one of those this app knows.
                Picker(L("服务", "Service"), selection: Binding(
                    get: { service.id },
                    set: { id in
                        // Another service has other voices and another key.
                        if id != service.id { settings.speaker = "" }
                        settings.provider = id
                        speechKey = ""
                        tryProblem = nil
                    })) {
                    ForEach(SpeechService.all) { one in
                        Text(AppLanguage.current == .zh ? one.name : one.englishName).tag(one.id)
                    }
                }
                .accessibilityIdentifier("voice.service")
                key($speechKey, saved: hasSpeechKey, forget: $forgetSpeechKey, id: "voice.speechKey")
                if hasSpeechKey || !speechKey.isEmpty {
                    if service.voices.isEmpty {
                        TextField(L("音色，留空用默认的", "Voice id; empty uses the default"), text: $settings.speaker)
                            .textInputAutocapitalization(.never).autocorrectionDisabled()
                    } else {
                        // The voice, from the service's list (or an ID typed in).
                        NavigationLink {
                            VoicePickerScreen(service: service, speaker: $settings.speaker)
                        } label: {
                            LabeledContent(L("音色", "Voice"), value: voiceName)
                        }
                        .accessibilityIdentifier("voice.speaker")
                    }
                    Stepper(value: $settings.rate, in: -50...100, step: 10) {
                        LabeledContent(L("语速", "Speed"), value: settings.rate == 0 ? L("正常", "Normal") : (settings.rate > 0 ? "+\(settings.rate)" : "\(settings.rate)"))
                    }
                    // One line in that voice, before anything is kept.
                    Button {
                        tryVoice()
                    } label: {
                        HStack {
                            Text(trying ? L("合成中…", "Speaking…") : L("试听", "Try it"))
                            if trying {
                                Spacer()
                                ProgressView()
                            }
                        }
                    }
                    .disabled(trying)
                    .tint(MycoColor.accent)
                    .accessibilityIdentifier("voice.try")
                    if let tryProblem {
                        Text(tryProblem).font(.footnote).foregroundStyle(MycoColor.amber)
                            .accessibilityIdentifier("voice.tryProblem")
                    }
                }
            } header: {
                Text(L("语音合成", "Speech"))
            } footer: {
                Text((AppLanguage.current == .zh ? service.help : service.englishHelp)
                     + L(" 没有密钥时用 iPhone 自带的声音。", " Without a key, the iPhone's own voice is used."))
            }
            Section {
                Toggle(L("存到工作区，多设备同步", "Keep on the workspaces"), isOn: $settings.share)
                    .tint(MycoColor.green)
                    .accessibilityIdentifier("voice.share")
            } header: {
                Text(L("多设备同步", "On your other devices"))
            } footer: {
                VStack(alignment: .leading, spacing: 6) {
                    Text(L("打开后，这里的设置和密钥存在你加入的工作区（Hub）里，电脑上的 myco 和别的手机不用再填一遍；哪台设备后改的算数。密钥只给你自己的设备，但 Hub 所在机器的管理员读得到它的数据库：不是你自己的 Hub，介意就关掉。",
                           "With this on, these settings and their keys are kept on the workspaces (hubs) you joined, so myco on your computer and other phones need not be set up again; the device that changed them last wins. The keys are handed only to your own devices, but whoever administers a hub's machine can read its database: turn this off for a hub that is not yours, if that matters."))
                    if settings.share, !kept.isEmpty {
                        Text(kept).foregroundStyle(MycoColor.fg2)
                            .accessibilityIdentifier("voice.kept")
                    }
                }
            }
            Section {
            } footer: {
                Text(L("密钥在这台 iPhone 上存在钥匙串里。你说的话由苹果的语音识别转成文字（这门语言能在手机上识别时不出手机），文字发给上面的模型；它回答的文字发给上面选的语音合成服务。",
                       "On this iPhone the keys are in the keychain. Apple's speech recognition turns what you say into text (on the phone when the language allows); the text goes to the model above, and its answers go to the speech service chosen above."))
            }
        }
        .navigationTitle(L("语音聊天", "Voice chat"))
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if closes {
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("完成", "Done")) { dismiss() }
                        .accessibilityIdentifier("voice.done")
                }
            }
        }
        .onAppear {
            guard !read else { return }
            read = true
            settings = app.voice
            loaded = app.voice
        }
        // What another device set since is here once the workspaces have
        // answered; fields already touched are left alone.
        .task {
            await app.syncVoice()
            if settings == loaded {
                settings = app.voice
                loaded = app.voice
            }
        }
        // Leaving the page keeps what was typed.
        .onDisappear {
            let chosen = settings
            let keys = (model: modelKey, speech: speechKey)
            let forget = (model: forgetModelKey && modelKey.isEmpty, speech: forgetSpeechKey && speechKey.isEmpty)
            Task {
                await app.saveVoice(
                    chosen, modelKey: keys.model, speechKey: keys.speech, forgetModelKey: forget.model, forgetSpeechKey: forget.speech)
            }
        }
    }

    /// The chosen voice by its name on the list; an ID typed in, as it is.
    private var voiceName: String {
        let id = settings.speakerInUse
        return service.voices.first { $0.id == id }?.name ?? id
    }

    private func tryVoice() {
        let chosen = settings
        let typed = speechKey
        trying = true
        tryProblem = nil
        Task {
            do {
                try await app.tryVoice(chosen, speechKey: typed)
            } catch {
                tryProblem = error.localizedDescription
            }
            trying = false
        }
    }

    /// A key's field: what is saved is never shown, only that it is.
    @ViewBuilder private func key(_ text: SwiftUI.Binding<String>, saved: Bool, forget: SwiftUI.Binding<Bool>, id: String) -> some View {
        HStack {
            SecureField(saved ? L("密钥已保存，要换就填新的", "Key saved; type a new one to replace it") : "API Key", text: text)
                .textInputAutocapitalization(.never).autocorrectionDisabled()
                .accessibilityIdentifier(id)
            if saved, text.wrappedValue.isEmpty {
                Button(L("清除", "Clear")) { forget.wrappedValue = true }
                    .font(.footnote).tint(MycoColor.red)
                    .buttonStyle(.borderless)
            }
        }
    }
}

/// Settings › 代理: as the system has it, none, or a proxy set here,
/// tried before it is saved. The desktop app's page, on the phone.
struct ProxySettingsScreen: View {
    @Environment(AppModel.self) private var app
    @State private var settings = ProxySettings()
    @State private var port = ""
    @State private var password = ""
    @State private var forgetPassword = false
    @State private var read = false
    @State private var busy = false
    /// How the last try went.
    @State private var tried: ProxyTrial?
    @State private var saved = false

    /// The setting as typed.
    private var typed: ProxySettings {
        var out = settings
        out.host = out.host.trimmingCharacters(in: .whitespaces)
        out.user = out.user.trimmingCharacters(in: .whitespaces)
        out.port = Int(port) ?? 0
        return out
    }

    private var hasPassword: Bool { app.proxyHasPassword && !forgetPassword }

    private var dirty: Bool {
        typed != app.proxy || !password.isEmpty || (forgetPassword && app.proxyHasPassword)
    }

    /// "System", "Off", "SOCKS5 host:port".
    static func word(for settings: ProxySettings) -> String {
        switch settings.mode {
        case .system: return L("跟系统", "System")
        case .none: return L("不用代理", "Off")
        case .manual: return name(of: settings.kind) + " " + settings.host + ":" + String(settings.port)
        }
    }

    static func name(of kind: ProxySettings.Kind) -> String {
        switch kind {
        case .socks5: return "SOCKS5"
        case .http: return "HTTP"
        case .https: return "HTTPS"
        }
    }

    /// How a try went, in words.
    static func words(for trial: ProxyTrial) -> String {
        switch trial {
        case .connected(let milliseconds): return L("连通了，用时 \(milliseconds) 毫秒", "Connected in \(milliseconds) ms")
        // (A proxy that wants a password and gets none, or a wrong one,
        // is asked again and again until the time is up: the system does
        // not say so.)
        case .timeout: return L("12 秒内没有回应。代理要用户名和密码而没填对时，也是这样。", "No answer within 12 seconds. A proxy that wants a user name and password, and did not get the right ones, ends this way too.")
        case .refused: return L("代理拒绝了连接：它在运行吗，端口对吗？", "The proxy refused the connection: is it running, on that port?")
        case .notFound: return L("找不到这个地址", "That address was not found")
        case .auth: return L("代理要用户名和密码，或者没接受填的这组", "The proxy wants a user name and password, or did not accept these")
        case .other(let said): return L("连不上：", "Could not connect: ") + said
        }
    }

    private var note: String {
        switch settings.mode {
        case .system: return L("用 iPhone 自己的设置：无线局域网里配的代理、VPN。没有就直连。", "Uses the iPhone's own settings: the proxy set for the Wi-Fi network, a VPN. Without one, connects directly.")
        case .none: return L("所有连接都直连，不走系统里配的代理。", "Every connection goes directly, not through a proxy the system is set to.")
        case .manual: return L("比如电脑上 Clash、V2Ray 一类的代理：SOCKS5 常见端口 1080 / 7891，HTTP 常见 7890 / 8080。填电脑在局域网里的地址，并在代理里打开「允许局域网连接」。",
                               "Such as Clash or V2Ray on a computer: SOCKS5 is often on 1080 / 7891, HTTP on 7890 / 8080. Use the computer's address on the local network, and let the proxy accept connections from it.")
        }
    }

    private var problem: String? {
        switch typed.problem {
        case nil: return nil
        case .host: return L("地址填主机名或 IP，不带 http:// 和端口。", "The address is a host name or IP, without http:// or a port.")
        case .port: return L("端口是 1 到 65535。", "The port is 1 to 65535.")
        }
    }

    var body: some View {
        Form {
            Section {
                Picker(L("代理", "Proxy"), selection: $settings.mode) {
                    Text(L("跟系统", "System")).tag(ProxySettings.Mode.system)
                    Text(L("不用代理", "Off")).tag(ProxySettings.Mode.none)
                    Text(L("手动设置", "Set here")).tag(ProxySettings.Mode.manual)
                }
                .pickerStyle(.segmented)
                .accessibilityIdentifier("proxy.mode")
            } footer: {
                Text(note)
            }
            if settings.mode == .manual {
                Section {
                    Picker(L("类型", "Type"), selection: $settings.kind) {
                        ForEach(ProxySettings.Kind.allCases, id: \.self) { kind in
                            Text(Self.name(of: kind)).tag(kind)
                        }
                    }
                    .pickerStyle(.segmented)
                    .accessibilityIdentifier("proxy.kind")
                    TextField(L("地址，如 192.168.1.10", "Host, e.g. 192.168.1.10"), text: $settings.host)
                        .keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled()
                        .accessibilityIdentifier("proxy.host")
                    TextField(L("端口，如 ", "Port, e.g. ") + (settings.kind == .socks5 ? "1080" : "7890"), text: $port)
                        .keyboardType(.numberPad)
                        .onChange(of: port) { _, new in
                            let digits = String(new.filter(\.isNumber).prefix(5))
                            if digits != new { port = digits }
                        }
                        .accessibilityIdentifier("proxy.port")
                } header: {
                    Text(L("代理", "The proxy"))
                } footer: {
                    if let problem, !typed.host.isEmpty || !port.isEmpty {
                        Text(problem).foregroundStyle(MycoColor.amber)
                    }
                }
                Section {
                    TextField(L("用户名（可不填）", "User (optional)"), text: $settings.user)
                        .textInputAutocapitalization(.never).autocorrectionDisabled()
                        .accessibilityIdentifier("proxy.user")
                    HStack {
                        SecureField(hasPassword ? L("密码已保存，要换就填新的", "Password saved; type a new one to replace it") : L("密码（可不填）", "Password (optional)"), text: $password)
                            .textInputAutocapitalization(.never).autocorrectionDisabled()
                            .accessibilityIdentifier("proxy.password")
                        if hasPassword, password.isEmpty {
                            Button(L("清除", "Clear")) { forgetPassword = true }
                                .font(.footnote).tint(MycoColor.red)
                                .buttonStyle(.borderless)
                        }
                    }
                } footer: {
                    Text(L("密码存在这台 iPhone 的钥匙串里。", "The password is kept in this iPhone's keychain."))
                }
            }
            Section {
                if settings.mode == .manual {
                    Button {
                        tryIt()
                    } label: {
                        HStack {
                            Text(L("试一下", "Try it"))
                            if busy {
                                Spacer()
                                ProgressView()
                            }
                        }
                    }
                    .disabled(busy || typed.problem != nil)
                    .tint(MycoColor.accent)
                    .accessibilityIdentifier("proxy.try")
                }
                Button(L("保存", "Save")) { save() }
                    .disabled(busy || !dirty || typed.problem != nil)
                    .tint(MycoColor.accent)
                    .accessibilityIdentifier("proxy.save")
            } footer: {
                VStack(alignment: .leading, spacing: 6) {
                    if let tried {
                        let connected: Bool = { if case .connected = tried { return true } else { return false } }()
                        Text(Self.words(for: tried)).foregroundStyle(connected ? MycoColor.green : MycoColor.amber)
                            .accessibilityIdentifier("proxy.tried")
                    }
                    if saved, !dirty {
                        Text(L("已保存，马上生效", "Saved; in effect now")).foregroundStyle(MycoColor.green)
                            .accessibilityIdentifier("proxy.saved")
                    }
                    Text(L("这台手机和局域网里的地址（比如家里电脑上的工作区）总是直连，不走这里设的代理。",
                           "Addresses on this phone and the local network (a workspace on a computer at home, say) always go direct, not through the proxy set here."))
                }
            }
        }
        .navigationTitle(L("代理", "Proxy"))
        .navigationBarTitleDisplayMode(.inline)
        .onAppear {
            guard !read else { return }
            read = true
            settings = app.proxy
            port = app.proxy.port > 0 ? String(app.proxy.port) : ""
        }
        .onChange(of: typed) { tried = nil }
    }

    private func tryIt() {
        let setting = typed
        let typedPassword = password
        busy = true
        tried = nil
        Task {
            tried = await app.tryProxy(setting, password: typedPassword)
            busy = false
        }
    }

    private func save() {
        let setting = typed
        let typedPassword = password
        let forget = forgetPassword && password.isEmpty
        busy = true
        Task {
            await app.saveProxy(setting, password: typedPassword, forgetPassword: forget)
            password = ""
            forgetPassword = false
            saved = true
            busy = false
        }
    }
}

/// The voices of a speech service, by language, to pick one; or an ID
/// typed in, for a voice that is not on the list (a cloned one).
struct VoicePickerScreen: View {
    let service: SpeechService
    @SwiftUI.Binding var speaker: String
    @Environment(\.dismiss) private var dismiss
    @State private var search = ""
    @State private var typed = ""

    /// The voice in use: the one chosen, or the service's default.
    private var chosen: String {
        let id = speaker.trimmingCharacters(in: .whitespacesAndNewlines)
        return id.isEmpty ? service.defaultSpeaker : id
    }

    private var onList: Bool { service.voices.contains { $0.id == chosen } }

    private var groups: [(lang: String, voices: [SpeechVoice])] {
        let wanted = search.trimmingCharacters(in: .whitespaces).lowercased()
        let voices = wanted.isEmpty ? service.voices : service.voices.filter {
            $0.name.lowercased().contains(wanted) || $0.id.lowercased().contains(wanted)
                || SpeechVoice.languageName($0.lang).lowercased().contains(wanted)
        }
        return SpeechVoice.groups(voices)
    }

    var body: some View {
        List {
            ForEach(groups, id: \.lang) { group in
                Section(SpeechVoice.languageName(group.lang)) {
                    ForEach(group.voices) { voice in
                        Button {
                            speaker = voice.id
                            dismiss()
                        } label: {
                            HStack {
                                Text(voice.label).foregroundStyle(MycoColor.fg)
                                Spacer()
                                if voice.id == chosen {
                                    Image(systemName: "checkmark").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.accent)
                                }
                            }
                        }
                        .accessibilityIdentifier("voice.pick." + voice.id)
                    }
                }
            }
            if search.isEmpty {
                Section {
                    HStack {
                        TextField("S_…", text: $typed)
                            .textInputAutocapitalization(.never).autocorrectionDisabled()
                            .font(.body.monospaced())
                            .accessibilityIdentifier("voice.pick.typed")
                        if !onList, typed.trimmingCharacters(in: .whitespaces) == chosen {
                            Image(systemName: "checkmark").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.accent)
                        }
                    }
                    Button(L("用这个音色 ID", "Use this voice ID")) {
                        speaker = typed.trimmingCharacters(in: .whitespacesAndNewlines)
                        dismiss()
                    }
                    .disabled(typed.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    .tint(MycoColor.accent)
                } header: {
                    Text(L("其他（填音色 ID）", "Other (type a voice ID)"))
                } footer: {
                    Text(L("不在列表里的音色，比如复刻的声音（以 S_ 开头），填它的 ID。", "For a voice that is not on the list, like a cloned one (it starts with S_), type its ID."))
                }
            }
        }
        .searchable(text: $search, prompt: L("搜索音色", "Search voices"))
        .navigationTitle(L("音色", "Voice"))
        .navigationBarTitleDisplayMode(.inline)
        .onAppear { if !onList { typed = chosen } }
    }
}

/// Adding an agent from the phone: one sentence to send to any AI session.
struct AddAgentSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session
    @State private var copied = false
    @State private var link: JoinLink?
    @State private var failure: Failure?
    @State private var attempt = 0

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
            if link == nil, let failure {
                // (outside the box below: that one is the sentence's)
                VStack(alignment: .leading, spacing: 6) {
                    Text(L("没能拿到加入链接。", "Could not make a join link.")).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg)
                    FailureLine(failure: failure) {
                        self.failure = nil
                        attempt += 1
                    }
                }
                .padding(14)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
            } else {
                Group {
                    if link != nil {
                        Text(sentence).textSelection(.enabled)
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
            }
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
        // (and again each time "重试" is tapped)
        .task(id: attempt) {
            do {
                link = try await session.hub.joinLink()
            } catch is CancellationError {
                // the sheet was closed
            } catch {
                failure = Failure.of(error, canRetry: true)
            }
        }
    }
}

/// The first page: join a workspace. The phone follows and directs agents;
/// a workspace is set up on a computer.
struct WelcomeScreen: View {
    @Environment(AppModel.self) private var app
    @State private var joining = false
    @State private var copiedSetup = false

    var body: some View {
        VStack(spacing: 14) {
            Spacer()
            // The logo as designed: the pixel mushroom and the wordmark.
            BrandLogo(.whole, height: 60)
                .foregroundStyle(MycoColor.fg)
                .padding(.bottom, 10)
                .accessibilityIdentifier("welcome.logo")
            Text(L("在手机上跟进和指挥你的 agent。", "Follow and direct your agents from your phone."))
                .font(.body).foregroundStyle(MycoColor.muted).multilineTextAlignment(.center)
            Spacer()
            Button(L("用链接加入工作区", "Join a workspace with a link")) { joining = true }
                .buttonStyle(PrimaryButtonStyle())
                .accessibilityIdentifier("welcome.join")
            Text(L("链接在电脑上的 myco 里：设置 → 成员 → 添加设备。", "The link is in myco on your computer: Settings → Members → Add a device."))
                .font(.footnote).foregroundStyle(MycoColor.muted).multilineTextAlignment(.center)
            // Not set up anywhere yet: a sentence for the agent one already
            // has (Claude Code, Codex…) on a computer, to set myco up there.
            Button(copiedSetup ? L("已复制，发给你电脑上的 agent", "Copied: send it to the agent on your computer") : L("还没装 myco？复制一句话让你的 agent 装好", "No myco yet? Copy a sentence for your agent to set it up")) {
                UIPasteboard.general.string = AgentSetup.setup
                copiedSetup = true
            }
            .font(.footnote.weight(.medium))
            .tint(MycoColor.fg2)
            .multilineTextAlignment(.center)
            .accessibilityIdentifier("welcome.setup")
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
    @State private var problem: Failure?

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
                    FailureLine(failure: problem, id: "join.problem")
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
                problem = Failure(failure.message, advice: failure.advice, detail: failure.detail)
            } catch {
                problem = Failure(L("没能加入，请再试一次。", "Could not join. Try again."))
            }
            working = false
        }
    }
}

/// An agent's open work: each task, what it waits on, from whom, how long
/// ago it moved, what it says; and for whoever manages it, close it as done
/// or stop it.
private struct CurrentWork: View {
    let status: AgentStatus
    let manages: Bool
    @Environment(WorkspaceSession.self) private var session
    @State private var busy: String?
    @State private var failure: Failure?

    var body: some View {
        CardSection(title: L("当前工作", "Current work")) {
            VStack(alignment: .leading, spacing: 0) {
                ForEach(status.current) { task in
                    row(task)
                    if task.id != status.current.last?.id { Divider().overlay(MycoColor.line) }
                }
                if let failure {
                    FailureLine(failure: failure).padding(.vertical, 8)
                }
            }
        }
        .accessibilityIdentifier("member.current")
    }

    private func row(_ task: Message) -> some View {
        let line = OpenWork.line(of: task, in: status, name: session.displayName(of:))
        let left = (status.staleTasks ?? []).contains(task.id)
        return VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 6) {
                Text(line).font(.caption.weight(.semibold)).foregroundStyle(left ? MycoColor.amber : MycoColor.fg2)
                Text(session.displayName(of: task.from) + " → " + session.displayName(of: task.to))
                    .font(.caption).foregroundStyle(MycoColor.muted).lineLimit(1)
                Spacer(minLength: 0)
                Text(RelativeTime.short(task.activeAt ?? task.updatedAt)).font(.caption).foregroundStyle(MycoColor.faint)
            }
            Text(task.body).font(.subheadline).foregroundStyle(MycoColor.fg).lineLimit(2)
            if manages {
                HStack(spacing: 16) {
                    Button(L("标记完成", "Mark done")) { act(task) { try await session.hub.dismiss(task.id) } }
                        .accessibilityIdentifier("member.current.done")
                    if task.status != "queued" {
                        Button(L("停止", "Stop")) { act(task) { try await session.hub.stop(task.id) } }
                            .accessibilityIdentifier("member.current.stop")
                    }
                }
                .font(.footnote.weight(.medium)).tint(MycoColor.accent)
                .disabled(busy != nil)
                .padding(.top, 2)
            }
        }
        .padding(.vertical, 10)
        .accessibilityElement(children: .contain)
    }

    private func act(_ task: Message, _ run: @escaping @MainActor () async throws -> Void) {
        busy = task.id
        Task {
            do {
                try await run()
                failure = nil
            } catch {
                failure = Failure.of(error)
            }
            await session.refresh()
            busy = nil
        }
    }
}

/// Naming my superagent, or starting a Shroom for it on one of my
/// computers (the desktop app's SuperagentSetup).
struct SuperagentSetupSheet: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(\.dismiss) private var dismiss
    @State private var startNew = true
    @State private var machine = ""
    @State private var cli = "claude"
    @State private var pick = ""

    private var computers: [Machine] {
        session.machines.filter { $0.online && $0.canSpawn == true }
    }

    private var mine: [Agent] {
        session.agentsByName.filter { $0.kind != "machine" && session.manages($0) }
    }

    var body: some View {
        let model = session.superagent
        NavigationStack {
            Form {
                Section {
                    Text(L(
                        "superagent 是你在这个工作区的总管，名字叫 \(SuperagentModel.shroom)：它能看全工作区，帮你开 agent、建项目、派活、盯进度。删东西、停东西、改别人的权限这类事，它会先来问你。它不能以你的名义说话。",
                        "Your superagent, \(SuperagentModel.shroom), runs this workspace for you: it sees everything, starts agents, makes projects, hands out work and keeps track. Deleting, stopping or changing someone's access, it asks you first. It never speaks as you."))
                        .font(.footnote).foregroundStyle(MycoColor.fg2)
                        .fixedSize(horizontal: false, vertical: true)
                    Picker(L("怎么配", "How"), selection: $startNew) {
                        Text(L("新开一个 \(SuperagentModel.shroom)", "Start a new \(SuperagentModel.shroom)")).tag(true)
                        Text(L("用已有的 agent", "Use one of your agents")).tag(false)
                    }
                    .pickerStyle(.segmented)
                    .accessibilityIdentifier("superagent.how")
                }
                if startNew {
                    if computers.isEmpty {
                        Section {
                            Text(L("你还没有在线、能开 agent 的电脑。先在电脑上连一台，或者用已有的 agent。", "None of your computers is online and able to start agents. Connect one, or use one of your agents."))
                                .font(.footnote).foregroundStyle(MycoColor.muted)
                        }
                    } else {
                        Section {
                            Picker(L("开在哪台电脑", "On which computer"), selection: $machine) {
                                ForEach(computers) { one in Text(one.name).tag(one.id) }
                            }
                            Picker(L("用哪个 AI", "Which AI"), selection: $cli) {
                                Text("Claude Code").tag("claude")
                                Text("Codex").tag("codex")
                            }
                        } footer: {
                            Text(L("它在那台电脑的 \(SuperagentModel.dir) 里干活。", "It works in \(SuperagentModel.dir) on that computer."))
                        }
                    }
                } else if mine.isEmpty {
                    Section {
                        Text(L("你还没有自己的 agent。", "You have no agents of your own yet.")).font(.footnote).foregroundStyle(MycoColor.muted)
                    }
                } else {
                    Section(L("让哪个 agent 当 superagent", "Which agent")) {
                        ForEach(mine) { agent in
                            Button {
                                pick = agent.name
                            } label: {
                                HStack {
                                    Text(session.displayName(of: agent.name)).foregroundStyle(MycoColor.fg)
                                    if agent.name == model.agent {
                                        Text(L("现在是它", "Current")).font(.caption).foregroundStyle(MycoColor.muted)
                                    }
                                    Spacer()
                                    if pick == agent.name { Image(systemName: "checkmark").foregroundStyle(MycoColor.accent) }
                                }
                            }
                            .accessibilityIdentifier("superagent.pick")
                        }
                    }
                }
                if let failure = model.failure {
                    Section { FailureLine(failure: failure) }
                }
                if model.agent != nil {
                    Section {
                        Button(L("不要 superagent 了", "Remove superagent"), role: .destructive) {
                            Task { if await model.set(nil) { dismiss() } }
                        }
                        .disabled(model.busy != nil)
                    }
                }
            }
            .navigationTitle(model.agent == nil ? L("配一个 superagent", "Set up your superagent") : L("换一个 superagent", "Change your superagent"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L("取消", "Cancel")) { dismiss() }.disabled(model.busy != nil)
                }
                ToolbarItem(placement: .confirmationAction) {
                    if model.busy != nil {
                        ProgressView()
                    } else {
                        Button(startNew ? L("开一个 \(SuperagentModel.shroom)", "Start \(SuperagentModel.shroom)") : L("就用它", "Use it")) {
                            Task {
                                let name: String? = startNew
                                    ? await model.start(on: machine, cli: cli)
                                    : (await model.set(pick) ? pick : nil)
                                if let name {
                                    dismiss()
                                    try? await Task.sleep(for: .milliseconds(400))
                                    await session.refresh()
                                    router.chat(with: name)
                                }
                            }
                        }
                        .fontWeight(.semibold)
                        .disabled(startNew ? machine.isEmpty : pick.isEmpty)
                        .accessibilityIdentifier("superagent.go")
                    }
                }
            }
            .onAppear {
                if machine.isEmpty { machine = computers.first?.id ?? "" }
                if pick.isEmpty { pick = model.agent ?? "" }
                if computers.isEmpty, !mine.isEmpty { startNew = false }
            }
        }
        .presentationDetents([.large])
        .interactiveDismissDisabled(model.busy != nil)
    }
}
