import Foundation

/// An agent's drawn face: a pixel mushroom whose cap colour, spots and face
/// come from its id, so it looks the same everywhere. A port of
/// web/src/app/persona.tsx; Tests/MycoUITests/Fixtures/looks.json is made
/// from that file, and the two must agree.
public struct MushroomLook: Sendable, Hashable {
    public var cap: Int
    public var pattern: Int
    public var face: Int
    public var sprout: Bool

    public static let capColors: [UInt32] = [
        0x3FA37A, 0xE0675A, 0xE7A33E, 0x5B8DEF, 0x9B6BD6, 0xE46AA5, 0x33AEBF, 0x8A9A3C, 0xD9824A, 0x6F7FD8,
    ]

    /// The cap (rows 0-5), with spots (o) in one of three patterns.
    static let capPatterns: [[String]] = [
        ["....CCCC....", "..CCCCCCCC..", ".CCoCCCCoCC.", "CCCCCCCCCCCC", "CCCCCoCCCCCC", ".CCCCCCCCCC."],
        ["....CCCC....", "..CCCoCCCC..", ".CCCCCCCCoC.", "CoCCCCCCCCCC", "CCCCCCCoCCCC", ".CCCCCCCCCC."],
        ["....CoCC....", "..CCCCCCCC..", ".CCCCoCCCCC.", "CCCCCCCCCCoC", "CCoCCCCCCCCC", ".CCCCCCCCCC."],
    ]

    /// The stem and face (rows 6-11): eyes (e), glasses (g), blush (b).
    static let faces: [[String]] = [
        ["...SSSSSS...", "...SeSSeS...", "...SSSSSS...", "...SSSSSS...", "...SS..SS...", "............"],
        ["...SSSSSS...", "..ggggggggg.", "...SeSSeS...", "...SSSSSS...", "...SS..SS...", "............"],
        ["...SSSSSS...", "...SeSSeS...", "...bSSSSb...", "...SSSSSS...", "...SS..SS...", "............"],
        ["...SSSSSS...", "...SSSSSS...", "...SeSSeS...", "...SSSSSS...", "...SS..SS...", "............"],
    ]

    public init(cap: Int, pattern: Int, face: Int, sprout: Bool) {
        self.cap = cap
        self.pattern = pattern
        self.face = face
        self.sprout = sprout
    }

    /// The default face of an id.
    public init(seed: String) {
        let h = Self.hash(seed.isEmpty ? "?" : seed)
        cap = Int(h % UInt32(Self.capColors.count))
        pattern = Int((h >> 8) % UInt32(Self.capPatterns.count))
        face = Int((h >> 12) % UInt32(Self.faces.count))
        // a sprout on top, now and then
        sprout = (h >> 16) % 4 == 0
    }

    /// A chosen face ("look:c3-p1-f2-s1"), or nil when `avatar` is not one.
    public init?(avatar: String?) {
        guard let avatar, avatar.hasPrefix("look:") else { return nil }
        let parts = avatar.dropFirst("look:".count).split(separator: "-", omittingEmptySubsequences: false)
        guard parts.count == 4 else { return nil }
        func number(_ part: Substring, _ letter: Character) -> Int? {
            guard part.first == letter else { return nil }
            let digits = part.dropFirst()
            guard !digits.isEmpty, digits.allSatisfy({ $0.isASCII && $0.isNumber }) else { return nil }
            return Int(digits)
        }
        guard let cap = number(parts[0], "c"), let pattern = number(parts[1], "p"), let face = number(parts[2], "f"),
              parts[3] == "s0" || parts[3] == "s1",
              cap < Self.capColors.count, pattern < Self.capPatterns.count, face < Self.faces.count
        else { return nil }
        self.init(cap: cap, pattern: pattern, face: face, sprout: parts[3] == "s1")
    }

    /// The chosen face when there is one, else the one from the id.
    public init(seed: String, avatar: String?) {
        self = MushroomLook(avatar: avatar) ?? MushroomLook(seed: seed)
    }

    /// The word a hub keeps for this face.
    public var word: String {
        "look:c\(cap)-p\(pattern)-f\(face)-s\(sprout ? 1 : 0)"
    }

    public var capColor: UInt32 { Self.capColors[cap] }

    /// The twelve rows of the drawing, top to bottom.
    public var rows: [String] { Self.capPatterns[pattern] + Self.faces[face] }

    /// FNV-1a over UTF-16 code units, as the desktop app computes it.
    static func hash(_ s: String) -> UInt32 {
        var h: UInt32 = 2_166_136_261
        for unit in s.utf16 {
            h ^= UInt32(unit)
            h = h &* 16_777_619
        }
        return h
    }
}
