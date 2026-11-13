import Foundation
import Testing
import MycoAPI

struct DecodingTests {
    @Test func readsAnAgentAsTheHubWritesIt() throws {
        // Go writes nanoseconds, leaves empty fields out, and writes a nil
        // list as null.
        let json = """
        {"id":"agt_1","name":"studio-claude-2b9c","display_name":"前端工程师","queued":0,"in_progress":1,"held":0,
         "description":"App 界面","capabilities":null,"labels":null,
         "binding":{"harness":"claude","session_id":"s1","workdir":"~/code/myco"},
         "delivery":{"mode":"pull"},"allow_from":[],"online":true,
         "created_at":"2026-11-13T03:03:46.677004785Z","kind":"agent","machine_id":"mach_1",
         "runtime":{"cli":"claude","cli_version":"2.1.240","model":"claude-opus-5-5"},"some_future_field":42}
        """
        let agent = try HubJSON.decoder().decode(Agent.self, from: Data(json.utf8))
        #expect(agent.displayName == "前端工程师")
        #expect(agent.labels == [])
        #expect(agent.binding.sessionId == "s1")
        #expect(agent.machineId == "mach_1")
        #expect(agent.runtime?.cliVersion == "2.1.240")
        #expect(abs(agent.createdAt.timeIntervalSince1970 - 1_794_539_026.677) < 0.001)
    }

    @Test func readsTimesWithAndWithoutFractions() throws {
        let whole = try #require(HubJSON.parseDate("2026-11-13T03:14:10Z"))
        let millis = try #require(HubJSON.parseDate("2026-11-13T03:14:10.842Z"))
        let nanos = try #require(HubJSON.parseDate("2026-11-13T03:14:10.842123456Z"))
        let offset = try #require(HubJSON.parseDate("2026-11-13T11:14:10.5+08:00"))
        #expect(abs(millis.timeIntervalSince(whole) - 0.842) < 0.001)
        #expect(abs(nanos.timeIntervalSince(whole) - 0.842) < 0.001)
        #expect(abs(offset.timeIntervalSince(whole) - 0.5) < 0.001)
        #expect(HubJSON.parseDate("yesterday") == nil)
    }

    @Test func readsAMessageWithoutOptionalFields() throws {
        let json = """
        {"id":"msg_1","from":"@console:glenn","to":"coder","thread_id":"msg_1","type":"task","body":"hi",
         "status":"acked","attempts":1,"created_at":"2026-11-13T03:14:10.842Z","updated_at":"2026-11-13T03:14:16Z"}
        """
        let message = try HubJSON.decoder().decode(Message.self, from: Data(json.utf8))
        #expect(message.threadId == "msg_1")
        #expect(message.attachments.isEmpty)
        #expect(message.isOpen)
        #expect(message.replyTo == nil)
    }

    @Test func readsTheHubsStructuredError() throws {
        let json = #"{"error":"not_found","message":"agent bob not found","suggestion":"list agents with: myco agent list","retryable":false}"#
        let error = try HubJSON.decoder().decode(HubError.self, from: Data(json.utf8))
        #expect(error.error == "not_found")
        #expect(error.retryable == false)
    }
}
