import Foundation
import MycoAPI

/// One row of the Chats tab: a project (the conversation with its
/// coordinator) or an agent talked to directly.
public struct ChatRow: Identifiable, Hashable, Sendable {
    public enum Target: Hashable, Sendable {
        case project(String)
        case agent(String)
    }

    public var target: Target
    public var title: String
    public var subtitle: String
    public var updatedAt: Date?
    /// The newest open task's state; nil when nothing is open.
    public var state: RunState?
    /// The agent whose face the row shows (a project shows its coordinator).
    public var face: Agent?
    public var away: Bool
    /// The agent on the other side (a project's: its coordinator).
    public var party: String = ""
    /// Something came from the other side since this conversation was last looked at.
    public var unread = false

    public var id: String {
        switch target {
        case .project(let name): return "project:" + name
        case .agent(let name): return "agent:" + name
        }
    }
}

extension WorkspaceSession {
    /// Everything I talk to directly, most recent first: each project (its
    /// coordinator's conversation) and each agent I exchanged messages with.
    public var chatRows: [ChatRow] {
        let now = Date()
        // Sort my threads: one with a project's coordinator belongs to the
        // project's row, any other to the agent on the other side.
        var byProject: [String: [RecentItem]] = [:]
        var byAgent: [String: [RecentItem]] = [:]
        let leads = Dictionary(uniqueKeysWithValues: projects.compactMap { p in coordinator(of: p).map { (p.name, $0) } })
        // An agent that coordinates a project is reached through that
        // project's row, whichever thread the words were said in.
        var projectOfLead: [String: String] = [:]
        for project in projects {
            if let lead = leads[project.name], projectOfLead[lead] == nil { projectOfLead[lead] = project.name }
        }
        for item in recent where item.participants.contains(me) {
            for other in item.participants where other != me {
                if let project = projectOfLead[other] {
                    byProject[project, default: []].append(item)
                } else {
                    byAgent[other, default: []].append(item)
                }
            }
        }

        var rows: [ChatRow] = []
        for project in projects {
            guard let lead = leads[project.name] else { continue }
            let mine = byProject[project.name] ?? []
            let newest = mine.max { $0.updatedAt < $1.updatedAt }
            let working = memberStatuses(of: project).filter { $0.state == "working" }.count
            var subtitle = newest.map { ConversationBuilder.firstLine(of: ($0.answer ?? $0.last).body) } ?? (project.goal ?? "")
            if working > 0 {
                subtitle = L("\(working) 个成员在干活", "\(working) at work")
            }
            rows.append(ChatRow(
                target: .project(project.name), title: project.title.isEmpty ? project.name : project.title,
                subtitle: subtitle, updatedAt: newest?.updatedAt ?? project.updatedAt,
                state: working > 0 ? .working(since: now) : openState(in: mine, now: now),
                face: agent(lead), away: false, party: lead, unread: isUnread(lead, in: mine)))
        }
        for (name, items) in byAgent {
            guard let status = status(of: name), Naming.kind(of: status.agent) == .agent else { continue }
            let newest = items.max { $0.updatedAt < $1.updatedAt }
            let open = openState(in: items.filter { $0.open?.to == name || $0.task?.to == name }, now: now)
            var subtitle = newest.map { ConversationBuilder.firstLine(of: ($0.answer ?? $0.last).body) } ?? status.agent.description
            if case .working = open, let text = status.agent.statusText, !text.isEmpty { subtitle = text }
            rows.append(ChatRow(
                target: .agent(name), title: displayName(of: name), subtitle: subtitle,
                updatedAt: newest?.updatedAt, state: open, face: status.agent, away: status.state == "offline",
                party: name, unread: isUnread(name, in: items)))
        }
        return rows.sorted { ($0.updatedAt ?? .distantPast, $0.id) > ($1.updatedAt ?? .distantPast, $1.id) }
    }

    /// Unread: the newest thing in the conversation came from the other
    /// side (an interim update does not count), after it was last looked at.
    private func isUnread(_ party: String, in items: [RecentItem]) -> Bool {
        let incoming = items.map(\.last).filter { $0.from != me && $0.type != "progress" }.map(\.createdAt).max()
        guard let incoming, let seen = readAt[party] ?? readBaseline else { return false }
        return incoming > seen
    }

    private func openState(in items: [RecentItem], now: Date) -> RunState? {
        let states = items.compactMap { item -> RunState? in
            if let open = item.open { return RunState.of(open, recipient: status(of: open.to), now: now) }
            if item.state == "failed", let task = item.task { return RunState.of(task, recipient: nil, now: now) }
            return nil
        }
        // What most needs a look comes first.
        func rank(_ s: RunState) -> Int {
            switch s {
            case .maybeStuck: return 0
            case .failed: return 1
            case .working: return 2
            case .queued: return 3
            case .done, .stopped: return 4
            }
        }
        return states.min { rank($0) < rank($1) }
    }
}
