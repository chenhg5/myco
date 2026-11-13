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

        // Another workspace's snapshot is not this one's.
        let other = WorkspaceSession(id: "w2", name: "Other", hub: hub, snapshots: store)
        await other.restore()
        #expect(other.members.isEmpty)
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
        #expect(model.items.contains { $0.kind == .mine(text: "British spelling", steer: true) })

        model.draft = "then the changelog"
        model.queueNext()
        #expect(model.queued == ["then the changelog"])
        #expect(model.draft.isEmpty)

        await model.submit() // stop
        #expect(model.items.contains { $0.kind == .stopped })
        // The open task closed, so the queued message went out as a new one.
        #expect(model.queued.isEmpty)
        #expect(model.openTask?.body == "then the changelog")
    }
}
