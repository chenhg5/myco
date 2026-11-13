import SwiftUI

// Feedback lives on the press, and it is instant: everything that can be
// tapped answers the moment a finger lands, before the tap is done.

/// A list row: a soft tile lights up behind it while it is pressed.
public struct RowButtonStyle: ButtonStyle {
    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background {
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .fill(MycoColor.pill)
                    .padding(.horizontal, -8)
                    .padding(.vertical, -4)
                    .opacity(configuration.isPressed ? 1 : 0)
            }
    }
}

/// A small thing that is pressed (a face, a capsule, a round button): it
/// gives a little under the finger.
public struct PressScaleStyle: ButtonStyle {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed && !reduceMotion ? 0.96 : 1)
            .opacity(configuration.isPressed ? 0.75 : 1)
            .animation(.snappy(duration: 0.15), value: configuration.isPressed)
    }
}
