import Foundation
import MycoAPI

/// Which of an agent's steps and updates show under which of my messages.
///
/// An agent in one long session (it receives in its conversation) picks up
/// what I send next and goes on, while its steps and updates still go to
/// the task it took first, which stays open. Shown as filed, all of that
/// would sit above my newer message, and the newer one would have nothing
/// under it. So the page goes by time: a task shows its own steps up to my
/// next message that was taken up, and that message shows what the earlier
/// tasks still open got since. An agent woken by a runner starts the next
/// task only after the first is done: nothing moves there.
///
/// The desktop app's `until` and `borrow`.
public struct RunWindows: Sendable, Hashable {
    public struct Window: Sendable, Hashable {
        /// Its own steps show up to here: my next message that was taken up.
        public var until: Date?
        /// The earlier tasks still open whose steps show here, from `since`
        /// (when this one was sent) to `borrowedUntil` (the next one taken up).
        public var lenders: [String] = []
        public var since: Date?
        public var borrowedUntil: Date?
    }

    public private(set) var windows: [String: Window] = [:]

    public init() {}

    /// - Parameters:
    ///   - tasks: what I asked the agent, in this conversation.
    ///   - answered: the tasks it has replied to.
    ///   - reported: the tasks it has sent an update on.
    public init(tasks: [Message], answered: Set<String>, reported: Set<String>) {
        let mine = tasks.sorted { ($0.createdAt, $0.id) < ($1.createdAt, $1.id) }
        // Taken up: it began on it, or said something about it. One that
        // failed before that (never delivered, taken back) was not.
        let taken = mine.map { ["acked", "held", "done"].contains($0.status) || reported.contains($0.id) }
        for (i, task) in mine.enumerated() where taken[i] {
            let lenders = mine[..<i].filter { $0.isOpen && !answered.contains($0.id) }.map(\.id)
            guard !lenders.isEmpty else { continue }
            var window = windows[task.id] ?? Window()
            window.lenders = lenders
            window.since = task.createdAt
            window.borrowedUntil = mine.indices.first { $0 > i && taken[$0] }.map { mine[$0].createdAt }
            windows[task.id] = window
            for id in lenders where windows[id]?.until == nil {
                windows[id, default: Window()].until = task.createdAt
            }
        }
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
