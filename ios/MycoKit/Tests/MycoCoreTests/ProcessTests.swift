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

    @Test func aMembersPageShowsTheWorkItsCoordinatorGaveIt() async {
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: FakeHub())
        await session.refresh()
        let model = ConversationModel(session: session, agent: "studio-codex-7f3a")
        await model.reload()
        // The coordinator's task is part of the conversation, named, with
        // how it is being worked on and where it stands.
        let asked = model.items.first { $0.id == "d1" }
        guard case .mine(let text, false, "cloud-lead")? = asked?.kind else {
            Issue.record("the coordinator's task shows as something asked, by the coordinator")
            return
        }
        #expect(text.hasPrefix("设计 Hub 的推送接口"))
        #expect(model.items.contains { $0.id == "d1.process" })
        guard case .status(.working, "d1")? = model.items.first(where: { $0.id == "d1.status" })?.kind else {
            Issue.record("and says it is being worked on")
            return
        }
        // What I said to it myself is there too; nothing is a capsule.
        #expect(model.items.contains { $0.kind == .mine(text: "推送中转先别定用哪家，等我回来再说。", steer: false, by: nil) })
        #expect(!model.items.contains { if case .dispatch = $0.kind { return true } else { return false } })
        // The input speaks to my own open task only: there is none.
        #expect(model.openTask == nil)
        #expect(model.action == .send)
    }

    @Test func aProjectsPageShowsWhatItsCoordinatorHandedOut() async {
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: FakeHub())
        await session.refresh()
        let model = ConversationModel(session: session, agent: "cloud-lead", project: "myco-ios")
        await model.reload()
        let handedOut = model.items.compactMap { item -> String? in
            if case .dispatch(let d) = item.kind { return d.to }
            return nil
        }
        #expect(Set(handedOut) == ["studio-codex-7f3a", "studio-claude-2b9c", "studio-claude-55d0", "laptop-claude-a1e2"])
        // Its page is my conversation with it: the members' work is not mixed in.
        #expect(!model.items.contains { if case .mine(_, _, let by) = $0.kind { return by != nil } else { return false } })
    }
}
