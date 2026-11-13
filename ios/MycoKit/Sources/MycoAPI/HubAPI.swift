import Foundation

/// What the app sends: `POST /v1/console/messages`.
public struct SendRequest: Codable, Sendable, Hashable {
    public var to: String
    public var body: String
    /// The thread it continues; nil starts a new one.
    public var threadId: String?
    /// Puts a new thread in this project.
    public var project: String?
    /// A task from this sender to the same recipient: delivery waits until
    /// it closes (hubs from v0.19.2).
    public var after: String?
    /// Files uploaded from this device (`upload`), by id: attaching one
    /// lets the recipient read it. Left out when there are none.
    public var attachments: [String]?

    public init(
        to: String, body: String, threadId: String? = nil, project: String? = nil, after: String? = nil, attachments: [String] = []
    ) {
        self.to = to
        self.body = body
        self.threadId = threadId
        self.project = project
        self.after = after
        self.attachments = attachments.isEmpty ? nil : attachments
    }
}

/// A file kept by the workspace: what an upload answers with.
public struct HubFile: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var size: Int64
    public var contentType: String?

    public init(id: String, name: String, size: Int64, contentType: String? = nil) {
        self.id = id
        self.name = name
        self.size = size
        self.contentType = contentType
    }
}

/// Which recent list to ask the hub for.
public enum RecentScope: Sendable, Hashable {
    /// What I talk to, one row per party (`group=pair&side=me`).
    case myPairs
    /// One project's threads.
    case project(String)
    /// Every thread this device may read, newest first.
    case everything
}

/// One page of a conversation, newest first; `more`: older ones exist.
public struct MessagePage: Sendable, Hashable {
    public var messages: [Message]
    public var more: Bool

    public init(messages: [Message], more: Bool) {
        self.messages = messages
        self.more = more
    }
}

/// A link that lets machines and agents join the workspace.
public struct JoinLink: Codable, Sendable, Hashable {
    public var joinUrl: String
    /// The hub's join script ends with the agent's last step, so one
    /// command is all an agent needs (hubs before this leave it out).
    public var agentGuide: Bool?

    public init(joinUrl: String, agentGuide: Bool? = true) {
        self.joinUrl = joinUrl
        self.agentGuide = agentGuide
    }
}

/// One workspace (a hub), as this device sees it. Every method is one of
/// the hub's `/v1/console/*` calls; nothing here caches, decrypts or knows
/// about more than one workspace.
public protocol HubAPI: Sendable {
    /// `GET /v1/console/me`
    func whoami() async throws -> Whoami
    /// `GET /v1/overview`
    func overview() async throws -> Overview
    /// `GET /v1/projects`
    func projects() async throws -> [Project]
    /// `GET /v1/console/recent`: newest first.
    func recent(_ scope: RecentScope) async throws -> [RecentItem]
    /// `GET /v1/console/exchanges`: the messages between two parties in
    /// every thread, newest first, a page at a time. `before`: only those
    /// older than this (to page back); nil is the newest page.
    func exchanges(between a: String, and b: String, before: Date?) async throws -> MessagePage
    /// `GET /v1/console/messages/{id}/trace`
    func trace(of messageID: String, after seq: Int) async throws -> Trace
    /// `POST /v1/console/messages`
    func send(_ request: SendRequest) async throws -> Message
    /// `POST /v1/console/messages/{id}/steer`: adds to a task being worked
    /// on, with files if any (their ids).
    func steer(_ messageID: String, body: String, attachments: [String]) async throws
    /// `POST /v1/console/files?name=`: keeps a file in the workspace as
    /// mine, to attach to what I send next. A workspace too old for this
    /// answers `not_found`.
    func upload(_ data: Data, name: String, contentType: String) async throws -> HubFile
    /// `GET /v1/console/files/{id}`: a file attached to a message this
    /// device may read.
    func file(_ id: String) async throws -> Data
    /// `POST /v1/console/messages/{id}/stop`
    func stop(_ messageID: String) async throws
    /// `POST /v1/enrollments` with `kind: "link"`: a link to hand to an
    /// agent or a machine (ten uses, a week), as the desktop app makes.
    func joinLink() async throws -> JoinLink
}

extension HubAPI {
    /// Adds words alone to a task being worked on.
    public func steer(_ messageID: String, body: String) async throws {
        try await steer(messageID, body: body, attachments: [])
    }
}

extension HubFile {
    /// The largest file a workspace takes by default (its `--max-file-size`).
    public static let maxSize = 50 * 1024 * 1024
}
