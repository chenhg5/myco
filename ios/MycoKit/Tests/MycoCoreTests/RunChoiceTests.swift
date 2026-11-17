import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// The model and effort beside the message box: what is shown, and who
/// may switch it.
@MainActor
struct RunChoiceTests {
    func agent(cli: String, model: String? = nil, effort: String? = nil, machine: String? = "mach_1", chosen: RunSettings? = nil, kind: String? = "agent") -> Agent {
        Agent(
            id: "agt_1", name: "coder", binding: Binding(harness: cli), kind: kind, machineId: machine,
            runtime: Runtime(cli: cli, model: model, effort: effort), runSettings: chosen)
    }

    @Test func aModelIsNamedAsAPersonSaysIt() {
        #expect(RunChoice.short("claude-opus-5-5") == "Opus 5.5")
        #expect(RunChoice.short("claude-sonnet-5-5-20260101") == "Sonnet 5.5")
        #expect(RunChoice.short("claude-fable-5-2[1m]") == "Fable 5.2")
        #expect(RunChoice.short("opus") == "Opus")
        #expect(RunChoice.short("gpt-6-sol") == "GPT-6 Sol")
        #expect(RunChoice.short("gpt-5-codex") == "GPT-5 Codex")
        #expect(RunChoice.short("gpt-6") == "GPT-6")
        #expect(RunChoice.short("o3") == "O3")
        #expect(RunChoice.short("  ") == "")
    }

    @Test func aRunnersAgentOfMineCanBeSwitched() {
        AppLanguage.$forced.withValue(.en) {
            let plain = RunChoice.of(agent(cli: "claude", model: "claude-opus-5-5"), manages: true)
            #expect(plain?.kind == .mine)
            #expect(plain?.label == "Opus 5.5")
            // What it runs with is offered too, first.
            #expect(plain?.models == ["claude-opus-5-5", "fable", "opus", "sonnet", "haiku"])
            #expect(plain?.efforts == ["low", "medium", "high", "xhigh", "max"])
            #expect(plain?.pending == false)
            // A switch shows at once, marked until it is in use.
            let switched = RunChoice.of(
                agent(cli: "codex", model: "gpt-6-sol", effort: "medium", chosen: RunSettings(model: "gpt-6", effort: "high")), manages: true)
            #expect(switched?.label == "GPT-6 · High")
            #expect(switched?.pending == true)
            #expect(switched?.inUse == "GPT-6 Sol · Medium")
            #expect(switched?.models.first == "gpt-6-sol")
            // In use now: no mark.
            let settled = RunChoice.of(agent(cli: "codex", model: "gpt-6", effort: "high", chosen: RunSettings(model: "gpt-6", effort: "high")), manages: true)
            #expect(settled?.pending == false)
            // Nothing known yet, but it can be chosen.
            #expect(RunChoice.of(agent(cli: "claude"), manages: true)?.label == "Default model")
        }
    }

    @Test func whatIsNotMineToSwitchSaysWhoDecides() {
        AppLanguage.$forced.withValue(.en) {
            // Someone else's: shown, not switched.
            let theirs = RunChoice.of(agent(cli: "claude", model: "opus"), manages: false)
            #expect(theirs?.kind == .theirs)
            #expect(theirs?.models.isEmpty == true)
            // Nothing known and nothing to switch: nothing shown.
            #expect(RunChoice.of(agent(cli: "claude"), manages: false) == nil)
            // In its own session: its session decides, whoever I am.
            let own = RunChoice.of(agent(cli: "inbox", model: "claude-fable-5-2", effort: "high"), manages: true)
            #expect(own?.kind == .itsSession)
            #expect(own?.label == "Fable 5.2 · High")
            #expect(own?.note.contains("/model") == true)
            let cloud = RunChoice.of(agent(cli: "claude-cloud", model: "claude-opus-5-5", machine: nil), manages: true)
            #expect(cloud?.kind == .itsSession)
            // It has not said yet.
            let silent = RunChoice.of(agent(cli: "inbox"), manages: true)
            #expect(silent?.kind == .unknown)
            #expect(silent?.label == "Model unknown")
            // A workspace that keeps no run settings: shown, with why.
            let old = RunChoice.of(agent(cli: "claude", model: "opus"), manages: true, tooOld: true)
            #expect(old?.kind == .workspaceTooOld)
            // A shell, a computer: no model.
            #expect(RunChoice.of(agent(cli: "shell", kind: "shell"), manages: true) == nil)
        }
    }

    @Test func aSwitchIsKeptByTheWorkspace() async {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        // Who I am is not known yet: nothing is offered to switch.
        #expect(session.runChoice(of: "studio-codex-7f3a") == nil)
        await session.refresh()
        let before = session.runChoice(of: "studio-codex-7f3a")
        #expect(before?.kind == .mine)
        #expect(before?.chosenModel == "")
        #expect(await session.setRun(of: "studio-codex-7f3a", model: "gpt-6", effort: "xhigh") == nil)
        let after = session.runChoice(of: "studio-codex-7f3a")
        #expect(after?.chosenModel == "gpt-6")
        #expect(after?.chosenEffort == "xhigh")
        #expect(after?.pending == true)
        // Still so after the lists are read again.
        await session.refresh()
        #expect(session.runChoice(of: "studio-codex-7f3a")?.chosenModel == "gpt-6")
        // Back to its computer's setting.
        #expect(await session.setRun(of: "studio-codex-7f3a", model: "", effort: "") == nil)
        #expect(session.runChoice(of: "studio-codex-7f3a")?.pending == false)
        // An older workspace: said, and from then on only shown.
        await hub.setTooOldForRunSettings(true)
        #expect(await session.setRun(of: "studio-codex-7f3a", model: "gpt-6", effort: "") != nil)
        #expect(session.runChoice(of: "studio-codex-7f3a")?.kind == .workspaceTooOld)
    }
}
