import Foundation
import Security

/// Secrets in the system keychain: device tokens and device keys. They stay
/// on this device (not synced, not in backups to other devices) and can be
/// read after the first unlock, so a refresh in the background still works.
public struct KeychainStore: SecretStore {
    private let service: String

    public init(service: String = "dev.myco.ios") {
        self.service = service
    }

    public struct Failure: Error, Sendable {
        public var status: OSStatus
    }

    private func query(_ key: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: key]
    }

    public func secret(for key: String) async throws -> String? {
        var query = query(key)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let data = result as? Data else { throw Failure(status: status) }
        return String(data: data, encoding: .utf8)
    }

    public func setSecret(_ value: String?, for key: String) async throws {
        SecItemDelete(query(key) as CFDictionary)
        guard let value else { return }
        var item = query(key)
        item[kSecValueData as String] = Data(value.utf8)
        item[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let status = SecItemAdd(item as CFDictionary, nil)
        guard status == errSecSuccess else { throw Failure(status: status) }
    }
}

/// The list of workspaces in a JSON file in the app's support folder. No
/// secrets in it: tokens are in the keychain.
public actor FileWorkspaceStore: WorkspaceStore {
    private let file: URL

    public init(file: URL? = nil) {
        self.file = file ?? URL.applicationSupportDirectory.appending(path: "myco/workspaces.json")
    }

    public func workspaces() throws -> [WorkspaceRecord] {
        guard FileManager.default.fileExists(atPath: file.path) else { return [] }
        return try JSONDecoder().decode([WorkspaceRecord].self, from: Data(contentsOf: file))
    }

    public func save(_ workspaces: [WorkspaceRecord]) throws {
        try FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
        try JSONEncoder().encode(workspaces).write(to: file, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
    }
}
