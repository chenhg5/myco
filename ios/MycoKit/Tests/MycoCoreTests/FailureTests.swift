import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// Why a request failed, for a person: words, what to do, and the raw
/// reason one tap away.
@MainActor
struct FailureTests {
    @Test func aWorkspaceThatIsDownOrSlowIsSaidInWords() {
        AppLanguage.$forced.withValue(.en) {
            let down = Failure.of(HubError.unreachable("The request timed out."), canRetry: true)
            #expect(down.kind == .transient)
            #expect(down.words == "Cannot reach the workspace right now, or it is answering too slowly.")
            #expect(down.advice?.hasPrefix("Tap Retry to try again.") == true)
            #expect(down.detail == "unreachable\nThe request timed out.")
            // Nothing promises a retry nobody makes.
            let alone = Failure.of(HubError.unreachable("The request timed out."))
            #expect(alone.advice?.hasPrefix("Try again in a moment.") == true)
            #expect(alone.advice?.contains("Retry") == false)
            // A gateway in front of it, by its status; the hub's own word for it.
            let gateway = Failure.of(HubError(error: "http_502", message: "502 Bad Gateway nginx", status: 502))
            #expect(gateway.kind == .transient)
            #expect(gateway.detail == "HTTP 502\n502 Bad Gateway nginx")
            #expect(Failure.of(HubError(error: "unavailable", message: "database is locked", status: 503)).detail == "HTTP 503 · unavailable\ndatabase is locked")
            #expect(Failure.of(HubError(error: "internal", message: "context deadline exceeded", status: 500)).kind == .transient)
        }
    }

    @Test func aWorkspaceTooOldForItSaysSo() {
        AppLanguage.$forced.withValue(.en) {
            let old = Failure.of(HubError(error: "http_405", message: "The workspace answered 405.", status: 405))
            #expect(old.kind == .workspaceTooOld)
            #expect(old.words.contains("older version"))
            #expect(old.detail == "HTTP 405\nThe workspace answered 405.")
            #expect(Failure.of(HubError(error: "not_found", message: "no route for PUT /v1/console/x", status: 404)).kind == .workspaceTooOld)
            #expect(Failure.of(HubError(error: "http_404", message: "The workspace answered 404.", status: 404)).kind == .workspaceTooOld)
            // A thing that is really not there is not that.
            let missing = Failure.of(HubError(error: "not_found", message: "agent x not found", suggestion: "check the name", status: 404))
            #expect(missing.kind == .other)
            #expect(missing.words == "agent x not found")
            #expect(missing.advice == "check the name")
            #expect(missing.detail == nil, "said as it came: nothing more to show")
            #expect(missing.text == "agent x not found check the name")
        }
    }

    @Test func aPageThatIsNotTheHubsKeepsItsFirstWords() {
        let page = Data("<html><head><title>502 Bad Gateway</title></head>\n<body><center><h1>502 Bad Gateway</h1></center>\n<hr><center>nginx</center></body></html>".utf8)
        #expect(HubClient.words(ofPage: page) == "502 Bad Gateway 502 Bad Gateway nginx")
        #expect(HubClient.words(ofPage: Data()) == "")
        #expect(HubClient.words(ofPage: Data(String(repeating: "x", count: 5_000).utf8)).count == 300)
    }

    @Test func aSendThatDidNotGoThroughKeepsTheRawReason() async {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        let model = session.conversation(with: "studio-claude-c41f")
        await model.reload()
        await hub.setUnreachable(true)
        model.draft = "这句发不出去"
        await model.submit()
        #expect(model.failure?.kind == .transient)
        #expect(model.failure?.detail == "unreachable\nThe workspace did not answer.")
        #expect(model.problem == model.failure?.text)
        // The lists say what is under "cannot reach" too (from the
        // second look in a row that failed).
        await session.refresh()
        await session.refresh()
        #expect(session.problem != nil)
        #expect(session.problemDetail == "unreachable\nThe workspace did not answer.")
        await hub.setUnreachable(false)
        await session.refresh()
        #expect(session.problemDetail == nil)
        model.draft = "这句发得出去"
        await model.submit()
        #expect(model.failure == nil)
    }
}

/// Sentences for one's own agent, as the desktop app words them.
struct AgentSetupTests {
    @Test func theSentencesCarryTheGuideAndWhatToAskFirst() {
        AppLanguage.$forced.withValue(.en) {
            #expect(AgentSetup.setup.contains(AgentSetup.guide))
            #expect(AgentSetup.setup.contains(AgentSetup.mirror))
            #expect(AgentSetup.setup.contains("before using sudo"))
            // An error's raw words, on one line, not too long.
            let debug = AgentSetup.debug("HTTP 502\n  502 Bad Gateway   nginx")
            #expect(debug.hasPrefix("myco shows an error: \"HTTP 502 502 Bad Gateway nginx\"."))
            #expect(debug.contains("排错"))
            #expect(AgentSetup.debug(String(repeating: "x", count: 2000)).count < 1000)
            let computer = AgentSetup.computer("curl -fsSL https://hub/j/x | bash", workspace: "Team")
            #expect(computer.contains("\"Team\""))
            #expect(computer.contains("is a credential"))
        }
    }
}
