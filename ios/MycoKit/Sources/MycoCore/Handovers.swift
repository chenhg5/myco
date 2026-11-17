import Foundation
import MycoAPI

/// 「换一个来接着做」: another session takes an agent over. Its name, its
/// conversations and its projects stay; the session behind it changes, and
/// the old one stops (it can be switched back to). Two ways: a new session
/// on one of my computers, or a one-time command run in any session. The
/// desktop app's `Handover.tsx`.
public enum HandoverWords {
    /// A CLI's name for people.
    static func cli(_ cli: String) -> String {
        switch cli {
        case "claude": return "Claude Code"
        case "codex": return "Codex"
        default: return cli
        }
    }

    /// One side of a handover for people: "Studio Mac · Codex", "claude.ai 云端".
    public static func side(_ side: HandoverSide?, machines: [Machine]) -> String {
        guard let side else { return "" }
        var cli = side.runtime?.cli ?? ""
        if cli.isEmpty { cli = side.binding?.harness ?? "" }
        var machine = side.machineName ?? ""
        if machine.isEmpty, let id = side.machineId { machine = machines.first { $0.id == id }?.name ?? "" }
        if !machine.isEmpty { return [machine, self.cli(cli)].filter { !$0.isEmpty }.joined(separator: " · ") }
        if cli.hasSuffix("-cloud") { return cli.hasPrefix("codex") ? L("Codex 云端", "Codex cloud") : L("claude.ai 云端", "claude.ai cloud") }
        if cli == "inbox" { return L("一个开着的会话", "an open session") }
        let said = [self.cli(cli), side.binding?.environment ?? ""].filter { !$0.isEmpty }.joined(separator: " · ")
        return said.isEmpty ? L("另一个会话", "another session") : said
    }

    /// Who started it, for people. `me`: my console address.
    public static func by(_ by: String?, me: String) -> String {
        guard let by, !by.isEmpty else { return "" }
        if by == "admin" { return L("管理员", "an admin") }
        if "@console:" + by == me { return L("你", "you") }
        return by
    }

    /// The line a conversation shows where a handover took effect:
    /// "换成 Studio Mac · Codex 接着做（你换的）".
    public static func line(_ handover: Handover, machines: [Machine], me: String) -> String {
        let to = side(handover.to, machines: machines)
        let who = by(handover.by, me: me)
        let what = (handover.back ?? "").isEmpty
            ? L("换成 \(to) 接着做", "\(to) carries on from here")
            : L("换回原来的会话 · \(to)", "Switched back to the earlier session · \(to)")
        return what + (who.isEmpty ? "" : L("（\(who)换的）", " (by \(who))"))
    }

    /// A handover that is not over: "等它接手", "没用上"; nil for one that is done.
    public static func state(_ handover: Handover) -> String? {
        switch handover.state {
        case "waiting": return L("等它接手", "Waiting for it")
        case "expired": return L("没用上", "Not used")
        default: return nil
        }
    }

    /// Whether a done handover's earlier session can be switched back to:
    /// not already undone, and not the session answering now.
    public static func canGoBack(_ handover: Handover, among all: [Handover], agent: Agent) -> Bool {
        guard handover.state == "done" else { return false }
        if all.contains(where: { $0.back == handover.id && $0.state == "done" }) { return false }
        guard let earlier = handover.from?.binding?.sessionId, !earlier.isEmpty else { return false }
        return earlier != agent.binding.sessionId
    }

    /// What will happen, said before it does.
    public static func consequences(back: Bool, open: Int, working: Bool) -> [String] {
        var lines: [String] = []
        if back {
            lines.append(L("现在回答它的会话会停下，不再收消息。", "The session answering it now stops and gets no more messages."))
        } else {
            lines.append(L("名字、聊天记录和项目都不变。新的会话先读一份交接说明：最近 30 条对话和还没做完的事。",
                           "Its name, chats and projects stay. The new session first reads a handover note: the last 30 messages and what is not done yet."))
            lines.append(L("原来的会话会停下，不再收消息；之后在「接手记录」里可以换回来。",
                           "The earlier session stops and gets no more messages; you can switch back under Handovers."))
        }
        if open > 0 {
            lines.append(L("它手上没做完的 \(open) 件事，交给\(back ? "换回去的" : "新的")会话重新做。",
                           "The \(open) task\(open > 1 ? "s" : "") it has not finished go to the \(back ? "earlier" : "new") session to redo."))
        }
        if working { lines.append(L("它现在正在干活，那件事也会交过去重新做。", "It is at work right now; that task goes over too.")) }
        return lines
    }

    /// Whether a computer's myco can take an agent over: newer than
    /// v0.19.22 (a later release, or a build from a later main).
    public static func canTakeOver(mycoVersion version: String?) -> Bool {
        guard let version, !version.isEmpty else { return false }
        func atLeast(_ want: [Int]) -> Bool {
            let parts = version.replacingOccurrences(of: "^[vV]", with: "", options: .regularExpression)
                .split(whereSeparator: { ".+-".contains($0) }).map { Int($0) ?? 0 }
            for index in 0..<max(parts.count, want.count) {
                let have = index < parts.count ? parts[index] : 0, need = index < want.count ? want[index] : 0
                if have != need { return have > need }
            }
            return true
        }
        if atLeast([0, 19, 23]) { return true }
        return version.range(of: "-g[0-9a-f]{4,}$", options: [.regularExpression, .caseInsensitive]) != nil && atLeast([0, 19, 22])
    }
}

extension WorkspaceSession {
    /// The computers that can start a session, mine first as they came.
    public var spawners: [Machine] {
        machines.filter { $0.canSpawn == true }
    }

    /// The folders agents already work in on a computer, most used first.
    public func folders(on machineID: String) -> [String] {
        var counts: [String: Int] = [:]
        for agent in statuses.map(\.agent) where agent.machineId == machineID {
            if let dir = agent.binding.workdir, !dir.isEmpty { counts[dir, default: 0] += 1 }
        }
        return counts.sorted { ($0.value, $1.key) > ($1.value, $0.key) }.map(\.key)
    }
}

/// An agent's handovers, and handing it over: one per agent, for as long
/// as the workspace is on screen.
@MainActor
@Observable
public final class HandoversModel {
    private let hub: any HubAPI
    public let agent: String

    /// Newest first; empty where there are none, or none may be seen.
    public private(set) var list: [Handover] = []
    /// The workspace is from before handovers.
    public private(set) var unsupported = false
    private var readAt: Date?

    static let freshFor: TimeInterval = 30

    public init(hub: any HubAPI, agent: String) {
        self.hub = hub
        self.agent = agent
    }

    /// Reads them, unless they were read a moment ago.
    public func load(force: Bool = false, now: Date = Date()) async {
        if !force, let readAt, now.timeIntervalSince(readAt) < Self.freshFor { return }
        do {
            let read = try await hub.handovers(of: agent)
            if read != list { list = read }
            unsupported = false
            readAt = now
        } catch let error as HubError where Failure.isTooOld(error) {
            unsupported = true
            readAt = now
        } catch let error as HubError where error.error == "not_found" || error.error == "permission_denied" {
            // Not mine to see, or a workspace from before them: none.
            readAt = now
        } catch {
            // Not reachable now: what is known stays.
        }
    }

    /// The ones that took effect, oldest first: where a conversation draws its line.
    public var done: [Handover] {
        list.filter { $0.state == "done" && $0.doneAt != nil }.sorted { ($0.doneAt ?? $0.at) < ($1.doneAt ?? $1.at) }
    }

    /// A new session on a computer takes over (or an earlier one takes it
    /// back). Answers when it has: that takes minutes.
    public func handOver(_ request: HandoverRequest) async throws -> HandoverResult {
        let result = try await hub.handOver(agent, request)
        await load(force: true)
        return result
    }

    /// A one-time command for whichever session should take over.
    public func link(note: String?, back: String?) async throws -> HandoverLink {
        let link = try await hub.handoverLink(of: agent, note: note, back: back)
        await load(force: true)
        return link
    }

    /// Whether the command was run: the handover it made, once it is done.
    public func arrived(_ id: String) async -> Handover? {
        await load(force: true)
        return list.first { $0.id == id && $0.state == "done" }
    }
}
