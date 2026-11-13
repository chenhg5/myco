import SwiftUI
import UIKit

/// The app's colours: the same values as the desktop app's variables
/// (web/src/app/app.css), in light and dark. Colour is kept for things that
/// mean something: state, links, danger.
public enum MycoColor {
    /// The page behind everything.
    public static let bg = dynamic(0xF8F8FA, 0x1B1B1F)
    /// Cards and sheets.
    public static let card = dynamic(0xFFFFFF, 0x232327)
    /// Inputs, my bubbles, grey buttons.
    public static let pill = dynamic(0xF1F1F4, 0x2D2D33)
    public static let line = dynamic(0xE3E3E8, 0x333339)
    public static let fg = dynamic(0x1C1C21, 0xEDEDF1)
    public static let fg2 = dynamic(0x4B4B53, 0xB9B9C1)
    public static let muted = dynamic(0x85858D, 0x8B8B94)
    public static let faint = dynamic(0xB9B9C2, 0x56565F)
    /// At work, online, done well.
    public static let green = dynamic(0x2F8A68, 0x6CBF9D)
    public static let greenSoft = dynamic(0xE3F1EB, 0x1F342B)
    /// Needs a look.
    public static let amber = dynamic(0xB45309, 0xF2B544)
    public static let amberSoft = dynamic(0xFBF0DF, 0x3F3220)
    /// Failed, unreachable, dangerous.
    public static let red = dynamic(0xD64545, 0xF28B82)
    public static let redSoft = dynamic(0xFBE9E9, 0x45272A)
    /// Text links.
    public static let accent = dynamic(0x3A9A78, 0x52A685)
    /// The one solid button on a screen.
    public static let button = dynamic(0x26262C, 0xE8E8ED)
    public static let buttonText = dynamic(0xFFFFFF, 0x17171B)

    static func dynamic(_ light: UInt32, _ dark: UInt32) -> Color {
        Color(uiColor: UIColor { traits in
            UIColor(hex: traits.userInterfaceStyle == .dark ? dark : light)
        })
    }
}

extension UIColor {
    convenience init(hex: UInt32, alpha: CGFloat = 1) {
        self.init(
            red: CGFloat((hex >> 16) & 0xFF) / 255, green: CGFloat((hex >> 8) & 0xFF) / 255,
            blue: CGFloat(hex & 0xFF) / 255, alpha: alpha)
    }
}

extension Color {
    /// A colour from 0xRRGGBB.
    public init(hex: UInt32, opacity: Double = 1) {
        self.init(uiColor: UIColor(hex: hex, alpha: opacity))
    }
}

/// A state and its one colour, the same on every page.
public enum StatusTone: Sendable, Hashable {
    /// At work, online.
    case working
    /// Needs a look: possibly stuck.
    case attention
    /// Free, or done.
    case idle
    /// Away, or waiting its turn.
    case away
    case failed

    public var color: Color {
        switch self {
        case .working: return MycoColor.green
        case .attention: return MycoColor.amber
        case .idle: return MycoColor.muted
        case .away: return MycoColor.faint
        case .failed: return MycoColor.red
        }
    }
}

public struct StatusDot: View {
    let tone: StatusTone
    let size: CGFloat

    public init(_ tone: StatusTone, size: CGFloat = 8) {
        self.tone = tone
        self.size = size
    }

    public var body: some View {
        Circle().fill(tone.color).frame(width: size, height: size)
    }
}
