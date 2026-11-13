import Foundation

/// A list that also reads as empty when the hub sends `null` or leaves the
/// field out (a nil Go slice, or a hub from before the field existed).
@propertyWrapper
public struct LossyList<Element: Codable & Sendable & Hashable>: Codable, Sendable, Hashable {
    public var wrappedValue: [Element]

    public init(wrappedValue: [Element] = []) {
        self.wrappedValue = wrappedValue
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        wrappedValue = c.decodeNil() ? [] : try c.decode([Element].self)
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        try c.encode(wrappedValue)
    }
}

extension KeyedDecodingContainer {
    /// Synthesized decoding lands here for `@LossyList` properties, so a
    /// missing key is an empty list, not an error.
    func decode<Element>(_ type: LossyList<Element>.Type, forKey key: Key) throws -> LossyList<Element> {
        try decodeIfPresent(type, forKey: key) ?? LossyList()
    }
}
