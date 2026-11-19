import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// An agent in one long session takes my next message and goes on, its
/// steps still filed under the first task: the page shows them in time,
/// under the message they came after.
@MainActor
struct RunWindowsTests {
    let start = Date(timeIntervalSince1970: 1_800_000_000)

    func at(_ seconds: TimeInterval) -> Date { start.addingTimeInterval(seconds) }

    func task(_ id: String, _ seconds: TimeInterval, _ status: String) -> Message {
        Message(id: id, from: "me", to: "coder", body: id, status: status, createdAt: at(seconds))
    }

    func said(_ seq: Int, _ seconds: TimeInterval?, _ text: String) -> TraceEvent {
        TraceEvent(seq: seq, kind: "text", data: TraceStep(text: text), at: seconds.map(at))
    }

    func tool(_ seq: Int, _ seconds: TimeInterval, _ id: String, _ input: String) -> TraceEvent {
        TraceEvent(seq: seq, kind: "tool", data: TraceStep(id: id, tool: "Bash", input: input), at: at(seconds))
    }

    func result(_ seq: Int, _ seconds: TimeInterval, _ id: String) -> TraceEvent {
        TraceEvent(seq: seq, kind: "tool_result", data: TraceStep(id: id, output: "ok"), at: at(seconds))
    }

    func texts(_ events: [TraceEvent]) -> [String] {
        events.map { $0.data.text ?? $0.data.input ?? $0.kind + ":" + ($0.data.id ?? "") }
    }

    @Test func whatItDoesAfterMyNextMessageShowsUnderThatMessage() {
        let windows = RunWindows(tasks: [task("t2", 100, "acked"), task("t1", 0, "acked")], answered: [], reported: [])
        #expect(windows.windows["t1"]?.until == at(100))
        #expect(windows.windows["t2"]?.lenders == ["t1"])
        #expect(windows.windows["t2"]?.since == at(100))
        // All filed under the first task, as a long session does.
        let traces = ["t1": [
            said(1, nil, "start"), said(2, 10, "looking"),
            tool(3, 20, "a", "ls"), result(4, 30, "a"),
            // Begun before my next message, done after it.
            tool(5, 90, "b", "make build"), result(6, 110, "b"),
            said(7, 120, "now the second"), tool(8, 130, "c", "make test"),
        ]]
        let first = windows.events(of: "t1", traces: traces)
        #expect(texts(first) == ["start", "looking", "ls", "tool_result:a", "make build", "tool_result:b"])
        // The build is done where it shows, not left running there.
        if case .steps(_, let steps)? = RunProcess(events: first).segments.last {
            #expect(steps.map(\.finished) == [true, true])
        } else {
            Issue.record("its steps close the first run")
        }
        let second = windows.events(of: "t2", traces: traces)
        #expect(texts(second) == ["now the second", "make test"])
        // Numbered anew: steps of two runs do not share numbers.
        #expect(second.map(\.seq) == [1, 2])
        // What tells whether it still moves is all it did.
        #expect(windows.activity(of: "t2", traces: traces).compactMap(\.at).max() == at(130))
        #expect(windows.activity(of: "t1", traces: traces).count == 8)
        // An update goes the same way.
        #expect(windows.home(of: "t1", at: at(50)) == "t1")
        #expect(windows.home(of: "t1", at: at(150)) == "t2")
        #expect(windows.home(of: "t2", at: at(150)) == "t2")
        #expect(windows.home(of: "elsewhere", at: at(150)) == "elsewhere")
    }

    @Test func aMessageNotTakenUpStillHasWhatCameAfterItUnderIt() {
        let traces = ["t1": [said(1, 10, "before"), said(2, 150, "after")]]
        // Waiting, delivered, never delivered: what the agent does for the
        // earlier task after it was sent is under it, headed with that one.
        for status in ["queued", "delivered", "failed"] {
            let windows = RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, status)], answered: [], reported: [])
            #expect(texts(windows.events(of: "t1", traces: traces)) == ["before"], "\(status)")
            #expect(texts(windows.events(of: "t2", traces: traces)) == ["after"], "\(status)")
            #expect(windows.carrying("t2", traces: traces) == "t1", "\(status)")
        }
        // Taken up: it is its own run, no heading.
        let taken = RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "acked")], answered: [], reported: [])
        #expect(taken.carrying("t2", traces: traces) == nil)
        // Said something about it: taken up too.
        let reported = RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "queued")], answered: [], reported: ["t2"])
        #expect(reported.carrying("t2", traces: traces) == nil)
        // Nothing came after it: no heading over nothing.
        let quiet = RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "queued")], answered: [], reported: [])
        #expect(quiet.carrying("t2", traces: ["t1": [said(1, 10, "before")]]) == nil)
        // The earlier one done, or answered: nothing moves.
        #expect(RunWindows(tasks: [task("t1", 0, "done"), task("t2", 100, "acked")], answered: [], reported: []).isEmpty)
        #expect(RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "acked")], answered: ["t1"], reported: []).isEmpty)
        #expect(RunWindows().home(of: "t1", at: at(500)) == "t1")
        // A long first line is cut.
        var long = task("t1", 0, "acked")
        long.body = String(repeating: "很长的任务说明", count: 6) + "\n第二行"
        let cut = RunWindows(tasks: [long, task("t2", 100, "queued")], answered: [], reported: [])
        #expect(cut.windows["t2"]?.carrying?.count == 24)
        #expect(cut.windows["t2"]?.carrying?.hasSuffix("…") == true)
    }

    @Test func eachNewerMessageTakesOverWhenItIsTakenUp() {
        let traces = [
            "t1": [said(1, 10, "one"), said(2, 150, "two"), said(3, 250, "three")],
            "t2": [said(1, 160, "its own"), said(2, 260, "its own, later")],
        ]
        // The third waits: from it on, what comes is under it, headed with
        // the one before.
        var windows = RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "acked"), task("t3", 200, "queued")], answered: [], reported: [])
        #expect(texts(windows.events(of: "t1", traces: traces)) == ["one"])
        #expect(texts(windows.events(of: "t2", traces: traces)) == ["two", "its own"])
        #expect(texts(windows.events(of: "t3", traces: traces)) == ["three", "its own, later"])
        #expect(windows.carrying("t3", traces: traces) == "t2")
        #expect(windows.home(of: "t1", at: at(250)) == "t3")
        // Taken up: from there on it is the third's.
        windows = RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "acked"), task("t3", 200, "acked")], answered: [], reported: [])
        #expect(texts(windows.events(of: "t1", traces: traces)) == ["one"])
        #expect(texts(windows.events(of: "t2", traces: traces)) == ["two", "its own"])
        #expect(texts(windows.events(of: "t3", traces: traces)) == ["three", "its own, later"])
        #expect(windows.home(of: "t1", at: at(150)) == "t2")
        #expect(windows.home(of: "t1", at: at(250)) == "t3")
        #expect(windows.home(of: "t2", at: at(250)) == "t3")
        // Nothing is shown twice, and nothing is lost.
        let all = ["t1", "t2", "t3"].flatMap { windows.events(of: $0, traces: traces) }
        #expect(all.count == 5)
    }

    @Test func anUpdateShowsInTheRunItCameIn() {
        let messages = [
            task("t1", 0, "acked"), task("t2", 100, "acked"),
            Message(id: "p1", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "first half", status: "done", createdAt: at(50)),
            Message(id: "p2", from: "coder", to: "me", threadId: "t1", replyTo: "t1", type: "progress", body: "second half", status: "done", createdAt: at(150)),
        ]
        let windows = RunWindows(tasks: Array(messages.prefix(2)), answered: [], reported: ["t1"])
        #expect(ConversationBuilder.runExtras(of: "t1", agent: "coder", messages: messages, windows: windows).reports.map(\.id) == ["p1"])
        #expect(ConversationBuilder.runExtras(of: "t2", agent: "coder", messages: messages, windows: windows).reports.map(\.id) == ["p2"])
        // Both runs shown: neither update is a row of the conversation too.
        var rows = ConversationBuilder.items(
            me: "me", agent: "coder", messages: messages, processes: ["t1", "t2"], windows: windows, status: { _ in nil }, now: at(200))
        #expect(!rows.contains { $0.id == "p1" || $0.id == "p2" })
        // The newer message has no run to show yet: its update is a row,
        // after that message, not lost.
        rows = ConversationBuilder.items(
            me: "me", agent: "coder", messages: messages, processes: ["t1"], windows: windows, status: { _ in nil }, now: at(200))
        #expect(!rows.contains { $0.id == "p1" })
        let order = rows.map(\.id)
        #expect(order.firstIndex(of: "p2").map { $0 > (order.firstIndex(of: "t2") ?? .max) } == true)
    }

    @Test func aLongSessionOnThePage() async throws {
        let hub = FakeHub(scenario: .session)
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        let model = session.conversation(with: "studio-claude-c41f")
        await model.reload()
        // Under the first message: what it did before the second.
        let first = model.run(of: "s1"), second = model.run(of: "s2")
        #expect(!first.isEmpty)
        #expect(!second.isEmpty, "the newer message has what it did since, not only a loading")
        func reported(_ parts: [RunPart]) -> [String] {
            parts.flatMap { part -> [String] in
                if case .reports(_, let reports) = part { return reports.map(\.text) }
                return []
            }
        }
        #expect(reported(first).isEmpty)
        #expect(reported(second) == ["第二场的录音也拿到了，正在一起整理。"])
        func steps(_ parts: [RunPart]) -> [ToolStep] {
            parts.flatMap { part -> [ToolStep] in
                if case .steps(_, let steps) = part { return steps }
                return []
            }
        }
        // (the fake's first task: a thought, a line, a command and its
        // result before my second message; a file read after it)
        #expect(steps(first).filter { $0.tool == "Bash" }.map(\.finished) == [true])
        #expect(steps(second).map(\.tool) == ["Read"])
        // In order on the page: each message, then its run.
        let order = model.items.map(\.id)
        let places = ["s1", "s1.process", "s2", "s2.process"].compactMap { order.firstIndex(of: $0) }
        #expect(places.count == 4)
        #expect(places == places.sorted())
        #expect(!order.contains("s1p"), "the update is in the run, not a row of its own")
    }

    func message(_ id: String, _ seconds: TimeInterval, from: String, to: String, type: String, replyTo: String? = nil, body: String? = nil) -> Message {
        Message(
            id: id, from: from, to: to, threadId: replyTo ?? id, replyTo: replyTo, type: type, body: body ?? id, status: "done",
            createdAt: at(seconds))
    }

    func quote(of id: String, in rows: [ConversationItem]) -> String? {
        rows.first { $0.id == id }?.quote
    }

    @Test func anAnswerOrAnAdditionNotRightUnderItsTaskQuotesIt() {
        let messages = [
            task("t1", 0, "done"), task("t2", 100, "acked"),
            // t1's answer comes after t2 was sent: it quotes t1.
            message("r1", 150, from: "coder", to: "me", type: "reply", replyTo: "t1"),
            // An addition to t2 after that answer: drawn where sent, quoting t2.
            message("s2", 200, from: "me", to: "coder", type: "steer", replyTo: "t2", body: "and the title"),
        ]
        let rows = ConversationBuilder.items(me: "me", agent: "coder", messages: messages, processes: ["t2"], status: { _ in nil }, now: at(300))
        #expect(quote(of: "r1", in: rows) == "t1")
        #expect(quote(of: "s2", in: rows) == "t2")
        #expect(rows.contains { $0.id == "s2" }, "not inside the run above")
        #expect(ConversationBuilder.runExtras(of: "t2", agent: "coder", messages: messages).added.isEmpty)
        // Right under its task, nothing quoted; an addition with nothing
        // between goes into the run.
        let close = [
            task("t1", 0, "acked"),
            message("s1", 50, from: "me", to: "coder", type: "steer", replyTo: "t1", body: "and the title"),
            message("r1", 80, from: "coder", to: "me", type: "reply", replyTo: "t1"),
        ]
        let near = ConversationBuilder.items(me: "me", agent: "coder", messages: close, processes: ["t1"], status: { _ in nil }, now: at(100))
        #expect(quote(of: "r1", in: near) == nil)
        #expect(!near.contains { $0.id == "s1" })
        #expect(ConversationBuilder.runExtras(of: "t1", agent: "coder", messages: close).added.map(\.id) == ["s1"])
    }

    @Test func aTaskDeliveredIntoASessionSaysSo() {
        let agent = AgentStatus(agent: Agent(id: "a", name: "coder"), state: "idle", receiving: "session")
        let messages = [task("t1", 0, "delivered")]
        let rows = ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in agent }, now: at(10))
        #expect(rows.first { $0.id == "t1" }?.receipt == .delivered)
        // Woken by a runner: not opened yet, as before.
        let runner = AgentStatus(agent: Agent(id: "a", name: "coder"), state: "idle", receiving: "runner")
        let other = ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in runner }, now: at(10))
        #expect(other.first { $0.id == "t1" }?.receipt == .notOpened)
    }

    @Test func theLoadingSaysWhatTheAgentIsDoingNow() {
        let agent = AgentStatus(agent: Agent(id: "a", name: "coder"), state: "working")
        // A coordinator keeps t1 open while it answers newer messages.
        var messages = [
            task("t1", 0, "acked"),
            message("p1", 10, from: "coder", to: "me", type: "progress", replyTo: "t1", body: "starting on the first"),
            message("n2", 100, from: "me", to: "coder", type: "note", body: "a question"),
            message("r2", 110, from: "coder", to: "me", type: "reply", replyTo: "n2"),
        ]
        func foot(_ messages: [Message]) -> ConversationItem? {
            ConversationBuilder.items(me: "me", agent: "coder", messages: messages, status: { _ in agent }, now: at(130))
                .first { $0.id == "t1.status" }
        }
        // It answered since its update: the update is old news.
        #expect(foot(messages)?.saying == nil)
        // Its clock runs from the latest message here.
        if case .status(.working(let since), _)? = foot(messages)?.kind {
            #expect(since == at(110))
        } else {
            Issue.record("it is at work")
        }
        // A newer update, about any task: that is what it is doing now.
        messages.append(message("p2", 120, from: "coder", to: "me", type: "progress", replyTo: "t1", body: "now the second part\nmore"))
        #expect(foot(messages)?.saying == "now the second part")
        // What it said between its steps since: that, not the older update
        // (the run above goes on; the line goes with it).
        func footSaying(_ words: (text: String, at: Date)?) -> String? {
            ConversationBuilder.items(me: "me", agent: "coder", messages: messages, stepWords: words, status: { _ in agent }, now: at(130))
                .first { $0.id == "t1.status" }?.saying
        }
        #expect(footSaying(("Implementing the editor changes: types\nand more", at(125))) == "Implementing the editor changes: types")
        // Said before the update: the update stands.
        #expect(footSaying(("Reading the editor", at(115))) == "now the second part")
        // It answered after both: neither.
        messages.append(message("r3", 128, from: "coder", to: "me", type: "reply", replyTo: "t1"))
        #expect(footSaying(("Implementing the editor changes", at(125))) == nil)
    }
}
