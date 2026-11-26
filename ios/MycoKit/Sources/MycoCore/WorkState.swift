import Foundation
import MycoAPI

/// What an agent does on its own, above its chat (the desktop's bars): the
/// goal it works on (目标) and what it waits for in the background (盯着),
/// with what ended lately. Read for one agent; changed by its owner.
@MainActor
@Observable
public final class WorkStateModel {
    private let hub: any HubAPI
    public let agent: String

    public private(set) var goal: Goal?
    public private(set) var rounds: [GoalRound] = []
    /// Waits going on, soonest due first.
    public private(set) var watches: [Watch] = []
    /// Waits and goals that ended in the last day, to leave a line where
    /// they ended.
    public private(set) var endedWatches: [Watch] = []
    public private(set) var endedGoals: [Goal] = []
    /// The workspace is from before watches and goals (v0.22).
    public private(set) var unsupported = false
    public private(set) var busy: String?
    public private(set) var failure: Failure?

    public init(hub: any HubAPI, agent: String) {
        self.hub = hub
        self.agent = agent
    }

    public func load(now: Date = Date()) async {
        do {
            async let allWatches = hub.watches(agent: agent, all: true)
            async let allGoals = hub.goals(agent: agent, all: true)
            let (w, g) = try await (allWatches, allGoals)
            let recent = now.addingTimeInterval(-86_400)
            watches = w.filter(\.isOn).sorted { $0.deadline < $1.deadline }
            endedWatches = w.filter { !$0.isOn && ($0.endedAt ?? .distantPast) > recent }
            let on = g.first(where: \.isOn)
            if let on, on.id != goal?.id || on.round != goal?.round {
                rounds = (try? await hub.goal(on.id).rounds) ?? []
            }
            goal = on
            endedGoals = g.filter { !$0.isOn && ($0.endedAt ?? .distantPast) > recent }
            unsupported = false
        } catch let error as HubError where error.status == 404 || Failure.isTooOld(error) {
            unsupported = true
            watches = []
            goal = nil
        } catch is CancellationError {
            return
        } catch {
            // (a failed look keeps what was shown)
        }
    }

    /// Stops a wait (the person asked first).
    public func stop(_ watch: Watch) async {
        await act(watch.id) { _ = try await self.hub.stopWatch(watch.id) }
    }

    /// Gives a wait more time, from now.
    public func extend(_ watch: Watch, seconds: Int) async {
        await act(watch.id) { _ = try await self.hub.extendWatch(watch.id, seconds: seconds) }
    }

    /// pause, resume or stop the goal.
    public func control(_ action: String) async {
        guard let goal else { return }
        await act(goal.id) { _ = try await self.hub.controlGoal(goal.id, action) }
    }

    public func setInterval(_ seconds: Int) async {
        guard let goal else { return }
        await act(goal.id) { _ = try await self.hub.setGoalInterval(goal.id, seconds: seconds) }
    }

    /// Sets a goal on the agent: woken every `every` seconds, at most
    /// `rounds` times, for at most `hours`.
    @discardableResult
    public func start(objective: String, doneWhen: String, every: Int, rounds: Int, hours: Int, project: String?) async -> Bool {
        let objective = objective.trimmingCharacters(in: .whitespacesAndNewlines)
        let doneWhen = doneWhen.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !objective.isEmpty, !doneWhen.isEmpty else { return false }
        return await act("new") {
            _ = try await self.hub.startGoal(GoalRequest(
                agent: self.agent, objective: objective, doneWhen: doneWhen, project: project,
                everySec: every, maxRounds: rounds, forSec: hours * 3_600))
        }
    }

    @discardableResult
    private func act(_ id: String, _ work: @escaping () async throws -> Void) async -> Bool {
        guard busy == nil else { return false }
        busy = id
        defer { busy = nil }
        do {
            try await work()
            failure = nil
            await load()
            return true
        } catch {
            failure = Failure.of(error)
            await load()
            return false
        }
    }

    // MARK: Words

    /// "12 分钟", "1 小时 20 分": a length of time.
    public static func span(_ seconds: TimeInterval) -> String {
        let minutes = max(0, Int((seconds / 60).rounded(.down)))
        if minutes < 60 { return L("\(minutes) 分钟", "\(minutes) min") }
        let h = minutes / 60, m = minutes % 60
        return m == 0 ? L("\(h) 小时", "\(h) h") : L("\(h) 小时 \(m) 分", "\(h) h \(m) min")
    }

    /// "已 12 分钟，剩 18 分钟 · 30 秒前看过": where a wait is.
    public static func watchLine(_ w: Watch, now: Date) -> String {
        var parts = [L("已 \(span(now.timeIntervalSince(w.startedAt)))，剩 \(span(w.deadline.timeIntervalSince(now)))",
                       "\(span(now.timeIntervalSince(w.startedAt))) in, \(span(w.deadline.timeIntervalSince(now))) left")]
        if let checked = w.lastCheckAt {
            let ago = Int(now.timeIntervalSince(checked))
            parts.append(ago < 60 ? L("\(max(ago, 0)) 秒前看过", "checked \(max(ago, 0)) s ago") : L("\(ago / 60) 分钟前看过", "checked \(ago / 60) min ago"))
        }
        return parts.joined(separator: " · ")
    }

    /// How a wait ended, in a few words.
    public static func watchEnded(_ w: Watch) -> String {
        switch w.state {
        case "met": return L("盯到了：\(w.what)", "Done waiting: \(w.what)")
        case "expired": return L("没等到，时间到了：\(w.what)", "Time ran out waiting: \(w.what)")
        default: return L("停止了盯：\(w.what)", "Stopped waiting: \(w.what)")
        }
    }

    /// "第 3/16 轮 · 下次唤醒 14:30（还有 12 分钟）· 已进行 1 小时 20 分".
    public static func goalLine(_ g: Goal, now: Date) -> String {
        var parts = [L("第 \(g.round)/\(g.maxRounds) 轮", "Round \(g.round)/\(g.maxRounds)")]
        if g.state == "paused" {
            parts.append(L("已暂停", "Paused"))
        } else if let next = g.nextWakeAt {
            let at = next.formatted(date: .omitted, time: .shortened)
            parts.append(next > now ? L("下次唤醒 \(at)（还有 \(span(next.timeIntervalSince(now)))）", "next \(at) (in \(span(next.timeIntervalSince(now))))")
                                    : L("正在唤醒", "Waking now"))
        }
        parts.append(L("已进行 \(span(now.timeIntervalSince(g.startedAt)))", "\(span(now.timeIntervalSince(g.startedAt))) so far"))
        return parts.joined(separator: " · ")
    }

    /// A paused goal's or a slowed one's note.
    public static func goalNote(_ g: Goal) -> String? {
        if g.state == "paused" { return L("暂停了，等你看一下再继续。", "Paused: it waits for you to look before going on.") }
        if let base = g.baseEverySec, g.everySec > base {
            return L("几轮没有进展，唤醒间隔拉长到 \(span(TimeInterval(g.everySec)))。", "No progress for a few rounds: it is woken every \(span(TimeInterval(g.everySec))) now.")
        }
        return nil
    }

    /// How a goal ended.
    public static func goalEnded(_ g: Goal) -> String {
        switch g.state {
        case "achieved": return L("目标达成：\(g.objective)", "Goal reached: \(g.objective)")
        case "abandoned": return L("目标放弃了：\(g.objective)", "Goal given up: \(g.objective)")
        case "expired": return L("目标到时间了：\(g.objective)", "Goal ran out of time: \(g.objective)")
        default: return L("目标停止了：\(g.objective)", "Goal stopped: \(g.objective)")
        }
    }
}

/// The plans a person closed above a chat, kept on this device: closed, a
/// plan stays closed when the chat is opened again, until a new plan comes.
public enum ClosedPlans {
    static let defaultsKey = "myco-closed-plans"
    /// At most this many chats remembered (the oldest go first).
    static let limit = 200

    /// The plan closed in this chat, if any.
    public static func closed(in chat: String, defaults: UserDefaults = .standard) -> String? {
        (defaults.dictionary(forKey: defaultsKey) as? [String: String])?[chat]
    }

    /// Remembers that this plan was closed in this chat (the one before it
    /// is forgotten: a new plan shows again).
    public static func close(_ plan: String, in chat: String, defaults: UserDefaults = .standard) {
        var all = defaults.dictionary(forKey: defaultsKey) as? [String: String] ?? [:]
        all[chat] = plan
        if all.count > limit { all.remove(at: all.startIndex) }
        defaults.set(all, forKey: defaultsKey)
    }

    /// Forgets every closed plan (tests start from what the demo shows).
    public static func forgetAll(defaults: UserDefaults = .standard) {
        defaults.removeObject(forKey: defaultsKey)
    }

    /// The key of a chat: the workspace, the agent and the project.
    public static func chat(workspace: String, agent: String, project: String?) -> String {
        [workspace, agent, project ?? ""].joined(separator: "|")
    }
}
