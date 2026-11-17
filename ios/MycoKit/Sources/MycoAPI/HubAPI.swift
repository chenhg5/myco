import Foundation

/// What the app sends: `POST /v1/console/messages`.
public struct SendRequest: Codable, Sendable, Hashable {
    public var to: String
    public var body: String
    /// The thread it continues; nil starts a new one.
    public var threadId: String?
    /// Puts a new thread in this project.
    public var project: String?
    /// A task from this sender to the same recipient: delivery waits until
    /// it closes (hubs from v0.19.2).
    public var after: String?
    /// Files uploaded from this device (`upload`), by id: attaching one
    /// lets the recipient read it. Left out when there are none.
    public var attachments: [String]?

    public init(
        to: String, body: String, threadId: String? = nil, project: String? = nil, after: String? = nil, attachments: [String] = []
    ) {
        self.to = to
        self.body = body
        self.threadId = threadId
        self.project = project
        self.after = after
        self.attachments = attachments.isEmpty ? nil : attachments
    }
}

/// A file kept by the workspace: what an upload answers with.
public struct HubFile: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var size: Int64
    public var contentType: String?

    public init(id: String, name: String, size: Int64, contentType: String? = nil) {
        self.id = id
        self.name = name
        self.size = size
        self.contentType = contentType
    }
}

/// A person's voice chat settings as the hub keeps them (`api.VoiceSettings`):
/// the model the voice thinks with, the speech service it speaks with, and
/// their keys. `updatedAt` is when the person changed them, on whichever
/// device: the newest wins.
public struct VoiceDocument: Codable, Sendable, Hashable {
    public var llmBaseUrl: String?
    public var llmModel: String?
    public var llmKey: String?
    /// The speech service ("doubao", ...). Its ids have no underscore:
    /// they are keys of `ttsKeys`, and JSON keys are rewritten on the way.
    public var ttsProvider: String?
    public var ttsSpeaker: String?
    /// How fast it talks, -50...100.
    public var ttsRate: Int?
    /// The speech services' keys, by service.
    public var ttsKeys: [String: String]?
    public var updatedAt: Date

    public init(
        llmBaseUrl: String? = nil, llmModel: String? = nil, llmKey: String? = nil, ttsProvider: String? = nil,
        ttsSpeaker: String? = nil, ttsRate: Int? = nil, ttsKeys: [String: String]? = nil, updatedAt: Date
    ) {
        self.llmBaseUrl = llmBaseUrl
        self.llmModel = llmModel
        self.llmKey = llmKey
        self.ttsProvider = ttsProvider
        self.ttsSpeaker = ttsSpeaker
        self.ttsRate = ttsRate
        self.ttsKeys = ttsKeys
        self.updatedAt = updatedAt
    }

    /// Nothing was ever set (the hub answers the year 1).
    public var neverSet: Bool { updatedAt.timeIntervalSince1970 < 0 }
}

/// Which recent list to ask the hub for.
public enum RecentScope: Sendable, Hashable {
    /// What I talk to, one row per party (`group=pair&side=me`).
    case myPairs
    /// One project's threads.
    case project(String)
    /// Every thread this device may read, newest first.
    case everything
}

/// One page of a conversation, newest first; `more`: older ones exist.
public struct MessagePage: Sendable, Hashable {
    public var messages: [Message]
    public var more: Bool

    public init(messages: [Message], more: Bool) {
        self.messages = messages
        self.more = more
    }
}

/// A link that lets machines and agents join the workspace.
public struct JoinLink: Codable, Sendable, Hashable {
    public var joinUrl: String
    /// The hub's join script ends with the agent's last step, so one
    /// command is all an agent needs (hubs before this leave it out).
    public var agentGuide: Bool?

    public init(joinUrl: String, agentGuide: Bool? = true) {
        self.joinUrl = joinUrl
        self.agentGuide = agentGuide
    }
}

/// One workspace (a hub), as this device sees it. Every method is one of
/// the hub's `/v1/console/*` calls; nothing here caches, decrypts or knows
/// about more than one workspace.
public protocol HubAPI: Sendable {
    /// `GET /v1/console/me`
    func whoami() async throws -> Whoami
    /// `GET /v1/overview`
    func overview() async throws -> Overview
    /// `GET /v1/projects`
    func projects() async throws -> [Project]
    /// `GET /v1/console/recent`: newest first.
    func recent(_ scope: RecentScope) async throws -> [RecentItem]
    /// `GET /v1/console/exchanges`: the messages between two parties in
    /// every thread, newest first, a page at a time. `before`: only those
    /// older than this (to page back); nil is the newest page.
    func exchanges(between a: String, and b: String, before: Date?) async throws -> MessagePage
    /// `GET /v1/console/messages/{id}/trace`
    func trace(of messageID: String, after seq: Int) async throws -> Trace
    /// `POST /v1/console/messages`
    func send(_ request: SendRequest) async throws -> Message
    /// `POST /v1/console/messages/{id}/steer`: adds to a task being worked
    /// on, with files if any (their ids).
    func steer(_ messageID: String, body: String, attachments: [String]) async throws
    /// `POST /v1/console/files?name=`: keeps a file in the workspace as
    /// mine, to attach to what I send next. A workspace too old for this
    /// answers `not_found`.
    func upload(_ data: Data, name: String, contentType: String) async throws -> HubFile
    /// `GET /v1/console/files/{id}`: a file attached to a message this
    /// device may read.
    func file(_ id: String) async throws -> Data
    /// `POST /v1/console/messages/{id}/stop`
    func stop(_ messageID: String) async throws
    /// `GET /v1/console/hidden`: the messages this person took out of
    /// their own view, on any of their devices. Nothing is deleted. A
    /// workspace from before v0.19.9 answers `not_found`.
    func hidden() async throws -> [String]
    /// `POST /v1/console/hidden`: takes messages out of this person's
    /// view, or puts them back; answers the list as it is then.
    func setHidden(hide: [String], show: [String]) async throws -> [String]
    /// `GET /v1/console/voice`: what this person set for voice chats, kept
    /// on the hub for all their devices, keys included. A workspace from
    /// before this answers `not_found`.
    func voiceSettings() async throws -> VoiceDocument
    /// `PUT /v1/console/voice`: keeps them, unless the hub holds newer
    /// ones; answers what it holds then.
    func putVoiceSettings(_ document: VoiceDocument) async throws -> VoiceDocument
    /// `PUT /v1/agents/{name}/run-settings`: the model and effort its
    /// runner starts it with from its next run ("" follows its computer's
    /// setting). For whoever owns the agent or manages the workspace. A
    /// workspace from before v0.19.5 answers `not_found`.
    func setRunSettings(of agent: String, model: String, effort: String) async throws -> Agent
    /// `POST /v1/enrollments` with `kind: "link"` and `harness: "exec"`:
    /// a link for one computer (one use, a day). Its command, run in a
    /// folder there, makes that folder a computer the agents in
    /// `allowFrom` may run commands in. For the workspace's owner and
    /// admins only.
    func computerLink(allowFrom: [String]) async throws -> JoinLink
    /// `PUT /v1/projects/{name}/members/{agent}`: puts a node in a project.
    func addProjectMember(_ agent: String, to project: String) async throws
    /// `GET /v1/console/automations`: mine, or everyone's for whoever
    /// manages the workspace. A workspace from before v0.19.22 answers
    /// `not_found`.
    func automations() async throws -> [Automation]
    /// `GET /v1/console/automations/{id}`: one, with its last runs.
    func automation(_ id: String) async throws -> AutomationDetail
    /// `POST /v1/console/automations`: `name`, `to`, `body`, and a
    /// schedule or a webhook. It sends from my console address.
    func createAutomation(_ request: AutomationRequest) async throws -> AutomationSaved
    /// `PATCH /v1/console/automations/{id}`: what is left out stays.
    func updateAutomation(_ id: String, _ request: AutomationRequest) async throws -> AutomationSaved
    /// `DELETE /v1/console/automations/{id}`: its runs go too; what it
    /// sent stays.
    func deleteAutomation(_ id: String) async throws
    /// `POST /v1/console/automations/{id}/run`: sends it once now, also
    /// when it is turned off.
    func runAutomation(_ id: String) async throws -> AutomationRun
    /// `POST /v1/enrollments` with `kind: "link"`: a link to hand to an
    /// agent or a machine (ten uses, a week), as the desktop app makes.
    func joinLink() async throws -> JoinLink
}

extension HubAPI {
    /// Adds words alone to a task being worked on.
    public func steer(_ messageID: String, body: String) async throws {
        try await steer(messageID, body: body, attachments: [])
    }
}

extension HubFile {
    /// The largest file a workspace takes by default (its `--max-file-size`).
    public static let maxSize = 50 * 1024 * 1024
}
