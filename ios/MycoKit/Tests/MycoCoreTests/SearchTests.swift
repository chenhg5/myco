import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// Finding things: in a conversation, and across the workspace.
@MainActor
struct SearchTests {
    @Test func aConversationsItemsAreFoundByTheirWords() {
        let items = [
            ConversationItem(id: "a", at: Date(timeIntervalSince1970: 1), kind: .mine(text: "加一个导出按钮", steer: false, by: nil)),
            ConversationItem(id: "b", at: Date(timeIntervalSince1970: 2), kind: .reply(text: "导出按钮加好了", taskID: "a", to: nil)),
            ConversationItem(id: "c", at: Date(timeIntervalSince1970: 3), kind: .progress(text: "Export works", latest: true)),
            ConversationItem(id: "d", at: Date(timeIntervalSince1970: 4), kind: .stopped),
        ]
        #expect(ConversationSearch.hits(in: items, query: "导出") == ["a", "b"])
        #expect(ConversationSearch.hits(in: items, query: "EXPORT") == ["c"])
        #expect(ConversationSearch.hits(in: items, query: "  ").isEmpty)
    }

    @Test func aSnippetShowsWhereItWasSaid() {
        let long = String(repeating: "前面的话 ", count: 20) + "关键的一句：导出按钮放在右上角\n" + String(repeating: "后面 ", count: 30)
        let snippet = ConversationSearch.snippet(long, query: "导出按钮")
        #expect(snippet.hasPrefix("…"))
        #expect(snippet.contains("导出按钮放在右上角"))
        #expect(!snippet.contains("\n"))
        #expect(ConversationSearch.snippet("short", query: "x") == "short")
    }

    @Test func theWorkspaceIsSearchedForMembersProjectsAndMessages() async throws {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        let search = session.search
        #expect(search.members("测试").map(\.agent.name).contains("studio-claude-55d0"))
        #expect(!search.projects("ios").isEmpty)
        await search.search("Tab")
        let hit = try #require(search.messages.first)
        #expect(hit.snippet.localizedCaseInsensitiveContains("tab"))
        #expect(!hit.party.isEmpty)
        // One letter: no messages looked for.
        await search.search("T")
        #expect(search.messages.isEmpty)
    }
}
