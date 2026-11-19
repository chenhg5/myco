import Foundation
import MycoAPI

/// Why an agent a runner wakes is offline, as far as can be told: its
/// runner said it was stopped; it runs in WSL, which Windows shuts down
/// when idle; on a Mac that left without a goodbye it most likely slept;
/// otherwise it went off or lost its network. Its tasks wait in the queue
/// meanwhile. The desktop app's `offlineWhy`.
public enum OfflineWhy: Sendable, Hashable {
    case stopped, wsl, sleep, gone

    /// Keeps WSL running: a process Windows waits for, started hidden.
    public static let keepWSL = "Start-Process wsl.exe -ArgumentList '-e','sleep','infinity' -WindowStyle Hidden"

    /// Nil for one that is online, or a session in the cloud (it is
    /// offline when its session ends, and comes back with it).
    public static func of(_ status: AgentStatus, machines: [Machine]) -> OfflineWhy? {
        guard status.state == "offline", Naming.kind(of: status.agent) == .agent else { return nil }
        let agent = status.agent
        if (agent.machineId ?? "").isEmpty, Naming.cli(of: agent).hasSuffix("-cloud") { return nil }
        if agent.runtime?.away == "stopped" { return .stopped }
        if agent.runtime?.env == "wsl" { return .wsl }
        if machines.first(where: { $0.id == agent.machineId })?.os == "darwin" { return .sleep }
        return .gone
    }

    public var words: String {
        switch self {
        case .stopped:
            return L("它那边的 myco 后台被停掉了：有人停了服务、注销或关机。", "Its runner was stopped: someone stopped the service, logged out or shut the computer down.")
        case .wsl:
            return L("它在 WSL 里。Windows 在一段时间没有打开的 WSL 窗口后，会自动关掉 WSL，里面的 myco 后台也就停了。在 Windows 的 PowerShell 里运行下面这条，WSL 会一直开着（开机后要再运行一次）。",
                     "It runs in WSL. Windows shuts WSL down after a while with no WSL window open, and the runner in it stops. Run this in a Windows PowerShell to keep WSL running (again after a restart).")
        case .sleep:
            return L("它没有说再见就断了：多半是那台 Mac 睡着了、合上了盖子，或者断网了。电脑醒来后会自己连上。",
                     "It went without saying goodbye: most likely that Mac is asleep, its lid is closed, or it lost its network. It connects again when the Mac wakes.")
        case .gone:
            return L("它没有说再见就断了：多半是那台电脑关机、睡眠或断网了。", "It went without saying goodbye: most likely that computer is off, asleep or offline.")
        }
    }

    /// What happens to what is sent meanwhile.
    public static var queued: String {
        L("离线时发给它的任务会先排队，上线后自动送达。", "Tasks sent while it is offline wait in the queue and arrive once it is back.")
    }
}
