import SwiftUI

/// What stands in for content that is on its way breathes slowly, so it
/// reads as "loading", not as a broken page. Still under Reduce Motion.
private struct Breathing: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var dim = false

    func body(content: Content) -> some View {
        content
            .opacity(dim && !reduceMotion ? 0.45 : 1)
            .onAppear {
                withAnimation(.easeInOut(duration: 0.9).repeatForever(autoreverses: true)) { dim = true }
            }
    }
}

/// Rows in the shape of what is about to load: a face and two lines. Shown
/// while a list waits for its first answer, instead of a spinner.
public struct SkeletonRows: View {
    let count: Int

    public init(count: Int = 6) {
        self.count = count
    }

    public var body: some View {
        VStack(spacing: 18) {
            ForEach(0..<count, id: \.self) { index in
                HStack(spacing: 12) {
                    RoundedRectangle(cornerRadius: 13, style: .continuous).fill(MycoColor.pill).frame(width: 50, height: 50)
                    VStack(alignment: .leading, spacing: 7) {
                        Capsule().fill(MycoColor.pill).frame(width: index.isMultiple(of: 2) ? 120 : 90, height: 11)
                        Capsule().fill(MycoColor.pill).frame(width: index.isMultiple(of: 3) ? 210 : 170, height: 9)
                    }
                    Spacer(minLength: 0)
                }
            }
            Spacer(minLength: 0)
        }
        .modifier(Breathing())
        .padding(.horizontal, 16)
        .padding(.top, 8)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(MycoColor.bg)
        .accessibilityHidden(true)
    }
}

/// A conversation that is on its way: something asked on the right, an
/// answer's lines on the left, twice, resting on the input like the real
/// one does. `label` is what VoiceOver says ("Loading").
public struct ConversationSkeleton: View {
    let label: String

    public init(label: String) {
        self.label = label
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: Space.xl) {
            Spacer(minLength: 0)
            turn(asked: 190, lines: [260, 300, 180])
            turn(asked: 130, lines: [280, 220])
            turn(asked: 220, lines: [300, 250, 290, 140])
        }
        .modifier(Breathing())
        .padding(.horizontal, Space.page)
        .padding(.bottom, Space.xl)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomLeading)
        .background(MycoColor.bg)
        .accessibilityElement()
        .accessibilityLabel(label)
        .accessibilityIdentifier("conversation.skeleton")
    }

    private func turn(asked: CGFloat, lines: [CGFloat]) -> some View {
        VStack(alignment: .leading, spacing: Space.l) {
            RoundedRectangle(cornerRadius: 20, style: .continuous).fill(MycoColor.pill)
                .frame(width: asked, height: 44)
                .frame(maxWidth: .infinity, alignment: .trailing)
            VStack(alignment: .leading, spacing: 11) {
                ForEach(Array(lines.enumerated()), id: \.offset) { _, width in
                    Capsule().fill(MycoColor.pill).frame(width: width, height: 11)
                }
            }
        }
    }
}

/// Something that comes into a page already on screen (a new message, an
/// older page) arrives: it fades in and rises a little, instead of being
/// there all at once. What was there when the page opened does not move.
public struct Arrival: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var here: Bool

    /// - Parameter animated: false for what the page opens with.
    public init(animated: Bool) {
        _here = State(initialValue: !animated)
    }

    public func body(content: Content) -> some View {
        content
            .opacity(here ? 1 : 0)
            .offset(y: here || reduceMotion ? 0 : 10)
            .onAppear {
                guard !here else { return }
                withAnimation(.easeOut(duration: 0.3)) { here = true }
            }
    }
}
