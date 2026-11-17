import SwiftUI
import UIKit
import MycoAPI
import MycoCore
import MycoUI

/// Adding a computer from the phone: one command to run on that computer,
/// in the folder agents should use. The page waits for the folder to show
/// up, ticks it off and puts it in the project chosen. The desktop app's
/// "加一台电脑".
struct AddComputerSheet: View {
    /// The project it was opened from, if any.
    let project: String?
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session

    /// The project its agents may use it in; "" for none.
    @State private var chosen = ""
    @State private var system = ComputerCommand.System.shell
    @State private var link: JoinLink?
    @State private var failure: Failure?
    /// Only the workspace's owner or an admin may make such a link.
    @State private var denied = false
    @State private var attempt = 0
    /// The computers that were here before the command was given.
    @State private var before: Set<String> = []
    @State private var arrived: Agent?
    @State private var placing: Failure?
    @State private var copied = false

    private var command: String {
        link.map { ComputerCommand.line(for: $0, on: system) } ?? ""
    }

    private func title(of name: String) -> String {
        session.project(name).map { $0.title.isEmpty ? $0.name : $0.title } ?? name
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack {
                Text(project == nil ? L("加一台电脑", "Add a computer") : L("给项目加一台电脑", "Add a computer to the project"))
                    .font(.title3.weight(.semibold)).foregroundStyle(MycoColor.fg)
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
            Text(L("一台电脑上的一个文件夹，agent 在里面跑命令。在那台电脑上打开终端，进到要给 agent 用的文件夹，运行这一行：",
                   "A folder on a computer, for agents to run commands in. On that computer, open a terminal in the folder agents should use, and run:"))
                .font(.subheadline).lineSpacing(3).foregroundStyle(MycoColor.fg2)
                .fixedSize(horizontal: false, vertical: true)
            if denied {
                Text(L("只有工作区的主人或管理员能用命令加电脑：请他们来加。", "Only the workspace's owner or an admin can add a computer with a command: ask them to."))
                    .font(.subheadline).foregroundStyle(MycoColor.fg)
                    .padding(14)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                    .accessibilityIdentifier("computer.denied")
            } else {
                // Which project's agents may use it: asked where it was not
                // opened from one.
                if project == nil, !session.projects.isEmpty {
                    Picker(L("放进哪个项目", "Which project"), selection: $chosen) {
                        ForEach(session.projects, id: \.name) { project in
                            Text(project.title.isEmpty ? project.name : project.title).tag(project.name)
                        }
                    }
                    .pickerStyle(.menu)
                    .tint(MycoColor.fg)
                    .disabled(arrived != nil)
                    .accessibilityIdentifier("computer.project")
                }
                Picker(L("那台电脑的系统", "That computer's system"), selection: $system) {
                    Text("Mac / Linux / WSL").tag(ComputerCommand.System.shell)
                    Text(L("Windows（PowerShell）", "Windows (PowerShell)")).tag(ComputerCommand.System.powershell)
                }
                .pickerStyle(.segmented)
                .accessibilityIdentifier("computer.system")
                if link == nil, let failure {
                    FailureLine(failure: failure) {
                        self.failure = nil
                        attempt += 1
                    }
                    .padding(14)
                    .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                } else {
                    Group {
                        if link != nil {
                            Text(command).textSelection(.enabled)
                        } else {
                            Text(String(repeating: "curl -fsSL 一条命令 ", count: 3)).redacted(reason: .placeholder)
                        }
                    }
                    .font(.callout.monospaced())
                    .lineSpacing(3)
                    .foregroundStyle(MycoColor.fg)
                    .padding(14)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                    .accessibilityIdentifier("computer.command")
                }
                standing
                HStack(spacing: 10) {
                    Button(copied ? L("已复制", "Copied") : L("复制", "Copy")) {
                        UIPasteboard.general.string = command
                        copied = true
                    }
                    .buttonStyle(PrimaryButtonStyle())
                    ShareLink(item: command) {
                        Text(L("发到电脑上", "Send it over")).font(.body.weight(.semibold)).foregroundStyle(MycoColor.fg)
                            .frame(maxWidth: .infinity).padding(.vertical, 13)
                            .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                    }
                }
                .disabled(link == nil)
                .opacity(link == nil ? 0.4 : 1)
            }
        }
        .padding(.horizontal, 20)
        .padding(.top, 26)
        .padding(.bottom, 12)
        .background(MycoColor.bg)
        .fittedSheet()
        .sensoryFeedback(.success, trigger: copied) { _, now in now }
        .sensoryFeedback(.success, trigger: arrived?.name) { _, now in now != nil }
        .onAppear {
            chosen = project ?? session.projects.first?.name ?? ""
        }
        // A link for the project chosen (a new one when another is chosen,
        // or "重试" is tapped): its agents are the ones it lets in.
        .task(id: chosen + "|\(attempt)") {
            guard arrived == nil else { return }
            link = nil
            copied = false
            do {
                before = Set(session.computers.map(\.name))
                link = try await session.computerLink(for: chosen.isEmpty ? nil : chosen)
            } catch let error as HubError where error.error == "permission_denied" {
                denied = true
            } catch is CancellationError {
                // the sheet was closed
            } catch {
                failure = Failure.of(error, canRetry: true)
            }
        }
        // Looks often while it waits: the folder shows up when the
        // command has run.
        .task(id: link) {
            guard link != nil else { return }
            while !Task.isCancelled, arrived == nil {
                try? await Task.sleep(for: .seconds(2))
                await session.refresh()
                guard let fresh = session.computer(newSince: before) else { continue }
                arrived = fresh
                if !chosen.isEmpty { placing = await session.add(fresh.name, toProject: chosen) }
            }
        }
    }

    /// Under the command: waiting for it to be run, or the folder that came.
    @ViewBuilder private var standing: some View {
        if let arrived {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Image(systemName: "checkmark.circle.fill").foregroundStyle(MycoColor.green)
                Text(arrivedWords(arrived)).font(.footnote).foregroundStyle(MycoColor.fg)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("computer.arrived")
            }
            if let placing {
                FailureLine(failure: placing)
            }
        } else if link != nil {
            HStack(spacing: 8) {
                ProgressView().controlSize(.small)
                Text(L("等它出现…链接只能用一次，24 小时内有效。", "Waiting for it to show up… The link works once, within 24 hours."))
                    .font(.footnote).foregroundStyle(MycoColor.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("computer.waiting")
            }
        }
        if arrived == nil, !denied, chosen.isEmpty {
            Text(L("这个工作区还没有项目：它加进来后，在电脑上的 App 里把它放进一个项目，那个项目的 agent 才能用它。",
                   "This workspace has no project yet: once it is here, put it in a project in the app on a computer, and that project's agents can use it."))
                .font(.footnote).foregroundStyle(MycoColor.muted)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private func arrivedWords(_ agent: Agent) -> String {
        let name = session.displayName(of: agent.name)
        guard !chosen.isEmpty, placing == nil else { return L("加进来了：\(name)", "It is here: \(name)") }
        return L("加进来了：\(name)，已放进「\(title(of: chosen))」。", "It is here: \(name), in \(title(of: chosen)).")
    }
}
