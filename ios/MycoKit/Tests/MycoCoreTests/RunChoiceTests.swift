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

    @Test func theModelsItsComputerOffersComeFirst() {
        AppLanguage.$forced.withValue(.en) {
            var a = agent(cli: "codex", model: "gpt-6-sol")
            a.runtime?.models = [RuntimeModel(id: "astra", name: "GPT Astra", efforts: ["low", "high"]), RuntimeModel(id: "gpt-6-sol")]
            let choice = RunChoice.of(a, manages: true)
            // Its computer's list (Codex's own), not the built-in one.
            #expect(choice?.models == ["astra", "gpt-6-sol"])
            #expect(choice?.name(of: "astra") == "GPT Astra")
            #expect(choice?.name(of: "gpt-6-sol") == "GPT-6 Sol")
            // gpt-6-sol said no efforts: the usual ones.
            #expect(choice?.efforts == ["minimal", "low", "medium", "high", "xhigh"])
            a.runSettings = RunSettings(model: "astra")
            #expect(RunChoice.of(a, manages: true)?.efforts == ["low", "high"])
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

/// An agent on another vendor: its provider's models, where they came
/// from, and its runner asked to list them again.
@MainActor
struct ProviderModelsTests {
    @Test func theProvidersModelsSayWhereTheyCameFrom() {
        AppLanguage.$forced.withValue(.zh) {
            let now = Date()
            var runtime = Runtime(cli: "claude", model: "deepseek-v4", models: [RuntimeModel(id: "deepseek-v4", name: "DeepSeek V4"), RuntimeModel(id: "deepseek-reasoner")])
            runtime.provider = "DeepSeek"
            runtime.modelsFrom = "provider"
            runtime.modelsAt = now.addingTimeInterval(-600)
            runtime.features = ["models_refresh"]
            let agent = Agent(id: "agt_1", name: "coder", binding: Binding(harness: "claude"), kind: "agent", machineId: "mach_1", runtime: runtime)
            let choice = RunChoice.of(agent, manages: true, now: now)
            #expect(choice?.models == ["deepseek-v4", "deepseek-reasoner"])
            #expect(choice?.name(of: "deepseek-v4") == "DeepSeek V4")
            #expect(choice?.source == "模型列表从 DeepSeek 读取 · 10 分钟前")
            #expect(choice?.canListAgain == true)
            // Not mine: no list, no line.
            #expect(RunChoice.of(agent, manages: false, now: now)?.source == nil)
            // The built-in list, before the vendor could be asked.
            var catalog = agent
            catalog.runtime?.modelsFrom = "catalog"
            catalog.runtime?.modelsAt = nil
            #expect(RunChoice.of(catalog, manages: true, now: now)?.source == "内置列表：还没能从 DeepSeek 读取")
            // An older runner: no line, and nothing to ask.
            var old = agent
            old.runtime?.features = nil
            #expect(RunChoice.of(old, manages: true, now: now)?.canListAgain == false)
            // Codex's own list says nothing.
            var codex = agent
            codex.runtime?.modelsFrom = "cli"
            #expect(RunChoice.of(codex, manages: true, now: now)?.source == nil)
        }
    }

    @Test func listingAgainAsksItsRunner() async throws {
        let hub = FakeHub(scenario: .team)
        // (the demo's list is ten minutes old; listed again, it is new)
        let agent = try await hub.listModelsAgain(of: "studio-claude-55d0")
        #expect(agent.runtime?.modelsFrom == "provider")
        #expect((agent.runtime?.modelsAt ?? .distantPast) > Date().addingTimeInterval(-60))
    }
}
