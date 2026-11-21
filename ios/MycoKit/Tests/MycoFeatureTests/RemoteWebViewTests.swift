import Foundation
import WebKit
import Testing
@testable import MycoFeatures

/// Probes the actual iOS WebKit page origin and injected controller bridge,
/// without screen capture, credentials, microphones or an external server.
@MainActor
struct RemoteWebViewTests {
    final class Harness: NSObject, WKURLSchemeHandler, WKNavigationDelegate, WKScriptMessageHandler {
        weak var view: WKWebView?
        var loaded: CheckedContinuation<Void, Never>?
        func webView(_ webView: WKWebView, start task: any WKURLSchemeTask) {
            let data = Data("<!doctype html><html><head><meta name='viewport' content='width=device-width'></head><body>Remote test</body></html>".utf8)
            task.didReceive(URLResponse(url: task.request.url!, mimeType: "text/html", expectedContentLength: data.count, textEncodingName: "utf-8"))
            task.didReceive(data); task.didFinish()
        }
        func webView(_ webView: WKWebView, stop task: any WKURLSchemeTask) {}
        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) { loaded?.resume(); loaded = nil }
        func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
            guard let body = message.body as? [String:Any], let id = body["id"] as? Int else { return }
            let value: String = (body["method"] as? String) == "ready" ? #"{"page":{"id":"pg_test","name":"Remote"},"me":{"name":"Test"},"state":null,"version":1,"preferences":{"theme":"dark","language":"en"}}"# : "[]"
            view?.evaluateJavaScript("window.__mycoAnswer(\(id), {value:\(value)}); void 0", completionHandler: nil)
        }
    }
    @Test(.timeLimit(.minutes(1))) func bridgeAndReceiveOnlyWebRTCWorkInThePageOrigin() async throws {
        let harness = Harness()
        let configuration = WKWebViewConfiguration()
        configuration.setURLSchemeHandler(harness, forURLScheme: "myco-page")
        configuration.mediaTypesRequiringUserActionForPlayback = []
        configuration.userContentController.add(harness, name: "myco")
        configuration.userContentController.addUserScript(WKUserScript(source: PageBridgeScript.source, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        let view = WKWebView(frame: .zero, configuration: configuration)
        harness.view = view; view.navigationDelegate = harness
        await withCheckedContinuation { harness.loaded = $0; view.load(URLRequest(url: URL(string:"myco-page://pg-test/")!)) }
        let bridge = try await view.callAsyncJavaScript("await window.myco.ready; const d = await window.myco.remote.call('devices'); return {controller:window.myco.features.includes('remote-controller'),count:d.length,theme:window.myco.preferences.theme,language:document.documentElement.lang};", arguments: [:], in: nil, contentWorld: .page) as? [String:Any]
        #expect(bridge?["controller"] as? Bool == true)
        #expect(bridge?["count"] as? Int == 0)
        #expect(bridge?["theme"] as? String == "dark")
        #expect(bridge?["language"] as? String == "en")
        let rtc = try await view.callAsyncJavaScript("const p=new RTCPeerConnection({iceServers:[]}); p.addTransceiver('video',{direction:'recvonly'}); const o=await p.createOffer(); p.close(); return o.type;", arguments: [:], in: nil, contentWorld: .page)
        #expect(rtc as? String == "offer")
        configuration.userContentController.removeScriptMessageHandler(forName: "myco")
    }
}
