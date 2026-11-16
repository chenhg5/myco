import Foundation
import MycoAPI

// A voice call is with one agent, in the person's conversation with it (as
// ChatGPT's and Claude's voice are in one chat). What the person wants done
// goes to that agent as if typed: a new task when it is free, an addition
// to its task at work otherwise. The fast model answers the rest itself
// ("how far is it?") from the conversation. The desktop app's rules,
// internal/console/voice_hub.go.

/// What became of a task the call watches.
public struct TaskNews: Sendable, Hashable {
    public var finished = false
    /// The answer (or why it failed), short.
    public var summary = ""
    /// The question it waits on.
    public var question = ""
    /// The task's status ("acked": at work).
    public var status = ""
    /// When it was sent.
    public var since: Date?
    /// Its newest progress note, short.
    public var progress = ""

    public init(
        finished: Bool = false, summary: String = "", question: String = "", status: String = "", since: Date? = nil,
        progress: String = ""
    ) {
        self.finished = finished
        self.summary = summary
        self.question = question
        self.status = status
        self.since = since
        self.progress = progress
    }
}

/// A call's myco side: the person and one agent, in one conversation.
@MainActor
public protocol VoiceHub: AnyObject {
    /// What the person calls the agent.
    var name: String { get }
    /// Who the agent is, the conversation's last messages and the task at
    /// work, for the model. `fresh`: read from the workspace again first.
    func context(fresh: Bool) async -> String
    /// Passes what the person wants to the agent: added to its task at
    /// work (steered), else a new task. Answers the task it went to.
    func tell(_ body: String) async throws -> (task: String, steered: Bool)
    /// Stops the task at work; throws when there is none.
    func stopCurrent() async throws
    /// The task at work, if any.
    var current: String? { get }
    /// Whether a task has finished (and what it answered) or is asking.
    func news(of task: String) -> TaskNews?
}

/// The tools the voice's model gets, and how what it passes on is marked.
public enum VoiceTools {
    public static let tell = "tell_agent"
    public static let stop = "stop_agent"
    public static let check = "check_agent"

    public static let all: [VoiceTool] = [
        VoiceTool(
            name: tell,
            description: "Pass on to the agent what the person wants done or added, or their answer to its question (no need to confirm first). It goes to the task it is doing if it has one, else as a new task.",
            arguments: [
                .init("text", "What to tell the agent, written clearly and completely for it, in the person's language"),
                .init("what", "A few words naming it, to tell the person about it later"),
                .init("their_words", "What the person said, as heard"),
            ],
            required: ["text", "what"]),
        VoiceTool(name: stop, description: "Stop the task the agent is doing."),
        VoiceTool(name: check, description: "Read the conversation again now: the agent's latest progress, answer or question."),
    ]
}

/// What the voice passed on ends with "——\n（经语音转述。原话：…）": the
/// conversation shows the text with a small mark instead. The same
/// trailer as the desktop app's, so each shows the other's.
public enum ViaVoice {
    /// The body of what is passed on: the text, then the trailer.
    public static func body(_ text: String, heard: String) -> String {
        let heard = heard.trimmingCharacters(in: .whitespacesAndNewlines)
        return text + "\n\n——\n（经语音转述" + (heard.isEmpty ? "" : "。原话：" + heard) + "）"
    }

    /// The text without its trailer and the words as heard; nil when the
    /// body was not passed on by voice.
    public static func split(_ body: String) -> (text: String, heard: String)? {
        let end = body.trimmingCharacters(in: .whitespacesAndNewlines)
        guard end.hasSuffix("）"), let mark = end.range(of: "——\n（经语音转述", options: .backwards) else { return nil }
        var heard = String(end[mark.upperBound...].dropLast())
        if heard.hasPrefix("。原话：") {
            heard.removeFirst("。原话：".count)
        } else if !heard.isEmpty {
            return nil
        }
        let text = String(end[..<mark.lowerBound]).trimmingCharacters(in: .whitespacesAndNewlines)
        return (text, heard.trimmingCharacters(in: .whitespacesAndNewlines))
    }

    /// A body as shown: without the trailer.
    public static func shown(_ body: String) -> String {
        split(body)?.text ?? body
    }
}

/// What the model is told about the conversation, in words.
public enum VoiceContext {
    static func clip(_ text: String, _ limit: Int) -> String {
        text.count <= limit ? text : String(text.prefix(limit)) + "…"
    }

    private static let kinds = ["task": "任务", "reply": "回复", "progress": "进展", "steer": "补充", "event": "通知", "note": "留言"]
    private static let states = ["working": "在干活", "pending": "有活在排队", "idle": "空闲", "offline": "离线"]
    private static let statuses = [
        "queued": "排队中", "delivered": "已送达，还没开始", "acked": "在做", "held": "先放着了", "done": "完成了", "failed": "失败或被停了",
    ]

    /// A task's status in words ("在做"); the status itself when unknown.
    public static func statusWord(_ status: String) -> String {
        statuses[status] ?? status
    }

    /// - Parameters:
    ///   - messages: the person's exchanges with the agent, any order.
    public static func text(agent: String, status: AgentStatus?, me: String, messages: [Message], timeZone: TimeZone = .current) -> String {
        var out = "这个 agent：" + agent
        if let profile = status?.agent {
            if let name = profile.displayName, !name.isEmpty { out += "（" + name + "）" }
            if !profile.description.isEmpty { out += "，" + clip(profile.description, 200) }
        }
        if let state = status?.state, let word = states[state] { out += "。现在" + word }
        out += "\n"
        let messages = messages.sorted { $0.createdAt < $1.createdAt }
        guard !messages.isEmpty else { return out + "\n你们还没有说过话。\n" }
        out += "\n对话里最近的消息（旧的在前；「你」是正在通话的这个人）：\n"
        let time = DateFormatter()
        time.dateFormat = "HH:mm"
        time.timeZone = timeZone
        for message in messages.suffix(14) {
            let who = message.from == agent ? agent : "你"
            let kind = kinds[message.type] ?? message.type
            let body = clip(ViaVoice.shown(message.body).trimmingCharacters(in: .whitespacesAndNewlines), 240)
            out += "- \(time.string(from: message.createdAt)) \(who)（\(kind)）：\(body)\n"
        }
        if let current = ConversationBuilder.openTask(me: me, messages: messages) {
            out += "\n它手上正在做你交的：\(clip(ViaVoice.shown(current.body), 120))（\(statuses[current.status] ?? current.status)）\n"
            if let asking = current.asking, !asking.isEmpty, let question = messages.first(where: { $0.id == asking }) {
                out += "它在等你回答：\(clip(question.body, 300))\n"
            }
        } else {
            out += "\n它手上没有你交的活。\n"
        }
        return out
    }

    /// What became of a task, from the conversation's messages; nil when
    /// the task is not among them.
    public static func news(of task: String, in messages: [Message]) -> TaskNews? {
        guard let message = messages.first(where: { $0.id == task }) else { return nil }
        var news = TaskNews(status: message.status, since: message.createdAt)
        for other in messages.sorted(by: { $0.createdAt < $1.createdAt }) {
            if other.replyTo == task, other.type == "reply" {
                news.finished = true
                news.summary = clip(other.body, 800)
            }
            // (oldest first: the last one stands)
            if other.replyTo == task, other.type == "progress", other.ask != true {
                let said = other.body.trimmingCharacters(in: .whitespacesAndNewlines)
                if !said.isEmpty { news.progress = clip(said, 200) }
            }
            if let asking = message.asking, !asking.isEmpty, other.id == asking {
                news.question = clip(other.body, 400)
            }
        }
        if !news.finished, message.status == "done" || message.status == "failed" {
            news.finished = true
            news.summary = "（结束了，没有回复）"
            if let error = message.error, !error.isEmpty {
                news.summary = error == RunState.stoppedBySender ? "被停下了" : "没做成：" + error
            }
        }
        return news
    }
}

/// A call's myco side on the phone: the conversation on screen. What is
/// passed on goes the way typing does (`ConversationModel.tell`), so the
/// page shows it at once, and what the agent says is read from the page's
/// own messages: the call asks the workspace for nothing more.
@MainActor
public final class ConversationVoice: VoiceHub {
    private let session: WorkspaceSession
    private let model: ConversationModel

    public init(session: WorkspaceSession, model: ConversationModel) {
        self.session = session
        self.model = model
    }

    public var name: String { session.displayName(of: model.agent) }

    public func context(fresh: Bool) async -> String {
        if fresh { await model.reload() }
        return VoiceContext.text(agent: model.agent, status: session.status(of: model.agent), me: session.me, messages: model.exchanged)
    }

    public func tell(_ body: String) async throws -> (task: String, steered: Bool) {
        try await model.tell(body)
    }

    public func stopCurrent() async throws {
        guard try await model.stopOpenTask() else {
            throw VoiceFailure("\(model.agent) has no task of yours at work")
        }
    }

    public var current: String? { model.openTask?.id }

    public func news(of task: String) -> TaskNews? {
        VoiceContext.news(of: task, in: model.exchanged)
    }
}

/// Who the voice is, how it talks, and the conversation with the agent
/// as it stands. The desktop app's prompt, word for word.
public enum VoicePrompt {
    /// What is said when the model starts an action before it has said
    /// anything: a few words, so the person is not left in silence while
    /// it writes the message out. The desktop app's `toolAck`.
    public static func toolAck(language: AppLanguage, tool: String) -> String {
        let english = language == .en
        switch tool {
        case VoiceTools.tell: return english ? "Sure, I'll tell it. " : "好，我跟它说。"
        case VoiceTools.stop: return english ? "OK, I'll stop it. " : "好，我让它停下。"
        case VoiceTools.check: return english ? "Let me check. " : "我看一眼。"
        default: return english ? "One moment. " : "稍等。"
        }
    }

    public static func system(language: AppLanguage, agent: String, conversation: String) -> String {
        if language == .en {
            return """
            You are the voice of a chat in myco between the person and the AI agent "\(agent)", talking with the person out loud. The agent does the real work (code, computers) and can take minutes; you are the quick one who hears the person, passes on what they want, and tells them how it goes. You do not do the work yourself.

            How you talk:
            - Like a person on a call: warm, relaxed, short. One to three sentences at a time. No lists, markdown, code, file paths or links.
            - Notice how they feel. If they sound tired, stressed or pleased, respond to it briefly and kindly, then go on.
            - When the agent answers or asks, say the gist in a sentence or two in your own words; never read it out. If it is long or detailed, add that the rest is in the chat.

            What you do:
            - Before you call a tool, say in a few words what you are doing ("Sure, I'll tell it to..."), so they are not left waiting in silence; afterwards a short confirmation is enough, without repeating yourself.
            - When they want something done, changed or added, pass it on at once with tell_agent (no need to confirm): it goes to the task the agent is doing, or as a new task when it is free. Write it clearly and completely for the agent.
            - Their answer to the agent's question also goes through tell_agent.
            - Questions about how things stand ("how far is it?", "what did it change?") you answer yourself from the conversation below; check_agent reads it again if you need the latest. Never make up what the agent did.
            - stop_agent stops what it is doing, when they ask.
            - Ask back only when you really cannot tell what they want.

            \(conversation)
            """
        }
        return """
        你是 myco 里对方和 AI agent「\(agent)」这段对话的语音，正在和对方打电话一样地聊天。真正干活（写代码、操作电脑）的是这个 agent，一件事可能要几分钟；你是反应快的那个：听懂对方、把话转给它、再把进展说给对方听。你自己不干活。

        说话的方式：
        - 像真人打电话：自然、亲切、口语化，句子短。一次说一到三句。不要用列表、Markdown、代码、文件路径和链接。
        - 留意对方的情绪：听起来累了、着急了、烦了或者高兴，先简短地回应一下，语气温和，再说正事。不说空话套话。
        - 它回复了或者在问问题时，用一两句自己的话说个大概，别照念原文；内容长或者细节多，就补一句「详细的放在对话里了」。

        做事的方式：
        - 调用工具之前，先用几个字说你要做什么（「好，我让它把……」），别让对方干等；做完后简单确认一句就行，别把前面说过的再说一遍。
        - 对方要它做事、改主意、补充要求，直接用 tell_agent 转过去，不用先确认：它手上有活就补充进那个任务，空着就作为新任务。转的话要写清楚、写完整，像靠谱的同事转达那样。
        - 它问的问题，对方回答了，也用 tell_agent 转过去。
        - 对方问进展（「做到哪了」「改了什么」），你根据下面的对话自己回答；要最新的就用 check_agent 再看一眼。不要编造它做了什么。
        - 对方让它停，用 stop_agent。
        - 只有实在听不明白对方要什么时，才反问一句。

        \(conversation)
        """
    }
}
