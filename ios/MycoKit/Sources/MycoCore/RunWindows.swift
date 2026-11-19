import Foundation
import MycoAPI

/// Which of an agent's steps and updates show under which of my messages.
///
/// Strictly in time: once I have sent my next message, nothing an earlier
/// task does grows above it. An agent in one long session (it receives in
/// its conversation) picks up what I send next and goes on, while its steps
/// and updates still go to the task it took first, which stays open; and
/// any agent may still be at the earlier task when the next one comes. So a
/// task shows its own steps up to my next message, and that message shows
/// what the earlier tasks still open got since: as its own run once the
/// agent has taken it up, else headed "接着做上一件" (the earlier task's
/// first words) while it says it waits behind.
///
/// The desktop app's `until` and `borrow`.
public struct RunWindows: Sendable, Hashable {
    public struct Window: Sendable, Hashable {
        /// Its own steps show up to here: my next message.
        public var until: Date?
        /// The earlier tasks still open whose steps show here, from `since`
        /// (when this one was sent) to `borrowedUntil` (my next message).
        public var lenders: [String] = []
        public var since: Date?
        public var borrowedUntil: Date?
        /// Not taken up yet: what it shows is still the earlier task's,
        /// whose first words these are.
        public var carrying: String?
    }

    public private(set) var windows: [String: Window] = [:]

    public init() {}

    /// - Parameters:
    ///   - tasks: what I asked the agent, in this conversation.
    ///   - answered: the tasks it has replied to.
    ///   - reported: the tasks it has sent an update on.
    public init(tasks: [Message], answered: Set<String>, reported: Set<String>) {
        let mine = tasks.sorted { ($0.createdAt, $0.id) < ($1.createdAt, $1.id) }
        for (i, task) in mine.enumerated() {
            let earlier = mine[..<i].filter { $0.isOpen && !answered.contains($0.id) }
            guard let last = earlier.last else { continue }
            // Taken up: it began on it, or said something about it.
            let taken = ["acked", "held", "done"].contains(task.status) || reported.contains(task.id)
            var window = windows[task.id] ?? Window()
            window.lenders = earlier.map(\.id)
            window.since = task.createdAt
            window.borrowedUntil = i + 1 < mine.count ? mine[i + 1].createdAt : nil
            window.carrying = taken ? nil : Self.title(of: last)
            windows[task.id] = window
            for id in window.lenders where windows[id]?.until == nil {
                windows[id, default: Window()].until = task.createdAt
            }
        }
    }

    /// A task's first words, as a heading says it.
    static func title(of task: Message) -> String {
        let line = ConversationBuilder.firstLine(of: task.body)
        return line.count > 24 ? String(line.prefix(23)) + "…" : line
    }

    /// The earlier task a run carries on with, when its own task has not
    /// been taken up yet and shows only what that one did since.
    public func carrying(_ taskID: String, traces: [String: [TraceEvent]]) -> String? {
        guard let window = windows[taskID], let words = window.carrying, !words.isEmpty else { return nil }
        guard RunProcess.withoutChores(traces[taskID] ?? []).isEmpty, !events(of: taskID, traces: traces).isEmpty else { return nil }
        return words
    }

    public var isEmpty: Bool { windows.isEmpty }

    /// The steps a task shows: its own up to my next message, and those
    /// the earlier tasks got since it was sent. In time, numbered anew
    /// (steps of two runs do not share numbers). Without the turns the
    /// agent woke for only to keep listening.
    public func events(of taskID: String, traces: [String: [TraceEvent]]) -> [TraceEvent] {
        let own = RunProcess.withoutChores(traces[taskID] ?? [])
        guard let window = windows[taskID] else { return own }
        let shown = window.until.map { Self.slice(own, from: nil, until: $0) } ?? own
        let more = borrowed(by: taskID, traces: traces, until: window.borrowedUntil)
        guard !more.isEmpty else { return shown }
        return Self.inTime(shown + more)
    }

    /// All a task did, for telling whether it still moves: its own steps,
    /// cut or not, and the ones shown under it.
    public func activity(of taskID: String, traces: [String: [TraceEvent]]) -> [TraceEvent] {
        let own = RunProcess.withoutChores(traces[taskID] ?? [])
        let more = borrowed(by: taskID, traces: traces)
        return more.isEmpty ? own : Self.inTime(own + more)
    }

    /// The task under which something filed under `taskID` at a time
    /// shows: that task, or my later message that had been taken up by then.
    public func home(of taskID: String, at: Date) -> String {
        guard let until = windows[taskID]?.until, at >= until else { return taskID }
        let takers = windows.filter { _, window in
            guard window.lenders.contains(taskID), let since = window.since, at >= since else { return false }
            return window.borrowedUntil.map { at < $0 } ?? true
        }
        return takers.max { ($0.value.since ?? .distantPast) < ($1.value.since ?? .distantPast) }?.key ?? taskID
    }

    private func borrowed(by taskID: String, traces: [String: [TraceEvent]], until: Date? = nil) -> [TraceEvent] {
        guard let window = windows[taskID], let since = window.since else { return [] }
        return window.lenders.flatMap { Self.slice(RunProcess.withoutChores(traces[$0] ?? []), from: since, until: until) }
    }

    /// The steps of one run between two times, oldest first. A step with
    /// no time counts as the run's start. A command's result stays with
    /// the command: one begun before my next message and done after it is
    /// done where it shows, not left running there.
    private static func slice(_ events: [TraceEvent], from: Date?, until: Date?) -> [TraceEvent] {
        var kept: Set<String> = []
        var left: Set<String> = []
        return events.filter { event in
            let call = event.data.id ?? ""
            if event.kind == "tool_result", !call.isEmpty {
                if kept.contains(call) { return true }
                if left.contains(call) { return false }
            }
            var inside = true
            if let from { inside = (event.at ?? .distantPast) >= from }
            if inside, let until, let at = event.at { inside = at < until }
            if event.kind == "tool", !call.isEmpty {
                if inside { kept.insert(call) } else { left.insert(call) }
            }
            return inside
        }
    }

    private static func inTime(_ events: [TraceEvent]) -> [TraceEvent] {
        events.enumerated()
            .sorted { a, b in
                let (x, y) = (a.element.at ?? .distantPast, b.element.at ?? .distantPast)
                if x != y { return x < y }
                if a.element.seq != b.element.seq { return a.element.seq < b.element.seq }
                return a.offset < b.offset
            }
            .enumerated()
            .map { index, entry in
                var event = entry.element
                event.seq = index + 1
                return event
            }
    }
}
