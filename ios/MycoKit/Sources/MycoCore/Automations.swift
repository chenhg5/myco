import Foundation
import MycoAPI

/// An automation being written: what the editor holds. It reads top to
/// bottom as the automation does: to whom, what, when. The desktop app's
/// `AutomationEditor` state.
public struct AutomationDraft: Sendable, Hashable {
    public enum Mode: String, Sendable, Hashable, CaseIterable {
        case daily, weekdays, weekly, hours, cron
    }

    public var name = ""
    public var to = ""
    public var body = ""
    /// Sent when it is time.
    public var timed = true
    public var mode: Mode = .weekdays
    public var hour = 9
    public var minute = 0
    /// 0 is Sunday.
    public var day = 1
    public var hours = 2
    public var cron = "0 9 * * 1-5"
    /// The schedule's time zone (an IANA name).
    public var zone: String
    /// Sent when an outside system calls its webhook.
    public var hook = false
    /// Skip a run while the last task it sent is not finished.
    public var skip = true

    /// Where a new one starts: blank, or one of the common ones.
    public struct Template: Sendable, Hashable, Identifiable {
        public var name: String
        public var hint: String
        public var body: String
        public var every: Every?
        public var hook: Bool
        public var symbol: String
        public var id: String { name }
    }

    public static var templates: [Template] {
        [
            Template(
                name: L("每天早上汇总", "Morning summary"),
                hint: L("工作日 09:00，把昨天新开的 issue 和 PR 挑重点列给你", "Weekdays at 09:00: yesterday's new issues and PRs, the ones that matter"),
                body: L("看一下昨天新开的 issue 和 PR，挑出要紧的，在这里回我一份清单。", "Look at yesterday's new issues and PRs, pick the ones that matter, and reply with a list."),
                every: .weekdays(hour: 9, minute: 0), hook: false, symbol: "cup.and.saucer"),
            Template(
                name: L("每周五写周报", "Friday report"),
                hint: L("每周五 17:00，总结这周做了什么、下周打算", "Fridays at 17:00: what got done this week, and what is next"),
                body: L("总结这周做完了什么、还在做什么、下周打算做什么，写成一份简短的周报回我。", "Sum up what got done this week, what is still going, and what is next, as a short report."),
                every: .weekly(day: 5, hour: 17, minute: 0), hook: false, symbol: "calendar"),
            Template(
                name: L("定时巡检", "Regular check"),
                hint: L("每 4 小时看一眼服务和 CI，有异常才说", "Every 4 hours: services and CI, a word only if something is wrong"),
                body: L("检查一下服务、CI 和依赖有没有异常。一切正常就回一句「正常」，有问题说清楚是什么、影响多大。",
                        "Check the services, CI and dependencies. If all is well, say so in a word; if not, say what it is and how much it matters."),
                every: .hours(every: 4, minute: 0), hook: false, symbol: "waveform.path.ecg"),
            Template(
                name: L("CI 失败时排查", "When CI fails"),
                hint: L("CI 构建失败时调用，让它看日志找原因", "Called when a build fails: it reads the log and finds why"),
                body: L("CI 构建失败了，看一下日志找出原因，能修就修，修不了告诉我卡在哪：\n{{payload}}",
                        "The CI build failed. Read the log and find why; fix it if you can, else tell me where it is stuck:\n{{payload}}"),
                every: nil, hook: true, symbol: "point.3.connected.trianglepath.dotted"),
        ]
    }

    /// A new one: blank (weekdays at nine), or from a template.
    public init(template: Template? = nil, to: String = "", zone: String = TimeZone.current.identifier) {
        self.zone = zone
        self.to = to
        guard let template else { return }
        name = template.name
        body = template.body
        timed = template.every != nil
        hook = template.hook
        if let every = template.every { set(every) }
    }

    /// One that exists, to change.
    public init(_ automation: Automation, zone: String = TimeZone.current.identifier) {
        self.zone = automation.schedule?.tz.flatMap { $0.isEmpty ? nil : $0 } ?? zone
        name = automation.name
        to = automation.to
        body = automation.body
        hook = automation.webhook
        skip = automation.skipIfBusy
        timed = automation.schedule != nil
        set(Every(cron: automation.schedule?.cron ?? "0 9 * * 1-5"))
    }

    public mutating func set(_ every: Every) {
        switch every {
        case .daily(let hour, let minute):
            mode = .daily
            self.hour = hour
            self.minute = minute
        case .weekdays(let hour, let minute):
            mode = .weekdays
            self.hour = hour
            self.minute = minute
        case .weekly(let day, let hour, let minute):
            mode = .weekly
            self.day = day
            self.hour = hour
            self.minute = minute
        case .hours(let every, let minute):
            mode = .hours
            hours = every
            self.minute = minute
        case .cron(let cron):
            mode = .cron
            self.cron = cron
            return
        }
        cron = every.cron
    }

    /// The schedule as chosen.
    public var every: Every {
        switch mode {
        case .daily: return .daily(hour: hour, minute: minute)
        case .weekdays: return .weekdays(hour: hour, minute: minute)
        case .weekly: return .weekly(day: day, hour: hour, minute: minute)
        case .hours: return .hours(every: hours, minute: minute)
        case .cron: return .cron(cron)
        }
    }

    /// What is still missing before it can be saved, in words; nil when nothing is.
    public var missing: String? {
        if name.trimmingCharacters(in: .whitespaces).isEmpty { return L("还没写名字", "It needs a name") }
        if to.isEmpty { return L("还没选发给谁", "Pick whom it goes to") }
        if body.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { return L("还没写让它做什么", "Say what to ask") }
        if !timed, !hook { return L("选一种触发方式：定时或 Webhook", "Pick when: on a schedule, or by webhook") }
        if timed, !every.isComplete { return L("cron 要写 5 段", "A cron has 5 fields") }
        return nil
    }

    /// When it next sends, when the schedule is in this device's time zone.
    public func next(after now: Date, here: TimeZone = .current) -> Date? {
        guard timed, zone == here.identifier else { return nil }
        return every.next(after: now, in: here)
    }

    /// Puts `{{payload}}` at the end of the message, once.
    public mutating func insertPayload() {
        guard !body.contains("{{payload}}") else { return }
        let text = body.replacingOccurrences(of: "\\s+$", with: "", options: .regularExpression)
        body = text.isEmpty ? "{{payload}}" : text + "\n{{payload}}"
    }

    /// What is sent to the workspace: all of it for a new one; for a
    /// change, the same fields (and its schedule taken away, if it was).
    public func request(changing existing: Automation? = nil) -> AutomationRequest {
        var request = AutomationRequest(
            name: name.trimmingCharacters(in: .whitespaces), to: to, body: body, webhook: hook, skipIfBusy: skip)
        if timed {
            request.schedule = AutomationSchedule(cron: every.cron, tz: zone)
        } else if existing?.schedule != nil {
            request.clearSchedule = true
        }
        if existing == nil { request.enabled = true }
        return request
    }
}

/// What a list of automations says of each: when, how the last one went.
public enum AutomationWords {
    /// "工作日 09:00，或 Webhook 调用时".
    public static func when(_ automation: Automation) -> String {
        [automation.schedule.map { Every(cron: $0.cron).words } ?? "", automation.webhook ? L("Webhook 调用时", "On webhook") : ""]
            .filter { !$0.isEmpty }.joined(separator: L("，或", ", or "))
    }

    /// "下次 明天 09:00", "已暂停", "等外部调用"; nil when there is nothing to say.
    public static func standing(_ automation: Automation, now: Date) -> String? {
        if !automation.enabled { return L("已暂停", "Paused") }
        if let next = automation.nextRunAt {
            let soon = Every.soon(next, now: now)
            return L("下次 \(soon)", "Next \(soon)")
        }
        return automation.webhook ? L("等外部调用", "Waiting for a call") : nil
    }

    public static func outcome(_ run: AutomationRun) -> String {
        let why = run.error ?? ""
        switch run.outcome {
        case "sent":
            return L("已发送", "Sent")
        case "skipped":
            // (the one reason the workspace gives is said in Chinese too)
            let said = why.contains("not finished") ? L("上一次的任务还没做完", "the last task is not finished") : why
            return why.isEmpty ? L("跳过了", "Skipped") : L("跳过了：\(said)", "Skipped: \(said)")
        default:
            return why.isEmpty ? L("没发出去", "Not sent") : L("没发出去：\(why)", "Not sent: \(why)")
        }
    }

    public static func trigger(_ run: AutomationRun) -> String {
        switch run.trigger {
        case "schedule": return L("到点", "On schedule")
        case "webhook": return "Webhook"
        default: return L("手动", "By hand")
        }
    }

    /// A line to try a webhook with, from a terminal.
    public static func curl(url: String, secret: String) -> String {
        "curl -X POST '\(url)' -H 'Authorization: Bearer \(secret)' --data '" + L("构建失败：…", "The build failed: …") + "'"
    }
}

/// A workspace's automations, as its page shows them.
@MainActor
@Observable
public final class AutomationsModel {
    private let hub: any HubAPI

    /// nil until the workspace has answered once.
    public private(set) var list: [Automation]?
    /// The workspace is from before automations.
    public private(set) var unsupported = false
    /// Why the last thing asked did not work.
    public private(set) var failure: Failure?
    /// The ones something is being done to.
    public private(set) var busy: Set<String> = []

    /// How often the page reads the list again while it is on screen.
    public static let pollInterval: Duration = .seconds(20)

    public init(hub: any HubAPI) {
        self.hub = hub
    }

    public func load() async {
        do {
            list = try await hub.automations()
            unsupported = false
            failure = nil
        } catch let error as HubError where Failure.isTooOld(error) || (error.error == "not_found" && list == nil) {
            unsupported = true
            list = []
        } catch is CancellationError {
            // the page was left
        } catch {
            failure = Failure.of(error, canRetry: true)
            if list == nil { list = [] }
        }
    }

    /// Reads the list until the surrounding task is cancelled.
    public func run() async {
        while !Task.isCancelled {
            await load()
            try? await Task.sleep(for: Self.pollInterval)
        }
    }

    private func act(on id: String, _ work: () async throws -> Void) async -> Bool {
        guard !busy.contains(id) else { return false }
        busy.insert(id)
        defer { busy.remove(id) }
        do {
            try await work()
            failure = nil
            await load()
            return true
        } catch {
            failure = Failure.of(error)
            return false
        }
    }

    /// Turns one off, or on again. Shown at once; put back if it did not go.
    public func setEnabled(_ automation: Automation, _ enabled: Bool) async {
        guard automation.enabled != enabled else { return }
        if let index = list?.firstIndex(where: { $0.id == automation.id }) { list?[index].enabled = enabled }
        let done = await act(on: automation.id) {
            _ = try await hub.updateAutomation(automation.id, AutomationRequest(enabled: enabled))
        }
        if !done, let index = list?.firstIndex(where: { $0.id == automation.id }) { list?[index].enabled = automation.enabled }
    }

    /// Sends it once now.
    @discardableResult
    public func runNow(_ automation: Automation) async -> Bool {
        await act(on: automation.id) { _ = try await hub.runAutomation(automation.id) }
    }

    @discardableResult
    public func remove(_ automation: Automation) async -> Bool {
        await act(on: automation.id) { try await hub.deleteAutomation(automation.id) }
    }

    /// Its last runs, newest first; none when they cannot be read.
    public func runs(of id: String) async -> [AutomationRun] {
        (try? await hub.automation(id).runs) ?? []
    }

    /// Creates one, or changes one that exists. Throws when the workspace refuses.
    public func save(_ draft: AutomationDraft, changing existing: Automation? = nil) async throws -> AutomationSaved {
        let request = draft.request(changing: existing)
        let saved = try await existing.map { try await hub.updateAutomation($0.id, request) } ?? hub.createAutomation(request)
        await load()
        return saved
    }

    /// A new secret for its webhook: the old one stops working.
    public func newSecret(of automation: Automation) async throws -> AutomationSaved {
        try await hub.updateAutomation(automation.id, AutomationRequest(rotateSecret: true))
    }
}
