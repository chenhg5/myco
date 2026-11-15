import SwiftUI

/// The myco logo: a pixel mushroom and a pixel wordmark, drawn in one
/// colour (docs/assets/logo has the files; the desktop app draws the same
/// paths in web/src/components/brand.tsx). The light parts (the cap's
/// spots, the stem, the inside of the o) are cut out, so it sits on any
/// background.
public struct BrandLogo: View {
    public enum Part: Sendable, Hashable {
        /// The mushroom alone.
        case symbol
        /// "myco" alone.
        case wordmark
        /// Both, side by side, as designed.
        case whole
    }

    let part: Part
    let height: CGFloat

    /// - Parameter height: how tall it is drawn; its width follows.
    public init(_ part: Part = .whole, height: CGFloat) {
        self.part = part
        self.height = height
    }

    public var body: some View {
        let box = BrandMark.box(of: part)
        BrandMark(part: part)
            .fill(style: FillStyle(eoFill: true))
            .frame(width: height * box.width / box.height, height: height)
            .accessibilityElement()
            .accessibilityLabel("myco")
            .accessibilityAddTraits(.isImage)
    }
}

/// The logo's outline, scaled to the space it is given. Filled with the
/// even-odd rule, the cut-outs inside the ink come out empty.
struct BrandMark: Shape {
    let part: BrandLogo.Part

    static let symbolInk = [
        "m550.5 471.8v-19.2h-18.18v-18.18c-0.384-1.28-20.22-0.64-20.48-0.64v-19.71c-0.768-0.64-33.79-0.256-34.18-0.256v-20.99h-52.74v20.99c-0.896 0-33.54 0.256-33.66 0.512v19.46c-0.64 0-19.07-0.384-19.46 0.384v18.43h-18.43v19.2h-19.58c-0.256 9.216 0.128 34.82 0.256 38.53l19.58-0.512 0.64 20.61 52.22 0.384 0.384 19.33h-10.62v53.12h17.66v18.69c10.62 0.128 74.5-0.128 75.26-0.128v-19.07h17.54v-53.5h-11.65v-19.46h55.42v-19.84h19.2v-38.14h-19.2z",
    ]
    static let symbolHoles = [
        "m442.2 424.2h33.66v34.82h-33.66v-34.82z",
        "m373.9 465.8h33.28v33.54h-33.28v-33.54z",
        "m487.7 471.8h29.44v27.14l-29.44-0.512v-26.62z",
        "m424.4 508.4v52.61h-9.728v30.34h13.31v11.26h46.85v-12.03h12.54v-31.74h-9.984v-51.84l-52.99 1.408z",
    ]
    static let wordInk = [
        "m594.2 484.5h19.33v-19.33h19.33l0.256-19.2h92.54v19.33h19.58v19.07h18.69v89.73h-26.11v-10.88h-11.26v-61.95h-28.03v61.7h-38.14v-61.7h-28.16v61.7h-11.26v10.75h-26.75v-89.22z",
        "m776.2 446.1h41.47v19.46h18.69v39.68h18.05v-20.1h18.94v-38.91h43.52v43.9h-17.92v22.78h-19.71v38.78h-18.69l-0.512 22.4h-20.1v36.86h-15.87v11.01h-28.29v-10.75h-11.26v-32.13h19.58v-13.44h16.38v-13.95h18.82v-18.18h-22.14v-18.56h-16.77v-23.17h-24.06l-0.128-45.7z",
        "m963.7 446h79.74v35.58h-48.26v11.14h-25.47v34.05h25.47v11.26h48.38v34.3h-80.38v-14.85h-17.28v-17.92h-18.69v-55.17h18.56v-19.07h17.66l0.256-19.33z",
        "m1187 484.4v-19.07h-24.19v-19.07h-63.23v19.07h-24.45v19.07h-16.26v55.04h16.38v17.41h24.19v16h63.49v-16h24.19v-17.41h16.13v-55.04h-16.26z",
    ]
    static let wordHoles = [
        "m1115 488.6h32.51v11.01h13.95v26.37h-13.95v11.65h-32.51v-11.52h-14.59v-26.5h14.59v-11.01z",
    ]

    /// The part's frame in the drawing's own coordinates (the files' viewBox).
    static func box(of part: BrandLogo.Part) -> CGRect {
        switch part {
        case .symbol: return CGRect(x: 333.7, y: 392.8, width: 236.0, height: 229.2)
        case .wordmark: return CGRect(x: 594.2, y: 446.0, width: 609.1, height: 176.0)
        case .whole: return CGRect(x: 333.7, y: 392.8, width: 869.6, height: 229.2)
        }
    }

    static func outlines(of part: BrandLogo.Part) -> [String] {
        switch part {
        case .symbol: return symbolInk + symbolHoles
        case .wordmark: return wordInk + wordHoles
        case .whole: return symbolInk + symbolHoles + wordInk + wordHoles
        }
    }

    func path(in rect: CGRect) -> Path {
        let box = Self.box(of: part)
        var path = Path()
        for outline in Self.outlines(of: part) { Self.add(outline, to: &path) }
        let scale = min(rect.width / box.width, rect.height / box.height)
        let fit = CGAffineTransform(translationX: -box.minX, y: -box.minY)
            .concatenating(CGAffineTransform(scaleX: scale, y: scale))
            .concatenating(CGAffineTransform(
                translationX: rect.minX + (rect.width - box.width * scale) / 2,
                y: rect.minY + (rect.height - box.height * scale) / 2))
        return path.applying(fit)
    }

    /// Adds an SVG outline to a path. The logo's files use only the
    /// relative commands m, l, h, v, c and z, so those are what is read.
    static func add(_ outline: String, to path: inout Path) {
        var point = CGPoint.zero
        var start = CGPoint.zero
        var command: Character = " "
        var numbers: [CGFloat] = []
        var digits = ""

        func endNumber() {
            if let value = Double(digits) { numbers.append(CGFloat(value)) }
            digits = ""
        }
        func lines() {
            while numbers.count >= 2 {
                point = CGPoint(x: point.x + numbers[0], y: point.y + numbers[1])
                numbers.removeFirst(2)
                path.addLine(to: point)
            }
        }
        // Draws what the command before this point asked for.
        func draw() {
            switch command {
            case "m":
                if numbers.count >= 2 {
                    point = CGPoint(x: point.x + numbers[0], y: point.y + numbers[1])
                    numbers.removeFirst(2)
                    path.move(to: point)
                    start = point
                    // More pairs after a move are lines.
                    lines()
                }
            case "l":
                lines()
            case "h":
                for dx in numbers {
                    point.x += dx
                    path.addLine(to: point)
                }
            case "v":
                for dy in numbers {
                    point.y += dy
                    path.addLine(to: point)
                }
            case "c":
                while numbers.count >= 6 {
                    let first = CGPoint(x: point.x + numbers[0], y: point.y + numbers[1])
                    let second = CGPoint(x: point.x + numbers[2], y: point.y + numbers[3])
                    point = CGPoint(x: point.x + numbers[4], y: point.y + numbers[5])
                    numbers.removeFirst(6)
                    path.addCurve(to: point, control1: first, control2: second)
                }
            default:
                break
            }
            numbers = []
        }

        for character in outline {
            if character.isLetter {
                endNumber()
                draw()
                if character == "z" {
                    path.closeSubpath()
                    point = start
                    command = " "
                } else {
                    command = character
                }
            } else if character == "-" {
                // A minus starts the next number, wherever it stands.
                endNumber()
                digits = "-"
            } else if character == " " || character == "," {
                endNumber()
            } else {
                digits.append(character)
            }
        }
        endNumber()
        draw()
    }
}
