import Foundation

/// How the hub's JSON is read and written: snake_case keys, and RFC 3339
/// times with or without fractional seconds (Go writes nanoseconds).
public enum HubJSON {
    public static func decoder() -> JSONDecoder {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        d.dateDecodingStrategy = .custom { decoder in
            let raw = try decoder.singleValueContainer().decode(String.self)
            if let date = parseDate(raw) { return date }
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "not an RFC 3339 time: \(raw)"))
        }
        return d
    }

    public static func encoder() -> JSONEncoder {
        let e = JSONEncoder()
        e.keyEncodingStrategy = .convertToSnakeCase
        e.dateEncodingStrategy = .custom { date, encoder in
            var c = encoder.singleValueContainer()
            // With milliseconds: what is kept between launches must sort
            // as it did, and two things in the same second stay apart.
            try c.encode(Date.ISO8601FormatStyle(includingFractionalSeconds: true).format(date))
        }
        return e
    }

    /// Parses an RFC 3339 time. Fractions longer than milliseconds are cut
    /// to milliseconds first: the system parser does not take nine digits.
    public static func parseDate(_ raw: String) -> Date? {
        if let dot = raw.firstIndex(of: ".") {
            let afterDot = raw[raw.index(after: dot)...]
            let digits = afterDot.prefix { $0.isNumber }
            let zone = afterDot.dropFirst(digits.count)
            let millis = String(digits.prefix(3)).padding(toLength: 3, withPad: "0", startingAt: 0)
            let trimmed = raw[..<dot] + "." + millis + zone
            return try? Date.ISO8601FormatStyle(includingFractionalSeconds: true).parse(String(trimmed))
        }
        return try? Date.ISO8601FormatStyle().parse(raw)
    }
}
