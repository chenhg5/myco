import Foundation
import Testing
import MycoAPI
@testable import MycoCore
import MycoFake

/// What waits for me, and where the tokens go.
@MainActor
struct NoticesTests {
    @Test func whatWaitsIsListedOncePerPlaceAndReadWhenOpened() async throws {
        let hub = FakeHub()
        let model = NoticesModel(hub: hub)
        await model.load()
        // 运维 asks, a task failed; the answer already read is not listed.
        #expect(model.waiting.map(\.kind) == ["asking", "failed"])
        #expect(model.badge == 2)
        // Opening a failure settles it; a question waits for its answer.
        await model.read(try #require(model.waiting.last))
        #expect(model.waiting.map(\.kind) == ["asking"])
        await model.read(try #require(model.waiting.first))
        #expect(model.waiting.map(\.kind) == ["asking"])
        _ = try await hub.send(SendRequest(to: "vps-codex-90aa", body: "现在重启吧"))
        await model.load()
        #expect(model.waiting.isEmpty)
    }

    @Test func aDecisionOrAnApprovalIsLeftToWhereItIsAnswered() async {
        let notices = [
            Notice(seq: 1, id: "a", kind: "decision", target: "workflow:w1?run=r1", createdAt: Date()),
            Notice(seq: 2, id: "b", kind: "approval", target: "superagent", createdAt: Date()),
            Notice(seq: 3, id: "c", kind: "stuck", target: "agent:x", createdAt: Date()),
            Notice(seq: 4, id: "d", kind: "failed", target: "agent:x", createdAt: Date().addingTimeInterval(-60)),
        ]
        #expect(notices.filter { $0.open && !NoticesModel.shownElsewhere.contains($0.kind) }.map(\.id) == ["c", "d"])
    }

    @Test func usageIsSummedAsTheDesktopSumsIt() async {
        let model = UsageModel(hub: FakeHub())
        model.summarize([
            UsageRow(day: "2026-11-20", agent: "a", project: "p", model: "m1", calls: 2, inputTokens: 100, cacheWriteTokens: 10, cacheReadTokens: 1_000, outputTokens: 50, costUsd: 0.5),
            UsageRow(day: "2026-11-21", agent: "b", model: "m2", calls: 1, inputTokens: 10, outputTokens: 5),
            UsageRow(day: "2026-11-21", agent: "a", project: "p", model: "m1", calls: 1, inputTokens: 1, outputTokens: 1),
        ], days: ["2026-11-19", "2026-11-20", "2026-11-21"])
        // Input, cache written and read, output: they do not overlap.
        #expect(model.total.total == 1_177)
        #expect(model.total.output == 56)
        #expect(model.total.calls == 4)
        #expect(model.byDay.map(\.total) == [0, 1_160, 17])
        #expect(model.agents.map(\.name) == ["a", "b"])
        #expect(model.projects.first?.name == "p")
        #expect(UsageModel.compact(1_234_567) == "1.2M")
        #expect(UsageModel.compact(340_000) == "340K")
        #expect(UsageModel.compact(12) == "12")
        #expect(UsageModel.range(days: 7, now: Date(timeIntervalSince1970: 1_795_000_000)).all.count == 7)
    }

    @Test func theDemoReportsTwoWeeksOfTokens() async {
        let model = UsageModel(hub: FakeHub())
        await model.load(days: 7)
        #expect(model.loaded && !model.unsupported)
        #expect(model.byDay.count == 7)
        #expect(model.total.total > 0)
        #expect(model.agents.first?.name == "studio-codex-7f3a")
    }
}
