import Foundation

/// The hub's structured error (any non-2xx answer), or a failure to reach it.
public struct HubError: Error, Codable, Sendable, Hashable {
    /// `invalid_argument`, `unauthenticated`, `permission_denied`,
    /// `not_found`, `conflict`, `invalid_state`, `internal`, or `unreachable`
    /// (made here, when the hub did not answer).
    public var error: String
    public var message: String
    public var suggestion: String?
    public var retryable: Bool?
    /// The HTTP status it came with, when it came from the hub (or from
    /// something in front of it).
    public var status: Int?

    public init(error: String, message: String, suggestion: String? = nil, retryable: Bool? = nil, status: Int? = nil) {
        self.error = error
        self.message = message
        self.suggestion = suggestion
        self.retryable = retryable
        self.status = status
    }

    public static func unreachable(_ message: String) -> HubError {
        HubError(error: "unreachable", message: message, retryable: true)
    }
}
