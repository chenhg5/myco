import Foundation
import Testing
@testable import MycoAPI

/// What a real hub (v0.19.2) answered, captured from a throwaway local hub:
/// the app must read every one of these.
struct RealAnswersTests {
    static func fixture(_ name: String) throws -> Data {
        let url = try #require(Bundle.module.url(forResource: name, withExtension: "json", subdirectory: "Fixtures"))
        return try Data(contentsOf: url)
    }

    static func read<T: Decodable>(_ type: T.Type, _ name: String) throws -> T {
        try HubJSON.decoder().decode(type, from: fixture(name))
    }

    @Test func whoIAmAndMyDevice() throws {
        let me = try Self.read(Whoami.self, "me")
        #expect(me.consoleSender == "@console:owner")
        #expect(me.device?.name == "iphone-sim")
        let pair = try Self.read(PairResponse.self, "pair")
        #expect(pair.token == "myco_console_EXAMPLE")
        #expect(pair.device.id.hasPrefix("con_"))
    }

    @Test func theOverview() throws {
        let overview = try Self.read(Overview.self, "overview")
        #expect(overview.agents.map(\.agent.name).sorted() == ["cloud-lead", "studio-claude-c41f", "studio-codex-7f3a"])
        // A hub with no machines leaves the list out.
        #expect(overview.machines.isEmpty)
        let helper = try #require(overview.agents.first { $0.agent.name == "studio-claude-c41f" })
        #expect(helper.state == "pending")
        #expect(helper.current.contains { $0.type == "task" })
    }

    @Test func projectsWithTheirCoordinator() throws {
        let project = try #require(try Self.read(ProjectsEnvelope.self, "projects").projects.first)
        #expect(project.title == "myco 手机端")
        #expect(project.members.first { $0.role == "coordinator" }?.name == "cloud-lead")
    }

    @Test func recentAsPairsAndByProject() throws {
        let pairs = try Self.read(RecentEnvelope.self, "recent-pairs").items
        #expect(pairs.count == 2)
        #expect(pairs.allSatisfy { $0.kind == "pair" && $0.participants.contains("@console:owner") })
        #expect(pairs.first { $0.participants.contains("studio-claude-c41f") }?.open != nil)
        let inProject = try Self.read(RecentEnvelope.self, "recent-project").items
        #expect(inProject.contains { $0.task?.from == "cloud-lead" && $0.task?.to == "studio-codex-7f3a" })
        #expect(try Self.read(RecentEnvelope.self, "recent").items.isEmpty == false)
    }

    @Test func aConversationWithItsProgressAndReply() throws {
        let messages = try Self.read(ExchangeEnvelope.self, "exchanges").messages
        #expect(messages.map(\.type) == ["reply", "progress", "task"])
        #expect(messages[0].replyTo == messages[2].id)
        let open = try Self.read(ExchangeEnvelope.self, "exchanges-open").messages
        #expect(open.map(\.type) == ["steer", "task"])
        #expect(open[1].isOpen)
    }

    @Test func aRunsSteps() throws {
        let trace = try Self.read(Trace.self, "trace")
        #expect(trace.status == "done")
        #expect(trace.events.map(\.kind) == ["text", "tool", "tool_result", "tool", "usage"])
        #expect(trace.events[1].data.id == "c1")
        #expect(trace.events[1].data.input == "git status --short")
    }

    @Test func whatSendingSteeringAndStoppingAnswer() throws {
        #expect(try Self.read(SendEnvelope.self, "send").message.status == "queued")
        #expect(try Self.read(Message.self, "steer").type == "steer")
        let stopped = try Self.read(Message.self, "stop")
        #expect(stopped.status == "failed")
        #expect(stopped.error == "stopped by the sender")
        #expect(try Self.read(HubError.self, "error").error == "not_found")
    }
}

struct AppLinkTests {
    @Test func readsALinkAndTheOpenInAppForm() throws {
        let link = try #require(AppLink("  https://hub.example.com/c/myco_pair_abc123\n"))
        #expect(link.hub.absoluteString == "https://hub.example.com")
        #expect(link.credential == "myco_pair_abc123")
        #expect(link.isPairingCode)

        let wrapped = try #require(AppLink("myco://add-hub?link=https%3A%2F%2Fhub.example.com%2Fc%2Fmyco_console_tok"))
        #expect(wrapped.hub.absoluteString == "https://hub.example.com")
        #expect(!wrapped.isPairingCode)

        // A hub on the local network, on a port, under a path.
        let local = try #require(AppLink("http://192.168.1.20:7777/myco/c/myco_pair_x/"))
        #expect(local.hub.absoluteString == "http://192.168.1.20:7777/myco")
    }

    @Test func refusesWhatIsNotOne() {
        for text in [
            "", "hello", "https://hub.example.com", "https://hub.example.com/c/", "https://hub.example.com/c/secret",
            "https://hub.example.com/j/myco_enroll_abc", "ftp://hub.example.com/c/myco_pair_abc",
            "https://user:pw@hub.example.com/c/myco_pair_abc", "https://hub.example.com/c/myco_pair_abc/extra",
        ] {
            #expect(AppLink(text) == nil, "\(text)")
        }
    }
}

/// The client against a stubbed network: the right calls, with the token,
/// and the hub's errors as they are.
@Suite(.serialized)
struct HubClientTests {
    final class Stub: URLProtocol, @unchecked Sendable {
        nonisolated(unsafe) static var handler: (@Sendable (URLRequest) -> (Int, Data))?
        nonisolated(unsafe) static var seen: [URLRequest] = []

        override class func canInit(with request: URLRequest) -> Bool { true }
        override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
        override func startLoading() {
            Stub.seen.append(request)
            let (status, data) = Stub.handler?(request) ?? (500, Data())
            let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        }
        override func stopLoading() {}
    }

    func client() -> HubClient {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [Stub.self]
        Stub.seen = []
        return HubClient(baseURL: URL(string: "https://hub.example.com/base")!, token: "tok", session: URLSession(configuration: configuration))
    }

    @Test func asksForMyPairsWithTheToken() async throws {
        let hub = client()
        Stub.handler = { _ in (200, try! RealAnswersTests.fixture("recent-pairs")) }
        let items = try await hub.recent(.myPairs)
        #expect(items.count == 2)
        let request = try #require(Stub.seen.first)
        #expect(request.url?.path == "/base/v1/console/recent")
        #expect(request.url?.query == "group=pair&limit=100&side=me")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer tok")
        // A project's threads, and everything this device may read.
        _ = try await hub.recent(.project("myco-ios"))
        _ = try await hub.recent(.everything)
        #expect(Stub.seen.map { $0.url?.query } == ["group=pair&limit=100&side=me", "limit=100&project=myco-ios", "limit=100"])
    }

    @Test func aConsoleSenderSurvivesTheQuery() async throws {
        let hub = client()
        Stub.handler = { _ in (200, try! RealAnswersTests.fixture("exchanges")) }
        let page = try await hub.exchanges(between: "@console:glenn", and: "cloud-lead", before: Date(timeIntervalSince1970: 1_800_000_000.5))
        #expect(page.messages.count == 3)
        #expect(page.more == false)
        let items = URLComponents(url: try #require(Stub.seen.first?.url), resolvingAgainstBaseURL: false)?.queryItems ?? []
        #expect(items.first { $0.name == "a" }?.value == "@console:glenn")
        #expect(items.first { $0.name == "b" }?.value == "cloud-lead")
        // Paging back: the hub takes the time in milliseconds.
        #expect(items.first { $0.name == "before" }?.value == "1800000000500")
    }

    @Test func theHubsErrorComesThroughAsItIs() async throws {
        let hub = client()
        Stub.handler = { _ in (404, try! RealAnswersTests.fixture("error")) }
        await #expect(throws: HubError(error: "not_found", message: "message not found", retryable: false)) {
            _ = try await hub.trace(of: "msg_nope", after: 0)
        }
        // Not JSON at all (a proxy's page): still an error, not a crash.
        Stub.handler = { _ in (502, Data("<html>bad gateway</html>".utf8)) }
        await #expect(throws: HubError.self) { _ = try await hub.whoami() }
    }

    @Test func aFileGoesUpAsItIsUnderItsName() async throws {
        let hub = client()
        Stub.handler = { _ in (201, Data(#"{"id":"f_1","name":"a+b 图.png","size":3,"sha256":"x","content_type":"image/png","owner":"@console:glenn","created_at":"2026-11-13T10:00:00Z"}"#.utf8)) }
        let file = try await hub.upload(Data([1, 2, 3]), name: "a+b 图.png", contentType: "image/png")
        #expect(file == HubFile(id: "f_1", name: "a+b 图.png", size: 3, contentType: "image/png"))
        let request = try #require(Stub.seen.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/base/v1/console/files")
        // The name survives the query: a plus stays a plus, not a space.
        #expect(request.url?.query(percentEncoded: true) == "name=a%2Bb%20%E5%9B%BE.png")
        #expect(request.value(forHTTPHeaderField: "Content-Type") == "image/png")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer tok")
    }

    @Test func aFileComesDownAsItsBytes() async throws {
        let hub = client()
        Stub.handler = { _ in (200, Data([9, 8, 7])) }
        #expect(try await hub.file("f_1") == Data([9, 8, 7]))
        #expect(Stub.seen.first?.url?.path == "/base/v1/console/files/f_1")
        // Gone, or a hub from before this: the hub's own error.
        Stub.handler = { _ in (404, try! RealAnswersTests.fixture("error")) }
        await #expect(throws: HubError.self) { _ = try await hub.file("f_gone") }
    }

    @Test func messagesNameTheirFilesOnlyWhenTheyHaveSome() throws {
        func json(_ request: SendRequest) throws -> [String: Any] {
            try #require(JSONSerialization.jsonObject(with: HubJSON.encoder().encode(request)) as? [String: Any])
        }
        let plain = try json(SendRequest(to: "cloud-lead", body: "hi"))
        #expect(plain["attachments"] == nil)
        let withFiles = try json(SendRequest(to: "cloud-lead", body: "hi", threadId: "t1", attachments: ["f_1", "f_2"]))
        #expect(withFiles["attachments"] as? [String] == ["f_1", "f_2"])
        #expect(withFiles["thread_id"] as? String == "t1")
    }

    @Test func tellsPicturesFromOtherFiles() {
        #expect(Attachment(fileId: "f", name: "x", size: 1, contentType: "image/png").isPicture)
        #expect(Attachment(fileId: "f", name: "x", size: 1, contentType: "image/jpeg; charset=binary").isPicture)
        #expect(!Attachment(fileId: "f", name: "x.png", size: 1, contentType: "application/pdf").isPicture)
        // No type (an older upload): by its name.
        #expect(Attachment(fileId: "f", name: "Shot.JPG", size: 1).isPicture)
        #expect(!Attachment(fileId: "f", name: "notes.md", size: 1).isPicture)
        #expect(!Attachment(fileId: "f", name: "vector.svg", size: 1, contentType: "image/svg+xml").isPicture)
    }

    @Test func pairingSendsTheCodeAndNoToken() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [Stub.self]
        Stub.seen = []
        Stub.handler = { _ in (200, try! RealAnswersTests.fixture("pair")) }
        let paired = try await HubClient.pair(
            hub: URL(string: "https://hub.example.com")!, code: "myco_pair_abc", device: "iphone",
            session: URLSession(configuration: configuration))
        #expect(paired.token == "myco_console_EXAMPLE")
        let request = try #require(Stub.seen.first)
        #expect(request.url?.path == "/v1/console/pair")
        #expect(request.value(forHTTPHeaderField: "Authorization") == nil)
    }
}
