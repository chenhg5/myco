import SwiftUI

/// The input at the bottom of a conversation: attach, type, and one button
/// that sends, adds to the running task, or stops it. What the button does
/// is decided outside (`mode`); this view only draws it.
public struct ComposerView: View {
    public enum Mode: Sendable, Hashable {
        case send, steer, stop
    }

    @Binding var text: String
    let placeholder: String
    let mode: Mode
    let enabled: Bool
    /// What the button is called for VoiceOver ("Send", "Add", "Stop").
    let submitLabel: String
    /// The title of "send after this one" in the long-press menu.
    let queueTitle: String
    let onSubmit: () -> Void
    let onQueue: () -> Void
    let onAttach: () -> Void

    public init(
        text: Binding<String>, placeholder: String, mode: Mode, enabled: Bool, submitLabel: String, queueTitle: String,
        onSubmit: @escaping () -> Void, onQueue: @escaping () -> Void, onAttach: @escaping () -> Void
    ) {
        _text = text
        self.placeholder = placeholder
        self.mode = mode
        self.enabled = enabled
        self.submitLabel = submitLabel
        self.queueTitle = queueTitle
        self.onSubmit = onSubmit
        self.onQueue = onQueue
        self.onAttach = onAttach
    }

    public var body: some View {
        HStack(alignment: .bottom, spacing: 8) {
            Button(action: onAttach) {
                Image(systemName: "plus")
                    .font(.system(size: 17, weight: .medium))
                    .foregroundStyle(MycoColor.fg2)
                    .frame(width: 36, height: 36)
                    .background(MycoColor.pill, in: Circle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("composer.attach")

            TextField(placeholder, text: $text, axis: .vertical)
                .lineLimit(1...8)
                .font(.body)
                .padding(.horizontal, 14)
                .padding(.vertical, 8)
                .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
                .accessibilityIdentifier("composer.input")

            submitButton
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
    }

    @ViewBuilder private var submitButton: some View {
        if mode == .steer {
            // A tap adds to the running task; a long press offers to send
            // after it instead.
            Menu {
                Button(queueTitle, systemImage: "text.append", action: onQueue)
            } label: {
                icon
            } primaryAction: {
                onSubmit()
            }
            .disabled(!enabled)
            .accessibilityLabel(submitLabel)
            .accessibilityIdentifier("composer.submit")
        } else {
            Button(action: onSubmit) { icon }
                .buttonStyle(.plain)
                .disabled(!enabled)
                .accessibilityLabel(submitLabel)
                .accessibilityIdentifier("composer.submit")
        }
    }

    private var icon: some View {
        Image(systemName: mode == .stop ? "stop.fill" : "arrow.up")
            .font(.system(size: mode == .stop ? 13 : 16, weight: .bold))
            .foregroundStyle(MycoColor.buttonText)
            .frame(width: 36, height: 36)
            .background(MycoColor.button.opacity(enabled ? 1 : 0.3), in: Circle())
    }
}
