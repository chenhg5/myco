import Foundation
import MycoAPI

/// Bounded HTTP requests for embedded apps on this phone. This never uses
/// workspace credentials or cookies; responses match desktop net.fetch.
/// Navigation, reload and backgrounding cancel the page's outstanding work.
@MainActor
public final class PageNetwork {
    private let session: URLSession?
    private var pending: [UUID: Task<Data, Error>] = [:]
    private var generation = 0

    public init(session: URLSession? = nil) { self.session = session }

    public func cancel() {
        generation += 1
        let tasks = pending.values
        pending.removeAll()
        for task in tasks { task.cancel() }
    }

    public func answer(_ args: [String: Any]) async throws -> Data {
        guard let raw = args["url"] as? String, let url = URL(string: raw), Self.allowed(url) else {
            throw PageBridge.Refusal(message: L("请输入不带账号密码的 HTTP(S) 地址。", "Enter an HTTP(S) address without a username or password."), code: "invalid_argument")
        }
        let mode = args["response"] as? String ?? "auto"
        guard ["auto", "text", "base64"].contains(mode) else {
            throw PageBridge.Refusal(message: L("不支持这种返回格式。", "This response format is unsupported."), code: "invalid_argument")
        }
        let limit = min(64 << 20, max(1, (args["max_bytes"] as? Int).flatMap { $0 > 0 ? $0 : nil } ?? 16 << 20))
        let milliseconds = min(600_000, (args["timeout_ms"] as? Int).flatMap { $0 > 0 ? $0 : nil } ?? 30_000)
        var request = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: Double(milliseconds) / 1000)
        request.httpShouldHandleCookies = false
        request.httpMethod = (args["method"] as? String ?? "GET").uppercased()
        guard ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"].contains(request.httpMethod ?? "") else {
            throw PageBridge.Refusal(message: L("不支持这种请求方式。", "This HTTP method is unsupported."), code: "invalid_argument")
        }
        let headers = args["headers"] as? [String: String] ?? [:]
        for (key, value) in headers {
            guard !key.contains(where: { $0 == "\r" || $0 == "\n" }), !value.contains(where: { $0 == "\r" || $0 == "\n" }) else {
                throw PageBridge.Refusal(message: L("请求头格式不正确。", "The request headers are invalid."), code: "invalid_argument")
            }
            request.setValue(value, forHTTPHeaderField: key)
        }
        if request.value(forHTTPHeaderField: "User-Agent") == nil { request.setValue("myco-app", forHTTPHeaderField: "User-Agent") }
        if let body = args["body"] as? String {
            if args["body_encoding"] as? String == "base64" {
                guard let data = Data(base64Encoded: body) else {
                    throw PageBridge.Refusal(message: L("请求内容读不出来。", "The request body could not be read."), code: "invalid_argument")
                }
                request.httpBody = data
            } else { request.httpBody = Data(body.utf8) }
            guard (request.httpBody?.count ?? 0) <= 64 << 20 else {
                throw PageBridge.Refusal(message: L("请求内容太大。", "The request body is too large."), code: "too_large")
            }
        }
        let transport = session ?? AppNetwork.shared.session(for: url)
        let policy = RedirectPolicy(follow: args["redirect"] as? Bool ?? true)
        let id = UUID(), epoch = generation
        let input = request
        let task = Task { try await Self.fetch(input, session: transport, policy: policy, limit: limit, mode: mode) }
        pending[id] = task
        defer { pending[id] = nil }
        do {
            let result = try await task.value
            guard epoch == generation, !task.isCancelled else { throw CancellationError() }
            return result
        } catch {
            let code = (error as? URLError)?.code
            if task.isCancelled || error is CancellationError || code == .cancelled {
                throw PageBridge.Refusal(message: L("请求已取消。", "The request was cancelled."), code: "cancelled")
            }
            if code == .timedOut {
                throw PageBridge.Refusal(message: L("连接超时，请检查网络后重试。", "The connection timed out. Check your network and try again."), code: "timeout")
            }
            AppNetwork.shared.failed(url, error)
            throw PageBridge.Refusal(message: L("连接失败，请检查网络后重试。", "The connection failed. Check your network and try again."), code: "failed")
        }
    }

    nonisolated static func allowed(_ url: URL) -> Bool {
        ["http", "https"].contains(url.scheme?.lowercased() ?? "") && !(url.host() ?? "").isEmpty && url.user == nil && url.password == nil
    }

    nonisolated private static func fetch(_ request: URLRequest, session: URLSession, policy: RedirectPolicy, limit: Int, mode: String) async throws -> Data {
        let (stream, response) = try await session.bytes(for: request, delegate: policy)
        // Breaking the sequence at the byte cap must also stop the download.
        defer { stream.task.cancel() }
        guard let response = response as? HTTPURLResponse else { throw URLError(.badServerResponse) }
        var body = Data()
        body.reserveCapacity(min(limit, max(0, Int(response.expectedContentLength))))
        var truncated = false
        for try await byte in stream {
            if body.count == limit { truncated = true; break }
            body.append(byte)
        }
        try Task.checkCancellation()
        let headers = response.allHeaderFields.reduce(into: [String: String]()) { values, pair in
            values[String(describing: pair.key).lowercased()] = String(describing: pair.value)
        }
        let type = (headers["content-type"] ?? "").lowercased()
        let text = String(data: body, encoding: .utf8)
        let asText = mode == "text" || mode == "auto" && text != nil && (type.isEmpty || type.hasPrefix("text/") || type.contains("json") || type.contains("xml") || type.contains("javascript") || type.contains("yaml"))
        return try JSONSerialization.data(withJSONObject: [
            "status": response.statusCode, "status_text": HTTPURLResponse.localizedString(forStatusCode: response.statusCode),
            "url": (response.url ?? request.url)?.absoluteString ?? "", "headers": headers,
            "body": asText ? (text ?? String(decoding: body, as: UTF8.self)) : body.base64EncodedString(),
            "encoding": asText ? "utf8" : "base64", "truncated": truncated,
        ])
    }
}

/// Per-request redirect/auth policy, with no retained user credentials.
private final class RedirectPolicy: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    let follow: Bool
    init(follow: Bool) { self.follow = follow }

    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        guard follow, let url = request.url, PageNetwork.allowed(url) else { completionHandler(nil); return }
        var next = request
        next.httpShouldHandleCookies = false
        if response.url?.scheme != url.scheme || response.url?.host() != url.host() || response.url?.port != url.port {
            for name in ["Authorization", "Cookie", "Proxy-Authorization"] { next.setValue(nil, forHTTPHeaderField: name) }
        }
        completionHandler(next)
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didReceive challenge: URLAuthenticationChallenge, completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        completionHandler(challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust ? .performDefaultHandling : .cancelAuthenticationChallenge, nil)
    }
}
