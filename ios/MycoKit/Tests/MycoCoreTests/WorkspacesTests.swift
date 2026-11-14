import Foundation
import Testing
import MycoAPI
import MycoStore
@testable import MycoCore

/// Joining, choosing, renaming and leaving workspaces: several at a time.
/// The hubs are a stubbed network; the stores are in memory.
@Suite(.serialized)
struct WorkspacesTests {
    final class Hubs: URLProtocol, @unchecked Sendable {
        nonisolated(unsafe) static var seen: [(method: String, url: URL, token: String?)] = []
        /// Hosts that refuse every pairing code.
        nonisolated(unsafe) static var refusing: Set<String> = []

        override class func canInit(with request: URLRequest) -> Bool { true }
        override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
        override func startLoading() {
            let url = request.url!
            Hubs.seen.append((request.httpMethod ?? "GET", url, request.value(forHTTPHeaderField: "Authorization")))
            var status = 200
            var body = "{}"
            if url.path.hasSuffix("/v1/console/pair") {
                if Hubs.refusing.contains(url.host ?? "") {
                    status = 401
                    body = #"{"error":"unauthorized","message":"pairing code expired"}"#
                } else {
                    // Every pairing gives a new token.
                    body = #"{"token":"myco_console_\#(Hubs.seen.count)","device":{"id":"con_1","name":"iphone"}}"#
                }
            } else if url.path.hasSuffix("/v1/console/me") {
                body = #"{"member":{"id":"mem_1","name":"glenn","role":"owner","created_at":"2026-11-13T09:21:30.593Z"},"device":{"id":"con_1","name":"iphone"},"can_manage":true}"#
            }
            let response = HTTPURLResponse(url: url, statusCode: status, httpVersion: nil, headerFields: nil)!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: Data(body.utf8))
            client?.urlProtocolDidFinishLoading(self)
        }
        override func stopLoading() {}
    }

    func book() -> (Workspaces, MemoryStore) {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [Hubs.self]
        Hubs.seen = []
        Hubs.refusing = []
        let store = MemoryStore()
        return (Workspaces(records: store, secrets: store, deviceName: "iphone", network: URLSession(configuration: configuration)), store)
    }

    @Test func joiningAnotherKeepsTheOnesJoinedBefore() async throws {
        let (book, store) = book()
        let team = try await book.join(link: "https://team.example.com/c/myco_pair_a")
        let side = try await book.join(link: "https://side.example.com/c/myco_pair_b")
        #expect(await book.all().map(\.name) == ["team.example.com", "side.example.com"])
        #expect(team.id != side.id)
        // Each has its own token, and its client speaks to its own hub with it.
        let teamToken = try #require(await store.secret(for: "workspace.\(team.id).token"))
        let sideToken = try #require(await store.secret(for: "workspace.\(side.id).token"))
        #expect(teamToken != sideToken)
        Hubs.seen = []
        let sideHub = await book.hub(of: side.id)
        _ = try await #require(sideHub).whoami()
        #expect(Hubs.seen.last?.url.host == "side.example.com")
        #expect(Hubs.seen.last?.token == "Bearer " + sideToken)

        // The same workspace again (a new link, to sign in again): the same
        // row with a new token, not a second one.
        let again = try await book.join(link: "https://team.example.com/c/myco_pair_c")
        #expect(again.id == team.id)
        #expect(await book.all().count == 2)
        #expect(await store.secret(for: "workspace.\(team.id).token") != teamToken)

        // A link that is refused changes nothing.
        Hubs.refusing = ["other.example.com"]
        await #expect(throws: JoinError.refused("pairing code expired")) {
            _ = try await book.join(link: "https://other.example.com/c/myco_pair_d")
        }
        await #expect(throws: JoinError.notALink) { _ = try await book.join(link: "hello") }
        #expect(await book.all().count == 2)
    }

    @Test func remembersWhichOneWasOnScreen() async throws {
        let (book, _) = book()
        #expect(await book.selected() == nil)
        let team = try await book.join(link: "https://team.example.com/c/myco_pair_a")
        let side = try await book.join(link: "https://side.example.com/c/myco_pair_b")
        // Nothing chosen yet: the first one joined.
        #expect(await book.selected() == team.id)
        await book.select(side.id)
        #expect(await book.selected() == side.id)

        // Leaving the one on screen: this device signs out there, and the
        // other is still here, and chosen.
        Hubs.seen = []
        await book.leave(side.id)
        #expect(Hubs.seen.contains { $0.method == "DELETE" && $0.url.host == "side.example.com" })
        #expect(await book.all().map(\.id) == [team.id])
        #expect(await book.hub(of: side.id) == nil)
        #expect(await book.selected() == team.id)
        await book.leave(team.id)
        #expect(await book.all().isEmpty)
        #expect(await book.selected() == nil)
    }

    @Test func noTwoReadTheSame() async throws {
        let (book, _) = book()
        // Hosts that are addresses say nothing: "my workspace", then numbered.
        let first = try await book.join(link: "http://192.168.1.20:7777/c/myco_pair_a")
        let second = try await book.join(link: "https://193-46-198-17.sslip.io/c/myco_pair_b")
        // A hub that serves several: the workspace's name in its address.
        let design = try await book.join(link: "https://hub.example.com/w/design/c/myco_pair_c")
        AppLanguage.$forced.withValue(.en) {
            #expect(Workspaces.displayName(of: first) == "My workspace")
            #expect(Workspaces.displayName(of: second) != "My workspace")
            #expect(Workspaces.displayName(of: second).hasSuffix(" 2"))
        }
        #expect(design.name == "design")
        #expect(design.url.absoluteString == "https://hub.example.com/w/design")

        // The person names one: that is what it is called from then on.
        await book.rename(second.id, to: "  家里的  ")
        #expect(await book.all().map(\.name).contains("家里的"))
        await book.rename(second.id, to: "   ")
        #expect(await book.all().first { $0.id == second.id }?.name == "家里的")
    }
}
