import SwiftUI
import UIKit

/// A row of a list: a face, two lines, and on the right a time and a state.
public struct PersonRow: View {
    let face: AvatarView.Face
    let badge: AvatarView.Badge?
    let title: String
    let subtitle: String
    let trailing: String?
    let tone: StatusTone?
    let away: Bool
    /// Something new is in it: a blue dot, and its words stand out more.
    let unread: Bool
    /// Nothing from it is announced: a small crossed bell by its name.
    let muted: Bool

    public init(
        face: AvatarView.Face, badge: AvatarView.Badge? = nil, title: String, subtitle: String,
        trailing: String? = nil, tone: StatusTone? = nil, away: Bool = false, unread: Bool = false, muted: Bool = false
    ) {
        self.unread = unread
        self.muted = muted
        self.face = face
        self.badge = badge
        self.title = title
        self.subtitle = subtitle
        self.trailing = trailing
        self.tone = tone
        self.away = away
    }

    public var body: some View {
        HStack(spacing: 13) {
            AvatarView(face, size: 50, badge: badge, away: away)
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 5) {
                    Text(title).font(.body.weight(.semibold)).foregroundStyle(MycoColor.fg).lineLimit(1)
                    if muted {
                        Image(systemName: "bell.slash.fill").font(.caption2).foregroundStyle(MycoColor.faint)
                            .accessibilityIdentifier("row.muted")
                    }
                }
                if !subtitle.isEmpty {
                    Text(subtitle).font(.subheadline.weight(unread ? .medium : .regular))
                        .foregroundStyle(unread ? MycoColor.fg : MycoColor.muted).lineLimit(1)
                }
            }
            Spacer(minLength: 8)
            VStack(alignment: .trailing, spacing: 7) {
                if let trailing {
                    Text(trailing).font(.footnote).foregroundStyle(unread ? MycoColor.unread : MycoColor.faint)
                }
                HStack(spacing: 6) {
                    if let tone { StatusDot(tone) }
                    if unread {
                        Circle().fill(MycoColor.unread).frame(width: 10, height: 10)
                            .accessibilityIdentifier("row.unread")
                    }
                }
            }
        }
        .opacity(away ? 0.6 : 1)
        .contentShape(Rectangle())
    }
}

/// A few faces overlapping, for a project's members in a small space.
public struct AvatarStack: View {
    let faces: [AvatarView.Face]
    let size: CGFloat
    let max: Int

    public init(_ faces: [AvatarView.Face], size: CGFloat = 24, max: Int = 4) {
        self.faces = faces
        self.size = size
        self.max = max
    }

    public var body: some View {
        HStack(spacing: -size * 0.28) {
            ForEach(Array(faces.prefix(max).enumerated()), id: \.offset) { _, face in
                AvatarView(face, size: size)
                    .overlay(RoundedRectangle(cornerRadius: size * 0.32, style: .continuous).stroke(MycoColor.bg, lineWidth: 1.5))
            }
            if faces.count > max {
                Text("+\(faces.count - max)")
                    .font(.caption2.weight(.medium))
                    .foregroundStyle(MycoColor.muted)
                    .padding(.leading, size * 0.4)
            }
        }
    }
}

/// A thin strip under the top bar: the workspace cannot be reached, and
/// what is shown is from earlier.
public struct StaleBanner: View {
    /// The raw reason, one tap away, and what its buttons are called.
    public struct Details {
        let text: String
        let show: String
        let hide: String
        let copy: String

        public init(_ text: String, show: String, hide: String, copy: String) {
            self.text = text
            self.show = show
            self.hide = hide
            self.copy = copy
        }
    }

    let text: String
    let details: Details?
    @State private var open = false

    public init(_ text: String, details: Details? = nil) {
        self.text = text
        self.details = details
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                StatusDot(.failed, size: 7)
                Text(text).font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(2)
                Spacer(minLength: 0)
                if let details {
                    Button(open ? details.hide : details.show) { withAnimation(.snappy(duration: 0.2)) { open.toggle() } }
                        .font(.footnote).foregroundStyle(MycoColor.muted)
                        .buttonStyle(.plain)
                        .accessibilityIdentifier("failure.details")
                }
            }
            if open, let details {
                RawReason(details.text, copyLabel: details.copy)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 8)
        .background(MycoColor.redSoft)
    }
}

/// Why something failed, as it came (status, code, message): small, in a
/// fixed-width face, with a button that copies it to pass on.
public struct RawReason: View {
    let text: String
    let copyLabel: String
    @State private var copied = false

    public init(_ text: String, copyLabel: String) {
        self.text = text
        self.copyLabel = copyLabel
    }

    public var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Text(text).font(.caption.monospaced()).foregroundStyle(MycoColor.fg2)
                .textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("failure.detail")
            Button {
                UIPasteboard.general.string = text
                copied = true
            } label: {
                Image(systemName: copied ? "checkmark" : "square.on.square")
                    .font(.system(size: 13, weight: .medium))
                    .foregroundStyle(copied ? MycoColor.green : MycoColor.muted)
                    .frame(width: 28, height: 28)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(copyLabel)
            .accessibilityIdentifier("failure.copy")
        }
        .padding(.leading, 10).padding(.trailing, 4).padding(.vertical, 6)
        .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 10, style: .continuous))
        .sensoryFeedback(.success, trigger: copied) { _, now in now }
    }
}

/// A whole page with nothing in it: a face or symbol, one line, one note,
/// and at most one solid button.
public struct EmptyState: View {
    let face: AvatarView.Face?
    let symbol: String?
    let title: String
    let note: String
    let buttonTitle: String?
    let action: (() -> Void)?

    public init(
        face: AvatarView.Face? = nil, symbol: String? = nil, title: String, note: String = "",
        buttonTitle: String? = nil, action: (() -> Void)? = nil
    ) {
        self.face = face
        self.symbol = symbol
        self.title = title
        self.note = note
        self.buttonTitle = buttonTitle
        self.action = action
    }

    public var body: some View {
        VStack(spacing: 10) {
            if let face {
                AvatarView(face, size: 72).padding(.bottom, 6)
            } else if let symbol {
                Image(systemName: symbol).font(.system(size: 34)).foregroundStyle(MycoColor.faint).padding(.bottom, 6)
            }
            Text(title).font(.headline).foregroundStyle(MycoColor.fg).multilineTextAlignment(.center)
            if !note.isEmpty {
                Text(note).font(.subheadline).foregroundStyle(MycoColor.muted).multilineTextAlignment(.center)
            }
            if let buttonTitle, let action {
                Button(buttonTitle, action: action).buttonStyle(PrimaryButtonStyle()).padding(.top, 10).frame(maxWidth: 260)
            }
        }
        .padding(32)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
