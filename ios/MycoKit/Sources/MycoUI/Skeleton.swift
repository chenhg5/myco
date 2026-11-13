import SwiftUI

/// Rows in the shape of what is about to load: a face and two lines. Shown
/// while a list waits for its first answer, instead of a spinner.
public struct SkeletonRows: View {
    let count: Int

    public init(count: Int = 6) {
        self.count = count
    }

    public var body: some View {
        VStack(spacing: 18) {
            ForEach(0..<count, id: \.self) { index in
                HStack(spacing: 12) {
                    RoundedRectangle(cornerRadius: 13, style: .continuous).fill(MycoColor.pill).frame(width: 50, height: 50)
                    VStack(alignment: .leading, spacing: 7) {
                        Capsule().fill(MycoColor.pill).frame(width: index.isMultiple(of: 2) ? 120 : 90, height: 11)
                        Capsule().fill(MycoColor.pill).frame(width: index.isMultiple(of: 3) ? 210 : 170, height: 9)
                    }
                    Spacer(minLength: 0)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 16)
        .padding(.top, 8)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(MycoColor.bg)
        .accessibilityHidden(true)
    }
}
