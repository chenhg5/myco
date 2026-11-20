import Foundation
import MycoAPI

/// My superagent in this workspace (the desktop app's Superagent.tsx): my
/// own agent with the app's powers, called Shroom when started for it.
/// What it may not do alone (deleting, stopping, opening access, …) waits
/// for me here, to approve or deny.
@MainActor
@Observable
public final class SuperagentModel {
    private let hub: any HubAPI

    /// Its name, if I have one.
    public private(set) var agent: String?
    /// How many of its requests wait for me.
    public private(set) var pending = 0
    /// The requests that wait for me (its, and other agents' such as a
    /// project's coordinator asking for a computer), oldest first.
    public private(set) var approvals: [Approval] = []
    /// Read at least once.
    public private(set) var loaded = false
    /// The workspace is from before superagents (v0.21.1).
    public private(set) var unsupported = false
    public private(set) var failure: Failure?
    /// A request being approved or denied, or a setup going on.
    public private(set) var busy: String?

    /// What a superagent started for me is called.
    public static let shroom = "Shroom"
    /// Where it works on the computer it is started on.
    public static let dir = "~/myco/shroom"

    public init(hub: any HubAPI) {
        self.hub = hub
    }

    public func load() async {
        do {
            let state = try await hub.superagent()
            agent = state.agent?.name
            pending = state.pending ?? 0
            unsupported = false
            // (all that wait for me: a coordinator asking for a computer
            // too, not only my superagent)
            approvals = try await hub.approvals().sorted { $0.createdAt < $1.createdAt }
            failure = nil
        } catch let error as HubError where error.status == 404 || Failure.isTooOld(error) {
            unsupported = true
            agent = nil
            pending = 0
            approvals = []
        } catch is CancellationError {
            return
        } catch {
            failure = Failure.of(error, canRetry: true)
        }
        loaded = true
    }

    /// Approves or denies one of its requests: approved, it is carried out
    /// once, as it came; either way the superagent is told.
    public func decide(_ approval: Approval, approve: Bool) async {
        guard busy == nil else { return }
        busy = approval.id
        defer { busy = nil }
        do {
            let decided = try await hub.decide(approval: approval.id, approve: approve)
            failure = nil
            approvals.removeAll { $0.id == approval.id }
            pending = max(0, pending - 1)
            // Carried out and refused (it went since, I may not after all).
            if approve, let result = decided.result, result >= 400 {
                failure = Failure(L("批准了，但没做成", "Approved, but it did not go through"),
                                  detail: "HTTP \(result) \(decided.resultBody ?? "")")
            }
        } catch {
            failure = Failure.of(error)
        }
        await load()
    }

    /// Makes one of my agents my superagent (`nil`: none). False when it did not work.
    @discardableResult
    public func set(_ name: String?) async -> Bool {
        guard busy == nil else { return false }
        busy = "set"
        defer { busy = nil }
        do {
            try await hub.setSuperagent(name)
            failure = nil
            await load()
            return true
        } catch {
            failure = Failure.of(error)
            return false
        }
    }

    /// Starts a new Shroom on one of my computers and makes it my
    /// superagent: its name, or nil and `failure` says why.
    public func start(on machine: String, cli: String) async -> String? {
        guard busy == nil else { return nil }
        busy = "start"
        defer { busy = nil }
        do {
            let result = try await hub.spawn(on: machine, SpawnRequest(cli: cli, dir: Self.dir, displayName: Self.shroom, access: "workspace"))
            guard let name = result.agent, !name.isEmpty else {
                failure = Failure(L("没开起来", "It did not start"), detail: result.error ?? (result.denied == true ? "denied" : nil))
                return nil
            }
            try await hub.setSuperagent(name)
            failure = nil
            await load()
            return name
        } catch {
            failure = Failure.of(error)
            return nil
        }
    }

    /// How long a request may still wait, in words.
    public static func left(_ approval: Approval, now: Date = Date()) -> String {
        let seconds = approval.expiresAt.timeIntervalSince(now)
        if seconds <= 0 { return L("已过期", "expired") }
        let hours = Int(seconds / 3_600)
        if hours >= 1 { return L("\(hours) 小时内有效", "\(hours)h left") }
        let minutes = max(1, Int((seconds / 60).rounded()))
        return L("\(minutes) 分钟内有效", "\(minutes)m left")
    }
}
