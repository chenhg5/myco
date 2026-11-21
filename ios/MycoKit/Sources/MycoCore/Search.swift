import Foundation
import MycoAPI

/// Finding things (the desktop app's ⌘K and ⌘F): in a conversation, the
/// items whose words contain what is typed; across the workspace, members,
/// projects and the messages of my conversations.
public enum ConversationSearch {
    /// The words an item of a conversation shows, for finding in it.
    public static func text(of item: ConversationItem) -> String? {
        switch item.kind {
        case .mine(let text, _, _): return text
        case .reply(let text, _, _): return text
        case .progress(let text, _): return text
        case .pastProgress(let texts): return texts.joined(separator: "\n")
        case .takenOver(let text): return text
        case .failure(let reason): return reason
        default: return nil
        }
    }

    /// The items that contain it, in order (oldest first).
    public static func hits(in items: [ConversationItem], query: String) -> [String] {
        let wanted = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !wanted.isEmpty else { return [] }
        return items.compactMap { item in
            text(of: item).flatMap { $0.localizedCaseInsensitiveContains(wanted) ? item.id : nil }
        }
    }

    /// A line of a message around the first place it says it: a little
    /// before, more after, on one line.
    public static func snippet(_ body: String, query: String) -> String {
        // (as it reads, without Markdown's marks)
        let plain = body.replacingOccurrences(of: "**", with: "").replacingOccurrences(of: "__", with: "")
            .replacingOccurrences(of: "`", with: "")
            .replacingOccurrences(of: #"(?m)^\s{0,3}#{1,6}\s+"#, with: "", options: .regularExpression)
        let flat = plain.split(whereSeparator: \.isWhitespace).joined(separator: " ")
        guard let range = flat.range(of: query, options: [.caseInsensitive, .diacriticInsensitive]) else {
            return String(flat.prefix(90))
        }
        let start = flat.index(range.lowerBound, offsetBy: -24, limitedBy: flat.startIndex) ?? flat.startIndex
        let end = flat.index(range.upperBound, offsetBy: 60, limitedBy: flat.endIndex) ?? flat.endIndex
        return (start > flat.startIndex ? "…" : "") + flat[start..<end] + (end < flat.endIndex ? "…" : "")
    }
}

/// What the search box of the chats page finds across the workspace.
@MainActor
@Observable
public final class SearchModel {
    /// A message of one of my conversations that says it.
    public struct MessageHit: Identifiable, Hashable, Sendable {
        /// The message.
        public var id: String
        /// Whom the conversation is with.
        public var party: String
        public var from: String
        public var snippet: String
        public var at: Date
    }

    private let session: WorkspaceSession

    /// What was looked for last, and what it found.
    public private(set) var query = ""
    public private(set) var messages: [MessageHit] = []
    public private(set) var searching = false
    public private(set) var failure: Failure?

    public init(session: WorkspaceSession) {
        self.session = session
    }

    /// The members whose name, display name or role says it.
    public func members(_ query: String) -> [AgentStatus] {
        let wanted = query.trimmingCharacters(in: .whitespaces)
        guard !wanted.isEmpty else { return [] }
        return session.members.filter { status in
            [session.displayName(of: status.agent.name), status.agent.name, status.agent.description]
                .contains { $0.localizedCaseInsensitiveContains(wanted) }
        }
    }

    /// The projects whose title, name or goal says it.
    public func projects(_ query: String) -> [Project] {
        let wanted = query.trimmingCharacters(in: .whitespaces)
        guard !wanted.isEmpty else { return [] }
        return session.projects.filter { project in
            [project.title, project.name, project.goal ?? ""].contains { $0.localizedCaseInsensitiveContains(wanted) }
        }
    }

    /// Looks for it in my conversations' messages: the workspace says
    /// which conversations have it; each one's latest page says where.
    public func search(_ query: String) async {
        let wanted = query.trimmingCharacters(in: .whitespacesAndNewlines)
        self.query = wanted
        guard wanted.count >= 2 else {
            messages = []
            searching = false
            return
        }
        searching = true
        defer { if self.query == wanted { searching = false } }
        let me = session.me
        do {
            let items = try await session.hub.recent(.search(wanted))
            guard self.query == wanted else { return }
            let hub = session.hub
            var found: [MessageHit] = []
            await withTaskGroup(of: [MessageHit].self) { group in
                for item in items.prefix(10) {
                    guard let party = item.participants.first(where: { $0 != me }) else { continue }
                    group.addTask {
                        let page = try? await hub.exchanges(between: me, and: party, before: nil)
                        var said = (page?.messages ?? []).filter { $0.body.localizedCaseInsensitiveContains(wanted) }
                        if said.isEmpty {
                            // (older than the latest page: what the list knows of it)
                            said = [item.answer, item.task, item.last, item.first, item.open].compactMap { $0 }
                                .filter { $0.body.localizedCaseInsensitiveContains(wanted) }
                        }
                        return said.sorted { $0.createdAt > $1.createdAt }.prefix(3).map { message in
                            MessageHit(id: message.id, party: party, from: message.from,
                                       snippet: ConversationSearch.snippet(message.body, query: wanted), at: message.createdAt)
                        }
                    }
                }
                for await hits in group { found += hits }
            }
            guard self.query == wanted else { return }
            var seen = Set<String>()
            messages = found.sorted { $0.at > $1.at }.filter { seen.insert($0.id).inserted }.prefix(20).map { $0 }
            failure = nil
        } catch is CancellationError {
            return
        } catch {
            guard self.query == wanted else { return }
            failure = Failure.of(error, canRetry: true)
            messages = []
        }
    }
}
