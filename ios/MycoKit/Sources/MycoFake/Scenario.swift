import Foundation
import MycoAPI

/// What a fake workspace starts with.
public enum Scenario: Sendable {
    /// A project with a coordinator and members in every state, and two
    /// agents talked to directly.
    case team
    /// The team, plus one very long conversation: for scrolling.
    case long
    /// Joined, but nobody is here yet.
    case empty

    struct World {
        var agents: [AgentStatus] = []
        var machines: [Machine] = []
        var projects: [Project] = []
        var messages: [Message] = []
    }

    func build(now: Date) -> World {
        guard self != .empty else { return World() }
        let me = FakeHub.me
        func ago(_ seconds: TimeInterval) -> Date { now.addingTimeInterval(-seconds) }
        func agent(_ id: String, _ name: String, _ display: String?, _ role: String, cli: String, dir: String, days: Double) -> Agent {
            Agent(
                id: id, name: name, displayName: display, description: role,
                binding: Binding(harness: cli, workdir: dir), createdAt: ago(days * 86_400),
                machineId: "mach_studio", runtime: Runtime(cli: cli, model: cli.hasPrefix("codex") ? "gpt-6-sol" : "claude-opus-5-5"))
        }

        let lead = agent("agt_lead0001", "cloud-lead", "协调者", "拆任务、派活、审查合入", cli: "claude-cloud", dir: "/home/user/myco", days: 12)
        let backend = agent("agt_back0002", "studio-codex-7f3a", "后端工程师", "改 Go：Hub、CLI 和 runner", cli: "codex", dir: "~/code/myco", days: 11)
        let frontend = agent("agt_front003", "studio-claude-2b9c", "前端工程师", "App 界面和构建验证", cli: "claude", dir: "~/code/myco/web", days: 10)
        let tester = agent("agt_test0004", "studio-claude-55d0", "测试", "写测试、跑冒烟、看 CI", cli: "claude", dir: "~/code/myco", days: 9)
        var writer = agent("agt_docs0005", "laptop-claude-a1e2", "文档", "README 和 docs", cli: "claude", dir: "~/code/myco/docs", days: 8)
        writer.online = false
        let ops = agent("agt_ops00006", "vps-codex-90aa", "运维", "看着服务器和发版", cli: "codex", dir: "/srv/myco", days: 7)
        // No display name: the app calls it by its folder.
        let helper = agent("agt_help0007", "studio-claude-c41f", nil, "", cli: "claude", dir: "~/notes", days: 3)
        let fresh = agent("agt_new00008", "studio-codex-e07b", "翻译", "中英文互译", cli: "codex", dir: "~/code/site", days: 1)

        var world = World()
        world.machines = [Machine(id: "mach_studio", name: "Studio Mac", hostname: "studio", os: "darwin")]
        world.agents = [
            AgentStatus(agent: lead, state: "idle", receiving: "session"),
            AgentStatus(agent: backend, state: "working", inProgress: 1),
            AgentStatus(agent: frontend, state: "pending", queued: 1),
            AgentStatus(agent: tester, state: "idle"),
            AgentStatus(agent: writer, state: "offline"),
            AgentStatus(agent: ops, state: "working", inProgress: 1),
            AgentStatus(agent: helper, state: "idle"),
            AgentStatus(agent: fresh, state: "idle"),
        ]
        world.projects = [
            Project(
                name: "myco-ios", title: "myco 手机端", goal: "做出 iOS 第一版，上 TestFlight",
                brief: "## 规则\n- 先跑通关键路径，再接真实数据。\n- 每一步都在真机上看。",
                createdAt: ago(12 * 86_400),
                links: [ProjectLink(title: "仓库", url: "https://github.com/chenhg5/myco")],
                members: [
                    ProjectMember(name: lead.name, role: "coordinator", note: "协调", addedAt: ago(12 * 86_400)),
                    ProjectMember(name: backend.name, note: "推送接口", addedAt: ago(11 * 86_400)),
                    ProjectMember(name: frontend.name, note: "界面", addedAt: ago(10 * 86_400)),
                    ProjectMember(name: tester.name, note: "验证", addedAt: ago(9 * 86_400)),
                    ProjectMember(name: writer.name, note: "文档", addedAt: ago(8 * 86_400)),
                ],
                docs: [ProjectDoc(name: "mobile-ios.md", size: 8_412, updatedAt: ago(3_600))]),
        ]

        let project = "myco-ios"
        world.messages = [
            // Me and the coordinator: a finished round.
            Message(id: "m1", from: me, to: lead.name, body: "把 iOS 第一版的导航定下来，然后分给大家做。", status: "done", project: project, createdAt: ago(3_000)),
            Message(id: "m2", from: lead.name, to: me, threadId: "m1", replyTo: "m1", type: "reply",
                    body: "定了：底部三个 Tab（**聊天、项目、成员**）。\n\n我已经把活分下去了：\n\n1. 后端工程师：推送接口的设计\n2. 前端工程师：对话页\n3. 测试：真机清单\n\n有进展我再告诉你。",
                    status: "done", project: project, createdAt: ago(2_700)),
            // The coordinator's dispatches, in every state.
            Message(id: "d1", from: lead.name, to: backend.name, body: "设计 Hub 的推送接口：设备登记推送令牌、哪些事件要推。\n先出设计，不要直接改。",
                    status: "acked", project: project, createdAt: ago(2_600), updatedAt: ago(2_500), activeAt: ago(40)),
            Message(id: "d2", from: lead.name, to: frontend.name, body: "做对话页：消息列表、输入框、补充和停止。", status: "queued", project: project, createdAt: ago(2_590)),
            Message(id: "d3", from: lead.name, to: tester.name, body: "写一份真机验证清单，按风险排序。", status: "done", project: project, createdAt: ago(2_580), updatedAt: ago(1_200)),
            Message(id: "d3r", from: tester.name, to: lead.name, threadId: "d3", replyTo: "d3", type: "reply",
                    body: "清单写好了：键盘和输入框、聊天滚动、导航返回、运行中补充和停止、空态和离线、浅色深色。",
                    status: "done", project: project, createdAt: ago(1_200)),
            Message(id: "d4", from: lead.name, to: writer.name, body: "把方案写进 docs/design/mobile-ios.md。", status: "failed",
                    error: "the session is gone", project: project, createdAt: ago(2_570), updatedAt: ago(2_000)),
            // Me and ops, directly: taken on two hours ago, quiet since.
            Message(id: "o1", from: me, to: ops.name, body: "看一下 Hub 那台机器的磁盘还剩多少。", status: "acked", createdAt: ago(7_500), updatedAt: ago(7_400), activeAt: ago(7_300)),
            // Me and the notes helper, directly: answered yesterday.
            Message(id: "h1", from: me, to: helper.name, body: "帮我把昨天的会议记录整理成三条结论。", status: "done", createdAt: ago(90_000)),
            Message(id: "h2", from: helper.name, to: me, threadId: "h1", replyTo: "h1", type: "reply",
                    body: "整理好了：\n\n1. iOS 用原生 SwiftUI\n2. 先做跟进和指挥，不做搭建\n3. 推送要一个中转服务",
                    status: "done", createdAt: ago(89_800)),
            // Me and the backend engineer, directly, inside the project.
            Message(id: "b1", from: me, to: backend.name, type: "note", body: "推送中转先别定用哪家，等我回来再说。", status: "done", project: project, createdAt: ago(2_400)),
            Message(id: "b2", from: backend.name, to: me, threadId: "d1", type: "progress", body: "看完了现有的唤醒方式，正在写设备登记的接口。", status: "done", project: project, createdAt: ago(600)),
        ]
        if self == .long {
            // Four hundred messages with the notes helper, oldest first,
            // with the kinds of answers agents give.
            let answers = [
                "好的，记下了。",
                "整理成三条：\n\n1. 先跑通关键路径\n2. 再接真实数据\n3. 每一步都在模拟器里看",
                "这段代码可以这样写：\n\n```swift\nlet rows = session.chatRows.filter { $0.state != nil }\nprint(rows.count)\n```\n\n跑一下就知道了。",
                "## 结论\n\n| 方案 | 好处 | 问题 |\n|---|---|---|\n| 原生 | 手感好 | 两套界面 |\n| 套壳 | 快 | 滚动难做顺 |\n\n> 我建议原生。",
            ]
            for i in 0..<200 {
                let at = ago(Double(86_000 - i * 400))
                world.messages.append(Message(id: "L\(i)q", from: me, to: helper.name, body: "第 \(i + 1) 个问题：这件事接下来怎么做？", status: "done", createdAt: at))
                world.messages.append(Message(
                    id: "L\(i)a", from: helper.name, to: me, threadId: "L\(i)q", replyTo: "L\(i)q", type: "reply",
                    body: i == 199 ? "这是最后一条回复。" : answers[i % answers.count], status: "done", createdAt: at.addingTimeInterval(60)))
            }
        }
        return world
    }
}
