import Foundation

/// What the app sends: `POST /v1/console/messages`.
public struct SendRequest: Codable, Sendable, Hashable {
    public var to: String
    public var body: String
    public var project: String?
    /// A task from this sender to the same recipient: delivery waits until
    /// it closes (hubs from v0.19.2).
    public var after: String?

    public init(to: String, body: String, project: String? = nil, after: String? = nil) {
        self.to = to
        self.body = body
        self.project = project
        self.after = after
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
    /// `GET /v1/console/recent`: newest first; `project` narrows it.
    func recent(project: String?) async throws -> [RecentItem]
    /// `GET /v1/console/exchanges`: every message between two parties,
    /// newest first.
    func exchanges(between a: String, and b: String) async throws -> [Message]
    /// `GET /v1/console/messages/{id}/trace`
    func trace(of messageID: String, after seq: Int) async throws -> Trace
    /// `POST /v1/console/messages`
    func send(_ request: SendRequest) async throws -> Message
    /// `POST /v1/console/messages/{id}/steer`
    func steer(_ messageID: String, body: String) async throws
    /// `POST /v1/console/messages/{id}/stop`
    func stop(_ messageID: String) async throws
}
