import Foundation
import MycoAPI

/// Adding a computer with one command: run on that computer, in the
/// folder agents should use, it installs myco, joins the workspace and
/// makes that folder a computer for them. Running it there, in that
/// folder, is the consent: no path to type, nothing to allow. The desktop
/// app's "加一台电脑".
public enum ComputerCommand {
    /// What the other computer runs.
    public enum System: String, Sendable, Hashable, CaseIterable {
        /// Mac, Linux, WSL.
        case shell
        /// Windows PowerShell.
        case powershell
    }

    public static func line(for link: JoinLink, on system: System) -> String {
        switch system {
        case .shell: return "curl -fsSL \(link.joinUrl) | bash"
        case .powershell: return "irm '\(link.joinUrl)?format=ps1' | iex"
        }
    }

    /// A name nothing goes by. A computer must name at least one agent
    /// that may use it; one added outside a project names this, until it
    /// is put in a project.
    public static func nobody() -> String {
        let letters = Array("abcdefghijklmnopqrstuvwxyz0123456789")
        return "nobody-" + String((0..<8).map { _ in letters.randomElement() ?? "a" })
    }

    public static func isNobody(_ name: String) -> Bool {
        name.range(of: "^nobody-[a-z0-9]{8}$", options: .regularExpression) != nil
    }
}

extension WorkspaceSession {
    /// The folders on computers that agents run commands in.
    public var computers: [Agent] {
        statuses.map(\.agent).filter { Naming.kind(of: $0) == .shell }
    }

    /// Who may use a computer added to this project: its agents.
    public func computerUsers(of project: String?) -> [String] {
        guard let project, let members = self.project(project)?.members else { return [] }
        return members.map(\.name).filter { name in agent(name).map { Naming.kind(of: $0) == .agent } ?? false }
    }

    /// A one-off link whose command makes the folder it is run in a
    /// computer for this project's agents (for nobody yet, without one).
    public func computerLink(for project: String?) async throws -> JoinLink {
        let users = computerUsers(of: project)
        return try await hub.computerLink(allowFrom: users.isEmpty ? [ComputerCommand.nobody()] : users)
    }

    /// The computer that showed up since: the folder the command was run in.
    public func computer(newSince before: Set<String>) -> Agent? {
        computers.first { !before.contains($0.name) }
    }

    /// Puts a node in a project. Answers why it could not, if it could not.
    public func add(_ agent: String, toProject project: String) async -> Failure? {
        do {
            try await hub.addProjectMember(agent, to: project)
            await refresh()
            return nil
        } catch {
            return Failure.of(error)
        }
    }
}
