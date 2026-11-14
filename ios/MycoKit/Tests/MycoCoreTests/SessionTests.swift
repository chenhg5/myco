import Foundation
import Testing
import MycoAPI
import MycoFake
import MycoStore
@testable import MycoCore

/// The session and a conversation against the fake workspace: what the
/// pages read, without any page.
@MainActor
struct SessionTests {
    func makeSession(_ scenario: Scenario = .team) async -> (WorkspaceSession, FakeHub) {
        let hub = FakeHub(scenario: scenario)
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        return (session, hub)
    }

    @Test func loadsMembersProjectsAndWhoIAm() async {
        let (session, _) = await makeSession()
        #expect(session.me == FakeHub.me)
        #expect(session.problem == nil)
        #expect(session.members.count == 8)
        let project = try! #require(session.project("myco-ios"))
        #expect(session.coordinator(of: project) == "cloud-lead")
        // The coordinator first, those away last.
        let order = session.memberStatuses(of: project).map(\.agent.name)
        #expect(order.first == "cloud-lead")
        #expect(order.last == "laptop-claude-a1e2")
    }

    @Test func chatsListsTheProjectAndTheAgentsITalkedTo() async {
        let (session, _) = await makeSession()
        let rows = session.chatRows
        let targets = Set(rows.map(\.target))
        #expect(targets.contains(.project("myco-ios")))
        #expect(targets.contains(.agent("vps-codex-90aa")))
        #expect(targets.contains(.agent("studio-claude-c41f")))
        // Talked to directly inside a project: a chat of its own, apart
        // from the project's row (which is its coordinator).
        #expect(targets.contains(.agent("studio-codex-7f3a")))
        #expect(!targets.contains(.agent("cloud-lead")))
        // Never talked to, and in no thread of mine: not in Chats.
        #expect(!targets.contains(.agent("studio-codex-e07b")))
        // Quiet for two hours after taking a task on: possibly stuck.
        let ops = try! #require(rows.first { $0.target == .agent("vps-codex-90aa") })
        guard case .maybeStuck = ops.state else {
            Issue.record("ops took a task two hours ago and went quiet")
            return
        }
        // And the member itself reads as possibly stuck, not as at work.
        let opsStatus = try! #require(session.status(of: "vps-codex-90aa"))
        #expect(MemberState.of(opsStatus) == .possiblyStuck)
        let backend = try! #require(session.status(of: "studio-codex-7f3a"))
        #expect(MemberState.of(backend) == .working(text: nil))
    }

    @Test func anUnreachableWorkspaceKeepsWhatItShowed() async {
        let (session, hub) = await makeSession()
        await hub.setUnreachable(true)
        await session.refresh()
        #expect(session.problem != nil)
        #expect(session.members.count == 8)
        await hub.setUnreachable(false)
        await session.refresh()
        #expect(session.problem == nil)
    }

    @Test func aConversationsFirstLookIsWhole() async throws {
        // A slow workspace: every answer takes a moment.
        let hub = FakeHub(latency: .milliseconds(40))
        let session = WorkspaceSession(id: "slow", name: "Demo", hub: hub)
        await session.refresh()
        let model = session.conversation(with: "studio-claude-c41f")
        #expect(!model.loaded)
        let loading = Task { await model.reload() }
        // Nothing shows until all of it is here: no page that fills in by jumps.
        while !model.loaded { try await Task.sleep(for: .milliseconds(5)) }
        #expect(model.items.contains { if case .process = $0.kind { return true } else { return false } })
        #expect(model.process(of: "h1") == RunProcess(events: FakeHub.scriptForTests))
        await loading.value

        // Two refreshes at once (the timer's and one after sending) do not
        // bring a task's steps twice.
        let again = session.conversation(with: "vps-codex-90aa")
        async let first: Void = again.reload()
        async let second: Void = again.reload()
        _ = await (first, second)
        let steps = try #require(again.process(of: "o1"))
        let once = RunProcess(events: Array(FakeHub.scriptForTests.prefix(5)))
        #expect(steps == once)
    }

    @Test func opensOnWhatItShowedLast() async {
        let store = MemoryStore()
        let first = WorkspaceSession(id: "w1", name: "Demo", hub: FakeHub(), snapshots: store)
        await first.refresh()
        #expect(first.members.count == 8)

        // The next launch: the workspace does not answer, the lists are there.
        let hub = FakeHub()
        await hub.setUnreachable(true)
        let second = WorkspaceSession(id: "w1", name: "Demo", hub: hub, snapshots: store)
        #expect(second.loadedAt == nil)
        await second.restore()
        #expect(second.members.count == 8)
        #expect(second.me == FakeHub.me)
        #expect(second.loadedAt != nil)
        #expect(second.chatRows.isEmpty == false)
        await second.refresh()
        #expect(second.problem != nil)
        #expect(second.members.count == 8)

        // Leaving a workspace forgets what was kept for it.
        await store.removeSnapshots(of: "w1")
        let third = WorkspaceSession(id: "w1", name: "Demo", hub: hub, snapshots: store)
        await third.restore()
        #expect(third.members.isEmpty)

        // Another workspace's snapshot is not this one's.
        let other = WorkspaceSession(id: "w2", name: "Other", hub: hub, snapshots: store)
        await other.restore()
        #expect(other.members.isEmpty)
    }

    @Test func whatCameSinceILookedIsUnread() async {
        let store = MemoryStore()
        let hub = FakeHub()
        let session = WorkspaceSession(id: "w1", name: "Demo", hub: hub, snapshots: store)
        await session.refresh()
        // The first look at a workspace: what was already there is not news.
        #expect(session.unreadCount == 0)

        await hub.deliver(from: "studio-claude-c41f", body: "还有一件事。")
        await hub.deliver(from: "cloud-lead", body: "活都派完了。")
        await session.refresh()
        let unread = Set(session.chatRows.filter(\.unread).map(\.target))
        // The coordinator's words light up its project's row.
        #expect(unread == [.agent("studio-claude-c41f"), .project("myco-ios")])
        #expect(session.unreadCount == 2)

        session.markRead("studio-claude-c41f")
        #expect(Set(session.chatRows.filter(\.unread).map(\.target)) == [.project("myco-ios")])

        // What I say myself is never unread.
        _ = try? await hub.send(SendRequest(to: "vps-codex-90aa", body: "还在吗"))
        await session.refresh()
        #expect(session.chatRows.first { $0.target == .agent("vps-codex-90aa") }?.unread == false)

        // The next launch remembers what was read.
        try? await Task.sleep(for: .milliseconds(50))
        let again = WorkspaceSession(id: "w1", name: "Demo", hub: hub, snapshots: store)
        await again.restore()
        await again.refresh()
        #expect(Set(again.chatRows.filter(\.unread).map(\.target)) == [.project("myco-ios")])
    }

    @Test func scrollingBackLoadsOlderPages() async {
        let session = WorkspaceSession(id: "w1", name: "Demo", hub: FakeHub(scenario: .long))
        await session.refresh()
        let model = session.conversation(with: "studio-claude-c41f")
        await model.reload()
        func questions() -> Int { model.items.filter { if case .mine = $0.kind { return true } else { return false } }.count }
        // The newest page only, and more behind it.
        #expect(model.hasEarlier)
        #expect(questions() == FakeHub.pageSize / 2)
        #expect(model.items.contains { $0.id == "L199a" })
        #expect(!model.items.contains { $0.id == "L0q" })

        #expect(await model.loadEarlier())
        #expect(questions() == FakeHub.pageSize)
        // A refresh of the newest page does not drop what was scrolled back to.
        await model.reload()
        #expect(questions() == FakeHub.pageSize)
        #expect(model.hasEarlier)

        var pages = 0
        while model.hasEarlier, pages < 20 {
            await model.loadEarlier()
            pages += 1
        }
        // All of it, once each, in order.
        #expect(!model.hasEarlier)
        #expect(model.items.contains { $0.id == "L0q" })
        let ids = model.items.map(\.id)
        #expect(Set(ids).count == ids.count)
        #expect(model.items.map(\.at) == model.items.map(\.at).sorted())
        #expect(await model.loadEarlier() == false)
    }

    @Test func aWorkspaceAtAnAddressGetsAPlainName() {
        func record(_ name: String, _ url: String) -> WorkspaceRecord { WorkspaceRecord(id: "x", name: name, url: URL(string: url)!) }
        AppLanguage.$forced.withValue(.en) {
            #expect(Workspaces.displayName(of: record("193-46-198-17.sslip.io", "https://193-46-198-17.sslip.io")) == "My workspace")
            #expect(Workspaces.displayName(of: record("192.168.1.20", "http://192.168.1.20:7777")) == "My workspace")
            #expect(Workspaces.displayName(of: record("hub.example.com", "https://hub.example.com")) == "hub.example.com")
            // A name someone gave it stays.
            #expect(Workspaces.displayName(of: record("Studio", "https://193-46-198-17.sslip.io")) == "Studio")
        }
    }

    @Test func anEmptyWorkspaceLoadsWithNobody() async {
        let (session, _) = await makeSession(.empty)
        #expect(session.loadedAt != nil)
        #expect(session.members.isEmpty)
        #expect(session.chatRows.isEmpty)
    }

    @Test func sendingThenAddingThenStopping() async {
        let (session, _) = await makeSession()
        let model = ConversationModel(session: session, agent: "studio-codex-e07b")
        await model.reload()
        #expect(model.loaded)
        #expect(model.items.isEmpty)
        #expect(model.action == .send)
        #expect(!model.canSubmit)

        model.draft = "translate the readme"
        await model.submit()
        #expect(model.draft.isEmpty)
        #expect(model.openTask != nil)
        // An open task and nothing typed: the button stops it.
        #expect(model.action == .stop)

        model.draft = "British spelling"
        #expect(model.action == .steer)
        await model.submit()
        #expect(model.items.contains { $0.kind == .mine(text: "British spelling", steer: true, by: nil) })

        // Put behind the open task: the workspace holds it. It waits above
        // the input, and is not part of the conversation yet.
        model.draft = "then the changelog"
        await model.queueNext()
        #expect(model.draft.isEmpty)
        #expect(model.waiting.map(\.body) == ["then the changelog"])
        #expect(model.waiting.first?.onHub == true)
        #expect(!model.items.contains { $0.kind == .mine(text: "then the changelog", steer: false, by: nil) })
        // The input still speaks to the task being worked on.
        #expect(model.openTask?.body == "translate the readme")

        // A second one, taken back to edit: gone from the queue, its words
        // in the input.
        model.draft = "and the license"
        await model.queueNext()
        #expect(model.waiting.count == 2)
        let second = try! #require(model.waiting.last)
        await model.withdraw(second, edit: true)
        #expect(model.waiting.map(\.body) == ["then the changelog"])
        #expect(model.draft == "and the license")
        model.draft = ""

        await model.submit() // stop
        #expect(model.items.contains { $0.kind == .stopped })
        // The open task ended: what waited is an ordinary message now.
        await model.reload()
        #expect(model.waiting.isEmpty)
        #expect(model.items.contains { $0.kind == .mine(text: "then the changelog", steer: false, by: nil) })
        #expect(model.openTask?.body == "then the changelog")
    }
}
