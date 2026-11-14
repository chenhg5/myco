import Foundation
import MycoAPI

/// What the person chose about being told of new messages. Kept on this
/// device.
public struct NotificationSettings: Codable, Hashable, Sendable {
    /// Tell me when something new comes. Off until the person turns it on
    /// (that is when the system asks for permission).
    public var enabled = false
    /// Also for an agent's interim updates, not only what needs me.
    public var progressToo = false
    /// Quiet hours: nothing is announced between these times of day.
    public var quietHours = false
    /// Minutes from midnight; a start later than the end spans the night.
    public var quietFrom = 23 * 60
    public var quietTo = 8 * 60
    /// Conversations that never announce anything (`muteKey`).
    public var muted: Set<String> = []

    public init() {}

    /// The key a conversation is muted under.
    public static func muteKey(workspace: String, party: String) -> String {
        workspace + "|" + party
    }

    /// Whether the quiet hours cover this moment.
    public func isQuiet(at date: Date, calendar: Calendar = .current) -> Bool {
        guard quietHours, quietFrom != quietTo else { return false }
        let parts = calendar.dateComponents([.hour, .minute], from: date)
        let minute = (parts.hour ?? 0) * 60 + (parts.minute ?? 0)
        // 23:00 to 08:00 spans midnight; 13:00 to 14:00 does not.
        return quietFrom < quietTo ? (quietFrom..<quietTo).contains(minute) : (minute >= quietFrom || minute < quietTo)
    }
}

/// One thing worth telling the person about: the newest message from the
/// other side of a conversation.
public struct Notice: Identifiable, Hashable, Sendable {
    public enum Kind: Hashable, Sendable {
        /// A task of mine was answered.
        case answered
        /// A task of mine did not get done.
        case failed
        /// The agent set a task aside to ask me something.
        case asking
        /// It said something on its own.
        case said
        /// An interim update.
        case progress
    }

    /// The message's id: a message is announced once.
    public var id: String
    public var workspace: String
    /// The agent on the other side.
    public var party: String
    /// The page it leads to.
    public var target: ChatRow.Target
    /// Who it is from, as the list calls the conversation.
    public var title: String
    /// What it says: its first line.
    public var body: String
    public var kind: Kind
    public var at: Date
}

extension WorkspaceSession {
    /// The newest message from the other side of each conversation that
    /// came after the conversation was last looked at. What was there when
    /// this device first saw the workspace is not news.
    public var notices: [Notice] {
        chatRows.compactMap { row -> Notice? in
            guard let message = row.incoming, let seen = readAt[row.party] ?? readBaseline, message.createdAt > seen else { return nil }
            let kind: Notice.Kind
            switch message.type {
            case "reply": kind = .answered
            case "event": kind = .failed
            case "progress": kind = message.ask == true ? .asking : .progress
            default: kind = .said
            }
            return Notice(
                id: message.id, workspace: id, party: row.party, target: row.target, title: row.title,
                body: ConversationBuilder.firstLine(of: message.body), kind: kind, at: message.createdAt)
        }
    }
}

/// Shows notifications: the system's, or a list in tests.
@MainActor
public protocol Notifier: AnyObject {
    /// Whether the person allowed notifications (asking them if they were
    /// never asked and `asking` is true).
    func authorized(asking: Bool) async -> Bool
    /// - Parameter workspaceName: said with it when the person has more
    ///   than one workspace; nil otherwise.
    func post(_ notice: Notice, workspaceName: String?) async
    /// The number on the app's icon.
    func setBadge(_ count: Int) async
}

/// Decides what is announced: each message once, not what is on screen,
/// not what the person muted, not during quiet hours, and interim updates
/// only on request.
@MainActor
public final class NotificationDesk {
    private let notifier: any Notifier
    /// Messages already announced (or deliberately not), by workspace.
    private var told: [String: Set<String>] = [:]
    public var settings: NotificationSettings
    /// The conversation being looked at now: nothing in it is announced.
    public var onScreen: (workspace: String, party: String)?
    /// The person has more than one workspace: a notification says which.
    public var severalWorkspaces = false
    private let now: () -> Date

    /// How many announced messages are remembered for a workspace.
    static let remembered = 300

    public init(notifier: any Notifier, settings: NotificationSettings = NotificationSettings(), now: @escaping () -> Date = Date.init) {
        self.notifier = notifier
        self.settings = settings
        self.now = now
    }

    /// Looks at a workspace after it refreshed and announces what is new.
    /// - Parameter active: the app is in front (so "on screen" means seen).
    public func check(_ session: WorkspaceSession, active: Bool) async {
        let notices = session.notices
        var known = told[session.id]
        if known == nil {
            known = await load(session)
            // The first look at a workspace: what is unread now was there
            // before notifications were; it is not announced.
            if known == nil {
                told[session.id] = Set(notices.map(\.id))
                await save(session)
                return
            }
        }
        var seen = known ?? []
        let fresh = notices.sorted(by: { $0.at < $1.at }).filter { !seen.contains($0.id) }
        seen.formUnion(fresh.map(\.id))
        // Only the newest are remembered: older ones are long read.
        if seen.count > Self.remembered {
            seen = Set(notices.map(\.id)).union(seen.prefix(Self.remembered))
        }
        // Noted before anything is shown: a second look that starts
        // meanwhile does not announce the same message again.
        told[session.id] = seen
        guard !fresh.isEmpty else { return }
        await save(session)
        for notice in fresh where announces(notice, active: active) {
            await notifier.post(notice, workspaceName: severalWorkspaces ? session.name : nil)
        }
    }

    /// Whether a new message is announced, by what the person chose.
    func announces(_ notice: Notice, active: Bool) -> Bool {
        guard settings.enabled else { return false }
        if settings.muted.contains(NotificationSettings.muteKey(workspace: notice.workspace, party: notice.party)) { return false }
        if notice.kind == .progress, !settings.progressToo { return false }
        if active, let onScreen, onScreen.workspace == notice.workspace, onScreen.party == notice.party { return false }
        return !settings.isQuiet(at: now())
    }

    /// Forgets a workspace that was left.
    public func forget(_ workspace: String) {
        told[workspace] = nil
    }

    private func load(_ session: WorkspaceSession) async -> Set<String>? {
        guard let data = await session.snapshots?.snapshot(of: session.id, name: "notified"),
              let ids = try? JSONDecoder().decode([String].self, from: data)
        else { return nil }
        return Set(ids)
    }

    private func save(_ session: WorkspaceSession) async {
        guard let ids = told[session.id] else { return }
        await session.snapshots?.setSnapshot(try? JSONEncoder().encode(Array(ids)), of: session.id, name: "notified")
    }
}

/// A notifier that only remembers what it was asked to show: for tests,
/// and for seeing the settings without the system's permission prompt.
@MainActor
public final class RecordingNotifier: Notifier {
    public private(set) var posted: [Notice] = []
    public private(set) var workspaces: [String?] = []
    public private(set) var badge = 0
    public var allowed: Bool

    public init(allowed: Bool = true) {
        self.allowed = allowed
    }

    public func authorized(asking: Bool) async -> Bool { allowed }

    public func post(_ notice: Notice, workspaceName: String?) async {
        posted.append(notice)
        workspaces.append(workspaceName)
    }

    public func setBadge(_ count: Int) async {
        badge = count
    }
}
