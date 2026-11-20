import SwiftUI
import UIKit

/// The title in the middle of a page's top bar: a name, and a quieter line
/// under it. One line each, never wider than the space left between the
/// bar's buttons on both sides, so the bar keeps it in the middle (a long
/// line under it is cut, not pushed against a button).
public struct BarTitle: View {
    let title: String
    let subtitle: String?

    public init(_ title: String, subtitle: String? = nil) {
        self.title = title
        self.subtitle = subtitle
    }

    public var body: some View {
        VStack(spacing: 1) {
            Text(title).font(.headline).foregroundStyle(MycoColor.fg).lineLimit(1)
            if let subtitle, !subtitle.isEmpty {
                Text(subtitle).font(.caption).foregroundStyle(MycoColor.muted).lineLimit(1)
            }
        }
        .multilineTextAlignment(.center)
        .frame(maxWidth: Self.maxWidth)
    }

    /// The screen's width, less room for a round button and its margins on
    /// each side (the same on both, so the middle is the middle).
    @MainActor public static var maxWidth: CGFloat {
        let width = UIApplication.shared.connectedScenes.compactMap { ($0 as? UIWindowScene)?.screen.bounds.width }.first ?? 390
        return max(160, width - 2 * 76)
    }
}
