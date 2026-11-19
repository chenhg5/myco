import SwiftUI
import UIKit
import WebKit
import MycoAPI
import MycoCore
import MycoUI

/// A web page an agent published, under the message that links to it:
/// what it is called, which version, who published it and when. A tap
/// opens it in the app. A page that cannot be read gets no card: its link
/// stays a link.
struct PageCard: View {
    let page: Page
    let publisher: String
    /// Pinned above the conversation: a small pin before 打开.
    var pinned = false
    let open: () -> Void

    var body: some View {
        Button(action: open) {
            HStack(spacing: 12) {
                Image(systemName: page.live == true ? "dot.radiowaves.left.and.right" : "macwindow")
                    .font(.system(size: 15, weight: .medium))
                    .foregroundStyle(MycoColor.green)
                    .frame(width: 34, height: 34)
                    .background(MycoColor.greenSoft, in: RoundedRectangle(cornerRadius: 9, style: .continuous))
                VStack(alignment: .leading, spacing: 2) {
                    Text(page.name).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg).lineLimit(1)
                    Text(PageWords.line(page, publisher: publisher)).font(.caption).foregroundStyle(MycoColor.muted).lineLimit(1)
                }
                Spacer(minLength: 4)
                if pinned {
                    Image(systemName: "pin.fill").font(.caption2).foregroundStyle(MycoColor.muted)
                        .accessibilityLabel(L("已置顶", "Pinned"))
                }
                Text(L("打开", "Open")).font(.footnote.weight(.medium)).foregroundStyle(MycoColor.fg2)
                Image(systemName: "chevron.right").font(.caption2.weight(.semibold)).foregroundStyle(MycoColor.faint)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).strokeBorder(MycoColor.line, lineWidth: 1))
            .contentShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        }
        .buttonStyle(PressScaleStyle())
        .accessibilityIdentifier("page.card")
    }
}

/// The cards of the pages a text links to. They are read when the text
/// comes on screen, and again now and then: a page gets new versions.
struct PageCards: View {
    let text: String
    /// The conversation's pins, when a card may be pinned above it.
    var pins: PinsModel?
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        let ids = session.pages.ids(in: text)
        if !ids.isEmpty {
            VStack(spacing: 6) {
                ForEach(ids, id: \.self) { id in
                    if let page = session.pages.pages[id] {
                        let address = page.url ?? ""
                        let pinned = pins?.pin(link: address)
                        PageCard(page: page, publisher: session.displayName(of: page.publisher), pinned: pinned != nil) {
                            router.page = PageRoute(id: id)
                        }
                        .contextMenu {
                            Button(L("打开", "Open"), systemImage: "macwindow") { router.page = PageRoute(id: id) }
                            if let pins, pins.supported, !address.isEmpty {
                                if let pinned {
                                    Button(L("取消置顶", "Unpin"), systemImage: "pin.slash") { Task { await pins.remove(pinned) } }
                                } else {
                                    Button(L("置顶到对话上方", "Pin above the conversation"), systemImage: "pin") {
                                        Task { await pins.pinLink(address, title: page.name) }
                                    }
                                }
                            }
                            if !address.isEmpty {
                                Button(L("复制链接", "Copy link"), systemImage: "link") { UIPasteboard.general.string = address }
                            }
                        }
                    }
                }
            }
            .task(id: ids) {
                while !Task.isCancelled {
                    await session.pages.need(ids)
                    try? await Task.sleep(for: .seconds(20))
                }
            }
        }
    }
}

/// A page, open over the whole screen: its name and version at the top,
/// the page under it. It stays on the version it opened with; a newer one
/// is offered, not swapped in. Links that lead elsewhere open in the
/// system's browser. The desktop app's page panel.
struct PageScreen: View {
    let id: String
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session

    /// The version on screen (a live page has none).
    @State private var shown: Int?
    @State private var reloads = 0
    /// Asked for, and not there.
    @State private var gone = false
    @State private var copied = false

    private var page: Page? { session.pages.pages[id] }

    var body: some View {
        NavigationStack {
            Group {
                if let page, page.live == true || shown != nil {
                    PageWebView(id: id, version: page.live == true ? nil : shown, reloads: reloads, pages: session.pages)
                        .ignoresSafeArea(edges: .bottom)
                } else if gone {
                    EmptyState(
                        symbol: "macwindow", title: L("这个网页打不开了", "This page cannot be opened"),
                        note: L("它可能已经删除了，或者你没有权限看。", "It may have been deleted, or it is not yours to see."))
                        .accessibilityIdentifier("page.gone")
                } else {
                    ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
                }
            }
            .background(MycoColor.bg)
            .safeAreaInset(edge: .top, spacing: 0) {
                if let page, let shown, page.version > shown {
                    HStack(spacing: Space.s) {
                        Text(L("有新版本：第 \(page.version) 版", "A newer version: \(page.version)")).font(.footnote).foregroundStyle(MycoColor.fg2)
                        Spacer(minLength: 0)
                        Button(L("看新版本", "Show it")) {
                            self.shown = page.version
                            reloads += 1
                        }
                        .font(.footnote.weight(.medium)).tint(MycoColor.accent)
                        .accessibilityIdentifier("page.newer.show")
                    }
                    .padding(.horizontal, 16).padding(.vertical, 8)
                    .background(MycoColor.greenSoft)
                    .accessibilityIdentifier("page.newer")
                }
            }
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        dismiss()
                    } label: {
                        Image(systemName: "xmark")
                    }
                    .accessibilityLabel(L("关闭", "Close"))
                    .accessibilityIdentifier("page.close")
                }
                ToolbarItem(placement: .principal) {
                    VStack(spacing: 1) {
                        Text(page?.name ?? L("网页", "Page")).font(.headline).foregroundStyle(MycoColor.fg).lineLimit(1)
                        if let page {
                            Text(title(of: page)).font(.caption).foregroundStyle(MycoColor.muted).lineLimit(1)
                        }
                    }
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("page.title")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Button(L("刷新", "Reload"), systemImage: "arrow.clockwise") { reloads += 1 }
                        if let address = page?.url, !address.isEmpty {
                            Button(copied ? L("已复制", "Copied") : L("复制链接", "Copy link"), systemImage: "link") {
                                UIPasteboard.general.string = address
                                copied = true
                            }
                        }
                    } label: {
                        Image(systemName: "ellipsis")
                    }
                    .accessibilityLabel(L("更多", "More"))
                    .accessibilityIdentifier("page.more")
                }
            }
        }
        // Reads it now, then now and then: a newer version is offered.
        .task {
            while !Task.isCancelled {
                let read = await session.pages.read(id)
                if let read {
                    gone = false
                    if shown == nil, read.live != true { shown = read.version }
                } else if page == nil {
                    gone = true
                }
                try? await Task.sleep(for: .seconds(6))
            }
        }
    }

    /// The version on screen, who published it and when.
    private func title(of page: Page) -> String {
        var one = page
        if let shown, page.live != true {
            // (what is said is the version looked at, not the newest)
            one.version = page.versions.count > 1 || shown > 1 ? shown : 1
            if let version = page.versions.first(where: { $0.n == shown }) { one.updatedAt = version.at ?? page.updatedAt }
        }
        return PageWords.line(one, publisher: session.displayName(of: page.publisher))
    }
}

/// Which page is open over the screen.
public struct PageRoute: Identifiable, Hashable, Sendable {
    public var id: String

    public init(id: String) {
        self.id = id
    }
}

/// The page itself: a web view that loads the page's files from the
/// workspace through the app (a scheme of its own, one origin per page),
/// each with the policy that keeps it to its own files.
private struct PageWebView: UIViewRepresentable {
    let id: String
    /// nil: a live page.
    let version: Int?
    let reloads: Int
    let pages: PagesModel

    func makeCoordinator() -> Coordinator {
        Coordinator(id: id, version: version, pages: pages)
    }

    func makeUIView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.setURLSchemeHandler(context.coordinator, forURLScheme: PageLinks.scheme)
        configuration.allowsInlineMediaPlayback = true
        let view = WKWebView(frame: .zero, configuration: configuration)
        view.navigationDelegate = context.coordinator
        view.uiDelegate = context.coordinator
        view.isOpaque = false
        view.backgroundColor = .clear
        view.scrollView.contentInsetAdjustmentBehavior = .automatic
        view.accessibilityIdentifier = "page.web"
        context.coordinator.reloads = reloads
        if let home = PageLinks.address(of: id) { view.load(URLRequest(url: home)) }
        return view
    }

    func updateUIView(_ view: WKWebView, context: Context) {
        let coordinator = context.coordinator
        guard coordinator.version != version || coordinator.reloads != reloads else { return }
        coordinator.version = version
        coordinator.reloads = reloads
        // Another version, or asked to: from its first file again.
        if let home = PageLinks.address(of: id) { view.load(URLRequest(url: home)) }
    }

    @MainActor
    final class Coordinator: NSObject, WKURLSchemeHandler, WKNavigationDelegate, WKUIDelegate {
        let id: String
        var version: Int?
        var reloads = 0
        let pages: PagesModel
        /// The requests still wanted: one that was stopped must not be answered.
        private var open: [ObjectIdentifier: Task<Void, Never>] = [:]

        init(id: String, version: Int?, pages: PagesModel) {
            self.id = id
            self.version = version
            self.pages = pages
        }

        func webView(_ webView: WKWebView, start task: any WKURLSchemeTask) {
            guard let url = task.request.url, PageLinks.isOwn(url, of: id) else {
                task.didFailWithError(URLError(.unsupportedURL))
                return
            }
            let key = ObjectIdentifier(task)
            let path = String(url.path.drop(while: { $0 == "/" }))
            let version = version, id = id, pages = pages
            open[key] = Task { [weak self] in
                var file: PageFile?
                do {
                    file = try await pages.file(of: id, version: version, path: path)
                } catch {
                    file = nil
                }
                guard let self, self.open[key] != nil, !Task.isCancelled else { return }
                self.open[key] = nil
                let body = file?.data ?? Data()
                let response = HTTPURLResponse(
                    url: url, statusCode: file?.status ?? 404, httpVersion: "HTTP/1.1",
                    headerFields: [
                        "Content-Type": file?.contentType ?? "text/plain; charset=utf-8",
                        "Content-Length": String(body.count),
                        "Content-Security-Policy": PageLinks.contentPolicy,
                        "X-Content-Type-Options": "nosniff",
                        "Cache-Control": "no-store",
                    ])
                guard let response else {
                    task.didFailWithError(URLError(.badServerResponse))
                    return
                }
                task.didReceive(response)
                task.didReceive(body)
                task.didFinish()
            }
        }

        func webView(_ webView: WKWebView, stop task: any WKURLSchemeTask) {
            let key = ObjectIdentifier(task)
            open[key]?.cancel()
            open[key] = nil
        }

        /// The page's own files load here; a link that leads elsewhere
        /// opens in the system's browser, not in this view.
        func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction) async -> WKNavigationActionPolicy {
            guard let url = action.request.url else { return .cancel }
            if PageLinks.isOwn(url, of: id) { return .allow }
            if let scheme = url.scheme, ["about", "data", "blob"].contains(scheme) { return .allow }
            if action.targetFrame?.isMainFrame ?? true, let scheme = url.scheme, ["http", "https", "mailto"].contains(scheme) {
                await UIApplication.shared.open(url)
            }
            return .cancel
        }

        /// A link meant for a new window: there is none; elsewhere goes
        /// to the browser, its own files open here.
        func webView(
            _ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for action: WKNavigationAction,
            windowFeatures: WKWindowFeatures
        ) -> WKWebView? {
            guard let url = action.request.url else { return nil }
            if PageLinks.isOwn(url, of: id) {
                webView.load(URLRequest(url: url))
            } else if let scheme = url.scheme, ["http", "https", "mailto"].contains(scheme) {
                UIApplication.shared.open(url)
            }
            return nil
        }
    }
}
