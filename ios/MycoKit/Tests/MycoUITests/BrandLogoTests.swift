import SwiftUI
import Testing
@testable import MycoUI

/// The logo drawn here is the one in docs/assets/logo: its outlines fill
/// the files' frames, and the cut-outs are empty.
@MainActor
struct BrandLogoTests {
    static func outline(_ part: BrandLogo.Part) -> Path {
        var path = Path()
        for outline in BrandMark.outlines(of: part) { BrandMark.add(outline, to: &path) }
        return path
    }

    @Test func eachPartFillsItsFrame() {
        for part in [BrandLogo.Part.symbol, .wordmark, .whole] {
            let drawn = Self.outline(part).boundingRect
            let box = BrandMark.box(of: part)
            // (The files' frames are rounded to a tenth.)
            #expect(abs(drawn.minX - box.minX) < 1.5, "\(part) left")
            #expect(abs(drawn.minY - box.minY) < 1.5, "\(part) top")
            #expect(abs(drawn.maxX - box.maxX) < 1.5, "\(part) right")
            #expect(abs(drawn.maxY - box.maxY) < 1.5, "\(part) bottom")
        }
    }

    @Test func theLightPartsAreCutOut() {
        let mushroom = Self.outline(.symbol)
        // Ink: the cap, beside its middle spot.
        #expect(mushroom.contains(CGPoint(x: 430, y: 440), eoFill: true))
        // Cut out: the cap's three spots and the stem.
        for hole in [CGPoint(x: 459, y: 441), CGPoint(x: 390, y: 482), CGPoint(x: 502, y: 485), CGPoint(x: 450, y: 560)] {
            #expect(!mushroom.contains(hole, eoFill: true), "\(hole) is a light part")
        }
        // Outside altogether.
        #expect(!mushroom.contains(CGPoint(x: 340, y: 400), eoFill: true))
        // The inside of the o.
        let word = Self.outline(.wordmark)
        #expect(!word.contains(CGPoint(x: 1131, y: 512), eoFill: true))
        #expect(word.contains(CGPoint(x: 1075, y: 512), eoFill: true))
    }

    @Test func itIsScaledIntoTheSpaceItIsGiven() {
        let drawn = BrandMark(part: .whole).path(in: CGRect(x: 10, y: 20, width: 869.6 / 2, height: 229.2 / 2)).boundingRect
        #expect(abs(drawn.minX - 10) < 1)
        #expect(abs(drawn.minY - 20) < 1)
        #expect(abs(drawn.width - 869.6 / 2) < 1.5)
        #expect(abs(drawn.height - 229.2 / 2) < 1.5)
    }
}
