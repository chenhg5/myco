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
/// An agent's goal in brief, on lists (`api.GoalBrief`).
public struct GoalBrief: Codable, Sendable, Hashable {
    public var id: String
    public var objective: String
    public var round: Int
    public var maxRounds: Int
    public var nextWakeAt: Date?
    public var state: String

    public init(id: String, objective: String, round: Int, maxRounds: Int, nextWakeAt: Date? = nil, state: String) {
        self.id = id
        self.objective = objective
        self.round = round
        self.maxRounds = maxRounds
        self.nextWakeAt = nextWakeAt
        self.state = state
    }
}

public struct Runtime: Codable, Sendable, Hashable {
    /// What it may touch when it runs, in use: readonly, workspace, full.
    public var access: String? = nil
    /// The most its computer allows (runners from v0.19.8).
    public var maxAccess: String? = nil
    public var cli: String?
    public var cliVersion: String?
    public var model: String?
    /// Where the model comes from: app (its run settings), self (it said
    /// so itself), configured, observed, default, cli-config, provider.
    public var modelSource: String?
    /// The reasoning effort its CLI runs with (hubs from v0.19.5).
    public var effort: String?
    public var effortSource: String?
    /// Why its runner went offline, sent as it stopped: "stopped" (by
    /// hand, its service, logging out or shutting down). None while it runs.
    public var away: String?
    /// Where it runs, when that explains going offline: "wsl".
    public var env: String?
    /// The models its CLI offers on that computer, in the CLI's order
    /// (Codex; runners from v0.21.1). Absent: use the known names.
    public var models: [RuntimeModel]?
    /// The account's usage limits, as last reported (only its owner sees
    /// them).
    public var quota: Quota? = nil

    public init(
        cli: String? = nil, cliVersion: String? = nil, model: String? = nil, modelSource: String? = nil,
        effort: String? = nil, effortSource: String? = nil, away: String? = nil, env: String? = nil,
        models: [RuntimeModel]? = nil
    ) {
        self.cli = cli
        self.cliVersion = cliVersion
        self.model = model
        self.modelSource = modelSource
        self.effort = effort
        self.effortSource = effortSource
        self.away = away
        self.env = env
        self.models = models
    }
}

/// A model a CLI offers: the id its --model takes, a display name, the
/// reasoning efforts it supports.
public struct RuntimeModel: Codable, Sendable, Hashable {
    public var id: String
    public var name: String?
    public var efforts: [String]?

    public init(id: String, name: String? = nil, efforts: [String]? = nil) {
        self.id = id
        self.name = name
        self.efforts = efforts
    }
}

/// The model and effort chosen for an agent's runner; "" or absent follows
/// its computer's own setting (`api.RunSettings`, hubs from v0.19.5).
public struct RunSettings: Codable, Sendable, Hashable {
    public var model: String?
    public var effort: String?
    /// readonly, workspace or full ("" or nil: its computer's).
    public var access: String?

    public init(model: String? = nil, effort: String? = nil, access: String? = nil) {
        self.model = model
        self.effort = effort
        self.access = access
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
    /// The member who added it, by id.
    public var ownerId: String?
    public var machineId: String?
    public var runtime: Runtime?
    /// What was chosen for its runner; absent when nothing was.
    public var runSettings: RunSettings?
    @LossyList public var labels: [String]

    public init(
        id: String, name: String, displayName: String? = nil, avatar: String? = nil, statusText: String? = nil,
        description: String = "", binding: Binding = Binding(), online: Bool = true, createdAt: Date = Date(),
        kind: String? = "agent", owner: String? = nil, machineId: String? = nil, runtime: Runtime? = nil, labels: [String] = [],
        ownerId: String? = nil, runSettings: RunSettings? = nil
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
        self.ownerId = ownerId
        self.runSettings = runSettings
    }
}

public struct Machine: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var hostname: String?
    public var os: String?
    public var online: Bool
    /// It can start sessions when asked (its machine node is there).
    public var canSpawn: Bool?
    /// The myco it runs ("v0.19.24").
    public var mycoVersion: String?

    public init(
        id: String, name: String, hostname: String? = nil, os: String? = nil, online: Bool = true, canSpawn: Bool? = nil,
        mycoVersion: String? = nil
    ) {
        self.id = id
        self.name = name
        self.hostname = hostname
        self.os = os
        self.online = online
        self.canSpawn = canSpawn
        self.mycoVersion = mycoVersion
    }
}

public struct Attachment: Codable, Sendable, Hashable, Identifiable {
    public var id: String { fileId }
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

    /// A picture the app can show: by its type, else by its name (the same
    /// rule as the desktop app's).
    public var isPicture: Bool {
        if let type = contentType?.split(separator: ";").first.map({ $0.trimmingCharacters(in: .whitespaces).lowercased() }), !type.isEmpty {
            return Self.pictureTypes.contains(type)
        }
        return Self.pictureExtensions.contains((name as NSString).pathExtension.lowercased())
    }

    static let pictureTypes: Set<String> = ["image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "image/bmp", "image/heic"]
    static let pictureExtensions: Set<String> = ["png", "jpg", "jpeg", "gif", "webp", "avif", "bmp", "heic"]
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
    /// On a task: the id of its question to its sender that still waits
    /// for an answer (the agent set the task aside to ask).
    public var asking: String?
    /// On an update: it asks the task's sender to decide.
    public var ask: Bool?
    /// On a task: what its recipient thinks I may want to say next, one
    /// short line (hubs from v0.19.15); the input offers it.
    public var suggest: String?
    /// The automation that sent it for its owner, when one did.
    public var automation: String?
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
    /// Background waits it runs (`myco watch`, hubs from v0.22).
    public var watching: Int? = nil
    /// The goal it works on, in brief (hubs from v0.22).
    public var goal: GoalBrief? = nil
    /// `session` (only while its own session waits) or `runner`.
    public var receiving: String?
    public var sessionInUse: Bool?
    @LossyList public var current: [Message]
    /// Held tasks waiting on a question to their sender (newer hubs).
    public var asking: Int?
    /// Tasks it took that show no life (left open): still open, not making
    /// it "working" (newer hubs).
    public var stale: Int?
    public var staleTasks: [String]?

    public var id: String { agent.id }

    public init(
        agent: Agent, state: String, queued: Int = 0, inProgress: Int = 0, held: Int = 0,
        receiving: String? = "runner", sessionInUse: Bool? = nil, current: [Message] = [],
        asking: Int? = nil, stale: Int? = nil, staleTasks: [String]? = nil
    ) {
        self.asking = asking
        self.stale = stale
        self.staleTasks = staleTasks
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
    /// Its group in this project (hub v0.21.34+); nil: ungrouped.
    public var group: String?

    public init(name: String, role: String = "member", note: String? = nil, addedAt: Date = Date(), group: String? = nil) {
        self.name = name
        self.role = role
        self.note = note
        self.addedAt = addedAt
        self.group = group
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
    /// I may change who is in it and what it may use (its owner, an admin).
    public var canManage: Bool? = nil
    /// The groups its members are sorted into, in order (hub v0.21.34+;
    /// nil from an older hub).
    public var groups: [String]? = nil

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
    /// The message the thread began with.
    public var first: Message?
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
    /// Pairs a tool call with its result.
    public var id: String?
    public var text: String?
    public var tool: String?
    public var input: String?
    public var output: String?
    public var failed: Bool?
    /// A `usage` step: what the run took.
    public var durationMs: Double?
    public var inputTokens: Int?
    public var outputTokens: Int?
    public var costUsd: Double?

    public init(id: String? = nil, text: String? = nil, tool: String? = nil, input: String? = nil, output: String? = nil, failed: Bool? = nil) {
        self.id = id
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
    /// This app on this device.
    public var device: Device?
    public var canManage: Bool

    public init(member: Member?, device: Device? = nil, canManage: Bool = true) {
        self.member = member
        self.device = device
        self.canManage = canManage
    }

    /// How messages sent from this app are signed.
    public var consoleSender: String {
        "@console:" + (member?.name ?? "owner")
    }
}

/// When an automation sends: a cron of five fields and its time zone
/// (an IANA name; empty is UTC).
public struct AutomationSchedule: Codable, Sendable, Hashable {
    public var cron: String
    public var tz: String?

    public init(cron: String, tz: String? = nil) {
        self.cron = cron
        self.tz = tz
    }
}

/// One time an automation ran.
public struct AutomationRun: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var automationId: String?
    public var at: Date
    /// schedule | webhook | manual
    public var trigger: String
    /// sent | skipped | failed
    public var outcome: String
    public var messageId: String?
    /// Why it was skipped or failed (short, English).
    public var error: String?

    public init(id: String, automationId: String? = nil, at: Date, trigger: String, outcome: String, messageId: String? = nil, error: String? = nil) {
        self.id = id
        self.automationId = automationId
        self.at = at
        self.trigger = trigger
        self.outcome = outcome
        self.messageId = messageId
        self.error = error
    }
}

/// A task sent for its owner to one agent, on a schedule or when an
/// outside system calls its webhook. It runs on the hub (from v0.19.22).
public struct Automation: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var owner: String?
    public var to: String
    public var project: String?
    /// The message; `{{payload}}` is replaced by what a webhook call sends.
    public var body: String
    public var schedule: AutomationSchedule?
    public var webhook: Bool
    /// Where an outside system posts (the secret is not in it).
    public var webhookUrl: String?
    public var skipIfBusy: Bool
    public var enabled: Bool
    public var nextRunAt: Date?
    public var lastRun: AutomationRun?

    public init(
        id: String, name: String, owner: String? = nil, to: String, project: String? = nil, body: String,
        schedule: AutomationSchedule? = nil, webhook: Bool = false, webhookUrl: String? = nil, skipIfBusy: Bool = true,
        enabled: Bool = true, nextRunAt: Date? = nil, lastRun: AutomationRun? = nil
    ) {
        self.id = id
        self.name = name
        self.owner = owner
        self.to = to
        self.project = project
        self.body = body
        self.schedule = schedule
        self.webhook = webhook
        self.webhookUrl = webhookUrl
        self.skipIfBusy = skipIfBusy
        self.enabled = enabled
        self.nextRunAt = nextRunAt
        self.lastRun = lastRun
    }
}

/// What creates or changes an automation: a field left out stays as it is.
public struct AutomationRequest: Codable, Sendable, Hashable {
    public var name: String?
    public var to: String?
    public var body: String?
    public var schedule: AutomationSchedule?
    /// Takes its schedule away (a change only).
    public var clearSchedule: Bool?
    /// Turns its webhook on (a new secret) or off.
    public var webhook: Bool?
    /// A new secret for its webhook; the old one stops working.
    public var rotateSecret: Bool?
    public var skipIfBusy: Bool?
    public var enabled: Bool?

    public init(
        name: String? = nil, to: String? = nil, body: String? = nil, schedule: AutomationSchedule? = nil, clearSchedule: Bool? = nil,
        webhook: Bool? = nil, rotateSecret: Bool? = nil, skipIfBusy: Bool? = nil, enabled: Bool? = nil
    ) {
        self.name = name
        self.to = to
        self.body = body
        self.schedule = schedule
        self.clearSchedule = clearSchedule
        self.webhook = webhook
        self.rotateSecret = rotateSecret
        self.skipIfBusy = skipIfBusy
        self.enabled = enabled
    }
}

/// What creating or changing one answers. `webhookSecret` is there only
/// when a secret was just made: it is shown once.
public struct AutomationSaved: Codable, Sendable, Hashable {
    public var automation: Automation
    public var webhookSecret: String?

    public init(automation: Automation, webhookSecret: String? = nil) {
        self.automation = automation
        self.webhookSecret = webhookSecret
    }
}

/// An automation with its last runs, newest first.
public struct AutomationDetail: Codable, Sendable, Hashable {
    public var automation: Automation
    @LossyList public var runs: [AutomationRun]

    public init(automation: Automation, runs: [AutomationRun] = []) {
        self.automation = automation
        self.runs = runs
    }
}

/// One publish of a page.
public struct PageVersion: Codable, Sendable, Hashable {
    public var n: Int
    public var by: String?
    public var at: Date?

    public init(n: Int, by: String? = nil, at: Date? = nil) {
        self.n = n
        self.by = by
        self.at = at
    }
}

/// A web page an agent published to the workspace (`myco page publish`,
/// hubs from v0.19.24): a folder of files kept by the hub, in versions.
public struct Page: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var project: String?
    public var publisher: String
    /// Its latest version's number.
    public var version: Int
    /// Its address on the hub (`<hub>/pages/<id>`).
    public var url: String?
    @LossyList public var versions: [PageVersion]
    public var createdAt: Date?
    public var updatedAt: Date?
    /// A server on its publisher's computer, forwarded while that one
    /// serves it: no versions, no files; `online` says whether it is now.
    public var live: Bool?
    public var online: Bool?
    /// The app it was made from, if any.
    public var template: PageTemplate?

    public init(
        id: String, name: String, project: String? = nil, publisher: String, version: Int = 1, url: String? = nil,
        versions: [PageVersion] = [], createdAt: Date? = nil, updatedAt: Date? = nil, live: Bool? = nil, online: Bool? = nil,
        template: PageTemplate? = nil
    ) {
        self.template = template
        self.id = id
        self.name = name
        self.project = project
        self.publisher = publisher
        self.version = version
        self.url = url
        self.versions = versions
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.live = live
        self.online = online
    }
}

/// One file of a page, as the hub serves it.
public struct PageFile: Sendable, Hashable {
    public var data: Data
    /// Its type, from its extension ("text/html; charset=utf-8").
    public var contentType: String
    /// The HTTP status it came with: a live page's server answers as it does.
    public var status: Int

    public init(data: Data, contentType: String, status: Int = 200) {
        self.data = data
        self.contentType = contentType
        self.status = status
    }
}

/// One session of a handover: where it ran and what it ran with.
public struct HandoverSide: Codable, Sendable, Hashable {
    public var binding: Binding?
    public var machineId: String?
    public var machineName: String?
    public var runtime: Runtime?

    public init(binding: Binding? = nil, machineId: String? = nil, machineName: String? = nil, runtime: Runtime? = nil) {
        self.binding = binding
        self.machineId = machineId
        self.machineName = machineName
        self.runtime = runtime
    }
}

/// One change of the session behind an agent (「换一个来接着做」, hubs
/// after v0.19.22): its name, conversations and projects stay; the session
/// that answers for it changes.
public struct Handover: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var agent: String
    public var at: Date
    /// Who started it: a member's name, "admin", or an agent.
    public var by: String?
    public var note: String?
    public var from: HandoverSide?
    /// Empty while `state` is waiting.
    public var to: HandoverSide?
    /// waiting (a command made, nobody ran it yet) | done | expired.
    public var state: String
    public var doneAt: Date?
    /// The tasks that went back to the queue when it was taken.
    @LossyList public var openTasks: [String]
    /// The handover this one undid (a switch back), if any.
    public var back: String?

    public init(
        id: String, agent: String, at: Date, by: String? = nil, note: String? = nil, from: HandoverSide? = nil, to: HandoverSide? = nil,
        state: String, doneAt: Date? = nil, openTasks: [String] = [], back: String? = nil
    ) {
        self.id = id
        self.agent = agent
        self.at = at
        self.by = by
        self.note = note
        self.from = from
        self.to = to
        self.state = state
        self.doneAt = doneAt
        self.openTasks = openTasks
        self.back = back
    }
}

/// A one-time take-over command (a day). The token is only inside the
/// commands, shown once: it is as good as the agent's own credential.
public struct HandoverLink: Codable, Sendable, Hashable {
    public var handover: Handover
    public var command: String?
    public var commandPs1: String?

    public init(handover: Handover, command: String? = nil, commandPs1: String? = nil) {
        self.handover = handover
        self.command = command
        self.commandPs1 = commandPs1
    }
}

/// A new session on a computer takes an agent over. `back`: the session
/// that handover replaced takes it back instead (the rest comes from it).
public struct HandoverRequest: Codable, Sendable, Hashable {
    /// A computer's id or name.
    public var machine: String
    public var cli: String?
    public var dir: String?
    public var note: String?
    public var back: String?

    public init(machine: String, cli: String? = nil, dir: String? = nil, note: String? = nil, back: String? = nil) {
        self.machine = machine
        self.cli = cli
        self.dir = dir
        self.note = note
        self.back = back
    }
}

/// What a handover on a computer answers: it is done, and what the new
/// session said after reading the handover note.
public struct HandoverResult: Codable, Sendable, Hashable {
    public struct Spawn: Codable, Sendable, Hashable {
        public var reply: String?

        public init(reply: String? = nil) {
            self.reply = reply
        }
    }

    public var handover: Handover
    public var spawn: Spawn?

    public init(handover: Handover, spawn: Spawn? = nil) {
        self.handover = handover
        self.spawn = spawn
    }
}

/// Something a conversation keeps at hand above its messages (hubs from
/// v0.20.3): a web address, a file, or one of its messages. Both sides of
/// the conversation see the same list.
public struct Pin: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    /// link | file | message
    public var kind: String
    public var title: String
    /// The address (link) or the message's id (message).
    public var target: String?
    public var file: Attachment?
    /// Who pinned it: an agent's name or a console sender.
    public var addedBy: String?
    public var at: Date?

    public init(id: String, kind: String, title: String, target: String? = nil, file: Attachment? = nil, addedBy: String? = nil, at: Date? = nil) {
        self.id = id
        self.kind = kind
        self.title = title
        self.target = target
        self.file = file
        self.addedBy = addedBy
        self.at = at
    }
}

/// What is pinned: a link, a file, or a message, with a title of its own
/// if given (else the address, the file's name or the message's id).
public struct PinRequest: Codable, Sendable, Hashable {
    public var with: String
    public var kind: String
    public var title: String?
    public var target: String?
    public var file: Attachment?

    public init(with: String, kind: String, title: String? = nil, target: String? = nil, file: Attachment? = nil) {
        self.with = with
        self.kind = kind
        self.title = title
        self.target = target
        self.file = file
    }
}

/// An app (a template): a page made from it is a tool people and agents
/// use together, its data kept as records (hubs from v0.20.5).
public struct AppTemplate: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    /// "author/name", e.g. "myco/whiteboard".
    public var slug: String
    public var name: String
    public var description: String?
    /// table, records, doc, slides, or anything else.
    public var kind: String?
    /// office, dev, tools, life, creative, game, other (hubs from v0.20.6).
    public var category: String?
    /// Its icon, SVG markup (hubs from v0.20.7): its shape only.
    public var icon: String?
    public var version: String
    public var publisher: String?
    /// One of myco's own, in every workspace.
    public var official: Bool?

    public init(
        id: String, slug: String, name: String, description: String? = nil, kind: String? = nil, category: String? = nil,
        icon: String? = nil, version: String = "1.0.0", publisher: String? = nil, official: Bool? = nil
    ) {
        self.id = id
        self.slug = slug
        self.name = name
        self.description = description
        self.kind = kind
        self.category = category
        self.icon = icon
        self.version = version
        self.publisher = publisher
        self.official = official
    }
}

/// Which app a page was made from.
public struct PageTemplate: Codable, Sendable, Hashable {
    public var id: String?
    public var slug: String?
    public var name: String?
    public var version: String?
    /// The app's newer version, when there is one (hubs from v0.20.7).
    public var latest: String?
    /// What it declares it uses: "send", "download" (and the computer's
    /// capabilities, in os).
    public var permissions: [String]?
    public var os: [String]?
    /// Hubs from v0.22.9: permissions and os are all it declares, so what
    /// they leave out is refused. Official: one of myco's own apps.
    public var declares: Bool?
    public var official: Bool?

    public init(
        id: String? = nil, slug: String? = nil, name: String? = nil, version: String? = nil, latest: String? = nil,
        permissions: [String]? = nil, os: [String]? = nil, declares: Bool? = nil, official: Bool? = nil
    ) {
        self.id = id
        self.slug = slug
        self.name = name
        self.version = version
        self.latest = latest
        self.permissions = permissions
        self.os = os
        self.declares = declares
        self.official = official
    }

    /// Whether the app may use a permission ("send", "download"): from hubs
    /// that say so, only what it declares. (Hubs do not say it for a page of
    /// myco's own apps kept from before declarations, until it is upgraded.)
    public func allows(_ permission: String) -> Bool {
        declares != true || (permissions ?? []).contains(permission)
    }
}

/// What the hub answered to a page's own call (its state, its records):
/// the JSON as it came, and with what status (a conflict carries the
/// latest in its body).
public struct PageAnswer: Sendable, Hashable {
    public var data: Data
    public var status: Int

    public init(data: Data, status: Int) {
        self.data = data
        self.status = status
    }
}

// MARK: Superagents

/// `GET /v1/console/superagent`: my superagent here, if any, and how many
/// of its requests wait for me (hubs from v0.21.1).
public struct SuperagentState: Codable, Sendable, Hashable {
    public var agent: Agent?
    public var pending: Int?

    public init(agent: Agent? = nil, pending: Int? = nil) {
        self.agent = agent
        self.pending = pending
    }
}

/// A superagent's request held for its owner: approved, it is carried out
/// once, as it came.
public struct Approval: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var agent: String
    public var what: String
    public var reason: String?
    public var method: String
    public var path: String
    /// pending, done, denied, expired, withdrew
    public var status: String
    /// The HTTP status of carrying it out.
    public var result: Int?
    public var resultBody: String?
    public var createdAt: Date
    public var expiresAt: Date

    public init(
        id: String, agent: String, what: String, reason: String? = nil, method: String = "DELETE", path: String = "",
        status: String = "pending", result: Int? = nil, resultBody: String? = nil, createdAt: Date, expiresAt: Date
    ) {
        self.id = id
        self.agent = agent
        self.what = what
        self.reason = reason
        self.method = method
        self.path = path
        self.status = status
        self.result = result
        self.resultBody = resultBody
        self.createdAt = createdAt
        self.expiresAt = expiresAt
    }
}

// MARK: Starting on a computer

/// `POST /v1/machines/{id}/spawn`: start a session on a computer, or (with
/// `cli: "upgrade"`) bring its myco up to date.
public struct SpawnRequest: Codable, Sendable, Hashable {
    public var cli: String
    public var dir: String
    public var displayName: String?
    public var access: String?
    /// For `upgrade`: the version (empty: the latest).
    public var version: String?

    public init(cli: String, dir: String = "", displayName: String? = nil, access: String? = nil, version: String? = nil) {
        self.cli = cli
        self.dir = dir
        self.displayName = displayName
        self.access = access
        self.version = version
    }
}

public struct SpawnResult: Codable, Sendable, Hashable {
    public struct Upgrade: Codable, Sendable, Hashable {
        public var from: String
        /// Empty: it runs that already.
        public var to: String?
        public var restarting: Bool?
        public var note: String?

        public init(from: String, to: String? = nil, restarting: Bool? = nil, note: String? = nil) {
            self.from = from
            self.to = to
            self.restarting = restarting
            self.note = note
        }
    }

    /// The node it joined as.
    public var agent: String?
    public var denied: Bool?
    public var error: String?
    public var upgrade: Upgrade?

    public init(agent: String? = nil, denied: Bool? = nil, error: String? = nil, upgrade: Upgrade? = nil) {
        self.agent = agent
        self.denied = denied
        self.error = error
        self.upgrade = upgrade
    }
}
