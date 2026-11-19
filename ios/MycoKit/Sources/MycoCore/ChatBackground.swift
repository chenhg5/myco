import Foundation
import Observation

/// 聊天背景: a background per conversation, and one default for those
/// without their own. One of a few quiet presets from the theme's own
/// colours, or a picture chosen on this phone, with a veil of the page's
/// colour over it so the words stay readable. Kept on this phone only: the
/// choice in its settings, a picture in its own files (never sent to a
/// workspace). The desktop app's `chatBg.ts`.
public struct ChatBackground: Codable, Sendable, Hashable {
    public static let presets = ["mist", "mint", "lilac", "sand", "dots", "grid"]

    /// One of `presets`.
    public var preset: String?
    /// A picture's file name, in `ChatBackgrounds.folder`.
    public var image: String?
    /// How much of the page's colour lies over it, 0...0.9.
    public var veil: Double

    public init(preset: String? = nil, image: String? = nil, veil: Double = 0.5) {
        self.preset = preset
        self.image = image
        self.veil = veil
    }

    public var isEmpty: Bool { preset == nil && image == nil }

    public static func name(of preset: String) -> String {
        switch preset {
        case "mist": return L("晨雾", "Mist")
        case "mint": return L("薄荷", "Mint")
        case "lilac": return L("丁香", "Lilac")
        case "sand": return L("沙丘", "Sand")
        case "dots": return L("点阵", "Dots")
        case "grid": return L("方格", "Grid")
        default: return preset
        }
    }
}

/// The backgrounds kept on this phone.
@MainActor
@Observable
public final class ChatBackgrounds {
    public static let shared = ChatBackgrounds()

    struct Kept: Codable {
        var def: ChatBackground?
        var by: [String: ChatBackground] = [:]
    }

    private let defaults: UserDefaults
    private let key = "myco-chat-bg"
    public let folder: URL
    private var kept: Kept

    public init(defaults: UserDefaults = .standard, folder: URL? = nil) {
        self.defaults = defaults
        self.folder = folder ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("chat-backgrounds", isDirectory: true)
        kept = (defaults.data(forKey: key)).flatMap { try? JSONDecoder().decode(Kept.self, from: $0) } ?? Kept()
    }

    /// A conversation's key: its workspace and agent.
    public static func key(workspace: String, agent: String) -> String { workspace + "|" + agent }

    /// What a conversation draws: its own, else the default.
    public func background(for key: String) -> ChatBackground? {
        kept.by[key] ?? kept.def
    }

    /// Its own (nil: it follows the default).
    public func own(_ key: String) -> ChatBackground? { kept.by[key] }
    public var fallback: ChatBackground? { kept.def }

    /// Sets (or, with nil, clears) a conversation's own background, or the
    /// default (key nil). Pictures nothing uses any more are deleted.
    public func set(_ background: ChatBackground?, for key: String?) {
        let value = background?.isEmpty == false ? background : nil
        if let key { kept.by[key] = value } else { kept.def = value }
        if let data = try? JSONEncoder().encode(kept) { defaults.set(data, forKey: self.key) }
        sweep()
    }

    /// Keeps a picture (already made small enough) and answers its name.
    public func keep(_ jpeg: Data) throws -> String {
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let name = UUID().uuidString + ".jpg"
        try jpeg.write(to: folder.appendingPathComponent(name), options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        return name
    }

    public func url(of image: String) -> URL { folder.appendingPathComponent(image) }

    private func sweep() {
        let used = Set(([kept.def] + kept.by.values.map { Optional($0) }).compactMap { $0?.image })
        let files = (try? FileManager.default.contentsOfDirectory(atPath: folder.path)) ?? []
        for file in files where !used.contains(file) {
            try? FileManager.default.removeItem(at: folder.appendingPathComponent(file))
        }
    }
}
