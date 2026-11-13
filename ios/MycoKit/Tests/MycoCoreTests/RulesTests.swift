import Foundation
import Testing
import MycoAPI
@testable import MycoCore

struct RunStateTests {
    let now = Date(timeIntervalSince1970: 1_800_000_000)

    func task(_ status: String, quietFor seconds: TimeInterval = 5, error: String? = nil) -> Message {
        Message(id: "t", from: "me", to: "coder", body: "do it", status: status, error: error,
                createdAt: now.addingTimeInterval(-seconds - 10), updatedAt: now.addingTimeInterval(-seconds))
    }

    func coder(_ state: String = "working", receiving: String = "runner") -> AgentStatus {
        AgentStatus(agent: Agent(id: "a", name: "coder"), state: state, receiving: receiving)
    }

    @Test func onlyATaskTakenOnCountsAsWorking() {
        // Pulled but not acked, or held, is still waiting.
        for status in ["queued", "delivered", "held"] {
            #expect(RunState.of(task(status), recipient: coder(), now: now) == .queued)
        }
        #expect(RunState.of(task("acked"), recipient: coder(), now: now) == .working(since: now.addingTimeInterval(-5)))
    }

    @Test func quietTooLongIsPossiblyStuck() {
        let elevenMinutes: TimeInterval = 11 * 60
        #expect(RunState.of(task("acked", quietFor: elevenMinutes), recipient: coder(), now: now) == .maybeStuck(quietSince: now.addingTimeInterval(-elevenMinutes)))
        // A node that only receives in its own session gets half an hour.
        guard case .working = RunState.of(task("acked", quietFor: elevenMinutes), recipient: coder(receiving: "session"), now: now) else {
            Issue.record("a session node quiet for 11 minutes is still working")
            return
        }
        // Away: at once.
        guard case .maybeStuck = RunState.of(task("acked"), recipient: coder("offline"), now: now) else {
            Issue.record("an acked task on a node that is away may be stuck")
            return
        }
    }

    @Test func aTaskISToppedIsNotAFailure() {
        #expect(RunState.of(task("failed", error: RunState.stoppedBySender), recipient: nil, now: now) == .stopped)
        #expect(RunState.of(task("failed", error: "boom"), recipient: nil, now: now) == .failed(reason: "boom"))
        #expect(RunState.of(task("done"), recipient: nil, now: now) == .done)
    }

    @Test func theSendButtonFollowsTheOpenTask() {
        #expect(ComposerAction.resolve(hasOpenTask: false, hasText: false) == .send)
        #expect(ComposerAction.resolve(hasOpenTask: false, hasText: true) == .send)
        #expect(ComposerAction.resolve(hasOpenTask: true, hasText: false) == .stop)
        #expect(ComposerAction.resolve(hasOpenTask: true, hasText: true) == .steer)
    }
}

struct NamingTests {
    let machines = [Machine(id: "m1", name: "Studio Mac", hostname: "Studio.local")]

    func agent(_ name: String, display: String? = nil, harness: String = "claude", dir: String = "~/code/myco", kind: String = "agent", day: Double = 0) -> Agent {
        Agent(id: "id-" + name, name: name, displayName: display, binding: Binding(harness: harness, workdir: dir),
              createdAt: Date(timeIntervalSince1970: 1_800_000_000 + day * 86_400), kind: kind, machineId: "m1")
    }

    @Test func aDisplayNameWins() {
        let a = agent("studio-claude-x9k2", display: "后端工程师")
        #expect(Naming.name(of: a, among: [a], machines: machines) == "后端工程师")
    }

    @Test func aNameSomeoneChoseStays() {
        let a = agent("docs-writer")
        #expect(Naming.name(of: a, among: [a], machines: machines) == "docs-writer")
    }

    @Test func aGeneratedNameBecomesTheFolder() {
        let first = agent("studio-claude-x9k2", day: 0)
        let second = agent("laptop-codex-0a1b", harness: "codex", day: 1)
        let third = agent("box-claude-77ff", dir: "/srv/myco/", day: 2)
        let all = [third, first, second]
        AppLanguage.$forced.withValue(.zh) {
            #expect(Naming.name(of: first, among: all, machines: machines) == "myco 助手")
            #expect(Naming.name(of: second, among: all, machines: machines) == "myco 助手 二号")
            #expect(Naming.name(of: third, among: all, machines: machines) == "myco 助手 三号")
        }
        AppLanguage.$forced.withValue(.en) {
            #expect(Naming.name(of: second, among: all, machines: machines) == "myco assistant #2")
        }
    }

    @Test func aComputerIsWhichOneAndTheFolder() {
        let shell = agent("studio-exec-12ab", harness: "exec", dir: "C:\\\\work\\\\site", kind: "shell")
        #expect(Naming.name(of: shell, among: [shell], machines: machines) == "Studio Mac · site")
    }

    @Test func tellsGeneratedNamesFromChosenOnes() {
        #expect(Naming.isGenerated("glenn-mac-claude-x9k2", hostname: nil) == false) // x9k2 is not hex
        #expect(Naming.isGenerated("glenn-mac-claude-9f2a", hostname: nil))
        #expect(Naming.isGenerated("studio-notes", hostname: "Studio.local"))
        #expect(Naming.isGenerated("mac-exec", hostname: "studio") == false)
    }
}

struct ConversationBuilderTests {
    let now = Date(timeIntervalSince1970: 1_800_000_000)
    func at(_ seconds: TimeInterval) -> Date { now.addingTimeInterval(seconds) }

    /// What a conversation shows, without the lines that only tell the time.
    func said(_ items: [ConversationItem]) -> [ConversationItem] {
        items.filter { if case .time = $0.kind { return false } else { return true } }
    }

    @Test func aLongPauseGetsTheTimeWritten() {
        let messages = [
            Message(id: "t1", from: "me", to: "coder", body: "first", status: "done", createdAt: at(-7_200)),
            Message(id: "r1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "reply", body: "done", status: "done", createdAt: at(-7_100)),
            Message(id: "t2", from: "me", to: "coder", body: "second", status: "acked", createdAt: at(-60), updatedAt: at(-5)),
        ]
        let items = ConversationBuilder.items(me: "me", agent: "coder", messages: messages, others: [], status: { _ in nil }, now: now)
        // At the top, and again after nearly two quiet hours; never before
        // the open task's status, which has no time of its own.
        #expect(items.map(\.id) == ["t1.time", "t1", "r1", "t2.time", "t2", "t2.status"])
        #expect(items[3].kind == .time(at(-60)))
        #expect(items[2].kind == .reply(text: "done", taskID: "t1", to: nil))
    }

    @Test func aRoundReadsInOrderWithItsOpenTaskLast() {
        let messages = [
            Message(id: "r1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "reply", body: "done", status: "done", createdAt: at(-80)),
            Message(id: "t1", from: "me", to: "coder", body: "first", status: "done", createdAt: at(-100)),
            Message(id: "t2", from: "me", to: "coder", body: "second", status: "acked", createdAt: at(-60), updatedAt: at(-5)),
            Message(id: "s1", from: "me", to: "coder", threadId: "t2", type: "steer", body: "also this", status: "done", createdAt: at(-30)),
            Message(id: "p1", from: "coder", to: "me", threadId: "t2", type: "progress", body: "halfway", status: "done", createdAt: at(-20)),
        ]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, others: [], status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["t1", "r1", "t2", "s1", "p1", "t2.status"])
        #expect(items[3].kind == .mine(text: "also this", steer: true, by: nil))
        #expect(items[5].kind == .status(.working(since: at(-5)), taskID: "t2"))
        #expect(ConversationBuilder.openTask(me: "me", messages: messages)?.id == "t2")
    }

    @Test func aCoordinatorsHandOutsShowAsCapsulesInTime() {
        let mine = [Message(id: "t1", from: "me", to: "lead", body: "plan it", status: "done", createdAt: at(-100))]
        let others = [Message(id: "d1", from: "lead", to: "coder", body: "write the API\\nwith tests", status: "queued", createdAt: at(-50))]
        let items = said(ConversationBuilder.items(me: "me", agent: "lead", messages: mine, others: others, status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["t1", "d1.dispatch"])
        guard case .dispatch(let dispatch) = items[1].kind else {
            Issue.record("the coordinator's task is a capsule")
            return
        }
        #expect(dispatch.to == "coder")
        #expect(dispatch.state == .queued)
    }

    @Test func aMembersPageIsAllItsWorkWhoeverAsked() {
        // As on the desktop: what I asked and what its colleagues asked, in
        // one conversation; a colleague's task names them, an answer to a
        // colleague says whom it answers, and each task has its own state.
        let messages = [
            Message(id: "c1", from: "lead", to: "coder", body: "design the API", status: "done", createdAt: at(-500)),
            Message(id: "c1r", from: "coder", to: "lead", threadId: "c1", replyTo: "c1", type: "reply", body: "designed", status: "done", createdAt: at(-400)),
            Message(id: "t1", from: "me", to: "coder", body: "and the docs?", status: "done", createdAt: at(-300)),
            Message(id: "t1r", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "reply", body: "on it", status: "done", createdAt: at(-290)),
            Message(id: "c2", from: "lead", to: "coder", body: "now build it", status: "acked", createdAt: at(-200), updatedAt: at(-5)),
            Message(id: "c2p", from: "coder", to: "lead", threadId: "c2", replyTo: "c2", type: "progress", body: "half built", status: "done", createdAt: at(-100)),
            Message(id: "t2", from: "me", to: "coder", body: "ping me after", status: "queued", createdAt: at(-50)),
        ]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, others: [], processes: ["c2"], status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["c1", "c1r", "t1", "t1r", "c2", "c2.process", "c2.status", "c2p", "t2", "t2.status"])
        #expect(items[0].kind == .mine(text: "design the API", steer: false, by: "lead"))
        #expect(items[1].kind == .reply(text: "designed", taskID: "c1", to: "lead"))
        #expect(items[2].kind == .mine(text: "and the docs?", steer: false, by: nil))
        #expect(items[3].kind == .reply(text: "on it", taskID: "t1", to: nil))
        // The colleague's open task keeps its state with it; mine closes the page.
        #expect(items[6].kind == .status(.working(since: at(-5)), taskID: "c2"))
        #expect(items[7].kind == .progress(text: "half built"))
        #expect(items[9].kind == .status(.queued, taskID: "t2"))
        // The input speaks to my open task, never to a colleague's.
        #expect(ConversationBuilder.openTask(me: "me", messages: messages)?.id == "t2")
    }

    @Test func aProjectsFeedTakesInItsMembersLooseThreads() {
        func item(_ id: String, _ parties: [String], project: String? = nil) -> RecentItem {
            let m = Message(id: id, from: parties[0], to: parties[1], body: "x", project: project, createdAt: at(-10))
            return RecentItem(threadId: id, participants: parties, updatedAt: at(-10), last: m, task: m, project: project)
        }
        let project = Project(name: "p", title: "P", members: [ProjectMember(name: "lead", role: "coordinator"), ProjectMember(name: "coder"), ProjectMember(name: "tester")])
        let own = [item("a", ["lead", "coder"], project: "p")]
        let everything = [
            item("a", ["lead", "coder"], project: "p"),      // already in
            item("b", ["lead", "tester"]),                    // among members, in no project: in
            item("c", ["@console:glenn", "coder"]),           // a person and a member: in
            item("d", ["lead", "outsider"]),                  // someone outside: out
            item("e", ["lead", "coder"], project: "other"),   // another project's: out
        ]
        let feed = ProjectFeed.items(of: project, own: own, everything: everything)
        #expect(feed.map(\.threadId) == ["a", "b", "c"])
        #expect(ProjectFeed.colleagues(of: "coder", in: feed, coordinator: "lead") == ["lead"])
        #expect(ProjectFeed.colleagues(of: "tester", in: feed, coordinator: "lead") == ["lead"])
        #expect(ProjectFeed.colleagues(of: "lead", in: feed, coordinator: "lead") == ["coder", "tester"])
    }

    @Test func updatesFoldOnceTheTaskIsAnswered() {
        let messages = [
            Message(id: "t1", from: "me", to: "coder", body: "first", status: "done", createdAt: at(-100)),
            Message(id: "p1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "one", status: "done", createdAt: at(-90)),
            Message(id: "p2", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "two", status: "done", createdAt: at(-80)),
            Message(id: "r1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "reply", body: "done", status: "done", createdAt: at(-70)),
            Message(id: "t2", from: "me", to: "coder", body: "second", status: "acked", createdAt: at(-60), updatedAt: at(-5)),
            Message(id: "p3", from: "coder", to: "me", threadId: "t2", replyTo: "t2", type: "progress", body: "halfway", status: "done", createdAt: at(-20)),
        ]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, others: [], status: { _ in nil }, now: now))
        // The answered task: one line above its answer. The open one: each update.
        #expect(items.map(\.id) == ["t1", "t1.progress", "r1", "t2", "p3", "t2.status"])
        #expect(items[1].kind == .pastProgress(texts: ["one", "two"]))
        #expect(items[4].kind == .progress(text: "halfway"))
    }

    @Test func whatWaitsBehindAnOpenTaskIsNotInTheConversation() {
        let messages = [
            Message(id: "t1", from: "me", to: "coder", body: "first", status: "acked", createdAt: at(-100), updatedAt: at(-5)),
            Message(id: "q1", from: "me", to: "coder", threadId: "t1", after: "t1", body: "then this", status: "queued", createdAt: at(-50)),
            Message(id: "q2", from: "me", to: "coder", threadId: "t1", after: "t1", body: "and this", status: "queued", createdAt: at(-40)),
            // Put after a task that has ended: an ordinary message.
            Message(id: "t0", from: "me", to: "coder", body: "earlier", status: "done", createdAt: at(-300)),
            Message(id: "q0", from: "me", to: "coder", threadId: "t0", after: "t0", body: "after earlier", status: "done", createdAt: at(-250)),
        ]
        #expect(ConversationBuilder.queuedBehind(messages).map(\.id) == ["q1", "q2"])
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, others: [], status: { _ in nil }, now: now))
        // No bubble and no "waiting its turn" line for either of them.
        #expect(items.map(\.id) == ["t0", "q0", "t1", "t1.status"])
        #expect(ConversationBuilder.openTask(me: "me", messages: messages)?.id == "t1")
        #expect(ConversationBuilder.threadToContinue(me: "me", messages: messages) == "t1")
    }

    @Test func aChecklistReportedIsThePlanNotAnUpdate() {
        let messages = [
            Message(id: "t1", from: "me", to: "coder", body: "build it", status: "acked", createdAt: at(-500), updatedAt: at(-5)),
            Message(id: "p1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "- [x] hub\n- [ ] app", status: "done", createdAt: at(-400)),
            Message(id: "p2", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "- [x] hub\n- [~] app\n- [ ] docs", status: "done", createdAt: at(-300)),
            // A checklist with a sentence after it is an update, in words.
            Message(id: "p3", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "- [x] hub pushed\nnotifications come later", status: "done", createdAt: at(-200)),
        ]
        // The steps hold an older plan: the newest one wins.
        let traced: [String: (items: [PlanItem], at: Date?)] = ["t1": ([PlanItem(text: "old", state: .todo), PlanItem(text: "plan", state: .todo)], at(-450))]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, others: [], tracedPlans: traced, status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["t1", "t1.plan", "p3", "t1.status"])
        #expect(items[1].kind == .plan(items: [PlanItem(text: "hub", state: .done), PlanItem(text: "app", state: .doing), PlanItem(text: "docs", state: .todo)], live: true))
        #expect(items[2].kind == .progress(text: "- [x] hub pushed\nnotifications come later"))
    }

    @Test func aFirstLineIsPlainText() {
        #expect(ConversationBuilder.firstLine(of: "定了：**三个 Tab**。\n\n1. 聊天") == "定了：三个 Tab。")
        #expect(ConversationBuilder.firstLine(of: "## Result  `ok`") == "Result ok")
        #expect(ConversationBuilder.firstLine(of: "") == "")
    }

    @Test func aStoppedTaskSaysSo() {
        let messages = [Message(id: "t1", from: "me", to: "coder", body: "x", status: "failed", error: RunState.stoppedBySender, createdAt: at(-10))]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, others: [], status: { _ in nil }, now: now))
        #expect(items.map(\.kind) == [.mine(text: "x", steer: false, by: nil), .stopped])
        #expect(ConversationBuilder.openTask(me: "me", messages: messages) == nil)
    }
}
