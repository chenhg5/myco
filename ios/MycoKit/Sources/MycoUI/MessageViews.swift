import SwiftUI
import UIKit

/// Something asked of the agent: a bubble on the right. Mine is filled;
/// one a colleague asked is outlined and carries their name, so whose
/// words they are is never in doubt. A message added to a running task
/// carries a small label.
public struct MyBubble: View {
    let text: String
    let label: String?
    let name: String?

    /// - Parameters:
    ///   - label: a small mark above it ("Added").
    ///   - name: who asked, when it was not me.
    public init(_ text: String, label: String? = nil, name: String? = nil) {
        self.text = text
        self.label = label
        self.name = name
    }

    public var body: some View {
        VStack(alignment: .trailing, spacing: 4) {
            if name != nil || label != nil {
                HStack(spacing: 6) {
                    if let name { Text(name).font(.footnote.weight(.medium)).foregroundStyle(MycoColor.fg2) }
                    if let label { Text(label).font(.caption2).foregroundStyle(MycoColor.muted) }
                }
                .padding(.trailing, 6)
            }
            Text(text)
                .font(.body)
                .lineSpacing(3)
                .foregroundStyle(MycoColor.fg)
                .padding(.horizontal, 16)
                .padding(.vertical, 11)
                .background(name == nil ? MycoColor.pill : MycoColor.card, in: RoundedRectangle(cornerRadius: 20, style: .continuous))
                .overlay {
                    if name != nil {
                        RoundedRectangle(cornerRadius: 20, style: .continuous).stroke(MycoColor.line, lineWidth: 1)
                    }
                }
        }
        .frame(maxWidth: .infinity, alignment: .trailing)
        .padding(.leading, 56)
    }
}

/// The time, written small and centred where a conversation paused.
public struct TimeBreak: View {
    let text: String

    public init(_ text: String) {
        self.text = text
    }

    public var body: some View {
        Text(text)
            .font(.footnote)
            .foregroundStyle(MycoColor.faint)
            .frame(maxWidth: .infinity, alignment: .center)
    }
}

/// What can be done with an answer, in a quiet row under it: copy it, and
/// see when it came and what it took.
public struct ReplyActions: View {
    let text: String
    let detail: String
    let copyLabel: String
    @State private var copied = false

    /// - Parameter detail: the time, and what the run took ("14:32 · 12 秒").
    public init(copying text: String, detail: String, copyLabel: String) {
        self.text = text
        self.detail = detail
        self.copyLabel = copyLabel
    }

    public var body: some View {
        HStack(spacing: Space.s) {
            Button {
                UIPasteboard.general.string = text
                copied = true
                Task {
                    try? await Task.sleep(for: .seconds(1.6))
                    copied = false
                }
            } label: {
                Image(systemName: copied ? "checkmark" : "square.on.square")
                    .font(.system(size: 15, weight: .regular))
                    .foregroundStyle(copied ? MycoColor.green : MycoColor.muted)
                    .frame(width: 34, height: 34)
                    .contentShape(Rectangle())
                    .contentTransition(.symbolEffect(.replace))
            }
            .buttonStyle(PressScaleStyle())
            .accessibilityLabel(copyLabel)
            .accessibilityIdentifier("reply.copy")
            .sensoryFeedback(.success, trigger: copied) { _, now in now }

            Text(detail).font(.footnote).foregroundStyle(MycoColor.faint).lineLimit(1)
            Spacer(minLength: 0)
        }
        // The icon's box hangs into the margin, so its glyph lines up with the text above.
        .padding(.leading, -9)
    }
}

/// Back to the newest message: a round button that floats over a
/// conversation while its end is out of sight.
public struct ScrollToEndButton: View {
    let label: String
    let action: () -> Void

    public init(label: String, action: @escaping () -> Void) {
        self.label = label
        self.action = action
    }

    public var body: some View {
        Button(action: action) {
            Image(systemName: "arrow.down")
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(MycoColor.fg)
                .frame(width: 38, height: 38)
                .background(.regularMaterial, in: Circle())
                .overlay(Circle().stroke(MycoColor.line, lineWidth: 0.5))
                .shadow(color: .black.opacity(0.12), radius: 8, y: 3)
        }
        .buttonStyle(PressScaleStyle())
        .accessibilityLabel(label)
        .accessibilityIdentifier("conversation.toEnd")
    }
}

/// An update from the agent while it works, as one entry of a timeline: a
/// dot on a thin line that joins it to the updates next to it, what it
/// reported (Markdown, a size smaller than an answer), and when. The
/// newest update of a task still being worked on has a green dot.
public struct ProgressLine: View {
    /// Where the entry sits among the updates it is drawn with: the line
    /// runs from the first dot to the last, and one alone has none.
    public enum Position: Sendable, Hashable {
        case only, first, middle, last
    }

    let text: String
    let time: String?
    let position: Position
    let latest: Bool

    public init(_ text: String, time: String? = nil, position: Position = .only, latest: Bool = false) {
        self.text = text
        self.time = time
        self.position = position
        self.latest = latest
    }

    /// How far down an entry its dot's centre is: on its first line.
    private static let dotY: CGFloat = 16
    /// The rail the dots and the line sit on.
    private static let railX: CGFloat = 7

    public var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Color.clear.frame(width: Self.railX * 2, height: 1)
            ReplyText(text, compact: true)
            if let time {
                Text(time).font(.caption.monospacedDigit()).foregroundStyle(MycoColor.faint).padding(.top, 2)
            }
        }
        .padding(.vertical, 6)
        .frame(maxWidth: .infinity, alignment: .leading)
        // The line joining the dots: from this entry's dot down, up to it,
        // or straight through.
        .background(alignment: .topLeading) {
            switch position {
            case .only:
                EmptyView()
            case .first:
                Rectangle().fill(MycoColor.line).frame(width: 1).padding(.top, Self.dotY).offset(x: Self.railX - 0.5)
            case .middle:
                Rectangle().fill(MycoColor.line).frame(width: 1).offset(x: Self.railX - 0.5)
            case .last:
                Rectangle().fill(MycoColor.line).frame(width: 1, height: Self.dotY).offset(x: Self.railX - 0.5)
            }
        }
        // The dot, with a ring of the page's colour so the line stops short of it.
        .overlay(alignment: .topLeading) {
            Circle().fill(latest ? MycoColor.green : MycoColor.faint)
                .frame(width: 6, height: 6)
                .padding(2)
                .background(MycoColor.bg, in: Circle())
                .offset(x: Self.railX - 5, y: Self.dotY - 5)
        }
    }

    /// The position of the entry at `index` among `count` drawn together.
    public static func position(_ index: Int, of count: Int) -> Position {
        if count <= 1 { return .only }
        if index == 0 { return .first }
        return index == count - 1 ? .last : .middle
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
                VStack(alignment: .leading, spacing: 0) {
                    ForEach(Array(texts.enumerated()), id: \.offset) { index, text in
                        ProgressLine(text, position: ProgressLine.position(index, of: texts.count))
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
    /// How the time it has taken reads ("20 分 37 秒").
    let elapsed: (Date) -> String

    public init(look: MushroomLook, text: String, since: Date?, elapsed: @escaping (Date) -> String = { _ in "" }) {
        self.look = look
        self.text = text
        self.since = since
        self.elapsed = elapsed
    }

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    public var body: some View {
        HStack(spacing: 10) {
            // At work, the little mushroom bobs; waiting its turn, it stands.
            AvatarView(.mushroom(look), size: 26)
                .modifier(Bobbing(active: since != nil && !reduceMotion))
            Text(text).font(.subheadline).foregroundStyle(MycoColor.fg2).lineLimit(2)
            if let since {
                // Counted in words, as the desktop app does, once a second.
                TimelineView(.periodic(from: .now, by: 1)) { context in
                    Text("· " + elapsed(since))
                        .font(.subheadline.monospacedDigit())
                        .foregroundStyle(MycoColor.muted)
                        .id(Int(context.date.timeIntervalSince1970))
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }
}

/// A slow, small up-and-down, for as long as `active`.
private struct Bobbing: ViewModifier {
    let active: Bool
    @State private var up = false

    func body(content: Content) -> some View {
        content
            .offset(y: active && up ? -2.5 : 0)
            .animation(active ? .easeInOut(duration: 0.6).repeatForever(autoreverses: true) : .default, value: up)
            .onAppear { up = active }
            .onChange(of: active) { _, now in up = now }
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
