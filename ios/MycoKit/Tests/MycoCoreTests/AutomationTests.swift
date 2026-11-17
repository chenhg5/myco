import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// Automations: schedules as people say them, what the editor sends, and
/// the page's list.
@MainActor
struct AutomationTests {
    let shanghai = TimeZone(identifier: "Asia/Shanghai")!
    /// Tuesday 2026-11-17 16:30 in Shanghai.
    let tuesday = Date(timeIntervalSince1970: 1_794_904_200)

    @Test func aScheduleIsACronAndBack() {
        #expect(Every.daily(hour: 9, minute: 0).cron == "0 9 * * *")
        #expect(Every.weekdays(hour: 18, minute: 30).cron == "30 18 * * 1-5")
        #expect(Every.weekly(day: 5, hour: 17, minute: 0).cron == "0 17 * * 5")
        #expect(Every.hours(every: 1, minute: 15).cron == "15 * * * *")
        #expect(Every.hours(every: 4, minute: 0).cron == "0 */4 * * *")
        #expect(Every.cron("  0   9 1 * * ").cron == "0 9 1 * *")
        // A cron that is one of the common ones reads as it.
        #expect(Every(cron: "0 9 * * *") == .daily(hour: 9, minute: 0))
        #expect(Every(cron: "30 18 * * 1-5") == .weekdays(hour: 18, minute: 30))
        #expect(Every(cron: "0 17 * * 7") == .weekly(day: 0, hour: 17, minute: 0))
        #expect(Every(cron: "15 * * * *") == .hours(every: 1, minute: 15))
        #expect(Every(cron: "0 */4 * * *") == .hours(every: 4, minute: 0))
        // Anything else is a cron of its own.
        #expect(Every(cron: "0 9 1 * *") == .cron("0 9 1 * *"))
        #expect(Every(cron: "0 */24 * * *") == .cron("0 */24 * * *"))
        #expect(Every(cron: "*/5 9 * * *") == .cron("*/5 9 * * *"))
        #expect(Every(cron: "nonsense") == .cron("nonsense"))
        #expect(!Every.cron("0 9 * *").isComplete)
        #expect(Every.cron("0 9 1 * *").isComplete)
    }

    @Test func aScheduleReadsInWords() {
        AppLanguage.$forced.withValue(.zh) {
            #expect(Every.daily(hour: 9, minute: 0).words == "每天 09:00")
            #expect(Every.weekdays(hour: 18, minute: 30).words == "工作日 18:30")
            #expect(Every.weekly(day: 5, hour: 17, minute: 0).words == "每周五 17:00")
            #expect(Every.hours(every: 1, minute: 5).words == "每小时（第 5 分）")
            #expect(Every.hours(every: 4, minute: 0).words == "每 4 小时（第 0 分）")
            #expect(Every.cron("0 9 1 * *").words == "cron 0 9 1 * *")
            #expect(Every.soon(tuesday.addingTimeInterval(600), now: tuesday, in: shanghai) == "今天 16:40")
            #expect(Every.soon(tuesday.addingTimeInterval(59_400), now: tuesday, in: shanghai) == "明天 09:00")
            #expect(Every.soon(tuesday.addingTimeInterval(3 * 86_400), now: tuesday, in: shanghai) == "周五 16:30")
            #expect(Every.soon(tuesday.addingTimeInterval(16 * 86_400), now: tuesday, in: shanghai) == "12月3日 16:30")
        }
        AppLanguage.$forced.withValue(.en) {
            #expect(Every.weekly(day: 5, hour: 17, minute: 0).words == "Every Friday at 17:00")
            #expect(Every.hours(every: 2, minute: 5).words == "Every 2 hours at :05")
            #expect(Every.soon(tuesday.addingTimeInterval(3 * 86_400), now: tuesday, in: shanghai) == "Fri 16:30")
            #expect(Every.soon(tuesday.addingTimeInterval(16 * 86_400), now: tuesday, in: shanghai) == "Dec 3 16:30")
        }
    }

    @Test func whenItNextSends() {
        func at(_ every: Every) -> String {
            AppLanguage.$forced.withValue(.zh) { every.next(after: tuesday, in: shanghai).map { Every.soon($0, now: tuesday, in: shanghai) } ?? "-" }
        }
        #expect(at(.daily(hour: 9, minute: 0)) == "明天 09:00")
        #expect(at(.daily(hour: 18, minute: 0)) == "今天 18:00")
        #expect(at(.weekdays(hour: 9, minute: 0)) == "明天 09:00")
        #expect(at(.weekly(day: 1, hour: 9, minute: 0)) == "周一 09:00")
        #expect(at(.hours(every: 4, minute: 0)) == "今天 20:00")
        #expect(at(.cron("0 9 1 * *")) == "-")
        // Friday evening: weekdays go on on Monday.
        let friday = tuesday.addingTimeInterval(3 * 86_400 + 4 * 3_600)
        let next = Every.weekdays(hour: 9, minute: 0).next(after: friday, in: shanghai)
        let weekday = next.map { Calendar(identifier: .gregorian).dateComponents(in: shanghai, from: $0).weekday }
        #expect(weekday == 2)
    }

    @Test func whatTheEditorHoldsIsWhatIsSent() {
        AppLanguage.$forced.withValue(.en) {
            var draft = AutomationDraft(to: "pm", zone: "Asia/Shanghai")
            #expect(draft.missing == "It needs a name")
            draft.name = " Morning "
            #expect(draft.missing == "Say what to ask")
            draft.body = "List the new issues."
            #expect(draft.missing == nil)
            // New: all of it, turned on. Weekdays at nine unless said otherwise.
            let made = draft.request()
            #expect(made.name == "Morning")
            #expect(made.to == "pm")
            #expect(made.schedule == AutomationSchedule(cron: "0 9 * * 1-5", tz: "Asia/Shanghai"))
            #expect(made.webhook == false)
            #expect(made.skipIfBusy == true)
            #expect(made.enabled == true)
            #expect(made.clearSchedule == nil)
            // Neither a schedule nor a webhook: not yet.
            draft.timed = false
            #expect(draft.missing == "Pick when: on a schedule, or by webhook")
            draft.hook = true
            #expect(draft.missing == nil)
            draft.insertPayload()
            #expect(draft.body == "List the new issues.\n{{payload}}")
            draft.insertPayload()
            #expect(draft.body == "List the new issues.\n{{payload}}", "once")
            // A cron of its own needs its five fields.
            draft.timed = true
            draft.mode = .cron
            draft.cron = "0 9 * *"
            #expect(draft.missing == "A cron has 5 fields")
            draft.cron = "0 9 1 * *"
            #expect(draft.request().schedule?.cron == "0 9 1 * *")
        }
    }

    @Test func oneThatExistsOpensAsItIs() {
        let existing = Automation(
            id: "aut_1", name: "周报", to: "pm", body: "写周报", schedule: AutomationSchedule(cron: "0 17 * * 5", tz: "Europe/Berlin"),
            webhook: true, skipIfBusy: false)
        var draft = AutomationDraft(existing, zone: "Asia/Shanghai")
        #expect(draft.mode == .weekly)
        #expect(draft.day == 5)
        #expect(draft.hour == 17)
        #expect(draft.zone == "Europe/Berlin", "its own time zone stays")
        #expect(draft.hook)
        #expect(!draft.skip)
        // Not this phone's time zone: no "next" is worked out here.
        #expect(draft.next(after: tuesday, here: shanghai) == nil)
        // A change keeps who it is; its schedule taken away is said.
        var change = draft.request(changing: existing)
        #expect(change.enabled == nil, "saving does not turn a paused one on")
        #expect(change.clearSchedule == nil)
        draft.timed = false
        change = draft.request(changing: existing)
        #expect(change.schedule == nil)
        #expect(change.clearSchedule == true)
        // A template brings its name, words and time.
        let friday = AutomationDraft(template: AutomationDraft.templates[1], to: "pm", zone: "Asia/Shanghai")
        #expect(friday.every == .weekly(day: 5, hour: 17, minute: 0))
        #expect(!friday.name.isEmpty)
        let ci = AutomationDraft(template: AutomationDraft.templates[3], to: "pm", zone: "Asia/Shanghai")
        #expect(!ci.timed)
        #expect(ci.hook)
        #expect(ci.body.contains("{{payload}}"))
    }

    @Test func eachRowSaysWhenAndHowTheLastOneWent() {
        AppLanguage.$forced.withValue(.zh) {
            var one = Automation(id: "a", name: "n", to: "pm", body: "b", schedule: AutomationSchedule(cron: "0 9 * * 1-5"), webhook: true)
            #expect(AutomationWords.when(one) == "工作日 09:00，或Webhook 调用时")
            one.nextRunAt = tuesday.addingTimeInterval(86_400)
            #expect(AutomationWords.standing(one, now: tuesday)?.hasPrefix("下次 明天") == true)
            one.enabled = false
            #expect(AutomationWords.standing(one, now: tuesday) == "已暂停")
            let hook = Automation(id: "h", name: "n", to: "pm", body: "b", webhook: true)
            #expect(AutomationWords.when(hook) == "Webhook 调用时")
            #expect(AutomationWords.standing(hook, now: tuesday) == "等外部调用")
            #expect(AutomationWords.outcome(AutomationRun(id: "r", at: tuesday, trigger: "schedule", outcome: "sent")) == "已发送")
            #expect(AutomationWords.outcome(AutomationRun(id: "r", at: tuesday, trigger: "schedule", outcome: "skipped", error: "the last task it sent is not finished")) == "跳过了：上一次的任务还没做完")
            #expect(AutomationWords.outcome(AutomationRun(id: "r", at: tuesday, trigger: "webhook", outcome: "failed", error: "agent pm not found")) == "没发出去：agent pm not found")
            #expect(AutomationWords.trigger(AutomationRun(id: "r", at: tuesday, trigger: "manual", outcome: "sent")) == "手动")
        }
    }

    @Test func thePageListsThemAndActsOnThem() async throws {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        let model = session.automations
        #expect(model.list == nil)
        await model.load()
        #expect(model.list?.map(\.id) == ["aut_morning", "aut_ci"])
        #expect(!model.unsupported)
        let morning = try #require(model.list?.first)

        // Turned off: at once on the page, and kept by the workspace.
        await model.setEnabled(morning, false)
        #expect(model.list?.first?.enabled == false)
        let kept = try await hub.automations().first
        #expect(kept?.enabled == false)
        await model.setEnabled(try #require(model.list?.first), true)

        // Sent once now: a task of mine to its agent, marked as its own.
        let before = await hub.allMessages().count
        #expect(await model.runNow(morning))
        let sent = try #require(await hub.allMessages().last)
        #expect(await hub.allMessages().count == before + 1)
        #expect(sent.automation == "aut_morning")
        #expect(sent.to == morning.to)
        #expect(model.list?.first?.lastRun?.trigger == "manual")
        #expect(await model.runs(of: "aut_morning").count == 2)
        // In the conversation it is mine, marked as sent by an automation.
        let chat = session.conversation(with: morning.to)
        await chat.reload()
        #expect(chat.items.first { $0.id == sent.id }?.automated == true)
        #expect(chat.items.first { $0.id == "m1" }?.automated == false)

        // A new one, with a webhook: its secret comes this once.
        var draft = AutomationDraft(template: AutomationDraft.templates[3], to: "studio-codex-7f3a")
        draft.name = "构建失败"
        let saved = try await model.save(draft)
        #expect(saved.webhookSecret?.hasPrefix("whk_") == true)
        #expect(saved.automation.webhookUrl?.contains("/hooks/") == true)
        #expect(model.list?.count == 3)
        // Changed: no new secret.
        var change = AutomationDraft(saved.automation)
        change.name = "构建失败时"
        let changed = try await model.save(change, changing: saved.automation)
        #expect(changed.webhookSecret == nil)
        #expect(model.list?.first?.name == "构建失败时")
        let rotated = try await model.newSecret(of: changed.automation)
        #expect(rotated.webhookSecret != nil)
        #expect(rotated.webhookSecret != saved.webhookSecret)
        // The workspace refuses one with nothing to trigger it.
        var nothing = AutomationDraft(saved.automation)
        nothing.hook = false
        await #expect(throws: HubError.self) { _ = try await model.save(nothing, changing: saved.automation) }

        #expect(await model.remove(changed.automation))
        #expect(model.list?.count == 2)

        // The workspace does not answer: said, and the list stays.
        await hub.setUnreachable(true)
        await model.load()
        #expect(model.failure?.kind == .transient)
        #expect(model.list?.count == 2)
        #expect(!(await model.runNow(morning)))
        await hub.setUnreachable(false)
    }

    @Test func aWorkspaceFromBeforeAutomationsSaysSo() async {
        let hub = FakeHub()
        await hub.setTooOldForAutomations(true)
        let model = AutomationsModel(hub: hub)
        await model.load()
        #expect(model.unsupported)
        #expect(model.list == [])
        #expect(model.failure == nil)
    }
}
