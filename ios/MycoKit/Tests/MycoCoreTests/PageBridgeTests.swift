import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// What a page opened in the app can ask of it (`window.myco`), as the
/// desktop app answers.
@MainActor
struct PageBridgeTests {
    final class Doors {
        var allow = true
        var asked: [(text: String, to: String)] = []
        var saved: [(name: String, data: Data)] = []
    }

    private func bridge(_ hub: FakeHub, page id: String = "pg_checklist", storage: UserDefaults? = nil) async throws -> (PageBridge, Doors) {
        let page = try #require(try await hub.pages().first { $0.id == id })
        let doors = Doors()
        let bridge = PageBridge(
            hub: hub, page: page, me: FakeHub.me, workspace: "ws_test", names: { $0 == FakeHub.me ? "Glenn" : $0 },
            doors: PageBridge.Doors(
                askToSend: { text, to in
                    doors.asked.append((text, to))
                    return doors.allow
                },
                save: { name, data, _ in doors.saved.append((name, data)) },
                send: { to, body, project in try await hub.send(SendRequest(to: to, body: body, project: project)).id }),
            storage: storage ?? UserDefaults(suiteName: "page-bridge-\(UUID().uuidString)")!)
        return (bridge, doors)
    }

    private func object(_ data: Data) -> [String: Any] {
        (try? JSONSerialization.jsonObject(with: data, options: [.fragmentsAllowed])) as? [String: Any] ?? [:]
    }

    @Test func readyExposesTheHubsPublicPageURLWhenAvailable() async throws {
        let hub = FakeHub()
        let (one, _) = try await bridge(hub)
        var page = one.page
        page.url = "https://hub.example.com/w/team/pages/pg_checklist"
        one.update(page)
        let ready = object(try await one.answer("ready", [:]))
        #expect((ready["page"] as? [String: Any])?["url"] as? String == page.url)
        for address in [nil, "http://127.0.0.1:7777/pages/pg_checklist", "https://user:secret@hub.example.com/pages/pg_checklist"] as [String?] {
            page.url = address
            one.update(page)
            let reply = object(try await one.answer("ready", [:]))
            #expect((reply["page"] as? [String: Any])?["url"] == nil)
        }
    }

    @Test func theSharedStateIsReadAndWrittenAndAConflictAnswersTheLatest() async throws {
        let hub = FakeHub()
        let (one, _) = try await bridge(hub)
        let ready = object(try await one.answer("ready", [:]))
        #expect((ready["me"] as? [String: Any])?["name"] as? String == "Glenn")
        #expect(ready["version"] as? Int == 0)
        let wrote = object(try await one.answer("update", ["state": ["n": 1], "version": 0]))
        #expect(wrote["ok"] as? Bool == true)
        #expect(wrote["version"] as? Int == 1)
        // Another copy of the page wrote with an old version: not ok, the latest.
        let (other, _) = try await bridge(hub)
        _ = try await other.answer("ready", [:])
        let late = object(try await other.answer("update", ["state": ["n": 9], "version": 0]))
        #expect(late["ok"] as? Bool == false)
        #expect((late["state"] as? [String: Any])?["n"] as? Int == 1)
        // A page that is open hears of what the other wrote.
        _ = try await other.answer("update", ["state": ["n": 2], "version": 1])
        let events = await one.poll().map(object)
        #expect(events.count == 1)
        #expect(events.first?["myco"] as? String == "change")
        #expect(events.first?["version"] as? Int == 2)
        #expect(await one.poll().isEmpty)
    }

    @Test func recordsArePutListedAndWatched() async throws {
        let hub = FakeHub()
        let (one, _) = try await bridge(hub)
        let put = object(try await one.answer("data.put", ["collection": "items", "id": "a", "data": ["text": "买牛奶"]]))
        #expect(put["version"] as? Int == 1)
        // A stale version is refused with the record as it is.
        await #expect(throws: PageBridge.Refusal.self) {
            _ = try await one.answer("data.put", ["collection": "items", "id": "a", "data": ["text": "x"], "ifVersion": 0])
        }
        let list = object(try await one.answer("data.list", ["collection": "items"]))
        #expect((list["records"] as? [[String: Any]])?.count == 1)
        // Listening: what an agent writes comes as one event, named.
        _ = try await one.answer("data.watch", [:])
        await hub.agentWrites(page: "pg_checklist", collection: "items", id: "b", data: ["text": "回邮件"])
        let events = await one.poll().map(object)
        let changes = try #require(events.first { $0["myco"] as? String == "data" }?["changes"] as? [[String: Any]])
        #expect(changes.map { $0["record"] as? String } == ["b"])
        #expect(await one.poll().isEmpty)
        _ = try await one.answer("data.remove", ["collection": "items", "id": "a"])
        let after = object(try await one.answer("data.list", ["collection": "items"]))
        #expect((after["records"] as? [[String: Any]])?.compactMap { $0["id"] as? String } == ["b"])
    }

    @Test func sendingAsMeIsAskedOnceAndLimited() async throws {
        let hub = FakeHub()
        let (one, doors) = try await bridge(hub)
        let before = await hub.messages(to: "studio-claude-55d0").count
        doors.allow = false
        await #expect(throws: PageBridge.Refusal.self) { _ = try await one.answer("send", ["text": "看一下"]) }
        #expect(await hub.messages(to: "studio-claude-55d0").count == before)
        doors.allow = true
        _ = try await one.answer("send", ["text": "看一下"])
        _ = try await one.answer("send", ["text": "再看一下"])
        #expect(doors.asked.count == 2)
        let sent = await hub.messages(to: "studio-claude-55d0").dropFirst(before)
        #expect(sent.count == 2)
        #expect(sent.first?.body.contains("真机验证清单") == true)
        // At most five a minute.
        for _ in 0..<3 { _ = try await one.answer("send", ["text": "又一条"]) }
        await #expect(throws: PageBridge.Refusal.self) { _ = try await one.answer("send", ["text": "太多了"]) }
        // Empty text is not sent.
        await #expect(throws: PageBridge.Refusal.self) { _ = try await one.answer("send", ["text": "  "]) }
    }

    @Test func anAppMayNotUseWhatItDoesNotDeclare() async throws {
        let hub = FakeHub()
        func bridge(_ template: PageTemplate) -> (PageBridge, Doors) {
            let doors = Doors()
            let page = Page(id: "pg_strict", name: "严格", publisher: "studio-claude-55d0", template: template)
            let one = PageBridge(
                hub: hub, page: page, me: FakeHub.me, workspace: "ws_test", names: { $0 },
                doors: PageBridge.Doors(
                    askToSend: { _, _ in true }, save: { name, data, _ in doors.saved.append((name, data)) },
                    send: { to, body, project in try await hub.send(SendRequest(to: to, body: body, project: project)).id }),
                storage: UserDefaults(suiteName: "page-bridge-\(UUID().uuidString)")!)
            return (one, doors)
        }
        let file = ["name": "a.txt", "base64": Data("a".utf8).base64EncodedString(), "type": "text/plain"]
        // From a hub that says what it lists is all it declares: refused.
        let (strict, saved) = bridge(PageTemplate(id: "tp_a", slug: "alice/a", permissions: ["send"], declares: true))
        do {
            _ = try await strict.answer("download", file)
            Issue.record("an undeclared download was let through")
        } catch let refusal as PageBridge.Refusal {
            #expect(refusal.code == "permission_denied")
        }
        #expect(saved.saved.isEmpty)
        _ = try await strict.answer("send", ["text": "看一下"])
        // An older hub does not say; myco's own apps that declare nothing yet may, for now.
        for template in [
            PageTemplate(id: "tp_a", slug: "alice/a", permissions: ["send"]),
            PageTemplate(id: "tp_myco_doc", slug: "myco/doc", declares: true, official: true),
        ] {
            let (one, doors) = bridge(template)
            _ = try await one.answer("download", file)
            #expect(doors.saved.count == 1)
        }
        let (held, _) = bridge(PageTemplate(id: "tp_myco_disk", slug: "myco/disk", os: ["files"], declares: true, official: true))
        await #expect(throws: PageBridge.Refusal.self) { _ = try await held.answer("download", file) }
    }

    @Test func itKeepsALittleOfItsOwnAndHandsFilesOver() async throws {
        let hub = FakeHub()
        let storage = try #require(UserDefaults(suiteName: "page-bridge-\(UUID().uuidString)"))
        let (one, doors) = try await bridge(hub, storage: storage)
        _ = try await one.answer("storage.set", ["key": "theme", "value": "dark"])
        // Another opening of the same page has it; a big one is refused.
        let (again, _) = try await bridge(hub, storage: storage)
        let value = try JSONSerialization.jsonObject(with: try await again.answer("storage.get", ["key": "theme"]), options: [.fragmentsAllowed])
        #expect(value as? String == "dark")
        await #expect(throws: PageBridge.Refusal.self) {
            _ = try await one.answer("storage.set", ["key": "big", "value": String(repeating: "x", count: 1 << 20)])
        }
        _ = try await one.answer("download", ["name": "清单.csv", "base64": Data("a,b".utf8).base64EncodedString(), "type": "text/csv"])
        #expect(doors.saved.first?.name == "清单.csv")
        #expect(doors.saved.first.map { String(decoding: $0.data, as: UTF8.self) } == "a,b")
        await #expect(throws: PageBridge.Refusal.self) { _ = try await one.answer("nope", [:]) }
    }
}

/// 应用: the apps on their shelves, and a page made from one.
@MainActor
struct AppsTests {
    @Test func appsAreOnShelvesAndAPageIsMadeFromOne() async throws {
        let hub = FakeHub()
        let model = AppsModel(hub: hub)
        await model.load()
        #expect(model.official.map(\.slug) == ["myco/checklist", "myco/whiteboard"])
        #expect(model.shelvesInUse.map(\.key) == ["office", "creative"])
        #expect(model.made.isEmpty)
        #expect(AppsModel.matches(model.official[0], "待办"))
        #expect(!AppsModel.matches(model.official[1], "待办"))
        // Hubs before categories: the office kinds go on the first shelf.
        #expect(AppsModel.shelf(of: AppTemplate(id: "t", slug: "x/t", name: "表格", kind: "table")) == "office")
        #expect(AppsModel.shelf(of: AppTemplate(id: "g", slug: "x/g", name: "游戏")) == "other")
        let page = try #require(await model.use(model.official[0], name: " 买菜 ", project: "", sample: true))
        #expect(page.name == "买菜")
        #expect(page.project == nil)
        #expect(model.made.first?.id == page.id)
        #expect(model.app(of: page)?.slug == "myco/checklist")
        // With its sample records, through the page's bridge.
        let answer = try await hub.pageRequest("GET", page: page.id, path: "data", query: ["collection": "items"], body: nil)
        let records = (try JSONSerialization.jsonObject(with: answer.data) as? [String: Any])?["records"] as? [[String: Any]]
        #expect(records?.count == 2)
        // Read again: the page made is listed.
        await model.load()
        #expect(model.made.map(\.id) == [page.id])
    }

    @Test func anOlderWorkspaceHasNoApps() async {
        let hub = FakeHub()
        await hub.setTooOldForApps(true)
        let model = AppsModel(hub: hub)
        await model.load()
        #expect(model.unsupported)
        #expect(model.list == [])
    }
}
