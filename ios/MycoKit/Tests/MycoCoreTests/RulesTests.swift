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
        let items = ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now)
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
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["t1", "r1", "t2", "s1", "p1", "t2.status"])
        #expect(items[3].kind == .mine(text: "also this", steer: true, by: nil))
        #expect(items[5].kind == .status(.working(since: at(-5)), taskID: "t2"))
        #expect(ConversationBuilder.openTask(me: "me", messages: messages)?.id == "t2")
    }

    @Test func whatAgentsHandEachOtherIsACardNotMessages() {
        // A member's page: what I said to it is the conversation; what it
        // and its colleagues said to each other is one card per exchange,
        // where each began.
        let between = [
            Message(id: "c1", from: "lead", to: "coder", body: "design the API\nwith tests", status: "done", createdAt: at(-500), updatedAt: at(-400)),
            Message(id: "c1p", from: "coder", to: "lead", threadId: "c1", replyTo: "c1", type: "progress", body: "half way", status: "done", createdAt: at(-450)),
            Message(id: "c1s", from: "lead", to: "coder", threadId: "c1", replyTo: "c1", type: "steer", body: "also errors", status: "done", createdAt: at(-440)),
            Message(id: "c1r", from: "coder", to: "lead", threadId: "c1", replyTo: "c1", type: "reply", body: "\n**designed**: three calls\nmore", status: "done", createdAt: at(-400)),
            Message(id: "c2", from: "lead", to: "coder", body: "now build it", status: "acked", createdAt: at(-200), updatedAt: at(-5)),
            Message(id: "n1", from: "coder", to: "tester", type: "note", body: "the API is in", status: "done", createdAt: at(-150)),
            Message(id: "n1b", from: "tester", to: "coder", threadId: "n1", type: "note", body: "thanks, testing", status: "done", createdAt: at(-140)),
            Message(id: "c3", from: "coder", to: "tester", body: "check the build", status: "failed", createdAt: at(-100)),
            Message(id: "c4", from: "lead", to: "coder", body: "ship it", status: "queued", createdAt: at(-50)),
            Message(id: "c5", from: "lead", to: "coder", body: "and tag it", status: "acked", createdAt: at(-40)),
            Message(id: "c5e", from: "hub", to: "lead", threadId: "c5", replyTo: "c5", type: "event", body: "no runner", status: "done", createdAt: at(-30)),
        ]
        let cards = ConversationBuilder.handoffs(between: between, and: "coder")
        #expect(cards.map(\.id) == ["c1", "c2", "n1", "c3", "c4", "c5"])
        // Given to it, done, with the first line of what it handed back:
        // not its updates, not what was added.
        #expect(cards[0] == Handoff(
            id: "c1", at: at(-500), updated: at(-400), outgoing: false, other: "lead", isTask: true, text: "design the API", state: .done,
            result: "designed: three calls"))
        #expect(cards[1].state == .working)
        #expect(cards[1].result == nil)
        // A note it sent, and the note that came back: one card.
        #expect(cards[2] == Handoff(
            id: "n1", at: at(-150), updated: at(-140), outgoing: true, other: "tester", isTask: false, text: "the API is in", state: .note,
            result: "thanks, testing"))
        // Work it gave out that did not get done; work waiting for it;
        // work the workspace said could not be done.
        #expect(cards[3].outgoing && cards[3].state == .failed)
        #expect(cards[4].state == .queued)
        #expect(cards[5].state == .failed)

        let mine = [
            Message(id: "t1", from: "me", to: "coder", body: "and the docs?", status: "done", createdAt: at(-300)),
            Message(id: "t1r", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "reply", body: "on it", status: "done", createdAt: at(-290)),
        ]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: mine, handoffs: cards, status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["c1.handoff", "t1", "t1r", "c2.handoff", "n1.handoff", "c3.handoff", "c4.handoff", "c5.handoff"])
        #expect(items[1].kind == .mine(text: "and the docs?", steer: false, by: nil))
        #expect(items[0].kind == .handoff(cards[0]))
        // The input speaks to my own work with it, never to a colleague's.
        #expect(ConversationBuilder.openTask(me: "me", messages: mine) == nil)

        // The card opens their exchange: the one that gave the work as I
        // am in mine, the one that did it answering.
        let theirs = said(ConversationBuilder.items(
            me: "lead", agent: "coder", messages: between.filter { $0.threadId == "c1" }, status: { _ in nil }, now: now))
        #expect(theirs.first?.kind == .mine(text: "design the API\nwith tests", steer: false, by: nil))
        #expect(theirs.last?.kind == .reply(text: "\n**designed**: three calls\nmore", taskID: "c1", to: nil))
    }

    @Test func aProjectsPageHasACardForEachThingHandedOverEitherWay() {
        func thread(_ first: Message, state: String? = nil, answer: Message? = nil, last: Message? = nil) -> RecentItem {
            RecentItem(
                threadId: first.id, participants: [first.from, first.to], updatedAt: (last ?? answer ?? first).createdAt, last: last ?? answer ?? first,
                task: first.type == "task" ? first : nil, answer: answer, state: state, project: "p")
        }
        let out = Message(id: "d1", from: "lead", to: "coder", body: "write the API\nwith tests", status: "acked", createdAt: at(-90))
        let back = Message(id: "d2", from: "lead", to: "tester", body: "test it", status: "done", createdAt: at(-80))
        let backAnswer = Message(id: "d2r", from: "tester", to: "lead", threadId: "d2", replyTo: "d2", type: "reply", body: "all green", status: "done", createdAt: at(-60))
        let incoming = Message(id: "m1", from: "coder", to: "lead", body: "which database?", status: "queued", createdAt: at(-70))
        var note = thread(Message(id: "n1", from: "tester", to: "lead", type: "note", body: "CI is slow today", status: "done", createdAt: at(-50)))
        note.first = note.last
        let mine = Message(id: "t1", from: "me", to: "lead", body: "plan it", status: "done", createdAt: at(-100))
        let person = Message(id: "h1", from: "@console:someone", to: "lead", body: "hello", status: "done", createdAt: at(-40))
        let aside = Message(id: "x1", from: "coder", to: "tester", body: "between members", status: "done", createdAt: at(-30))
        let cards = ConversationBuilder.handoffs(
            ofHome: [thread(out, state: "working"), thread(back, state: "done", answer: backAnswer), thread(incoming, state: "queued"), note,
                     thread(mine, state: "done"), thread(person, state: "done"), thread(aside, state: "done")],
            coordinator: "lead") { $0 == "me" || $0.hasPrefix("@") }
        // What it handed out and what the members sent it; not mine, not a
        // person's, not what two members said to each other.
        #expect(cards.map(\.id) == ["d1", "d2", "m1", "n1"])
        #expect(cards[0] == Handoff(id: "d1", at: at(-90), updated: at(-90), outgoing: true, other: "coder", isTask: true, text: "write the API", state: .working))
        #expect(cards[1].state == .done && cards[1].result == "all green")
        #expect(!cards[2].outgoing && cards[2].other == "coder" && cards[2].state == .queued)
        #expect(cards[3] == Handoff(id: "n1", at: at(-50), updated: at(-50), outgoing: false, other: "tester", isTask: false, text: "CI is slow today", state: .note))

        let items = said(ConversationBuilder.items(me: "me", agent: "lead", messages: [mine], handoffs: cards, status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["t1", "d1.handoff", "d2.handoff", "m1.handoff", "n1.handoff"])
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
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now))
        // The answered task: one line above its answer. The open one: each update.
        #expect(items.map(\.id) == ["t1", "t1.progress", "r1", "t2", "p3", "t2.status"])
        #expect(items[1].kind == .pastProgress(texts: ["one", "two"]))
        #expect(items[4].kind == .progress(text: "halfway", latest: true))
    }

    @Test func anUpdateThatSaysALineAndThenItsPlan() {
        let messages = [
            Message(id: "t1", from: "me", to: "coder", body: "do it", status: "acked", createdAt: at(-100), updatedAt: at(-5)),
            // A line, then the checklist: the plan card takes the list, the line is still said.
            Message(id: "p1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress",
                    body: "好，计划：\n- [x] 看代码\n- [~] 改接口\n- [ ] 补测试", status: "done", createdAt: at(-90)),
            // Only a checklist: the newer plan, and nothing said.
            Message(id: "p2", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress",
                    body: "- [x] 看代码\n- [x] 改接口\n- [~] 补测试", status: "done", createdAt: at(-50)),
        ]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["t1", "t1.plan", "p1", "t1.status"])
        #expect(items[1].kind == .plan(
            items: [PlanItem(text: "看代码", state: .done), PlanItem(text: "改接口", state: .done), PlanItem(text: "补测试", state: .doing)], live: true))
        #expect(items[2].kind == .progress(text: "好，计划：", latest: true))
    }

    @Test func whatWasTakenBackBeforeItsTurnWasNeverSent() {
        var withdrawn = Message(id: "q1", from: "me", to: "coder", after: "t1", body: "next thing", status: "failed", createdAt: at(-50))
        withdrawn.error = RunState.stoppedBySender
        var stopped = Message(id: "t0", from: "me", to: "coder", body: "an old one", status: "failed", createdAt: at(-500))
        stopped.error = RunState.stoppedBySender
        let messages = [
            stopped,
            Message(id: "t1", from: "me", to: "coder", body: "first", status: "acked", createdAt: at(-100), updatedAt: at(-5)),
            withdrawn,
        ]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now))
        // Put behind the first and taken back: not here, not even as "stopped".
        // One that had begun and was stopped still says so.
        #expect(items.map(\.id) == ["t0", "t0.stopped", "t1", "t1.status"])
        #expect(ConversationBuilder.neverSent(messages, me: "me") == ["q1"])
    }

    @Test func aTaskSetAsideDoesNotTakeWhatIType() {
        let held = Message(id: "t1", from: "me", to: "coder", body: "wait for the build", status: "held", createdAt: at(-100))
        // Set aside: the agent is free, so what I type is a new task.
        #expect(ConversationBuilder.openTask(me: "me", messages: [held]) == nil)
        // Set aside to ask me something: what I type is the answer.
        var asking = held
        asking.asking = "p9"
        #expect(ConversationBuilder.openTask(me: "me", messages: [asking])?.id == "t1")
        // Set aside, then at it again (its steps say so): it takes it.
        #expect(ConversationBuilder.openTask(me: "me", messages: [held], moving: ["t1"])?.id == "t1")
        // Another one still open takes it.
        let open = Message(id: "t2", from: "me", to: "coder", body: "and this", status: "acked", createdAt: at(-50))
        #expect(ConversationBuilder.openTask(me: "me", messages: [held, open])?.id == "t2")
        // One it stopped working on (its turn is over) takes nothing
        // either: what I type is a new message, and there is nothing to
        // stop. At work again, it does.
        #expect(ConversationBuilder.openTask(me: "me", messages: [open], paused: ["t2"]) == nil)
        #expect(ConversationBuilder.openTask(me: "me", messages: [open], paused: [])?.id == "t2")
        // Stopped, but asking me something: what I type is the answer.
        var stoppedAsking = open
        stoppedAsking.asking = "q2"
        #expect(ConversationBuilder.openTask(me: "me", messages: [stoppedAsking], paused: ["t2"])?.id == "t2")
    }

    @Test func whatTheAgentOffersToSayNextIsOnMyNewestTask() {
        var answered = Message(id: "t1", from: "me", to: "coder", body: "fix the test", status: "done", createdAt: at(-100))
        answered.suggest = "  now open the pull request \n"
        let reply = Message(id: "r1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "reply", body: "fixed", status: "done", createdAt: at(-90))
        // On my newest task, the agent not at work: offered, trimmed.
        #expect(ConversationBuilder.suggestion(me: "me", messages: [answered, reply], open: nil) == "now open the pull request")
        // While it works on something of mine, nothing is offered.
        let open = Message(id: "t2", from: "me", to: "coder", body: "and this", status: "acked", createdAt: at(-50))
        #expect(ConversationBuilder.suggestion(me: "me", messages: [answered, reply, open], open: open) == nil)
        // A newer task of mine without one: the old offer is over.
        var done = open
        done.status = "done"
        #expect(ConversationBuilder.suggestion(me: "me", messages: [answered, reply, done], open: nil) == nil)
        // A task someone else gave the agent is not mine to continue.
        var theirs = answered
        theirs.from = "lead"
        #expect(ConversationBuilder.suggestion(me: "me", messages: [theirs], open: nil) == nil)
        // Sealed for a device that can open it, or on a task this one
        // cannot read: left out.
        var sealed = answered
        sealed.suggest = "myco-e2e:v1:abc" // Envelope.prefix
        var unreadable = answered
        unreadable.unreadable = "no key"
        #expect(ConversationBuilder.suggestion(me: "me", messages: [sealed], open: nil) == nil)
        #expect(ConversationBuilder.suggestion(me: "me", messages: [unreadable], open: nil) == nil)
        // A hub from before suggestions says nothing.
        let plain = Message(id: "t3", from: "me", to: "coder", body: "x", status: "done", createdAt: at(-10))
        #expect(ConversationBuilder.suggestion(me: "me", messages: [plain], open: nil) == nil)
    }

    @Test func aTaskNotStartedHasAFewWordsUnderIt() {
        // The desktop app's rule: a task reads as a message, not a state.
        let first = Message(id: "t1", from: "me", to: "coder", body: "fix the login", status: "acked", createdAt: at(-300), updatedAt: at(-5))
        let second = Message(id: "t2", from: "me", to: "coder", body: "then the docs", status: "queued", createdAt: at(-100))
        func receipt(_ id: String, _ messages: [Message], _ status: AgentStatus? = nil) -> TaskReceipt? {
            ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in status }, now: now)
                .first { $0.id == id }?.receipt
        }
        // At work: nothing under it. The one behind it says so.
        #expect(receipt("t1", [first, second]) == nil)
        #expect(receipt("t2", [first, second]) == .behind)
        // Alone and not looked at yet.
        #expect(receipt("t2", [second]) == .notOpened)
        var delivered = second
        delivered.status = "delivered"
        #expect(receipt("t2", [delivered]) == .notOpened)
        // Why it has not been: the agent is away, or its session is open elsewhere.
        let coder = Agent(id: "a1", name: "coder")
        #expect(receipt("t2", [second], AgentStatus(agent: coder, state: "offline")) == .offline)
        #expect(receipt("t2", [second], AgentStatus(agent: coder, state: "idle", receiving: nil, sessionInUse: true)) == .sessionBusy)
        // (A node that only receives in its own session is not "busy elsewhere".)
        #expect(receipt("t2", [second], AgentStatus(agent: coder, state: "idle", receiving: "session", sessionInUse: true)) == .notOpened)
        // Set aside: it answers later. Unless it asks me something: the question says it.
        var held = second
        held.status = "held"
        #expect(receipt("t2", [held]) == .later)
        held.asking = "q1"
        #expect(receipt("t2", [held]) == nil)
        // Started without taking it on formally (it reported): gone.
        let report = Message(id: "p1", from: "coder", to: "me", threadId: "t2", replyTo: "t2", type: "progress", body: "looking", status: "done", createdAt: at(-50))
        #expect(receipt("t2", [second, report]) == nil)
        // Answered, or a colleague's: never.
        let answer = Message(id: "r1", from: "coder", to: "me", threadId: "t2", replyTo: "t2", type: "reply", body: "done", status: "done", createdAt: at(-40))
        #expect(receipt("t2", [second, answer]) == nil)
        let theirs = Message(id: "c1", from: "lead", to: "coder", body: "build it", status: "queued", createdAt: at(-80))
        #expect(receipt("c1", [theirs]) == nil)
    }

    @Test func aRunThatWaitsDoesNotLookAtWork() {
        // Mine, taken on; it handed work to the tester, whose task is open.
        let mine = Message(id: "t1", from: "me", to: "lead", body: "ship it", status: "acked", createdAt: at(-300), updatedAt: at(-200), activeAt: at(-200))
        let handed = Message(id: "d1", from: "lead", to: "tester", body: "run the suite", status: "acked", createdAt: at(-250))
        func kinds(_ messages: [Message], paused: Set<String>) -> [ConversationItem.Kind] {
            ConversationBuilder.items(me: "me", agent: "lead", messages: messages, paused: paused, status: { _ in nil }, now: now)
                .filter { $0.id.hasSuffix(".status") }.map(\.kind)
        }
        // Still at it: the usual line.
        #expect(kinds([mine, handed], paused: []) == [.status(.working(since: at(-200)), taskID: "t1")])
        // Its turn ended (it handed work to the tester and waits): nothing
        // is said. What it said last stands, as in any chat.
        #expect(kinds([mine, handed], paused: ["t1"]) == [])
        // That work is done and it has not answered yet: the same.
        var finished = handed
        finished.status = "done"
        #expect(kinds([mine, finished], paused: ["t1"]) == [])
        // Set aside, and nothing since: no line (a receipt, below).
        var held = mine
        held.status = "held"
        #expect(kinds([held], paused: ["t1"]) == [])
        // Set aside, then at it again: at work, not "waiting its turn".
        #expect(kinds([held], paused: []) == [.status(.working(since: at(-200)), taskID: "t1")])
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
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now))
        // No bubble and no "waiting its turn" line for either of them.
        #expect(items.map(\.id) == ["t0", "q0", "t1", "t1.status"])
        #expect(ConversationBuilder.openTask(me: "me", messages: messages)?.id == "t1")
        #expect(ConversationBuilder.threadToContinue(me: "me", messages: messages) == "t1")
    }

    @Test func severalOpenTasksOfMineStillMakeOneStatusLine() {
        let messages = [
            Message(id: "t1", from: "me", to: "coder", body: "one", status: "queued", createdAt: at(-300)),
            Message(id: "t2", from: "me", to: "coder", body: "two", status: "acked", createdAt: at(-200), updatedAt: at(-9)),
            Message(id: "t3", from: "me", to: "coder", body: "three", status: "acked", createdAt: at(-100), updatedAt: at(-5)),
        ]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now))
        // The newest one being worked on speaks for all of them.
        #expect(items.map(\.id) == ["t1", "t2", "t3", "t3.status"])
        #expect(items[3].kind == .status(.working(since: at(-5)), taskID: "t3"))
    }

    @Test func onlyTheNewestUpdateOfARunningTaskIsMarked() {
        let messages = [
            Message(id: "t1", from: "me", to: "coder", body: "build it", status: "acked", createdAt: at(-500), updatedAt: at(-5)),
            Message(id: "p1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "started", status: "done", createdAt: at(-400)),
            Message(id: "p2", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "halfway there", status: "done", createdAt: at(-300)),
            Message(id: "p3", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "nearly done", status: "done", createdAt: at(-200)),
        ]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["t1", "p1", "p2", "p3", "t1.status"])
        #expect(items[1].kind == .progress(text: "started", latest: false))
        #expect(items[2].kind == .progress(text: "halfway there", latest: false))
        #expect(items[3].kind == .progress(text: "nearly done", latest: true))
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
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, tracedPlans: traced, status: { _ in nil }, now: now))
        #expect(items.map(\.id) == ["t1", "t1.plan", "p3", "t1.status"])
        #expect(items[1].kind == .plan(items: [PlanItem(text: "hub", state: .done), PlanItem(text: "app", state: .doing), PlanItem(text: "docs", state: .todo)], live: true))
        #expect(items[2].kind == .progress(text: "- [x] hub pushed\nnotifications come later", latest: true))
    }

    @Test func aFirstLineIsPlainText() {
        #expect(ConversationBuilder.firstLine(of: "定了：**三个 Tab**。\n\n1. 聊天") == "定了：三个 Tab。")
        #expect(ConversationBuilder.firstLine(of: "## Result  `ok`") == "Result ok")
        #expect(ConversationBuilder.firstLine(of: "") == "")
    }

    @Test func aStoppedTaskSaysSo() {
        let messages = [Message(id: "t1", from: "me", to: "coder", body: "x", status: "failed", error: RunState.stoppedBySender, createdAt: at(-10))]
        let items = said(ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now))
        #expect(items.map(\.kind) == [.mine(text: "x", steer: false, by: nil), .stopped])
        #expect(ConversationBuilder.openTask(me: "me", messages: messages) == nil)
    }
}
