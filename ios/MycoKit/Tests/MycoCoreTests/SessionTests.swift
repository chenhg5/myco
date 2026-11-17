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
        #expect(model.process(of: "h1")?.segments.map(\.id) == RunProcess(events: FakeHub.scriptForTests).segments.map(\.id))
        await loading.value

        // Two refreshes at once (the timer's and one after sending) do not
        // bring a task's steps twice.
        let again = session.conversation(with: "vps-codex-90aa")
        async let first: Void = again.reload()
        async let second: Void = again.reload()
        _ = await (first, second)
        let steps = try #require(again.process(of: "o1"))
        let once = RunProcess(events: Array(FakeHub.scriptForTests.prefix(5)))
        #expect(steps.segments.map(\.id) == once.segments.map(\.id))
        guard case .steps(_, let last) = steps.segments.last, case .steps(_, let expected) = once.segments.last else {
            Issue.record("the run ends with steps")
            return
        }
        #expect(last.map(\.id) == expected.map(\.id))
    }

    @Test func aMessageTakenOutOfMyViewIsGoneOnEveryDevice() async throws {
        let hub = FakeHub(scenario: .waiting)
        let session = WorkspaceSession(id: "hide", name: "Demo", hub: hub)
        await session.refresh()
        // The coordinator reported on a task of mine.
        let model = session.conversation(with: "cloud-lead", project: "myco-ios")
        await model.reload()
        #expect(model.items.contains { $0.id == "m3" })
        // Its update shows: a row, or part of its run.
        func reported(_ model: ConversationModel, _ id: String) -> Bool {
            model.items.contains { $0.id == id } || model.run(of: "m3").contains { part in
                if case .reports(_, let reports) = part { return reports.contains { $0.id == id } } else { return false }
            }
        }
        #expect(reported(model, "m3p"))

        // The task goes, and its updates with it; nothing is deleted.
        await model.setHidden(["m3"])
        #expect(!model.items.contains { $0.id == "m3" || $0.id.hasPrefix("m3.") })
        #expect(!reported(model, "m3p"))
        #expect(await hub.allMessages().contains { $0.id == "m3" })
        #expect(try await hub.hidden() == ["m3"])

        // Another device of mine sees the same.
        let phone = WorkspaceSession(id: "hide-2", name: "Demo", hub: hub)
        await phone.refresh()
        #expect(phone.hidden == ["m3"])
        let there = phone.conversation(with: "cloud-lead", project: "myco-ios")
        await there.reload()
        #expect(!there.items.contains { $0.id == "m3" || $0.id.hasPrefix("m3.") })
        #expect(!reported(there, "m3p"))

        // Taken back within a moment: all of it is there again.
        await model.setHidden(["m3"], hidden: false)
        #expect(model.items.contains { $0.id == "m3" })
        #expect(reported(model, "m3p"))
    }

    @Test func anOlderWorkspaceKeepsWhatIHidOnThisDevice() async throws {
        let hub = FakeHub()
        await hub.setTooOldForHidden(true)
        let store = MemoryStore()
        let session = WorkspaceSession(id: "old", name: "Demo", hub: hub, snapshots: store)
        await session.refresh()
        let model = session.conversation(with: "studio-claude-c41f")
        await model.reload()
        await model.setHidden(["h2"])
        #expect(model.problem == nil)
        #expect(!model.items.contains { $0.id == "h2" })
        // The next launch still knows.
        let again = WorkspaceSession(id: "old", name: "Demo", hub: hub, snapshots: store)
        await again.refresh()
        #expect(again.hidden == ["h2"])
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

/// A computer added with one command, run in the folder agents should use.
@MainActor
struct ComputerTests {
    @Test func theCommandIsOneLineForEachSystem() {
        let link = JoinLink(joinUrl: "https://hub.example.com/j/myco_enroll_x")
        #expect(ComputerCommand.line(for: link, on: .shell) == "curl -fsSL https://hub.example.com/j/myco_enroll_x | bash")
        #expect(ComputerCommand.line(for: link, on: .powershell) == "irm 'https://hub.example.com/j/myco_enroll_x?format=ps1' | iex")
        let nobody = ComputerCommand.nobody()
        #expect(ComputerCommand.isNobody(nobody))
        #expect(nobody != ComputerCommand.nobody())
        #expect(!ComputerCommand.isNobody("studio-codex-7f3a"))
    }

    @Test func theFolderShowsUpAndGoesIntoTheProject() async throws {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        // The project's agents are the ones it lets in (not its computers, not me).
        let users = session.computerUsers(of: "myco-ios")
        #expect(users.contains("cloud-lead"))
        #expect(users.contains("studio-codex-7f3a"))
        #expect(session.computerUsers(of: nil).isEmpty)
        #expect(session.computerUsers(of: "nope").isEmpty)
        let before = Set(session.computers.map(\.name))
        let link = try await session.computerLink(for: "myco-ios")
        #expect(link.joinUrl.contains("/j/"))
        #expect(session.computer(newSince: before) == nil)
        // (the demo "runs the command" a few seconds later)
        var fresh: Agent?
        for _ in 0..<40 where fresh == nil {
            try? await Task.sleep(for: .milliseconds(200))
            await session.refresh()
            fresh = session.computer(newSince: before)
        }
        let computer = try #require(fresh)
        #expect(Naming.kind(of: computer) == .shell)
        #expect(await session.add(computer.name, toProject: "myco-ios") == nil)
        #expect(session.project("myco-ios")?.members.contains { $0.name == computer.name } == true)
        // A project that is not there: said.
        #expect(await session.add(computer.name, toProject: "nope") != nil)
        // Without a project: for nobody yet (the workspace wants a name).
        _ = try await session.computerLink(for: nil)
        // Not the owner or an admin: refused, as the workspace does.
        await hub.setMayNotAddComputers(true)
        await #expect(throws: HubError.self) { _ = try await session.computerLink(for: "myco-ios") }
    }
}

/// 「换一个来接着做」: another session takes an agent over.
@MainActor
struct HandoverTests {
    let studio = Machine(id: "mach_studio", name: "Studio Mac", canSpawn: true, mycoVersion: "v0.19.25")

    @Test func aSideIsSaidAsPeopleSayIt() {
        AppLanguage.$forced.withValue(.zh) {
            let onMac = HandoverSide(binding: Binding(harness: "codex"), machineId: "mach_studio", runtime: Runtime(cli: "codex"))
            #expect(HandoverWords.side(onMac, machines: [studio]) == "Studio Mac · Codex")
            #expect(HandoverWords.side(HandoverSide(binding: Binding(harness: "claude"), machineName: "mini"), machines: []) == "mini · Claude Code")
            #expect(HandoverWords.side(HandoverSide(binding: Binding(harness: "claude-cloud")), machines: []) == "claude.ai 云端")
            #expect(HandoverWords.side(HandoverSide(binding: Binding(harness: "codex-cloud")), machines: []) == "Codex 云端")
            #expect(HandoverWords.side(HandoverSide(binding: Binding(harness: "inbox")), machines: []) == "一个开着的会话")
            #expect(HandoverWords.side(HandoverSide(), machines: []) == "另一个会话")
            #expect(HandoverWords.side(nil, machines: []) == "")
            // The line a conversation draws where it took effect.
            var done = Handover(id: "hov_1", agent: "coder", at: Date(), by: "glenn", to: onMac, state: "done", doneAt: Date())
            #expect(HandoverWords.line(done, machines: [studio], me: "@console:glenn") == "换成 Studio Mac · Codex 接着做（你换的）")
            #expect(HandoverWords.line(done, machines: [studio], me: "@console:lee") == "换成 Studio Mac · Codex 接着做（glenn换的）")
            done.back = "hov_0"
            done.by = "admin"
            #expect(HandoverWords.line(done, machines: [studio], me: "@console:glenn") == "换回原来的会话 · Studio Mac · Codex（管理员换的）")
            #expect(HandoverWords.state(done) == nil)
            done.state = "waiting"
            #expect(HandoverWords.state(done) == "等它接手")
            done.state = "expired"
            #expect(HandoverWords.state(done) == "没用上")
        }
    }

    @Test func whatWillHappenIsSaidBeforeItDoes() {
        AppLanguage.$forced.withValue(.en) {
            #expect(HandoverWords.consequences(back: false, open: 0, working: false).count == 2)
            let busy = HandoverWords.consequences(back: false, open: 2, working: true)
            #expect(busy.count == 4)
            #expect(busy[2] == "The 2 tasks it has not finished go to the new session to redo.")
            let back = HandoverWords.consequences(back: true, open: 1, working: false)
            #expect(back.count == 2)
            #expect(back[1] == "The 1 task it has not finished go to the earlier session to redo.")
        }
    }

    @Test func aComputerNeedsANewEnoughMyco() {
        #expect(HandoverWords.canTakeOver(mycoVersion: "v0.19.23"))
        #expect(HandoverWords.canTakeOver(mycoVersion: "0.20.0"))
        #expect(HandoverWords.canTakeOver(mycoVersion: "v0.19.22-7-g8484653"), "a build from a later main")
        #expect(!HandoverWords.canTakeOver(mycoVersion: "v0.19.22"))
        #expect(!HandoverWords.canTakeOver(mycoVersion: "v0.19.9"))
        #expect(!HandoverWords.canTakeOver(mycoVersion: "v0.19.21-3-gabcdef0"))
        #expect(!HandoverWords.canTakeOver(mycoVersion: nil))
    }

    @Test func anEarlierSessionCanBeSwitchedBackToOnce() {
        let now = Agent(id: "a", name: "coder", binding: Binding(harness: "codex", sessionId: "sess_now"))
        let first = Handover(
            id: "hov_1", agent: "coder", at: Date(), from: HandoverSide(binding: Binding(harness: "claude-cloud", sessionId: "sess_before")),
            state: "done")
        #expect(HandoverWords.canGoBack(first, among: [first], agent: now))
        // Already undone; the session answering now; one that never took effect.
        let undo = Handover(id: "hov_2", agent: "coder", at: Date(), state: "done", back: "hov_1")
        #expect(!HandoverWords.canGoBack(first, among: [undo, first], agent: now))
        let same = Agent(id: "a", name: "coder", binding: Binding(harness: "claude-cloud", sessionId: "sess_before"))
        #expect(!HandoverWords.canGoBack(first, among: [first], agent: same))
        var waiting = first
        waiting.state = "waiting"
        #expect(!HandoverWords.canGoBack(waiting, among: [waiting], agent: now))
    }

    @Test func aNewSessionOnAComputerTakesOver() async throws {
        let hub = FakeHub(scenario: .session)
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        let backend = "studio-codex-7f3a"
        let model = session.handovers(of: backend)
        await model.load()
        #expect(model.list.map(\.id) == ["hov_demo1"])
        #expect(session.spawners.map(\.name) == ["Studio Mac"])
        #expect(session.folders(on: "mach_studio").contains("~/code/myco"))
        // In my conversation with it: a line where the earlier one took effect,
        // when that is within what is on the page (two days ago: it is not).
        let chat = session.conversation(with: backend)
        await chat.reload()
        #expect(!chat.items.contains { if case .takenOver = $0.kind { return true } else { return false } })

        let result = try await model.handOver(HandoverRequest(machine: "mach_studio", cli: "claude", dir: "~/code/site", note: "换个模型试试"))
        #expect(result.handover.state == "done")
        #expect(result.spawn?.reply?.isEmpty == false)
        #expect(model.list.first?.id == result.handover.id)
        await session.refresh()
        #expect(session.agent(backend)?.binding.workdir == "~/code/site")
        #expect(Naming.cli(of: try #require(session.agent(backend))) == "claude")
        // The line is on the page now, after what was said before.
        await chat.reload()
        let line = chat.items.first { if case .takenOver = $0.kind { return true } else { return false } }
        guard case .takenOver(let text)? = line?.kind else {
            Issue.record("the conversation says where the new session took over")
            return
        }
        AppLanguage.$forced.withValue(.en) {
            #expect(HandoverWords.line(result.handover, machines: session.machines, me: session.me) == "Studio Mac · Claude Code carries on from here (by you)")
        }
        #expect(!text.isEmpty)
        let order = chat.items.map(\.id)
        let place = order.firstIndex(of: line?.id ?? "") ?? -1
        #expect(place > (order.firstIndex(of: "b1") ?? .max), "after what was said before it")

        // Back to the session before that one: its own computer, AI and folder.
        let back = try await model.handOver(HandoverRequest(machine: "mach_studio", back: result.handover.id))
        #expect(back.handover.back == result.handover.id)
        await session.refresh()
        #expect(session.agent(backend)?.binding.workdir == "~/code/myco")
        let agent = try #require(session.agent(backend))
        #expect(!HandoverWords.canGoBack(result.handover, among: model.list, agent: agent), "undone once")
    }

    @Test func aCommandLetsAnySessionTakeOver() async throws {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        let model = session.handovers(of: "studio-codex-7f3a")
        let link = try await model.link(note: nil, back: nil)
        #expect(link.command?.hasPrefix("curl -fsSL '") == true)
        #expect(link.commandPs1?.hasSuffix("| iex") == true)
        #expect(model.list.first?.state == "waiting")
        #expect(await model.arrived(link.handover.id) == nil)
        // (the demo "runs the command" a few seconds later)
        var taken: Handover?
        for _ in 0..<40 where taken == nil {
            try? await Task.sleep(for: .milliseconds(200))
            taken = await model.arrived(link.handover.id)
        }
        #expect(taken?.state == "done")
        #expect(HandoverWords.side(taken?.to, machines: []) .contains("claude.ai"))
        // A workspace from before handovers: none, and said so.
        await hub.setTooOldForHandovers(true)
        await model.load(force: true)
        #expect(model.unsupported)
        await #expect(throws: HubError.self) { _ = try await model.link(note: nil, back: nil) }
    }
}
