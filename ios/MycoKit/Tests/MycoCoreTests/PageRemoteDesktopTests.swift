import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

@MainActor
struct PageRemoteDesktopTests {
    let computer = RemoteComputer(machine: "app:desktop", name: "My Mac", os: "darwin", control: true)
    func request(control: Bool = false) -> [String: Any] {
        ["machine": computer.machine, "name": "untrusted title", "offer": ["description": ["type":"offer", "sdp":"v=0\r\n"], "pairing_code":"test-only", "control":control]]
    }
    @Test func nativeConsentIsRequiredAndTheTargetComesFromTheHub() async throws {
        let hub = FakeHub(); await hub.seedRemote([computer])
        var asked: [String] = []
        let page = PageRemoteDesktop(hub: hub, ask: { computer, _ in asked.append(computer.name); return false })
        await #expect(throws: PageBridge.Refusal.self) { _ = try await page.answer("connect", request()) }
        #expect(asked == ["My Mac"])
        #expect(await hub.remoteOffers.isEmpty)
    }
    @Test func aPageOnlyManagesItsOwnConnections() async throws {
        let hub = FakeHub(); await hub.seedRemote([computer])
        let one = PageRemoteDesktop(hub: hub, ask: { _,_ in true })
        let other = PageRemoteDesktop(hub: hub, ask: { _,_ in true })
        let data = try await one.answer("connect", request())
        let connection = try HubJSON.decoder().decode(RemoteConnection.self, from: data)
        await #expect(throws: PageBridge.Refusal.self) { _ = try await other.answer("session", ["id":connection.id]) }
        await #expect(throws: PageBridge.Refusal.self) { _ = try await other.answer("disconnect", ["id":connection.id]) }
        _ = try await one.answer("session", ["id":connection.id])
        await one.close()
        #expect(await hub.remoteDisconnected == [connection.id])
    }
    @Test func closeBeforeTheConnectResponseRetractsTheLateSession() async throws {
        let hub = FakeHub(); await hub.seedRemote([computer], hold: true)
        let page = PageRemoteDesktop(hub: hub, ask: { _,_ in true })
        let connecting = Task { try await page.answer("connect", request()) }
        while await hub.remoteOffers.isEmpty { await Task.yield() }
        await page.close()
        await hub.releaseRemote()
        let result = await connecting.result
        if case .success = result { Issue.record("Late connection survived closing its page") }
        #expect(await hub.remoteDisconnected == ["rd_1"])
    }
    @Test func suspendDisconnectsAndResumeAllowsAFreshConnection() async throws {
        let hub = FakeHub(); await hub.seedRemote([computer])
        let page = PageRemoteDesktop(hub: hub, ask: { _,_ in true })
        _ = try await page.answer("connect", request())
        await page.suspend()
        #expect(await hub.remoteDisconnected == ["rd_1"])
        await #expect(throws: PageBridge.Refusal.self) { _ = try await page.answer("connect", request()) }
        page.resume()
        _ = try await page.answer("connect", request())
        await page.close()
        page.resume()
        await #expect(throws: PageBridge.Refusal.self) { _ = try await page.answer("devices", [:]) }
    }
    @Test func controlAndHostActionsCannotBypassNativeRules() async throws {
        let hub = FakeHub(); var viewOnly = computer; viewOnly.control = false; await hub.seedRemote([viewOnly])
        var asks = 0
        let page = PageRemoteDesktop(hub: hub, ask: { _,_ in asks += 1; return true })
        await #expect(throws: PageBridge.Refusal.self) { _ = try await page.answer("connect", request(control: true)) }
        for action in ["enable","install","remove","disable","anything"] {
            await #expect(throws: PageBridge.Refusal.self) { _ = try await page.answer(action, [:]) }
        }
        #expect(asks == 0)
        #expect(await hub.remoteOffers.isEmpty)
    }
    @Test func immersivePresentationFollowsOwnedConnectionsAndRevocation() async throws {
        let hub = FakeHub(); await hub.seedRemote([computer])
        var states: [Bool] = []
        let page = PageRemoteDesktop(hub: hub, ask: { _,_ in true }, changed: { states.append($0) })
        let connection = try HubJSON.decoder().decode(RemoteConnection.self, from: try await page.answer("connect", request()))
        #expect(states == [true])
        _ = try await page.answer("disconnect", ["id": connection.id])
        #expect(states == [true, false])
        _ = try await page.answer("connect", request())
        page.suspendNow()
        #expect(states.suffix(2) == [true, false])
    }

}
