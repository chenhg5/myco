import Foundation
import Network
import Security
import Darwin

/// Public media streamed to a page's existing origin. DNS is checked once per
/// hop and the connection is pinned to one of those IPs, with the original TLS
/// server name. No workspace token, cookies, proxy credentials or page headers
/// other than Range/If-Range ever leave the App.
@MainActor
public final class PageMedia {
    public static let path = "/__myco/stream"
    public struct Failure: Error, Sendable {
        public let status: Int
        public let message: String
    }
    typealias Resolver = @Sendable (String) async throws -> [String]
    typealias Connector = @MainActor (URL, [String]) async throws -> any MediaWire
    private let resolve: Resolver
    private let connect: Connector
    private var active: [UUID: Transfer] = [:]
    private var paused: Set<String> = []

    public init() {
        resolve = { try await Self.lookup($0) }
        connect = MediaConnection.connect
    }
    init(resolve: @escaping Resolver, connect: @escaping Connector) {
        self.resolve = resolve
        self.connect = connect
    }

    static func target(_ raw: String) throws -> URL {
        guard raw.utf8.count <= 4096, var parts = URLComponents(string: raw),
              let scheme = parts.scheme?.lowercased(), ["http", "https"].contains(scheme),
              let host = parts.host, !host.isEmpty, parts.user == nil, parts.password == nil,
              parts.port.map({ (1...65535).contains($0) }) ?? true else {
            throw Failure(status: 400, message: "Only credential-free HTTP(S) addresses can be streamed")
        }
        parts.scheme = scheme
        parts.fragment = nil
        guard let url = parts.url else { throw Failure(status: 400, message: "Invalid media address") }
        return url
    }

    static func isPublic(_ text: String) -> Bool {
        if let ip = IPv4Address(text) {
            let b = [UInt8](ip.rawValue)
            return !(b[0] == 0 || b[0] == 10 || b[0] == 127 || b[0] >= 224
                || (b[0] == 172 && (16...31).contains(b[1])) || (b[0] == 192 && b[1] == 168)
                || (b[0] == 169 && b[1] == 254) || (b[0] == 100 && b[1] & 0xc0 == 64))
        }
        if let ip = IPv6Address(text) {
            let b = [UInt8](ip.rawValue)
            if b.prefix(10).allSatisfy({ $0 == 0 }), b[10] == 255, b[11] == 255 {
                return isPublic(b[12...15].map(String.init).joined(separator: "."))
            }
            // Well-known NAT64 prefix: the embedded IPv4 must also be public.
            if Array(b.prefix(12)) == [0, 100, 255, 155, 0, 0, 0, 0, 0, 0, 0, 0] {
                return isPublic(b[12...15].map(String.init).joined(separator: "."))
            }
            return !b.prefix(12).allSatisfy({ $0 == 0 }) && b[0] != 255 && b[0] & 0xfe != 0xfc
                && !(b[0] == 0xfe && b[1] & 0xc0 >= 0x80)
        }
        return false
    }

    static func mediaType(_ raw: String) -> Bool {
        let type = (raw.split(separator: ";", maxSplits: 1).first ?? "").trimmingCharacters(in: .whitespaces).lowercased()
        return type.hasPrefix("audio/") || type.hasPrefix("video/") || ["application/octet-stream", "binary/octet-stream", "application/ogg", "application/vnd.apple.mpegurl", "application/x-mpegurl"].contains(type)
    }

    private func addresses(for url: URL) async throws -> [String] {
        let host = (url.host(percentEncoded: false) ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[].")).lowercased()
        guard host != "localhost", !host.hasSuffix(".localhost"), !host.hasSuffix(".local") else {
            throw Failure(status: 403, message: "Only public addresses can be streamed")
        }
        let addresses = try await resolve(host)
        try Task.checkCancellation()
        guard !addresses.isEmpty, addresses.allSatisfy(Self.isPublic) else {
            throw Failure(status: 403, message: "Only public addresses can be streamed")
        }
        return addresses
    }

    nonisolated private static func lookup(_ host: String) async throws -> [String] {
        let reply = MediaDNSReply()
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                guard reply.install(continuation) else { return }
                DispatchQueue.global(qos: .utility).async {
                    reply.finish(Result { try lookupBlocking(host) })
                }
                DispatchQueue.global(qos: .utility).asyncAfter(deadline: .now() + 15) {
                    reply.finish(.failure(URLError(.timedOut)))
                }
            }
        } onCancel: { reply.finish(.failure(CancellationError())) }
    }

    nonisolated private static func lookupBlocking(_ host: String) throws -> [String] {
        var hints = addrinfo()
        hints.ai_family = AF_UNSPEC
        hints.ai_socktype = SOCK_STREAM
        hints.ai_protocol = IPPROTO_TCP
        var result: UnsafeMutablePointer<addrinfo>?
        guard getaddrinfo(host, nil, &hints, &result) == 0, let result else { throw URLError(.cannotFindHost) }
        defer { freeaddrinfo(result) }
        var found: [String] = [], next: UnsafeMutablePointer<addrinfo>? = result
        while let item = next {
            var name = [CChar](repeating: 0, count: Int(NI_MAXHOST))
            if getnameinfo(item.pointee.ai_addr, item.pointee.ai_addrlen, &name, socklen_t(name.count), nil, 0, NI_NUMERICHOST) == 0 {
                found.append(String(decoding: name.prefix { $0 != 0 }.map { UInt8(bitPattern: $0) }, as: UTF8.self))
            }
            next = item.pointee.ai_next
        }
        return found
    }

    private static func key(_ url: URL?) -> String {
        URLComponents(url: url ?? URL(string: "about:blank")!, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "url" }?.value ?? ""
    }
    public func pause(address: String) {
        let key = Self.key(URL(string: address))
        guard !key.isEmpty else { return }
        paused.insert(key)
        for item in active.values where item.key == key {
            item.paused = true
            item.generation += 1
            item.wire?.close()
        }
    }
    public func resume(address: String) {
        let key = Self.key(URL(string: address))
        paused.remove(key)
        for item in active.values where item.key == key { item.resume() }
    }
    public func cancel() {
        paused.removeAll()
        for item in active.values { item.task?.cancel(); item.cancel() }
        active.removeAll()
    }

    /// Callbacks are serialized on the main actor, in wire order. Each read
    /// is at most 64 KiB, independent of the size of the audio/video file.
    public func stream(_ request: URLRequest, response: @escaping @MainActor (HTTPURLResponse) -> Void, data: @escaping @MainActor (Data) -> Void) async throws {
        guard let local = request.url, ["GET", "HEAD"].contains(request.httpMethod ?? "GET") else {
            throw Failure(status: 405, message: "Only GET and HEAD media requests are supported")
        }
        let key = Self.key(local)
        let target = try Self.target(key)
        let item = Transfer(key: key, paused: paused.contains(key))
        let id = UUID()
        let work = Task { try await self.transfer(request, target: target, item: item, response: response, data: data) }
        item.task = work
        active[id] = item
        defer { item.cancel(); item.task = nil; active[id] = nil }
        try await withTaskCancellationHandler { try await work.value } onCancel: { work.cancel() }
    }

    private func transfer(_ original: URLRequest, target: URL, item: Transfer, response: @MainActor (HTTPURLResponse) -> Void, data: @MainActor (Data) -> Void) async throws {
        var url = target, hops = 0
        var first: MediaHead?
        var delivered: UInt64 = 0
        while true {
            try Task.checkCancellation()
            try await item.waitUntilPlaying()
            if let length = first?.length, delivered >= length { return }
            let generation = item.generation
            do {
                let addresses = try await addresses(for: url)
                try await item.waitUntilPlaying()
                let wire = try await connect(url, addresses)
                item.wire = wire
                defer { wire.close(); item.wire = nil }
                if item.paused || item.generation != generation { continue }
                var range = original.value(forHTTPHeaderField: "Range")
                var ifRange = original.value(forHTTPHeaderField: "If-Range")
                if let first, delivered > 0 {
                    range = "bytes=\(first.start + delivered)-" + (first.length.map { String(first.start + $0 - 1) } ?? "")
                    ifRange = first.headers["etag"] ?? first.headers["last-modified"] ?? ifRange
                }
                try await wire.send(Self.request(url, method: original.httpMethod ?? "GET", range: range, ifRange: ifRange))
                let reader = MediaReader(wire)
                let head = try await reader.head()
                if [301, 302, 303, 307, 308].contains(head.status) {
                    guard hops < 8, let location = head.headers["location"], let next = URL(string: location, relativeTo: url)?.absoluteURL else { throw URLError(.httpTooManyRedirects) }
                    url = try Self.target(next.absoluteString)
                    hops += 1
                    continue // Each hop resolves and checks all addresses again.
                }
                guard [200, 206, 416].contains(head.status) else { throw Failure(status: 502, message: "The media server refused this request") }
                guard head.status == 416 || Self.mediaType(head.headers["content-type"] ?? "") else { throw Failure(status: 415, message: "Not audio or video") }
                if let first, delivered > 0 {
                    guard head.status == 206, head.start == first.start + delivered,
                          first.headers["etag"].map({ head.headers["etag"] == $0 }) ?? true else {
                        throw Failure(status: 502, message: "The media changed or cannot resume with Range")
                    }
                } else if first == nil {
                    first = head
                    var headers: [String: String] = ["Cache-Control": "no-store", "Content-Security-Policy": "sandbox", "X-Content-Type-Options": "nosniff"]
                    for name in ["content-type", "content-length", "content-range", "accept-ranges", "etag", "last-modified"] { headers[name] = head.headers[name] }
                    guard let answer = HTTPURLResponse(url: original.url!, statusCode: head.status, httpVersion: "HTTP/1.1", headerFields: headers) else { throw URLError(.badServerResponse) }
                    response(answer)
                }
                if original.httpMethod == "HEAD" { return }
                try await reader.body(head) { chunk in
                    try Task.checkCancellation()
                    guard !item.paused, generation == item.generation else { throw CancellationError() }
                    guard delivered <= UInt64(Int64.max) - first!.start - UInt64(chunk.count) else { throw URLError(.dataLengthExceedsMaximum) }
                    delivered += UInt64(chunk.count)
                    data(chunk)
                }
                return
            } catch {
                try Task.checkCancellation()
                // Pause closes the socket, not the WebKit resource. Play opens
                // a validated Range continuation at exactly the forwarded byte.
                if item.generation != generation { continue }
                throw error
            }
        }
    }

    static func request(_ url: URL, method: String, range: String?, ifRange: String?) throws -> Data {
        guard let parts = URLComponents(url: url, resolvingAgainstBaseURL: false), let host = parts.host else { throw URLError(.badURL) }
        var path = parts.percentEncodedPath.isEmpty ? "/" : parts.percentEncodedPath
        if let query = parts.percentEncodedQuery { path += "?" + query }
        let authority = host + (parts.port.map { ":\($0)" } ?? "")
        var text = "\(method) \(path) HTTP/1.1\r\nHost: \(authority)\r\nUser-Agent: myco-app\r\nAccept-Encoding: identity\r\nConnection: close\r\n"
        for (name, value) in [("Range", range), ("If-Range", ifRange)] {
            if let value {
                guard value.utf8.count <= 8192, !value.contains(where: { $0 == "\r" || $0 == "\n" }) else { throw URLError(.badURL) }
                text += "\(name): \(value)\r\n"
            }
        }
        return Data((text + "\r\n").utf8)
    }

    @MainActor
    private final class Transfer {
        let key: String
        var paused: Bool
        var generation = 0
        var wire: (any MediaWire)?
        var task: Task<Void, Error>?
        var waiting: CheckedContinuation<Void, Error>?
        init(key: String, paused: Bool) { self.key = key; self.paused = paused }
        func resume() { paused = false; waiting?.resume(); waiting = nil }
        func cancel() { wire?.close(); waiting?.resume(throwing: CancellationError()); waiting = nil }
        func waitUntilPlaying() async throws {
            try Task.checkCancellation()
            if !paused { return }
            try await withTaskCancellationHandler {
                try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                    if Task.isCancelled { continuation.resume(throwing: CancellationError()) }
                    else { waiting = continuation }
                }
            } onCancel: { Task { @MainActor in self.cancel() } }
            try Task.checkCancellation()
        }
    }
}

/// System DNS runs on a worker; cancellation/deadline releases the page's
/// waiter immediately even if getaddrinfo itself has not returned yet.
private final class MediaDNSReply: @unchecked Sendable {
    private let lock = NSLock()
    private var result: Result<[String], Error>?
    private var continuation: CheckedContinuation<[String], Error>?
    func install(_ continuation: CheckedContinuation<[String], Error>) -> Bool {
        lock.lock()
        if let result { lock.unlock(); continuation.resume(with: result); return false }
        self.continuation = continuation
        lock.unlock()
        return true
    }
    func finish(_ result: Result<[String], Error>) {
        lock.lock()
        guard self.result == nil else { lock.unlock(); return }
        self.result = result
        let waiting = continuation
        continuation = nil
        lock.unlock()
        waiting?.resume(with: result)
    }
}

/// A small injectable boundary: tests provide bytes, never disable the public-IP policy.
@MainActor
protocol MediaWire: AnyObject {
    func send(_ data: Data) async throws
    func read() async throws -> Data
    func close()
}

@MainActor
private final class MediaConnection: MediaWire {
    private let connection: NWConnection
    private var ready: CheckedContinuation<Void, Error>?
    private var deadline: Task<Void, Never>?
    private init(_ url: URL, address: String) throws {
        let host = (url.host(percentEncoded: false) ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        let parameters: NWParameters
        if url.scheme == "https" {
            let tls = NWProtocolTLS.Options()
            sec_protocol_options_set_tls_server_name(tls.securityProtocolOptions, host)
            sec_protocol_options_add_tls_application_protocol(tls.securityProtocolOptions, "http/1.1")
            parameters = NWParameters(tls: tls, tcp: NWProtocolTCP.Options())
        } else { parameters = .tcp }
        guard let port = NWEndpoint.Port(rawValue: UInt16(url.port ?? (url.scheme == "https" ? 443 : 80))) else { throw URLError(.badURL) }
        connection = NWConnection(host: NWEndpoint.Host(address), port: port, using: parameters)
    }
    static func connect(_ url: URL, addresses: [String]) async throws -> any MediaWire {
        var last: any Error = URLError(.cannotConnectToHost)
        for address in addresses {
            try Task.checkCancellation()
            let wire = try MediaConnection(url, address: address)
            do { try await wire.start(); return wire } catch { last = error; wire.close() }
        }
        throw last
    }
    private func armDeadline() { deadline?.cancel(); deadline = Task { try? await Task.sleep(for: .seconds(30)); if !Task.isCancelled { close() } } }
    private func start() async throws {
        armDeadline()
        defer { deadline?.cancel() }
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                ready = continuation
                connection.stateUpdateHandler = { [weak self] state in
                    Task { @MainActor in
                        guard let self, let waiting = self.ready else { return }
                        switch state {
                        case .ready: self.ready = nil; waiting.resume()
                        case .failed(let error), .waiting(let error): self.ready = nil; waiting.resume(throwing: error)
                        case .cancelled: self.ready = nil; waiting.resume(throwing: CancellationError())
                        default: break
                        }
                    }
                }
                connection.start(queue: .global(qos: .utility))
            }
        } onCancel: { connection.cancel() }
    }
    func send(_ data: Data) async throws {
        try Task.checkCancellation()
        armDeadline()
        defer { deadline?.cancel() }
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                connection.send(content: data, completion: .contentProcessed { error in
                    if let error { continuation.resume(throwing: error) } else { continuation.resume() }
                })
            }
        } onCancel: { connection.cancel() }
    }
    func read() async throws -> Data {
        try Task.checkCancellation()
        armDeadline()
        defer { deadline?.cancel() }
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                connection.receive(minimumIncompleteLength: 1, maximumLength: 65536) { data, _, _, error in
                    if let error { continuation.resume(throwing: error) } else { continuation.resume(returning: data ?? Data()) }
                }
            }
        } onCancel: { connection.cancel() }
    }
    func close() { deadline?.cancel(); connection.cancel() }
}

struct MediaHead {
    var status: Int
    var headers: [String: String]
    var length: UInt64? { headers["content-length"].flatMap(UInt64.init) }
    var start: UInt64 {
        guard let range = headers["content-range"], range.hasPrefix("bytes ") else { return 0 }
        return UInt64(range.dropFirst(6).split(separator: "-").first ?? "") ?? 0
    }
}

/// Bounded HTTP/1.1 decoding, including transfer-chunk framing (not media
/// chunks). Content bytes are handed on as they arrive, without accumulation.
@MainActor
private final class MediaReader {
    let wire: any MediaWire
    var buffer = Data()
    init(_ wire: any MediaWire) { self.wire = wire }
    private func more() async throws {
        let chunk = try await wire.read()
        guard !chunk.isEmpty else { throw URLError(.networkConnectionLost) }
        buffer.append(chunk)
    }
    private func line() async throws -> String {
        while true {
            if let end = buffer.range(of: Data([13, 10])) {
                guard end.lowerBound - buffer.startIndex <= 8192 else { throw URLError(.badServerResponse) }
                let text = String(decoding: buffer[..<end.lowerBound], as: UTF8.self)
                buffer.removeSubrange(..<end.upperBound)
                return text
            }
            guard buffer.count <= 8192 else { throw URLError(.badServerResponse) }
            try await more()
        }
    }
    func head() async throws -> MediaHead {
        for _ in 0..<5 {
            let status = try await line().split(separator: " ", maxSplits: 2)
            guard status.count >= 2, ["HTTP/1.0", "HTTP/1.1"].contains(status[0]), let code = Int(status[1]) else { throw URLError(.badServerResponse) }
            var headers: [String: String] = [:], size = 0
            while true {
                let field = try await line()
                if field.isEmpty { break }
                size += field.utf8.count
                guard size <= 65536, let colon = field.firstIndex(of: ":") else { throw URLError(.badServerResponse) }
                let key = field[..<colon].lowercased(), value = field[field.index(after: colon)...].trimmingCharacters(in: .whitespaces)
                if headers[key] != nil {
                    guard !["content-length", "content-range", "transfer-encoding", "content-encoding", "location", "etag", "content-type"].contains(key) else { throw URLError(.badServerResponse) }
                } else { headers[key] = value }
            }
            if code == 100 || code == 103 { continue }
            guard headers["content-length"] == nil || Int64(headers["content-length"]!).map({ $0 >= 0 }) == true,
                  headers["transfer-encoding"] == nil || headers["transfer-encoding"]?.lowercased() == "chunked",
                  headers["content-encoding"] == nil || headers["content-encoding"]?.lowercased() == "identity",
                  headers["transfer-encoding"] == nil || headers["content-length"] == nil else { throw URLError(.badServerResponse) }
            if code == 206 {
                guard let range = headers["content-range"], range.hasPrefix("bytes ") else { throw URLError(.badServerResponse) }
                let pieces = range.dropFirst(6).split(separator: "/", omittingEmptySubsequences: false)
                let bounds = pieces.first?.split(separator: "-", omittingEmptySubsequences: false) ?? []
                guard pieces.count == 2, bounds.count == 2,
                      let lower = Int64(bounds[0]), let upper = Int64(bounds[1]), lower >= 0, upper >= lower, upper < Int64.max,
                      pieces[1] == "*" || Int64(pieces[1]).map({ $0 > upper }) == true,
                      headers["content-length"] == nil || Int64(headers["content-length"]!) == upper - lower + 1 else { throw URLError(.badServerResponse) }
            }
            return MediaHead(status: code, headers: headers)
        }
        throw URLError(.badServerResponse)
    }
    private func bytes(_ count: UInt64, receive: @MainActor (Data) throws -> Void) async throws {
        var remaining = count
        while remaining > 0 {
            if buffer.isEmpty { try await more() }
            let count = Int(min(remaining, UInt64(buffer.count)))
            try receive(Data(buffer.prefix(count)))
            buffer.removeFirst(count)
            remaining -= UInt64(count)
        }
    }
    func body(_ head: MediaHead, receive: @MainActor (Data) throws -> Void) async throws {
        if head.headers["transfer-encoding"]?.lowercased() == "chunked" {
            while true {
                let size = try await line().split(separator: ";", maxSplits: 1).first ?? ""
                guard let count = UInt64(size.trimmingCharacters(in: .whitespaces), radix: 16) else { throw URLError(.badServerResponse) }
                if count == 0 { return }
                try await bytes(count, receive: receive)
                guard try await line().isEmpty else { throw URLError(.badServerResponse) }
            }
        } else if let count = head.length { try await bytes(count, receive: receive) }
        else {
            if !buffer.isEmpty { try receive(buffer); buffer.removeAll(keepingCapacity: true) }
            while true { let chunk = try await wire.read(); if chunk.isEmpty { return }; try receive(chunk) }
        }
    }
}
