import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

// How the things in a workspace are shown: which face, which colour, which
// words. Pages use these, so the same state reads the same everywhere.

extension AvatarView.Face {
    static func of(_ agent: Agent) -> AvatarView.Face {
        let look = MushroomLook(seed: agent.id.isEmpty ? agent.name : agent.id, avatar: agent.avatar)
        switch Naming.kind(of: agent) {
        case .shell, .machine:
            return .computer
        case .human:
            let name = agent.displayName ?? agent.name
            return .person(initial: name.trimmingCharacters(in: .whitespaces).prefix(1).uppercased(), tint: look.capColor)
        case .agent:
            return .mushroom(look)
        }
    }
}

extension AvatarView.Badge {
    static func of(_ agent: Agent) -> AvatarView.Badge? {
        let cli = Naming.cli(of: agent)
        if cli.hasPrefix("claude") { return .claude }
        if cli.hasPrefix("codex") { return .codex }
        return nil
    }
}

extension MushroomLook {
    static func of(_ agent: Agent?) -> MushroomLook {
        guard let agent else { return MushroomLook(seed: "?") }
        return MushroomLook(seed: agent.id.isEmpty ? agent.name : agent.id, avatar: agent.avatar)
    }
}

extension StatusTone {
    /// A task's state. Nothing for one that is done: quiet is the default.
    static func of(_ state: RunState?) -> StatusTone? {
        switch state {
        case .working: return .working
        case .maybeStuck: return .attention
        case .failed: return .failed
        case .queued: return .away
        case .done, .stopped, nil: return nil
        }
    }

    /// A member's state; nothing while it is free.
    static func of(member status: AgentStatus) -> StatusTone? {
        switch MemberState.of(status) {
        case .working: return .working
        case .possiblyStuck, .waiting: return .attention
        case .away: return .away
        case .free: return nil
        }
    }
}

enum Words {
    /// A task's state, in a word.
    static func state(_ state: RunState) -> String {
        switch state {
        case .queued: return L("排队中", "Waiting")
        case .working: return L("在做", "Working")
        case .maybeStuck: return L("可能卡住", "Possibly stuck")
        case .done: return L("做完了", "Done")
        case .stopped: return L("已停止", "Stopped")
        case .failed: return L("没做成", "Did not finish")
        }
    }

    /// What a member is doing, in plain words.
    static func member(_ status: AgentStatus) -> String {
        switch MemberState.of(status) {
        case .working(let text): return text ?? L("在干活", "Working")
        case .possiblyStuck: return L("可能卡住", "Possibly stuck")
        case .waiting: return L("等着接活", "About to start")
        case .away: return L("不在线", "Away")
        case .free: return L("空闲", "Free")
        }
    }

    /// Which AI a member is: "Claude Code · claude-opus-5-5".
    static func ai(_ agent: Agent) -> String {
        let cli = Naming.cli(of: agent)
        let names = ["claude": "Claude Code", "claude-cloud": "Claude (claude.ai)", "codex": "Codex", "codex-cloud": "Codex (cloud)"]
        let name = names[cli] ?? (cli.isEmpty ? L("未上报", "Not reported") : cli)
        guard let model = agent.runtime?.model, !model.isEmpty else { return name }
        return name + " · " + model
    }
}
