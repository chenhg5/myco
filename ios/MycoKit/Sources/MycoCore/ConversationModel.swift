import Foundation
import Observation
import MycoAPI

/// One conversation on screen: what was said, where the open task stands,
/// and what the input does. The page binds to it and draws; it does not
/// decide any of this itself.
@MainActor
@Observable
public final class ConversationModel {
    public let agent: String
    /// Set on a project's page: the conversation is with its coordinator,
    /// and the coordinator's dispatches show as capsules.
    public let project: String?

    public private(set) var items: [ConversationItem] = []
    public private(set) var openTask: Message?
    public private(set) var loaded = false
    /// Why the last send, steer or stop did not go through.
    public private(set) var problem: String?
    /// Messages waiting for the open task to finish, in order.
    public private(set) var queued: [String] = []
    public var draft = ""

    private let session: WorkspaceSession
    private var sending = false

    /// How often an open conversation refreshes.
    public static let pollInterval: Duration = .seconds(2)

    public init(session: WorkspaceSession, agent: String, project: String? = nil) {
        self.session = session
        self.agent = agent
        self.project = project
    }

    public var action: ComposerAction {
        ComposerAction.resolve(hasOpenTask: openTask != nil, hasText: !trimmedDraft.isEmpty)
    }

    /// Sending is off while a send is under way, or with nothing to send.
    public var canSubmit: Bool {
        !sending && (action == .stop || !trimmedDraft.isEmpty)
    }

    private var trimmedDraft: String {
        draft.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public func run() async {
        while !Task.isCancelled {
            await reload()
            try? await Task.sleep(for: Self.pollInterval)
        }
    }

    public func reload() async {
        await load()
        // The open task closed: what waited behind it goes out now.
        if await sendNextQueued() { await load() }
    }

    private func load() async {
        let me = session.me
        guard !me.isEmpty else { return }
        do {
            let messages = try await session.hub.exchanges(between: me, and: agent)
            let others = try await dispatches()
            let built = ConversationBuilder.items(
                me: me, messages: messages, others: others, status: { self.session.status(of: $0) }, now: Date())
            // Nothing new: leave the list alone, so the page does not redraw.
            if built != items { items = built }
            openTask = ConversationBuilder.openTask(me: me, messages: messages)
            loaded = true
        } catch is CancellationError {
            return
        } catch {
            // The workspace's own banner says it cannot be reached.
            loaded = true
        }
    }

    /// On a project's page: what its coordinator gave the other members.
    private func dispatches() async throws -> [Message] {
        guard let project else { return [] }
        let recent = try await session.hub.recent(project: project)
        return recent.compactMap(\.task).filter { $0.from == agent && $0.to != session.me }
    }

    /// The send button: a new message, an addition to the open task, or stop.
    public func submit() async {
        guard canSubmit else { return }
        let action = action
        let text = trimmedDraft
        if action != .stop { draft = "" }
        sending = true
        do {
            switch action {
            case .send:
                _ = try await session.hub.send(SendRequest(to: agent, body: text, project: project))
            case .steer:
                if let task = openTask { try await session.hub.steer(task.id, body: text) }
            case .stop:
                if let task = openTask { try await session.hub.stop(task.id) }
            }
            problem = nil
        } catch {
            // Give the words back: nothing typed is lost to a failed send.
            if action != .stop, draft.isEmpty { draft = text }
            problem = (error as? HubError)?.message ?? error.localizedDescription
        }
        sending = false
        await reload()
    }

    /// Stops the open task whatever is typed (the action on a stuck task).
    public func submitStop() async {
        guard let task = openTask, !sending else { return }
        sending = true
        do {
            try await session.hub.stop(task.id)
            problem = nil
        } catch {
            problem = (error as? HubError)?.message ?? error.localizedDescription
        }
        sending = false
        await reload()
    }

    /// Holds the typed text until the open task finishes, then sends it.
    public func queueNext() {
        let text = trimmedDraft
        guard !text.isEmpty else { return }
        queued.append(text)
        draft = ""
    }

    public func unqueue(at index: Int, edit: Bool) {
        guard queued.indices.contains(index) else { return }
        let text = queued.remove(at: index)
        if edit { draft = text }
    }

    /// Sends the first waiting message once nothing is open. True if it went.
    private func sendNextQueued() async -> Bool {
        guard openTask == nil, !sending, let next = queued.first else { return false }
        sending = true
        defer { sending = false }
        do {
            _ = try await session.hub.send(SendRequest(to: agent, body: next, project: project))
            queued.removeFirst()
            return true
        } catch {
            problem = (error as? HubError)?.message ?? error.localizedDescription
            return false
        }
    }
}
