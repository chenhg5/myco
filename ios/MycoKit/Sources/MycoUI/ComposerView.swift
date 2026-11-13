import SwiftUI

/// One way to add a file from the input's plus button ("Photo Library").
public struct AttachChoice: Identifiable {
    public var id: String
    public var title: String
    public var systemImage: String
    public var action: () -> Void

    public init(id: String, title: String, systemImage: String, action: @escaping () -> Void) {
        self.id = id
        self.title = title
        self.systemImage = systemImage
        self.action = action
    }
}

/// The input at the bottom of a conversation: attach, type, and one button
/// that sends, adds to the running task, or stops it. What the button does
/// is decided outside (`mode`); this view only draws it. The files about
/// to go sit above the words, inside the same shape.
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
    /// The files about to go, and what can be done with each.
    let files: [TrayFile]
    /// Why a file did not go up, said under the tiles.
    let fileNote: String?
    /// What VoiceOver calls the plus button, a tile's remove button and
    /// its "try again".
    let attachLabel: String
    let removeLabel: String
    let retryLabel: String
    /// What the plus button offers.
    let attachChoices: [AttachChoice]
    let onRemoveFile: (String) -> Void
    let onRetryFile: (String) -> Void

    public init(
        text: Binding<String>, placeholder: String, mode: Mode, enabled: Bool, submitLabel: String, queueTitle: String,
        files: [TrayFile] = [], fileNote: String? = nil, attachLabel: String, removeLabel: String, retryLabel: String,
        attachChoices: [AttachChoice],
        onSubmit: @escaping () -> Void, onQueue: @escaping () -> Void,
        onRemoveFile: @escaping (String) -> Void = { _ in }, onRetryFile: @escaping (String) -> Void = { _ in }
    ) {
        _text = text
        self.placeholder = placeholder
        self.mode = mode
        self.enabled = enabled
        self.submitLabel = submitLabel
        self.queueTitle = queueTitle
        self.onSubmit = onSubmit
        self.onQueue = onQueue
        self.files = files
        self.fileNote = fileNote
        self.attachLabel = attachLabel
        self.removeLabel = removeLabel
        self.retryLabel = retryLabel
        self.attachChoices = attachChoices
        self.onRemoveFile = onRemoveFile
        self.onRetryFile = onRetryFile
    }

    public var body: some View {
        HStack(alignment: .bottom, spacing: 10) {
            // A tap opens the ways to add something, right at the finger.
            Menu {
                ForEach(attachChoices) { choice in
                    Button(choice.title, systemImage: choice.systemImage, action: choice.action)
                }
            } label: {
                Image(systemName: "plus")
                    .font(.system(size: 18, weight: .medium))
                    .foregroundStyle(MycoColor.fg2)
                    .frame(width: 42, height: 42)
                    .background(MycoColor.pill, in: Circle())
            }
            .menuOrder(.fixed)
            .accessibilityLabel(attachLabel)
            .accessibilityIdentifier("composer.attach")

            // The files, the field and its button are one shape: the
            // button sits inside the field's trailing edge and stays at
            // the bottom as it grows.
            VStack(alignment: .leading, spacing: 0) {
                if !files.isEmpty {
                    AttachmentTray(files: files, removeLabel: removeLabel, retryLabel: retryLabel, onRemove: onRemoveFile, onRetry: onRetryFile)
                        .padding(.top, 6)
                    if let fileNote {
                        Text(fileNote).font(.caption).foregroundStyle(MycoColor.red)
                            .padding(.horizontal, 14).padding(.top, 6)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                HStack(alignment: .bottom, spacing: 4) {
                    TextField(placeholder, text: $text, axis: .vertical)
                        .lineLimit(1...8)
                        .font(.body)
                        .padding(.leading, 16)
                        .padding(.vertical, 10)
                        .accessibilityIdentifier("composer.input")
                    submitButton
                        .padding(.trailing, 5)
                        .padding(.bottom, 5)
                }
                .frame(minHeight: 42)
            }
            .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 21, style: .continuous))
            .animation(.snappy(duration: 0.2), value: files.isEmpty)
        }
        .padding(.horizontal, 14)
        .padding(.top, 8)
        .padding(.bottom, 8)
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
                .buttonStyle(PressScaleStyle())
                .disabled(!enabled)
                .accessibilityLabel(submitLabel)
                .accessibilityIdentifier("composer.submit")
        }
    }

    private var icon: some View {
        Image(systemName: mode == .stop ? "stop.fill" : "arrow.up")
            .font(.system(size: mode == .stop ? 12 : 15, weight: .bold))
            .foregroundStyle(MycoColor.buttonText)
            .frame(width: 32, height: 32)
            .background(MycoColor.button.opacity(enabled ? 1 : 0.3), in: Circle())
    }
}
