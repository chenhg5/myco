import SwiftUI

/// The app's tab bar: one row across the whole width, each tab a large
/// target of equal size. It belongs to a tab's first page, so pages pushed
/// on top cover it.
public struct MycoTabBar<ID: Hashable & Sendable>: View {
    public struct Item: Identifiable {
        public var id: ID
        public var title: String
        public var symbol: String
        /// A small dot on the icon: something in this tab is at work, or
        /// needs a look.
        public var tone: StatusTone?
        /// Something unread is in this tab.
        public var unread: Bool
        public var accessibilityID: String

        public init(id: ID, title: String, symbol: String, tone: StatusTone? = nil, unread: Bool = false, accessibilityID: String) {
            self.unread = unread
            self.id = id
            self.title = title
            self.symbol = symbol
            self.tone = tone
            self.accessibilityID = accessibilityID
        }
    }

    let items: [Item]
    let selection: ID
    let onSelect: (ID) -> Void

    public init(items: [Item], selection: ID, onSelect: @escaping (ID) -> Void) {
        self.items = items
        self.selection = selection
        self.onSelect = onSelect
    }

    public var body: some View {
        HStack(spacing: 0) {
            ForEach(items) { item in
                let selected = item.id == selection
                Button {
                    onSelect(item.id)
                } label: {
                    VStack(spacing: 3) {
                        Image(systemName: item.symbol)
                            .symbolVariant(selected ? .fill : .none)
                            .font(.system(size: 22, weight: .regular))
                            .frame(height: 26)
                            .overlay(alignment: .topTrailing) {
                                if item.unread {
                                    Circle().fill(MycoColor.unread).frame(width: 9, height: 9).offset(x: 7, y: -2)
                                } else if let tone = item.tone {
                                    StatusDot(tone, size: 7).offset(x: 6, y: -1)
                                }
                            }
                        Text(item.title).font(.system(size: 11, weight: selected ? .semibold : .regular))
                    }
                    .foregroundStyle(selected ? MycoColor.fg : MycoColor.muted)
                    .frame(maxWidth: .infinity, minHeight: 54)
                    .contentShape(Rectangle())
                }
                .buttonStyle(PressScaleStyle())
                .accessibilityIdentifier(item.accessibilityID)
                .accessibilityAddTraits(selected ? [.isSelected] : [])
            }
        }
        .padding(.top, 2)
        // A translucent layer: the list scrolls on under it, and no hard
        // line divides the two. (The system makes it solid when people ask
        // for less transparency.)
        .background(.bar, ignoresSafeAreaEdges: .bottom)
        .sensoryFeedback(.selection, trigger: selection)
    }
}
