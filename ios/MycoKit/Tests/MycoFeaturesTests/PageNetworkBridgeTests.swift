import Foundation
import Testing
import WebKit
import MycoAPI
import MycoCore
import MycoFake
@testable import MycoFeatures

private final class CatalogProtocol: URLProtocol, @unchecked Sendable {
    override class func canInit(with request: URLRequest) -> Bool { request.url?.host() == "catalog.invalid" }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        let query = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "wd" }?.value
        let title = query == "Interstellar" ? "Interstellar" : "No match"
        let data = try! JSONSerialization.data(withJSONObject: ["list": [["vod_id": 1, "vod_name": title]]])
        let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: "HTTP/1.1", headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: data)
        client?.urlProtocolDidFinishLoading(self)
    }
    override func stopLoading() {}
}

@MainActor
private final class NativePageHandler: NSObject, WKScriptMessageHandler {
    let bridge: PageBridge
    weak var view: WKWebView?
    init(bridge: PageBridge) { self.bridge = bridge }
    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let body = message.body as? [String: Any], let id = body["id"] as? Int, let method = body["method"] as? String else { return }
        let args = body["args"] as? [String: Any] ?? [:]
        Task {
            let answer: [String: Any]
            do { answer = ["value": try JSONSerialization.jsonObject(with: await bridge.answer(method, args), options: [.fragmentsAllowed])] }
            catch let error as PageBridge.Refusal { answer = ["error": error.message, "code": error.code ?? "failed"] }
            catch { answer = ["error": "Fixture request failed"] }
            let data = try! JSONSerialization.data(withJSONObject: answer)
            view?.evaluateJavaScript("window.__mycoAnswer(\(id), \(String(decoding: data, as: UTF8.self))); void 0", completionHandler: nil)
        }
    }
}

/// The iPhone's real WebKit + script bridge + native URLSession transport.
/// The page has network access, without claiming desktop OS capabilities.
@MainActor
struct PageNetworkBridgeTests {
    private func evaluate(_ view: WKWebView, _ script: String) async throws -> String {
        try await withCheckedThrowingContinuation { continuation in
            view.evaluateJavaScript(script) { value, error in
                if let error { continuation.resume(throwing: error) }
                else { continuation.resume(returning: value as? String ?? "") }
            }
        }
    }
    private func wait(_ view: WKWebView, _ script: String, for expected: String) async throws {
        for _ in 0..<100 {
            if (try? await evaluate(view, script)) == expected { return }
            try await Task.sleep(for: .milliseconds(100))
        }
        Issue.record("WebKit did not reach \(expected)")
    }

    @Test func changingAPageFragmentKeepsTheDocumentAndEmitsHashchange() async throws {
        let hub = FakeHub()
        let pages = PagesModel(hub: hub)
        let coordinator = PageWebView.Coordinator(id: "pg_checklist", version: 2, pages: pages, bridge: nil)
        coordinator.fragment = "watch?movie=1&episode=2"
        let config = WKWebViewConfiguration()
        config.setURLSchemeHandler(coordinator, forURLScheme: PageLinks.scheme)
        let view = WKWebView(frame: CGRect(x: 0, y: 0, width: 393, height: 852), configuration: config)
        view.navigationDelegate = coordinator
        coordinator.view = view
        defer { view.stopLoading(); view.navigationDelegate = nil }
        let url = try #require(PageLinks.address(of: "pg_checklist", fragment: coordinator.fragment))
        view.load(URLRequest(url: url))
        try await wait(view, "document.readyState", for: "complete")
        try await wait(view, "location.hash", for: "#watch?movie=1&episode=2")
        _ = try await evaluate(view, "window.kept=42;window.hashes=0;window.addEventListener('hashchange',()=>window.hashes++);'ok'")
        let next = try #require(PageLinks.address(of: "pg_checklist", fragment: "watch?movie=2&episode=3%264"))
        #expect(coordinator.navigateWithinDocument(to: next))
        try await wait(view, "String(window.hashes)", for: "1")
        #expect(try await evaluate(view, "location.hash") == "#watch?movie=2&episode=3%264")
        #expect(try await evaluate(view, "String(window.kept)") == "42")
        #expect(view.url?.query == nil)
        // Reopening the exact same public deep link still repositions the page.
        let firstRoute = PageRoute(id: "pg_checklist", fragment: coordinator.fragment)
        let repeatedRoute = PageRoute(id: "pg_checklist", fragment: coordinator.fragment)
        #expect(firstRoute.id == repeatedRoute.id)
        #expect(firstRoute.navigationID != repeatedRoute.navigationID)
        coordinator.navigationID = firstRoute.navigationID
        #expect(coordinator.requestFragment(repeatedRoute.fragment, navigationID: repeatedRoute.navigationID))
        coordinator.applyFragment()
        try await wait(view, "String(window.hashes)", for: "2")
        #expect(try await evaluate(view, "String(window.kept)") == "42")
        // A normal SwiftUI redraw is not a new navigation request.
        #expect(!coordinator.requestFragment(repeatedRoute.fragment, navigationID: repeatedRoute.navigationID))
        coordinator.applyFragment()
        #expect(try await evaluate(view, "String(window.hashes)") == "2")
        // Page-owned navigation must not be undone by an unrelated redraw.
        coordinator.fragment = "watch?movie=3"
        #expect(!coordinator.requestFragment(repeatedRoute.fragment, navigationID: repeatedRoute.navigationID))
        #expect(coordinator.fragment == "watch?movie=3")
        coordinator.fragment = nil
        coordinator.applyFragment()
        try await wait(view, "String(window.hashes)", for: "3")
        #expect(try await evaluate(view, "location.hash") == "")
        #expect(try await evaluate(view, "String(window.kept)") == "42")
    }

    @Test func phoneSearchUsesTheNativeNetworkBridgeWithoutDesktopOS() async throws {
        let hub = FakeHub()
        let page = try #require(try await hub.pages().first)
        let sessionConfig = URLSessionConfiguration.ephemeral
        sessionConfig.protocolClasses = [CatalogProtocol.self]
        let network = PageNetwork(session: URLSession(configuration: sessionConfig))
        let bridge = PageBridge(hub: hub, page: page, me: FakeHub.me, workspace: "net-fixture", names: { $0 }, doors: .init(askToSend: { _, _ in false }, save: { _, _, _ in }, send: { _, _, _ in "unused" }), network: network)
        let handler = NativePageHandler(bridge: bridge)
        let configuration = WKWebViewConfiguration()
        configuration.userContentController.addUserScript(WKUserScript(source: PageBridgeScript.source, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        configuration.userContentController.add(handler, name: "myco")
        let view = WKWebView(frame: CGRect(x: 0, y: 0, width: 393, height: 852), configuration: configuration)
        handler.view = view
        defer { bridge.cancelNetwork(); configuration.userContentController.removeScriptMessageHandler(forName: "myco") }
        view.loadHTMLString(#"""
        <!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body>
        <input id="query" aria-label="Film title"><button id="search" disabled>Search</button><p id="results">Loading</p>
        <script>
        myco.ready.then(()=>{document.querySelector('#search').disabled=!myco.features.includes('net');document.querySelector('#results').textContent='Ready';});
        document.querySelector('#search').onclick=async()=>{try {
          const value=await myco.net.fetch({url:'https://catalog.invalid/api?wd='+encodeURIComponent(document.querySelector('#query').value),response:'text',timeout_ms:2000});
          document.querySelector('#results').textContent=JSON.parse(value.body).list.map(f=>f.vod_name).join(', ');
        }catch(e){document.querySelector('#results').textContent='Failed: '+e.message;}};
        </script></body></html>
        """#, baseURL: URL(string: "https://page.invalid"))
        try await wait(view, "document.querySelector('#results')?.textContent || ''", for: "Ready")
        #expect(try await evaluate(view, "String(!!myco.net && !myco.os)") == "true")
        #expect(try await evaluate(view, "String(document.querySelector('#search').disabled)") == "false")
        _ = try await evaluate(view, "document.querySelector('#query').value='Interstellar';document.querySelector('#search').click();'searched'")
        try await wait(view, "document.querySelector('#results').textContent", for: "Interstellar")
        #expect(try await evaluate(view, "document.querySelector('#results').textContent") == "Interstellar")
    }
}
