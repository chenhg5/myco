import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// The pieces a voice call is made of that need no microphone: reading
/// the model's and the voice's streams, the words passed on, and what the
/// model is told about the conversation.
struct VoicePartsTests {
    static func lines(_ status: Int, _ lines: [String], seen: Seen? = nil) -> VoiceLines {
        { request in
            seen?.set(request)
            let (stream, continuation) = AsyncThrowingStream<String, Error>.makeStream()
            for line in lines { continuation.yield(line) }
            continuation.finish()
            return (status, stream)
        }
    }

    final class Seen: @unchecked Sendable {
        private let lock = NSLock()
        private var request: URLRequest?

        func set(_ request: URLRequest) {
            lock.lock()
            self.request = request
            lock.unlock()
        }

        var last: URLRequest? {
            lock.lock()
            defer { lock.unlock() }
            return request
        }
    }

    @Test func aStreamedAnswerIsPutTogetherWithItsToolCalls() throws {
        var reader = ChatStreamReader()
        #expect(try reader.take(#"data: {"choices":[{"delta":{"role":"assistant","content":"好，"}}]}"#) == "好，")
        #expect(try reader.take(": keep-alive") == nil)
        #expect(try reader.take(#"data: {"choices":[{"delta":{"content":null,"tool_calls":[{"index":0,"id":"call_a","function":{"name":"tell_agent","arguments":"{\"text\":"}}]}}]}"#) == nil)
        // A tool's start is told as soon as its name is there, and once.
        #expect(reader.takeBegun() == [VoiceTools.tell])
        #expect(try reader.take(#"data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"改标题\",\"what\":\"标题\"}"}}]}}]}"#) == nil)
        #expect(reader.takeBegun().isEmpty)
        #expect(try reader.take(#"data: {"choices":[{"delta":{"content":"转过去了。"}}]}"#) == "转过去了。")
        #expect(!reader.done)
        _ = try reader.take("data: [DONE]")
        #expect(reader.done)
        let reply = reader.reply
        #expect(reply.role == "assistant")
        #expect(reply.content == "好，转过去了。")
        #expect(reply.toolCalls.count == 1)
        #expect(reply.toolCalls[0].id == "call_a")
        #expect(reply.toolCalls[0].name == VoiceTools.tell)
        #expect(reply.toolCalls[0].texts == ["text": "改标题", "what": "标题"])
    }

    @Test func whatTheModelSaysWentWrongIsSaid() {
        var reader = ChatStreamReader()
        #expect(throws: VoiceFailure.self) {
            _ = try reader.take(#"data: {"error":{"message":"quota used up"}}"#)
        }
    }

    @Test func theModelIsAskedTheOpenAIWay() async throws {
        let seen = Seen()
        let chat = OpenAIChat(
            base: "https://api.example.com/v1/", model: "fast-1", key: "sk-test", session: "s1",
            lines: Self.lines(200, [#"data: {"choices":[{"delta":{"content":"在。"}}]}"#, "data: [DONE]"], seen: seen))
        let reply = try await chat.stream([ChatMessage(role: "user", content: "在吗")], tools: VoiceTools.all) { _ in }
        #expect(reply.content == "在。")
        let request = try #require(seen.last)
        #expect(request.url?.absoluteString == "https://api.example.com/v1/chat/completions")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer sk-test")
        let sent = try #require(request.httpBody)
        let body = try #require(try JSONSerialization.jsonObject(with: sent) as? [String: Any])
        #expect(body["model"] as? String == "fast-1")
        #expect(body["stream"] as? Bool == true)
        #expect((body["tools"] as? [[String: Any]])?.count == 3)
        #expect((body["messages"] as? [[String: Any]])?.first?["content"] as? String == "在吗")
        // The whole endpoint pasted in works too; no address at all does not.
        #expect(OpenAIChat.endpoint("https://api.example.com/v1/chat/completions")?.absoluteString == "https://api.example.com/v1/chat/completions")
        #expect(OpenAIChat.endpoint("not an address") == nil)
    }

    @Test func aRefusalIsSaidInItsOwnWords() async {
        let chat = OpenAIChat(
            base: "https://api.example.com/v1", model: "m", key: "bad",
            lines: Self.lines(401, [#"{"error":{"message":"Incorrect API key"}}"#]))
        do {
            _ = try await chat.stream([], tools: []) { _ in }
            Issue.record("a refusal must throw")
        } catch {
            #expect((error as? VoiceFailure)?.message == "HTTP 401: Incorrect API key")
        }
    }

    @Test func doubaoSoundComesInPiecesOfWholeSamples() throws {
        var reader = DoubaoStreamReader()
        let three = Data([1, 2, 3]).base64EncodedString()
        let one = Data([4]).base64EncodedString()
        // An odd byte waits for the next piece.
        #expect(try reader.take(#"{"code":0,"data":"\#(three)"}"#) == Data([1, 2]))
        #expect(try reader.take(#"{"code":0,"data":"\#(one)"}"#) == Data([3, 4]))
        #expect(try reader.take(#"{"code":0,"data":""}"#) == nil)
        #expect(!reader.finished)
        #expect(try reader.take(#"{"code":20000000,"message":"ok"}"#) == nil)
        #expect(reader.finished)
        var failing = DoubaoStreamReader()
        #expect(throws: VoiceFailure.self) {
            _ = try failing.take(#"{"code":45000001,"message":"bad speaker"}"#)
        }
    }

    @Test func doubaoIsAskedWithTheKeyAndTheVoice() async throws {
        let seen = Seen()
        let sound = Data([9, 9]).base64EncodedString()
        let speech = DoubaoSpeech(
            key: "k", speaker: "S_mine",
            lines: Self.lines(200, [#"{"code":0,"data":"\#(sound)"}"#, #"{"code":20000000}"#], seen: seen))
        let played = await MainActor.run { Played() }
        try await speech.speak("你好") { samples, rate in played.add(samples, rate) }
        #expect(await played.all == [Data([9, 9])])
        #expect(await played.rate == 24000)
        let request = try #require(seen.last)
        #expect(request.value(forHTTPHeaderField: "X-Api-Key") == "k")
        // A cloned voice has its own resource.
        #expect(request.value(forHTTPHeaderField: "X-Api-Resource-Id") == "seed-icl-2.0")
    }

    @MainActor
    final class Played {
        var all: [Data] = []
        var rate = 0

        func add(_ samples: Data, _ rate: Int) {
            all.append(samples)
            self.rate = rate
        }
    }

    @Test func sentencesAreCutWhereTheyCanBeSaid() {
        var splitter = SentenceSplitter()
        #expect(splitter.add("好的，我") == [])
        #expect(splitter.add("转过去了。它") == ["好的，我转过去了。"])
        // A comma cuts only once the piece is long.
        #expect(splitter.add("刚把下载页的版式和里面所有的链接都改完，现在") == ["它刚把下载页的版式和里面所有的链接都改完，"])
        #expect(splitter.flush() == "现在")
        #expect(splitter.flush() == "")
    }

    @Test func whatIsPassedOnCarriesItsMarkAndLosesItOnThePage() {
        let body = ViaVoice.body("把下载页的标题改短一点", heard: "顺便把标题也改短一点")
        #expect(body == "把下载页的标题改短一点\n\n——\n（经语音转述。原话：顺便把标题也改短一点）")
        let split = ViaVoice.split(body)
        #expect(split?.text == "把下载页的标题改短一点")
        #expect(split?.heard == "顺便把标题也改短一点")
        // Without the words as heard.
        #expect(ViaVoice.split(ViaVoice.body("停一下", heard: ""))?.heard == "")
        #expect(ViaVoice.split(ViaVoice.body("停一下", heard: ""))?.text == "停一下")
        // Anything typed is left alone.
        #expect(ViaVoice.split("把标题改短一点（之前说过的）") == nil)
        #expect(ViaVoice.shown("就这样") == "就这样")
    }

    @Test func theModelIsToldWhoTheAgentIsAndHowThingsStand() {
        let me = "@console:glenn", agent = "studio-claude"
        let start = Date(timeIntervalSince1970: 1_800_000_000)
        var task = Message(id: "t1", from: me, to: agent, body: "把下载页改一下", status: "held", createdAt: start)
        task.asking = "q1"
        var question = Message(id: "q1", from: agent, to: me, threadId: "t1", replyTo: "t1", type: "progress", body: "标题用哪一版？", status: "done", createdAt: start.addingTimeInterval(60))
        question.ask = true
        let messages = [
            question,
            Message(id: "p0", from: agent, to: me, threadId: "t1", replyTo: "t1", type: "progress", body: "先看了现在的版式", status: "done", createdAt: start.addingTimeInterval(20)),
            Message(id: "p1", from: agent, to: me, threadId: "t1", replyTo: "t1", type: "progress", body: " 版式改了一半 \n", status: "done", createdAt: start.addingTimeInterval(40)),
            task,
            Message(id: "t0", from: me, to: agent, body: ViaVoice.body("先看看首页", heard: "看下首页"), status: "done", createdAt: start.addingTimeInterval(-600)),
            Message(id: "r0", from: agent, to: me, threadId: "t0", replyTo: "t0", type: "reply", body: "首页看过了，没问题。", status: "done", createdAt: start.addingTimeInterval(-500)),
        ]
        let status = AgentStatus(agent: Agent(id: "a1", name: agent, displayName: "前端", description: "写页面的"), state: "working")
        let text = VoiceContext.text(agent: agent, status: status, me: me, messages: messages, timeZone: TimeZone(identifier: "UTC")!)
        #expect(text.hasPrefix("这个 agent：studio-claude（前端），写页面的。现在在干活\n"))
        // Oldest first, and what was passed on by voice without its mark.
        let first = try! #require(text.range(of: "你（任务）：先看看首页\n"))
        let last = try! #require(text.range(of: "studio-claude（进展）：标题用哪一版？"))
        #expect(first.lowerBound < last.lowerBound)
        #expect(!text.contains("经语音转述"))
        #expect(text.contains("它手上正在做你交的：把下载页改一下（先放着了）"))
        #expect(text.contains("它在等你回答：标题用哪一版？"))
        // What became of each task.
        #expect(VoiceContext.news(of: "t0", in: messages)
            == TaskNews(finished: true, summary: "首页看过了，没问题。", status: "done", since: start.addingTimeInterval(-600)))
        // Its status, when it was sent, and its newest progress note (a
        // question is not one).
        #expect(VoiceContext.news(of: "t1", in: messages)
            == TaskNews(question: "标题用哪一版？", status: "held", since: start, progress: "版式改了一半"))
        #expect(VoiceContext.statusWord("acked") == "在做")
        #expect(VoiceContext.statusWord("odd") == "odd")
        #expect(VoiceContext.news(of: "nope", in: messages) == nil)
        #expect(VoiceContext.text(agent: agent, status: nil, me: me, messages: []).contains("你们还没有说过话"))
    }

    @Test func aLongTalkKeepsItsEndAndNeverStartsAtAToolsResult() {
        var history: [ChatMessage] = []
        for index in 0..<20 {
            history.append(ChatMessage(role: "user", content: "u\(index)"))
            history.append(ChatMessage(role: "assistant", content: "", toolCalls: [ToolCall(id: "c\(index)", name: VoiceTools.check, arguments: "{}")]))
            history.append(ChatMessage(role: "tool", content: "ok", toolCallID: "c\(index)"))
        }
        let kept = VoiceCall.trim(history)
        #expect(kept.count <= 30)
        #expect(kept.first?.role == "user")
        #expect(kept.last == history.last)
        #expect(VoiceCall.trim(Array(history.prefix(6))) == Array(history.prefix(6)))
    }
}

/// A model that is refused.
private struct FailingModel: VoiceModel {
    func stream(
        _ messages: [ChatMessage], tools: [VoiceTool], onText: @escaping @MainActor @Sendable (String) -> Void,
        onTool: @escaping @MainActor @Sendable (String) -> Void
    ) async throws -> ChatMessage {
        throw VoiceFailure("HTTP 401: Incorrect API key")
    }
}

/// A call from its first word to hanging up, with a microphone that hears
/// what the test says, a scripted model and a silent voice.
@MainActor
struct VoiceCallTests {
    /// The agent's side, in memory.
    @MainActor
    final class Hub: VoiceHub {
        let name = "前端"
        var current: String?
        var told: [String] = []
        var stops = 0
        var contexts = 0
        var freshContexts = 0
        var newsOf: [String: TaskNews] = [:]
        var refuses = false

        func context(fresh: Bool) async -> String {
            contexts += 1
            if fresh { freshContexts += 1 }
            return "它手上没有你交的活。"
        }

        func tell(_ body: String) async throws -> (task: String, steered: Bool) {
            if refuses { throw VoiceFailure("workspace unreachable") }
            told.append(body)
            if let current { return (current, true) }
            current = "task-\(told.count)"
            return (current!, false)
        }

        func stopCurrent() async throws {
            guard current != nil else { throw VoiceFailure("nothing at work") }
            stops += 1
            current = nil
        }

        func news(of task: String) -> TaskNews? { newsOf[task] }
    }

    struct Setup {
        let call: VoiceCall
        let audio: ScriptedVoiceAudio
        let model: ScriptedVoiceModel
        let hub: Hub
    }

    func makeCall(
        _ answers: [ScriptedVoiceModel.Answer], speech: SilentSpeech = SilentSpeech(perCharacter: .milliseconds(2)),
        audio: ScriptedVoiceAudio? = nil, pace: Duration = .zero, endOfSpeech: Duration = .milliseconds(120),
        stillWorkingEvery: Duration = .seconds(120), memo: String = "", ended: (@MainActor @Sendable ([ChatMessage]) -> Void)? = nil
    ) -> Setup {
        let audio = audio ?? ScriptedVoiceAudio()
        let model = ScriptedVoiceModel(answers, pace: pace)
        let hub = Hub()
        let call = VoiceCall(.init(
            audio: audio, model: model, speech: speech, hub: hub, language: .zh,
            endOfSpeech: endOfSpeech, quietBeforeNews: .milliseconds(100), watchEvery: .milliseconds(250),
            stillWorkingEvery: stillWorkingEvery, memo: memo, ended: ended))
        return Setup(call: call, audio: audio, model: model, hub: hub)
    }

    /// Waits until something is true, three seconds at most.
    func until(_ condition: @MainActor () async -> Bool) async {
        for _ in 0..<300 {
            if await condition() { return }
            try? await Task.sleep(for: .milliseconds(10))
        }
    }

    func started(_ setup: Setup) async {
        setup.call.start()
        await until { setup.call.state == .listening }
    }

    /// What the voice said last.
    func said(_ setup: Setup) -> String {
        setup.call.lines.last { $0.kind == .assistant }?.text ?? ""
    }

    static func call(_ name: String, _ arguments: [String: String] = [:], id: String = "c1") -> ToolCall {
        let json = String(data: try! JSONSerialization.data(withJSONObject: arguments), encoding: .utf8)!
        return ToolCall(id: id, name: name, arguments: json)
    }

    @Test func aQuestionIsAnsweredOutLoud() async {
        // (A long pause ends what is said here, so that two words heard
        // one after the other are one sentence however busy the machine is.)
        let setup = makeCall([.init("它还在做。刚改完版式。")], endOfSpeech: .milliseconds(600))
        #expect(setup.call.state == .connecting)
        await started(setup)
        setup.audio.hear("做到")
        await until { setup.call.lines.last?.text == "做到" }
        setup.audio.hear("做到哪了")
        await until { setup.call.lines.last?.text == "做到哪了" }
        // One line that grows as the person goes on.
        #expect(setup.call.lines.map(\.kind) == [.hearing])
        // Going quiet ends what was said; the model is asked.
        await until { setup.call.lines.contains { $0.kind == .assistant && $0.text == "它还在做。刚改完版式。" } }
        await until { setup.call.state == .listening }
        #expect(setup.call.lines.map(\.kind) == [.user, .assistant])
        #expect(setup.audio.queued > 0)
        // The recognizer was told to start over: its next words are new.
        #expect(setup.audio.cuts >= 1)
        let asked = await setup.model.asked
        #expect(asked.count == 1)
        #expect(asked[0].first?.role == "system")
        #expect(asked[0].first?.content.contains("「前端」") == true)
        #expect(asked[0].first?.content.contains("它手上没有你交的活。") == true)
        #expect(asked[0].last == ChatMessage(role: "user", content: "做到哪了"))
        setup.call.hangUp()
        #expect(setup.call.state == .ended)
        #expect(setup.audio.stopped)
    }

    @Test func aSettledSentenceDoesNotWaitForQuiet() async {
        let setup = makeCall([.init("好。")])
        await started(setup)
        setup.audio.hear("做到哪了。", final: true)
        await until { setup.call.lines.contains { $0.kind == .assistant } }
        #expect(setup.call.lines.first?.kind == .user)
        #expect(setup.call.lines.first?.text == "做到哪了。")
        // It settled by itself: nothing to start over.
        #expect(setup.audio.cuts == 0)
        setup.call.hangUp()
    }

    @Test func aNodIsNotATurn() async {
        let setup = makeCall([.init("不该说话")])
        await started(setup)
        setup.audio.hear("嗯")
        try? await Task.sleep(for: .milliseconds(200))
        #expect(setup.call.lines.isEmpty)
        #expect(await setup.model.asked.isEmpty)
        #expect(setup.call.state == .listening)
        setup.call.hangUp()
    }

    @Test func whatIsWantedGoesToTheAgentMarkedAsSaid() async {
        let setup = makeCall([
            .init(calls: [Self.call(VoiceTools.tell, ["text": "把下载页的标题改短一点", "what": "改短标题", "their_words": "顺便把标题也改短一点"])]),
            .init("好，转过去了。"),
        ])
        await started(setup)
        setup.audio.hear("顺便把标题也改短一点")
        // A few words before it acts (the model said nothing first), then
        // what it says once it is done: not silence until then.
        await until { said(setup) == "好，我跟它说。好，转过去了。" }
        #expect(said(setup) == "好，我跟它说。好，转过去了。")
        #expect(setup.hub.told == ["把下载页的标题改短一点\n\n——\n（经语音转述。原话：顺便把标题也改短一点）"])
        // What it did is a line of its own, under the words it began with.
        #expect(setup.call.lines.map(\.kind) == [.user, .assistant, .action])
        #expect(setup.call.lines.first { $0.kind == .action }?.text == "交给 前端：改短标题")
        // The model saw how the tool went.
        let asked = await setup.model.asked
        #expect(asked.count == 2)
        #expect(asked[1].last?.role == "tool")
        #expect(asked[1].last?.content == "sent as a new task")
        #expect(asked[1].last?.toolCallID == "c1")
        // The agent has a task now: the next thing said is added to it.
        await until { setup.call.state == .listening }
        setup.call.hangUp()
    }

    @Test func addingStoppingAndLookingAgain() async {
        let setup = makeCall([
            .init(calls: [Self.call(VoiceTools.tell, ["text": "注册页也改", "what": "注册页"])]),
            .init("补上了。"),
            .init(calls: [Self.call(VoiceTools.check, id: "c2")]),
            .init("它还在做。"),
            .init(calls: [Self.call(VoiceTools.stop, id: "c3")]),
            .init("停了。"),
            .init(calls: [Self.call(VoiceTools.stop, id: "c4")]),
            .init("它现在没在做什么。"),
        ])
        setup.hub.current = "t9"
        await started(setup)
        setup.audio.hear("注册页也改一下")
        await until { said(setup) == "好，我跟它说。补上了。" && setup.call.state == .listening }
        #expect(setup.call.lines.first { $0.kind == .action }?.text == "补充给 前端：注册页")
        #expect(setup.hub.freshContexts == 0)
        setup.audio.hear("现在怎么样了")
        await until { said(setup) == "我看一眼。它还在做。" && setup.call.state == .listening }
        // Having passed something on, the conversation was read again for
        // this answer; and looking again reads it once more.
        #expect(setup.hub.freshContexts == 2)
        setup.audio.hear("让它停下")
        await until { said(setup) == "好，我让它停下。停了。" && setup.call.state == .listening }
        #expect(setup.hub.stops == 1)
        #expect(setup.hub.freshContexts == 2, "nothing was done since: what is on the page will do")
        #expect(setup.call.lines.contains { $0.kind == .action && $0.text == "让 前端 停下了" })
        // Nothing at work: the model is told so, and no action is shown.
        setup.audio.hear("再让它停一次")
        await until { said(setup) == "好，我让它停下。它现在没在做什么。" }
        #expect(setup.hub.freshContexts == 3, "it stopped the agent: read again")
        let asked = await setup.model.asked
        #expect(asked.last?.last?.content == "error: nothing at work")
        #expect(setup.call.lines.filter { $0.kind == .action }.count == 2)
        setup.call.hangUp()
    }

    @Test func speakingOverItCutsItOff() async {
        let setup = makeCall([.init("它刚把下载页的版式改完。现在正在一条一条地核对里面的链接。"), .init("好的。")],
                             speech: SilentSpeech(perCharacter: .milliseconds(40)))
        await started(setup)
        setup.audio.hear("做到哪了")
        await until { setup.call.state == .speaking }
        // Its own voice coming back (heard a little wrong, as echoes
        // are), or a nod, is not the person cutting in.
        setup.audio.hear("他刚把下载页的板式")
        try? await Task.sleep(for: .milliseconds(50))
        #expect(setup.call.state == .speaking)
        #expect(setup.audio.flushes == 0)
        // The person's words come after that echo in the recognizer's text.
        setup.audio.hear("他刚把下载页的板式，等一下")
        await until { setup.audio.flushes == 1 }
        #expect(setup.audio.flushes == 1)
        let cut = try! #require(setup.call.lines.first { $0.kind == .assistant })
        #expect(cut.interrupted)
        // What the person heard of it: the second sentence never began.
        #expect(cut.text == "它刚把下载页的版式改完。")
        // What it got to say is kept, marked as cut off, for the next answer.
        await until { setup.call.lines.last?.text == "好的。" }
        let asked = await setup.model.asked
        #expect(asked.count == 2)
        #expect(asked[1].contains { $0.role == "assistant" && $0.content.hasSuffix("（被打断了）") })
        #expect(asked[1].last == ChatMessage(role: "user", content: "等一下"))
        setup.call.hangUp()
    }

    @Test func anEchoIsToldFromThePersonCuttingIn() {
        let said = "它刚把下载页的版式改完。现在正在一条一条地核对里面的链接。"
        #expect(VoiceCall.isEcho("下载页的版式", of: said))
        // Heard a little wrong, as an echo is.
        #expect(VoiceCall.isEcho("他刚把下载页的板式改完", of: said))
        #expect(!VoiceCall.isEcho("等一下", of: said))
        #expect(!VoiceCall.isEcho("下载页先不要改了", of: said))
        #expect(!VoiceCall.isEcho("等一下", of: ""))
        let english = "It just finished the layout of the download page."
        #expect(VoiceCall.isEcho("the layout of the download", of: english))
        #expect(!VoiceCall.isEcho("wait, stop that", of: english))
        // What follows an echo in the recognizer's text is the person's.
        #expect(VoiceCall.after("他刚把下载页的板式", in: "他刚把下载页的板式，等一下") == "等一下")
        #expect(VoiceCall.after("他刚把下载页", in: "等一下") == "等一下")
        #expect(VoiceCall.after("", in: "等一下") == "等一下")
        #expect(VoiceCall.after("等一下", in: "等一下") == "")
    }

    @Test func theAgentsAnswerIsToldInAQuietMoment() async {
        let setup = makeCall([.init("它回复了，说首页没问题。")])
        setup.hub.current = "t1"
        await started(setup)
        setup.hub.newsOf["t1"] = TaskNews(finished: true, summary: "首页看过了，没问题。")
        await until { setup.call.lines.last?.text == "它回复了，说首页没问题。" }
        let asked = await setup.model.asked
        #expect(asked.count == 1)
        #expect(asked[0].last?.role == "system")
        #expect(asked[0].last?.content.contains("它回复了：首页看过了，没问题。") == true)
        // Told once.
        try? await Task.sleep(for: .milliseconds(600))
        #expect(await setup.model.asked.count == 1)
        setup.call.hangUp()
    }

    @Test func aQuestionFromTheAgentIsToldOnce() async {
        let setup = makeCall([.init("它想知道标题用哪一版。")])
        setup.hub.current = "t1"
        await started(setup)
        setup.hub.newsOf["t1"] = TaskNews(question: "标题用哪一版？")
        await until { setup.call.lines.last?.text == "它想知道标题用哪一版。" }
        let asked = await setup.model.asked
        #expect(asked[0].last?.content.contains("它在问你：标题用哪一版？") == true)
        try? await Task.sleep(for: .milliseconds(600))
        #expect(await setup.model.asked.count == 1)
        setup.call.hangUp()
    }

    /// The desktop app's TestVoiceCallAckOnlyWhenSilent.
    @Test func wordsBeforeActingAreNotSaidTwice() async {
        // The model said what it is about to do: nothing is added.
        let spoken = makeCall([
            .init("好，让它加暗色模式。", calls: [Self.call(VoiceTools.tell, ["text": "加一个暗色模式", "what": "暗色模式"])]),
            .init("转过去了。"),
        ])
        await started(spoken)
        spoken.audio.hear("加个暗色模式")
        await until { said(spoken) == "好，让它加暗色模式。转过去了。" }
        #expect(said(spoken) == "好，让它加暗色模式。转过去了。")
        spoken.call.hangUp()

        // Half a sentence when it starts to act: that is said then, not
        // held back until the action is over; nothing else is added.
        let half = makeCall([
            .init("好我让它改", calls: [Self.call(VoiceTools.tell, ["text": "改标题", "what": "标题"])]),
            .init("转过去了。"),
        ], pace: .milliseconds(20))
        await started(half)
        half.audio.hear("把标题改一下")
        await until { half.hub.told.count == 1 }
        #expect(said(half).hasPrefix("好我让它改"))
        await until { said(half) == "好我让它改转过去了。" }
        #expect(!said(half).contains("我跟它说"))
        half.call.hangUp()

        #expect(VoicePrompt.toolAck(language: .en, tool: VoiceTools.stop) == "OK, I'll stop it. ")
        #expect(VoicePrompt.toolAck(language: .zh, tool: VoiceTools.check) == "我看一眼。")
        #expect(VoicePrompt.toolAck(language: .zh, tool: "something_new") == "稍等。")
        // The prompt asks for the same: a few words before a tool.
        #expect(VoicePrompt.system(language: .zh, agent: "前端", conversation: "").contains("调用工具之前，先用几个字说你要做什么"))
        #expect(VoicePrompt.system(language: .en, agent: "Frontend", conversation: "").contains("Before you call a tool, say in a few words"))
    }

    /// The desktop app's TestVoiceCallSaysALongTaskIsStillGoing.
    @Test func aLongTaskGetsAWordThatItIsStillGoing() async {
        let setup = makeCall([
            .init(calls: [Self.call(VoiceTools.tell, ["text": "把整个项目的测试跑一遍", "what": "跑全部测试"])]),
            .init("转过去了。"),
            .init("它还在跑测试。"),
            .init("还在跑。"),
        ], stillWorkingEvery: .milliseconds(400))
        await started(setup)
        setup.hub.newsOf["task-1"] = TaskNews(status: "acked", since: Date().addingTimeInterval(-6 * 60 - 5), progress: "在跑集成测试")
        setup.audio.hear("把测试都跑一遍")
        await until { said(setup).hasSuffix("转过去了。") && setup.call.state == .listening }
        func notices() async -> [String] {
            var out: [String] = []
            for messages in await setup.model.asked {
                for message in messages where message.role == "system" && message.content.contains("还在进行") && !out.contains(message.content) {
                    out.append(message.content)
                }
            }
            return out
        }
        await until { await !notices().isEmpty }
        let first = await notices().first ?? ""
        #expect(first.contains("「跑全部测试」"))
        #expect(first.contains("它在做"))
        #expect(first.contains("已经 6 分钟了"))
        #expect(first.contains("它最新报的进展：在跑集成测试"))
        #expect(first.contains("别说做完了"))
        // And again after as long, without the note already told.
        await until { await notices().count > 1 }
        let second = await notices().dropFirst().first ?? ""
        #expect(!second.isEmpty)
        #expect(!second.contains("在跑集成测试"))
        setup.call.hangUp()

        // The words of it: a task not started yet is said to be waiting,
        // and under a minute no minutes are counted.
        let waiting = VoiceCall.stillWorking(
            what: "", TaskNews(status: "delivered", since: Date(timeIntervalSince1970: 1_000)), told: "", now: Date(timeIntervalSince1970: 1_030))
        #expect(waiting.contains("它已送达，还没开始。"))
        #expect(!waiting.contains("分钟"))
    }

    /// The desktop app's TestVoiceMemoCarriesTheLastCallOver.
    @Test func theNextCallStartsWithTheEndOfTheLastOne() async {
        let store = MemoryMemoStore()
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        var history: [ChatMessage] = []
        for index in 0..<6 {
            history += [
                ChatMessage(role: "user", content: "第 \(index) 句"),
                ChatMessage(role: "assistant", content: "", toolCalls: [ToolCall(id: "x", name: VoiceTools.tell, arguments: "{}")]),
                ChatMessage(role: "tool", content: "sent", toolCallID: "x"),
                ChatMessage(role: "system", content: "（系统消息）"),
                ChatMessage(role: "assistant", content: "回答 \(index)"),
            ]
        }
        let key = VoiceMemo.key(workspace: "h1", agent: "backend")
        VoiceMemos.keep(history, for: key, in: store, now: now)
        // The last eight lines said: the person's and the voice's words,
        // not what was done or what the system said.
        let memo = VoiceMemos.memo(for: key, in: store, now: now.addingTimeInterval(60))
        #expect(memo?.lines.count == 8)
        #expect(memo?.lines.first == VoiceMemo.Line(who: "person", text: "第 2 句"))
        #expect(memo?.lines.last == VoiceMemo.Line(who: "assistant", text: "回答 5"))
        // By workspace and agent.
        #expect(VoiceMemos.memo(for: VoiceMemo.key(workspace: "h1", agent: "frontend"), in: store, now: now) == nil)
        #expect(VoiceMemos.memo(for: VoiceMemo.key(workspace: "h2", agent: "backend"), in: store, now: now) == nil)
        // A call where nothing was said keeps the memo from before.
        VoiceMemos.keep([ChatMessage(role: "system", content: "x")], for: key, in: store, now: now.addingTimeInterval(30))
        #expect(VoiceMemos.memo(for: key, in: store, now: now.addingTimeInterval(60))?.lines.first?.text == "第 2 句")
        // A long line is cut; a day later the memo is gone, and another
        // call's memo kept then drops it from the device.
        #expect(VoiceMemo.lines(of: [ChatMessage(role: "user", content: String(repeating: "长", count: 400))]).first?.text.count == 301)
        #expect(VoiceMemos.memo(for: key, in: store, now: now.addingTimeInterval(25 * 3600)) == nil)
        let other = VoiceMemo.key(workspace: "h1", agent: "tester")
        VoiceMemos.keep([ChatMessage(role: "user", content: "嗨")], for: other, in: store, now: now.addingTimeInterval(25 * 3600))
        #expect(VoiceMemos.memo(for: key, in: store, now: now.addingTimeInterval(60)) == nil)
        #expect(VoiceMemos.memo(for: other, in: store, now: now.addingTimeInterval(25 * 3600 + 60)) != nil)

        // In words, for the model.
        let prompt = memo?.prompt(language: .zh, now: now.addingTimeInterval(60)) ?? ""
        #expect(prompt.hasPrefix("\n上一通电话（1 分钟前）最后说的，供参考，以上面的对话为准；用得上再提，别主动复述：\n"))
        #expect(prompt.contains("- 对方：第 2 句\n"))
        #expect(prompt.hasSuffix("- 你：回答 5\n"))
        let english = memo?.prompt(language: .en, now: now.addingTimeInterval(3 * 3600 + 10)) ?? ""
        #expect(english.contains("The end of your last call with them (3 h ago), for reference"))
        #expect(english.contains("- They：第 2 句\n"))
        #expect(english.contains("- You：回答 5\n"))

        // A call: the memo goes into its instructions, and what was said
        // is handed back when it ends.
        final class Kept: @unchecked Sendable { var history: [ChatMessage]? }
        let kept = Kept()
        let setup = makeCall([.init("下午再说，记住了。")], memo: prompt, ended: { kept.history = $0 })
        await started(setup)
        setup.audio.hear("这个先不急，下午再说")
        await until { said(setup) == "下午再说，记住了。" && setup.call.state == .listening }
        let instructions = await setup.model.asked.first?.first?.content ?? ""
        #expect(instructions.contains("上一通电话"))
        #expect(instructions.contains("回答 5"))
        #expect(kept.history == nil)
        setup.call.hangUp()
        let lines = VoiceMemo.lines(of: kept.history ?? [])
        #expect(lines.map(\.text) == ["这个先不急，下午再说", "下午再说，记住了。"])
    }

    @Test func aMemoIsKeptInAFileOfItsOwn() throws {
        let folder = FileManager.default.temporaryDirectory.appendingPathComponent("memo-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: folder) }
        let store = FileMemoStore(url: folder.appendingPathComponent("voice-memo.json"))
        #expect(store.read() == nil)
        let key = VoiceMemo.key(workspace: "h1", agent: "backend")
        VoiceMemos.keep([ChatMessage(role: "user", content: "嗨")], for: key, in: store)
        // Another launch reads the same file.
        let again = FileMemoStore(url: folder.appendingPathComponent("voice-memo.json"))
        #expect(VoiceMemos.memo(for: key, in: again)?.lines == [VoiceMemo.Line(who: "person", text: "嗨")])
    }

    @Test func mutedItHearsNothing() async {
        let setup = makeCall([.init("在。")])
        await started(setup)
        setup.call.setMuted(true)
        setup.audio.hear("这句不算")
        try? await Task.sleep(for: .milliseconds(200))
        #expect(setup.call.lines.isEmpty)
        let cuts = setup.audio.cuts
        setup.call.setMuted(false)
        // What was said meanwhile is not the start of what comes next.
        #expect(setup.audio.cuts == cuts + 1)
        setup.audio.hear("在吗")
        await until { setup.call.lines.last?.text == "在。" }
        #expect(await setup.model.asked.first?.last?.content == "在吗")
        setup.call.hangUp()
    }

    @Test func aModelThatFailsSaysSoAndTheCallGoesOn() async {
        let audio = ScriptedVoiceAudio()
        let hub = Hub()
        let call = VoiceCall(.init(audio: audio, model: FailingModel(), speech: SilentSpeech(), hub: hub, language: .zh, endOfSpeech: .milliseconds(40)))
        call.start()
        await until { call.state == .listening }
        audio.hear("在吗")
        await until { call.lines.contains { $0.kind == .notice } }
        #expect(call.lines.map(\.kind) == [.user, .notice])
        #expect(call.lines.last?.text == "语音助手没能回答：HTTP 401: Incorrect API key")
        #expect(call.state == .listening)
        call.hangUp()
    }

    @Test func aVoiceThatFailsStillShowsTheWords() async {
        let setup = makeCall([.init("在的。")], speech: SilentSpeech(failure: "豆包语音 HTTP 403"))
        await started(setup)
        setup.audio.hear("在吗")
        await until { setup.call.lines.contains { $0.kind == .notice } && setup.call.state == .listening }
        #expect(setup.call.lines.contains { $0.kind == .assistant && $0.text == "在的。" })
        #expect(setup.call.lines.contains { $0.kind == .notice && $0.text == "说不出声：豆包语音 HTTP 403" })
        setup.call.hangUp()
    }

    @Test func withoutTheMicrophoneTheCallEndsAndSaysWhy() async {
        let setup = makeCall([], audio: ScriptedVoiceAudio(problem: .microphoneDenied))
        setup.call.start()
        await until { setup.call.state == .ended }
        #expect(setup.call.problem == .microphoneDenied)
    }

    @Test func theDemoCallAsksAndPassesOn() async {
        let hub = Hub()
        let audio = ScriptedVoiceAudio([
            .init(after: .milliseconds(20), ["做到", "做到哪了"]),
            .init(after: .milliseconds(400), ["顺便把标题也改短一点"]),
        ], pace: .milliseconds(10))
        let call = VoiceCall(.init(
            audio: audio, model: ScriptedVoiceModel.demo(), speech: SilentSpeech(perCharacter: .milliseconds(1)), hub: hub, language: .zh,
            endOfSpeech: .milliseconds(60)))
        call.start()
        await until { hub.told.count == 1 && call.state == .listening && call.lines.last?.kind == .assistant }
        #expect(hub.told.first?.hasPrefix("顺便把标题也改短一点\n\n——\n（经语音转述") == true)
        #expect(call.lines.contains { $0.kind == .assistant && $0.text.contains("还在做") })
        call.hangUp()
    }
}

/// The call's myco side on the conversation on screen.
@MainActor
struct ConversationVoiceTests {
    @Test func whatIsToldGoesTheWayTypingDoes() async throws {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "demo", name: "Demo", hub: hub)
        await session.refresh()
        // A member never talked to.
        let agent = "studio-codex-e07b"
        let model = session.conversation(with: agent)
        await model.reload()
        let voice = ConversationVoice(session: session, model: model)
        #expect(voice.current == nil)
        #expect(await voice.context(fresh: false).contains("你们还没有说过话"))
        // Free: a new task.
        let first = try await voice.tell(ViaVoice.body("看一下首页", heard: "看下首页"))
        #expect(!first.steered)
        #expect(voice.current == first.task)
        #expect(model.openTask?.id == first.task)
        // At work: added to that task.
        let second = try await voice.tell(ViaVoice.body("标题也看看", heard: ""))
        #expect(second.steered)
        #expect(second.task == first.task)
        let sent = await hub.allMessages().filter { $0.from == FakeHub.me && $0.to == agent }
        #expect(sent.map(\.type).sorted() == ["steer", "task"])
        // Not done and not asking: at work since it was sent.
        let standing = voice.news(of: first.task)
        #expect(standing?.finished == false)
        #expect(standing?.question == "")
        #expect(standing?.status == "acked")
        #expect(standing?.since != nil)
        let context = await voice.context(fresh: true)
        #expect(context.contains("你（任务）：看一下首页"))
        #expect(context.contains("它手上正在做你交的：看一下首页"))
        // Stopped: its ending shows as news, and nothing is left to stop.
        try await voice.stopCurrent()
        #expect(voice.current == nil)
        #expect(voice.news(of: first.task)?.finished == true)
        await #expect(throws: VoiceFailure.self) { try await voice.stopCurrent() }
        // What typed words look like is untouched.
        #expect(model.draft.isEmpty)
    }
}
