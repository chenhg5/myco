import Foundation
import Testing
@testable import MycoUI

/// The faces drawn here must be the ones the desktop app draws, pixel for
/// pixel. The fixture is made from web/src/app/persona.tsx by
/// ios/scripts/gen-look-fixtures.mjs.
@Suite(.serialized)
struct LookTests {
    struct Parts: Decodable, Equatable {
        var cap: Int
        var pattern: Int
        var face: Int
        var shape: Int
        var acc: Int
    }

    struct Fixture: Decodable {
        struct Seed: Decodable {
            var seed: String
            var cap: Int
            var pattern: Int
            var face: Int
            var shape: Int
            var acc: Int
            var word: String
            var color: String
            var grid: [[String?]]
        }
        struct Avatar: Decodable {
            var avatar: String
            var look: Parts?
        }
        struct Drawn: Decodable {
            var avatar: String
            var grid: [[String?]]
        }
        struct Member: Decodable {
            var id: String
            var created_at: String
            var avatar: String?
            var cap: Int
            var pattern: Int
            var face: Int
            var shape: Int
            var acc: Int
            var grid: [[String?]]?
        }
        var seeds: [Seed]
        var avatars: [Avatar]
        var drawn: [Drawn]
        var workspace: [Member]
    }

    static func fixture() throws -> Fixture {
        let url = try #require(Bundle.module.url(forResource: "looks", withExtension: "json", subdirectory: "Fixtures"))
        return try JSONDecoder().decode(Fixture.self, from: Data(contentsOf: url))
    }

    /// A drawing in the fixture's terms: a part's letter, an accessory's
    /// colour ("#rrggbb"), or nil.
    static func grid(_ look: MushroomLook) -> [[String?]] {
        look.pixels.map { row in
            row.map { paint in
                switch paint {
                case .part(let k): return String(k)
                case .color(let hex): return String(format: "#%06x", hex)
                case nil: return nil
                }
            }
        }
    }

    static func parts(_ look: MushroomLook) -> Parts {
        Parts(cap: look.cap, pattern: look.pattern, face: look.face, shape: look.shape, acc: look.acc)
    }

    @Test func facesFromIDsMatchTheDesktopApp() throws {
        let fixture = try Self.fixture()
        #expect(fixture.seeds.count >= 40)
        for expected in fixture.seeds {
            let look = MushroomLook(seed: expected.seed)
            let want = Parts(cap: expected.cap, pattern: expected.pattern, face: expected.face, shape: expected.shape, acc: expected.acc)
            #expect(Self.parts(look) == want, "seed \(expected.seed)")
            #expect(look.word == expected.word)
            #expect(String(format: "#%06x", look.capColor) == expected.color)
            #expect(Self.grid(look) == expected.grid, "seed \(expected.seed)")
        }
    }

    @Test func chosenFacesParseAsOnTheDesktopApp() throws {
        for expected in try Self.fixture().avatars {
            let look = MushroomLook(avatar: expected.avatar)
            #expect(look.map(Self.parts) == expected.look, "avatar \(expected.avatar)")
            // and the word for it reads back the same
            if let look { #expect(MushroomLook(avatar: look.word) == look) }
        }
    }

    @Test func everyShapeAndAccessoryIsDrawnAsOnTheDesktopApp() throws {
        let drawn = try Self.fixture().drawn
        #expect(drawn.count == MushroomLook.shapes.count * MushroomLook.accessories.count)
        for expected in drawn {
            let look = try #require(MushroomLook(avatar: expected.avatar), "avatar \(expected.avatar)")
            #expect(Self.grid(look) == expected.grid, "avatar \(expected.avatar)")
        }
    }

    @Test func aWorkspaceSpreadsItsFacesAsTheDesktopAppDoes() throws {
        let workspace = try Self.fixture().workspace
        let dates = ISO8601DateFormatter()
        let members = try workspace.map {
            MushroomLook.Member(id: $0.id, createdAt: try #require(dates.date(from: $0.created_at)), avatar: $0.avatar)
        }
        MushroomLook.rememberWorkspaceFaces(members)
        defer { MushroomLook.assigned.removeAll() }
        var moved = 0
        for expected in workspace {
            let look = MushroomLook(seed: expected.id, avatar: expected.avatar)
            if expected.avatar?.hasPrefix("pic:") != true {
                let want = Parts(cap: expected.cap, pattern: expected.pattern, face: expected.face, shape: expected.shape, acc: expected.acc)
                #expect(Self.parts(look) == want, "agent \(expected.id)")
                #expect(Self.grid(look) == expected.grid, "agent \(expected.id)")
            }
            if Self.parts(look) != Self.parts(MushroomLook(seed: expected.id)) && MushroomLook(avatar: expected.avatar) == nil { moved += 1 }
        }
        // the fixture has clashes to spread
        #expect(moved > 0)
    }

    @Test func aNewcomerNeverChangesAnOlderFace() {
        let start = Date(timeIntervalSince1970: 1_800_000_000)
        let older = (0..<12).map { MushroomLook.Member(id: "agt_old\($0)", createdAt: start.addingTimeInterval(Double($0)), avatar: nil) }
        let before = MushroomLook.spread(older)
        let after = MushroomLook.spread(older + [MushroomLook.Member(id: "agt_new", createdAt: start.addingTimeInterval(100), avatar: nil)])
        for m in older { #expect(before[m.id] == after[m.id]) }
    }

    @Test func anOldLookKeepsTheDomeAndItsSprout() throws {
        let old = try #require(MushroomLook(avatar: "look:c3-p1-f2-s1"))
        #expect(old.shape == 0 && old.acc == 1 && old.sprout)
        #expect(old.word == "look:c3-p1-f2-s1")
        let bare = try #require(MushroomLook(avatar: "look:c3-p1-f2-s0"))
        #expect(bare.shape == 0 && bare.acc == 0)
        // a newer one is written with its shape and what it wears
        #expect(MushroomLook(cap: 3, pattern: 1, face: 2, shape: 2, acc: 5).word == "look:c3-p1-f2-s0-h2-a5")
    }

    @Test func everyFaceIsTwelveByFourteen() {
        for shape in MushroomLook.shapes.indices {
            for acc in MushroomLook.accessories.indices {
                for pattern in MushroomLook.spots.indices {
                    for face in MushroomLook.faces.indices {
                        let pixels = MushroomLook(cap: 0, pattern: pattern, face: face, shape: shape, acc: acc).pixels
                        #expect(pixels.count == 14)
                        #expect(pixels.allSatisfy { $0.count == 12 })
                    }
                }
            }
        }
    }
}
