import Foundation
import Testing
@testable import MycoCore

/// A background per conversation, and a default, kept on this phone.
@MainActor
struct ChatBackgroundTests {
    @Test func aConversationsOwnElseTheDefault() throws {
        let defaults = try #require(UserDefaults(suiteName: "chat-bg-\(UUID().uuidString)"))
        let folder = FileManager.default.temporaryDirectory.appendingPathComponent("chat-bg-\(UUID().uuidString)")
        let backgrounds = ChatBackgrounds(defaults: defaults, folder: folder)
        let one = ChatBackgrounds.key(workspace: "w", agent: "coder")
        let other = ChatBackgrounds.key(workspace: "w", agent: "pm")
        #expect(backgrounds.background(for: one) == nil)
        backgrounds.set(ChatBackground(preset: "mint"), for: nil)
        #expect(backgrounds.background(for: one)?.preset == "mint", "the default")
        let picture = try backgrounds.keep(Data([0xFF, 0xD8, 0xFF]))
        backgrounds.set(ChatBackground(image: picture, veil: 0.3), for: one)
        #expect(backgrounds.background(for: one)?.image == picture)
        #expect(backgrounds.background(for: other)?.preset == "mint")
        #expect(FileManager.default.fileExists(atPath: backgrounds.url(of: picture).path))
        // Kept across launches.
        let again = ChatBackgrounds(defaults: defaults, folder: folder)
        #expect(again.own(one)?.veil == 0.3)
        // Its own taken away: it follows the default, and the picture goes.
        backgrounds.set(nil, for: one)
        #expect(backgrounds.background(for: one)?.preset == "mint")
        #expect(!FileManager.default.fileExists(atPath: backgrounds.url(of: picture).path))
        // "None" chosen counts as following.
        backgrounds.set(ChatBackground(), for: other)
        #expect(backgrounds.own(other) == nil)
        try? FileManager.default.removeItem(at: folder)
    }
}
