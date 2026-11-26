import Foundation

/// An account's usage limits at one moment, as its CLI or session reported
/// them (`api.Quota`, hubs from v0.19.5). Only its owner sees it.
public struct Quota: Codable, Sendable, Hashable {
    /// claude, codex, opencode_go, minimax ("unknown": no supported source).
    public var provider: String?
    public var source: String?
    public var plan: String?
    public var machineId: String?
    /// event, cloud_session, ...
    public var sourceKind: String?
    /// ok, or why the last read failed (auth_required, no_credentials, ...).
    public var status: String?
    public var at: Date?
    @LossyList public var windows: [QuotaWindow]

    public init(provider: String? = nil, source: String? = nil, plan: String? = nil, status: String? = "ok", at: Date? = nil, windows: [QuotaWindow] = []) {
        self.provider = provider
        self.source = source
        self.plan = plan
        self.status = status
        self.at = at
        _windows = LossyList(wrappedValue: windows)
    }
}

/// One limit: `used` is the share used, 0...1.
public struct QuotaWindow: Codable, Sendable, Hashable {
    /// five_hour, seven_day (Claude), primary, secondary (Codex), ...
    public var name: String
    public var label: String?
    public var durationSec: Int?
    public var modelScope: String?
    public var used: Double
    public var resetsAt: Date?
    /// allowed, or why work is refused.
    public var status: String?

    public init(name: String, label: String? = nil, durationSec: Int? = nil, used: Double, resetsAt: Date? = nil, status: String? = "allowed") {
        self.name = name
        self.label = label
        self.durationSec = durationSec
        self.used = used
        self.resetsAt = resetsAt
        self.status = status
    }
}

/// A durable, bounded wait an agent runs in the background (`myco watch`,
/// hubs from v0.22): for a reply, a task, a workflow run or a command.
public struct Watch: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var agent: String
    public var project: String?
    public var task: String?
    public var what: String
    /// reply, task, workflow, command
    public var kind: String
    public var startedAt: Date
    public var deadline: Date
    public var lastCheckAt: Date?
    /// A status summary, never the target's content.
    public var lastResult: String?
    public var checks: Int?
    /// watching, met, expired, stopped
    public var state: String
    public var endedAt: Date?
    public var endedBy: String?
    public var outcome: String?

    public init(
        id: String, agent: String, project: String? = nil, task: String? = nil, what: String, kind: String = "reply",
        startedAt: Date, deadline: Date, lastCheckAt: Date? = nil, lastResult: String? = nil, checks: Int? = 0,
        state: String = "watching", endedAt: Date? = nil, endedBy: String? = nil, outcome: String? = nil
    ) {
        self.id = id
        self.agent = agent
        self.project = project
        self.task = task
        self.what = what
        self.kind = kind
        self.startedAt = startedAt
        self.deadline = deadline
        self.lastCheckAt = lastCheckAt
        self.lastResult = lastResult
        self.checks = checks
        self.state = state
        self.endedAt = endedAt
        self.endedBy = endedBy
        self.outcome = outcome
    }

    public var isOn: Bool { state == "watching" }
}

/// Bounded work on an objective, set on an agent by a person or its
/// project's coordinator (`myco goal`, hubs from v0.22): the agent is woken
/// every so often until it is done, the rounds or the time run out.
public struct Goal: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var agent: String
    public var project: String?
    public var thread: String?
    public var objective: String
    public var doneWhen: String
    public var everySec: Int
    public var baseEverySec: Int?
    public var maxRounds: Int
    public var deadline: Date
    public var round: Int
    public var noProgress: Int?
    public var lastSummary: String?
    public var nextStep: String?
    public var nextWakeAt: Date?
    /// active, paused, achieved, abandoned, expired, stopped
    public var state: String
    public var startedAt: Date
    public var endedAt: Date?
    public var endedBy: String?
    public var outcome: String?
    public var createdBy: String?

    public init(
        id: String, agent: String, project: String? = nil, thread: String? = nil, objective: String, doneWhen: String,
        everySec: Int = 1800, baseEverySec: Int? = 1800, maxRounds: Int = 16, deadline: Date, round: Int = 0, noProgress: Int? = 0,
        lastSummary: String? = nil, nextStep: String? = nil, nextWakeAt: Date? = nil, state: String = "active", startedAt: Date,
        endedAt: Date? = nil, endedBy: String? = nil, outcome: String? = nil, createdBy: String? = nil
    ) {
        self.id = id
        self.agent = agent
        self.project = project
        self.thread = thread
        self.objective = objective
        self.doneWhen = doneWhen
        self.everySec = everySec
        self.baseEverySec = baseEverySec
        self.maxRounds = maxRounds
        self.deadline = deadline
        self.round = round
        self.noProgress = noProgress
        self.lastSummary = lastSummary
        self.nextStep = nextStep
        self.nextWakeAt = nextWakeAt
        self.state = state
        self.startedAt = startedAt
        self.endedAt = endedAt
        self.endedBy = endedBy
        self.outcome = outcome
        self.createdBy = createdBy
    }

    /// Still going (a paused goal still runs out).
    public var isOn: Bool { state == "active" || state == "paused" }
}

/// One wake of a goal and what the agent said about it.
public struct GoalRound: Codable, Sendable, Hashable {
    public var round: Int
    public var startedAt: Date
    public var endedAt: Date?
    public var summary: String?
    public var nextStep: String?
    public var progressed: Bool?
    public var blocked: Bool?

    public init(round: Int, startedAt: Date, endedAt: Date? = nil, summary: String? = nil, nextStep: String? = nil, progressed: Bool? = nil, blocked: Bool? = nil) {
        self.round = round
        self.startedAt = startedAt
        self.endedAt = endedAt
        self.summary = summary
        self.nextStep = nextStep
        self.progressed = progressed
        self.blocked = blocked
    }
}

/// What a new goal asks for.
public struct GoalRequest: Codable, Sendable, Hashable {
    public var agent: String
    public var objective: String
    public var doneWhen: String
    public var project: String?
    public var thread: String?
    public var everySec: Int?
    public var maxRounds: Int?
    public var forSec: Int?

    public init(agent: String, objective: String, doneWhen: String, project: String? = nil, thread: String? = nil, everySec: Int? = nil, maxRounds: Int? = nil, forSec: Int? = nil) {
        self.agent = agent
        self.objective = objective
        self.doneWhen = doneWhen
        self.project = project
        self.thread = thread
        self.everySec = everySec
        self.maxRounds = maxRounds
        self.forSec = forSec
    }
}
