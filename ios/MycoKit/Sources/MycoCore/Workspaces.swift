import Foundation
import MycoAPI
import MycoStore

/// Joining and leaving workspaces: the one place that knows a workspace is
/// a record (which hub) plus a secret (this device's token).
public struct Workspaces: Sendable {
    private let records: any WorkspaceStore
    private let secrets: any SecretStore
    /// What this device is called in the member's list of devices.
    private let deviceName: String

    public init(records: any WorkspaceStore, secrets: any SecretStore, deviceName: String) {
        self.records = records
        self.secrets = secrets
        self.deviceName = deviceName
    }

    public enum JoinError: Error, Sendable {
        /// The text is not a link that adds a workspace.
        case notALink
        /// The hub refused (a used or expired link), or could not be reached.
        case refused(String)
    }

    private func tokenKey(_ id: String) -> String { "workspace.\(id).token" }

    /// Joins the workspace a link leads to. A one-time code is traded for
    /// this device's own token; the token goes to the keychain.
    public func join(link: String) async throws -> WorkspaceRecord {
        guard let link = AppLink(link) else { throw JoinError.notALink }
        do {
            var token = link.credential
            if link.isPairingCode {
                token = try await HubClient.pair(hub: link.hub, code: link.credential, device: deviceName).token
            }
            // Proves the token works before anything is saved.
            _ = try await HubClient(baseURL: link.hub, token: token).whoami()
            let record = WorkspaceRecord(id: UUID().uuidString, name: link.hub.host() ?? link.hub.absoluteString, url: link.hub)
            try await secrets.setSecret(token, for: tokenKey(record.id))
            // One workspace at a time for now: joining replaces the list.
            try await records.save([record])
            return record
        } catch let error as HubError {
            throw JoinError.refused(error.message)
        }
    }

    /// The workspace joined last, with a client for it; nil when there is none.
    public func current() async -> (record: WorkspaceRecord, hub: HubClient)? {
        guard let record = try? await records.workspaces().first,
              let token = try? await secrets.secret(for: tokenKey(record.id))
        else { return nil }
        return (record, HubClient(baseURL: record.url, token: token))
    }

    /// Leaves a workspace: signs this device out on the hub when it can be
    /// reached, and forgets the token either way.
    public func leave(_ id: String) async {
        guard let all = try? await records.workspaces(), let record = all.first(where: { $0.id == id }) else { return }
        if let token = try? await secrets.secret(for: tokenKey(id)) {
            try? await HubClient(baseURL: record.url, token: token).removeThisDevice()
        }
        try? await secrets.setSecret(nil, for: tokenKey(id))
        try? await records.save(all.filter { $0.id != id })
    }
}
