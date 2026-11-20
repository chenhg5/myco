import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// What an agent may touch when it runs, beside its model.
struct AccessChoiceTests {
    func agent(cli: String = "codex", access: String? = "workspace", max: String? = "workspace", chosen: String? = nil, machine: String? = "mach_1") -> Agent {
        var runtime = Runtime(cli: cli, model: "gpt-6-sol")
        runtime.access = access
        runtime.maxAccess = max
        return Agent(id: "a", name: "coder", machineId: machine, runtime: runtime, ownerId: "mem_me",
                     runSettings: chosen.map { RunSettings(access: $0) })
    }

    @Test func itsOwnerSwitchesItUpToWhatItsComputerAllows() throws {
        let mine = try #require(AccessChoice.of(agent(), owns: true, tooOld: false))
        #expect(mine.editable)
        #expect(mine.allowed == ["readonly", "workspace"])
        #expect(!mine.pending)
        let open = try #require(AccessChoice.of(agent(max: "full", chosen: "full"), owns: true, tooOld: false))
        #expect(open.allowed == ["readonly", "workspace", "full"])
        #expect(open.shown == "full")
        #expect(open.pending, "full from its next run")
        // A computer that allows less than it runs with: what it runs with.
        #expect(AccessChoice.of(agent(access: "full", max: "readonly"), owns: true, tooOld: false)?.cap == "full")
    }

    @Test func othersAreToldWhyNot() throws {
        AppLanguage.$forced.withValue(.zh) {
            #expect(AccessChoice.of(agent(), owns: false, tooOld: false)?.why == "只有它的主人能改。")
            #expect(AccessChoice.of(agent(max: nil), owns: true, tooOld: false)?.why == "升级那台电脑上的 myco 后才能在这里改。")
        }
        // Nothing said about it, a cloud session, a session in its own conversation: none.
        #expect(AccessChoice.of(agent(access: nil), owns: true, tooOld: false) == nil)
        #expect(AccessChoice.of(agent(cli: "claude-cloud", machine: nil), owns: true, tooOld: false) == nil)
        #expect(AccessChoice.of(agent(cli: "inbox"), owns: true, tooOld: false) == nil)
    }

    @Test func aSwitchKeepsItsModelAndEffort() async throws {
        let hub = FakeHub()
        _ = try await hub.setRunSettings(of: "studio-codex-7f3a", model: "gpt-6-sol", effort: "high", access: nil)
        let switched = try await hub.setRunSettings(of: "studio-codex-7f3a", model: "gpt-6-sol", effort: "high", access: "readonly")
        #expect(switched.runSettings == RunSettings(model: "gpt-6-sol", effort: "high", access: "readonly"))
        // Left out, it stays.
        let again = try await hub.setRunSettings(of: "studio-codex-7f3a", model: "", effort: "", access: nil)
        #expect(again.runSettings?.access == "readonly")
    }
}
