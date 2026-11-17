import Foundation
import Network

/// The app's network proxy (Settings › 网络代理): how it reaches its
/// workspaces and the voice services. As the system has it (the iPhone's
/// Wi-Fi proxy and VPN, the default), none, or one set here: SOCKS5, HTTP
/// or HTTPS, with a user and password if it needs them (the password in
/// the keychain). Addresses on this phone and the local network always go
/// direct: a workspace there keeps working. The desktop app's,
/// internal/console/proxy.go.
public struct ProxySettings: Codable, Sendable, Hashable {
    public enum Mode: String, Codable, Sendable, CaseIterable {
        /// What the system is set to.
        case system
        /// Every connection goes directly.
        case none
        /// The proxy set here.
        case manual
    }

    public enum Kind: String, Codable, Sendable, CaseIterable {
        case socks5, http, https
    }

    public var mode: Mode
    public var kind: Kind
    public var host: String
    public var port: Int
    public var user: String

    public init(mode: Mode = .system, kind: Kind = .socks5, host: String = "", port: Int = 0, user: String = "") {
        self.mode = mode
        self.kind = kind
        self.host = host
        self.port = port
        self.user = user
    }

    // What was saved before a field existed, or with a value this app does
    // not know, still reads: as the system has it.
    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        mode = (try? values.decodeIfPresent(Mode.self, forKey: .mode)) ?? .system
        kind = (try? values.decodeIfPresent(Kind.self, forKey: .kind)) ?? .socks5
        host = (try? values.decodeIfPresent(String.self, forKey: .host)) ?? ""
        port = (try? values.decodeIfPresent(Int.self, forKey: .port)) ?? 0
        user = (try? values.decodeIfPresent(String.self, forKey: .user)) ?? ""
    }

    /// Where the proxy's password is kept in the keychain.
    public static let passwordKey = "proxy:password"

    /// What is wrong with a setting, in a word for the page to say; nil
    /// when it can be used.
    public enum Problem: Sendable, Hashable {
        case host, port
    }

    public var problem: Problem? {
        guard mode == .manual else { return nil }
        let host = host.trimmingCharacters(in: .whitespaces)
        if host.isEmpty || host.contains("://") || host.contains(where: { "/ \t@".contains($0) }) { return .host }
        if port < 1 || port > 65535 { return .port }
        return nil
    }

    /// A host on this device or the local network: never through a proxy
    /// set here.
    public static func isLocal(_ host: String) -> Bool {
        var name = host.lowercased()
        while name.hasSuffix(".") { name.removeLast() }
        if name.hasPrefix("["), name.hasSuffix("]") { name = String(name.dropFirst().dropLast()) }
        if name == "localhost" || name.hasSuffix(".localhost") || name.hasSuffix(".local") { return true }
        if let v4 = IPv4Address(name) {
            let b = [UInt8](v4.rawValue)
            return b[0] == 127 || b[0] == 10 || (b[0] == 172 && (16...31).contains(b[1])) || (b[0] == 192 && b[1] == 168)
                || (b[0] == 169 && b[1] == 254) || b.allSatisfy { $0 == 0 }
        }
        if let v6 = IPv6Address(name) {
            let b = [UInt8](v6.rawValue)
            let loopback = b.dropLast().allSatisfy { $0 == 0 } && b[15] == 1
            let unspecified = b.allSatisfy { $0 == 0 }
            // fc00::/7 (private) and fe80::/10 (link-local)
            return loopback || unspecified || (b[0] & 0xFE) == 0xFC || (b[0] == 0xFE && (b[1] & 0xC0) == 0x80)
        }
        return false
    }
}

/// The sessions the app's requests go through, by the proxy setting in
/// force. Whoever makes a request asks for the session at that moment, so
/// a setting saved is in force at once.
public final class AppNetwork: @unchecked Sendable {
    public static let shared = AppNetwork()

    private let lock = NSLock()
    private var settings = ProxySettings()
    /// What the setting says: the system's way, no proxy, or the one set.
    private var through: URLSession = .shared
    /// No proxy at all: for this device and the local network while a
    /// proxy is set here.
    private var directly: URLSession?

    public init() {}

    /// Puts a setting in force (at launch, and when saved). One that
    /// cannot be used counts as the system's.
    public func apply(_ settings: ProxySettings, password: String) {
        let usable = settings.problem == nil ? settings : ProxySettings()
        let through = Self.session(usable, password: password)
        let directly = usable.mode == .manual ? Self.session(ProxySettings(mode: .none), password: "") : nil
        lock.lock()
        let old = [self.through, self.directly].compactMap { $0 }.filter { $0 !== URLSession.shared }
        self.settings = usable
        self.through = through
        self.directly = directly
        lock.unlock()
        // What is under way finishes on the session it began on.
        for session in old { session.finishTasksAndInvalidate() }
    }

    /// The setting in force.
    public var current: ProxySettings {
        lock.lock()
        defer { lock.unlock() }
        return settings
    }

    /// The session a request to this address goes through now.
    public func session(for url: URL?) -> URLSession {
        lock.lock()
        defer { lock.unlock() }
        if let directly, let host = url?.host(), ProxySettings.isLocal(host) { return directly }
        return through
    }

    /// A session of its own for a setting: the app's (`apply`), or one to
    /// try a setting with before it is saved.
    public static func session(_ settings: ProxySettings, password: String) -> URLSession {
        switch settings.mode {
        case .system:
            return .shared
        case .none:
            let configuration = URLSessionConfiguration.default
            // An empty list: not the system's proxies either.
            configuration.connectionProxyDictionary = [:]
            return URLSession(configuration: configuration)
        case .manual:
            let configuration = URLSessionConfiguration.default
            // (Not with an empty `connectionProxyDictionary` beside it: that
            // turns the proxy set here off as well, and every request goes
            // directly without a word.)
            configuration.proxyConfigurations = [proxy(settings, password: password)]
            return URLSession(configuration: configuration)
        }
    }

    /// The proxy set here, as the system takes it.
    static func proxy(_ settings: ProxySettings, password: String) -> ProxyConfiguration {
        let host = settings.host.trimmingCharacters(in: .whitespaces)
        let port = NWEndpoint.Port(rawValue: UInt16(clamping: settings.port)) ?? 1080
        let endpoint = NWEndpoint.hostPort(host: NWEndpoint.Host(host), port: port)
        var proxy: ProxyConfiguration
        switch settings.kind {
        case .socks5: proxy = ProxyConfiguration(socksv5Proxy: endpoint)
        case .http: proxy = ProxyConfiguration(httpCONNECTProxy: endpoint)
        case .https: proxy = ProxyConfiguration(httpCONNECTProxy: endpoint, tlsOptions: NWProtocolTLS.Options())
        }
        if !settings.user.isEmpty { proxy.applyCredential(username: settings.user, password: password) }
        // The proxy or nothing: a request must not slip past it.
        proxy.allowFailover = false
        return proxy
    }
}

/// How trying a proxy setting went.
public enum ProxyTrial: Sendable, Hashable {
    case connected(milliseconds: Int)
    case timeout
    case refused
    case notFound
    case auth
    case other(String)

    /// How long a try waits for an answer.
    public static let patience: TimeInterval = 12

    /// One request through the setting given, to a workspace or the
    /// address given: connected and how long it took, or why not.
    public static func run(_ settings: ProxySettings, password: String, to target: URL) async -> ProxyTrial {
        let session = AppNetwork.session(settings, password: password)
        defer { if session !== URLSession.shared { session.invalidateAndCancel() } }
        var request = URLRequest(url: target, timeoutInterval: patience)
        request.httpMethod = "HEAD"
        request.cachePolicy = .reloadIgnoringLocalAndRemoteCacheData
        let start = Date()
        do {
            let (_, response) = try await session.data(for: request)
            if (response as? HTTPURLResponse)?.statusCode == 407 { return .auth }
            return .connected(milliseconds: max(1, Int(Date().timeIntervalSince(start) * 1000)))
        } catch {
            return of(error)
        }
    }

    /// Why a request through a proxy failed, without its address (it may
    /// carry the proxy's password).
    static func of(_ error: Error) -> ProxyTrial {
        // The error and what is under it.
        var chain: [NSError] = []
        var next: NSError? = error as NSError
        while let one = next, chain.count < 6 {
            chain.append(one)
            next = one.userInfo[NSUnderlyingErrorKey] as? NSError
        }
        // A proxy that wants a password and got none it accepts.
        if chain.contains(where: { $0.domain == NSPOSIXErrorDomain && $0.code == Int(EAUTH) }) { return .auth }
        guard let failure = chain.first(where: { $0.domain == NSURLErrorDomain }) else {
            return .other(chain.first?.localizedDescription ?? "")
        }
        switch URLError.Code(rawValue: failure.code) {
        case .timedOut: return .timeout
        case .cannotConnectToHost: return .refused
        case .cannotFindHost, .dnsLookupFailed: return .notFound
        case .userAuthenticationRequired, .userCancelledAuthentication: return .auth
        default: return .other(failure.localizedDescription)
        }
    }
}
