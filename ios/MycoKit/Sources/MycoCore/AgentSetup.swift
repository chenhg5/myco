import Foundation

/// Sentences for people to hand to their own agent (Claude Code, Codex,
/// claude.ai…): it reads docs/agent-setup.md and sets myco up, or looks
/// into an error, asking before it decides anything for them. The desktop
/// app's `agentSetup.ts`, word for word.
public enum AgentSetup {
    public static let guide = "https://raw.githubusercontent.com/chenhg5/myco/main/docs/agent-setup.md"
    /// The same file through jsDelivr, for networks where GitHub's raw host is slow or blocked.
    public static let mirror = "https://cdn.jsdelivr.net/gh/chenhg5/myco@main/docs/agent-setup.md"

    /// Set myco up from nothing: a hub, the app connected, computers and agents.
    public static var setup: String {
        L("帮我装好 myco：先读 \(guide) （打不开就读 \(mirror) ），照着做。要我决定的先问我，用 sudo 或改系统设置前也先问我，装完告诉我结果。",
          "Set up myco for me: read \(guide) (if it does not load, \(mirror)) and follow it. Ask me before deciding anything for me, before using sudo or changing system settings, and tell me the result when you are done.")
    }

    /// Look into an error the app showed: its raw words, and the guide's Troubleshooting section.
    public static func debug(_ detail: String) -> String {
        let raw = String(detail.split(whereSeparator: \.isWhitespace).joined(separator: " ").prefix(600))
        return L("myco 出错了：「\(raw)」。读 \(guide) （打不开就读 \(mirror) ）里「Troubleshooting」（排错）一节，帮我查原因并修好；用 sudo 或改系统设置前先问我。",
                 "myco shows an error: \"\(raw)\". Read the Troubleshooting section of \(guide) (if it does not load, \(mirror)), find the cause and fix it; ask me before using sudo or changing system settings.")
    }

    /// Add a computer with one's agent: the agent there runs the one-line
    /// command (a credential: the sentence says so) in the folder its
    /// person names, and reads the guide if myco is new to it.
    public static func computer(_ command: String, workspace: String) -> String {
        L("帮我把这台电脑加进 myco 工作区「\(workspace)」：先问我要给 agent 用哪个文件夹，进到那里运行 \(command) ，看到加好了再告诉我。不熟悉 myco 就先读 \(guide) 。这条命令等同凭据，不要发到别处。",
          "Add this computer to the myco workspace \"\(workspace)\": ask me which folder agents should use, run \(command) in it, and tell me once it has joined. New to myco? Read \(guide) first. The command is a credential: do not post it anywhere else.")
    }
}
