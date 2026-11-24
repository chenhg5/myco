import Foundation
import Testing
import MycoAPI
@testable import MycoCore

/// An agent's open work: what each task waits on, and the tasks from
/// elsewhere said in its chat.
struct OpenWorkTests {
    let now = Date(timeIntervalSince1970: 1_800_000_000)

    func task(_ id: String, from: String, status: String, ago: TimeInterval, asking: String? = nil) -> Message {
        var m = Message(id: id, from: from, to: "coder", body: id, status: status, createdAt: now.addingTimeInterval(-ago))
        m.asking = asking
        return m
    }

    @Test func eachTaskSaysWhatItWaitsOn() {
        AppLanguage.$forced.withValue(.zh) {
            let status = AgentStatus(agent: Agent(id: "a", name: "coder"), state: "working", staleTasks: ["t_left"])
            let name: (String) -> String = { $0 == "lead" ? "协调者" : $0 }
            #expect(OpenWork.line(of: task("t1", from: "lead", status: "held", ago: 600, asking: "q1"), in: status, name: name) == "在等 协调者 回答")
            #expect(OpenWork.line(of: task("t_left", from: "lead", status: "acked", ago: 6_000), in: status, name: name) == "没收尾")
            #expect(OpenWork.line(of: task("t2", from: "lead", status: "acked", ago: 60), in: status, name: name) == "处理中")
            #expect(OpenWork.line(of: task("t3", from: "lead", status: "queued", ago: 60), in: status, name: name) == "待处理")
        }
    }

    @Test func tasksFromElsewhereAreSaidInItsChat() {
        let current = [
            task("mine", from: "@console:alice", status: "acked", ago: 600),
            task("old", from: "lead", status: "acked", ago: 6_000),
            task("ask", from: "tester", status: "held", ago: 900, asking: "q"),
            task("new", from: "lead", status: "acked", ago: 30),
            task("left", from: "lead", status: "acked", ago: 30),
            task("waiting", from: "lead", status: "queued", ago: 900),
        ]
        let status = AgentStatus(agent: Agent(id: "a", name: "coder"), state: "working", current: current, staleTasks: ["old", "left"])
        // Not the ones in this chat, not one just given (unless left open),
        // not one not taken up yet.
        let elsewhere = OpenWork.elsewhere(status, agent: "coder", shown: ["mine"], now: now)
        #expect(elsewhere.map(\.id) == ["old", "ask", "left"])
        AppLanguage.$forced.withValue(.zh) {
            let words = OpenWork.words(elsewhere, status: status, me: "@console:alice", name: { $0 == "tester" ? "测试" : $0 })
            #expect(words == "它还有 3 条别处来的任务没做完，其中 2 条好一阵没有动静，其中 1 条在等 测试 回答它的问题。")
        }
        #expect(OpenWork.elsewhere(nil, agent: "coder", shown: [], now: now).isEmpty)
    }
}
