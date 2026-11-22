import Foundation
import Network
import MycoAPI

/// Web pages agents publish to the workspace (`myco page publish`). A link
/// to one, `<workspace>/pages/<id>`, shows under its message as a card,
/// and opens in the app. The desktop app's `pages.tsx`.
public enum PageLinks {
    /// A workspace page and its opaque, percent-encoded client-side location.
    public struct Target: Hashable, Sendable {
        public let id: String
        public let fragment: String?
    }

    public static let maximumFragmentBytes = 1024

    /// Fragments never become a hub query or file path. Oversized ones are
    /// ignored, while the page itself still opens.
    public static func boundedFragment(_ fragment: String?) -> String? {
        guard let fragment, !fragment.isEmpty, fragment.utf8.count <= maximumFragmentBytes,
              let encoded = URLComponents(string: "#" + fragment)?.percentEncodedFragment,
              encoded.utf8.count <= maximumFragmentBytes else { return nil }
        return encoded
    }

    public static func target(of href: String, in workspace: URL?) -> Target? {
        guard let workspace else { return nil }
        let parts = href.trimmingCharacters(in: .whitespacesAndNewlines).split(separator: "#", maxSplits: 1, omittingEmptySubsequences: false)
        let link = String(parts[0])
        guard let match = link.range(of: "/pages/pg_[a-z0-9]+/?$", options: [.regularExpression, .caseInsensitive]) else { return nil }
        let base = String(link[..<match.lowerBound])
        guard same(base, workspace.absoluteString),
              let id = link[match].split(separator: "/").last.map(String.init) else { return nil }
        return Target(id: id, fragment: boundedFragment(parts.count > 1 ? String(parts[1]) : nil))
    }

    /// The page a link leads to, when it is a page of this workspace.
    public static func id(of href: String, in workspace: URL?) -> String? {
        target(of: href, in: workspace)?.id
    }

    private static func same(_ a: String, _ b: String) -> Bool {
        func trimmed(_ text: String) -> String {
            var text = text.lowercased()
            while text.hasSuffix("/") { text.removeLast() }
            return text
        }
        return trimmed(a) == trimmed(b)
    }

    /// The pages linked in a message, once per page, in order. A card keeps
    /// the first link's fragment, just as tapping that link would.
    public static func targets(in body: String, workspace: URL?) -> [Target] {
        guard workspace != nil, body.contains("/pages/pg_") else { return [] }
        guard let pattern = try? NSRegularExpression(pattern: "https?://[^\\s<>()\"'`，。；、]+") else { return [] }
        var targets: [Target] = []
        for match in pattern.matches(in: body, range: NSRange(body.startIndex..., in: body)) {
            guard let range = Range(match.range, in: body) else { continue }
            let link = String(body[range]).replacingOccurrences(of: "[.,;:!?)\\]]+$", with: "", options: .regularExpression)
            if let target = target(of: link, in: workspace), !targets.contains(where: { $0.id == target.id }) { targets.append(target) }
        }
        return targets
    }

    public static func ids(in body: String, workspace: URL?) -> [String] {
        targets(in: body, workspace: workspace).map(\.id)
    }

    /// Only the hub's public page address may be exposed to page scripts.
    /// Never substitute the app's local transport address or carry credentials.
    public static func publicURL(of page: Page) -> String? {
        guard let text = page.url, var url = URLComponents(string: text),
              let scheme = url.scheme?.lowercased(), ["https", "http"].contains(scheme),
              let host = url.host?.lowercased(), !host.isEmpty,
              url.user == nil, url.password == nil else { return nil }
        let name = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]."))
        guard name != "localhost", !name.hasSuffix(".localhost") else { return nil }
        if let address = IPv4Address(name) {
            let bytes = [UInt8](address.rawValue)
            if bytes[0] == 127 || bytes.allSatisfy({ $0 == 0 }) { return nil }
        }
        if let address = IPv6Address(name) {
            let bytes = [UInt8](address.rawValue)
            let loopback = bytes.dropLast().allSatisfy { $0 == 0 } && bytes[15] == 1
            let mappedLoopback = bytes.prefix(10).allSatisfy { $0 == 0 }
                && bytes[10] == 255 && bytes[11] == 255 && bytes[12] == 127
            if loopback || mappedLoopback || bytes.allSatisfy({ $0 == 0 }) { return nil }
        }
        let path = url.path.hasSuffix("/") ? String(url.path.dropLast()) : url.path
        guard path.hasSuffix("/pages/" + page.id) else { return nil }
        url.query = nil
        url.fragment = nil
        return url.string
    }

    /// What a page is loaded as inside the app: a scheme of its own, one
    /// origin per page, so pages share nothing with each other or the app.
    public static let scheme = "myco-page"

    /// A page's name as a host: its id without the underscore, which a
    /// host may not have (pg_abc → pg-abc).
    public static func host(of id: String) -> String {
        id.lowercased().replacingOccurrences(of: "_", with: "-")
    }

    public static func address(of id: String, path: String = "", fragment: String? = nil) -> URL? {
        let base = "\(scheme)://\(host(of: id))/" + (path.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? "")
        return URL(string: base + (boundedFragment(fragment).map { "#" + $0 } ?? ""))
    }

    /// Whether an address inside the app is this page's own.
    public static func isOwn(_ url: URL, of id: String) -> Bool {
        url.scheme == scheme && url.host == host(of: id)
    }

    /// What every file of a page is served with: a page loads only its
    /// own files (and data:, blob:), and cannot send anything elsewhere.
    /// The desktop app's `pageCSP`.
    public static let contentPolicy =
        "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; "
        + "style-src 'self' 'unsafe-inline' data: blob:; img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' data: blob:; "
        + "connect-src 'self' data: blob:; worker-src 'self' blob:; frame-src 'self' data: blob:; form-action 'self'; base-uri 'self'; object-src 'none'"
}

/// What a page's card and title say of it.
public enum PageWords {
    /// "第 2 版 · 由 测试 发布 · 18 分钟前"; a page with one version does
    /// not say which; a live one says whether it is being served.
    public static func line(_ page: Page, publisher: String, now: Date = Date()) -> String {
        var parts: [String] = []
        if page.live == true {
            parts.append(page.online == true ? L("实时网页", "Live page") : L("实时网页 · 现在没有在转发", "Live page · not being served now"))
        } else if page.version > 1 {
            parts.append(L("第 \(page.version) 版", "Version \(page.version)"))
        }
        parts.append(L("由 \(publisher) 发布", "Published by \(publisher)"))
        if let at = page.updatedAt ?? page.createdAt { parts.append(RelativeTime.short(at, now: now)) }
        return parts.joined(separator: " · ")
    }
}

/// The pages a workspace's messages link to, as far as they were asked for.
@MainActor
@Observable
public final class PagesModel {
    private let hub: any HubAPI
    /// The pages that could be read.
    public private(set) var pages: [String: Page] = [:]
    /// Asked for and not there (gone, not mine to see, an older workspace):
    /// no card, the link stays a link.
    private var missing: [String: Date] = [:]
    private var asked: [String: Date] = [:]
    private var fetching: Set<String> = []

    /// A page read this long ago is read again when it is asked for (it
    /// may have a newer version).
    static let freshFor: TimeInterval = 20

    public init(hub: any HubAPI) {
        self.hub = hub
    }

    public var workspace: URL? { hub.address }

    /// The pages linked in a text.
    public func ids(in text: String) -> [String] {
        PageLinks.ids(in: text, workspace: hub.address)
    }

    /// Reads the pages that are not known yet, or were read a while ago.
    public func need(_ ids: [String], now: Date = Date()) async {
        let wanted = ids.filter { id in
            guard !fetching.contains(id) else { return false }
            return asked[id].map { now.timeIntervalSince($0) > Self.freshFor } ?? true
        }
        guard !wanted.isEmpty else { return }
        for id in wanted {
            fetching.insert(id)
            asked[id] = now
        }
        await withTaskGroup(of: (String, Page?, Bool).self) { group in
            let hub = hub
            for id in wanted {
                group.addTask {
                    do {
                        return (id, try await hub.page(id), false)
                    } catch let error as HubError where error.error == "not_found" || error.error == "permission_denied" || error.status == 404 {
                        return (id, nil, true)
                    } catch {
                        // Not reachable now: what is known stays.
                        return (id, nil, false)
                    }
                }
            }
            for await (id, page, gone) in group {
                fetching.remove(id)
                if let page {
                    if pages[id] != page { pages[id] = page }
                    missing[id] = nil
                } else if gone {
                    pages[id] = nil
                    missing[id] = now
                }
            }
        }
    }

    /// Reads one page now, whatever is known: its latest version.
    @discardableResult
    public func read(_ id: String) async -> Page? {
        asked[id] = nil
        await need([id])
        return pages[id]
    }

    /// One file of a page, for the page's own view.
    public func file(of id: String, version: Int?, path: String) async throws -> PageFile {
        try await hub.pageFile(id, version: version, path: path)
    }
}
