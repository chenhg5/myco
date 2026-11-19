import SwiftUI
import UIKit
import MycoAPI
import MycoCore
import MycoUI

/// What a conversation keeps at hand, one tap away above its messages: a
/// chip a pin, in their order. A tap opens it (a page in the app, another
/// address in the browser, a file full size, a message in place); a long
/// press unpins it or copies its address. The desktop app's `PinBar`.
struct PinStrip: View {
    let pins: PinsModel
    /// Opens a file pinned here.
    let openFile: (Attachment) -> Void
    /// Scrolls to a message pinned here.
    let jump: (String) -> Void
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        if !pins.list.isEmpty {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    ForEach(pins.list) { pin in
                        Button {
                            open(pin)
                        } label: {
                            HStack(spacing: 5) {
                                Image(systemName: Self.symbol(of: pin)).font(.caption.weight(.medium)).foregroundStyle(MycoColor.muted)
                                Text(pin.title).font(.footnote).foregroundStyle(MycoColor.fg).lineLimit(1)
                            }
                            .padding(.horizontal, 11).frame(height: 32)
                            .background(MycoColor.pill, in: Capsule())
                            .contentShape(Capsule())
                        }
                        .buttonStyle(PressScaleStyle())
                        .contextMenu {
                            if pin.kind == "link", let address = pin.target {
                                Button(L("复制链接", "Copy link"), systemImage: "link") { UIPasteboard.general.string = address }
                            }
                            Button(L("取消置顶", "Unpin"), systemImage: "pin.slash", role: .destructive) {
                                Task { await pins.remove(pin) }
                            }
                        }
                        .accessibilityLabel(L("置顶：\(pin.title)", "Pinned: \(pin.title)"))
                        .accessibilityIdentifier("pin." + pin.kind)
                    }
                }
                .padding(.horizontal, Space.page)
                .padding(.vertical, 6)
            }
            .background(MycoColor.bg)
            .accessibilityIdentifier("pins")
        }
    }

    static func symbol(of pin: Pin) -> String {
        switch pin.kind {
        case "link": return "globe"
        case "file": return "doc"
        default: return "text.bubble"
        }
    }

    private func open(_ pin: Pin) {
        switch pin.kind {
        case "link":
            guard let address = pin.target else { return }
            if let id = PageLinks.id(of: address, in: session.pages.workspace) {
                router.page = PageRoute(id: id)
            } else if let url = URL(string: address) {
                UIApplication.shared.open(url)
            }
        case "file":
            if let file = pin.file { openFile(file) }
        default:
            if let id = pin.target { jump(id) }
        }
    }
}

/// Pins an address to a conversation: typed or pasted, with a name if
/// wanted. The desktop app's 置顶一个网址.
struct PinAddressSheet: View {
    let agent: String
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session
    @State private var address = ""
    @State private var title = ""
    @State private var busy = false

    private var pins: PinsModel { session.pins(with: agent) }

    private var usable: URL? {
        let typed = address.trimmingCharacters(in: .whitespaces)
        guard let url = URL(string: typed), ["http", "https"].contains(url.scheme?.lowercased() ?? ""), url.host() != nil else { return nil }
        return url
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("https://…", text: $address)
                        .keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled()
                        .accessibilityIdentifier("pin.address")
                    TextField(L("名字（可不填）", "Name (optional)"), text: $title)
                        .accessibilityIdentifier("pin.title")
                } footer: {
                    Text(L("置顶在这段对话最上面，你和 agent 看到的是同一份。工作区里的网页会在 App 里打开。", "Kept at the top of this conversation, the same for you and the agent. A page of this workspace opens in the app."))
                }
                if let failure = pins.failure {
                    Section { FailureLine(failure: failure) }
                }
            }
            .navigationTitle(L("置顶一个网址", "Pin an address"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button(L("取消", "Cancel")) { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("置顶", "Pin")) {
                        guard let usable else { return }
                        busy = true
                        let name = title.trimmingCharacters(in: .whitespaces)
                        Task {
                            if await pins.pinLink(usable.absoluteString, title: name.isEmpty ? nil : name) { dismiss() }
                            busy = false
                        }
                    }
                    .fontWeight(.semibold)
                    .disabled(usable == nil || busy)
                    .accessibilityIdentifier("pin.save")
                }
            }
            .onAppear {
                pins.forgetFailure()
                // An address just copied is the likely one.
                if address.isEmpty, let copied = UIPasteboard.general.string, copied.hasPrefix("http") { address = copied }
            }
        }
        .presentationDetents([.medium])
    }
}
