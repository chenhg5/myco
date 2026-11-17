import SwiftUI
import UIKit
import MycoAPI
import MycoCore
import MycoUI

/// 「换一个来接着做」: another session takes an agent over. Which way (a
/// new session on a computer, or a one-time command for any session),
/// what will happen, then it waits for the new session. With `backID`,
/// the session an earlier handover replaced takes the agent back. The
/// desktop app's `HandoverDialog`.
struct HandoverSheet: View {
    let agent: String
    /// A done handover to undo, by id.
    let backID: String?
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session

    enum Way: Hashable { case machine, link }

    @State private var way = Way.machine
    @State private var machineID = ""
    @State private var cli = "claude"
    @State private var dir = ""
    @State private var note = ""
    /// Since when it is being handed over.
    @State private var busy: Date?
    @State private var failure: Failure?
    @State private var result: HandoverResult?
    @State private var link: HandoverLink?
    @State private var system = ComputerCommand.System.shell
    @State private var arrived: Handover?
    @State private var copied = false
    @State private var prepared = false

    private var model: HandoversModel { session.handovers(of: agent) }
    private var back: Handover? { backID.flatMap { id in model.list.first { $0.id == id } } }
    private var name: String { session.displayName(of: agent) }
    private var status: AgentStatus? { session.status(of: agent) }
    private var machine: Machine? { session.spawners.first { $0.id == machineID } }
    private var machineTooOld: Bool { machine.map { !HandoverWords.canTakeOver(mycoVersion: $0.mycoVersion) } ?? false }
    private var done: Handover? { result?.handover ?? arrived }

    private func said(_ side: HandoverSide?) -> String {
        HandoverWords.side(side, machines: session.machines)
    }

    private var ready: Bool {
        if way == .link { return true }
        if back != nil { return true }
        return machine != nil && !machineTooOld && !dir.trimmingCharacters(in: .whitespaces).isEmpty
    }

    private var command: String {
        guard let link else { return "" }
        return (system == .powershell ? link.commandPs1 : link.command) ?? ""
    }

    var body: some View {
        NavigationStack {
            Form {
                if let done {
                    finished(done)
                } else if let link {
                    commandSection(link)
                } else {
                    choosing
                }
            }
            .navigationTitle(backID == nil ? L("换一个来接着做", "Hand over to another session") : L("换回原来的会话", "Switch back"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(done != nil || link != nil ? L("关闭", "Close") : L("取消", "Cancel")) { dismiss() }
                        .disabled(busy != nil)
                }
            }
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if done == nil, link == nil { startBar }
            }
        }
        .interactiveDismissDisabled(busy != nil)
        .task {
            await model.load()
            guard !prepared else { return }
            prepared = true
            prepare()
        }
        // The command path: looks until whoever runs it has.
        .task(id: link?.handover.id) {
            guard let id = link?.handover.id else { return }
            while !Task.isCancelled, arrived == nil {
                try? await Task.sleep(for: .seconds(3))
                if let taken = await model.arrived(id) {
                    arrived = taken
                    await session.refresh()
                }
            }
        }
    }

    /// A fresh start: its own computer first, the AI it runs now, its folder.
    private func prepare() {
        guard let known = session.agent(agent) else { return }
        let spawners = session.spawners
        let own = spawners.first { $0.id == known.machineId }
        let first = own ?? spawners.first(where: \.online) ?? spawners.first
        if let back {
            way = (back.from?.machineId ?? "").isEmpty ? .link : .machine
        } else {
            way = spawners.isEmpty ? .link : .machine
        }
        machineID = first?.id ?? ""
        cli = Naming.cli(of: known).replacingOccurrences(of: "-cloud", with: "") == "codex" ? "codex" : "claude"
        dir = own != nil ? known.binding.workdir ?? "" : ""
    }

    // MARK: Choosing

    @ViewBuilder private var choosing: some View {
        Section {
            if back == nil {
                Picker(L("怎么接手", "How"), selection: $way) {
                    Text(L("在一台电脑上新开", "On a computer")).tag(Way.machine)
                    Text(L("用一条命令接手", "With a command")).tag(Way.link)
                }
                .pickerStyle(.segmented)
                .disabled(busy != nil)
                .accessibilityIdentifier("handover.way")
            }
        } header: {
            Text(back.map {
                L("让 \(said($0.from)) 那个会话重新回答 \(name)。它自己的上下文还在，只补上它离开期间的对话。",
                  "The session on \(said($0.from)) answers for \(name) again. It still has its own context; it only catches up on what it missed.")
            } ?? L("换一个会话来接着做 \(name)：名字、聊天记录和项目都不变。", "Another session carries on as \(name): its name, chats and projects stay."))
                .textCase(nil).font(.footnote)
        } footer: {
            if way == .link {
                Text(back != nil
                    ? L("会生成一条一次性的命令，到原来那个会话里运行，它就接回来了。", "You get a one-time command; run it in the earlier session and it takes the agent back.")
                    : L("会生成一条一次性的命令。在任何会话里运行它（缺 myco 会先装上），那个会话就接手了，适合换到另一个 claude.ai 账号。",
                        "You get a one-time command. Run it in any session (it installs myco if needed) and that session takes over: the way to move to another claude.ai account."))
            }
        }
        if way == .machine, back == nil {
            if session.spawners.isEmpty {
                Section {
                    Text(L("还没有能新开会话的电脑。可以用一条命令接手，或者先在电脑上的 App 里连上一台。",
                           "No computer can start sessions yet. Take over with a command, or connect one in the app on a computer first."))
                        .font(.subheadline).foregroundStyle(MycoColor.fg2)
                }
            } else {
                Section {
                    ForEach(session.spawners) { computer in
                        Button {
                            machineID = computer.id
                            dir = computer.id == session.agent(agent)?.machineId ? session.agent(agent)?.binding.workdir ?? "" : ""
                        } label: {
                            HStack(spacing: 10) {
                                Image(systemName: "desktopcomputer").foregroundStyle(MycoColor.muted).frame(width: 24)
                                VStack(alignment: .leading, spacing: 1) {
                                    Text(computer.name).foregroundStyle(MycoColor.fg)
                                    if let note = note(of: computer) {
                                        Text(note).font(.footnote).foregroundStyle(MycoColor.muted)
                                    }
                                }
                                Spacer(minLength: 0)
                                if computer.id == machineID {
                                    Image(systemName: "checkmark").font(.body.weight(.semibold)).foregroundStyle(MycoColor.green)
                                }
                            }
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .opacity(computer.online && HandoverWords.canTakeOver(mycoVersion: computer.mycoVersion) ? 1 : 0.6)
                        .accessibilityIdentifier("handover.machine")
                    }
                } header: {
                    Text(L("在哪台电脑上", "On which computer"))
                } footer: {
                    if machineTooOld, let machine {
                        Text(L("\(machine.name) 上的 myco 太旧，还不会接手。在那台电脑上运行 myco upgrade 再来。",
                               "The myco on \(machine.name) is too old to take over. Run myco upgrade there first."))
                            .accessibilityIdentifier("handover.machine.old")
                    }
                }
                Section(L("用哪个 AI", "Which AI")) {
                    Picker(L("用哪个 AI", "Which AI"), selection: $cli) {
                        Text("Claude Code").tag("claude")
                        Text("Codex").tag("codex")
                    }
                    .pickerStyle(.segmented)
                }
                Section {
                    TextField(L("比如 ~/code/我的项目", "e.g. ~/code/my-project"), text: $dir)
                        .font(.body.monospaced())
                        .textInputAutocapitalization(.never).autocorrectionDisabled()
                        .accessibilityIdentifier("handover.dir")
                    let folders = session.folders(on: machineID).filter { $0 != dir.trimmingCharacters(in: .whitespaces) }
                    ForEach(folders.prefix(4), id: \.self) { folder in
                        Button {
                            dir = folder
                        } label: {
                            Label(folder, systemImage: "folder").font(.subheadline.monospaced()).foregroundStyle(MycoColor.fg2).lineLimit(1)
                        }
                    }
                } header: {
                    Text(L("在哪个文件夹干活", "Which folder it works in"))
                }
            }
        }
        Section {
            TextField(L("比如 原来的订阅到期了", "e.g. the old subscription ran out"), text: $note)
                .accessibilityIdentifier("handover.note")
        } header: {
            Text(L("为什么换（可以不填，会写进交接说明）", "Why (optional, goes into the handover note)"))
        }
        Section(L("会发生什么", "What will happen")) {
            let lines = HandoverWords.consequences(
                back: back != nil, open: (status?.inProgress ?? 0) + (status?.held ?? 0), working: status?.state == "working")
            ForEach(lines, id: \.self) { line in
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Circle().fill(MycoColor.faint).frame(width: 5, height: 5).offset(y: -2)
                    Text(line).font(.subheadline).foregroundStyle(MycoColor.fg2)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
        .accessibilityIdentifier("handover.what")
        if let failure {
            Section {
                FailureLine(failure: failure)
            }
        }
    }

    /// What is said under a computer's name.
    private func note(of computer: Machine) -> String? {
        if !HandoverWords.canTakeOver(mycoVersion: computer.mycoVersion) { return L("要先升级", "Needs an update") }
        if !computer.online { return L("不在线", "Away") }
        if computer.id == session.agent(agent)?.machineId { return L("原来那台", "Where it is now") }
        return nil
    }

    /// The button that starts it, and, while a new session starts, how
    /// long that has taken.
    private var startBar: some View {
        VStack(spacing: 8) {
            if let busy, way == .machine {
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    TimelineView(.periodic(from: busy, by: 1)) { moment in
                        Text(L("正在开新的会话，它在读交接说明", "Starting the new session; it is reading the handover note")
                            + " · " + RelativeTime.working(since: busy, now: moment.date))
                            .font(.footnote).foregroundStyle(MycoColor.muted)
                    }
                }
                .accessibilityIdentifier("handover.starting")
            }
            Button(startTitle) { start() }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(busy != nil || !ready)
                .opacity(busy != nil || !ready ? 0.45 : 1)
                .accessibilityIdentifier("handover.start")
        }
        .padding(.horizontal, 20)
        .padding(.top, 10)
        .padding(.bottom, 8)
        .background(.bar)
    }

    private var startTitle: String {
        if way == .link { return busy != nil ? L("正在生成…", "Making it…") : L("生成命令", "Make the command") }
        if busy != nil { return L("正在接手…", "Handing over…") }
        return back != nil ? L("换回去", "Switch back") : L("开始接手", "Start the handover")
    }

    private func start() {
        guard busy == nil, ready else { return }
        busy = Date()
        failure = nil
        let why = note.trimmingCharacters(in: .whitespaces)
        let folder = dir.trimmingCharacters(in: .whitespaces)
        Task {
            defer { busy = nil }
            do {
                if way == .machine {
                    let request = back.map {
                        HandoverRequest(machine: $0.from?.machineId ?? machineID, note: why.isEmpty ? nil : why, back: $0.id)
                    } ?? HandoverRequest(machine: machineID, cli: cli, dir: folder.isEmpty ? nil : folder, note: why.isEmpty ? nil : why)
                    result = try await model.handOver(request)
                    await session.refresh()
                } else {
                    link = try await model.link(note: why.isEmpty ? nil : why, back: back?.id)
                }
            } catch {
                failure = Failure.of(error)
            }
        }
    }

    // MARK: The command

    @ViewBuilder private func commandSection(_ link: HandoverLink) -> some View {
        Section {
            Picker(L("在哪种系统上运行", "Where it runs"), selection: $system) {
                Text(L("Mac / Linux / 云端", "Mac / Linux / cloud")).tag(ComputerCommand.System.shell)
                Text(L("Windows（PowerShell）", "Windows (PowerShell)")).tag(ComputerCommand.System.powershell)
            }
            .pickerStyle(.segmented)
            Text(command).font(.callout.monospaced()).textSelection(.enabled)
                .accessibilityIdentifier("handover.command")
            HStack(spacing: 10) {
                Button(copied ? L("已复制", "Copied") : L("复制", "Copy")) {
                    UIPasteboard.general.string = command
                    copied = true
                }
                .buttonStyle(PrimaryButtonStyle())
                ShareLink(item: command) {
                    Text(L("发过去", "Send it over")).font(.body.weight(.semibold)).foregroundStyle(MycoColor.fg)
                        .frame(maxWidth: .infinity).padding(.vertical, 13)
                        .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                }
                .buttonStyle(.plain)
            }
            HStack(spacing: 8) {
                ProgressView().controlSize(.small)
                Text(L("等它接手…运行完这里会自己打勾。", "Waiting for it… this ticks itself once it has run."))
                    .font(.footnote).foregroundStyle(MycoColor.muted)
            }
            .accessibilityIdentifier("handover.waiting")
        } header: {
            Text(back.map { L("在原来那个会话（\(said($0.from))）里运行这一行：", "Run this in the earlier session (\(said($0.from))):") }
                ?? L("在要接手的会话里运行这一行（比如另一个 claude.ai 账号的云端会话，或者任何一台电脑的终端）：",
                     "Run this in the session that should take over (another claude.ai account's cloud session, or a terminal on any computer):"))
                .textCase(nil).font(.footnote)
        } footer: {
            Text(L("命令 24 小时内有效，只能用一次。它等同于这个 agent 的凭证，别发到别人能看到的地方。",
                   "The command works once, for 24 hours. It is as good as this agent's credential: keep it where only you can see it."))
        }
    }

    // MARK: Done

    @ViewBuilder private func finished(_ handover: Handover) -> some View {
        Section {
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                Image(systemName: "checkmark.circle.fill").foregroundStyle(MycoColor.green)
                let to = said(handover.to)
                Text(L("已接手：现在由 \(to.isEmpty ? "新的会话" : to) 回答 \(name)。原来的会话不会再收到消息。",
                       "Taken over: \(to.isEmpty ? "the new session" : to) answers for \(name) now. The earlier session gets no more messages."))
                    .font(.subheadline).foregroundStyle(MycoColor.fg)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("handover.done")
            }
        }
        if let reply = result?.spawn?.reply, !reply.isEmpty {
            Section(L("它读完交接说明后说：", "After reading the handover note, it said:")) {
                Text(reply).font(.subheadline).foregroundStyle(MycoColor.fg2)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("handover.reply")
            }
        }
        Section {
            Button(L("完成", "Done")) { dismiss() }
                .frame(maxWidth: .infinity)
                .fontWeight(.semibold)
                .foregroundStyle(MycoColor.fg)
                .accessibilityIdentifier("handover.close")
        }
    }
}

/// In an agent's card, for whoever may manage it: hand it over, and the
/// times it was (接手记录), each with a way back where there is one.
struct HandoverCardSections: View {
    let agent: Agent
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        let model = session.handovers(of: agent.name)
        Group {
            if !model.unsupported {
                CardSection(title: L("管理", "Manage")) {
                    Button {
                        router.sheet = .handover(agent: agent.name, back: nil)
                    } label: {
                        HStack {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(L("换一个来接着做", "Hand over to another session")).foregroundStyle(MycoColor.fg)
                                Text(L("另一个会话接手它，名字和聊天记录不变", "Another session takes it over; its name and chats stay"))
                                    .font(.footnote).foregroundStyle(MycoColor.muted)
                            }
                            Spacer()
                            Image(systemName: "chevron.right").font(.footnote.weight(.semibold)).foregroundStyle(MycoColor.faint)
                        }
                        .font(.subheadline)
                        .padding(.vertical, 10)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("member.handover")
                }
            }
            if !model.list.isEmpty {
                CardSection(title: L("接手记录", "Handovers")) {
                    ForEach(model.list) { handover in
                        row(handover, among: model.list)
                    }
                }
            }
        }
        .task { await model.load() }
    }

    private func row(_ handover: Handover, among all: [Handover]) -> some View {
        let from = HandoverWords.side(handover.from, machines: session.machines)
        let to = handover.state == "done" ? HandoverWords.side(handover.to, machines: session.machines) : L("一个新的会话", "a new session")
        let who = HandoverWords.by(handover.by, me: session.me)
        let details = [
            RelativeTime.short(handover.doneAt ?? handover.at),
            who.isEmpty ? "" : L("\(who)换的", "by \(who)"),
            (handover.back ?? "").isEmpty ? "" : L("换回", "switch back"),
            HandoverWords.state(handover) ?? "",
            handover.note ?? "",
        ].filter { !$0.isEmpty }
        return HStack(alignment: .center, spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                Text(from + " → " + to).font(.subheadline).foregroundStyle(MycoColor.fg).lineLimit(2)
                Text(details.joined(separator: " · ")).font(.footnote).foregroundStyle(MycoColor.muted).lineLimit(2)
            }
            Spacer(minLength: 0)
            if HandoverWords.canGoBack(handover, among: all, agent: agent) {
                Button(L("换回这个", "Switch back")) { router.sheet = .handover(agent: agent.name, back: handover.id) }
                    .buttonStyle(QuietButtonStyle())
                    .accessibilityIdentifier("member.handover.back")
            }
        }
        .padding(.vertical, 9)
    }
}
