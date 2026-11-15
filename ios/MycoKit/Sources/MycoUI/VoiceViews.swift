import SwiftUI

/// A voice call with an agent, docked above the input: how the call
/// stands, the last things said, and the two buttons a call needs. It
/// draws plain values; the call itself is elsewhere.
public struct VoiceBarView: View {
    public enum Stage: Sendable, Hashable {
        case connecting, listening, thinking, speaking
    }

    /// One line of the talk.
    public struct Line: Identifiable, Hashable, Sendable {
        public enum Kind: Sendable, Hashable {
            /// I said it; `hearing`: I am still saying it.
            case me(hearing: Bool)
            /// The voice said it; `cut`: I talked over it.
            case voice(cut: Bool)
            /// It did something ("Handed to X: ...").
            case action
            /// Something to know.
            case notice
        }

        public var id: Int
        public var kind: Kind
        public var text: String

        public init(id: Int, kind: Kind, text: String) {
            self.id = id
            self.kind = kind
            self.text = text
        }
    }

    let stage: Stage
    let muted: Bool
    /// "在听", "Thinking": the stage in words.
    let stageText: String
    /// "和 X 语音聊天".
    let title: String
    /// What to say while nothing has been said yet.
    let hint: String
    let lines: [Line]
    let meLabel: String
    let voiceLabel: String
    let muteLabel: String
    let hangUpLabel: String
    let onMute: () -> Void
    let onHangUp: () -> Void

    public init(
        stage: Stage, muted: Bool, stageText: String, title: String, hint: String, lines: [Line],
        meLabel: String, voiceLabel: String, muteLabel: String, hangUpLabel: String,
        onMute: @escaping () -> Void, onHangUp: @escaping () -> Void
    ) {
        self.stage = stage
        self.muted = muted
        self.stageText = stageText
        self.title = title
        self.hint = hint
        self.lines = lines
        self.meLabel = meLabel
        self.voiceLabel = voiceLabel
        self.muteLabel = muteLabel
        self.hangUpLabel = hangUpLabel
        self.onMute = onMute
        self.onHangUp = onHangUp
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            HStack(spacing: Space.s) {
                VoiceDot(stage: stage, muted: muted)
                    .frame(width: 22, height: 22)
                VStack(alignment: .leading, spacing: 1) {
                    Text(stageText).font(.subheadline.weight(.semibold)).foregroundStyle(MycoColor.fg)
                        .accessibilityIdentifier("voice.state")
                    Text(title).font(.caption).foregroundStyle(MycoColor.muted).lineLimit(1)
                }
                Spacer(minLength: Space.s)
                Button(action: onMute) {
                    Image(systemName: muted ? "mic.slash.fill" : "mic.fill")
                        .font(.system(size: 15, weight: .medium))
                        .foregroundStyle(muted ? MycoColor.buttonText : MycoColor.fg2)
                        .frame(width: 36, height: 36)
                        .background(muted ? MycoColor.button : MycoColor.card, in: Circle())
                        .frame(width: 44, height: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(PressScaleStyle())
                .disabled(stage == .connecting)
                .accessibilityLabel(muteLabel)
                .accessibilityIdentifier("voice.mute")
                Button(action: onHangUp) {
                    Image(systemName: "phone.down.fill")
                        .font(.system(size: 15, weight: .medium))
                        .foregroundStyle(.white)
                        .frame(width: 36, height: 36)
                        .background(MycoColor.red, in: Circle())
                        .frame(width: 44, height: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(PressScaleStyle())
                .accessibilityLabel(hangUpLabel)
                .accessibilityIdentifier("voice.hangup")
            }
            if lines.isEmpty {
                Text(hint).font(.footnote).foregroundStyle(MycoColor.muted).lineSpacing(2)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("voice.hint")
            } else {
                VStack(alignment: .leading, spacing: 5) {
                    ForEach(lines) { line in
                        row(line)
                    }
                }
                .animation(.snappy(duration: 0.2), value: lines.map(\.id))
            }
        }
        .padding(.leading, 14)
        .padding(.trailing, 6)
        .padding(.top, 6)
        .padding(.bottom, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 20, style: .continuous))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("voice.bar")
    }

    /// The column of who spoke: wide enough for "Voice", the same for
    /// every line, so the words start at one edge.
    private static let nameWidth: CGFloat = 36

    @ViewBuilder private func row(_ line: Line) -> some View {
        switch line.kind {
        case .me(let hearing):
            said(meLabel, line.text.isEmpty ? "…" : line.text, quiet: hearing)
                .accessibilityIdentifier("voice.line.me")
        case .voice(let cut):
            said(voiceLabel, (line.text.isEmpty ? "…" : line.text) + (cut ? " …" : ""), quiet: cut)
                .accessibilityIdentifier("voice.line.voice")
        case .action:
            // The arrow stands where a speaker's name does: what was done
            // lines up with what was said.
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Image(systemName: "arrow.turn.down.right").font(.caption)
                    .foregroundStyle(MycoColor.faint)
                    .frame(width: Self.nameWidth, alignment: .leading)
                Text(line.text).lineLimit(2)
            }
            .font(.footnote).foregroundStyle(MycoColor.fg2)
            .padding(.trailing, 8)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("voice.line.action")
        case .notice:
            Text(line.text).font(.footnote).foregroundStyle(MycoColor.amber).lineLimit(3)
                .padding(.trailing, 8)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("voice.line.notice")
        }
    }

    private func said(_ who: String, _ text: String, quiet: Bool) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Text(who).font(.caption).foregroundStyle(MycoColor.faint).lineLimit(1)
                .frame(width: Self.nameWidth, alignment: .leading)
            Text(text).font(.subheadline).foregroundStyle(quiet ? MycoColor.muted : MycoColor.fg)
                .lineLimit(3)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(.trailing, 8)
        .accessibilityElement(children: .combine)
    }
}

/// The call's pulse: it breathes while it listens, quickens while it
/// thinks, ripples while it talks, and is grey when muted or not yet on.
private struct VoiceDot: View {
    let stage: VoiceBarView.Stage
    let muted: Bool
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var beat = false

    private var still: Bool { reduceMotion || muted || stage == .connecting }

    private var period: Double {
        switch stage {
        case .listening, .connecting: return 1.3
        case .thinking: return 0.5
        case .speaking: return 0.42
        }
    }

    var body: some View {
        ZStack {
            if stage == .speaking, !still {
                Circle().stroke(MycoColor.accent.opacity(0.45), lineWidth: 1.5)
                    .scaleEffect(beat ? 1.9 : 0.9)
                    .opacity(beat ? 0 : 0.8)
            }
            Circle().fill(still ? MycoColor.faint : MycoColor.accent)
                .scaleEffect(still ? 1 : (beat ? 1.12 : 0.82))
                .opacity(still || stage != .thinking ? 1 : 0.7)
        }
        .frame(width: 11, height: 11)
        .onChange(of: stage, initial: true) { restart() }
        .onChange(of: muted) { restart() }
        .accessibilityHidden(true)
    }

    private func restart() {
        withAnimation(.linear(duration: 0.01)) { beat = false }
        guard !still else { return }
        withAnimation(.easeInOut(duration: period).repeatForever(autoreverses: true)) { beat = true }
    }
}

/// The small mark on something that was passed on by voice. A tap shows
/// the words as they were heard.
public struct VoiceMark: View {
    let label: String
    /// "原话：…"; empty when the words were not kept.
    let heard: String
    @State private var open = false

    public init(label: String, heard: String) {
        self.label = label
        self.heard = heard
    }

    public var body: some View {
        VStack(alignment: .trailing, spacing: 3) {
            Button {
                withAnimation(.snappy(duration: 0.2)) { open.toggle() }
            } label: {
                Label(label, systemImage: "waveform")
                    .labelStyle(.titleAndIcon)
                    .font(.caption2.weight(.medium))
                    .foregroundStyle(MycoColor.muted)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .disabled(heard.isEmpty)
            .accessibilityIdentifier("voice.mark")
            .accessibilityHint(heard)
            if open, !heard.isEmpty {
                Text(heard).font(.caption).foregroundStyle(MycoColor.muted)
                    .multilineTextAlignment(.trailing)
                    .padding(.horizontal, 6)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .trailing)
        .padding(.leading, 56)
    }
}
