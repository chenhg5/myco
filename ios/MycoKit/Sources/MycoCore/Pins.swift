import Foundation
import MycoAPI

/// What my conversation with an agent keeps at hand above its messages: a
/// web address (a page of this workspace opens in the app, the rest in the
/// browser), a file, or one of its messages. Kept by the workspace, so the
/// agent sees the same list (and pins with `myco pin add`). The desktop
/// app's `pins.tsx`.
@MainActor
@Observable
public final class PinsModel {
    private let hub: any HubAPI
    public let agent: String

    /// In their order; empty on a workspace that keeps none.
    public private(set) var list: [Pin] = []
    /// The workspace keeps pins (from v0.20.3): until it says so, nothing
    /// offers to pin.
    public private(set) var supported = false
    /// Why the last change did not go through.
    public private(set) var failure: Failure?

    /// How often the list is read again while its conversation is open:
    /// what the agent pins comes a while later.
    public static let pollInterval: Duration = .seconds(20)

    public init(hub: any HubAPI, agent: String) {
        self.hub = hub
        self.agent = agent
    }

    public func load() async {
        do {
            let read = try await hub.pins(with: agent)
            if read != list { list = read }
            supported = true
        } catch let error as HubError where error.error == "not_found" || error.error == "not_supported" || error.status == 404 {
            supported = false
            list = []
        } catch {
            // Not reachable now: what is known stays.
        }
    }

    public func run() async {
        while !Task.isCancelled {
            await load()
            try? await Task.sleep(for: Self.pollInterval)
        }
    }

    /// The pin of a link, a file or a message, if it is pinned.
    public func pin(link: String) -> Pin? { list.first { $0.kind == "link" && $0.target == link } }
    public func pin(message id: String) -> Pin? { list.first { $0.kind == "message" && $0.target == id } }

    @discardableResult
    public func add(_ request: PinRequest) async -> Bool {
        do {
            let pin = try await hub.addPin(request)
            if !list.contains(where: { $0.id == pin.id }) { list.append(pin) }
            failure = nil
            return true
        } catch let error as HubError where error.error == "conflict" {
            failure = Failure(L("最多置顶 20 个，先取消几个再加。", "At most 20 pins: unpin some first."))
            return false
        } catch {
            failure = Failure.of(error)
            return false
        }
    }

    public func pinLink(_ address: String, title: String? = nil) async -> Bool {
        await add(PinRequest(with: agent, kind: "link", title: title, target: address))
    }

    public func pinMessage(_ id: String, title: String) async -> Bool {
        await add(PinRequest(with: agent, kind: "message", title: title, target: id))
    }

    public func remove(_ pin: Pin) async {
        let before = list
        list.removeAll { $0.id == pin.id }
        do {
            try await hub.removePin(pin.id, with: agent)
            failure = nil
        } catch {
            list = before
            failure = Failure.of(error)
        }
    }

    public func forgetFailure() {
        failure = nil
    }

    /// A message's first words, as a pin's title: without links and marks,
    /// at most 24 characters.
    public static func title(of body: String) -> String {
        let lines = body.split(whereSeparator: \.isNewline).map { line in
            String(line)
                .replacingOccurrences(of: "\\[([^\\]]*)\\]\\([^)]*\\)", with: "$1", options: .regularExpression)
                .replacingOccurrences(of: "https?://\\S+", with: "", options: .regularExpression)
                .replacingOccurrences(of: "^[#>*\\-\\s`]+", with: "", options: .regularExpression)
                .replacingOccurrences(of: "[*_`\\[\\]]", with: "", options: .regularExpression)
                .replacingOccurrences(of: "[\\s:：,，、]+$", with: "", options: .regularExpression)
                .trimmingCharacters(in: .whitespaces)
        }
        let line = lines.first { !$0.isEmpty } ?? ""
        return line.count > 24 ? String(line.prefix(24)) + "…" : line
    }
}

extension WorkspaceSession {
    /// The pins of my conversation with an agent: one model each, for as
    /// long as the workspace is on screen.
    public func pins(with agent: String) -> PinsModel {
        if let known = pinModels[agent] { return known }
        let model = PinsModel(hub: hub, agent: agent)
        pinModels[agent] = model
        return model
    }
}
