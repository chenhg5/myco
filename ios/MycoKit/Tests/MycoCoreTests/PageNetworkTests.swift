import Foundation
import Testing
@testable import MycoCore

/// Real URLSession request/response behavior through a local URLProtocol;
/// no external site, workspace credential or caller's proxy is needed.
private final class PageNetworkProtocol: URLProtocol, @unchecked Sendable {
    override class func canInit(with request: URLRequest) -> Bool { request.url?.host() == "catalog.invalid" }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        let path = request.url!.path
        if path == "/failure" { client?.urlProtocol(self, didFailWithError: URLError(.timedOut)); return }
        if path == "/wait" { return }
        let data: Data
        let type: String
        switch path {
        case "/binary": data = Data([0, 1, 128, 255]); type = "video/mp2t"
        case "/large": data = Data(repeating: 65, count: 16384); type = "text/plain"
        case "/echo":
            data = try! JSONSerialization.data(withJSONObject: ["range": request.value(forHTTPHeaderField: "Range") ?? "", "cookies": request.httpShouldHandleCookies, "method": request.httpMethod ?? "", "userAgent": request.value(forHTTPHeaderField: "User-Agent") ?? ""])
            type = "application/json"
        default: data = Data(#"{"list":[{"vod_id":1,"vod_name":"星际穿越"}]}"#.utf8); type = "application/json"
        }
        let response = HTTPURLResponse(url: request.url!, statusCode: path == "/binary" ? 206 : 200, httpVersion: "HTTP/1.1", headerFields: ["Content-Type": type, "Content-Length": String(data.count)])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: data)
        client?.urlProtocolDidFinishLoading(self)
    }
    override func stopLoading() {}
}

@MainActor
struct PageNetworkTests {
    private func network() -> PageNetwork {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [PageNetworkProtocol.self]
        return PageNetwork(session: URLSession(configuration: configuration))
    }
    private func object(_ data: Data) throws -> [String: Any] { try #require(JSONSerialization.jsonObject(with: data) as? [String: Any]) }

    @Test func catalogSearchAndBinarySegmentsUseTheSameResponseShape() async throws {
        let net = network()
        let text = try object(await net.answer(["url": "https://catalog.invalid/catalog?wd=test", "response": "text"]))
        #expect(text["status"] as? Int == 200)
        #expect(text["encoding"] as? String == "utf8")
        #expect((text["body"] as? String)?.contains("星际穿越") == true)
        let segment = try object(await net.answer(["url": "https://catalog.invalid/binary", "response": "base64"]))
        #expect(segment["status"] as? Int == 206)
        #expect(segment["encoding"] as? String == "base64")
        #expect(Data(base64Encoded: segment["body"] as? String ?? "") == Data([0, 1, 128, 255]))
    }

    @Test func responseCapRangeHeadersAndNoAmbientCookies() async throws {
        let net = network()
        let capped = try object(await net.answer(["url": "https://catalog.invalid/large", "max_bytes": 1024, "response": "text"]))
        #expect(capped["truncated"] as? Bool == true)
        #expect((capped["body"] as? String)?.utf8.count == 1024)
        let response = try object(await net.answer(["url": "https://catalog.invalid/echo", "headers": ["Range": "bytes=0-4095"]]))
        let echo = try object(Data((response["body"] as? String ?? "").utf8))
        #expect(echo["range"] as? String == "bytes=0-4095")
        #expect(echo["cookies"] as? Bool == false)
        #expect(echo["userAgent"] as? String == "myco-app")
    }

    @Test func malformedAddressesAndTimeoutsAreStructured() async throws {
        let net = network()
        for address in ["file:///etc/passwd", "javascript:alert(1)", "https://user:password@catalog.invalid/a"] {
            do { _ = try await net.answer(["url": address]); Issue.record("An invalid address was accepted") }
            catch let refusal as PageBridge.Refusal { #expect(refusal.code == "invalid_argument") }
        }
        do { _ = try await net.answer(["url": "https://catalog.invalid/failure"]); Issue.record("A timed-out request succeeded") }
        catch let refusal as PageBridge.Refusal { #expect(refusal.code == "timeout") }
    }

    @Test func closingThePageCancelsPendingRequests() async throws {
        let net = network()
        let task = Task { try await net.answer(["url": "https://catalog.invalid/wait"]) }
        try await Task.sleep(for: .milliseconds(30))
        net.cancel()
        do { _ = try await task.value; Issue.record("A closed page kept its request") }
        catch let refusal as PageBridge.Refusal { #expect(refusal.code == "cancelled") }
    }
}
