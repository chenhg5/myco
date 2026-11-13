import Foundation

/// A real hub, over HTTPS, as one device: its `/v1/console/*` calls with
/// the device's token. Nothing is cached here.
public struct HubClient: HubAPI {
    public let baseURL: URL
    private let token: String
    private let session: URLSession

    /// How many rows or messages one call asks for.
    static let pageSize = 100

    public init(baseURL: URL, token: String, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.token = token
        self.session = session
    }

    public func whoami() async throws -> Whoami {
        try await get("/v1/console/me")
    }

    public func overview() async throws -> Overview {
        try await get("/v1/overview", ["recent": "0"])
    }

    public func projects() async throws -> [Project] {
        let envelope: ProjectsEnvelope = try await get("/v1/projects")
        return envelope.projects
    }

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
        }
        let envelope: RecentEnvelope = try await get("/v1/console/recent", query)
        return envelope.items
    }

    public func exchanges(between a: String, and b: String, before: Date?) async throws -> MessagePage {
        var query = ["a": a, "b": b, "limit": String(Self.pageSize)]
        // The hub pages by time in milliseconds.
        if let before { query["before"] = String(Int64((before.timeIntervalSince1970 * 1000).rounded(.down))) }
        let envelope: ExchangeEnvelope = try await get("/v1/console/exchanges", query)
        return MessagePage(messages: envelope.messages, more: envelope.more ?? false)
    }

    public func trace(of messageID: String, after seq: Int) async throws -> Trace {
        try await get("/v1/console/messages/\(messageID)/trace", ["after": String(seq)])
    }

    public func send(_ request: SendRequest) async throws -> Message {
        let data = try await call("POST", "/v1/console/messages", body: HubJSON.encoder().encode(request))
        return try Self.decode(SendEnvelope.self, from: data).message
    }

    public func steer(_ messageID: String, body: String) async throws {
        _ = try await call("POST", "/v1/console/messages/\(messageID)/steer", body: HubJSON.encoder().encode(["body": body]))
    }

    public func stop(_ messageID: String) async throws {
        _ = try await call("POST", "/v1/console/messages/\(messageID)/stop")
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
    public static func pair(hub: URL, code: String, device: String, session: URLSession = .shared) async throws -> PairResponse {
        let client = HubClient(baseURL: hub, token: "", session: session)
        let data = try await client.call("POST", "/v1/console/pair", body: HubJSON.encoder().encode(["code": code, "device": device]))
        return try decode(PairResponse.self, from: data)
    }

    // MARK: Plumbing

    private func get<T: Decodable>(_ path: String, _ query: [String: String] = [:]) async throws -> T {
        try Self.decode(T.self, from: try await call("GET", path, query: query))
    }

    private static func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        do {
            return try HubJSON.decoder().decode(type, from: data)
        } catch {
            throw HubError(error: "bad_response", message: "The workspace answered in a way this version does not understand.")
        }
    }

    private func call(_ method: String, _ path: String, query: [String: String] = [:], body: Data? = nil) async throws -> Data {
        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false) else {
            throw HubError(error: "invalid_argument", message: "The workspace's address is not a URL.")
        }
        components.path = components.path.trimmingSuffix("/") + path
        if !query.isEmpty {
            components.queryItems = query.sorted { $0.key < $1.key }.map { URLQueryItem(name: $0.key, value: $0.value) }
        }
        guard let url = components.url else {
            throw HubError(error: "invalid_argument", message: "The workspace's address is not a URL.")
        }
        var request = URLRequest(url: url, timeoutInterval: 20)
        request.httpMethod = method
        if !token.isEmpty { request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization") }
        if let body {
            request.httpBody = body
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch let error as URLError where error.code == .cancelled {
            throw CancellationError()
        } catch {
            throw HubError.unreachable(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else { throw HubError.unreachable("no HTTP response") }
        guard (200..<300).contains(http.statusCode) else {
            if let error = try? HubJSON.decoder().decode(HubError.self, from: data) { throw error }
            throw HubError(error: "http_\(http.statusCode)", message: "The workspace answered \(http.statusCode).")
        }
        return data
    }
}

struct ProjectsEnvelope: Decodable { @LossyList var projects: [Project] }
struct RecentEnvelope: Decodable { @LossyList var items: [RecentItem] }
struct ExchangeEnvelope: Decodable {
    @LossyList var messages: [Message]
    var more: Bool?
}
struct SendEnvelope: Decodable { var message: Message }

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
