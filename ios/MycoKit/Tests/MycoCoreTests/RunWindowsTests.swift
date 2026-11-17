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

    @Test func aMessageNotTakenUpMovesNothing() {
        // Still waiting, or never delivered: what the agent does is the
        // first task's.
        #expect(RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "queued")], answered: [], reported: []).isEmpty)
        #expect(RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "delivered")], answered: [], reported: []).isEmpty)
        #expect(RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "failed")], answered: [], reported: []).isEmpty)
        // It said something about it: taken up, whatever its status says.
        #expect(!RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "queued")], answered: [], reported: ["t2"]).isEmpty)
        // An agent woken by a runner starts the next when the first is
        // done, or answered: nothing moves.
        #expect(RunWindows(tasks: [task("t1", 0, "done"), task("t2", 100, "acked")], answered: [], reported: []).isEmpty)
        #expect(RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "acked")], answered: ["t1"], reported: []).isEmpty)
        #expect(RunWindows().home(of: "t1", at: at(500)) == "t1")
    }

    @Test func eachNewerMessageTakesOverWhenItIsTakenUp() {
        let traces = [
            "t1": [said(1, 10, "one"), said(2, 150, "two"), said(3, 250, "three")],
            "t2": [said(1, 160, "its own"), said(2, 260, "its own, later")],
        ]
        // The third waits: the second still shows all that comes.
        var windows = RunWindows(tasks: [task("t1", 0, "acked"), task("t2", 100, "acked"), task("t3", 200, "queued")], answered: [], reported: [])
        #expect(texts(windows.events(of: "t1", traces: traces)) == ["one"])
        #expect(texts(windows.events(of: "t2", traces: traces)) == ["two", "its own", "three", "its own, later"])
        #expect(windows.events(of: "t3", traces: traces).isEmpty)
        #expect(windows.home(of: "t1", at: at(250)) == "t2")
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
}
