import SwiftUI

/// A face: an agent's pixel mushroom, a computer's little screen, or a
/// person's initial. With `status`, a small dot at the bottom left says
/// whether it is at work, needs a look, or is away.
public struct AvatarView: View {
    public enum Face: Sendable, Hashable {
        case mushroom(MushroomLook)
        case computer
        case person(initial: String, tint: UInt32)
    }

    /// The small mark for which AI it is.
    public enum Badge: Sendable, Hashable {
        case claude, codex
    }

    let face: Face
    let size: CGFloat
    let status: StatusTone?
    let badge: Badge?
    let away: Bool

    public init(_ face: Face, size: CGFloat = 40, status: StatusTone? = nil, badge: Badge? = nil, away: Bool = false) {
        self.face = face
        self.size = size
        self.status = status
        self.badge = badge
        self.away = away
    }

    public var body: some View {
        tile
            .frame(width: size, height: size)
            .opacity(away ? 0.55 : 1)
            .overlay(alignment: .bottomLeading) {
                if let status {
                    StatusDot(status, size: max(8, size * 0.24))
                        .padding(1.5)
                        .background(MycoColor.bg, in: Circle())
                        .offset(x: -2, y: 2)
                }
            }
            .overlay(alignment: .bottomTrailing) {
                if let badge, size >= 28 {
                    BadgeMark(badge: badge, size: max(12, size * 0.34)).offset(x: 3, y: 3)
                }
            }
            .accessibilityHidden(true)
    }

    @ViewBuilder private var tile: some View {
        switch face {
        case .mushroom(let look):
            let cap = Color(hex: look.capColor)
            ZStack {
                RoundedRectangle(cornerRadius: size * 0.32, style: .continuous).fill(MycoColor.card)
                RoundedRectangle(cornerRadius: size * 0.32, style: .continuous).fill(cap.opacity(0.18))
                PixelGrid(rows: look.rows, top: 2, sprout: look.sprout) { key in
                    switch key {
                    case "C": return cap
                    case "o": return Color.white.opacity(0.85)
                    case "S": return Color(hex: 0xEFE3CC)
                    case "e", "g": return Color(hex: 0x2F2A22)
                    case "b": return Color(hex: 0xE86E6E, opacity: 0.55)
                    default: return nil
                    }
                }
                .padding(size * 0.14)
            }
        case .computer:
            ZStack {
                RoundedRectangle(cornerRadius: size * 0.32, style: .continuous).fill(MycoColor.pill)
                PixelGrid(rows: Self.screen, top: 0, sprout: false) { key in
                    switch key {
                    case "W": return Color(hex: 0x5A5F6B)
                    case "F": return Color(hex: 0x15171C)
                    case "P": return Color(hex: 0x5FD7A7)
                    case "L": return Color(hex: 0xCDD0D8)
                    default: return nil
                    }
                }
                .padding(size * 0.14)
            }
        case .person(let initial, let tint):
            Circle().fill(Color(hex: tint).opacity(0.75))
                .overlay {
                    Text(initial).font(.system(size: size * 0.45, weight: .semibold)).foregroundStyle(.white)
                }
        }
    }

    static let screen = [
        "............", ".WWWWWWWWWW.", ".WFFFFFFFFW.", ".WFPFFFFFFW.", ".WFFPFFFFFW.", ".WFPFFFFFFW.",
        ".WFFFFLLLFW.", ".WFFFFFFFFW.", ".WWWWWWWWWW.", "....WWWW....", "...WWWWWW...", "............",
    ]
}

/// Twelve columns of square pixels, drawn on whole points so edges stay crisp.
struct PixelGrid: View {
    let rows: [String]
    /// Empty rows above the drawing (a mushroom leaves two for its sprout).
    let top: Int
    let sprout: Bool
    let fill: (Character) -> Color?

    var body: some View {
        Canvas { context, size in
            let total = CGFloat(rows.count + top)
            let pixel = min(size.width / 12, size.height / total)
            let originX = (size.width - pixel * 12) / 2
            let originY = (size.height - pixel * total) / 2
            func rect(_ x: Int, _ y: Int, _ w: Int = 1, _ h: Int = 1) -> Path {
                Path(CGRect(
                    x: originX + CGFloat(x) * pixel, y: originY + CGFloat(y + top) * pixel,
                    width: CGFloat(w) * pixel + 0.3, height: CGFloat(h) * pixel + 0.3))
            }
            if sprout {
                let green = Color(hex: 0x5AA86A)
                context.fill(rect(6, -2, 1, 2), with: .color(green))
                context.fill(rect(7, -2, 1, 1), with: .color(green))
            }
            for (y, row) in rows.enumerated() {
                for (x, key) in row.enumerated() {
                    if let color = fill(key) { context.fill(rect(x, y), with: .color(color)) }
                }
            }
        }
        .aspectRatio(12 / CGFloat(rows.count + top), contentMode: .fit)
    }
}

/// Claude's orange asterisk, Codex's blue-violet prompt.
struct BadgeMark: View {
    let badge: AvatarView.Badge
    let size: CGFloat

    var body: some View {
        ZStack {
            Circle().fill(badge == .claude ? Color(hex: 0xD97757) : Color(hex: 0x6D5EF0))
            Image(systemName: badge == .claude ? "asterisk" : "chevron.right")
                .font(.system(size: size * 0.56, weight: .heavy))
                .foregroundStyle(.white)
        }
        .frame(width: size, height: size)
        .overlay(Circle().stroke(MycoColor.bg, lineWidth: 1.5))
    }
}
