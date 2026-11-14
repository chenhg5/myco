import Foundation
import Testing
import MycoAPI
import MycoFake
import MycoStore
@testable import MycoCore

/// What is announced, and what the person can do about it.
@MainActor
struct NotificationTests {
    func setUp(_ change: (inout NotificationSettings) -> Void = { _ in }) async -> (WorkspaceSession, FakeHub, NotificationDesk, RecordingNotifier) {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "w1", name: "Demo", hub: hub)
        await session.refresh()
        let notifier = RecordingNotifier()
        var settings = NotificationSettings()
        settings.enabled = true
        change(&settings)
        let desk = NotificationDesk(notifier: notifier, settings: settings)
        // The first look: what was already there is not news.
        await desk.check(session, active: true)
        return (session, hub, desk, notifier)
    }

    @Test func whatWasThereBeforeIsNotAnnounced() async {
        let (session, _, _, notifier) = await setUp()
        #expect(session.notices.isEmpty)
        #expect(notifier.posted.isEmpty)
    }

    @Test func aNewMessageIsAnnouncedOnce() async {
        let (session, hub, desk, notifier) = await setUp()
        await hub.deliver(from: "studio-claude-c41f", body: "还有一件事。\n细节在后面。")
        await session.refresh()
        await desk.check(session, active: true)
        #expect(notifier.posted.count == 1)
        let notice = notifier.posted[0]
        #expect(notice.party == "studio-claude-c41f")
        #expect(notice.target == .agent("studio-claude-c41f"))
        #expect(notice.kind == .said)
        // Its first line, and no workspace name with only one workspace.
        #expect(notice.body == "还有一件事。")
        #expect(notifier.workspaces == [nil])
        // Looking again says nothing new, twice at once neither.
        await session.refresh()
        async let a: Void = desk.check(session, active: true)
        async let b: Void = desk.check(session, active: false)
        _ = await (a, b)
        #expect(notifier.posted.count == 1)

        // The coordinator's words lead to its project.
        await hub.deliver(from: "cloud-lead", body: "活都派完了。")
        await session.refresh()
        desk.severalWorkspaces = true
        await desk.check(session, active: true)
        #expect(notifier.posted.last?.target == .project("myco-ios"))
        #expect(notifier.workspaces.last == "Demo")
    }

    @Test func tellsAnAnswerFromAFailureFromAQuestion() async {
        let (session, hub, desk, notifier) = await setUp { $0.progressToo = true }
        func arrives(_ type: String, ask: Bool = false, from agent: String) async -> Notice.Kind? {
            var message = Message(id: "n-\(type)-\(agent)", from: agent, to: FakeHub.me, threadId: "h1", replyTo: "h1", type: type, body: "x", status: "done", createdAt: Date())
            if ask { message.ask = true }
            await hub.add(message)
            await session.refresh()
            await desk.check(session, active: false)
            return notifier.posted.last { $0.id == message.id }?.kind
        }
        #expect(await arrives("reply", from: "studio-claude-c41f") == .answered)
        #expect(await arrives("event", from: "studio-claude-c41f") == .failed)
        #expect(await arrives("progress", from: "studio-claude-c41f") == .progress)
        #expect(await arrives("progress", ask: true, from: "vps-codex-90aa") == .asking)
    }

    @Test func interimUpdatesOnlyOnRequest() async {
        let (session, hub, desk, notifier) = await setUp()
        await hub.add(Message(id: "p1", from: "studio-claude-c41f", to: FakeHub.me, threadId: "h1", replyTo: "h1", type: "progress", body: "half way", status: "done", createdAt: Date()))
        await session.refresh()
        await desk.check(session, active: false)
        #expect(notifier.posted.isEmpty)
        // A question is not an interim update: it needs the person.
        var question = Message(id: "p2", from: "studio-claude-c41f", to: FakeHub.me, threadId: "h1", replyTo: "h1", type: "progress", body: "A or B?", status: "done", createdAt: Date())
        question.ask = true
        await hub.add(question)
        await session.refresh()
        await desk.check(session, active: false)
        #expect(notifier.posted.map(\.kind) == [.asking])
    }

    @Test func notWhatIsOnScreenNorWhatIsMutedNorWhenOff() async {
        let (session, hub, desk, notifier) = await setUp()
        // Looking at that conversation, with the app in front: seen already.
        desk.onScreen = ("w1", "studio-claude-c41f")
        await hub.deliver(from: "studio-claude-c41f", body: "one")
        await session.refresh()
        await desk.check(session, active: true)
        #expect(notifier.posted.isEmpty)
        // The app is not in front: the same page being the last one open does not count.
        await hub.deliver(from: "studio-claude-c41f", body: "two")
        await session.refresh()
        await desk.check(session, active: false)
        #expect(notifier.posted.map(\.body) == ["two"])

        // Muted: never, though it is still unread in the list.
        desk.onScreen = nil
        desk.settings.muted = [NotificationSettings.muteKey(workspace: "w1", party: "studio-claude-c41f")]
        await hub.deliver(from: "studio-claude-c41f", body: "three")
        await hub.deliver(from: "cloud-lead", body: "from the lead")
        await session.refresh()
        await desk.check(session, active: false)
        #expect(notifier.posted.map(\.body) == ["two", "from the lead"])
        #expect(session.chatRows.first { $0.party == "studio-claude-c41f" }?.unread == true)

        // Off: nothing, and turning it on later does not bring it back.
        desk.settings.muted = []
        desk.settings.enabled = false
        await hub.deliver(from: "studio-claude-c41f", body: "four")
        await session.refresh()
        await desk.check(session, active: false)
        desk.settings.enabled = true
        await desk.check(session, active: false)
        #expect(notifier.posted.map(\.body) == ["two", "from the lead"])
    }

    @Test func quietHoursSpanTheNight() async {
        var settings = NotificationSettings()
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        func at(_ hour: Int, _ minute: Int = 0) -> Date {
            calendar.date(from: DateComponents(year: 2026, month: 11, day: 14, hour: hour, minute: minute))!
        }
        // Off: never quiet.
        #expect(!settings.isQuiet(at: at(2), calendar: calendar))
        settings.quietHours = true
        // 23:00 to 08:00, the default.
        #expect(settings.isQuiet(at: at(23), calendar: calendar))
        #expect(settings.isQuiet(at: at(2, 30), calendar: calendar))
        #expect(settings.isQuiet(at: at(7, 59), calendar: calendar))
        #expect(!settings.isQuiet(at: at(8), calendar: calendar))
        #expect(!settings.isQuiet(at: at(22, 59), calendar: calendar))
        // Within one day.
        settings.quietFrom = 13 * 60
        settings.quietTo = 14 * 60 + 30
        #expect(settings.isQuiet(at: at(13), calendar: calendar))
        #expect(settings.isQuiet(at: at(14, 29), calendar: calendar))
        #expect(!settings.isQuiet(at: at(14, 30), calendar: calendar))
        #expect(!settings.isQuiet(at: at(2), calendar: calendar))

        // During them nothing is announced, and it is not sent later either.
        let hub = FakeHub()
        let session = WorkspaceSession(id: "w1", name: "Demo", hub: hub)
        await session.refresh()
        let notifier = RecordingNotifier()
        var night = NotificationSettings()
        night.enabled = true
        night.quietHours = true
        let desk = NotificationDesk(notifier: notifier, settings: night, now: { calendar.date(from: DateComponents(year: 2026, month: 11, day: 14, hour: 3))! })
        await desk.check(session, active: false)
        await hub.deliver(from: "studio-claude-c41f", body: "at night")
        await session.refresh()
        await desk.check(session, active: false)
        #expect(notifier.posted.isEmpty)
        // The list still shows it as unread in the morning.
        #expect(session.unreadCount == 1)
    }

    @Test func remembersWhatItAnnouncedAcrossLaunches() async {
        let store = MemoryStore()
        let hub = FakeHub()
        let session = WorkspaceSession(id: "w1", name: "Demo", hub: hub, snapshots: store)
        await session.refresh()
        var settings = NotificationSettings()
        settings.enabled = true
        let notifier = RecordingNotifier()
        let desk = NotificationDesk(notifier: notifier, settings: settings)
        await desk.check(session, active: false)
        await hub.deliver(from: "studio-claude-c41f", body: "once")
        await session.refresh()
        await desk.check(session, active: false)
        #expect(notifier.posted.count == 1)

        // The next launch (or the system waking the app): not again.
        try? await Task.sleep(for: .milliseconds(50))
        let again = WorkspaceSession(id: "w1", name: "Demo", hub: hub, snapshots: store)
        await again.restore()
        await again.refresh()
        let later = RecordingNotifier()
        let desk2 = NotificationDesk(notifier: later, settings: settings)
        await desk2.check(again, active: false)
        #expect(later.posted.isEmpty)
        // But what came since is.
        await hub.deliver(from: "studio-claude-c41f", body: "twice")
        await again.refresh()
        await desk2.check(again, active: false)
        #expect(later.posted.map(\.body) == ["twice"])
    }
}
