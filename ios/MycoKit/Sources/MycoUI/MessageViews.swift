import SwiftUI

/// What I said: a small bubble on the right. A message added to a running
/// task carries a small label above it.
public struct MyBubble: View {
    let text: String
    let label: String?

    public init(_ text: String, label: String? = nil) {
        self.text = text
        self.label = label
    }

    public var body: some View {
        VStack(alignment: .trailing, spacing: 3) {
            if let label {
                Text(label).font(.caption2).foregroundStyle(MycoColor.muted)
            }
            Text(text)
                .font(.body)
                .foregroundStyle(MycoColor.fg)
                .textSelection(.enabled)
                .padding(.horizontal, 14)
                .padding(.vertical, 9)
                .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
        }
        .frame(maxWidth: .infinity, alignment: .trailing)
        .padding(.leading, 48)
    }
}

/// An interim update from the agent: one quiet line.
public struct ProgressLine: View {
    let text: String

    public init(_ text: String) {
        self.text = text
    }

    public var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Circle().fill(MycoColor.faint).frame(width: 5, height: 5).offset(y: -2)
            Text(text).font(.subheadline).foregroundStyle(MycoColor.fg2)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// The updates of a task that has ended, as one quiet line; a tap opens them.
public struct PastProgress: View {
    let title: String
    let texts: [String]
    @State private var open = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    public init(title: String, texts: [String]) {
        self.title = title
        self.texts = texts
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Button {
                withAnimation(reduceMotion ? nil : .snappy(duration: 0.22)) { open.toggle() }
            } label: {
                HStack(spacing: 6) {
                    Text(title).font(.subheadline).foregroundStyle(MycoColor.muted)
                    Image(systemName: "chevron.right")
                        .font(.caption2.weight(.semibold))
                        .foregroundStyle(MycoColor.faint)
                        .rotationEffect(.degrees(open ? 90 : 0))
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("progress.fold")

            if open {
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(Array(texts.enumerated()), id: \.offset) { _, text in
                        ProgressLine(text)
                    }
                }
                .transition(.opacity)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// Where an open task stands, at the end of the conversation: a little
/// mushroom, what is happening, and for how long.
public struct WorkingLine: View {
    let look: MushroomLook
    let text: String
    /// Counted from here while it is being worked on; nil while it waits.
    let since: Date?

    public init(look: MushroomLook, text: String, since: Date?) {
        self.look = look
        self.text = text
        self.since = since
    }

    public var body: some View {
        HStack(spacing: 8) {
            AvatarView(.mushroom(look), size: 22)
            Text(text).font(.subheadline).foregroundStyle(MycoColor.fg2)
            if let since {
                Text(since, style: .timer)
                    .font(.subheadline.monospacedDigit())
                    .foregroundStyle(MycoColor.muted)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }
}

/// A line that needs a look or reports a failure, with up to two actions.
public struct NoticeLine: View {
    public struct Action {
        let title: String
        let run: () -> Void

        public init(_ title: String, run: @escaping () -> Void) {
            self.title = title
            self.run = run
        }
    }

    let tone: StatusTone
    let text: String
    let actions: [Action]

    public init(_ tone: StatusTone, _ text: String, actions: [Action] = []) {
        self.tone = tone
        self.text = text
        self.actions = actions
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                StatusDot(tone, size: 7).offset(y: -1)
                Text(text).font(.subheadline).foregroundStyle(tone == .failed ? MycoColor.red : MycoColor.fg2)
            }
            if !actions.isEmpty {
                HStack(spacing: 8) {
                    ForEach(Array(actions.enumerated()), id: \.offset) { _, action in
                        Button(action.title, action: action.run).buttonStyle(QuietButtonStyle())
                    }
                }
                .padding(.leading, 15)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// Work given to someone, as one line: whose face and name, what, and
/// where it stands. The name gives way first: the task is what is read.
public struct DispatchCapsule: View {
    let face: AvatarView.Face
    let name: String
    let text: String
    let stateText: String
    let tone: StatusTone
    /// The whole line in words, for VoiceOver ("To X: …, Working").
    let spoken: String
    /// Where a tap leads; nil for a capsule that only reports.
    let action: (() -> Void)?

    public init(
        face: AvatarView.Face, name: String, text: String, stateText: String, tone: StatusTone, spoken: String,
        action: (() -> Void)? = nil
    ) {
        self.face = face
        self.name = name
        self.spoken = spoken
        self.text = text
        self.stateText = stateText
        self.tone = tone
        self.action = action
    }

    public var body: some View {
        if let action {
            Button(action: action) { capsule }.buttonStyle(PressScaleStyle())
        } else {
            capsule
        }
    }

    private var capsule: some View {
        HStack(spacing: 8) {
            AvatarView(face, size: 22)
            Text(name).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg2).lineLimit(1)
                .frame(maxWidth: 84, alignment: .leading).fixedSize(horizontal: true, vertical: false)
            Text(text).font(.subheadline).foregroundStyle(MycoColor.muted).lineLimit(1)
                .layoutPriority(1)
            Spacer(minLength: 6)
            StatusDot(tone, size: 6)
            Text(stateText).font(.footnote).foregroundStyle(MycoColor.muted).fixedSize()
            if action != nil {
                Image(systemName: "chevron.right").font(.caption2.weight(.semibold)).foregroundStyle(MycoColor.faint)
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(MycoColor.line, lineWidth: 0.5))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spoken)
    }
}

/// A grey, small button: secondary actions inside a conversation.
public struct QuietButtonStyle: ButtonStyle {
    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.subheadline.weight(.medium))
            .foregroundStyle(MycoColor.fg)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(MycoColor.pill.opacity(configuration.isPressed ? 0.6 : 1), in: Capsule())
    }
}

/// The one solid button of a screen.
public struct PrimaryButtonStyle: ButtonStyle {
    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.body.weight(.semibold))
            .foregroundStyle(MycoColor.buttonText)
            .frame(maxWidth: .infinity)
            .padding(.vertical, 13)
            .background(MycoColor.button.opacity(configuration.isPressed ? 0.8 : 1), in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }
}
