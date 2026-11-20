import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// 工作流 on the phone: decisions waiting for me, running one, its runs.
@MainActor
struct WorkflowTests {
    @Test func aDecisionWaitingForMeIsAnswered() async throws {
        let hub = FakeHub()
        let model = WorkflowsModel(hub: hub)
        await model.load()
        #expect(model.list?.map(\.name) == ["写-审-改"])
        #expect(model.list?.first?.lastRun?.status == "waiting")
        let decision = try #require(model.decisions.first)
        #expect(decision.step.options == ["合并", "不合并"])
        #expect(await model.decide(decision, choice: "合并", note: " 好 "))
        #expect(model.decisions.isEmpty)
        let run = try await hub.workflowRun(decision.run.id)
        #expect(run.status == "succeeded")
        #expect(run.steps?.last?.summary == "合并：好")
    }

    @Test func aRunAsksForWhatItsDefinitionSays() async throws {
        let hub = FakeHub()
        let model = WorkflowsModel(hub: hub)
        await model.load()
        let listed = try #require(model.list?.first)
        let workflow = try #require(await model.detail(listed))
        let fields = WorkflowsModel.fields(of: workflow.definition)
        #expect(fields.map(\.id) == ["task", "urgent"])
        #expect(fields[0].kind == .longText && fields[0].required)
        #expect(fields[1].kind == .toggle && fields[1].start == "false")
        #expect(!WorkflowsModel.ready([:], definition: workflow.definition))
        let values = ["task": "加一个分享按钮", "urgent": "true"]
        #expect(WorkflowsModel.ready(values, definition: workflow.definition))
        #expect(WorkflowsModel.inputs(values, definition: workflow.definition) == ["task": .string("加一个分享按钮"), "urgent": .bool(true)])
        let run = try #require(await model.start(workflow, values: values, definition: workflow.definition))
        #expect(run.status == "running")
        #expect(model.runs.first?.id == run.id)
        // Cancelled, then started again.
        let cancelled = try #require(await model.cancel(run))
        #expect(cancelled.status == "cancelled")
        let again = try #require(await model.retry(cancelled))
        #expect(again.status == "running")
        // Turned off, it cannot be run.
        await model.setEnabled(workflow, false)
        #expect(model.list?.first?.enabled == false)
        #expect(await model.start(workflow, values: values, definition: workflow.definition) == nil)
        #expect(model.failure != nil)
    }

    @Test func anOlderWorkspaceHasNone() async {
        let hub = FakeHub()
        await hub.setTooOldForWorkflows(true)
        let model = WorkflowsModel(hub: hub)
        await model.load()
        #expect(model.unsupported)
        #expect(model.list == [])
    }

    @Test func aWorkflowReadsFromTheWorkspace() throws {
        // A schema says more than the phone reads (a type that may be null,
        // a pattern): the workflow still reads.
        let json = #"""
        {"id":"wf_1","name":"x","version":1,"enabled":true,"created_at":"2026-11-19T00:00:00Z","updated_at":"2026-11-19T00:00:00Z",
         "definition":{"name":"x","inputs":{"required":["n"],"properties":{"n":{"type":["integer","null"],"minimum":1},"tag":{"type":"string","enum":["a","b"],"pattern":"^[a-z]$"}}},
           "steps":[{"id":"s","agent":"coder","prompt":"do {{inputs.n}}","output":{"properties":{}}}],"edges":[]}}
        """#
        let workflow = try HubJSON.decoder().decode(Workflow.self, from: Data(json.utf8))
        let fields = WorkflowsModel.fields(of: workflow.definition)
        #expect(fields.map(\.id) == ["n", "tag"])
        #expect(fields[0].kind == .number)
        #expect(fields[1].kind == .choice(["a", "b"]))
        #expect(workflow.definition?.steps?.first?.agent == "coder")
    }
}
