import Foundation
import Testing
import MycoAPI
@testable import MycoCore

struct RunStateTests {
    let now = Date(timeIntervalSince1970: 1_800_000_000)

    func task(_ status: String, quietFor seconds: TimeInterval = 5, error: String? = nil) -> Message {
        Message(id: "t", from: "me", to: "coder", body: "do it", status: status, error: error,
                createdAt: now.addingTimeInterval(-seconds - 10), updatedAt: now.addingTimeInterval(-seconds))
    }

    func coder(_ state: String = "working", receiving: String = "runner") -> AgentStatus {
        AgentStatus(agent: Agent(id: "a", name: "coder"), state: state, receiving: receiving)
    }

    @Test func onlyATaskTakenOnCountsAsWorking() {
        // Pulled but not acked, or held, is still waiting.
        for status in ["queued", "delivered", "held"] {
            #expect(RunState.of(task(status), recipient: coder(), now: now) == .queued)
        }
        #expect(RunState.of(task("acked"), recipient: coder(), now: now) == .working(since: now.addingTimeInterval(-5)))
    }

    @Test func quietTooLongIsPossiblyStuck() {
        let elevenMinutes: TimeInterval = 11 * 60
        #expect(RunState.of(task("acked", quietFor: elevenMinutes), recipient: coder(), now: now) == .maybeStuck(quietSince: now.addingTimeInterval(-elevenMinutes)))
        // A node that only receives in its own session gets half an hour.
        guard case .working = RunState.of(task("acked", quietFor: elevenMinutes), recipient: coder(receiving: "session"), now: now) else {
            Issue.record("a session node quiet for 11 minutes is still working")
            return
        }
        // Away: at once.
        guard case .maybeStuck = RunState.of(task("acked"), recipient: coder("offline"), now: now) else {
            Issue.record("an acked task on a node that is away may be stuck")
            return
        }
    }

    @Test func aTaskISToppedIsNotAFailure() {
        #expect(RunState.of(task("failed", error: RunState.stoppedBySender), recipient: nil, now: now) == .stopped)
        #expect(RunState.of(task("failed", error: "boom"), recipient: nil, now: now) == .failed(reason: "boom"))
        #expect(RunState.of(task("done"), recipient: nil, now: now) == .done)
    }

    @Test func theSendButtonFollowsTheOpenTask() {
        #expect(ComposerAction.resolve(hasOpenTask: false, hasText: false) == .send)
        #expect(ComposerAction.resolve(hasOpenTask: false, hasText: true) == .send)
        #expect(ComposerAction.resolve(hasOpenTask: true, hasText: false) == .stop)
        #expect(ComposerAction.resolve(hasOpenTask: true, hasText: true) == .steer)
    }
}

struct NamingTests {
    let machines = [Machine(id: "m1", name: "Studio Mac", hostname: "Studio.local")]

    func agent(_ name: String, display: String? = nil, harness: String = "claude", dir: String = "~/code/myco", kind: String = "agent", day: Double = 0) -> Agent {
        Agent(id: "id-" + name, name: name, displayName: display, binding: Binding(harness: harness, workdir: dir),
              createdAt: Date(timeIntervalSince1970: 1_800_000_000 + day * 86_400), kind: kind, machineId: "m1")
    }

    @Test func aDisplayNameWins() {
        let a = agent("studio-claude-x9k2", display: "后端工程师")
        #expect(Naming.name(of: a, among: [a], machines: machines) == "后端工程师")
    }

    @Test func aNameSomeoneChoseStays() {
        let a = agent("docs-writer")
        #expect(Naming.name(of: a, among: [a], machines: machines) == "docs-writer")
    }

    @Test func aGeneratedNameBecomesTheFolder() {
        let first = agent("studio-claude-x9k2", day: 0)
        let second = agent("laptop-codex-0a1b", harness: "codex", day: 1)
        let third = agent("box-claude-77ff", dir: "/srv/myco/", day: 2)
        let all = [third, first, second]
        AppLanguage.$forced.withValue(.zh) {
            #expect(Naming.name(of: first, among: all, machines: machines) == "myco 助手")
            #expect(Naming.name(of: second, among: all, machines: machines) == "myco 助手 二号")
            #expect(Naming.name(of: third, among: all, machines: machines) == "myco 助手 三号")
        }
        AppLanguage.$forced.withValue(.en) {
            #expect(Naming.name(of: second, among: all, machines: machines) == "myco assistant #2")
        }
    }

    @Test func aComputerIsWhichOneAndTheFolder() {
        let shell = agent("studio-exec-12ab", harness: "exec", dir: "C:\\\\work\\\\site", kind: "shell")
        #expect(Naming.name(of: shell, among: [shell], machines: machines) == "Studio Mac · site")
    }

    @Test func tellsGeneratedNamesFromChosenOnes() {
        #expect(Naming.isGenerated("glenn-mac-claude-x9k2", hostname: nil) == false) // x9k2 is not hex
        #expect(Naming.isGenerated("glenn-mac-claude-9f2a", hostname: nil))
        #expect(Naming.isGenerated("studio-notes", hostname: "Studio.local"))
        #expect(Naming.isGenerated("mac-exec", hostname: "studio") == false)
    }
}

struct ConversationBuilderTests {
    let now = Date(timeIntervalSince1970: 1_800_000_000)
    func at(_ seconds: TimeInterval) -> Date { now.addingTimeInterval(seconds) }

    @Test func aRoundReadsInOrderWithItsOpenTaskLast() {
        let messages = [
            Message(id: "r1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "reply", body: "done", status: "done", createdAt: at(-80)),
            Message(id: "t1", from: "me", to: "coder", body: "first", status: "done", createdAt: at(-100)),
            Message(id: "t2", from: "me", to: "coder", body: "second", status: "acked", createdAt: at(-60), updatedAt: at(-5)),
            Message(id: "s1", from: "me", to: "coder", threadId: "t2", type: "steer", body: "also this", status: "done", createdAt: at(-30)),
            Message(id: "p1", from: "coder", to: "me", threadId: "t2", type: "progress", body: "halfway", status: "done", createdAt: at(-20)),
        ]
        let items = ConversationBuilder.items(me: "me", messages: messages, others: [], status: { _ in nil }, now: now)
        #expect(items.map(\.id) == ["t1", "r1", "t2", "s1", "p1", "t2.status"])
        #expect(items[3].kind == .mine(text: "also this", steer: true))
        #expect(items[5].kind == .status(.working(since: at(-5)), taskID: "t2"))
        #expect(ConversationBuilder.openTask(me: "me", messages: messages)?.id == "t2")
    }

    @Test func othersWorkShowsAsCapsulesInTime() {
        let mine = [Message(id: "t1", from: "me", to: "lead", body: "plan it", status: "done", createdAt: at(-100))]
        let others = [Message(id: "d1", from: "lead", to: "coder", body: "write the API\\nwith tests", status: "queued", createdAt: at(-50))]
        let items = ConversationBuilder.items(me: "me", messages: mine, others: others, status: { _ in nil }, now: now)
        #expect(items.map(\.id) == ["t1", "d1.dispatch"])
        guard case .dispatch(let dispatch) = items[1].kind else {
            Issue.record("the coordinator's task is a capsule")
            return
        }
        #expect(dispatch.to == "coder")
        #expect(dispatch.state == .queued)
    }

    @Test func aStoppedTaskSaysSo() {
        let messages = [Message(id: "t1", from: "me", to: "coder", body: "x", status: "failed", error: RunState.stoppedBySender, createdAt: at(-10))]
        let items = ConversationBuilder.items(me: "me", messages: messages, others: [], status: { _ in nil }, now: now)
        #expect(items.map(\.kind) == [.mine(text: "x", steer: false), .stopped])
        #expect(ConversationBuilder.openTask(me: "me", messages: messages) == nil)
    }
}
