import Foundation

/// End-to-end encrypted bodies (docs/design/e2e-encryption.md). Sealing and
/// opening come with the pairing phase; for now this only tells a sealed
/// body from plain text, so a page can say "encrypted" instead of showing
/// the envelope.
public enum Envelope {
    public static let prefix = "myco-e2e:v1:"

    public static func isSealed(_ body: String) -> Bool {
        body.hasPrefix(prefix)
    }
}
