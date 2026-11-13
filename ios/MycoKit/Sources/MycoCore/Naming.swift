import Foundation
import MycoAPI

/// What a node is. `machine` is a machine node, never shown as a member.
public enum NodeKind: String, Sendable {
    case agent, shell, human, machine
}

/// What nodes are called in the app. A port of `nodeName()` in
/// web/src/app/nodes.ts: the two must give the same name for the same node.
public enum Naming {
    public static func kind(of agent: Agent) -> NodeKind {
        if let k = agent.kind, let kind = NodeKind(rawValue: k) { return kind }
        switch agent.binding.harness ?? "" {
        case "machine": return .machine
        case "shell", "exec": return .shell
        case "human": return .human
        default: return .agent
        }
    }

    /// The CLI a node runs ("claude", "codex", "claude-cloud", ...).
    public static func cli(of agent: Agent) -> String {
        agent.runtime?.cli ?? agent.binding.harness ?? ""
    }

    /// Its display name, or else one made from what it is: an agent by the
    /// folder it works in ("myco 助手"), a computer by which one and the
    /// folder ("Studio Mac · myco"). A name someone chose stays.
    public static func name(of agent: Agent, among agents: [Agent], machines: [Machine]) -> String {
        if let display = agent.displayName, !display.isEmpty { return display }
        let kind = Naming.kind(of: agent)
        guard kind == .agent || kind == .shell else { return agent.name }
        let machine = machines.first { $0.id == agent.machineId }
        guard isGenerated(agent.name, hostname: machine?.hostname) else { return agent.name }
        let base = friendlyName(of: agent, machine: machine?.name)
        // Several of the same: the second is 二号, the third 三号, in the
        // order they joined.
        let same = agents
            .filter { ($0.displayName ?? "").isEmpty && self.kind(of: $0) == kind }
            .filter { other in
                friendlyName(of: other, machine: machines.first { $0.id == other.machineId }?.name) == base
            }
            .sorted { ($0.createdAt, $0.name) < ($1.createdAt, $1.name) }
        guard let index = same.firstIndex(where: { $0.name == agent.name }), index > 0 else { return base }
        return "\(base) \(ordinal(index + 1))"
    }

    static func friendlyName(of agent: Agent, machine: String?) -> String {
        let dir = agent.binding.workdir ?? ""
        let trimmed = dir.replacingOccurrences(of: "[\\\\/]+$", with: "", options: .regularExpression)
        let folder = trimmed.split(whereSeparator: { $0 == "/" || $0 == "\\" }).last.map(String.init) ?? ""
        let place = folder == "~" ? "" : folder
        if kind(of: agent) == .shell {
            return [machine ?? L("电脑", "Computer"), place].filter { !$0.isEmpty }.joined(separator: " · ")
        }
        return place.isEmpty ? L("助手", "Assistant") : L("\(place) 助手", "\(place) assistant")
    }

    static func ordinal(_ n: Int) -> String {
        let zh = ["", "一", "二", "三", "四", "五", "六", "七", "八", "九", "十"]
        return L(n <= 10 ? "\(zh[n])号" : "\(n)号", "#\(n)")
    }

    /// A name made up when the node joined (host name, CLI, a random
    /// suffix), not one a person chose.
    static func isGenerated(_ name: String, hostname: String?) -> Bool {
        let slug = (hostname ?? "").lowercased()
            .replacingOccurrences(of: "\\.local$", with: "", options: .regularExpression)
            .replacingOccurrences(of: "[^a-z0-9]+", with: "-", options: .regularExpression)
            .replacingOccurrences(of: "^-|-$", with: "", options: .regularExpression)
        if slug.count >= 4, name.lowercased().hasPrefix(slug) { return true }
        let pattern = "-(claude|codex|gemini|cursor|opencode|pi|kiro|exec|shell|agent|inbox)-[0-9a-f]{3,}$"
        return name.range(of: pattern, options: [.regularExpression, .caseInsensitive]) != nil
    }
}
