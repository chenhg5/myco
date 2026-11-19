import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// What a conversation keeps at hand above its messages.
@MainActor
struct PinTests {
    @Test func aMessagesTitleIsItsFirstWords() {
        #expect(PinsModel.title(of: "## 定了：底部三个 Tab\n\n更多") == "定了：底部三个 Tab")
        #expect(PinsModel.title(of: "看 [这个页面](https://x.example/p) 的结论：") == "看 这个页面 的结论")
        #expect(PinsModel.title(of: "https://only.example/a\n\n第二行才是话") == "第二行才是话")
        #expect(PinsModel.title(of: String(repeating: "长", count: 40)).count == 25)
    }

    @Test func pinsAreKeptByTheWorkspace() async {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        let pins = session.pins(with: "cloud-lead")
        #expect(session.pins(with: "cloud-lead") === pins, "one model a conversation")
        #expect(!pins.supported)
        await pins.load()
        #expect(pins.supported)
        #expect(pins.list.map(\.title) == ["仓库", "定了：底部三个 Tab"])
        #expect(pins.pin(message: "m2") != nil)
        #expect(pins.pin(link: "https://github.com/chenhg5/myco") != nil)

        #expect(await pins.pinMessage("d1", title: "推送接口"))
        #expect(pins.list.count == 3)
        // The same thing again: not twice.
        #expect(await pins.pinLink("https://github.com/chenhg5/myco"))
        #expect(pins.list.count == 3)
        let added = pins.list.last!
        await pins.remove(added)
        #expect(pins.list.count == 2)
        let kept = try? await hub.pins(with: "cloud-lead")
        #expect(kept?.count == 2)

        // The workspace does not answer: the pin comes back, and why is said.
        await hub.setUnreachable(true)
        await pins.remove(pins.list[0])
        #expect(pins.list.count == 2)
        #expect(pins.failure?.kind == .transient)
        await hub.setUnreachable(false)

        // Past twenty: said in words.
        for i in 0..<18 { await pins.pinLink("https://example.com/\(i)") }
        #expect(pins.list.count == 20)
        #expect(!(await pins.pinLink("https://example.com/one-too-many")))
        #expect(pins.failure != nil)

        // A workspace from before pins: none, and nothing offers to pin.
        let old = FakeHub()
        await old.setTooOldForPins(true)
        let none = WorkspaceSession(id: "old", name: "Old", hub: old).pins(with: "cloud-lead")
        await none.load()
        #expect(!none.supported)
        #expect(none.list.isEmpty)
    }
}
