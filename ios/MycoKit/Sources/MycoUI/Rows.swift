import SwiftUI

/// A row of a list: a face, two lines, and on the right a time and a state.
public struct PersonRow: View {
    let face: AvatarView.Face
    let badge: AvatarView.Badge?
    let title: String
    let subtitle: String
    let trailing: String?
    let tone: StatusTone?
    let away: Bool

    public init(
        face: AvatarView.Face, badge: AvatarView.Badge? = nil, title: String, subtitle: String,
        trailing: String? = nil, tone: StatusTone? = nil, away: Bool = false
    ) {
        self.face = face
        self.badge = badge
        self.title = title
        self.subtitle = subtitle
        self.trailing = trailing
        self.tone = tone
        self.away = away
    }

    public var body: some View {
        HStack(spacing: 12) {
            AvatarView(face, size: 40, badge: badge, away: away)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg).lineLimit(1)
                if !subtitle.isEmpty {
                    Text(subtitle).font(.subheadline).foregroundStyle(MycoColor.muted).lineLimit(1)
                }
            }
            Spacer(minLength: 8)
            VStack(alignment: .trailing, spacing: 6) {
                if let trailing {
                    Text(trailing).font(.footnote).foregroundStyle(MycoColor.faint)
                }
                if let tone {
                    StatusDot(tone)
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
    let text: String

    public init(_ text: String) {
        self.text = text
    }

    public var body: some View {
        HStack(spacing: 8) {
            StatusDot(.failed, size: 7)
            Text(text).font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(2)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 8)
        .background(MycoColor.redSoft)
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
