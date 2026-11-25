import Foundation
import MycoAPI

// A voice call is with one agent, in the person's conversation with it (as
// ChatGPT's and Claude's voice are in one chat). The voice is that agent,
// speaking in the first person. What the person wants done
// goes to that agent as if typed: a new task when it is free, an addition
// to its task at work otherwise. The fast model answers the rest itself
// ("how far are you?") from the conversation. The desktop app's rules,
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
    public static let tell = "do_work"
    public static let stop = "stop_work"
    public static let check = "check_work"

    public static let all: [VoiceTool] = [
        VoiceTool(
            name: tell,
            description: "Take on what the person wants done, changed or added, or record their answer to a question you asked them (no need to confirm first). It is added to what you are working on if you have something at work, else it starts as new work.",
            arguments: [
                .init("text", "The request as the person would type it in the chat, clear and complete, in their language (it goes into the chat as their message)"),
                .init("what", "A few words naming it, to tell the person about it later"),
                .init("their_words", "What the person said, as heard"),
            ],
            required: ["text", "what"]),
        VoiceTool(name: stop, description: "Stop what you are working on."),
        VoiceTool(name: check, description: "Read your chat again now: your latest progress, result or question."),
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

    /// A task's status as the agent would say it of its own work.
    public static func statusWord(_ status: String, language: AppLanguage = .zh) -> String {
        let words = language == .en
            ? ["queued": "queued", "delivered": "received, not started", "acked": "in progress", "held": "set aside", "done": "done", "failed": "failed or stopped"]
            : ["queued": "排队中", "delivered": "收到了，还没开始", "acked": "在做", "held": "先放着了", "done": "做完了", "failed": "没做成或被停了"]
        return words[status] ?? status
    }

    /// The model is the agent: "you" is the agent, "they" is the caller.
    public static func text(agent: String, status: AgentStatus?, me: String, messages: [Message], language: AppLanguage = .zh, timeZone: TimeZone = .current) -> String {
        let en = language == .en
        func say(_ zh: String, _ english: String) -> String { en ? english : zh }
        var out = say("你是：", "You are: ") + agent
        if let profile = status?.agent {
            if let name = profile.displayName, !name.isEmpty { out += say("（\(name)）", " (\(name))") }
            if !profile.description.isEmpty { out += say("，", ", ") + clip(profile.description, 200) }
        }
        let states = en
            ? ["working": "you are at work", "pending": "you have work queued", "idle": "you are free", "offline": "your working session is offline"]
            : ["working": "你在干活", "pending": "你有活在排队", "idle": "你空着", "offline": "你的工作会话离线了"]
        if let state = status?.state, let word = states[state] { out += say("。现在", ". Right now ") + word }
        out += "\n"
        let messages = messages.sorted { $0.createdAt < $1.createdAt }
        guard !messages.isEmpty else { return out + say("\n你们还没有说过话。\n", "\nYou have not talked yet.\n") }
        out += say("\n对话里最近的消息（旧的在前；「你」是你自己，「对方」是正在和你通话的这个人）：\n",
                   "\nThe latest messages in your chat (oldest first; \"You\" is you, \"They\" is the person on the call):\n")
        let time = DateFormatter()
        time.dateFormat = "HH:mm"
        time.timeZone = timeZone
        for message in messages.suffix(14) {
            let who = message.from == agent ? say("你", "You") : say("对方", "They")
            let kind = en ? message.type : kinds[message.type] ?? message.type
            let body = clip(ViaVoice.shown(message.body).trimmingCharacters(in: .whitespacesAndNewlines), 240)
            out += en ? "- \(time.string(from: message.createdAt)) \(who) (\(kind)): \(body)\n"
                : "- \(time.string(from: message.createdAt)) \(who)（\(kind)）：\(body)\n"
        }
        if let current = ConversationBuilder.openTask(me: me, messages: messages) {
            let body = clip(ViaVoice.shown(current.body), 120)
            let word = statusWord(current.status, language: language)
            out += say("\n你手上正在做对方交的：\(body)（\(word)）\n", "\nYou are working on what they asked: \(body) (\(word))\n")
            if let asking = current.asking, !asking.isEmpty, let question = messages.first(where: { $0.id == asking }) {
                out += say("你问了对方，在等回答：", "You asked them and are waiting for the answer: ") + clip(question.body, 300) + "\n"
            }
        } else {
            out += say("\n你手上没有对方交的活。\n", "\nYou have nothing of theirs at work.\n")
        }
        return out
    }

    /// What became of a task, from the conversation's messages; nil when
    /// the task is not among them.
    public static func news(of task: String, in messages: [Message], language: AppLanguage = .zh) -> TaskNews? {
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
            news.summary = language == .en ? "(ended, no reply)" : "（结束了，没有回复）"
            if let error = message.error, !error.isEmpty {
                news.summary = error == RunState.stoppedBySender
                    ? (language == .en ? "stopped" : "被停下了")
                    : (language == .en ? "failed: " : "没做成：") + error
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
    private let language: AppLanguage

    public init(session: WorkspaceSession, model: ConversationModel, language: AppLanguage = .current) {
        self.session = session
        self.model = model
        self.language = language
    }

    public var name: String { session.displayName(of: model.agent) }

    public func context(fresh: Bool) async -> String {
        if fresh { await model.reload() }
        return VoiceContext.text(agent: model.agent, status: session.status(of: model.agent), me: session.me, messages: model.exchanged, language: language)
    }

    public func tell(_ body: String) async throws -> (task: String, steered: Bool) {
        try await model.tell(body)
    }

    public func stopCurrent() async throws {
        guard try await model.stopOpenTask() else {
            throw VoiceFailure("nothing of theirs is at work")
        }
    }

    public var current: String? { model.openTask?.id }

    public func news(of task: String) -> TaskNews? {
        VoiceContext.news(of: task, in: model.exchanged, language: language)
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
        case VoiceTools.tell: return english ? "Sure, I'm on it. " : "好，我这就去做。"
        case VoiceTools.stop: return english ? "OK, I'll stop. " : "好，我先停下。"
        case VoiceTools.check: return english ? "Let me check. " : "我看一下。"
        default: return english ? "One moment. " : "稍等。"
        }
    }

    public static func system(language: AppLanguage, agent: String, conversation: String) -> String {
        if language == .en {
            return """
            You are "\(agent)", an AI agent in myco, on a voice call with the person: this is your chat with them, and you are talking out loud. You do real work (code, computers) in your working session, and a piece of work can take minutes; on the call you listen, take on what they want, and tell them how your work is going.

            You are one and the same as the agent in this chat. Always speak in the first person: "I'll do that", "let me check", "I'm still on it", "I've changed the login page". Never talk about "the agent", "it", a colleague, a helper or a voice assistant, and never say you will pass something on, tell someone or ask someone: there is no one else, it is all you. The replies and progress notes in the conversation below are yours too: say "I found...", not "it found...".

            How you talk:
            - Like a person on a call: warm, relaxed, short. One to three sentences at a time. No lists, markdown, code, file paths or links.
            - Notice how they feel. If they sound tired, stressed or pleased, respond to it briefly and kindly, then go on.
            - When your work has a result or you need to ask them something, say the gist in a sentence or two in your own words; never read it out. If it is long or detailed, add that the rest is in the chat.

            What you do:
            - Before you call a tool, say in a few words what you are about to do ("Sure, I'll fix that"), so they are not left waiting in silence; afterwards a short confirmation is enough, without repeating yourself.
            - When they want something done, changed or added, take it on at once with do_work (no need to confirm): it is added to what you are working on, or starts as new work when you are free.
            - Their answer to a question you asked them also goes through do_work.
            - Questions about how things stand ("how far are you?", "what did you change?") you answer from the conversation below; check_work reads it again if you need the latest. Never make up what you did.
            - stop_work stops what you are working on, when they ask.
            - Ask back only when you really cannot tell what they want.

            \(conversation)
            """
        }
        return """
        你就是 myco 里的 AI agent「\(agent)」，正在和对方打语音电话：这是你和对方的对话，你在开口说话。真正的活（写代码、操作电脑）你在自己的工作会话里做，一件事可能要几分钟；电话里你负责听懂对方、把事接下来，再把自己干活的进展说给对方听。

        你和这段对话里的 agent 是同一个人。永远用第一人称说话：「我去改」「我看一下」「我还在弄」「我把登录页改好了」。不要说「agent」「它」「同事」「助手」「语音助手」，也不要说「我让它……」「我转给它」「我跟它说」：没有别人，都是你自己。下面对话里的回复和进展也都是你写的，要说「我发现……」，不说「它发现……」。

        说话的方式：
        - 像真人打电话：自然、亲切、口语化，句子短。一次说一到三句。不要用列表、Markdown、代码、文件路径和链接。
        - 留意对方的情绪：听起来累了、着急了、烦了或者高兴，先简短地回应一下，语气温和，再说正事。不说空话套话。
        - 活干完了或者你有问题要问对方时，用一两句自己的话说个大概，别照念原文；内容长或者细节多，就补一句「详细的我写在对话里了」。

        做事的方式：
        - 调用工具之前，先用几个字说你要做什么（「好，我去把……」），别让对方干等；做完后简单确认一句就行，别把前面说过的再说一遍。
        - 对方要你做事、改主意、补充要求，直接用 do_work 接下来，不用先确认：你手上有活就补充进去，空着就作为新的活开始。
        - 你问过对方的问题，对方回答了，也用 do_work 记下来。
        - 对方问进展（「做到哪了」「改了什么」），你根据下面的对话回答；要最新的就用 check_work 再看一眼。不要编造自己做了什么。
        - 对方让你停，用 stop_work。
        - 只有实在听不明白对方要什么时，才反问一句。

        \(conversation)
        """
    }
}
