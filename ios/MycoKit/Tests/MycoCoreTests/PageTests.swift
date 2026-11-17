import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// Web pages agents publish: which links are pages, what a card says, and
/// reading them from the workspace.
@MainActor
struct PageTests {
    let hub = URL(string: "https://hub.example.com")!

    @Test func aLinkToAPageOfThisWorkspaceIsKnown() {
        #expect(PageLinks.id(of: "https://hub.example.com/pages/pg_ab12", in: hub) == "pg_ab12")
        #expect(PageLinks.id(of: " https://HUB.example.com/pages/pg_ab12/ ", in: hub) == "pg_ab12")
        // Another workspace's, something else on this one, no workspace known.
        #expect(PageLinks.id(of: "https://other.example.com/pages/pg_ab12", in: hub) == nil)
        #expect(PageLinks.id(of: "https://hub.example.com/pages/pg_ab12/v/1/index.html", in: hub) == nil)
        #expect(PageLinks.id(of: "https://hub.example.com/c/myco_pair_x", in: hub) == nil)
        #expect(PageLinks.id(of: "https://hub.example.com/pages/pg_ab12", in: nil) == nil)
        // A workspace behind a path.
        let under = URL(string: "https://example.com/myco/")!
        #expect(PageLinks.id(of: "https://example.com/myco/pages/pg_9z", in: under) == "pg_9z")
        // In a message: once each, in order, without the sentence's full stop.
        let body = "做好了：https://hub.example.com/pages/pg_one。另一个在 (https://hub.example.com/pages/pg_two)，"
            + "再说一遍 https://hub.example.com/pages/pg_one. 这个不是：https://example.org/pages/pg_no"
        #expect(PageLinks.ids(in: body, workspace: hub) == ["pg_one", "pg_two"])
        #expect(PageLinks.ids(in: "没有链接", workspace: hub).isEmpty)
    }

    @Test func aPageHasItsOwnAddressInsideTheApp() throws {
        let home = try #require(PageLinks.address(of: "pg_ab12"))
        #expect(home.absoluteString == "myco-page://pg-ab12/")
        #expect(PageLinks.isOwn(home, of: "pg_ab12"))
        let file = try #require(PageLinks.address(of: "pg_ab12", path: "img/图 1.png"))
        #expect(file.host == "pg-ab12")
        #expect(file.path == "/img/图 1.png")
        // Another page's files, and the web, are not its own.
        #expect(!PageLinks.isOwn(home, of: "pg_other"))
        #expect(!PageLinks.isOwn(try #require(URL(string: "https://pg-ab12/")), of: "pg_ab12"))
        // It loads its own files only, and sends nothing elsewhere.
        #expect(PageLinks.contentPolicy.contains("default-src 'self'"))
        #expect(PageLinks.contentPolicy.contains("connect-src 'self' data: blob:"))
        #expect(!PageLinks.contentPolicy.contains("http"))
    }

    @Test func aCardSaysWhichVersionWhoAndWhen() {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        AppLanguage.$forced.withValue(.zh) {
            var page = Page(id: "pg_a", name: "周报", publisher: "pm", version: 2, updatedAt: now.addingTimeInterval(-1_080))
            #expect(PageWords.line(page, publisher: "产品", now: now) == "第 2 版 · 由 产品 发布 · 18 分钟前")
            // One version: not said which.
            page.version = 1
            #expect(PageWords.line(page, publisher: "产品", now: now) == "由 产品 发布 · 18 分钟前")
            page.live = true
            page.online = false
            #expect(PageWords.line(page, publisher: "产品", now: now).hasPrefix("实时网页 · 现在没有在转发"))
        }
    }

    @Test func pagesAreReadWhenAskedForAndAgainLater() async throws {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        let pages = session.pages
        let link = FakeHub.demoAddress.absoluteString + "/pages/pg_checklist"
        #expect(pages.ids(in: "看这里：\(link)") == ["pg_checklist"])
        #expect(pages.pages.isEmpty)
        let start = Date()
        await pages.need(["pg_checklist", "pg_gone"], now: start)
        let page = try #require(pages.pages["pg_checklist"])
        #expect(page.name == "真机验证清单")
        #expect(page.version == 2)
        // One that is not there: no card, and not asked for again at once.
        #expect(pages.pages["pg_gone"] == nil)
        // A newer version is seen when it is asked for again a while later.
        await hub.publishAgain("pg_checklist")
        await pages.need(["pg_checklist"], now: start.addingTimeInterval(5))
        #expect(pages.pages["pg_checklist"]?.version == 2, "read a moment ago: not again yet")
        await pages.need(["pg_checklist"], now: start.addingTimeInterval(30))
        #expect(pages.pages["pg_checklist"]?.version == 3)
        // Its files, by version; the first one when no path is given.
        let home = try await pages.file(of: "pg_checklist", version: 3, path: "")
        #expect(home.contentType.hasPrefix("text/html"))
        #expect(String(decoding: home.data, as: UTF8.self).contains("第 3 版"))
        let style = try await pages.file(of: "pg_checklist", version: 3, path: "style.css")
        #expect(style.contentType.hasPrefix("text/css"))
        await #expect(throws: HubError.self) { _ = try await pages.file(of: "pg_checklist", version: 3, path: "nope.js") }
        // The workspace does not answer: what is known stays.
        await hub.setUnreachable(true)
        #expect(await pages.read("pg_checklist")?.version == 3)
    }
}
