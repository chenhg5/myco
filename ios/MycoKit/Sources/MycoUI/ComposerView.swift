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

/// The microphone in the input: how it looks now.
public enum MicState: Sendable, Hashable {
    case idle
    /// Listening: red, and breathing.
    case listening
    /// It could not listen (no permission, no microphone): amber.
    case blocked
}

/// The input at the bottom of a conversation: a card with the words on
/// top and the controls under them. Attach and speak on the left; on the
/// right, alone, the one button that sends, adds to the running task,
/// stops it, or, with nothing typed, starts a voice call. What that
/// button does is decided outside (`mode`); this view only draws it. The
/// files about to go sit above the words.
public struct ComposerView: View {
    public enum Mode: Sendable, Hashable {
        case send, steer, stop
        /// Nothing typed and nothing running: the button starts a voice
        /// call with the agent (as in ChatGPT and Claude).
        case call
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
    /// Speaking instead of typing: nil leaves the microphone out.
    let mic: MicState?
    let micLabel: String
    let onMic: () -> Void
    /// The keyboard's Return sends instead of breaking the line (a
    /// setting, off unless the person chose it).
    let returnSends: Bool

    public init(
        text: Binding<String>, placeholder: String, mode: Mode, enabled: Bool, submitLabel: String, queueTitle: String,
        files: [TrayFile] = [], fileNote: String? = nil, attachLabel: String, removeLabel: String, retryLabel: String,
        attachChoices: [AttachChoice], mic: MicState? = nil, micLabel: String = "", returnSends: Bool = false,
        onSubmit: @escaping () -> Void, onQueue: @escaping () -> Void,
        onRemoveFile: @escaping (String) -> Void = { _ in }, onRetryFile: @escaping (String) -> Void = { _ in },
        onMic: @escaping () -> Void = {}
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
        self.mic = mic
        self.micLabel = micLabel
        self.onMic = onMic
        self.returnSends = returnSends
    }

    @FocusState private var focused: Bool

    /// Every control can be hit without aiming: Apple asks for 44 points.
    static let target: CGFloat = 44

    public var body: some View {
        // One card, two rows. The words get the whole width; the controls
        // sit under them: what adds to a message (files, speech) on the
        // left, the one button that sends it (or stops the run) alone at
        // the right, under the thumb. The microphone and Send are a whole
        // card apart, so one is never hit for the other, and the
        // microphone stays there while there are words: speaking can go on
        // where it stopped.
        VStack(alignment: .leading, spacing: 0) {
            if !files.isEmpty {
                AttachmentTray(files: files, removeLabel: removeLabel, retryLabel: retryLabel, onRemove: onRemoveFile, onRetry: onRetryFile)
                    .padding(.top, 8)
                if let fileNote {
                    Text(fileNote).font(.caption).foregroundStyle(MycoColor.red)
                        .padding(.horizontal, 16).padding(.top, 6)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            TextField(placeholder, text: $text, axis: .vertical)
                .lineLimit(1...8)
                .font(.body)
                .focused($focused)
                .submitLabel(returnSends ? .send : .return)
                .padding(.horizontal, 16)
                .padding(.top, 13)
                .padding(.bottom, 4)
                .accessibilityIdentifier("composer.input")
                // With a keyboard attached, ⌘Return sends.
                .onKeyPress(.return, phases: .down) { press in
                    guard press.modifiers.contains(.command), enabled, mode != .stop, mode != .call else { return .ignored }
                    onSubmit()
                    return .handled
                }
                .onChange(of: text) { old, new in
                    // For those who chose it: Return sends, as in a chat
                    // app. The line break it typed is taken back out.
                    guard returnSends, let typed = Self.withoutTypedReturn(old: old, new: new) else { return }
                    text = typed
                    if enabled, mode != .stop, mode != .call { onSubmit() }
                }
            HStack(spacing: 0) {
                // A tap opens the ways to add something, right at the finger.
                Menu {
                    ForEach(attachChoices) { choice in
                        Button(choice.title, systemImage: choice.systemImage, action: choice.action)
                    }
                } label: {
                    Image(systemName: "plus")
                        .font(.system(size: 20, weight: .medium))
                        .foregroundStyle(MycoColor.fg2)
                        .frame(width: Self.target, height: Self.target)
                        .contentShape(Rectangle())
                }
                .menuOrder(.fixed)
                .accessibilityLabel(attachLabel)
                .accessibilityIdentifier("composer.attach")
                if let mic {
                    MicButton(state: mic, label: micLabel, action: onMic)
                }
                Spacer(minLength: Space.l)
                submitButton
            }
            .padding(.horizontal, 4)
            .padding(.bottom, 3)
        }
        .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 24, style: .continuous))
        // Anywhere on the card that is not a control is the text field.
        .contentShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
        .onTapGesture { focused = true }
        .animation(.snappy(duration: 0.2), value: files.isEmpty)
        .padding(.horizontal, 12)
        .padding(.top, 8)
        .padding(.bottom, 8)
    }

    /// The text before a Return was typed into it: `new` is `old` with
    /// one line break more, anywhere. Nil for any other change (typing,
    /// pasting several lines, dictation).
    static func withoutTypedReturn(old: String, new: String) -> String? {
        guard new.count == old.count + 1 else { return nil }
        let before = Array(old), after = Array(new)
        var index = 0
        while index < before.count, before[index] == after[index] { index += 1 }
        guard after[index] == "\n", Array(after[(index + 1)...]) == Array(before[index...]) else { return nil }
        return old
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

    private var symbol: (name: String, size: CGFloat) {
        switch mode {
        case .stop: return ("stop.fill", 13)
        case .call: return ("waveform", 16)
        case .send, .steer: return ("arrow.up", 17)
        }
    }

    private var icon: some View {
        Image(systemName: symbol.name)
            .font(.system(size: symbol.size, weight: .bold))
            .foregroundStyle(MycoColor.buttonText)
            .frame(width: 36, height: 36)
            .background(MycoColor.button.opacity(enabled ? 1 : 0.3), in: Circle())
            // The circle is what is seen; the whole corner is what is hit.
            .frame(width: Self.target, height: Self.target)
            .contentShape(Rectangle())
    }
}

/// The microphone: a tap listens, another stops. While it listens it is
/// red and breathes.
private struct MicButton: View {
    let state: MicState
    let label: String
    let action: () -> Void
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var breathing = false

    private var color: Color {
        switch state {
        case .idle: return MycoColor.fg2
        case .listening: return MycoColor.red
        case .blocked: return MycoColor.amber
        }
    }

    var body: some View {
        Button(action: action) {
            Image(systemName: state == .listening ? "mic.fill" : "mic")
                .font(.system(size: 19, weight: .medium))
                .foregroundStyle(color)
                .frame(width: 34, height: 34)
                .background {
                    if state == .listening {
                        Circle().fill(MycoColor.red.opacity(0.14))
                            .scaleEffect(breathing && !reduceMotion ? 1.18 : 0.92)
                            .opacity(breathing && !reduceMotion ? 0.55 : 1)
                    }
                }
                .frame(width: ComposerView.target, height: ComposerView.target)
                .contentShape(Rectangle())
        }
        .buttonStyle(PressScaleStyle())
        .accessibilityLabel(label)
        .accessibilityIdentifier("composer.mic")
        .accessibilityAddTraits(state == .listening ? .isSelected : [])
        .onChange(of: state, initial: true) { _, now in
            if now == .listening {
                withAnimation(.easeInOut(duration: 0.9).repeatForever(autoreverses: true)) { breathing = true }
            } else {
                withAnimation(.easeOut(duration: 0.15)) { breathing = false }
            }
        }
    }
}
