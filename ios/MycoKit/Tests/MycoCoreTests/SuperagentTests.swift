import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// My superagent: naming one, starting a Shroom for it, and what it waits
/// on me for.
@MainActor
struct SuperagentTests {
    @Test func oneOfMyAgentsBecomesItAndItsRequestsWaitForMe() async throws {
        let hub = FakeHub()
        let model = SuperagentModel(hub: hub)
        await model.load()
        #expect(model.loaded)
        #expect(model.agent == nil)
        #expect(await model.set("studio-claude-c41f"))
        #expect(model.agent == "studio-claude-c41f")
        await hub.addApproval(what: "删除项目「旧官网」", reason: "已经合并进 myco-web")
        await hub.addApproval(what: "停止测试的任务")
        await model.load()
        #expect(model.pending == 2)
        #expect(model.approvals.map(\.what) == ["删除项目「旧官网」", "停止测试的任务"])
        await model.decide(model.approvals[0], approve: true)
        await model.decide(model.approvals[0], approve: false)
        #expect(model.approvals.isEmpty)
        #expect(model.pending == 0)
        #expect(try await hub.approvals().isEmpty)
        // None any more.
        #expect(await model.set(nil))
        #expect(model.agent == nil)
    }

    @Test func aShroomIsStartedOnOneOfMyComputers() async throws {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        let computer = try #require(session.machines.first)
        let name = try #require(await session.superagent.start(on: computer.id, cli: "codex"))
        #expect(session.superagent.agent == name)
        let asked = try #require(await hub.spawned.last)
        #expect(asked.request.displayName == "Shroom")
        #expect(asked.request.dir == "~/myco/shroom")
        #expect(asked.request.access == "workspace")
    }

    @Test func anOlderWorkspaceHasNone() async {
        let hub = FakeHub()
        await hub.setTooOldForSuperagents(true)
        let model = SuperagentModel(hub: hub)
        await model.load()
        #expect(model.unsupported)
    }

    @Test func aRequestSaysHowLongItMayWait() {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        func approval(_ left: TimeInterval) -> Approval {
            Approval(id: "a", agent: "s", what: "x", createdAt: now, expiresAt: now.addingTimeInterval(left))
        }
        AppLanguage.$forced.withValue(.zh) {
            #expect(SuperagentModel.left(approval(5 * 3_600 + 60), now: now) == "5 小时内有效")
            #expect(SuperagentModel.left(approval(600), now: now) == "10 分钟内有效")
            #expect(SuperagentModel.left(approval(-1), now: now) == "已过期")
        }
    }
}
