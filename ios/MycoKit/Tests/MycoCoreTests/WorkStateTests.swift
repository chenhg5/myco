import Foundation
import Testing
import MycoAPI
@testable import MycoCore
import MycoFake

/// Usage limits in words (as the desktop's quota.ts), and an agent's goal
/// and background waits.
@MainActor
struct WorkStateTests {
    let now = Date(timeIntervalSince1970: 1_795_000_000)

    @Test func theTightestWindowIsSaidAsTheDesktopSaysIt() {
        let five = QuotaWindow(name: "five_hour", durationSec: 18_000, used: 0.62, resetsAt: now.addingTimeInterval(9_000))
        let week = QuotaWindow(name: "seven_day", durationSec: 604_800, used: 0.41)
        let quota = Quota(provider: "claude", plan: "claude_subscription", at: now, windows: [week, five])
        #expect(QuotaWords.worst(quota) == five)
        #expect(QuotaWords.pill(five) == L("5h 剩 38%", "5h 38% left"))
        #expect(QuotaWords.short(week) == "7d")
        #expect(QuotaWords.label(five) == L("5 小时", "5 hours"))
        #expect(QuotaWords.level(five) == .ok)
        #expect(QuotaWords.level(QuotaWindow(name: "x", used: 0.85)) == .low)
        #expect(QuotaWords.level(QuotaWindow(name: "x", used: 0.97)) == .out)
        #expect(QuotaWords.row(five, now: now) == L("剩 38% · 3 小时后重置", "38% left · resets in 3 h"))
        #expect(QuotaWords.source(quota) == L("Claude · Claude 订阅", "Claude · Claude plan"))
        #expect(QuotaWords.outNote(quota, now: now) == nil)
        // A spent window is the tightest, whatever the numbers.
        let out = QuotaWindow(name: "seven_day", durationSec: 604_800, used: 0.5, resetsAt: now.addingTimeInterval(3_600), status: "rejected")
        let spent = Quota(provider: "claude", at: now, windows: [five, out])
        #expect(QuotaWords.worst(spent) == out)
        #expect(QuotaWords.pill(out) == L("7d 用完", "7d out"))
        #expect(QuotaWords.outNote(spent, now: now)?.contains(L("额度已用完", "quota is used up")) == true)
        // A failed read with no numbers never looks like a full quota.
        #expect(!QuotaWords.hasNumbers(Quota(provider: "codex", status: "auth_required")))
        #expect(QuotaWords.status(Quota(provider: "codex", status: "auth_required")) == L("需要在 CLI 重新登录", "Sign in again in the CLI"))
    }

    @Test func aGoalAndAWaitAreReadAndChanged() async {
        let hub = FakeHub()
        let ops = WorkStateModel(hub: hub, agent: "vps-codex-90aa")
        await ops.load()
        #expect(ops.watches.map(\.id) == ["wat_demo1"])
        #expect(ops.goal == nil)
        let watch = ops.watches[0]
        await ops.extend(watch, seconds: 3_600)
        #expect((ops.watches.first?.deadline.timeIntervalSinceNow ?? 0) > 3_500)
        await ops.stop(watch)
        #expect(ops.watches.isEmpty)
        #expect(ops.endedWatches.map(\.state) == ["stopped"])
        #expect(WorkStateModel.watchEnded(ops.endedWatches[0]).contains("等 CI 跑完再发版"))

        let backend = WorkStateModel(hub: hub, agent: "studio-codex-7f3a")
        await backend.load()
        #expect(backend.goal?.round == 3)
        #expect(backend.rounds.count == 3)
        await backend.control("pause")
        #expect(backend.goal?.state == "paused")
        #expect(WorkStateModel.goalNote(backend.goal!) != nil)
        await backend.control("resume")
        await backend.setInterval(3_600)
        #expect(backend.goal?.everySec == 3_600)
        await backend.control("stop")
        #expect(backend.goal == nil)
        #expect(backend.endedGoals.first?.state == "stopped")
        // A new one; one at a time.
        #expect(await backend.start(objective: "写文档", doneWhen: "README 更新", every: 900, rounds: 8, hours: 2, project: nil))
        #expect(backend.goal?.objective == "写文档")
        #expect(!(await backend.start(objective: "又一个", doneWhen: "x", every: 900, rounds: 8, hours: 2, project: nil)))
    }

    @Test func anOlderWorkspaceHasNeither() async {
        let hub = FakeHub()
        await hub.setWithoutWorkState(true)
        let model = WorkStateModel(hub: hub, agent: "vps-codex-90aa")
        await model.load()
        #expect(model.unsupported)
        #expect(model.watches.isEmpty && model.goal == nil)
    }

    @Test func clocksAreSaidInMinutesAndHours() {
        #expect(WorkStateModel.span(720) == L("12 分钟", "12 min"))
        #expect(WorkStateModel.span(4_800) == L("1 小时 20 分", "1 h 20 min"))
        let goal = Goal(id: "g", agent: "a", objective: "o", doneWhen: "d", maxRounds: 16, deadline: now.addingTimeInterval(9_000), round: 3,
                        nextWakeAt: now.addingTimeInterval(720), startedAt: now.addingTimeInterval(-4_800))
        let line = WorkStateModel.goalLine(goal, now: now)
        #expect(line.contains(L("第 3/16 轮", "Round 3/16")))
        #expect(line.contains(L("还有 12 分钟", "in 12 min")))
        #expect(line.contains(L("已进行 1 小时 20 分", "1 h 20 min so far")))
    }
}
