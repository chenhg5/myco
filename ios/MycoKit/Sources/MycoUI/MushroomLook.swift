import Foundation

/// An agent's drawn face: a pixel mushroom whose cap colour, spots, face,
/// cap shape and what it wears come from its id, so it looks the same
/// everywhere. A port of web/src/app/persona.tsx; the fixture
/// Tests/MycoUITests/Fixtures/looks.json is made from that file, and the
/// two must agree pixel for pixel.
public struct MushroomLook: Sendable, Hashable {
    public var cap: Int
    public var pattern: Int
    public var face: Int
    /// The cap's outline: 0 is the dome every face had before shapes came.
    public var shape: Int
    /// What it wears: 0 nothing, 1 a sprout, then the rest of `accessories`.
    public var acc: Int

    /// Whether it wears a sprout (the one accessory from before shapes).
    public var sprout: Bool { acc == 1 }

    public static let capColors: [UInt32] = [
        0x3FA37A, 0xE0675A, 0xE7A33E, 0x5B8DEF, 0x9B6BD6, 0xE46AA5, 0x33AEBF, 0x8A9A3C, 0xD9824A, 0x6F7FD8,
    ]

    /// The cap's outline (rows y = -2..5, 12 wide): six shapes that tell
    /// apart even at 20pt.
    static let shapes: [[String]] = [
        // dome
        ["............", "............", "....CCCC....", "..CCCCCCCC..", ".CCCCCCCCCC.", "CCCCCCCCCCCC", "CCCCCCCCCCCC", ".CCCCCCCCCC."],
        // cone: a pointed top
        [".....CC.....", "....CCCC....", "....CCCC....", "...CCCCCC...", "..CCCCCCCC..", ".CCCCCCCCCC.", "CCCCCCCCCCCC", ".CCCCCCCCCC."],
        // flat: wide and low
        ["............", "............", "............", "............", "............", ".CCCCCCCCCC.", "CCCCCCCCCCCC", "CCCCCCCCCCCC"],
        // bell: tall and narrow
        ["............", "....CCCC....", "...CCCCCC...", "...CCCCCC...", "...CCCCCC...", "..CCCCCCCC..", "..CCCCCCCC..", "..CCCCCCCC.."],
        // frill: a dome with a scalloped edge
        ["............", "............", "....CCCC....", "..CCCCCCCC..", ".CCCCCCCCCC.", "CCCCCCCCCCCC", "CCCCCCCCCCCC", "C.CC.CC.CC.C"],
        // brim: a small crown on a wide brim
        ["............", "............", "............", "....CCCC....", "...CCCCCC...", "...CCCCCC...", "CCCCCCCCCCCC", ".CCCCCCCCCC."],
    ]

    /// Where accessories sit on each shape: its top, left and right shoulders.
    struct Anchors {
        var top: (Int, Int)
        var left: (Int, Int)
        var right: (Int, Int)
    }

    static let anchors: [Anchors] = [
        Anchors(top: (6, 0), left: (2, 1), right: (9, 1)),
        Anchors(top: (6, -2), left: (3, 1), right: (8, 1)),
        Anchors(top: (6, 3), left: (1, 3), right: (10, 3)),
        Anchors(top: (6, -1), left: (3, 0), right: (8, 0)),
        Anchors(top: (6, 0), left: (2, 1), right: (9, 1)),
        Anchors(top: (6, 1), left: (4, 1), right: (7, 1)),
    ]

    /// The spots, as (x, y) on the cap: one of three patterns; a spot falls
    /// only where the shape has cap.
    static let spots: [[(Int, Int)]] = [
        [(3, 2), (8, 2), (5, 4)],
        [(5, 1), (9, 2), (1, 3), (7, 4)],
        [(5, 0), (5, 2), (10, 3), (2, 4)],
    ]

    /// The stem and face (rows 6-11): eyes (e), glasses (g), blush (b); the
    /// stem's right edge in shade (T), so it stands out from a pale tile.
    static let faces: [[String]] = [
        ["...SSSSST...", "...SeSSeT...", "...SSSSST...", "...SSSSST...", "...SS..ST...", "............"],
        ["...SSSSST...", "..ggggggggg.", "...SeSSeT...", "...SSSSST...", "...SS..ST...", "............"],
        ["...SSSSST...", "...SeSSeT...", "...bSSSSb...", "...SSSSST...", "...SS..ST...", "............"],
        ["...SSSSST...", "...SSSSST...", "...SeSSeT...", "...SSSSST...", "...SS..ST...", "............"],
    ]

    /// What it can wear (one at most), with the shapes each one fits (a
    /// sprout needs room on top, headphones a dome); nil fits all.
    public static let accessories: [(name: String, fits: [Int]?)] = [
        ("none", nil), ("sprout", [0, 2, 4, 5]), ("leaf", nil), ("bow", nil),
        ("headphones", [0, 4]), ("scarf", nil), ("star", nil), ("flower", nil),
    ]

    /// Whether an accessory can be worn on a shape.
    public static func fits(_ acc: Int, _ shape: Int) -> Bool {
        guard accessories.indices.contains(acc) else { return true }
        return accessories[acc].fits.map { $0.contains(shape) } ?? true
    }

    public init(cap: Int, pattern: Int, face: Int, shape: Int = 0, acc: Int = 0) {
        self.cap = cap
        self.pattern = pattern
        self.face = face
        self.shape = shape
        self.acc = acc
    }

    /// A dome face from before shapes: a sprout or nothing.
    public init(cap: Int, pattern: Int, face: Int, sprout: Bool) {
        self.init(cap: cap, pattern: pattern, face: face, shape: 0, acc: sprout ? 1 : 0)
    }

    /// The default face of an id. The colour, spots and face are what they
    /// always were; the shape and what it wears come from a second hash, so
    /// they vary apart from the colour.
    public init(seed: String) {
        let s = seed.isEmpty ? "?" : seed
        let h = Self.hash(s)
        let h2 = Self.hash("\u{1}" + s)
        let shape = Int(h2 % UInt32(Self.shapes.count))
        let fit = Self.accessories.indices.filter { $0 > 0 && Self.fits($0, shape) }
        // about one in four wears nothing
        let k = Int((h2 >> 8) % UInt32(fit.count + 2))
        self.init(
            cap: Int(h % UInt32(Self.capColors.count)), pattern: Int((h >> 8) % UInt32(Self.spots.count)),
            face: Int((h >> 12) % UInt32(Self.faces.count)), shape: shape, acc: k < fit.count ? fit[k] : 0)
    }

    /// A chosen face, or nil when `avatar` is not one. "look:c3-p1-f2-s1"
    /// is one chosen before shapes: the dome, a sprout or nothing, as it
    /// always was; newer ones add "-h<shape>-a<accessory>".
    public init?(avatar: String?) {
        guard let avatar, avatar.hasPrefix("look:") else { return nil }
        let parts = avatar.dropFirst("look:".count).split(separator: "-", omittingEmptySubsequences: false)
        guard parts.count == 4 || parts.count == 6 else { return nil }
        func number(_ part: Substring, _ letter: Character) -> Int? {
            guard part.first == letter else { return nil }
            let digits = part.dropFirst()
            guard !digits.isEmpty, digits.allSatisfy({ $0.isASCII && $0.isNumber }) else { return nil }
            return Int(digits)
        }
        guard let cap = number(parts[0], "c"), let pattern = number(parts[1], "p"), let face = number(parts[2], "f"),
              parts[3] == "s0" || parts[3] == "s1",
              cap < Self.capColors.count, pattern < Self.spots.count, face < Self.faces.count
        else { return nil }
        var shape = 0
        var acc = parts[3] == "s1" ? 1 : 0
        if parts.count == 6 {
            guard let h = number(parts[4], "h"), let a = number(parts[5], "a") else { return nil }
            shape = h
            acc = a
        }
        guard shape < Self.shapes.count, acc < Self.accessories.count else { return nil }
        self.init(cap: cap, pattern: pattern, face: face, shape: shape, acc: acc)
    }

    /// The chosen face when there is one, else the workspace's pick for it
    /// (`rememberWorkspaceFaces`), else the one from the id.
    public init(seed: String, avatar: String?) {
        self = MushroomLook(avatar: avatar) ?? Self.assigned.look(of: seed) ?? MushroomLook(seed: seed)
    }

    /// The word a hub keeps for this face: as before for a dome with a
    /// sprout or nothing (older apps read it), else with its shape and what
    /// it wears.
    public var word: String {
        let base = "look:c\(cap)-p\(pattern)-f\(face)-s\(acc == 1 ? 1 : 0)"
        return shape == 0 && acc <= 1 ? base : "\(base)-h\(shape)-a\(acc)"
    }

    public var capColor: UInt32 { Self.capColors[cap] }

    /// One pixel of the drawing: a part of the mushroom (C cap, o spot,
    /// S stem, T stem in shade, e eyes, g glasses, b blush), or an
    /// accessory's own colour.
    public enum Paint: Hashable, Sendable {
        case part(Character)
        case color(UInt32)
    }

    /// The drawing, 12 wide and 14 tall (y = -2 at the top), nil where it
    /// is clear: the cap with its spots, the stem and face, then what it
    /// wears (only when it fits the shape).
    public var pixels: [[Paint?]] {
        let outline = Self.shapes.indices.contains(shape) ? Self.shapes[shape] : Self.shapes[0]
        let spotted = Set(Self.spots[pattern].map { "\($0.0),\($0.1)" })
        var grid: [[Paint?]] = []
        for (i, row) in outline.enumerated() {
            grid.append(row.enumerated().map { x, k in
                guard k != "." else { return nil }
                return .part(k == "C" && spotted.contains("\(x),\(i - 2)") ? "o" : k)
            })
        }
        for row in Self.faces[face] {
            grid.append(row.map { $0 == "." ? nil : .part($0) })
        }
        for (x, y, color) in Self.accessoryPixels(Self.fits(acc, shape) ? acc : 0, shape)
        where (0..<12).contains(x) && (-2..<12).contains(y) {
            grid[y + 2][x] = .color(color)
        }
        return grid
    }

    /// What it wears, as (x, y, colour) pixels on its shape.
    static func accessoryPixels(_ acc: Int, _ shape: Int) -> [(Int, Int, UInt32)] {
        let a = anchors.indices.contains(shape) ? anchors[shape] : anchors[0]
        let (tx, ty) = a.top
        let (lx, ly) = a.left
        let (rx, ry) = a.right
        let green: UInt32 = 0x5AA86A
        guard accessories.indices.contains(acc) else { return [] }
        switch accessories[acc].name {
        case "sprout":
            return [(tx, ty - 2, green), (tx, ty - 1, green), (tx + 1, ty - 2, green)]
        case "leaf":
            return [(lx, ly - 1, 0x3F8F4F), (lx + 1, ly - 1, green), (lx - 1, ly, green), (lx, ly, 0x3F8F4F)]
        case "bow":
            let pink: UInt32 = 0xF0507A
            return [(rx - 1, ry - 1, pink), (rx + 1, ry - 1, pink), (rx - 1, ry, pink), (rx, ry, 0xB8325A), (rx + 1, ry, pink)]
        case "headphones":
            let ink: UInt32 = 0x3B3F4A
            return (2...9).map { ($0, -1, ink) } + [
                (1, 0, ink), (10, 0, ink), (0, 1, ink), (1, 1, ink), (0, 2, ink), (1, 2, ink),
                (10, 1, ink), (11, 1, ink), (10, 2, ink), (11, 2, ink),
            ]
        case "scarf":
            return (2...9).map { ($0, 9, 0xD9433F) } + [(9, 10, 0xD9433F), (9, 11, 0xB8322F)]
        case "star":
            let gold: UInt32 = 0xF2B632
            return [(10, -2, gold), (9, -1, gold), (10, -1, 0xFFF3C4), (11, -1, gold), (10, 0, gold)]
        case "flower":
            let (fx, fy) = (lx + 1, ly)
            let white: UInt32 = 0xFFFFFF
            return [(fx, fy - 1, white), (fx - 1, fy, white), (fx + 1, fy, white), (fx, fy + 1, white), (fx, fy, 0xF2B632)]
        default:
            return []
        }
    }

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

// MARK: Faces in a workspace

extension MushroomLook {
    /// An agent as `rememberWorkspaceFaces` needs it.
    public struct Member: Sendable {
        public var id: String
        public var createdAt: Date
        public var avatar: String?

        public init(id: String, createdAt: Date, avatar: String?) {
            self.id = id
            self.createdAt = createdAt
            self.avatar = avatar
        }
    }

    /// Spreads the faces drawn from ids in one workspace: agents in the
    /// order they joined (so a newcomer never changes an older one's face),
    /// each keeping its own face unless an earlier one already has its
    /// colour and shape; then it takes the next free shape, else the next
    /// free colour. Chosen faces count as taken and stay as they are.
    /// Returns the agents whose face moved, by id.
    public static func spread(_ members: [Member]) -> [String: MushroomLook] {
        // as the desktop sorts: by when they joined, then by id
        let en = Locale(identifier: "en")
        let byAge = members.sorted {
            $0.createdAt != $1.createdAt ? $0.createdAt < $1.createdAt
                : $0.id.compare($1.id, locale: en) == .orderedAscending
        }
        var taken = Set<Int>()
        func key(_ cap: Int, _ shape: Int) -> Int { cap * shapes.count + shape }
        var moved: [String: MushroomLook] = [:]
        for m in byAge {
            if let chosen = MushroomLook(avatar: m.avatar) {
                taken.insert(key(chosen.cap, chosen.shape))
                continue
            }
            if m.avatar?.hasPrefix("pic:") == true { continue }
            let own = MushroomLook(seed: m.id)
            var pick = own
            if taken.contains(key(own.cap, own.shape)) {
                search: for dc in capColors.indices {
                    for ds in shapes.indices {
                        let shape = (own.shape + ds) % shapes.count
                        let cap = (own.cap + dc) % capColors.count
                        if !taken.contains(key(cap, shape)) {
                            pick.cap = cap
                            pick.shape = shape
                            pick.acc = fits(own.acc, shape) ? own.acc : 0
                            break search
                        }
                    }
                }
            }
            taken.insert(key(pick.cap, pick.shape))
            moved[m.id] = pick == own ? nil : pick
        }
        return moved
    }

    /// Remembers the faces of one workspace's agents (see `spread`), so
    /// `init(seed:avatar:)` draws them spread out. Call it whenever the
    /// workspace's agents are loaded.
    public static func rememberWorkspaceFaces(_ members: [Member]) {
        let moved = spread(members)
        assigned.update(members.map(\.id), moved)
    }

    /// The workspace's picks, by agent id.
    static let assigned = Assigned()

    final class Assigned: @unchecked Sendable {
        private let lock = NSLock()
        private var looks: [String: MushroomLook] = [:]

        func look(of id: String) -> MushroomLook? {
            lock.lock()
            defer { lock.unlock() }
            return looks[id]
        }

        func update(_ ids: [String], _ moved: [String: MushroomLook]) {
            lock.lock()
            defer { lock.unlock() }
            for id in ids { looks[id] = moved[id] }
        }

        func removeAll() {
            lock.lock()
            defer { lock.unlock() }
            looks = [:]
        }
    }
}
