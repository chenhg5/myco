import Foundation
import Testing
import MycoAPI
import MycoFake
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
        #expect(model.items.map(\.id).starts(with: ["m1", "m1.process", "m2"]))
        let process = try! #require(model.process(of: "m1"))
        #expect(process.plan?.count == 4)
        #expect(!process.segments.isEmpty)
    }

    @Test func workOthersGaveAnAgentShowsOnItsPage() async {
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: FakeHub())
        await session.refresh()
        let model = ConversationModel(session: session, agent: "studio-codex-7f3a")
        await model.reload()
        let incoming = model.items.compactMap { item -> Dispatch? in
            if case .dispatch(let d) = item.kind { return d }
            return nil
        }
        #expect(incoming.map(\.taskID) == ["d1"])
        #expect(incoming.first?.incoming == true)
        #expect(incoming.first?.from == "cloud-lead")
    }
}
