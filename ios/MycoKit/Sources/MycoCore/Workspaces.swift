import Foundation
import MycoAPI
import MycoStore

/// The workspaces this device has joined, as the app uses them: which
/// there are, which one was on screen last, joining another, renaming and
/// leaving one. The app's is `Workspaces`; tests and the demo have their own.
public protocol WorkspaceBook: Sendable {
    /// In the order they were joined.
    func all() async -> [WorkspaceRecord]
    /// The one on screen last, when it is still joined.
    func selected() async -> String?
    func select(_ id: String?) async
    /// Joins the workspace a link leads to; throws `JoinError`. Joining
    /// one already joined signs this device in again, without a second row.
    func join(link: String) async throws -> WorkspaceRecord
    /// A client for a workspace; nil when its token is gone.
    func hub(of id: String) async -> (any HubAPI)?
    func rename(_ id: String, to name: String) async
    /// Leaves a workspace: signs this device out there when it can be
    /// reached, and forgets it here either way.
    func leave(_ id: String) async
}

/// Why joining did not work.
public enum JoinError: Error, Sendable, Equatable {
    /// The text is not a link that adds a workspace.
    case notALink
    /// The hub refused (a used or expired link), or could not be reached.
    case refused(String)
}

/// Joining and leaving workspaces: the one place that knows a workspace is
/// a record (which hub) plus a secret (this device's token).
public struct Workspaces: WorkspaceBook {
    private let records: any WorkspaceStore
    private let secrets: any SecretStore
    /// What this device is called in the member's list of devices.
    private let deviceName: String
    private let network: URLSession

    public init(records: any WorkspaceStore, secrets: any SecretStore, deviceName: String, network: URLSession = .shared) {
        self.records = records
        self.secrets = secrets
        self.deviceName = deviceName
        self.network = network
    }

    public typealias JoinError = MycoCore.JoinError

    private func tokenKey(_ id: String) -> String { "workspace.\(id).token" }

    /// What a workspace is called on screen. A host that is an address
    /// ("192.168.1.20", "193-46-198-17.sslip.io") says nothing to a person:
    /// it reads as "my workspace" until they name it.
    public static func displayName(of record: WorkspaceRecord) -> String {
        let host = record.url.host() ?? ""
        guard record.name == host, host.first?.isNumber == true || host == "localhost" else { return record.name }
        return L("我的工作区", "My workspace")
    }

    /// What a workspace is called when it is joined: the name in its
    /// address when the hub serves several ("…/w/design" is "design"),
    /// else its host. Two that would read the same get a number, so a
    /// list never shows the same name twice.
    static func name(for hub: URL, among others: [WorkspaceRecord]) -> String {
        var name = hub.host() ?? hub.absoluteString
        let parts = hub.pathComponents
        if let marker = parts.lastIndex(of: "w"), marker + 1 < parts.count, !parts[marker + 1].isEmpty {
            name = parts[marker + 1]
        }
        let taken = Set(others.map(displayName))
        let shown = displayName(of: WorkspaceRecord(id: "", name: name, url: hub))
        guard taken.contains(shown) else { return name }
        var number = 2
        while taken.contains("\(shown) \(number)") { number += 1 }
        return "\(shown) \(number)"
    }

    public func all() async -> [WorkspaceRecord] {
        (try? await records.workspaces()) ?? []
    }

    public func selected() async -> String? {
        let all = await all()
        if let id = await records.selected(), all.contains(where: { $0.id == id }) { return id }
        return all.first?.id
    }

    public func select(_ id: String?) async {
        await records.select(id)
    }

    /// Joins the workspace a link leads to. A one-time code is traded for
    /// this device's own token; the token goes to the keychain.
    public func join(link: String) async throws -> WorkspaceRecord {
        guard let link = AppLink(link) else { throw JoinError.notALink }
        do {
            var token = link.credential
            if link.isPairingCode {
                token = try await HubClient.pair(hub: link.hub, code: link.credential, device: deviceName, session: network).token
            }
            // Proves the token works before anything is saved.
            _ = try await HubClient(baseURL: link.hub, token: token, session: network).whoami()
            var all = await all()
            // Joined before: the same row, signed in again.
            if let known = all.first(where: { Self.same($0.url, link.hub) }) {
                try await secrets.setSecret(token, for: tokenKey(known.id))
                return known
            }
            let record = WorkspaceRecord(id: UUID().uuidString, name: Self.name(for: link.hub, among: all), url: link.hub)
            try await secrets.setSecret(token, for: tokenKey(record.id))
            all.append(record)
            try await records.save(all)
            return record
        } catch let error as HubError {
            throw JoinError.refused(error.message)
        }
    }

    private static func same(_ a: URL, _ b: URL) -> Bool {
        func key(_ url: URL) -> String {
            var text = url.absoluteString.lowercased()
            while text.hasSuffix("/") { text.removeLast() }
            return text
        }
        return key(a) == key(b)
    }

    public func hub(of id: String) async -> (any HubAPI)? {
        guard let client = await client(of: id) else { return nil }
        return client
    }

    private func client(of id: String) async -> HubClient? {
        guard let record = await all().first(where: { $0.id == id }),
              let token = try? await secrets.secret(for: tokenKey(id))
        else { return nil }
        return HubClient(baseURL: record.url, token: token, session: network)
    }

    public func rename(_ id: String, to name: String) async {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        var all = await all()
        guard !name.isEmpty, let index = all.firstIndex(where: { $0.id == id }) else { return }
        all[index].name = name
        try? await records.save(all)
    }

    public func leave(_ id: String) async {
        let all = await all()
        guard all.contains(where: { $0.id == id }) else { return }
        try? await client(of: id)?.removeThisDevice()
        try? await secrets.setSecret(nil, for: tokenKey(id))
        try? await records.save(all.filter { $0.id != id })
        if await records.selected() == id { await records.select(nil) }
    }
}
