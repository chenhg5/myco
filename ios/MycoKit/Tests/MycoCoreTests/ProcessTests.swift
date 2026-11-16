import Foundation
import Testing
import MycoAPI
import MycoFake
import MycoStore
@testable import MycoCore

struct ProcessTests {
    func tool(_ seq: Int, _ id: String, _ tool: String, _ input: String) -> TraceEvent {
        TraceEvent(seq: seq, kind: "tool", data: TraceStep(id: id, tool: tool, input: input))
    }

    func result(_ seq: Int, _ id: String, failed: Bool = false) -> TraceEvent {
        TraceEvent(seq: seq, kind: "tool_result", data: TraceStep(id: id, output: "", failed: failed))
    }

    @Test func stepsBetweenTwoThingsItSaidFoldIntoOneGroup() {
        let process = RunProcess(events: [
            TraceEvent(seq: 1, kind: "text", data: TraceStep(text: "looking")),
            tool(2, "a", "Bash", "ls"), result(3, "a"),
            tool(4, "b", "Read", "a.go"), result(5, "b"),
            TraceEvent(seq: 6, kind: "text", data: TraceStep(text: "found it")),
            tool(7, "c", "Edit", "a.go"),
        ])
        #expect(process.segments.count == 4)
        guard case .steps(_, let first) = process.segments[1], case .steps(_, let last) = process.segments[3] else {
            Issue.record("the second and fourth segments are steps")
            return
        }
        #expect(first.map(\.tool) == ["Bash", "Read"])
        #expect(first.allSatisfy { $0.finished })
        // No result yet: still going.
        #expect(last.map(\.finished) == [false])
    }

    @Test func thinkingFoldsIntoTheLineOfSteps() {
        let process = RunProcess(events: [
            TraceEvent(seq: 1, kind: "thinking", data: TraceStep(text: "where does this live?")),
            tool(2, "a", "Bash", "ls"), result(3, "a"),
            TraceEvent(seq: 4, kind: "text", data: TraceStep(text: "found it")),
            TraceEvent(seq: 5, kind: "thinking", data: TraceStep(text: "now the test")),
        ])
        // Not prose between what it says: part of the steps, there on a tap.
        #expect(process.segments.count == 3)
        guard case .steps(_, let first) = process.segments[0], case .steps(_, let last) = process.segments[2] else {
            Issue.record("thinking is a step of its group")
            return
        }
        #expect(first.map(\.tool) == [RunProcess.thinking, "Bash"])
        #expect(first[0].input == "where does this live?")
        // A result still finds its own step after a thought came first.
        #expect(first[1].finished)
        AppLanguage.$forced.withValue(.en) {
            #expect(RunProcess.summary(of: first) == "thought, ran 1 command")
            #expect(RunProcess.summary(of: last) == "thought")
        }
        AppLanguage.$forced.withValue(.zh) {
            #expect(RunProcess.summary(of: first) == "想了一下、跑了 1 条命令")
        }
    }

    @Test func tellsARunThatWaitsFromOneAtWork() {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        func at(_ seconds: TimeInterval) -> Date { now.addingTimeInterval(seconds) }
        func event(_ seq: Int, _ kind: String, _ seconds: TimeInterval) -> TraceEvent {
            TraceEvent(seq: seq, kind: kind, data: TraceStep(text: "x"), at: at(seconds))
        }
        let task = Message(id: "t1", from: "me", to: "coder", body: "do it", status: "acked", createdAt: at(-900), updatedAt: at(-800))
        func report(_ seconds: TimeInterval) -> Message {
            Message(id: "p\(Int(seconds))", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "…", status: "done", createdAt: at(seconds))
        }
        func paused(_ events: [TraceEvent], _ task: Message, _ reports: [Message] = []) -> Bool {
            RunProcess.isPaused(events: events, task: task, reports: reports, now: now)
        }
        // Just taken on, nothing yet: at work.
        #expect(!paused([], task))
        // Its turn is over, or its run is: it waits.
        #expect(paused([event(1, "tool", -60), event(2, "turn_end", -50)], task))
        #expect(paused([event(1, "usage", -5)], task))
        // It reported after its turn ended: at it again.
        #expect(!paused([event(1, "turn_end", -50)], task, [report(-10)]))
        // A command still running is at work, however long.
        #expect(!paused([event(1, "tool", -3_000)], task))
        // It said something and went silent for two minutes: it waits.
        #expect(!paused([event(1, "text", -60)], task))
        #expect(paused([event(1, "text", -200)], task))
        #expect(paused([event(1, "tool", -400)], task, [report(-200)]))
        // Set aside, and nothing since: it waits. New steps: back at work.
        var held = task
        held.status = "held"
        #expect(paused([event(1, "tool", -850)], held))
        #expect(!paused([event(1, "tool", -30)], held))
    }

    @Test func aTurnThatOnlyKeepsListeningIsNotWork() {
        // The CLI's own cases (trace_chores_test.go).
        let cases: [String: Bool] = [
            "myco inbox wait --timeout 55m --format text": true,
            "cd /home/user/myco && myco inbox wait --timeout 55m": true,
            "myco inbox pull --format text 2>&1 | head -40": true,
            "myco message ack --id msg_x >/dev/null 2>&1; echo ok": false,
            "myco message ack --id msg_x && myco inbox wait": true,
            #"bash -lc "myco inbox wait --timeout 55m""#: true,
            "myco message reply --id msg_x --body done": false,
            "myco message progress --id msg_x --body 'half way'": false,
            "go test ./... && myco inbox wait": false,
            "": false,
            "cd /tmp": false,
            "myco inbox": false,
            "myco message done --id msg_y": true,
        ]
        for (input, want) in cases {
            #expect(RunProcess.isChore(input) == want, "\(input)")
        }

        let now = Date(timeIntervalSince1970: 1_800_000_000)
        func at(_ seconds: TimeInterval) -> Date { now.addingTimeInterval(seconds) }
        var seq = 0
        func event(_ kind: String, _ seconds: TimeInterval, _ input: String? = nil) -> TraceEvent {
            seq += 1
            return TraceEvent(seq: seq, kind: kind, data: TraceStep(text: "x", tool: input == nil ? nil : "Bash", input: input), at: at(seconds))
        }
        let wait = "myco inbox wait --timeout 55m"
        // The turn that took the task (chores and all), its answer, then
        // two turns that only started the wait again, one of them going.
        let took = [event("tool", -900, "myco message ack --id t1"), event("tool", -890, "go test ./..."), event("text", -880), event("turn_end", -870)]
        let woke = [event("thinking", -500), event("tool", -499, wait), event("tool_result", -498), event("usage", -497), event("turn_end", -496)]
        let going = [event("thinking", -20), event("tool", -19, wait)]
        let kept = RunProcess.withoutChores(took + woke + going)
        #expect(kept.map(\.seq) == took.map(\.seq) + [woke[3].seq], "only what it did, and what the idle turn cost")
        // A first turn of chores alone is the task being taken: kept.
        let first = [event("tool", -100, "myco message ack --id t1"), event("turn_end", -99)]
        #expect(RunProcess.withoutChores(first) == first)
        // A later turn that does something is work again, chores and all.
        let again = [event("tool", -10, "myco inbox pull"), event("tool", -9, "make test")]
        #expect(RunProcess.withoutChores(took + again) == took + again)
        // Nothing to leave out: the same steps.
        #expect(RunProcess.withoutChores(took) == took)

        // And such a turn does not make the run look at work: its turn
        // ended, and only chores came since.
        let task = Message(id: "t1", from: "me", to: "coder", body: "do it", status: "acked", createdAt: at(-1_000), updatedAt: at(-950))
        #expect(RunProcess.isPaused(events: took + going, task: task, reports: [], now: now))
        #expect(RunProcess.isPaused(events: took + woke, task: task, reports: [], now: now))
        // A step that does something after the turn ended: at work.
        #expect(!RunProcess.isPaused(events: took + again, task: task, reports: [], now: now))
    }

    @Test func whatItReportedSitsInTheRunWhereItCame() {
        let start = Date(timeIntervalSince1970: 1_800_000_000)
        func at(_ seconds: TimeInterval) -> Date { start.addingTimeInterval(seconds) }
        func step(_ seq: Int, _ id: String, _ seconds: TimeInterval) -> TraceEvent {
            TraceEvent(seq: seq, kind: "tool", data: TraceStep(id: id, tool: "Bash", input: "cmd \(seq)"), at: at(seconds))
        }
        func said(_ seq: Int, _ text: String, _ seconds: TimeInterval) -> TraceEvent {
            TraceEvent(seq: seq, kind: "text", data: TraceStep(text: text), at: at(seconds))
        }
        // What a session followed by myco looks like: steps, words to its
        // own terminal in between, and at the end the summary it wrote.
        let process = RunProcess(events: [
            step(1, "a", 10), step(2, "b", 20),
            said(3, "the Mac went to sleep; waiting", 40),
            step(4, "c", 60), step(5, "d", 70),
            said(6, "all done: here is the summary", 90),
        ])
        let reports = [
            RunReport(id: "p1", text: "taken on", at: at(15)),
            RunReport(id: "p2", text: "research done", at: at(25)),
            RunReport(id: "p3", text: "tests pass", at: at(65)),
        ]
        func shape(_ parts: [RunPart]) -> [String] {
            parts.map { part in
                switch part {
                case .said(_, let text): return "said:" + String(text.prefix(8))
                case .steps(_, let steps): return "steps:" + steps.map { String($0.id) }.joined(separator: ",")
                case .failure: return "failure"
                case .reports(_, let reports): return "reports:" + reports.map(\.id).joined(separator: ",")
                case .added(let report): return "added:" + report.id
                }
            }
        }
        // Answered: each update after the step it came after, never under
        // what was written later; and the closing summary is not said here,
        // it is the answer under the run.
        #expect(shape(process.parts(reports: reports, live: false)) == [
            "steps:1", "reports:p1", "steps:2", "reports:p2", "said:the Mac ", "steps:4", "reports:p3", "steps:5",
        ])
        // Still at work: what it is writing now shows, at the end.
        #expect(shape(process.parts(reports: reports, live: true)).last == "said:all done")
        // What I added to the task is in the run where I sent it.
        let added = [RunReport(id: "s1", text: "also check the title", at: at(45))]
        #expect(shape(process.parts(reports: [], added: added, live: false)) == [
            "steps:1,2", "said:the Mac ", "added:s1", "steps:4,5",
        ])
        // Two updates in a row are one timeline.
        let together = [RunReport(id: "p1", text: "a", at: at(21)), RunReport(id: "p2", text: "b", at: at(22))]
        #expect(shape(process.parts(reports: together, live: false)) == ["steps:1,2", "reports:p1,p2", "said:the Mac ", "steps:4,5"])
        // Steps without a time (an older runner): updates go at the end.
        let untimed = RunProcess(events: [tool(1, "a", "Bash", "ls"), result(2, "a"), tool(3, "b", "Bash", "ls")])
        #expect(shape(untimed.parts(reports: reports, live: true)) == ["steps:1,3", "reports:p1,p2,p3"])
    }

    @Test func aRunTakesItsUpdatesAndAdditionsNotItsPlan() {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        func at(_ seconds: TimeInterval) -> Date { now.addingTimeInterval(seconds) }
        let messages = [
            Message(id: "t1", from: "me", to: "coder", body: "do it", status: "acked", createdAt: at(-100)),
            Message(id: "p1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "好，计划：\n- [x] a\n- [ ] b", status: "done", createdAt: at(-90)),
            Message(id: "p2", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "- [x] a\n- [x] b", status: "done", createdAt: at(-80)),
            Message(id: "s1", from: "me", to: "coder", threadId: "t1", replyTo: "t1", type: "steer", body: "and c", status: "done", createdAt: at(-70)),
            Message(id: "p9", from: "coder", to: "me", threadId: "t1", replyTo: "t0", type: "progress", body: "another task's", status: "done", createdAt: at(-60)),
        ]
        let extras = ConversationBuilder.runExtras(of: "t1", agent: "coder", messages: messages)
        // The line said with the plan stays; the checklist is the plan card's.
        #expect(extras.reports.map(\.id) == ["p1"])
        #expect(extras.reports.first?.text == "好，计划：")
        #expect(extras.added.map(\.text) == ["and c"])
        // With its run shown, they are not rows of the conversation too.
        let rows = ConversationBuilder.items(me: "me", agent: "coder", messages: messages, processes: ["t1"], status: { _ in nil }, now: now)
        #expect(!rows.contains { $0.id == "p1" || $0.id == "s1" })
        #expect(rows.contains { $0.id == "t1.process" })
        // Without a run to put them in, they stay rows.
        let plain = ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in nil }, now: now)
        #expect(plain.contains { $0.id == "p1" })
        #expect(plain.contains { $0.id == "s1" })
    }

    @Test func aFailedResultMarksItsOwnStep() {
        let process = RunProcess(events: [tool(1, "a", "Bash", "go test"), tool(2, "b", "Bash", "go vet"), result(3, "a", failed: true), result(4, "b")])
        guard case .steps(_, let steps) = process.segments[0] else {
            Issue.record("one group of steps")
            return
        }
        #expect(steps.map(\.failed) == [true, false])
    }

    @Test func thePlanIsAChecklistNotAStep() {
        let process = RunProcess(events: [
            tool(1, "a", "TodoWrite", "[x] read\n[ ] write"),
            tool(2, "b", "Bash", "ls"),
            tool(3, "c", "TodoWrite", "[x] read\n[~] write\n[ ] test"),
        ])
        #expect(process.plan == [PlanItem(text: "read", state: .done), PlanItem(text: "write", state: .doing), PlanItem(text: "test", state: .todo)])
        #expect(process.segments.count == 1)
    }

    @Test func readsPlansAsTheDesktopAppDoes() {
        #expect(RunProcess.parsePlan("- [x] a\n- [ ] b") == [PlanItem(text: "a", state: .done), PlanItem(text: "b", state: .todo)])
        #expect(RunProcess.parsePlan("* [X] a\n+ [-] b") == [PlanItem(text: "a", state: .done), PlanItem(text: "b", state: .doing)])
        // One item, or any line that is not an item: not a plan.
        #expect(RunProcess.parsePlan("[x] only one") == nil)
        #expect(RunProcess.parsePlan("[x] a\nand some prose") == nil)
        #expect(RunProcess.parsePlan("") == nil)
    }

    @Test func aPlanWithALineBeforeItIsStillAPlan() throws {
        // What agents really write: a line, then the checklist.
        let split = try #require(RunProcess.splitPlan("好，计划：\n- [x] 看现有的接口\n- [~] 写设备登记\n- [ ] 补测试"))
        #expect(split.items.map(\.state) == [.done, .doing, .todo])
        #expect(split.items[1].text == "写设备登记")
        #expect(split.rest == "好，计划：")
        // Said after it too, and blank lines between the items.
        let around = try #require(RunProcess.splitPlan("先这样：\n\n[x] a\n\n[ ] b\n\n做完告诉你。"))
        #expect(around.items.count == 2)
        #expect(around.rest == "先这样：\n\n\n做完告诉你。")
        // Only the checklist: nothing left to say.
        #expect(RunProcess.splitPlan("- [x] a\n- [ ] b")?.rest == "")
        // The longest checklist of the text is the plan.
        let longest = try #require(RunProcess.splitPlan("- [x] 一条\n说明\n- [ ] a\n- [ ] b\n- [ ] c"))
        #expect(longest.items.map(\.text) == ["a", "b", "c"])
        #expect(longest.rest == "- [x] 一条\n说明")
        // One item is a remark, not a plan; and prose is prose.
        #expect(RunProcess.splitPlan("记一下：\n- [ ] 只有一条") == nil)
        #expect(RunProcess.splitPlan("没有清单") == nil)
    }

    @Test func summarisesAGroupInOneLine() {
        let steps = [
            ToolStep(id: 1, tool: "Read", input: "a", failed: false, finished: true),
            ToolStep(id: 2, tool: "Bash", input: "x", failed: true, finished: true),
            ToolStep(id: 3, tool: "Bash", input: "y", failed: false, finished: true),
            ToolStep(id: 4, tool: "Edit", input: "a", failed: false, finished: true),
            ToolStep(id: 5, tool: "Write", input: "a", failed: false, finished: true),
            ToolStep(id: 6, tool: "Grep", input: "z", failed: false, finished: true),
        ]
        // The same file edited twice counts once.
        AppLanguage.$forced.withValue(.en) {
            #expect(RunProcess.summary(of: steps) == "read 1 file, ran 2 commands, edited 1 file, 1 other step, 1 failed")
        }
        AppLanguage.$forced.withValue(.zh) {
            #expect(RunProcess.summary(of: steps) == "读了 1 个文件、跑了 2 条命令、改了 1 个文件、另外 1 次操作、1 次失败")
        }
    }
}

@MainActor
struct ConversationProcessTests {
    @Test func aFinishedTaskShowsItsStepsUnderWhatIAsked() async {
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: FakeHub())
        await session.refresh()
        let model = ConversationModel(session: session, agent: "cloud-lead", project: "myco-ios")
        await model.reload()
        // What I asked, its plan, how it was worked on, the answer.
        #expect(model.items.map(\.id).starts(with: ["m1.time", "m1", "m1.plan", "m1.process", "m2"]))
        let process = try! #require(model.process(of: "m1"))
        #expect(process.plan?.count == 4)
        #expect(!process.segments.isEmpty)
        AppLanguage.$forced.withValue(.en) {
            #expect(RunProcess(events: FakeHub.scriptForTests).usage == "12s · 1500 tokens · $0.02")
        }
    }

    @Test func aConversationOpensOnWhatItShowedLast() async {
        let store = MemoryStore()
        let first = WorkspaceSession(id: "w1", name: "Demo", hub: FakeHub(), snapshots: store)
        await first.refresh()
        let model = first.conversation(with: "cloud-lead", project: "myco-ios")
        await model.reload()
        let shown = model.items.map(\.id)
        #expect(shown.contains("m2"))
        // Asked for again while the workspace is on screen: the same one.
        #expect(first.conversation(with: "cloud-lead", project: "myco-ios") === model)

        // The next launch, with the workspace not answering.
        let hub = FakeHub()
        await hub.setUnreachable(true)
        let second = WorkspaceSession(id: "w1", name: "Demo", hub: hub, snapshots: store)
        await second.restore()
        let again = second.conversation(with: "cloud-lead", project: "myco-ios")
        #expect(again.items.isEmpty)
        await again.restore()
        #expect(again.loaded)
        #expect(again.items.map(\.id) == shown)
        #expect(again.process(of: "m1")?.plan?.count == 4)
        // Another conversation has nothing kept under this one's name.
        let other = second.conversation(with: "studio-codex-e07b")
        await other.restore()
        #expect(other.items.isEmpty)
    }

    @Test func aMembersPageShowsWhatItAndItsColleaguesHandedEachOtherAsCards() async {
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: FakeHub())
        await session.refresh()
        let model = ConversationModel(session: session, agent: "studio-codex-7f3a")
        await model.reload()
        // The coordinator's task is not a message in my conversation with
        // the member (it would read as if I had asked): it is a card.
        #expect(!model.items.contains { $0.id == "d1" })
        #expect(!model.items.contains { $0.id == "d1.process" })
        guard case .handoff(let card)? = model.items.first(where: { $0.id == "d1.handoff" })?.kind else {
            Issue.record("the coordinator's task shows as a handoff card")
            return
        }
        #expect(!card.outgoing)
        #expect(card.other == "cloud-lead")
        #expect(card.isTask)
        #expect(card.text.hasPrefix("设计 Hub 的推送接口"))
        #expect(card.state == .working)
        // What I said to it myself is the conversation.
        #expect(model.items.contains { $0.kind == .mine(text: "推送中转先别定用哪家，等我回来再说。", steer: false, by: nil) })
        #expect(!model.items.contains { if case .mine(_, _, let by) = $0.kind { return by != nil } else { return false } })
        // The input speaks to my own open task only: there is none.
        #expect(model.openTask == nil)
        #expect(model.action == .send)
        #expect(!model.readOnly)

        // The card opens the two agents' exchange, to read: the
        // coordinator's task as asked, how the member works on it.
        let theirs = session.exchange(of: "studio-codex-7f3a", with: "cloud-lead")
        await theirs.reload()
        #expect(theirs.readOnly)
        guard case .mine(let text, false, nil)? = theirs.items.first(where: { $0.id == "d1" })?.kind else {
            Issue.record("in their exchange the coordinator's task is what was asked")
            return
        }
        #expect(text.hasPrefix("设计 Hub 的推送接口"))
        #expect(theirs.items.contains { $0.id == "d1.process" })
        // Only theirs: what I said to the member is not in it.
        #expect(!theirs.items.contains { $0.id == "b1" })
        #expect(!theirs.items.contains { if case .handoff = $0.kind { return true } else { return false } })
    }

    @Test func aProjectsPageShowsWhatItsCoordinatorAndMembersHandedEachOther() async {
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: FakeHub())
        await session.refresh()
        let model = ConversationModel(session: session, agent: "cloud-lead", project: "myco-ios")
        await model.reload()
        let cards = model.items.compactMap { item -> Handoff? in
            if case .handoff(let card) = item.kind { return card }
            return nil
        }
        #expect(Set(cards.filter(\.outgoing).map(\.other)) == ["studio-codex-7f3a", "studio-claude-2b9c", "studio-claude-55d0", "laptop-claude-a1e2"])
        // The tester handed its list back: the card says so, with its first line.
        let tested = cards.first { $0.id == "d3" }
        let came = tested?.result ?? ""
        #expect(tested?.state == .done)
        #expect(came.hasPrefix("清单写好了"))
        // Its page is my conversation with it: the members' work is not mixed in.
        #expect(!model.items.contains { if case .mine(_, _, let by) = $0.kind { return by != nil } else { return false } })
    }
}
