import SwiftUI

/// The distances things keep from each other. Every gap in the app is one
/// of these, so pages breathe the same way: tight inside one thing, wider
/// between things, widest between turns of a conversation.
public enum Space {
    /// Inside one thing: an icon and its label.
    public static let xs: CGFloat = 4
    /// Lines that belong together: steps of one run.
    public static let s: CGFloat = 8
    /// Blocks of one answer: paragraphs, a list, a code block.
    public static let m: CGFloat = 14
    /// Parts of one turn: what was asked, how it was worked on, the answer.
    public static let l: CGFloat = 20
    /// Between turns of a conversation.
    public static let xl: CGFloat = 28
    /// A page's side margins.
    public static let page: CGFloat = 18
}

extension View {
    /// Sizes a sheet to what is in it: no empty stretch under its buttons.
    public func fittedSheet() -> some View {
        modifier(FittedSheet())
    }
}

private struct FittedSheet: ViewModifier {
    @State private var height: CGFloat = 320

    func body(content: Content) -> some View {
        content
            .fixedSize(horizontal: false, vertical: true)
            .onGeometryChange(for: CGFloat.self) { proxy in
                proxy.size.height
            } action: { new in
                height = new
            }
            .presentationDetents([.height(height)])
            .presentationDragIndicator(.visible)
    }
}
