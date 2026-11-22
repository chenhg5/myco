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

    @Test func pageFragmentsStayOpaqueAndNeverBecomeHubQueries() throws {
        let fragment = "watch?source=src&movie=42&line=%E4%B8%BB%E7%BA%BF&episode=1%262"
        let href = "https://hub.example.com/pages/pg_ab12#" + fragment
        let target = try #require(PageLinks.target(of: href, in: hub))
        #expect(target.id == "pg_ab12")
        #expect(target.fragment == fragment)
        #expect(PageLinks.id(of: href, in: hub) == target.id)
        let address = try #require(PageLinks.address(of: target.id, fragment: target.fragment))
        #expect(address.absoluteString == "myco-page://pg-ab12/#" + fragment)
        #expect(address.path == "/")
        #expect(address.query == nil)
        #expect(PageLinks.target(of: "https://hub.example.com/pages/pg_ab12", in: hub)?.fragment == nil)
        #expect(PageLinks.target(of: "https://hub.example.com/pages/pg_ab12#", in: hub)?.fragment == nil)
        #expect(PageLinks.target(of: "https://hub.example.com/w/other/pages/pg_ab12#watch?movie=1", in: hub) == nil)
        #expect(PageLinks.target(of: "https://other.example.com/pages/pg_ab12#watch?movie=1", in: hub) == nil)
        #expect(PageLinks.target(of: "https://hub.example.com/pages/pg_ab12?movie=1#watch", in: hub) == nil)
        #expect(PageLinks.target(of: "https://hub.example.com/pages/pg_ab12/v/1/index.html#watch", in: hub) == nil)
        let workspace = URL(string: "https://hub.example.com/w/team/")
        #expect(PageLinks.target(of: "https://hub.example.com/w/team/pages/pg_ab12#" + fragment, in: workspace)?.fragment == fragment)
    }

    @Test func aCardKeepsTheFirstLinksFragmentAndDeduplicatesPages() {
        let first = "https://hub.example.com/pages/pg_one#watch?movie=1&episode=2"
        let body = "\(first) https://hub.example.com/pages/pg_one#watch?movie=2 https://hub.example.com/pages/pg_two"
        let targets = PageLinks.targets(in: body, workspace: hub)
        #expect(targets.map(\.id) == ["pg_one", "pg_two"])
        #expect(targets.first?.fragment == "watch?movie=1&episode=2")
        #expect(targets.last?.fragment == nil)
        #expect(PageLinks.ids(in: body, workspace: hub) == ["pg_one", "pg_two"])
    }

    @Test func oversizedFragmentsAreDroppedWithoutLosingThePage() throws {
        let maximum = String(repeating: "x", count: 1024)
        #expect(PageLinks.target(of: "https://hub.example.com/pages/pg_ab12#" + maximum, in: hub)?.fragment == maximum)
        for fragment in [maximum + "x", String(repeating: "影", count: 400)] {
            let target = try #require(PageLinks.target(of: "https://hub.example.com/pages/pg_ab12#" + fragment, in: hub))
            #expect(target.id == "pg_ab12")
            #expect(target.fragment == nil)
            #expect(PageLinks.address(of: target.id, fragment: fragment)?.absoluteString == "myco-page://pg-ab12/")
        }
    }

    @Test func onlyPublicCredentialFreePageURLsAreExposed() {
        var page = Page(id: "pg_a", name: "Films", publisher: "agent")
        for address in ["http://127.0.0.1:7777/pages/pg_a", "http://127.3.2.1/pages/pg_a", "http://localhost/pages/pg_a", "http://[::1]/pages/pg_a", "http://[0:0:0:0:0:0:0:1]/pages/pg_a", "http://[::ffff:127.0.0.1]/pages/pg_a", "https://user:secret@hub.example.com/pages/pg_a", "myco-page://pg-a/", "https://hub.example.com/pages/pg_other"] {
            page.url = address
            #expect(PageLinks.publicURL(of: page) == nil)
        }
        page.url = "https://hub.example.com/w/team/pages/pg_a?token=secret#watch?movie=1"
        #expect(PageLinks.publicURL(of: page) == "https://hub.example.com/w/team/pages/pg_a")
        page.url = nil
        #expect(PageLinks.publicURL(of: page) == nil)
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
