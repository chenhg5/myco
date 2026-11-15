import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// A conversation stays fresh: what was said shows as soon as it comes,
/// whatever else is slow, and the page says when the workspace is silent.
@MainActor
struct RefreshTests {
    static let agent = "studio-claude-c41f"

    func makeConversation() async -> (ConversationModel, FakeHub) {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        let model = session.conversation(with: Self.agent)
        await model.reload()
        return (model, hub)
    }

    /// What the agent said, as the page shows it.
    func said(_ model: ConversationModel) -> [String] {
        model.items.compactMap { if case .reply(let text, _, _) = $0.kind { return text } else { return nil } }
    }

    /// Waits until something is true; answers whether it came in time.
    func until(_ limit: Duration, _ condition: @MainActor () -> Bool) async -> Bool {
        let deadline = ContinuousClock.now + limit
        while ContinuousClock.now < deadline {
            if condition() { return true }
            try? await Task.sleep(for: .milliseconds(10))
        }
        return condition()
    }

    @Test func whatWasSaidShowsWithoutWaitingForTheSteps() async throws {
        let (model, hub) = await makeConversation()
        // A task at work, whose steps take three seconds to come each time.
        let task = try await hub.send(SendRequest(to: Self.agent, body: "看一下首页"))
        await hub.change(task.id) { $0.status = "acked" }
        await hub.setSlowSteps(.seconds(3))
        let watching = Task { await model.run() }
        defer { watching.cancel() }
        #expect(await until(.seconds(1)) { model.openTask?.id == task.id }, "my task shows before its steps do")
        // Something new from the agent: it is on the page at the next
        // look, not when the slow steps have come.
        await hub.deliver(from: Self.agent, body: "新消息到了")
        #expect(await until(.milliseconds(2600)) { said(model).contains("新消息到了") }, "a message that is here is not kept back by a slow answer")
    }

    @Test func readingAgainOnRequestBringsTheStepsToo() async throws {
        let (model, hub) = await makeConversation()
        let task = try await hub.send(SendRequest(to: Self.agent, body: "看一下首页"))
        await hub.change(task.id) { $0.status = "acked" }
        #expect(model.process(of: task.id) == nil)
        await model.reload()
        // All of it: the task and how it is being worked on.
        #expect(model.openTask?.id == task.id)
        #expect(model.process(of: task.id) != nil)
    }

    @Test func aSilentWorkspaceIsSaidAfterTheSecondLook() async {
        let (model, hub) = await makeConversation()
        #expect(model.unreachable == nil)
        let before = model.items
        await hub.setUnreachable(true)
        await model.reload()
        // One look that failed is a blip.
        #expect(model.unreachable == nil)
        await model.reload()
        #expect(model.unreachable != nil)
        // What was there stays.
        #expect(model.items == before)
        await hub.setUnreachable(false)
        await model.reload()
        #expect(model.unreachable == nil)
    }

    @Test func aRefreshThePersonAskedForSaysAtOnceWhenItFails() async {
        let (model, hub) = await makeConversation()
        await hub.deliver(from: Self.agent, body: "刚到的")
        await model.refresh()
        #expect(!model.refreshing)
        #expect(said(model).contains("刚到的"))
        await hub.setUnreachable(true)
        await model.refresh()
        #expect(model.unreachable != nil)
        #expect(!model.refreshing)
    }
}
