import Foundation
import MycoAPI

/// One page's remote connections. The page supplies no URL or credential;
/// every request uses the joined workspace and the device's own identity.
@MainActor
public final class PageRemoteDesktop {
    private let hub: any HubAPI
    private let changed: @MainActor (Bool) -> Void
    private let ask: @MainActor (RemoteComputer, Bool) async -> Bool
    private var sessions: Set<String> = []
    private var generation = 0
    private var closed = false
    private var suspended = false
    private var connecting = false

    public init(hub: any HubAPI, ask: @escaping @MainActor (RemoteComputer, Bool) async -> Bool, changed: @escaping @MainActor (Bool) -> Void = { _ in }) {
        self.hub = hub; self.ask = ask; self.changed = changed
    }

    private func json<T: Encodable>(_ value: T) throws -> Data { try HubJSON.encoder().encode(value) }
    private func denied(_ message: String) -> PageBridge.Refusal { PageBridge.Refusal(message: message, code: "permission_denied") }

    public func answer(_ action: String, _ args: [String: Any]) async throws -> Data {
        if action == "close" { await reset(); return Data("true".utf8) }
        guard !closed, !suspended else { throw denied(L("页面已关闭或暂停，请重新连接。", "The page is closed or suspended. Connect again.")) }
        switch action {
        case "status":
            return Data(#"{"protocol":1,"version":"controller","installed":false,"supported":false,"running":false,"reason":"controller_only"}"#.utf8)
        case "devices":
            return try json(await hub.remoteComputers())
        case "connect":
            guard !connecting, sessions.isEmpty else { throw denied(L("请先结束当前连接。", "End the current connection first.")) }
            guard let machine = args["machine"] as? String, let raw = args["offer"], JSONSerialization.isValidJSONObject(raw),
                  let data = try? JSONSerialization.data(withJSONObject: raw), data.count <= 70 << 10,
                  let offer = try? HubJSON.decoder().decode(RemoteOffer.self, from: data), offer.description.type == "offer",
                  offer.description.sdp.hasPrefix("v=0"), offer.pairingCode.count <= 32 else {
                throw PageBridge.Refusal(message: L("连接请求无效。", "Invalid connection request."), code: "invalid_argument")
            }
            connecting = true
            defer { connecting = false }
            let epoch = generation
            guard let computer = try await hub.remoteComputers().first(where: { $0.machine == machine }) else {
                throw PageBridge.Refusal(message: L("这台电脑已停止共享。", "This computer stopped sharing."), code: "not_found")
            }
            guard !offer.control || computer.control else { throw denied(L("这台电脑尚未允许键鼠控制。", "This computer has not allowed keyboard and mouse control.")) }
            guard epoch == generation, !closed, !suspended, await ask(computer, offer.control),
                  epoch == generation, !closed, !suspended else {
                throw denied(L("连接未获允许。", "The connection was not allowed."))
            }
            let result = try await hub.connectRemote(machine: machine, offer: offer)
            guard epoch == generation, !closed, !suspended else {
                try? await hub.disconnectRemote(result.id)
                throw denied(L("页面已关闭，连接已取消。", "The page closed; the connection was cancelled."))
            }
            sessions.insert(result.id)
            changed(true)
            return try json(result)
        case "session", "disconnect":
            guard let id = args["id"] as? String, sessions.contains(id) else { throw denied(L("这个连接不属于此页面。", "This connection does not belong to this page.")) }
            if action == "disconnect" {
                sessions.remove(id)
                if sessions.isEmpty { changed(false) }
                try await hub.disconnectRemote(id)
                return Data("true".utf8)
            }
            return try json(await hub.remoteConnection(id))
        default:
            throw denied(L("手机只能连接电脑，不能安装组件或共享屏幕。", "This phone can connect to computers; it cannot install a host component or share a screen."))
        }
    }

    /// Revocation is synchronous, before the first network suspension. A late
    /// connect response is retracted even when dismissal arrives first.
    public func reset() async {
        generation += 1
        let ids = sessions; sessions.removeAll(); if !ids.isEmpty { changed(false) }
        for id in ids { try? await hub.disconnectRemote(id) }
    }
    public func resetNow() {
        generation += 1
        let ids = sessions; sessions.removeAll(); if !ids.isEmpty { changed(false) }
        let hub = hub
        Task { for id in ids { try? await hub.disconnectRemote(id) } }
    }
    public func closeNow() { closed = true; resetNow() }
    public func suspendNow() { suspended = true; resetNow() }
    public func close() async { closed = true; await reset() }
    public func suspend() async { suspended = true; await reset() }
    public func resume() { if !closed { suspended = false } }
}
