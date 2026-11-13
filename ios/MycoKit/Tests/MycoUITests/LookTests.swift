import Foundation
import Testing
@testable import MycoUI

/// The faces drawn here must be the ones the desktop app draws. The
/// fixture is made from web/src/app/persona.tsx by
/// ios/scripts/gen-look-fixtures.mjs.
struct LookTests {
    struct Fixture: Decodable {
        struct Seed: Decodable {
            var seed: String
            var cap: Int
            var pattern: Int
            var face: Int
            var sprout: Bool
            var word: String
            var color: String
        }
        struct Parts: Decodable {
            var cap: Int
            var pattern: Int
            var face: Int
            var sprout: Bool
        }
        struct Avatar: Decodable {
            var avatar: String
            var look: Parts?
        }
        var seeds: [Seed]
        var avatars: [Avatar]
    }

    static func fixture() throws -> Fixture {
        let url = try #require(Bundle.module.url(forResource: "looks", withExtension: "json", subdirectory: "Fixtures"))
        return try JSONDecoder().decode(Fixture.self, from: Data(contentsOf: url))
    }

    @Test func facesFromIDsMatchTheDesktopApp() throws {
        let fixture = try Self.fixture()
        #expect(fixture.seeds.count >= 40)
        for expected in fixture.seeds {
            let look = MushroomLook(seed: expected.seed)
            #expect(look == MushroomLook(cap: expected.cap, pattern: expected.pattern, face: expected.face, sprout: expected.sprout), "seed \(expected.seed)")
            #expect(look.word == expected.word)
            #expect(String(format: "#%06x", look.capColor) == expected.color)
        }
    }

    @Test func chosenFacesParseAsOnTheDesktopApp() throws {
        for expected in try Self.fixture().avatars {
            let look = MushroomLook(avatar: expected.avatar)
            let want = expected.look.map { MushroomLook(cap: $0.cap, pattern: $0.pattern, face: $0.face, sprout: $0.sprout) }
            #expect(look == want, "avatar \(expected.avatar)")
        }
    }

    @Test func everyFaceIsTwelveByTwelve() {
        for cap in MushroomLook.capColors.indices {
            for pattern in MushroomLook.capPatterns.indices {
                for face in MushroomLook.faces.indices {
                    let rows = MushroomLook(cap: cap, pattern: pattern, face: face, sprout: false).rows
                    #expect(rows.count == 12)
                    #expect(rows.allSatisfy { $0.count == 12 })
                }
            }
        }
    }
}
