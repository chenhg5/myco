import Foundation

/// A real hub, over HTTPS, as one device: its `/v1/console/*` calls with
/// the device's token. Nothing is cached here.
public struct HubClient: HubAPI {
    public let baseURL: URL
    private let token: String
    /// A session of its own (tests); nil is the app's, as its proxy
    /// setting has it at the moment of each request.
    private let session: URLSession?

    /// How many rows or messages one call asks for.
    static let pageSize = 100

    public init(baseURL: URL, token: String, session: URLSession? = nil) {
        self.baseURL = baseURL
        self.token = token
        self.session = session
    }

    public func remoteComputers() async throws -> [RemoteComputer] {
        struct Envelope: Decodable { var machines: [RemoteComputer] }
        let result: Envelope = try await get("/v1/remote/machines", timeout: Self.pollTimeout)
        return result.machines
    }

    public func connectRemote(machine: String, offer: RemoteOffer) async throws -> RemoteConnection {
        struct Request: Encodable { var machine: String; var offer: RemoteOffer }
        let data = try await call("POST", "/v1/remote/sessions", body: HubJSON.encoder().encode(Request(machine: machine, offer: offer)))
        return try Self.decode(RemoteConnection.self, from: data)
    }

    public func remoteConnection(_ id: String) async throws -> RemoteConnection {
        try await get("/v1/remote/sessions/\(Self.segment(id))", timeout: Self.pollTimeout)
    }

    public func disconnectRemote(_ id: String) async throws {
        _ = try await call("DELETE", "/v1/remote/sessions/\(Self.segment(id))")
    }

    public func whoami() async throws -> Whoami {
        try await get("/v1/console/me")
    }

    public func overview() async throws -> Overview {
        try await get("/v1/overview", ["recent": "0"], timeout: Self.pollTimeout)
    }

    public func projects() async throws -> [Project] {
        let envelope: ProjectsEnvelope = try await get("/v1/projects", timeout: Self.pollTimeout)
        return envelope.projects
    }

    /// How long what is asked for again and again (the lists, a
    /// conversation's messages) may go without a byte before it is given
    /// up: the time between packets, not the whole request. The next
    /// asking is seconds away, so one that hangs is given up early; but
    /// through a slow proxy a look takes several seconds and can stall for
    /// more than ten, and twelve made the lists say the workspace could not
    /// be reached while it answered.
    static let pollTimeout: TimeInterval = 20

    public func recent(_ scope: RecentScope) async throws -> [RecentItem] {
        var query = ["limit": "100"]
        switch scope {
        case .myPairs:
            query["group"] = "pair"
            query["side"] = "me"
        case .project(let name):
            query["project"] = name
        case .everything:
            break
        case .search(let text):
            query["group"] = "pair"
            query["side"] = "me"
            query["q"] = text
            query["limit"] = "20"
        }
        let envelope: RecentEnvelope = try await get("/v1/console/recent", query, timeout: Self.pollTimeout)
        return envelope.items
    }

    public func exchanges(between a: String, and b: String, before: Date?) async throws -> MessagePage {
        var query = ["a": a, "b": b, "limit": String(Self.pageSize)]
        // The hub pages by time in milliseconds.
        if let before { query["before"] = String(Int64((before.timeIntervalSince1970 * 1000).rounded(.down))) }
        let envelope: ExchangeEnvelope = try await get("/v1/console/exchanges", query, timeout: Self.pollTimeout)
        return MessagePage(messages: envelope.messages, more: envelope.more ?? false)
    }

    public func trace(of messageID: String, after seq: Int) async throws -> Trace {
        try await get("/v1/console/messages/\(messageID)/trace", ["after": String(seq)])
    }

    public func send(_ request: SendRequest) async throws -> Message {
        let data = try await call("POST", "/v1/console/messages", body: HubJSON.encoder().encode(request))
        return try Self.decode(SendEnvelope.self, from: data).message
    }

    public func steer(_ messageID: String, body: String, attachments: [String]) async throws {
        struct Request: Encodable {
            var body: String
            var attachments: [String]?
        }
        let request = Request(body: body, attachments: attachments.isEmpty ? nil : attachments)
        _ = try await call("POST", "/v1/console/messages/\(messageID)/steer", body: HubJSON.encoder().encode(request))
    }

    public func upload(_ data: Data, name: String, contentType: String) async throws -> HubFile {
        // A file takes as long as the connection needs; the hub refuses
        // one that is too large by itself.
        let answer = try await call(
            "POST", "/v1/console/files", query: ["name": name], body: data,
            contentType: contentType.isEmpty ? "application/octet-stream" : contentType, timeout: Self.transferTimeout)
        return try Self.decode(HubFile.self, from: answer)
    }

    public func file(_ id: String) async throws -> Data {
        let safe = id.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-_."))) ?? id
        return try await call("GET", "/v1/console/files/\(safe)", timeout: Self.transferTimeout)
    }

    /// How long a file may take to go up or come down.
    static let transferTimeout: TimeInterval = 300

    public func stop(_ messageID: String) async throws {
        _ = try await call("POST", "/v1/console/messages/\(messageID)/stop")
    }

    public func dismiss(_ messageID: String) async throws {
        _ = try await call("POST", "/v1/console/messages/\(messageID)/dismiss")
    }

    public func workflows() async throws -> [Workflow] {
        struct Envelope: Decodable { @LossyList var workflows: [Workflow] }
        let envelope: Envelope = try await get("/v1/workflows", timeout: Self.pollTimeout)
        return envelope.workflows
    }

    public func workflow(_ ref: String) async throws -> Workflow {
        try await get("/v1/workflows/\(Self.segment(ref))")
    }

    public func setWorkflow(_ ref: String, enabled: Bool) async throws {
        struct Body: Encodable { var enabled: Bool }
        _ = try await call("PATCH", "/v1/workflows/\(Self.segment(ref))", body: HubJSON.encoder().encode(Body(enabled: enabled)))
    }

    public func startWorkflow(_ ref: String, inputs: [String: JSONValue]) async throws -> WorkflowRun {
        struct Body: Encodable { var inputs: [String: JSONValue] }
        let data = try await call("POST", "/v1/workflows/\(Self.segment(ref))/runs", body: HubJSON.encoder().encode(Body(inputs: inputs)), timeout: 60)
        return try Self.decode(WorkflowRun.self, from: data)
    }

    public func workflowRuns(workflow: String?, limit: Int) async throws -> [WorkflowRun] {
        struct Envelope: Decodable { @LossyList var runs: [WorkflowRun] }
        var query = ["limit": String(limit)]
        if let workflow { query["workflow"] = workflow }
        let envelope: Envelope = try await get("/v1/workflow-runs", query, timeout: Self.pollTimeout)
        return envelope.runs
    }

    public func workflowRun(_ id: String) async throws -> WorkflowRun {
        try await get("/v1/workflow-runs/\(Self.segment(id))", timeout: Self.pollTimeout)
    }

    public func cancelRun(_ id: String) async throws -> WorkflowRun {
        try Self.decode(WorkflowRun.self, from: try await call("POST", "/v1/workflow-runs/\(Self.segment(id))/cancel", timeout: 60))
    }

    public func retryRun(_ id: String) async throws -> WorkflowRun {
        try Self.decode(WorkflowRun.self, from: try await call("POST", "/v1/workflow-runs/\(Self.segment(id))/retry", body: Data("{}".utf8), timeout: 60))
    }

    public func decide(run id: String, step: String, choice: String, note: String?) async throws -> WorkflowRun {
        struct Body: Encodable { var step: String; var choice: String; var note: String? }
        let data = try await call(
            "POST", "/v1/workflow-runs/\(Self.segment(id))/decide", body: HubJSON.encoder().encode(Body(step: step, choice: choice, note: note)), timeout: 60)
        return try Self.decode(WorkflowRun.self, from: data)
    }

    public func workflowDecisions() async throws -> [WorkflowDecision] {
        struct Envelope: Decodable { @LossyList var decisions: [WorkflowDecision] }
        let envelope: Envelope = try await get("/v1/workflow-decisions", timeout: Self.pollTimeout)
        return envelope.decisions
    }

    public func projectMachines(_ project: String) async throws -> [ProjectMachine] {
        struct Envelope: Decodable { @LossyList var machines: [ProjectMachine] }
        let envelope: Envelope = try await get("/v1/projects/\(Self.segment(project))/machines", timeout: Self.pollTimeout)
        return envelope.machines
    }

    public func setProjectMachine(_ project: String, machine: String, allowed: Bool) async throws {
        _ = try await call(allowed ? "PUT" : "DELETE", "/v1/projects/\(Self.segment(project))/machines/\(Self.segment(machine))")
    }

    public func superagent() async throws -> SuperagentState {
        try await get("/v1/console/superagent", timeout: Self.pollTimeout)
    }

    public func setSuperagent(_ agent: String?) async throws {
        struct Body: Encodable { var agent: String }
        if let agent {
            _ = try await call("PUT", "/v1/console/superagent", body: HubJSON.encoder().encode(Body(agent: agent)))
        } else {
            _ = try await call("DELETE", "/v1/console/superagent")
        }
    }

    public func approvals() async throws -> [Approval] {
        struct Envelope: Decodable { @LossyList var approvals: [Approval] }
        let envelope: Envelope = try await get("/v1/console/approvals", ["status": "pending"], timeout: Self.pollTimeout)
        return envelope.approvals
    }

    public func notices(after: Int64?, limit: Int) async throws -> HubNoticePage {
        var query = ["limit": String(limit)]
        if let after { query["after"] = String(after) }
        return try await get("/v1/console/notices", query, timeout: Self.pollTimeout)
    }

    public func readNotices(target: String?) async throws {
        struct Body: Encodable { var target: String? }
        _ = try await call("POST", "/v1/console/notices/read", body: HubJSON.encoder().encode(Body(target: target)))
    }

    public func usage(from: String, to: String, project: String?) async throws -> UsageReport {
        var query = ["from": from, "to": to]
        if let project { query["project"] = project }
        return try await get("/v1/console/usage", query, timeout: Self.pollTimeout)
    }

    public func decide(approval id: String, approve: Bool) async throws -> Approval {
        let data = try await call("POST", "/v1/console/approvals/\(Self.segment(id))/\(approve ? "approve" : "deny")", timeout: 60)
        return try Self.decode(Approval.self, from: data)
    }

    public func spawn(on machine: String, _ request: SpawnRequest) async throws -> SpawnResult {
        let data = try await call(
            "POST", "/v1/machines/\(Self.segment(machine))/spawn", body: HubJSON.encoder().encode(request), timeout: 330)
        return try Self.decode(SpawnResult.self, from: data)
    }

    public func hidden() async throws -> [String] {
        let list: HiddenEnvelope = try await get("/v1/console/hidden")
        return list.ids
    }

    public func setHidden(hide: [String], show: [String]) async throws -> [String] {
        struct Request: Encodable {
            var hide: [String]?
            var show: [String]?
        }
        let request = Request(hide: hide.isEmpty ? nil : hide, show: show.isEmpty ? nil : show)
        let data = try await call("POST", "/v1/console/hidden", body: HubJSON.encoder().encode(request))
        return try Self.decode(HiddenEnvelope.self, from: data).ids
    }

    public func voiceSettings() async throws -> VoiceDocument {
        try await get("/v1/console/voice", timeout: Self.pollTimeout)
    }

    public func putVoiceSettings(_ document: VoiceDocument) async throws -> VoiceDocument {
        let data = try await call("PUT", "/v1/console/voice", body: HubJSON.encoder().encode(document), timeout: Self.pollTimeout)
        return try Self.decode(VoiceDocument.self, from: data)
    }

    public func setRunSettings(of agent: String, model: String, effort: String, access: String?) async throws -> Agent {
        let safe = agent.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-_."))) ?? agent
        // (no `access`: what it may do on its computer stays as it is)
        var body = ["model": model, "effort": effort]
        if let access { body["access"] = access }
        let data = try await call("PUT", "/v1/agents/\(safe)/run-settings", body: HubJSON.encoder().encode(body))
        return try Self.decode(Agent.self, from: data)
    }

    public func handovers(of agent: String) async throws -> [Handover] {
        let envelope: HandoversEnvelope = try await get("/v1/agents/\(Self.segment(agent))/handovers", timeout: Self.pollTimeout)
        return envelope.handovers
    }

    public func handOver(_ agent: String, _ request: HandoverRequest) async throws -> HandoverResult {
        // (the hub waits up to five minutes for the new session)
        let data = try await call(
            "POST", "/v1/agents/\(Self.segment(agent))/handover", body: HubJSON.encoder().encode(request), timeout: 330)
        return try Self.decode(HandoverResult.self, from: data)
    }

    public func handoverLink(of agent: String, note: String?, back: String?) async throws -> HandoverLink {
        struct Request: Encodable {
            var note: String?
            var back: String?
        }
        let data = try await call(
            "POST", "/v1/agents/\(Self.segment(agent))/handover-link", body: HubJSON.encoder().encode(Request(note: note, back: back)))
        return try Self.decode(HandoverLink.self, from: data)
    }

    public var address: URL? { baseURL }

    public func apps() async throws -> [AppTemplate] {
        let envelope: AppsEnvelope = try await get("/v1/templates", timeout: Self.pollTimeout)
        return envelope.templates
    }

    public func useApp(_ id: String, name: String, project: String?, sample: Bool) async throws -> Page {
        struct Request: Encodable {
            var name: String
            var project: String?
            var sample: Bool
        }
        struct Made: Decodable { var page: Page }
        let data = try await call(
            "POST", "/v1/templates/\(Self.segment(id))/use", body: HubJSON.encoder().encode(Request(name: name, project: project, sample: sample)))
        return try Self.decode(Made.self, from: data).page
    }

    public func pages() async throws -> [Page] {
        let envelope: PagesEnvelope = try await get("/v1/pages", timeout: Self.pollTimeout)
        return envelope.pages
    }

    public func pageRequest(_ method: String, page: String, path: String, query: [String: String], body: Data?) async throws -> PageAnswer {
        // (only a page's own calls: its state, its records, its changes)
        guard path == "state" || path == "changes" || path == "data" || path.hasPrefix("data/") else {
            throw HubError(error: "invalid_argument", message: "not a page call: \(path)")
        }
        let (data, response) = try await answer(
            method, "/v1/pages/\(Self.segment(page))/" + path, query: query, body: body, timeout: 70, anyStatus: true)
        guard (200..<300).contains(response.statusCode) || response.statusCode == 409 else {
            if var error = try? HubJSON.decoder().decode(HubError.self, from: data) {
                error.status = response.statusCode
                throw error
            }
            throw HubError(error: "http_\(response.statusCode)", message: "The workspace answered \(response.statusCode).", status: response.statusCode)
        }
        return PageAnswer(data: data, status: response.statusCode)
    }

    public func pins(with agent: String) async throws -> [Pin] {
        let envelope: PinsEnvelope = try await get("/v1/pins", ["with": agent], timeout: Self.pollTimeout)
        return envelope.pins
    }

    public func addPin(_ request: PinRequest) async throws -> Pin {
        let data = try await call("POST", "/v1/pins", body: HubJSON.encoder().encode(request))
        return try Self.decode(Pin.self, from: data)
    }

    public func removePin(_ id: String, with agent: String) async throws {
        _ = try await call("DELETE", "/v1/pins/\(Self.segment(id))", query: ["with": agent])
    }

    public func page(_ id: String) async throws -> Page {
        try await get("/v1/pages/\(Self.segment(id))", timeout: Self.pollTimeout)
    }

    public func pageFile(_ id: String, version: Int?, path: String) async throws -> PageFile {
        let place = version.map { "v/\($0)" } ?? "live"
        // (the path as it is: `call` escapes what a URL cannot hold)
        let (data, response) = try await answer(
            "GET", "/v1/pages/\(Self.segment(id))/\(place)/" + path, timeout: Self.transferTimeout, anyStatus: version == nil)
        return PageFile(
            data: data, contentType: response.value(forHTTPHeaderField: "Content-Type") ?? "application/octet-stream",
            status: response.statusCode)
    }

    public func computerLink(allowFrom: [String]) async throws -> JoinLink {
        struct Request: Encodable {
            var kind = "link"
            var harness = "exec"
            var allowFrom: [String]
            var maxUses = 1
            var ttlSeconds = 24 * 3600
            var note = "myco ios: a folder as a computer"
        }
        let data = try await call("POST", "/v1/enrollments", body: HubJSON.encoder().encode(Request(allowFrom: allowFrom)))
        return try Self.decode(JoinLink.self, from: data)
    }

    public func addProjectMember(_ agent: String, to project: String) async throws {
        _ = try await call("PUT", "/v1/projects/\(Self.segment(project))/members/\(Self.segment(agent))", body: Data("{}".utf8))
    }

    public func automations() async throws -> [Automation] {
        let envelope: AutomationsEnvelope = try await get("/v1/console/automations", timeout: Self.pollTimeout)
        return envelope.automations
    }

    public func automation(_ id: String) async throws -> AutomationDetail {
        try await get("/v1/console/automations/\(Self.segment(id))")
    }

    public func createAutomation(_ request: AutomationRequest) async throws -> AutomationSaved {
        let data = try await call("POST", "/v1/console/automations", body: HubJSON.encoder().encode(request))
        return try Self.decode(AutomationSaved.self, from: data)
    }

    public func updateAutomation(_ id: String, _ request: AutomationRequest) async throws -> AutomationSaved {
        let data = try await call("PATCH", "/v1/console/automations/\(Self.segment(id))", body: HubJSON.encoder().encode(request))
        return try Self.decode(AutomationSaved.self, from: data)
    }

    public func deleteAutomation(_ id: String) async throws {
        _ = try await call("DELETE", "/v1/console/automations/\(Self.segment(id))")
    }

    public func runAutomation(_ id: String) async throws -> AutomationRun {
        let data = try await call("POST", "/v1/console/automations/\(Self.segment(id))/run")
        return try Self.decode(AutomationRun.self, from: data)
    }

    /// An id as one segment of a path.
    private static func segment(_ id: String) -> String {
        id.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-_."))) ?? id
    }

    public func joinLink() async throws -> JoinLink {
        struct Request: Encodable {
            var kind = "link"
            var maxUses = 10
            var ttlSeconds = 7 * 24 * 3600
            var note = "myco ios"
        }
        let data = try await call("POST", "/v1/enrollments", body: HubJSON.encoder().encode(Request()))
        return try Self.decode(JoinLink.self, from: data)
    }

    /// Signs this device out on the hub: its token stops working.
    public func removeThisDevice() async throws {
        guard let id = try await whoami().device?.id else { return }
        _ = try await call("DELETE", "/v1/console/me/devices/\(id)")
    }

    /// Trades a one-time pairing code for this device's own token
    /// (`POST /v1/console/pair`; the code is the proof, no token needed).
    public static func pair(hub: URL, code: String, device: String, session: URLSession? = nil) async throws -> PairResponse {
        let client = HubClient(baseURL: hub, token: "", session: session)
        let data = try await client.call("POST", "/v1/console/pair", body: HubJSON.encoder().encode(["code": code, "device": device]))
        return try decode(PairResponse.self, from: data)
    }

    // MARK: Plumbing

    private func get<T: Decodable>(_ path: String, _ query: [String: String] = [:], timeout: TimeInterval = 20) async throws -> T {
        try Self.decode(T.self, from: try await call("GET", path, query: query, timeout: timeout))
    }

    private static func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        do {
            return try HubJSON.decoder().decode(type, from: data)
        } catch {
            throw HubError(error: "bad_response", message: "The workspace answered in a way this version does not understand.")
        }
    }

    private func call(
        _ method: String, _ path: String, query: [String: String] = [:], body: Data? = nil,
        contentType: String = "application/json", timeout: TimeInterval = 20
    ) async throws -> Data {
        try await answer(method, path, query: query, body: body, contentType: contentType, timeout: timeout).0
    }

    /// One request to the hub: what it answered, and with what.
    /// - Parameter anyStatus: an answer that is not a success is handed
    ///   back as it is, not thrown (a live page's own server answered).
    private func answer(
        _ method: String, _ path: String, query: [String: String] = [:], body: Data? = nil,
        contentType: String = "application/json", timeout: TimeInterval = 20, anyStatus: Bool = false
    ) async throws -> (Data, HTTPURLResponse) {
        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false) else {
            throw HubError(error: "invalid_argument", message: "The workspace's address is not a URL.")
        }
        components.path = components.path.trimmingSuffix("/") + path
        if !query.isEmpty {
            components.queryItems = query.sorted { $0.key < $1.key }.map { URLQueryItem(name: $0.key, value: $0.value) }
            // A "+" is left as it is by URLComponents and read as a space
            // by the hub: a file called "a+b.txt" would lose its plus.
            components.percentEncodedQuery = components.percentEncodedQuery?.replacingOccurrences(of: "+", with: "%2B")
        }
        guard let url = components.url else {
            throw HubError(error: "invalid_argument", message: "The workspace's address is not a URL.")
        }
        var request = URLRequest(url: url, timeoutInterval: timeout)
        request.httpMethod = method
        if !token.isEmpty { request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization") }
        if let body {
            request.httpBody = body
            request.setValue(contentType, forHTTPHeaderField: "Content-Type")
        }
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await (session ?? AppNetwork.shared.session(for: request.url)).data(for: request)
        } catch let error as URLError where error.code == .cancelled {
            throw CancellationError()
        } catch {
            // (only through the app's own sessions: a session given is a test's)
            if session == nil { AppNetwork.shared.failed(request.url, error) }
            throw HubError.unreachable(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else { throw HubError.unreachable("no HTTP response") }
        guard (200..<300).contains(http.statusCode) || anyStatus else {
            if var error = try? HubJSON.decoder().decode(HubError.self, from: data) {
                error.status = http.statusCode
                throw error
            }
            // A gateway's own page (no JSON) still says why: its first
            // words are kept, for whoever looks at the details.
            let page = http.statusCode >= 500 ? Self.words(ofPage: data) : ""
            throw HubError(
                error: "http_\(http.statusCode)", message: page.isEmpty ? "The workspace answered \(http.statusCode)." : page,
                status: http.statusCode)
        }
        return (data, http)
    }
}

extension HubClient {
    /// The first words of a page that is not the hub's JSON, without its tags.
    public static func words(ofPage data: Data) -> String {
        guard let text = String(data: data.prefix(4_000), encoding: .utf8) else { return "" }
        let plain = text.replacingOccurrences(of: "<[^>]*>", with: " ", options: .regularExpression)
            .replacingOccurrences(of: "\\s+", with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespaces)
        return String(plain.prefix(300))
    }
}

struct AppsEnvelope: Decodable { @LossyList var templates: [AppTemplate] }
struct PagesEnvelope: Decodable { @LossyList var pages: [Page] }
struct PinsEnvelope: Decodable { @LossyList var pins: [Pin] }
struct HandoversEnvelope: Decodable { @LossyList var handovers: [Handover] }
struct AutomationsEnvelope: Decodable { @LossyList var automations: [Automation] }
struct ProjectsEnvelope: Decodable { @LossyList var projects: [Project] }
struct RecentEnvelope: Decodable { @LossyList var items: [RecentItem] }
struct ExchangeEnvelope: Decodable {
    @LossyList var messages: [Message]
    var more: Bool?
}
struct SendEnvelope: Decodable { var message: Message }
struct HiddenEnvelope: Decodable { @LossyList var ids: [String] }

private extension String {
    func trimmingSuffix(_ suffix: String) -> String {
        hasSuffix(suffix) ? String(dropLast(suffix.count)) : self
    }
}

/// A device of a member: one app on one phone or computer.
public struct Device: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String

    public init(id: String, name: String) {
        self.id = id
        self.name = name
    }
}

/// What pairing gives a device: its own token (once) and who it is.
public struct PairResponse: Codable, Sendable {
    public var token: String
    public var device: Device
    public var member: Member?
}

/// A link that adds a workspace to the app: `https://<hub>/c/<code>`, or
/// `myco://add-hub?link=<that link>` (the "open in the app" button). The
/// code is a one-time pairing code or, from older hubs, a device token.
public struct AppLink: Sendable, Hashable {
    public var hub: URL
    public var credential: String

    static let pairingPrefix = "myco_pair_"
    static let tokenPrefix = "myco_console_"

    public var isPairingCode: Bool { credential.hasPrefix(Self.pairingPrefix) }

    public init?(_ raw: String) {
        var text = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if let outer = URLComponents(string: text), outer.scheme == "myco" {
            text = outer.queryItems?.first { $0.name == "link" }?.value ?? ""
        }
        guard let url = URLComponents(string: text), let scheme = url.scheme, scheme == "https" || scheme == "http",
              let host = url.host, !host.isEmpty, url.user == nil, url.password == nil,
              let marker = url.path.range(of: "/c/")
        else { return nil }
        let code = url.path[marker.upperBound...].trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard code.hasPrefix(Self.pairingPrefix) || code.hasPrefix(Self.tokenPrefix), !code.contains("/") else { return nil }
        var base = URLComponents()
        base.scheme = scheme
        base.host = host
        base.port = url.port
        base.path = String(url.path[..<marker.lowerBound]).trimmingSuffix("/")
        guard let hub = base.url else { return nil }
        self.hub = hub
        credential = code
    }
}
