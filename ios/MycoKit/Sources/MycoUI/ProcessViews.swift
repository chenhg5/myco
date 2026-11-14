import SwiftUI

/// One tool step, as a line: what kind, on what, and whether it failed or
/// is still going.
public struct StepLine: Identifiable, Hashable, Sendable {
    public var id: Int
    public var symbol: String
    public var label: String
    public var detail: String
    public var failed: Bool
    public var running: Bool
    /// Its detail is words to read (what it thought), not a command or a
    /// path: shown in full, as text.
    public var prose: Bool

    public init(
        id: Int, symbol: String, label: String, detail: String, failed: Bool = false, running: Bool = false, prose: Bool = false
    ) {
        self.id = id
        self.symbol = symbol
        self.label = label
        self.detail = detail
        self.failed = failed
        self.running = running
        self.prose = prose
    }
}

/// A run of steps folded into one grey line ("ran 3 commands, edited 1
/// file"); a tap opens it. It never opens by itself.
public struct StepGroupView: View {
    let summary: String
    let steps: [StepLine]
    @State private var open = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    public init(summary: String, steps: [StepLine]) {
        self.summary = summary
        self.steps = steps
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Button {
                // Opens in place, quickly, with no bounce; at once for
                // people who asked for less motion.
                withAnimation(reduceMotion ? nil : .snappy(duration: 0.22)) { open.toggle() }
            } label: {
                HStack(spacing: 6) {
                    Text(summary).font(.subheadline).foregroundStyle(MycoColor.muted).multilineTextAlignment(.leading)
                    Image(systemName: "chevron.right")
                        .font(.caption2.weight(.semibold))
                        .foregroundStyle(MycoColor.faint)
                        .rotationEffect(.degrees(open ? 90 : 0))
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("steps.summary")

            if open {
                VStack(alignment: .leading, spacing: 7) {
                    ForEach(steps) { step in
                        HStack(alignment: .firstTextBaseline, spacing: 8) {
                            Image(systemName: step.symbol).font(.caption).foregroundStyle(MycoColor.muted).frame(width: 16)
                            if step.prose {
                                Text(step.detail).font(.footnote).foregroundStyle(MycoColor.muted).lineSpacing(2)
                                    .fixedSize(horizontal: false, vertical: true)
                            } else {
                                Text(step.label).font(.footnote).foregroundStyle(MycoColor.fg2)
                                Text(step.detail).font(.footnote.monospaced()).foregroundStyle(MycoColor.muted).lineLimit(1).truncationMode(.middle)
                            }
                            if step.failed {
                                StatusDot(.failed, size: 6)
                            } else if step.running {
                                ProgressView().controlSize(.mini)
                            }
                        }
                    }
                }
                .padding(.leading, 2)
                .transition(.opacity)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// The plan the agent is working through: a short list being ticked off.
public struct PlanCard: View {
    public enum Mark: Hashable, Sendable {
        case done, doing, todo
    }

    public struct Line: Hashable, Sendable {
        public var text: String
        public var mark: Mark

        public init(text: String, mark: Mark) {
            self.text = text
            self.mark = mark
        }
    }

    let title: String
    let count: String
    let lines: [Line]

    public init(title: String, count: String, lines: [Line]) {
        self.title = title
        self.count = count
        self.lines = lines
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(title).font(.footnote.weight(.medium)).foregroundStyle(MycoColor.fg2)
                Spacer()
                Text(count).font(.footnote.monospacedDigit()).foregroundStyle(MycoColor.muted)
            }
            ForEach(Array(lines.enumerated()), id: \.offset) { _, line in
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Image(systemName: line.mark == .done ? "checkmark.square.fill" : "square")
                        .font(.subheadline)
                        .foregroundStyle(line.mark == .done ? MycoColor.muted : (line.mark == .doing ? MycoColor.fg : MycoColor.faint))
                    Text(line.text)
                        .font(.subheadline.weight(line.mark == .doing ? .semibold : .regular))
                        .strikethrough(line.mark == .done)
                        .foregroundStyle(line.mark == .done ? MycoColor.muted : MycoColor.fg)
                }
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(MycoColor.line, lineWidth: 0.5))
        .accessibilityIdentifier("plan.card")
    }
}
