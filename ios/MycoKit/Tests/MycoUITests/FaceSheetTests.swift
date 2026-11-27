import SwiftUI
import Testing
@testable import MycoUI

/// A sheet of every cap shape with everything it can wear, light and dark,
/// saved as a picture to look at when MYCO_SHOTS_DIR is set (xcodebuild
/// passes TEST_RUNNER_MYCO_SHOTS_DIR on). Without it, it only draws.
@MainActor
struct FaceSheetTests {
    struct Sheet: View {
        var body: some View {
            VStack(alignment: .leading, spacing: 10) {
                ForEach(MushroomLook.shapes.indices, id: \.self) { shape in
                    HStack(spacing: 8) {
                        ForEach(MushroomLook.accessories.indices, id: \.self) { acc in
                            let look = MushroomLook(cap: (shape * 3 + acc) % 10, pattern: acc % 3, face: (shape + acc) % 4, shape: shape, acc: acc)
                            AvatarView(.mushroom(look), size: 40)
                                .opacity(MushroomLook.fits(acc, shape) ? 1 : 0.25)
                        }
                    }
                }
            }
            .padding(16)
            .background(MycoColor.bg)
        }
    }

    @Test func drawsEveryShapeAndAccessory() throws {
        for scheme in [ColorScheme.light, .dark] {
            let renderer = ImageRenderer(content: Sheet().environment(\.colorScheme, scheme))
            renderer.scale = 3
            let image = try #require(renderer.uiImage)
            #expect(image.size.width > 100)
            if let dir = ProcessInfo.processInfo.environment["MYCO_SHOTS_DIR"], let png = image.pngData() {
                try png.write(to: URL(fileURLWithPath: dir).appendingPathComponent("faces-\(scheme == .dark ? "dark" : "light").png"))
            }
        }
    }
}
