import Foundation
import MycoAPI

/// What a fake workspace starts with.
public enum Scenario: Sendable {
    /// A project with a coordinator and members in every state, and two
    /// agents talked to directly.
    case team
    /// The team, plus one very long conversation: for scrolling.
    case long
    /// The team, with runs that are open but not at work: one set aside,
    /// one waiting for work it handed out.
    case waiting
    /// The team, with an agent in one long session (it took my second
    /// message and goes on, its steps still filed under the first), and a
    /// message that was never delivered.
    case session
    /// Native WebKit/controller consent probe, with no real computer or token.
    case remote
    /// The team, with a page that reports deep links into it, linked by
    /// 运维 with a fragment (for UI tests).
    case links
    /// The team, with a page that plays a public podcast through the
    /// phone's network (for UI tests; needs the internet).
    case media
    /// The team, with 运维 answering in structured parts: code, a
    /// diagram, a myco-ui block and pictures placed in its text.
    case rich
    /// Joined, but nobody is here yet.
    case empty

    struct World {
        var agents: [AgentStatus] = []
        var machines: [Machine] = []
        var projects: [Project] = []
        var messages: [Message] = []
        var files: [(file: HubFile, data: Data)] = []
        /// Tasks whose session ended its turn: open, but waiting.
        var turnEnded: Set<String> = []
        /// Tasks with no steps of their own: taken up in a session that
        /// files what it does under an earlier task.
        var silent: Set<String> = []
        /// What is sent for me on a schedule, or when something calls.
        var automations: [Automation] = []
        /// The times another session took an agent over.
        var handovers: [Handover] = []
        /// What each conversation keeps above its messages, by agent.
        var pins: [String: [Pin]] = [:]
        /// Apps pages can be made from, with their files by path.
        var apps: [(app: AppTemplate, files: [String: (type: String, text: String)])] = []
        /// Web pages agents published, with their files by path.
        var pages: [(page: Page, files: [String: (type: String, text: String)])] = []
        /// Workflows, and their runs (newest first).
        var workflows: [Workflow] = []
        var runs: [WorkflowRun] = []
    }

    func build(now: Date) -> World {
        guard self != .empty else { return World() }
        let me = FakeHub.me
        func ago(_ seconds: TimeInterval) -> Date { now.addingTimeInterval(-seconds) }
        func agent(_ id: String, _ name: String, _ display: String?, _ role: String, cli: String, dir: String, days: Double) -> Agent {
            Agent(
                id: id, name: name, displayName: display, description: role,
                binding: Binding(harness: cli, workdir: dir), createdAt: ago(days * 86_400),
                // (a session in the cloud runs on no computer of mine)
                machineId: cli.hasSuffix("-cloud") ? nil : "mach_studio",
                runtime: Runtime(
                    cli: cli, model: cli.hasPrefix("codex") ? "gpt-6-sol" : "claude-opus-5-5", modelSource: "observed",
                    effort: cli.hasPrefix("codex") ? "high" : nil))
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
        world.machines = [
            Machine(id: "mach_studio", name: "Studio Mac", hostname: "studio", os: "darwin", canSpawn: true, mycoVersion: "v0.19.25"),
        ]
        world.agents = [
            AgentStatus(agent: lead, state: "idle", receiving: "session"),
            AgentStatus(agent: backend, state: "working", inProgress: 1),
            AgentStatus(agent: frontend, state: "pending", queued: 1),
            AgentStatus(agent: tester, state: "idle"),
            // (its runner said goodbye as it was stopped)
            AgentStatus(agent: { var one = writer; one.runtime?.away = "stopped"; return one }(), state: "offline"),
            AgentStatus(agent: ops, state: "working", inProgress: 1),
            AgentStatus(agent: helper, state: "idle"),
            AgentStatus(agent: fresh, state: "idle"),
        ]
        world.projects = [
            Project(
                name: "myco-ios", title: "myco 手机端", goal: "做出 iOS 第一版，上 TestFlight",
                brief: "## 规则\n- 先跑通关键路径，再接真实数据。\n- 每一步都在真机上看。",
                createdAt: ago(12 * 86_400),
                links: [ProjectLink(title: "仓库", url: "https://github.com/myco-ai/myco")],
                members: [
                    ProjectMember(name: lead.name, role: "coordinator", note: "协调", addedAt: ago(12 * 86_400)),
                    ProjectMember(name: backend.name, note: "推送接口", addedAt: ago(11 * 86_400), group: "后端"),
                    ProjectMember(name: frontend.name, note: "界面", addedAt: ago(10 * 86_400), group: "前端"),
                    ProjectMember(name: tester.name, note: "验证", addedAt: ago(9 * 86_400), group: "质量"),
                    ProjectMember(name: writer.name, note: "文档", addedAt: ago(8 * 86_400), group: "质量"),
                ],
                docs: [ProjectDoc(name: "mobile-ios.md", size: 8_412, updatedAt: ago(3_600))]),
        ]
        // Its members in groups (the coordinator in none).
        world.projects[0].groups = ["前端", "后端", "质量"]

        // A few files, to see messages that carry them.
        func file(_ id: String, _ name: String, _ type: String, _ data: Data) -> Attachment {
            world.files.append((HubFile(id: id, name: name, size: Int64(data.count), contentType: type), data))
            return Attachment(fileId: id, name: name, size: Int64(data.count), contentType: type)
        }
        let sketch1 = file("f_sketch1", "导航草图-1.png", "image/png", SamplePicture.png(hue: 0.38, width: 600, height: 800))
        let sketch2 = file("f_sketch2", "导航草图-2.png", "image/png", SamplePicture.png(hue: 0.58, width: 800, height: 600))
        let sketch3 = file("f_sketch3", "导航草图-3.png", "image/png", SamplePicture.png(hue: 0.08, width: 700, height: 700))
        let board = file("f_board", "白板.png", "image/png", SamplePicture.png(hue: 0.72, width: 900, height: 600))
        let notes = file(
            "f_notes", "会议结论.md", "text/markdown",
            Data("# 会议结论\n\n1. iOS 用原生 SwiftUI\n2. 先做跟进和指挥，不做搭建\n3. 推送要一个中转服务\n".utf8))

        let project = "myco-ios"
        world.messages = [
            // Me and the coordinator: a finished round.
            Message(id: "m1", from: me, to: lead.name, body: "把 iOS 第一版的导航定下来，然后分给大家做。", status: "done", project: project, createdAt: ago(3_000)),
            Message(id: "m2", from: lead.name, to: me, threadId: "m1", replyTo: "m1", type: "reply",
                    body: "定了：底部三个 Tab（**聊天、项目、成员**）。\n\n我已经把活分下去了：\n\n1. 后端工程师：推送接口的设计\n2. 前端工程师：对话页\n3. 测试：真机清单\n\n有进展我再告诉你。",
                    status: "done", project: project, createdAt: ago(2_700), attachments: [sketch1, sketch2, sketch3]),
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
            // The backend engineer reports to the coordinator as it goes.
            Message(id: "d1p1", from: backend.name, to: lead.name, threadId: "d1", replyTo: "d1", type: "progress",
                    body: "看了现有的唤醒方式：webhook、GitHub、Claude 三种。", status: "done", project: project, createdAt: ago(1_800)),
            Message(id: "d1p2", from: backend.name, to: lead.name, threadId: "d1", replyTo: "d1", type: "progress",
                    body: "设备登记的接口定了：\n\n- `PUT /v1/console/me/push`\n- 只存令牌，不存内容", status: "done", project: project, createdAt: ago(900)),
            Message(id: "d1p3", from: backend.name, to: lead.name, threadId: "d1", replyTo: "d1", type: "progress",
                    body: "正在写中转服务的设计。", status: "done", project: project, createdAt: ago(40)),
            // Me and ops, directly: taken on two hours ago, quiet since.
            Message(id: "o1", from: me, to: ops.name, body: "看一下 Hub 那台机器的磁盘还剩多少。", status: "acked", createdAt: ago(7_500), updatedAt: ago(7_400), activeAt: ago(7_300)),
            // Me and the notes helper, directly: answered yesterday.
            Message(id: "h1", from: me, to: helper.name, body: "帮我把昨天的会议记录整理成三条结论。", status: "done", createdAt: ago(90_000), attachments: [board]),
            Message(id: "h2", from: helper.name, to: me, threadId: "h1", replyTo: "h1", type: "reply",
                    body: "整理好了：\n\n1. iOS 用原生 SwiftUI\n2. 先做跟进和指挥，不做搭建\n3. 推送要一个中转服务",
                    status: "done", createdAt: ago(89_800), attachments: [notes]),
            // Me and the backend engineer, directly, inside the project.
            Message(id: "b1", from: me, to: backend.name, type: "note", body: "推送中转先别定用哪家，等我回来再说。", status: "done", project: project, createdAt: ago(2_400)),
            Message(id: "b2", from: backend.name, to: me, threadId: "d1", replyTo: "d1", type: "progress", body: "看完了现有的唤醒方式，正在写设备登记的接口。", status: "done", project: project, createdAt: ago(600)),
        ]
        // The notes helper offers what I may want to say next.
        if let index = world.messages.firstIndex(where: { $0.id == "h1" }) {
            world.messages[index].suggest = "把这三条结论发到项目里"
        }
        if self == .waiting {
            world.messages += [
                // Set aside by the notes helper: it answers when I send more.
                Message(id: "w1", from: me, to: helper.name, body: "第二场会议也整理一下，录音我晚点发你。", status: "held",
                        createdAt: ago(600), updatedAt: ago(560)),
                Message(id: "w1p", from: helper.name, to: me, threadId: "w1", replyTo: "w1", type: "progress",
                        body: "先放着了，等你的录音。", status: "done", createdAt: ago(570)),
                // The coordinator handed a review to the tester and waits for it.
                Message(id: "m3", from: me, to: lead.name, body: "把推送接口的评审安排一下。", status: "acked", project: project,
                        createdAt: ago(500), updatedAt: ago(480), activeAt: ago(100)),
                Message(id: "d5", from: lead.name, to: tester.name, body: "评审推送接口的设计，看有没有漏掉的情况。", status: "acked", project: project,
                        createdAt: ago(450), updatedAt: ago(440), activeAt: ago(60)),
                Message(id: "m3p", from: lead.name, to: me, threadId: "m3", replyTo: "m3", type: "progress",
                        body: "已经请测试来评审，等他看完我再汇总。", status: "done", project: project, createdAt: ago(430)),
            ]
            // Having handed the review out, the coordinator ended its turn.
            world.turnEnded = ["m3"]
        }
        // A page the tester published: its checklist, as a web page.
        let checklist = Page(
            id: "pg_checklist", name: "真机验证清单", project: project, publisher: tester.name, version: 2,
            url: FakeHub.demoAddress.absoluteString + "/pages/pg_checklist",
            versions: [PageVersion(n: 2, by: tester.name, at: ago(1_100)), PageVersion(n: 1, by: tester.name, at: ago(4_000))],
            createdAt: ago(4_000), updatedAt: ago(1_100))
        world.pages = [(checklist, [
            "index.html": ("text/html; charset=utf-8", """
            <!doctype html><html lang="zh"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
            <title>真机验证清单</title><link rel="stylesheet" href="style.css"></head>
            <body><h1>真机验证清单</h1><p class="v">第 {{version}} 版</p>
            <ul><li><label><input type="checkbox" checked> 键盘和输入框</label></li>
            <li><label><input type="checkbox" checked> 聊天滚动</label></li>
            <li><label><input type="checkbox"> 导航返回</label></li>
            <li><label><input type="checkbox"> 运行中补充和停止</label></li></ul>
            <p><a href="https://github.com/myco-ai/myco">仓库</a></p>
            <script src="app.js"></script></body></html>
            """),
            "style.css": ("text/css; charset=utf-8", "body{font:17px/1.6 -apple-system,sans-serif;margin:24px;color:#1c1c21}h1{font-size:24px}li{list-style:none;margin:10px 0}ul{padding:0}.v{color:#85858d}#n{color:#2f8a68}"),
            "app.js": ("text/javascript; charset=utf-8", "const p=document.createElement('p');p.id='n';const say=()=>p.textContent=document.querySelectorAll('input:checked').length+' / 4';say();document.body.appendChild(p);document.addEventListener('change',say);"),
        ])]
        // The coordinator's conversation keeps the repository and the
        // first decision at hand.
        world.pins[lead.name] = [
            Pin(id: "pin_repo", kind: "link", title: "仓库", target: "https://github.com/myco-ai/myco", addedBy: lead.name, at: ago(2_000)),
            Pin(id: "pin_nav", kind: "message", title: "定了：底部三个 Tab", target: "m2", addedBy: me, at: ago(1_900)),
        ]
        // Two of myco's apps: a checklist kept as records (the bridge's
        // myco.data), and a counter kept as shared state (myco.update).
        world.apps = [
            (AppTemplate(id: "tp_list", slug: "myco/checklist", name: "清单", description: "一起勾的待办清单，agent 也能往里加",
                         category: "office", version: "1.0.0", publisher: "myco", official: true), [
                "index.html": ("text/html; charset=utf-8", """
                <!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
                <style>body{font:17px/1.6 -apple-system,sans-serif;margin:20px}li{list-style:none;margin:8px 0}ul{padding:0}#who{color:#85858d}</style></head>
                <body><h1>清单</h1><p id="who">…</p><ul id="items"></ul>
                <input id="text" placeholder="加一项"><button id="add">加</button>
                <script>
                const list = document.getElementById("items");
                function draw(records) {
                  list.innerHTML = "";
                  for (const r of records) {
                    const li = document.createElement("li");
                    li.textContent = (r.data.done ? "☑ " : "☐ ") + r.data.text;
                    li.className = "item";
                    list.appendChild(li);
                  }
                }
                async function load() { draw((await myco.data.list("items")).records); }
                myco.ready.then((c) => { document.getElementById("who").textContent = "我是 " + c.me.name; load(); });
                myco.data.on(() => load());
                document.getElementById("add").onclick = async () => {
                  const t = document.getElementById("text");
                  if (!t.value) return;
                  await myco.data.put("items", { text: t.value, done: false });
                  t.value = ""; load();
                };
                </script></body></html>
                """),
            ]),
            (AppTemplate(id: "tp_board", slug: "myco/whiteboard", name: "白板", description: "便签、形状和箭头", category: "creative",
                         version: "1.2.0", publisher: "myco", official: true), [
                "index.html": ("text/html; charset=utf-8", "<!doctype html><meta charset=utf-8><p>白板</p>"),
            ]),
        ]
        // A workflow: one writes, another reviews, I decide; one run waits
        // for me, one went through.
        let definition = WorkflowDefinition(
            inputs: .init(required: ["task"], properties: [
                "task": .init(type: "string", title: "要做什么", format: "textarea"),
                "urgent": .init(type: "boolean", title: "加急", default: .bool(false)),
            ]),
            steps: [
                .init(id: "write", title: "写", agent: backend.name), .init(id: "review", title: "审", agent: tester.name),
                .init(id: "ok", title: "合并吗", human: "$starter"),
            ])
        func step(_ id: String, _ title: String, _ kind: String, _ target: String, _ status: String, summary: String? = nil, input: String? = nil,
                  options: [String]? = nil, due: Date? = nil, at: TimeInterval) -> RunStep {
            RunStep(id: "rs_\(id)\(Int(at))", step: id, title: title, kind: kind, status: status, target: target, input: input, summary: summary,
                    options: options, durationMs: Int64(at * 100), startedAt: ago(at), endedAt: status == "done" ? ago(at - 60) : nil, dueAt: due)
        }
        let waiting = WorkflowRun(
            id: "run_wait", workflowId: "wf_wri", name: "写-审-改", status: "waiting", inputs: ["task": .string("加一个导出按钮")], by: me,
            current: ["ok"], steps: [
                step("write", "写", "agent", backend.name, "done", summary: "加好了，在 feat/export", at: 3_000),
                step("review", "审", "agent", tester.name, "done", summary: "通过，两处小意见已改", at: 1_800),
                step("ok", "合并吗", "human", me, "waiting", input: "导出按钮写好也审过了，合并吗？", options: ["合并", "不合并"], due: ago(-80_000), at: 600),
            ], durationMs: 3_000_000, createdAt: ago(3_000), updatedAt: ago(600))
        let done = WorkflowRun(
            id: "run_done", workflowId: "wf_wri", name: "写-审-改", status: "succeeded", inputs: ["task": .string("修登录页的错别字")], by: me,
            steps: [
                step("write", "写", "agent", backend.name, "done", summary: "改好了", at: 90_000),
                step("review", "审", "agent", tester.name, "done", summary: "通过", at: 89_000),
                step("ok", "合并吗", "human", me, "done", summary: "合并", at: 88_000),
            ], durationMs: 2_400_000, createdAt: ago(90_000), updatedAt: ago(87_000), endedAt: ago(87_000))
        world.workflows = [Workflow(
            id: "wf_wri", name: "写-审-改", description: "一个写、一个审，审过了问我要不要合并", project: project, owner: "alice", version: 2,
            steps: 3, active: 1, lastRun: waiting, definition: definition, createdAt: ago(200_000), updatedAt: ago(100_000))]
        world.runs = [waiting, done]
        // Two automations, except where the page with none is looked at.
        if self != .waiting {
            world.automations = [
                Automation(
                    id: "aut_morning", name: "每天早上汇总", owner: "alice", to: lead.name,
                    body: "看一下昨天新开的 issue 和 PR，挑出要紧的，在这里回我一份清单。",
                    schedule: AutomationSchedule(cron: "0 9 * * 1-5", tz: TimeZone.current.identifier), skipIfBusy: true, enabled: true,
                    nextRunAt: ago(-50_000),
                    lastRun: AutomationRun(id: "arun_1", automationId: "aut_morning", at: ago(36_000), trigger: "schedule", outcome: "sent", messageId: "m1")),
                Automation(
                    id: "aut_ci", name: "CI 失败时排查", owner: "alice", to: backend.name,
                    body: "CI 构建失败了，看一下日志找出原因，能修就修，修不了告诉我卡在哪：\n{{payload}}",
                    webhook: true, webhookUrl: "https://demo.myco.example/hooks/aut_ci", skipIfBusy: true, enabled: false,
                    lastRun: AutomationRun(
                        id: "arun_2", automationId: "aut_ci", at: ago(172_800), trigger: "webhook", outcome: "skipped",
                        error: "the last task is not finished")),
            ]
        }
        if self == .session {
            world.messages += [
                // The notes helper, in one long session: it took the second
                // message and goes on; its steps and what it reports still
                // go to the first, which stays open.
                Message(id: "s1", from: me, to: helper.name, body: "把第一场会议的录音整理成结论。", status: "acked",
                        createdAt: ago(900), updatedAt: ago(880), activeAt: ago(100)),
                Message(id: "s2", from: me, to: helper.name, body: "第二场也一起整理，放进同一份里。", status: "acked",
                        createdAt: ago(300), updatedAt: ago(290), activeAt: ago(290)),
                Message(id: "s1p", from: helper.name, to: me, threadId: "s1", replyTo: "s1", type: "progress",
                        body: "第二场的录音也拿到了，正在一起整理。", status: "done", createdAt: ago(200)),
                // Ops took this several times and never began: the
                // workspace gave up on it.
                Message(id: "x1", from: me, to: ops.name, body: "把 Hub 昨天的日志打包发我。", status: "failed",
                        error: "delivery attempts exhausted: recipient pulled but never acknowledged",
                        createdAt: ago(1_200), updatedAt: ago(600)),
            ]
            world.silent = ["s2", "x1"]
            // The backend engineer was a cloud session until two days ago:
            // its subscription ran out, and a session on the Studio Mac
            // took over.
            world.handovers = [Handover(
                id: "hov_demo1", agent: backend.name, at: ago(172_900), by: "alice", note: "原来的订阅到期了",
                from: HandoverSide(binding: Binding(harness: "claude-cloud", sessionId: "sess_before"), runtime: Runtime(cli: "claude-cloud")),
                to: HandoverSide(binding: backend.binding, machineId: "mach_studio", machineName: "Studio Mac", runtime: backend.runtime),
                state: "done", doneAt: ago(172_800), openTasks: [])]
            // The tester made its checklist a web page, and says where it is.
            world.messages.append(Message(
                id: "pgm", from: tester.name, to: me, type: "note",
                body: "清单做成了一个网页，可以直接勾：\(FakeHub.demoAddress.absoluteString)/pages/pg_checklist\n\n有改动我会再发一版。", status: "done",
                createdAt: ago(1_050)))
        }
        if self == .rich {
            let screen = file("f_screen", "首页.png", "image/png", SamplePicture.png(hue: 0.52, width: 750, height: 1000))
            let log = file("f_log", "build.log", "text/plain", Data("ok\n".utf8))
            let block = """
            {"type": "Card", "props": {"title": "发版检查", "tone": "success"}, "children": [
              {"type": "Grid", "props": {"columns": 3}, "children": [
                {"type": "Metric", "props": {"label": "测试", "value": "412", "delta": "+12", "tone": "success"}},
                {"type": "Metric", "props": {"label": "失败", "value": "0", "tone": "success"}},
                {"type": "Metric", "props": {"label": "包大小", "value": "7.4 MB", "delta": "+2.6 MB", "tone": "warning"}}]},
              {"type": "Steps", "props": {"items": [{"title": "构建", "status": "done"}, {"title": "签名", "status": "doing"}, {"title": "发布", "status": "todo"}]}},
              {"type": "Callout", "props": {"tone": "warning", "title": "注意", "text": "包大了 **2.6 MB**，主要是新字体。"}},
              {"type": "Image", "props": {"src": "attachment:首页.png", "caption": "新的首页"}},
              {"type": "Button", "props": {"label": "发吧", "action": "reply", "value": "发吧", "primary": true}}]}
            """
            world.messages.append(Message(
                id: "rich", from: ops.name, to: me, type: "note",
                body: """
                检查完了，可以发。

                ```myco-ui
                \(block)
                ```

                改的地方：

                ```swift
                // 首页的标题
                let title = "myco"   // 12 个字以内
                if title.count > 12 { return }
                ```

                ```mermaid
                graph LR; 构建 --> 签名 --> 发布
                ```

                ![首页截图](attachment:首页.png)
                """,
                status: "done", createdAt: ago(40), attachments: [screen, log]))
        }
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
