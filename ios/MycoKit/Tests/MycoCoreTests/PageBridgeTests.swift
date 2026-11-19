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
        doors.allow = false
        await #expect(throws: PageBridge.Refusal.self) { _ = try await one.answer("send", ["text": "看一下"]) }
        #expect(await hub.messages(to: "studio-claude-55d0").isEmpty)
        doors.allow = true
        _ = try await one.answer("send", ["text": "看一下"])
        _ = try await one.answer("send", ["text": "再看一下"])
        #expect(doors.asked.count == 2)
        let sent = await hub.messages(to: "studio-claude-55d0")
        #expect(sent.count == 2)
        #expect(sent.first?.body.contains("真机验证清单") == true)
        // At most five a minute.
        for _ in 0..<3 { _ = try await one.answer("send", ["text": "又一条"]) }
        await #expect(throws: PageBridge.Refusal.self) { _ = try await one.answer("send", ["text": "太多了"]) }
        // Empty text is not sent.
        await #expect(throws: PageBridge.Refusal.self) { _ = try await one.answer("send", ["text": "  "]) }
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
