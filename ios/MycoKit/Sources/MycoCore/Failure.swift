import Foundation
import MycoAPI

/// Why something asked of the workspace did not work, for a person: what
/// happened in words, what to do, and the raw reason for whoever wants to
/// see it or pass it on. A workspace that is down or slow is said in
/// words ("context deadline exceeded" means nothing to a person); so is
/// one too old for what was asked. Nothing here promises a retry that
/// nobody makes. The desktop app's `ErrorText`.
public struct Failure: Sendable, Hashable {
    public enum Kind: Sendable, Hashable {
        /// Not reached, or too slow: a later try may well work.
        case transient
        /// The workspace is older than the app and lacks this.
        case workspaceTooOld
        /// Anything else: said as the workspace said it.
        case other
    }

    public var kind: Kind
    /// What happened.
    public var words: String
    /// What to do about it, when there is something.
    public var advice: String?
    /// The raw reason (status, code, message), when the words replaced it.
    public var detail: String?

    public init(_ words: String, advice: String? = nil, detail: String? = nil, kind: Kind = .other) {
        self.kind = kind
        self.words = words
        self.advice = advice
        self.detail = detail
    }

    /// Both lines as one, where there is room for one string only.
    public var text: String {
        [words, advice ?? ""].filter { !$0.isEmpty }.joined(separator: L("", " "))
    }

    /// - Parameter canRetry: the page offers a button to try again.
    public static func of(_ error: any Error, canRetry: Bool = false) -> Failure {
        guard let hub = error as? HubError else { return Failure(error.localizedDescription) }
        if isTooOld(hub) {
            return Failure(
                L("你的工作区版本有点旧，还不支持这个功能。", "This workspace runs an older version that does not support this yet."),
                advice: L("升级工作区后再试。", "Upgrade the workspace, then try again."),
                detail: detail(of: hub), kind: .workspaceTooOld)
        }
        if isTransient(hub) {
            let rest = L("一直这样的话，看看手机的网络，或者到设置 ›「代理」里看看。", "If it keeps happening, check this phone's network, or look at Settings › Proxy.")
            return Failure(
                L("暂时连不上工作区，或者它响应太慢。", "Cannot reach the workspace right now, or it is answering too slowly."),
                advice: (canRetry ? L("点「重试」再试一次。", "Tap Retry to try again. ") : L("等一下再试。", "Try again in a moment. ")) + rest,
                detail: detail(of: hub), kind: .transient)
        }
        return Failure(hub.message, advice: hub.suggestion)
    }

    /// Not reached or too slow.
    public static func isTransient(_ error: HubError) -> Bool {
        if ["unreachable", "unavailable", "timeout"].contains(error.error) { return true }
        if let status = error.status, [502, 503, 504].contains(status) { return true }
        return error.message.range(
            of: "deadline exceeded|timed? ?out|connection (refused|reset)|no such host|EOF", options: [.regularExpression, .caseInsensitive]) != nil
    }

    /// An unknown path on an older workspace answers 404 ("no route
    /// for …", or a plain "404 page not found") or 405. A real "agent x
    /// not found" is not this.
    public static func isTooOld(_ error: HubError) -> Bool {
        if error.status == 405 || error.error == "http_405" { return true }
        guard error.status == 404 || error.error == "http_404" else { return false }
        return error.error == "http_404" || error.message.hasPrefix("no route for") || error.message.contains("404 page not found")
    }

    /// "HTTP 502 · unavailable" and the message under it.
    static func detail(of error: HubError) -> String {
        let code = error.error.hasPrefix("http_") ? "" : error.error
        let head = [error.status.map { "HTTP \($0)" } ?? "", code].filter { !$0.isEmpty }.joined(separator: " · ")
        if head.isEmpty { return error.message }
        return error.message.isEmpty || error.message == head ? head : head + "\n" + error.message
    }
}
