import SwiftUI
import UIKit
import MycoCore
import MycoUI

/// Why something did not work, under what was tried: what happened, what
/// to do, a button to try again where the page can, and the raw reason one
/// tap away (with a copy button) when the words replaced it.
struct FailureLine: View {
    let failure: Failure
    /// What the words are called for a test that looks for them.
    var id = "failure.words"
    var onRetry: (() -> Void)?

    @State private var open = false
    @State private var asked = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(failure.words).font(.footnote).foregroundStyle(MycoColor.red)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier(id)
            if let advice = failure.advice, !advice.isEmpty {
                Text(advice).font(.footnote).foregroundStyle(MycoColor.muted)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if onRetry != nil || failure.detail != nil || failure.kind == .transient {
                HStack(spacing: 14) {
                    if let onRetry {
                        Button(L("重试", "Retry"), action: onRetry)
                            .buttonStyle(QuietButtonStyle())
                            .accessibilityIdentifier("failure.retry")
                    }
                    if failure.detail != nil {
                        Button(open ? L("收起详细信息", "Hide details") : L("详细信息", "Details")) {
                            withAnimation(.snappy(duration: 0.2)) { open.toggle() }
                        }
                        .font(.footnote).foregroundStyle(MycoColor.muted)
                        .buttonStyle(.plain)
                        .frame(minHeight: 30)
                        .accessibilityIdentifier("failure.details")
                    }
                    // A sentence for the person's own agent: the raw error
                    // and where to look.
                    Button(asked ? L("已复制，发给你的 agent", "Copied: send it to your agent") : L("让 agent 帮我查", "Ask my agent")) {
                        UIPasteboard.general.string = AgentSetup.debug(failure.detail ?? failure.text)
                        asked = true
                    }
                    .font(.footnote).foregroundStyle(MycoColor.muted)
                    .buttonStyle(.plain)
                    .frame(minHeight: 30)
                    .accessibilityIdentifier("failure.ask")
                }
            }
            if open, let detail = failure.detail {
                RawReason(detail, copyLabel: L("复制", "Copy"))
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onChange(of: failure) {
            open = false
            asked = false
        }
    }
}

extension StaleBanner.Details {
    /// The raw reason behind "cannot reach the workspace".
    static func of(_ text: String?) -> StaleBanner.Details? {
        guard let text, !text.isEmpty else { return nil }
        return .init(text, show: L("详细信息", "Details"), hide: L("收起", "Hide"), copy: L("复制", "Copy"))
    }
}
