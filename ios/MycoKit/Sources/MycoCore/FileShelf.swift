import Foundation
import MycoAPI

/// The files of messages, kept on this device once fetched: a picture is
/// downloaded once, however often its conversation is opened. They live in
/// the caches folder, which the system may clear; nothing depends on them
/// being there.
public actor FileShelf {
    private let folder: URL
    private let fetch: @Sendable (String) async throws -> Data
    /// Downloads under way, by file id: asked for twice, fetched once.
    private var running: [String: Task<URL, Error>] = [:]
    private var active = 0
    private var waiting: [CheckedContinuation<Void, Never>] = []

    /// At most this many files come down at a time.
    static let atOnce = 4

    /// - Parameters:
    ///   - folder: where this workspace's files are kept.
    ///   - fetch: gets a file's bytes from the workspace, by id.
    public init(folder: URL, fetch: @escaping @Sendable (String) async throws -> Data) {
        self.folder = folder
        self.fetch = fetch
    }

    /// Where every workspace's files are kept, unless told otherwise.
    public static var root: URL { URL.cachesDirectory.appending(path: "myco/files") }

    /// The folder of one workspace's files.
    public static func folder(of workspace: String, in root: URL = FileShelf.root) -> URL {
        root.appending(path: safe(workspace))
    }

    /// Forgets a workspace's files (it was left).
    public static func forget(_ workspace: String, in root: URL = FileShelf.root) {
        try? FileManager.default.removeItem(at: folder(of: workspace, in: root))
    }

    /// A file on this device, fetched if it is not here yet. It keeps its
    /// own name, so a preview and "save" call it what its sender did.
    public func url(of attachment: Attachment) async throws -> URL {
        let file = place(of: attachment.fileId, name: attachment.name)
        if FileManager.default.fileExists(atPath: file.path) { return file }
        if let task = running[attachment.fileId] { return try await task.value }
        let id = attachment.fileId
        let task = Task<URL, Error> {
            // Its turn among the downloads, given back however it ends.
            await self.enter()
            let fetched: Result<Data, Error>
            do {
                fetched = .success(try await self.fetch(id))
            } catch {
                fetched = .failure(error)
            }
            self.leave()
            try self.write(try fetched.get(), to: file)
            return file
        }
        running[id] = task
        defer { running[id] = nil }
        return try await task.value
    }

    /// Keeps a file that is already here (one I just sent): showing it
    /// needs no download.
    public func keep(_ data: Data, as attachment: Attachment) {
        try? write(data, to: place(of: attachment.fileId, name: attachment.name))
    }

    private func write(_ data: Data, to file: URL) throws {
        try FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
        try data.write(to: file, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
    }

    private func place(of id: String, name: String) -> URL {
        folder.appending(path: Self.safe(id)).appending(path: Self.fileName(name))
    }

    private func enter() async {
        if active < Self.atOnce {
            active += 1
            return
        }
        await withCheckedContinuation { waiting.append($0) }
    }

    private func leave() {
        if waiting.isEmpty {
            active -= 1
        } else {
            // Its place goes straight to the next one in line.
            waiting.removeFirst().resume()
        }
    }

    /// Letters, digits, dots and dashes: safe as a folder's name.
    static func safe(_ text: String) -> String {
        let cleaned = String(text.map { $0.isASCII && ($0.isLetter || $0.isNumber || $0 == "." || $0 == "-") ? $0 : "_" })
        return cleaned.isEmpty || cleaned.allSatisfy({ $0 == "." }) ? "_" : cleaned
    }

    /// A sender's file name made safe to write: no folders, no leading
    /// dot, not too long, its ending kept.
    static func fileName(_ name: String) -> String {
        var cleaned = name.replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: ":", with: "_")
            .replacingOccurrences(of: "\0", with: "").trimmingCharacters(in: .whitespacesAndNewlines)
        while cleaned.hasPrefix(".") { cleaned.removeFirst() }
        if cleaned.isEmpty { return "file" }
        let ending = (cleaned as NSString).pathExtension
        let base = (cleaned as NSString).deletingPathExtension
        if base.count > 80 { cleaned = String(base.prefix(80)) + (ending.isEmpty ? "" : "." + ending) }
        return cleaned
    }
}
