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

    /// Which workspace was on screen last: its id, in a file beside the list.
    private var selectedFile: URL { file.deletingLastPathComponent().appending(path: "selected") }

    public func selected() -> String? {
        guard let text = try? String(contentsOf: selectedFile, encoding: .utf8) else { return nil }
        let id = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return id.isEmpty ? nil : id
    }

    public func select(_ id: String?) {
        guard let id else {
            try? FileManager.default.removeItem(at: selectedFile)
            return
        }
        try? FileManager.default.createDirectory(at: selectedFile.deletingLastPathComponent(), withIntermediateDirectories: true)
        try? Data(id.utf8).write(to: selectedFile, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
    }
}

/// Snapshots as files in the app's caches folder, one folder per
/// workspace, protected until the device is first unlocked. The system may
/// clear them; nothing depends on them being there.
public struct FileSnapshotStore: SnapshotStore {
    private let folder: URL

    public init(folder: URL? = nil) {
        self.folder = folder ?? URL.cachesDirectory.appending(path: "myco/snapshots")
    }

    /// A name safe to use as a file's: letters, digits, dots, dashes.
    private static func safe(_ text: String) -> String {
        String(text.map { $0.isASCII && ($0.isLetter || $0.isNumber || $0 == "." || $0 == "-") ? $0 : "_" })
    }

    private func file(_ workspace: String, _ name: String) -> URL {
        folder.appending(path: Self.safe(workspace)).appending(path: Self.safe(name) + ".json")
    }

    public func snapshot(of workspace: String, name: String) async -> Data? {
        try? Data(contentsOf: file(workspace, name))
    }

    public func setSnapshot(_ data: Data?, of workspace: String, name: String) async {
        let file = file(workspace, name)
        guard let data else {
            try? FileManager.default.removeItem(at: file)
            return
        }
        try? FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
        try? data.write(to: file, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
    }

    public func removeSnapshots(of workspace: String) async {
        try? FileManager.default.removeItem(at: folder.appending(path: Self.safe(workspace)))
    }
}
