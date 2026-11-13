import Foundation

/// A workspace this device joined (a hub, by the name people gave it here).
public struct WorkspaceRecord: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var url: URL

    public init(id: String, name: String, url: URL) {
        self.id = id
        self.name = name
        self.url = url
    }
}

/// Device tokens and device keys. The app keeps them in the keychain.
public protocol SecretStore: Sendable {
    func secret(for key: String) async throws -> String?
    func setSecret(_ value: String?, for key: String) async throws
}

/// The workspaces this device joined, in the order they were added.
public protocol WorkspaceStore: Sendable {
    func workspaces() async throws -> [WorkspaceRecord]
    func save(_ workspaces: [WorkspaceRecord]) async throws
}

/// Text typed but not sent, per conversation.
public protocol DraftStore: Sendable {
    func draft(for conversation: String) async -> String
    func setDraft(_ text: String, for conversation: String) async
}

/// What a workspace showed last, so its pages open with something at once.
public protocol SnapshotStore: Sendable {
    func snapshot(for workspace: String) async -> Data?
    func setSnapshot(_ data: Data?, for workspace: String) async
}

/// Everything in memory: for previews, tests and the demo workspace.
public actor MemoryStore: SecretStore, WorkspaceStore, DraftStore, SnapshotStore {
    private var snapshots: [String: Data] = [:]
    public func snapshot(for workspace: String) -> Data? { snapshots[workspace] }
    public func setSnapshot(_ data: Data?, for workspace: String) { snapshots[workspace] = data }

    private var secrets: [String: String] = [:]
    private var records: [WorkspaceRecord] = []
    private var drafts: [String: String] = [:]

    public init() {}

    public func secret(for key: String) -> String? { secrets[key] }
    public func setSecret(_ value: String?, for key: String) { secrets[key] = value }
    public func workspaces() -> [WorkspaceRecord] { records }
    public func save(_ workspaces: [WorkspaceRecord]) { records = workspaces }
    public func draft(for conversation: String) -> String { drafts[conversation] ?? "" }
    public func setDraft(_ text: String, for conversation: String) {
        drafts[conversation] = text.isEmpty ? nil : text
    }
}
