import Foundation
import Testing
@testable import MycoCore

@MainActor
struct PageMediaTests {
    private func request(_ target: String = "https://media.example/song.mp3", range: String? = nil) -> URLRequest {
        var parts = URLComponents(string: "myco-page://pg-test" + PageMedia.path)!
        parts.queryItems = [URLQueryItem(name: "url", value: target)]
        var request = URLRequest(url: parts.url!)
        request.setValue(range, forHTTPHeaderField: "Range")
        request.setValue("\"version-1\"", forHTTPHeaderField: "If-Range")
        request.setValue("Bearer never-forward", forHTTPHeaderField: "Authorization")
        return request
    }
    private func client(_ wires: [MediaStub], resolve: @escaping PageMedia.Resolver = { _ in ["8.8.8.8"] }) -> PageMedia {
        var pending = wires
        return PageMedia(resolve: resolve) { _, addresses in
            #expect(addresses.allSatisfy(PageMedia.isPublic))
            return pending.removeFirst()
        }
    }
    private func failure(_ media: PageMedia, request: URLRequest, status: Int) async {
        do {
            try await media.stream(request, response: { _ in Issue.record("Unexpected response") }, data: { _ in Issue.record("Unexpected data") })
            Issue.record("Expected a failure")
        } catch let error as PageMedia.Failure { #expect(error.status == status) }
        catch { Issue.record("Unexpected error: \(error)") }
    }

    @Test func addressesAreCredentialFreeHTTPOnly() throws {
        #expect(try PageMedia.target("https://media.example/song#ignore").absoluteString == "https://media.example/song")
        #expect(try PageMedia.target("HTTPS://media.example/a").scheme == "https")
        for raw in ["file:///etc/passwd", "https://user:password@media.example/a", "https://user@media.example/a", "https://media.example:0/a", String(repeating: "a", count: 4097)] {
            #expect(throws: (any Error).self) { try PageMedia.target(raw) }
        }
    }
    @Test func publicIPPolicyCoversBothFamiliesAndNAT64() {
        for ip in ["0.0.0.0", "10.1.2.3", "127.2.3.4", "172.16.0.1", "192.168.2.1", "169.254.1.1", "100.64.0.1", "100.127.255.255", "224.1.1.1", "255.255.255.255", "::", "::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "fc00::1", "fe80::1", "fec0::1", "ff02::1", "invalid"] {
            #expect(!PageMedia.isPublic(ip), "Must reject \(ip)")
        }
        for ip in ["8.8.8.8", "100.128.0.1", "2606:4700:4700::1111", "::ffff:8.8.8.8", "64:ff9b::808:808"] { #expect(PageMedia.isPublic(ip)) }
    }
    @Test func allResolvedIPsMustBePublicAndLocalNamesNeverResolve() async {
        let media = client([], resolve: { _ in ["8.8.8.8", "192.168.0.1"] })
        await failure(media, request: request(), status: 403)
        let local = client([], resolve: { _ in Issue.record("Local name reached DNS"); return ["8.8.8.8"] })
        for host in ["localhost", "localhost.", "foo.localhost", "printer.local"] {
            await failure(local, request: request("http://\(host)/a"), status: 403)
        }
    }
    @Test func rangesHeadersAndBytesPassThroughWithoutCredentials() async throws {
        let wire = MediaStub(["HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\nContent-Length: 4\r\nContent-Range: bytes 2-5/10\r\nAccept-Ranges: bytes\r\nETag: \"version-1\"\r\nLast-Modified: yesterday\r\nSet-Cookie: secret\r\n\r\na", "bc", "d"])
        let media = client([wire])
        var received = Data(), chunks = 0
        try await media.stream(request(range: "bytes=2-5"), response: {
            #expect($0.statusCode == 206)
            #expect($0.value(forHTTPHeaderField: "Content-Range") == "bytes 2-5/10")
            #expect($0.value(forHTTPHeaderField: "Content-Length") == "4")
            #expect($0.value(forHTTPHeaderField: "Accept-Ranges") == "bytes")
            #expect($0.value(forHTTPHeaderField: "ETag") == "\"version-1\"")
            #expect($0.value(forHTTPHeaderField: "Last-Modified") == "yesterday")
            #expect($0.value(forHTTPHeaderField: "Set-Cookie") == nil)
            #expect(wire.reads == 1) // Headers arrive before the remaining body is read.
        }, data: { received.append($0); chunks += 1 })
        #expect(received == Data("abcd".utf8))
        #expect(chunks == 3)
        #expect(wire.sent.contains("Range: bytes=2-5\r\n"))
        #expect(wire.sent.contains("If-Range: \"version-1\"\r\n"))
        #expect(!wire.sent.contains("Authorization"))
        #expect(wire.closed)
    }
    @Test func htmlAndMissingTypesAreRejectedBeforeAnyResponse() async {
        for type in ["Content-Type: text/html\r\n", ""] {
            await failure(client([MediaStub(["HTTP/1.1 200 OK\r\n\(type)Content-Length: 1\r\n\r\nx"])]), request: request(), status: 415)
        }
    }
    @Test func chunkedMediaAndUnsatisfiableRangeWork() async throws {
        let wire = MediaStub(["HTTP/1.1 200 OK\r\nContent-Type: application/vnd.apple.mpegurl\r\nTransfer-Encoding: chunked\r\n\r\n3\r\na", "bc\r\n2;extra=1\r\nde\r\n0\r\n\r\n"])
        var data = Data()
        try await client([wire]).stream(request(), response: { #expect($0.statusCode == 200) }, data: { data.append($0) })
        #expect(data == Data("abcde".utf8))
        let unsatisfied = MediaStub(["HTTP/1.1 416 Range Not Satisfiable\r\nContent-Type: text/plain\r\nContent-Length: 0\r\nContent-Range: bytes */5\r\n\r\n"])
        try await client([unsatisfied]).stream(request(range: "bytes=99-"), response: {
            #expect($0.statusCode == 416)
            #expect($0.value(forHTTPHeaderField: "Content-Range") == "bytes */5")
        }, data: { _ in Issue.record("Empty response") })
    }
    @Test func redirectDestinationIsResolvedAndCheckedAgain() async {
        let redirect = MediaStub(["HTTP/1.1 302 Found\r\nLocation: https://private.example/a\r\n\r\n"])
        let media = client([redirect], resolve: { host in host == "private.example" ? ["127.0.0.1"] : ["8.8.8.8"] })
        await failure(media, request: request(), status: 403)
        #expect(redirect.closed)
    }
    @Test func pauseClosesUpstreamAndResumesAtForwardedByte() async throws {
        let first = MediaStub(["HTTP/1.1 200 OK\r\nContent-Type: audio/mpeg\r\nContent-Length: 6\r\nETag: \"version-1\"\r\n\r\nab", "unused"])
        let second = MediaStub(["HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\nContent-Length: 4\r\nContent-Range: bytes 2-5/6\r\nETag: \"version-1\"\r\n\r\ncdef"])
        let media = client([first, second]), req = request()
        var body = Data(), responses = 0
        try await media.stream(req, response: { _ in responses += 1 }, data: { chunk in
            body.append(chunk)
            if body.count == 2 {
                media.pause(address: req.url!.absoluteString)
                #expect(first.closed)
                Task { media.resume(address: req.url!.absoluteString) }
            }
        })
        #expect(responses == 1)
        #expect(body == Data("abcdef".utf8))
        #expect(second.sent.contains("Range: bytes=2-5\r\n"))
        #expect(second.sent.contains("If-Range: \"version-1\"\r\n"))
    }
    @Test func cancellationClosesUpstream() async {
        let wire = MediaStub(["HTTP/1.1 200 OK\r\nContent-Type: audio/mpeg\r\nContent-Length: 9\r\n\r\na"])
        let media = client([wire])
        do {
            try await media.stream(request(), response: { _ in }, data: { _ in media.cancel() })
            Issue.record("Cancelled stream finished")
        } catch { #expect(error is CancellationError) }
        #expect(wire.closed)
    }
    @Test func aPublicRedirectPreservesTheRangeRequest() async throws {
        let first = MediaStub(["HTTP/1.1 307 Temporary Redirect\r\nLocation: https://cdn.example/audio.mp3?part=2\r\n\r\n"])
        let second = MediaStub(["HTTP/1.1 206 Partial Content\r\nContent-Type: application/octet-stream\r\nContent-Length: 1\r\nContent-Range: bytes 2-2/3\r\n\r\nx"])
        try await client([first, second]).stream(request(range: "bytes=2-2"), response: { #expect($0.statusCode == 206) }, data: { #expect($0 == Data("x".utf8)) })
        #expect(second.sent.hasPrefix("GET /audio.mp3?part=2 HTTP/1.1\r\nHost: cdn.example\r\n"))
        #expect(second.sent.contains("Range: bytes=2-2\r\n"))
    }
    @Test func stoppingAPausedWebKitResourceCancelsItsWaiter() async {
        let media = client([]), req = request()
        media.pause(address: req.url!.absoluteString)
        let task = Task { try await media.stream(req, response: { _ in Issue.record("Paused response") }, data: { _ in }) }
        await Task.yield()
        task.cancel()
        do { try await task.value; Issue.record("Cancelled request finished") }
        catch { #expect(error is CancellationError) }
    }

    @Test func hostileRangeAndLengthHeadersDoNotOverflow() async {
        for fields in ["Content-Length: 18446744073709551615\r\n", "Content-Length: 1\r\nContent-Range: bytes 9223372036854775807-9223372036854775807/*\r\n", "Content-Length: 1\r\nContent-Range: bytes 2-1/3\r\n", "Content-Length: 1\r\nContent-Length: 1\r\n"] {
            do {
                try await client([MediaStub(["HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\n\(fields)\r\nx"])]).stream(request(), response: { _ in Issue.record("Unsafe headers forwarded") }, data: { _ in })
                Issue.record("Malformed headers accepted")
            } catch { }
        }
    }
}

@MainActor
private final class MediaStub: MediaWire {
    var parts: [Data]
    var sent = "", reads = 0, closed = false
    init(_ parts: [String]) { self.parts = parts.map { Data($0.utf8) } }
    func send(_ data: Data) async throws { sent += String(decoding: data, as: UTF8.self) }
    func read() async throws -> Data {
        try Task.checkCancellation()
        guard !closed else { throw URLError(.cancelled) }
        reads += 1
        return parts.isEmpty ? Data() : parts.removeFirst()
    }
    func close() { closed = true }
}
