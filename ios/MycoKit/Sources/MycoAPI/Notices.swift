import Foundation

/// Something a person should hear about (hub v0.21.21+): an agent asking
/// them, a decision, an approval, a failure, an answer. `GET
/// /v1/console/notices`.
public struct Notice: Codable, Sendable, Hashable, Identifiable {
    public var seq: Int64
    public var id: String
    /// asking, decision, approval, failed, run_failed, stuck (needs_you);
    /// answered, said, mention (results); progress (activity).
    public var kind: String
    /// needs_you, results, activity
    public var tier: String
    /// Where it leads: `agent:<name>`, `project:<name>`,
    /// `workflow:<id>?run=<run>`, `superagent`.
    public var target: String?
    public var project: String?
    /// Who it is from.
    public var actor: String?
    /// A few words, when the hub knows them (a run's name, what an
    /// approval would do); never a message's text.
    public var subject: String?
    public var messageId: String?
    public var createdAt: Date
    public var readAt: Date?
    /// It no longer waits for the person.
    public var resolvedAt: Date?

    public init(
        seq: Int64, id: String, kind: String, tier: String = "needs_you", target: String? = nil, project: String? = nil,
        actor: String? = nil, subject: String? = nil, messageId: String? = nil, createdAt: Date, readAt: Date? = nil, resolvedAt: Date? = nil
    ) {
        self.seq = seq
        self.id = id
        self.kind = kind
        self.tier = tier
        self.target = target
        self.project = project
        self.actor = actor
        self.subject = subject
        self.messageId = messageId
        self.createdAt = createdAt
        self.readAt = readAt
        self.resolvedAt = resolvedAt
    }

    /// Still waits for the person.
    public var open: Bool { tier == "needs_you" && resolvedAt == nil }
}

/// A page of notices: those after a `seq`, and how many wait.
public struct NoticePage: Codable, Sendable {
    @LossyList public var notices: [Notice]
    public var last: Int64?
    /// Open needs-you notices.
    public var badge: Int?

    public init(notices: [Notice], last: Int64? = nil, badge: Int? = nil) {
        _notices = LossyList(wrappedValue: notices)
        self.last = last
        self.badge = badge
    }
}

/// Tokens used, as agents report them (hub v0.21.34+): one row per day,
/// agent, project, model and source. `GET /v1/console/usage`.
public struct UsageRow: Codable, Sendable, Hashable {
    /// UTC YYYY-MM-DD.
    public var day: String
    public var agent: String
    public var project: String?
    public var model: String?
    public var source: String?
    public var calls: Int64?
    public var inputTokens: Int64?
    public var cacheWriteTokens: Int64?
    public var cacheReadTokens: Int64?
    public var outputTokens: Int64?
    public var costUsd: Double?

    public init(
        day: String, agent: String, project: String? = nil, model: String? = nil, source: String? = "runner", calls: Int64 = 0,
        inputTokens: Int64 = 0, cacheWriteTokens: Int64 = 0, cacheReadTokens: Int64 = 0, outputTokens: Int64 = 0, costUsd: Double? = nil
    ) {
        self.day = day
        self.agent = agent
        self.project = project
        self.model = model
        self.source = source
        self.calls = calls
        self.inputTokens = inputTokens
        self.cacheWriteTokens = cacheWriteTokens
        self.cacheReadTokens = cacheReadTokens
        self.outputTokens = outputTokens
        self.costUsd = costUsd
    }

    /// All its tokens: input, cache written and read, output (they do not
    /// overlap).
    public var total: Int64 { (inputTokens ?? 0) + (cacheWriteTokens ?? 0) + (cacheReadTokens ?? 0) + (outputTokens ?? 0) }
}

public struct UsageReport: Codable, Sendable {
    public var from: String
    public var to: String
    @LossyList public var rows: [UsageRow]

    public init(from: String, to: String, rows: [UsageRow]) {
        self.from = from
        self.to = to
        _rows = LossyList(wrappedValue: rows)
    }
}
