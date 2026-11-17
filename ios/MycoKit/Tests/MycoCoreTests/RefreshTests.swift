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
        #expect(await until(.seconds(4)) { said(model).contains("新消息到了") }, "a message that is here is not kept back by a slow answer")
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

    /// What I said, as the page shows it.
    func mine(_ model: ConversationModel) -> [ConversationItem] {
        model.items.filter { if case .mine = $0.kind { return true } else { return false } }
    }

    func text(_ item: ConversationItem) -> String {
        if case .mine(let text, _, _) = item.kind { return text }
        return ""
    }

    @Test func whatISendShowsAtOnceAndTheRealMessageTakesItsPlace() async {
        let (model, hub) = await makeConversation()
        // A workspace that takes a moment to answer.
        await hub.setLatency(.milliseconds(200))
        model.draft = "  翻译一下 readme  "
        let sending = Task { await model.submit() }
        #expect(await until(.milliseconds(150)) { model.outgoing.count == 1 }, "it is on the page before the workspace answers")
        #expect(model.outgoing.first?.text == "翻译一下 readme")
        #expect(model.draft.isEmpty)
        #expect(!mine(model).contains { text($0) == "翻译一下 readme" })
        await sending.value
        // The workspace has it: one message, in place of the one on its way.
        #expect(model.outgoing.isEmpty)
        let arrived = mine(model).filter { text($0) == "翻译一下 readme" }
        #expect(arrived.count == 1)
        #expect(model.placed.contains(arrived.first?.id ?? ""), "it does not arrive a second time")
        #expect(model.problem == nil)
    }

    @Test func whatIsAddedToATaskShowsAtOnceToo() async throws {
        let (model, hub) = await makeConversation()
        let task = try await hub.send(SendRequest(to: Self.agent, body: "看一下首页"))
        await hub.change(task.id) { $0.status = "acked" }
        await model.reload()
        #expect(model.action == .stop)
        await hub.setLatency(.milliseconds(150))
        model.draft = "标题也看看"
        let sending = Task { await model.submit() }
        #expect(await until(.milliseconds(120)) { model.outgoing.first?.text == "标题也看看" })
        await sending.value
        #expect(model.outgoing.isEmpty)
        #expect(await hub.allMessages().contains { $0.type == "steer" && $0.body == "标题也看看" })
    }

    @Test func aTaskThatWasNotDeliveredCanBeSentAgain() async {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        // The workspace gave up on a task of mine: taken, never started on.
        let file = try? await hub.upload(Data([1, 2, 3]), name: "p.png", contentType: "image/png")
        let picture = Attachment(fileId: file?.id ?? "", name: "p.png", size: 3, contentType: "image/png")
        await hub.add(Message(
            id: "lost", from: session.me, to: Self.agent, threadId: "lost-thread", body: "把首页的标题改短", status: "failed",
            error: "delivery attempts exhausted: recipient pulled but never acknowledged", createdAt: Date().addingTimeInterval(-60),
            attachments: [picture]))
        let model = session.conversation(with: Self.agent)
        await model.reload()
        guard case .failure(let reason)? = model.items.first(where: { $0.id == "lost.failed" })?.kind else {
            Issue.record("the failed task says so under it")
            return
        }
        // Said in words, not as the workspace put it.
        AppLanguage.$forced.withValue(.en) {
            #expect(FailureWords.short(reason) == "Not delivered")
            #expect(FailureWords.long(reason).hasPrefix("Not delivered: it picked this up several times"))
            #expect(!FailureWords.long(reason).contains("exhausted"))
            #expect(FailureWords.short("lease expired") == "Timed out")
            #expect(FailureWords.long("request timed out after 30s") == "Timed out: request timed out after 30s")
            #expect(FailureWords.long("the runner crashed") == "Failed: the runner crashed")
            #expect(FailureWords.long("") == "Failed")
            #expect(FailureWords.line(reason) == "Not delivered: it picked this up several times but never started on it.")
            #expect(FailureWords.line("the runner crashed") == "Failed: the runner crashed")
        }

        // Sent again: the same words and file, to the same agent, in the
        // same thread, as a new task. The failed one stays, marked.
        #expect(model.canSendAgain("lost"))
        let before = await hub.allMessages().count
        await model.sendAgain("lost")
        let all = await hub.allMessages()
        #expect(all.count == before + 1)
        let again = all.last
        #expect(again?.id != "lost")
        #expect(again?.body == "把首页的标题改短")
        #expect(again?.to == Self.agent)
        #expect(again?.threadId == "lost-thread")
        #expect(again?.attachments.map(\.fileId) == [picture.fileId])
        #expect(model.resending == nil)
        #expect(model.items.contains { $0.id == "lost.failed" })
        #expect(model.items.contains { $0.id == again?.id })

        // The workspace does not answer: nothing more is sent, and why is said.
        await hub.setUnreachable(true)
        await model.sendAgain("lost")
        #expect(model.problem != nil)
        // Nothing that is not a failed task of mine is sent again.
        await hub.setUnreachable(false)
        let count = await hub.allMessages().count
        await model.sendAgain("nope")
        #expect(!model.canSendAgain("nope"))
        #expect(!model.canSendAgain(again?.id ?? ""), "the new one has not failed")
        #expect(await hub.allMessages().count == count)
    }

    @Test func aSendThatFailsGivesTheWordsBack() async {
        let (model, hub) = await makeConversation()
        await hub.setUnreachable(true)
        model.draft = "这句发不出去"
        await model.submit()
        #expect(model.outgoing.isEmpty, "nothing is left looking sent")
        #expect(model.draft == "这句发不出去")
        #expect(model.problem != nil)
    }

    @Test func aFileAboutToGoCanBeLookedAt() async throws {
        let (model, hub) = await makeConversation()
        let picture = SamplePicture.png(hue: 0.3, width: 40, height: 30)
        await model.attach(picture, name: "照片.png", contentType: "image/png")
        let ready = try #require(model.pending.first)
        // In the workspace already: it is on this device's shelf.
        let kept = try #require(await model.preview(ofPending: ready.id))
        #expect(try Data(contentsOf: kept) == picture)
        // One that did not go up is still here to look at (and to try again).
        await hub.setTooOldForFiles(true)
        await model.attach(Data("# 说明".utf8), name: "说明.md", contentType: "text/markdown")
        let failed = try #require(model.pending.last)
        guard case .failed = failed.state else {
            Issue.record("this workspace takes no files")
            return
        }
        let local = try #require(await model.preview(ofPending: failed.id))
        #expect(local.lastPathComponent == "说明.md")
        #expect(try String(contentsOf: local, encoding: .utf8) == "# 说明")
        #expect(await model.preview(ofPending: "nope") == nil)
    }

    @Test func aListPulledDownSaysItIsBeingReadAgain() async {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        #expect(!session.refreshingByHand)
        let pulling = Task { await session.refreshByHand(atLeast: .milliseconds(200)) }
        #expect(await until(.milliseconds(150)) { session.refreshingByHand })
        // A second pull meanwhile is the same refresh.
        await session.refreshByHand()
        #expect(session.refreshingByHand)
        await pulling.value
        #expect(!session.refreshingByHand)
        #expect(session.problem == nil)
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
