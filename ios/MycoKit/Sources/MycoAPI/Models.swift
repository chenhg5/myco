import Foundation

// Types mirror internal/api (the hub's JSON contract). Only the fields the
// app reads are here; unknown fields are ignored, and fields the hub leaves
// out when empty are optional.

public struct Binding: Codable, Sendable, Hashable {
    public var harness: String?
    public var sessionId: String?
    public var environment: String?
    public var workdir: String?

    public init(harness: String? = nil, sessionId: String? = nil, environment: String? = nil, workdir: String? = nil) {
        self.harness = harness
        self.sessionId = sessionId
        self.environment = environment
        self.workdir = workdir
    }
}

/// What a node actually runs; absent on nodes reported by old runners.
public struct Runtime: Codable, Sendable, Hashable {
    public var cli: String?
    public var cliVersion: String?
    public var model: String?

    public init(cli: String? = nil, cliVersion: String? = nil, model: String? = nil) {
        self.cli = cli
        self.cliVersion = cliVersion
        self.model = model
    }
}

public struct Agent: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var displayName: String?
    /// "" or absent: the face drawn from the id; `look:…`; `pic:<hash>`.
    public var avatar: String?
    public var statusText: String?
    public var description: String
    public var binding: Binding
    public var online: Bool
    public var createdAt: Date
    /// agent | shell | human | machine (older hubs: absent, see the harness).
    public var kind: String?
    public var owner: String?
    public var machineId: String?
    public var runtime: Runtime?
    @LossyList public var labels: [String]

    public init(
        id: String, name: String, displayName: String? = nil, avatar: String? = nil, statusText: String? = nil,
        description: String = "", binding: Binding = Binding(), online: Bool = true, createdAt: Date = Date(),
        kind: String? = "agent", owner: String? = nil, machineId: String? = nil, runtime: Runtime? = nil, labels: [String] = []
    ) {
        self.id = id
        self.name = name
        self.displayName = displayName
        self.avatar = avatar
        self.statusText = statusText
        self.description = description
        self.binding = binding
        self.online = online
        self.createdAt = createdAt
        self.kind = kind
        self.owner = owner
        self.machineId = machineId
        self.runtime = runtime
        self.labels = labels
    }
}

public struct Machine: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var hostname: String?
    public var os: String?
    public var online: Bool

    public init(id: String, name: String, hostname: String? = nil, os: String? = nil, online: Bool = true) {
        self.id = id
        self.name = name
        self.hostname = hostname
        self.os = os
        self.online = online
    }
}

public struct Attachment: Codable, Sendable, Hashable {
    public var fileId: String
    public var name: String
    public var size: Int64
    public var contentType: String?

    public init(fileId: String, name: String, size: Int64, contentType: String? = nil) {
        self.fileId = fileId
        self.name = name
        self.size = size
        self.contentType = contentType
    }
}

public struct Message: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var from: String
    public var to: String
    public var threadId: String
    public var replyTo: String?
    public var after: String?
    /// task | note | reply | event | progress | steer
    public var type: String
    public var body: String
    /// queued | delivered | acked | held | done | failed
    public var status: String
    public var error: String?
    public var project: String?
    public var fromDisplay: String?
    public var e2e: Bool?
    /// Set when the body is sealed and this device is not one of its readers.
    public var unreadable: String?
    public var createdAt: Date
    public var updatedAt: Date
    /// The recipient's latest observed activity on an open task.
    public var activeAt: Date?
    @LossyList public var attachments: [Attachment]

    public init(
        id: String, from: String, to: String, threadId: String? = nil, replyTo: String? = nil, after: String? = nil,
        type: String = "task", body: String, status: String = "queued", error: String? = nil, project: String? = nil,
        createdAt: Date = Date(), updatedAt: Date? = nil, activeAt: Date? = nil, attachments: [Attachment] = []
    ) {
        self.id = id
        self.from = from
        self.to = to
        self.threadId = threadId ?? id
        self.replyTo = replyTo
        self.after = after
        self.type = type
        self.body = body
        self.status = status
        self.error = error
        self.project = project
        self.createdAt = createdAt
        self.updatedAt = updatedAt ?? createdAt
        self.activeAt = activeAt
        self.attachments = attachments
    }

    /// A task nobody has finished yet.
    public var isOpen: Bool {
        type == "task" && ["queued", "delivered", "held", "acked"].contains(status)
    }
}

public struct AgentStatus: Codable, Sendable, Hashable, Identifiable {
    public var agent: Agent
    /// working | pending | idle | offline
    public var state: String
    public var queued: Int
    public var inProgress: Int
    public var held: Int
    /// `session` (only while its own session waits) or `runner`.
    public var receiving: String?
    public var sessionInUse: Bool?
    @LossyList public var current: [Message]

    public var id: String { agent.id }

    public init(
        agent: Agent, state: String, queued: Int = 0, inProgress: Int = 0, held: Int = 0,
        receiving: String? = "runner", sessionInUse: Bool? = nil, current: [Message] = []
    ) {
        self.agent = agent
        self.state = state
        self.queued = queued
        self.inProgress = inProgress
        self.held = held
        self.receiving = receiving
        self.sessionInUse = sessionInUse
        self.current = current
    }
}

public struct Overview: Codable, Sendable, Hashable {
    public var generatedAt: Date
    @LossyList public var machines: [Machine]
    @LossyList public var agents: [AgentStatus]

    public init(generatedAt: Date = Date(), machines: [Machine] = [], agents: [AgentStatus] = []) {
        self.generatedAt = generatedAt
        self.machines = machines
        self.agents = agents
    }
}

public struct ProjectLink: Codable, Sendable, Hashable {
    public var title: String?
    public var url: String

    public init(title: String? = nil, url: String) {
        self.title = title
        self.url = url
    }
}

public struct ProjectMember: Codable, Sendable, Hashable {
    public var name: String
    /// member | coordinator
    public var role: String
    public var note: String?
    public var addedAt: Date

    public init(name: String, role: String = "member", note: String? = nil, addedAt: Date = Date()) {
        self.name = name
        self.role = role
        self.note = note
        self.addedAt = addedAt
    }
}

public struct ProjectDoc: Codable, Sendable, Hashable {
    public var name: String
    public var size: Int
    public var updatedAt: Date

    public init(name: String, size: Int, updatedAt: Date = Date()) {
        self.name = name
        self.size = size
        self.updatedAt = updatedAt
    }
}

public struct Project: Codable, Sendable, Hashable, Identifiable {
    public var name: String
    public var title: String
    public var goal: String?
    public var brief: String?
    public var archived: Bool?
    public var createdAt: Date
    public var updatedAt: Date
    @LossyList public var links: [ProjectLink]
    @LossyList public var members: [ProjectMember]
    @LossyList public var docs: [ProjectDoc]

    public var id: String { name }

    public init(
        name: String, title: String, goal: String? = nil, brief: String? = nil, createdAt: Date = Date(),
        links: [ProjectLink] = [], members: [ProjectMember] = [], docs: [ProjectDoc] = []
    ) {
        self.name = name
        self.title = title
        self.goal = goal
        self.brief = brief
        self.createdAt = createdAt
        self.updatedAt = createdAt
        self.links = links
        self.members = members
        self.docs = docs
    }
}

/// One row of the recent list: a thread and where its newest task stands.
public struct RecentItem: Codable, Sendable, Hashable, Identifiable {
    public var kind: String
    public var threadId: String
    public var updatedAt: Date
    public var last: Message
    public var task: Message?
    public var answer: Message?
    /// The newest task not finished yet (hubs from v0.18.40).
    public var open: Message?
    /// queued | working | done | failed | "" (no task)
    public var state: String?
    public var project: String?
    @LossyList public var participants: [String]

    public var id: String { threadId }

    public init(
        threadId: String, participants: [String], updatedAt: Date, last: Message, task: Message? = nil,
        answer: Message? = nil, open: Message? = nil, state: String? = nil, project: String? = nil
    ) {
        self.kind = "thread"
        self.threadId = threadId
        self.participants = participants
        self.updatedAt = updatedAt
        self.last = last
        self.task = task
        self.answer = answer
        self.open = open
        self.state = state
        self.project = project
    }
}

/// One step of the run that handled a task.
public struct TraceStep: Codable, Sendable, Hashable {
    public var text: String?
    public var tool: String?
    public var input: String?
    public var output: String?
    public var failed: Bool?

    public init(text: String? = nil, tool: String? = nil, input: String? = nil, output: String? = nil, failed: Bool? = nil) {
        self.text = text
        self.tool = tool
        self.input = input
        self.output = output
        self.failed = failed
    }
}

public struct TraceEvent: Codable, Sendable, Hashable, Identifiable {
    public var seq: Int
    /// text | thinking | tool | tool_result | usage | error | output
    public var kind: String
    public var data: TraceStep
    public var at: Date?

    public var id: Int { seq }

    public init(seq: Int, kind: String, data: TraceStep, at: Date? = nil) {
        self.seq = seq
        self.kind = kind
        self.data = data
        self.at = at
    }
}

public struct Trace: Codable, Sendable, Hashable {
    public var message: String
    public var status: String
    @LossyList public var events: [TraceEvent]

    public init(message: String, status: String, events: [TraceEvent] = []) {
        self.message = message
        self.status = status
        self.events = events
    }
}

public struct Member: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var displayName: String?
    /// owner | admin | member
    public var role: String

    public init(id: String, name: String, displayName: String? = nil, role: String = "owner") {
        self.id = id
        self.name = name
        self.displayName = displayName
        self.role = role
    }
}

/// The person behind this device's token.
public struct Whoami: Codable, Sendable, Hashable {
    public var member: Member?
    public var canManage: Bool

    public init(member: Member?, canManage: Bool = true) {
        self.member = member
        self.canManage = canManage
    }

    /// How messages sent from this app are signed.
    public var consoleSender: String {
        "@console:" + (member?.name ?? "owner")
    }
}
