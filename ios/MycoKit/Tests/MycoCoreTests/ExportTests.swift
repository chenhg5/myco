import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// A conversation written out as a Markdown file, in the shape the
/// desktop app and `myco thread export` write.
@MainActor
struct ExportTests {
    let utc = TimeZone(identifier: "UTC")!
    /// 2026-11-17 09:30 UTC.
    let morning = Date(timeIntervalSince1970: 1_794_907_800)

    @Test func aConversationReadsInTimeWithWhoSaidWhat() {
        var sealed = Message(id: "t4", from: "coder", to: "@console:alice", type: "note", body: "x", status: "done", createdAt: morning.addingTimeInterval(2_400))
        sealed.unreadable = "no key"
        let messages = [
            Message(id: "r1", from: "coder", to: "@console:alice", threadId: "t1", replyTo: "t1", type: "reply", body: "改好了。", status: "done",
                    createdAt: morning.addingTimeInterval(600), attachments: [Attachment(fileId: "f1", name: "截图.png", size: 20_480, contentType: "image/png")]),
            Message(id: "t1", from: "@console:alice", to: "coder", body: "把首页的标题改短", status: "done", createdAt: morning),
            Message(id: "t2", from: "@console:alice", to: "coder", body: "再跑一遍测试", status: "failed", error: "stopped by the sender",
                    createdAt: morning.addingTimeInterval(1_200)),
            Message(id: "t3", from: "@console:alice", to: "coder", body: "发版", status: "failed", error: "lease expired",
                    createdAt: morning.addingTimeInterval(1_800)),
            sealed,
        ]
        let steps = ["t1": [
            ConversationExport.Step(tool: "Read", input: "web/index.html", failed: false),
            ConversationExport.Step(tool: "Bash", input: "echo `date`", failed: true),
        ]]
        let text = ConversationExport.markdown(
            messages, title: "和 写代码的 的对话", name: { $0 == "coder" ? "写代码的" : "你" }, steps: steps,
            now: morning.addingTimeInterval(3_600), timeZone: utc)
        #expect(text == """
        # 和 写代码的 的对话

        导出时间：2026-11-17 10:30 · 共 5 条消息

        ---

        **你** → 写代码的 · 11-17 09:30 · 任务

        把首页的标题改短

        <details><summary>过程：2 步</summary>

        - Read `web/index.html`
        - Bash `echo 'date'`（失败）

        </details>

        ---

        **写代码的** → 你 · 11-17 09:40 · 回复

        改好了。

        附件：截图.png（20 KB）

        ---

        **你** → 写代码的 · 11-17 09:50 · 任务 · 已停止

        再跑一遍测试

        ---

        **你** → 写代码的 · 11-17 10:00 · 任务 · 失败：lease expired

        发版

        ---

        **写代码的** → 你 · 11-17 10:10 · 留言

        （读不出：no key）

        """)
    }

    @Test func aTasksToolCallsAreItsSteps() {
        let long = String(repeating: "很长的命令 ", count: 60)
        let steps = ConversationExport.steps(of: [
            TraceEvent(seq: 1, kind: "text", data: TraceStep(text: "looking")),
            TraceEvent(seq: 2, kind: "tool", data: TraceStep(id: "a", tool: "Bash", input: "go test\n  ./...")),
            TraceEvent(seq: 3, kind: "tool_result", data: TraceStep(id: "a", output: "FAIL", failed: true)),
            TraceEvent(seq: 4, kind: "tool", data: TraceStep(id: "b", tool: "Read", input: long)),
            TraceEvent(seq: 5, kind: "tool_result", data: TraceStep(id: "b", output: "…")),
            TraceEvent(seq: 6, kind: "tool", data: TraceStep(id: "c", tool: "TodoWrite")),
        ])
        #expect(steps.map(\.tool) == ["Bash", "Read", "TodoWrite"])
        #expect(steps.map(\.failed) == [true, false, false])
        // One line each, not longer than a line should be.
        #expect(steps[0].input == "go test ./...")
        #expect(steps[1].input.count == 160)
        #expect(steps[1].input.hasSuffix("…"))
        #expect(steps[2].input == "")
    }

    @Test func theFileIsNamedForWhomAndTheDay() {
        #expect(ConversationExport.fileName(with: "后端工程师", now: morning, timeZone: utc) == "和 后端工程师 的对话 2026-11-17.md")
        // Safe on every system.
        #expect(ConversationExport.fileName(with: "a/b:c*?", now: morning, timeZone: utc) == "和 a b c 的对话 2026-11-17.md")
        #expect(ConversationExport.fileName(with: " / ", now: morning, timeZone: utc) == "和 myco 的对话 2026-11-17.md")
        #expect(ConversationExport.size(900) == "900 B")
        #expect(ConversationExport.size(1_536) == "2 KB")
        #expect(ConversationExport.size(3 * 1024 * 1024 + 200_000) == "3.2 MB")
    }

    @Test func anExportIsTheWholeConversationNotOnlyThePagesOnScreen() async throws {
        let hub = FakeHub(scenario: .long)
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        let model = session.conversation(with: "studio-claude-c41f")
        await model.reload()
        // Four hundred and two messages with the notes helper; a page shows the newest.
        let onScreen = model.items.count
        #expect(model.hasEarlier)
        let exported = try await model.export(steps: false, title: "和 笔记助手 的对话", with: "笔记助手", name: { session.displayName(of: $0) })
        #expect(exported.messages == 402)
        #expect(exported.text.contains("第 1 个问题"))
        #expect(exported.text.contains("这是最后一条回复。"))
        #expect(exported.text.hasPrefix("# 和 笔记助手 的对话\n"))
        #expect(exported.name.hasPrefix("和 笔记助手 的对话 "))
        #expect(exported.name.hasSuffix(".md"))
        #expect(!exported.text.contains("<details>"))
        // The page is as it was: nothing was added to it.
        #expect(model.items.count == onScreen)
        #expect(model.hasEarlier)

        // With the steps: under each task that has a run.
        let withSteps = try await model.export(steps: true, title: "t", with: "x", name: { $0 })
        #expect(withSteps.text.contains("<details><summary>过程："))
        #expect(withSteps.text.contains("- Bash `git status --short`"))

        // What I took out of my view is not in the file.
        await model.setHidden(["h1"], hidden: true)
        let fewer = try await model.export(steps: false, title: "t", with: "x", name: { $0 })
        #expect(fewer.messages == 401)
        #expect(!fewer.text.contains("帮我把昨天的会议记录整理成三条结论"))

        // The workspace does not answer part of the way: said, not half a file.
        await hub.setUnreachable(true)
        await #expect(throws: HubError.self) {
            _ = try await model.export(steps: false, title: "t", with: "x", name: { $0 })
        }
    }
}
