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
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.scenePhase) private var scenePhase
    @AppStorage(AppLanguage.defaultsKey) private var language = "system"
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session

    /// The version on screen (a live page has none).
    @State private var shown: Int?
    @State private var reloads = 0
    /// Asked for, and not there.
    @State private var gone = false
    @State private var copied = false
    /// Answers the page's `window.myco`; made once the page is known.
    @State private var bridge: PageBridge?
    /// The page asks to send its agent a message as me.
    @State private var asking: PageAsking?
    /// A file the page handed over, to save or send on.
    @State private var handed: HandedFile?
    @State private var remoteActive = false
    @State private var remoteKeyboardVisible = false
    @State private var confirmingRemoteExit = false

    private var page: Page? { session.pages.pages[id] }

    private struct PageAsking: Identifiable {
        let id = UUID()
        let title: String
        let message: String
        let accept: String
        let answer: Once
    }

    /// Answered once, whichever way the alert goes away.
    @MainActor
    private final class Once {
        private var waiting: CheckedContinuation<Bool, Never>?

        init(_ waiting: CheckedContinuation<Bool, Never>) {
            self.waiting = waiting
        }

        func resume(returning allowed: Bool) {
            waiting?.resume(returning: allowed)
            waiting = nil
        }
    }

    private struct HandedFile: Identifiable {
        let id = UUID()
        let url: URL
    }

    /// The bridge for this page, as me in this workspace.
    private func makeBridge(for page: Page) -> PageBridge {
        let doors = PageBridge.Doors(
            askToSend: { text, to in
                await withCheckedContinuation { answer in
                    if asking != nil { answer.resume(returning: false); return }
                    asking = PageAsking(
                        title: L("允许这个网页以你的名义发消息吗？", "Let this page send a message as you?"),
                        message: L("它要给 \(to) 发：\(text.prefix(200))\n允许后，这次打开期间不再问。", "It wants to send \(to): \(text.prefix(200))\nOnce allowed, it won't ask again while open."),
                        accept: L("允许", "Allow"), answer: Once(answer))
                }
            },
            save: { name, data, _ in
                let folder = FileManager.default.temporaryDirectory.appendingPathComponent("page-files", isDirectory: true)
                try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
                let safe = name.replacingOccurrences(of: "/", with: "-").trimmingCharacters(in: .whitespaces)
                let url = folder.appendingPathComponent(safe.isEmpty ? "file" : safe)
                guard (try? data.write(to: url, options: .atomic)) != nil else { return }
                handed = HandedFile(url: url)
            },
            send: { to, body, project in
                try await session.hub.send(SendRequest(to: to, body: body, project: project)).id
            },
            askToConnect: { computer, control in
                await withCheckedContinuation { answer in
                    // Read the live application state; this retained closure must not
                    // capture the scene phase from when the page was created.
                    if asking != nil || UIApplication.shared.applicationState != .active { answer.resume(returning: false); return }
                    let capability = control ? L("查看画面并操作键盘和鼠标", "view the screen and control its keyboard and mouse") : L("仅查看画面", "view its screen only")
                    asking = PageAsking(
                        title: L("允许连接这台电脑吗？", "Allow this remote connection?"),
                        message: L("「\(page.name.prefix(80))」请求连接「\(computer.name.prefix(120))」：\(capability)。关闭页面或切到后台会断开。", "“\(page.name.prefix(80))” wants to connect to “\(computer.name.prefix(120))” and \(capability). Closing the page or moving the App to the background disconnects it."),
                        accept: L("连接", "Connect"), answer: Once(answer))
                }
            }, remoteConnectionChanged: { active in
                // Do not re-enter SwiftUI state while its bridge property is borrowed.
                Task { @MainActor in remoteActive = active }
            })
        return PageBridge(
            hub: session.hub, page: page, me: session.me, workspace: session.id,
            names: { session.displayName(of: $0) }, doors: doors)
    }

    var body: some View {
        NavigationStack {
            Group {
                if let page, let bridge, page.live == true || shown != nil {
                    PageWebView(id: id, version: page.live == true ? nil : shown, reloads: reloads, pages: session.pages, bridge: bridge, language: AppLanguage.current.rawValue, theme: colorScheme == .dark ? "dark" : "light", active: scenePhase == .active)
                        .ignoresSafeArea(edges: remoteActive ? .all : .bottom)
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
                if !remoteActive, let page, let shown, page.version > shown {
                    HStack(spacing: Space.s) {
                        Text(L("有新版本：第 \(page.version) 版", "A newer version: \(page.version)")).font(.footnote).foregroundStyle(MycoColor.fg2)
                        Spacer(minLength: 0)
                        Button(L("看新版本", "Show it")) {
                            cancelAsk()
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
            .toolbar(remoteActive ? .hidden : .visible, for: .navigationBar)
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
                    BarTitle(page?.name ?? L("网页", "Page"), subtitle: page.map(title(of:)))
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("page.title")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Button(L("刷新", "Reload"), systemImage: "arrow.clockwise") { cancelAsk(); reloads += 1 }
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
        .alert(
            asking?.title ?? "",
            isPresented: Binding(get: { asking != nil }, set: { if !$0, let asking { asking.answer.resume(returning: false); self.asking = nil } }),
            presenting: asking
        ) { one in
            Button(one.accept) {
                one.answer.resume(returning: true)
                asking = nil
            }
            Button(L("不允许", "Don't allow"), role: .cancel) {
                one.answer.resume(returning: false)
                asking = nil
            }
        } message: { one in
            Text(one.message)
        }
        // The remote display is a fixed canvas. Its keyboard overlays the
        // connection instead of resizing the NavigationStack and WebView.
        .ignoresSafeArea(remoteActive ? .keyboard : [], edges: .bottom)
        .overlay(alignment: remoteKeyboardVisible ? .topLeading : .topTrailing) {
            if remoteActive {
                Button { confirmingRemoteExit = true } label: {
                    Image(systemName: "xmark").font(.system(size: 16, weight: .medium))
                        .foregroundStyle(.white).frame(width: 44, height: 44)
                        .background(.black.opacity(0.65), in: Circle())
                }
                .accessibilityLabel(L("退出远程桌面", "Exit remote desktop"))
                .accessibilityIdentifier("remote.exit")
                .padding(.top, 8).padding(.horizontal, 12)
            }
        }
        .alert(L("断开远程桌面？", "Disconnect remote desktop?"), isPresented: $confirmingRemoteExit) {
            Button(L("继续连接", "Stay connected"), role: .cancel) { }
            Button(L("断开并退出", "Disconnect and exit"), role: .destructive) { dismiss() }
        } message: {
            Text(L("这会结束当前连接。", "This ends the current connection."))
        }
        // Keep the trusted exit away from the page's Send button while typing.
        .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillShowNotification)) { _ in
            remoteKeyboardVisible = true
        }
        .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillHideNotification)) { _ in
            remoteKeyboardVisible = false
        }
        .statusBarHidden(remoteActive)
        .onChange(of: remoteActive) { _, active in RemotePresentation.setActive(active) }
        .onDisappear { cancelAsk(); bridge?.closeRemote(); RemotePresentation.setActive(false) }
        .onChange(of: scenePhase) { _, phase in
            if phase != .active { cancelAsk(); bridge?.suspendRemote() } else { bridge?.resumeRemote() }
        }
        .onChange(of: language) { _, _ in cancelAsk() }
        .sheet(item: $handed) { file in
            ActivityView(items: [file.url])
        }
        // Reads it now, then now and then: a newer version is offered.
        .task {
            while !Task.isCancelled {
                let read = await session.pages.read(id)
                if let read {
                    if let bridge { bridge.update(read) } else { bridge = makeBridge(for: read) }
                    gone = false
                    if shown == nil, read.live != true { shown = read.version }
                } else if page == nil {
                    gone = true
                }
                try? await Task.sleep(for: .seconds(6))
            }
        }
    }

    private func cancelAsk() { asking?.answer.resume(returning: false); asking = nil }

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
    /// Answers the page's `window.myco` calls.
    let bridge: PageBridge?
    let language: String
    let theme: String
    let active: Bool

    func makeCoordinator() -> Coordinator {
        Coordinator(id: id, version: version, pages: pages, bridge: bridge)
    }

    static func dismantleUIView(_ view: WKWebView, coordinator: Coordinator) {
        coordinator.invalidateDocument(closing: true)
        view.stopLoading()
        coordinator.listening?.cancel()
        view.configuration.userContentController.removeScriptMessageHandler(forName: "myco")
    }

    func makeUIView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.setURLSchemeHandler(context.coordinator, forURLScheme: PageLinks.scheme)
        configuration.allowsInlineMediaPlayback = true
        configuration.mediaTypesRequiringUserActionForPlayback = []
        // The bridge: `window.myco` before the page's own scripts, and the
        // app answering it.
        bridge?.setPreferences(language: language, theme: theme)
        if bridge != nil {
            let raw = (try? JSONSerialization.data(withJSONObject: ["language": language, "theme": theme])) ?? Data("{}".utf8)
            let json = String(data: raw, encoding: .utf8) ?? "{}"
            configuration.userContentController.addUserScript(WKUserScript(source: "window.__mycoInitialPreferences = \(json);", injectionTime: .atDocumentStart, forMainFrameOnly: true))
            configuration.userContentController.addUserScript(
                WKUserScript(source: PageBridgeScript.source, injectionTime: .atDocumentStart, forMainFrameOnly: true))
            configuration.userContentController.add(WeakHandler(context.coordinator), name: "myco")
        }
        let view = WKWebView(frame: .zero, configuration: configuration)
        view.navigationDelegate = context.coordinator
        view.uiDelegate = context.coordinator
        view.isOpaque = false
        view.backgroundColor = .clear
        view.scrollView.contentInsetAdjustmentBehavior = .automatic
        view.accessibilityIdentifier = "page.web"
        context.coordinator.reloads = reloads
        context.coordinator.language = language
        context.coordinator.theme = theme
        context.coordinator.active = active
        context.coordinator.view = view
        if let home = PageLinks.address(of: id) { view.load(URLRequest(url: home)) }
        context.coordinator.listen()
        return view
    }

    func updateUIView(_ view: WKWebView, context: Context) {
        let coordinator = context.coordinator
        coordinator.bridge?.setPreferences(language: language, theme: theme)
        let prefsChanged = coordinator.language != language || coordinator.theme != theme
        let reload = coordinator.version != version || coordinator.reloads != reloads || coordinator.language != language
        if coordinator.active != active {
            coordinator.active = active
            if active { coordinator.bridge?.resumeRemote() } else { coordinator.endRemote(); coordinator.bridge?.suspendRemote(); coordinator.bridge?.cancelNetwork() }
        }
        if prefsChanged {
            let raw = (try? JSONSerialization.data(withJSONObject: ["language": language, "theme": theme])) ?? Data("{}".utf8)
            let json = String(data: raw, encoding: .utf8) ?? "{}"
            // This script also initializes a reloaded document before its own code.
            let scripts = view.configuration.userContentController
            scripts.removeAllUserScripts()
            scripts.addUserScript(WKUserScript(source: "window.__mycoInitialPreferences = \(json);", injectionTime: .atDocumentStart, forMainFrameOnly: true))
            scripts.addUserScript(WKUserScript(source: PageBridgeScript.source, injectionTime: .atDocumentStart, forMainFrameOnly: true))
            view.evaluateJavaScript("window.__mycoPreferences && window.__mycoPreferences(\(json)); void 0", completionHandler: nil)
        }
        coordinator.language = language; coordinator.theme = theme
        guard reload else { return }
        coordinator.invalidateDocument()
        coordinator.version = version
        coordinator.reloads = reloads
        // Another version, or asked to: from its first file again.
        if let home = PageLinks.address(of: id) { view.load(URLRequest(url: home)) }
    }

    @MainActor
    final class Coordinator: NSObject, WKURLSchemeHandler, WKNavigationDelegate, WKUIDelegate, WKScriptMessageHandler {
        let id: String
        var version: Int?
        var reloads = 0
        var language = ""
        var theme = ""
        var active = true
        private var documentGeneration = 0
        let pages: PagesModel
        let bridge: PageBridge?
        weak var view: WKWebView?
        /// What others change comes to the page while it is open.
        var listening: Task<Void, Never>?
        /// The requests still wanted: one that was stopped must not be answered.
        private var open: [ObjectIdentifier: Task<Void, Never>] = [:]

        init(id: String, version: Int?, pages: PagesModel, bridge: PageBridge?) {
            self.id = id
            self.version = version
            self.pages = pages
            self.bridge = bridge
        }

        func endRemote() {
            bridge?.resetRemote()
            view?.evaluateJavaScript("window.__mycoEvent && window.__mycoEvent({myco:'remote-disconnect'}); void 0", completionHandler: nil)
        }

        func invalidateDocument(closing: Bool = false) {
            documentGeneration += 1
            bridge?.cancelNetwork()
            if closing {
                bridge?.closeRemote()
                for task in open.values { task.cancel() }
                open.removeAll()
            } else { endRemote() }
        }

        /// A call from the page: answered back into the same document only.
        func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
            guard let bridge, let body = message.body as? [String: Any], let call = body["id"] as? Int, let method = body["method"] as? String,
                  message.frameInfo.isMainFrame
            else { return }
            let args = body["args"] as? [String: Any] ?? [:]
            let generation = documentGeneration
            Task {
                var reply: [String: Any]
                do {
                    let value = try await bridge.answer(method, args)
                    reply = ["value": (try? JSONSerialization.jsonObject(with: value, options: [.fragmentsAllowed])) ?? NSNull()]
                } catch let refusal as PageBridge.Refusal {
                    reply = ["error": refusal.message, "code": refusal.code ?? "error"]
                    if let latest = refusal.latest, let object = try? JSONSerialization.jsonObject(with: latest, options: [.fragmentsAllowed]) {
                        reply["latest"] = object
                    }
                } catch {
                    reply = ["error": error.localizedDescription, "code": "error"]
                }
                guard generation == documentGeneration, let json = try? JSONSerialization.data(withJSONObject: reply, options: [.fragmentsAllowed]),
                      let text = String(data: json, encoding: .utf8)
                else { return }
                // (the callback form: the async one traps on `undefined`)
                view?.evaluateJavaScript("window.__mycoAnswer && window.__mycoAnswer(\(call), \(text)); void 0", completionHandler: nil)
            }
        }

        func webView(_ webView: WKWebView, didStartProvisionalNavigation navigation: WKNavigation!) {
            invalidateDocument()
        }

        /// Every two seconds, while it is open: what others changed.
        func listen() {
            guard let bridge, listening == nil else { return }
            listening = Task { [weak self] in
                while !Task.isCancelled {
                    try? await Task.sleep(for: .seconds(2))
                    for event in await bridge.poll() {
                        guard let text = String(data: event, encoding: .utf8) else { continue }
                        self?.view?.evaluateJavaScript("window.__mycoEvent && window.__mycoEvent(\(text)); void 0", completionHandler: nil)
                    }
                }
            }
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

/// The web view keeps its message handler strongly: this one does not
/// keep the page's coordinator alive.
@MainActor
private final class WeakHandler: NSObject, WKScriptMessageHandler {
    weak var target: (any WKScriptMessageHandler)?

    init(_ target: any WKScriptMessageHandler) {
        self.target = target
    }

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        target?.userContentController(controller, didReceive: message)
    }
}
