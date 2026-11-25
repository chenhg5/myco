import Foundation
import Observation

// Talking with an agent by voice, in the person's
// chat with it: the phone hears the person (speech recognition), a fast
// text model speaks as the agent while its working session does the
// work requested, as if typed in this chat. It stops talking as soon as
// the person speaks, and reports its own results and questions in a pause.
//
// On the desktop this loop is in the App's local backend
// (internal/console/voice_call.go). A phone has no such backend: the same
// loop runs here, with the same rules.

/// What the microphone and the speaker tell the call.
public enum VoiceAudioEvent: Sendable, Hashable {
    /// The microphone is open: the call is on.
    case ready
    /// The person's words so far in this utterance; `final` when the
    /// recognizer has settled it.
    case heard(String, final: Bool)
    /// The speaker has played everything queued.
    case played
}

/// A voice call's microphone and speaker: the phone's (one audio engine
/// with the system's voice processing, so the call's own voice is not
/// heard as the person's), or a script in the demo and in tests.
@MainActor
public protocol VoiceAudio: AnyObject {
    /// Opens the microphone and recognises speech in a language. The
    /// stream ends when the microphone closes; it throws a
    /// `DictationProblem` when it could not open or was taken away.
    func start(language: AppLanguage) -> AsyncThrowingStream<VoiceAudioEvent, Error>
    /// Queues sound to play: 16-bit little-endian mono samples at `rate` a second.
    func play(_ samples: Data, rate: Int)
    /// Stops what is playing and drops what is queued.
    func flush()
    /// What was heard so far has been taken: the next words start a new
    /// utterance, and anything late from the old one is dropped.
    func newUtterance()
    /// Closes the microphone and the speaker.
    func stop()
}

/// One line of the call, as its bar shows it.
public struct VoiceLine: Identifiable, Hashable, Sendable {
    public enum Kind: Sendable, Hashable {
        /// The person said (settled).
        case user
        /// The person is saying.
        case hearing
        /// The voice said.
        case assistant
        /// It did something in myco.
        case action
        /// Something to know (an error, a hint).
        case notice
    }

    public var id: Int
    public var kind: Kind
    public var text: String
    /// The voice was cut off; `text` is what it got to say.
    public var interrupted = false
}

/// One call with one agent.
@MainActor
@Observable
public final class VoiceCall {
    public enum State: Sendable, Hashable {
        /// The microphone is not open yet.
        case connecting
        /// Waiting for the person.
        case listening
        /// The model is working on an answer.
        case thinking
        /// The answer is being said.
        case speaking
        /// Hung up.
        case ended
    }

    /// What a call is made of.
    public struct Parts {
        public var audio: any VoiceAudio
        public var model: any VoiceModel
        public var speech: any VoiceSpeech
        public var hub: any VoiceHub
        public var language: AppLanguage
        /// This long without a new word ends what the person is saying.
        public var endOfSpeech: Duration
        /// News waits for this much quiet before it is told.
        public var quietBeforeNews: Duration
        /// How often the tasks handed out are looked at.
        public var watchEvery: Duration
        /// How long the call goes without a word about a task that is not
        /// done before it says that it still is (a long task would
        /// otherwise be minutes of silence).
        public var stillWorkingEvery: Duration
        /// Whether the agent says hello when the microphone opens.
        public var greet: Bool
        /// The end of the last call with this agent, for the model's
        /// instructions (`VoiceMemo.prompt`); empty when there was none.
        public var memo: String
        /// Gets what was said in this call when it ends (to keep its memo).
        public var ended: (@MainActor @Sendable ([ChatMessage]) -> Void)?

        public init(
            audio: any VoiceAudio, model: any VoiceModel, speech: any VoiceSpeech, hub: any VoiceHub, language: AppLanguage = .current,
            endOfSpeech: Duration = .milliseconds(900), quietBeforeNews: Duration = .milliseconds(1500), watchEvery: Duration = .seconds(2),
            stillWorkingEvery: Duration = .seconds(120), memo: String = "", greet: Bool = true, ended: (@MainActor @Sendable ([ChatMessage]) -> Void)? = nil
        ) {
            self.audio = audio
            self.model = model
            self.speech = speech
            self.hub = hub
            self.language = language
            self.endOfSpeech = endOfSpeech
            self.quietBeforeNews = quietBeforeNews
            self.watchEvery = watchEvery
            self.stillWorkingEvery = stillWorkingEvery
            self.memo = memo
            self.greet = greet
            self.ended = ended
        }
    }

    public private(set) var state: State = .connecting
    public private(set) var lines: [VoiceLine] = []
    public private(set) var muted = false
    /// Why the call ended by itself (no microphone, no permission).
    public private(set) var problem: DictationProblem?

    private let parts: Parts
    private let clock = ContinuousClock()

    /// The talk so far, as the model reads it.
    @ObservationIgnored private var history: [ChatMessage] = []
    /// What the person is saying, and when its last word came.
    @ObservationIgnored private var partial = ""
    @ObservationIgnored private var partialAt: ContinuousClock.Instant?
    @ObservationIgnored private var endTimer: Task<Void, Never>?
    /// The last moment anyone made a sound.
    @ObservationIgnored private var lastSound = ContinuousClock.now
    /// The speaker still has something to play.
    @ObservationIgnored private var speaking = false
    /// When what was queued should have been played, by its length.
    @ObservationIgnored private var playUntil = ContinuousClock.now
    /// The answer being made.
    @ObservationIgnored private var turn: Turn?
    /// What the voice said last: its echo is not the person cutting in.
    @ObservationIgnored private var lastSaid = ""
    /// Words were heard while the voice talked and left alone (its own
    /// echo, an "嗯"): they are still in the recognizer's text.
    @ObservationIgnored private var residue = false
    /// Those words, as the recognizer gave them: what the person says
    /// next comes after them in its text.
    @ObservationIgnored private var echoed = ""
    /// News about tasks, waiting for a quiet moment.
    @ObservationIgnored private var news: [String] = []
    @ObservationIgnored private var watched: [String: Watched] = [:]
    /// The voice started or stopped work: the
    /// conversation is read again before the next answer.
    @ObservationIgnored private var acted = false
    @ObservationIgnored private var nextLine = 0
    @ObservationIgnored private var running: [Task<Void, Never>] = []

    /// One answer: the model's rounds and the saying of them.
    @MainActor
    private final class Turn {
        let line: Int
        /// What the model wrote so far, and how much of it the voice has
        /// begun to say.
        var said = ""
        var spoken = ""
        var splitter = SentenceSplitter()
        var work: Task<Void, Never>?
        var voice: Task<Void, Never>?

        init(line: Int) {
            self.line = line
        }
    }

    private struct Watched {
        /// What it was asked, short.
        var what: String
        /// The question it asked last (told once).
        var asked = ""
        var finished = false
        /// When the person last heard about it.
        var toldAt: ContinuousClock.Instant
        /// The progress note told last.
        var progress = ""
    }

    public init(_ parts: Parts) {
        self.parts = parts
    }

    public var agentName: String { parts.hub.name }

    // MARK: Starting and ending

    /// Opens the microphone and begins.
    public func start() {
        guard state == .connecting, running.isEmpty else { return }
        let events = parts.audio.start(language: parts.language)
        running.append(Task { [weak self] in
            do {
                for try await event in events {
                    guard let self, self.state != .ended else { return }
                    self.handle(event)
                }
                self?.end(nil)
            } catch let problem as DictationProblem {
                self?.end(problem)
            } catch is CancellationError {
                self?.end(nil)
            } catch {
                self?.end(.other(error.localizedDescription))
            }
        })
        let step = Duration.milliseconds(250)
        let watchEvery = parts.watchEvery
        running.append(Task { [weak self] in
            var sinceWatch = Duration.zero
            while !Task.isCancelled {
                try? await Task.sleep(for: step)
                guard let self, self.state != .ended else { return }
                sinceWatch += step
                if sinceWatch >= watchEvery {
                    sinceWatch = .zero
                    self.lookAtTasks()
                }
                self.missedPlayed()
                self.maybeTellNews()
            }
        })
    }

    public func hangUp() {
        end(nil)
    }

    private func end(_ problem: DictationProblem?) {
        guard state != .ended else { return }
        state = .ended
        self.problem = problem
        endTimer?.cancel()
        turn?.work?.cancel()
        turn?.voice?.cancel()
        turn = nil
        for task in running { task.cancel() }
        running = []
        if let last = lines.last, last.kind == .hearing { lines.removeLast() }
        parts.audio.flush()
        parts.audio.stop()
        // What was said, for the next call with this agent.
        parts.ended?(history)
    }

    /// Muted: nothing the microphone hears is taken.
    public func setMuted(_ on: Bool) {
        guard muted != on, state != .ended else { return }
        muted = on
        if on {
            endTimer?.cancel()
            partial = ""
            if let last = lines.last, last.kind == .hearing { lines.removeLast() }
        } else {
            // What was said meanwhile is not part of what comes next.
            residue = false
            echoed = ""
            parts.audio.newUtterance()
        }
    }

    // MARK: What the microphone and the speaker say

    private func handle(_ event: VoiceAudioEvent) {
        switch event {
        case .ready:
            guard state == .connecting else { return }
            state = .listening
            lastSound = clock.now
            // What the agent is doing already: its answer is news too.
            if let current = parts.hub.current { watch(current, what: "") }
            if parts.greet { greet() }
        case .heard(let text, let final):
            heard(text.trimmingCharacters(in: .whitespacesAndNewlines), final: final)
        case .played:
            speaking = false
            lastSound = clock.now
            if turn == nil {
                if state == .speaking || state == .thinking { state = .listening }
                dropResidue()
            }
        }
    }

    /// The speaker should have said by now that it played everything
    /// (the audio was rebuilt under it, or a piece went nowhere): it is
    /// taken as played, so the call does not wait for ever.
    private func missedPlayed() {
        guard speaking, clock.now > playUntil + .seconds(2) else { return }
        handle(.played)
    }

    /// The person's words so far. While the voice talks, real words (not
    /// "嗯", not its own voice coming back) cut it off.
    private func heard(_ whole: String, final: Bool) {
        guard state != .connecting, state != .ended, !muted else { return }
        guard !whole.isEmpty else {
            // The recognizer settled on what was already taken.
            if final, !partial.isEmpty { finishUtterance() }
            return
        }
        let now = clock.now
        // What was taken for the voice's echo is still at the start of
        // the recognizer's text: the person's words are what follows it.
        let text = Self.after(echoed, in: whole)
        if turn != nil || speaking {
            guard interrupts(text) else {
                residue = true
                echoed = whole
                return
            }
            interrupt()
        } else if text.isEmpty {
            return
        }
        partial = text
        partialAt = now
        lastSound = now
        showHearing(text)
        endTimer?.cancel()
        if final {
            finishUtterance()
            return
        }
        let pause = parts.endOfSpeech
        endTimer = Task { [weak self] in
            try? await Task.sleep(for: pause)
            guard !Task.isCancelled, let self, self.partialAt == now else { return }
            self.finishUtterance(cut: true)
        }
    }

    /// What people say to show they listen; not a turn.
    nonisolated static let backchannel: Set<String> = [
        "嗯", "嗯嗯", "啊", "哦", "噢", "对", "对对", "好", "是", "嗯哼", "ok", "okay", "mm", "uhhuh", "yeah",
    ]

    /// Letters and digits only, lower case: how heard words are compared.
    nonisolated static func normalize(_ text: String) -> String {
        String(String.UnicodeScalarView(text.lowercased().unicodeScalars.filter {
            $0.properties.isAlphabetic || $0.properties.numericType != nil
        }))
    }

    /// Whether words heard while the voice talks are the person cutting
    /// in: at least two real characters, not a backchannel, and not what
    /// the voice is saying coming back through the microphone.
    private func interrupts(_ text: String) -> Bool {
        let heard = Self.normalize(text)
        guard heard.count >= 2, !Self.backchannel.contains(heard) else { return false }
        return !Self.isEcho(text, of: turn?.said ?? lastSaid)
    }

    /// Whether what was heard is what the voice said, come back through
    /// the microphone. The recognizer rarely hears an echo word for word
    /// (a character here and there comes out as another that sounds the
    /// same), so a long stretch in common is enough: four characters in
    /// a row for Chinese, ten letters otherwise, and two fifths of what
    /// was heard.
    nonisolated static func isEcho(_ heard: String, of said: String) -> Bool {
        let heard = Array(normalize(heard)), said = Array(normalize(said))
        guard !heard.isEmpty, !said.isEmpty else { return false }
        var best = 0
        var runs = [Int](repeating: 0, count: said.count + 1)
        for character in heard {
            var previous = 0
            for index in 1...said.count {
                let above = runs[index]
                runs[index] = said[index - 1] == character ? previous + 1 : 0
                best = max(best, runs[index])
                previous = above
            }
        }
        if best == heard.count { return true }
        let wide = heard.contains { $0.unicodeScalars.contains { $0.value >= 0x2E80 } }
        return best >= (wide ? 4 : 10) && best * 5 >= heard.count * 2
    }

    /// What follows `echo` in the recognizer's text; all of it when the
    /// text does not begin with it (the recognizer started over, or
    /// changed its mind about the beginning).
    nonisolated static func after(_ echo: String, in text: String) -> String {
        guard !echo.isEmpty else { return text }
        var left = normalize(echo).count
        guard left > 0, normalize(text).hasPrefix(normalize(echo)) else { return text }
        var rest = Substring(text)
        while left > 0, let first = rest.first {
            left -= normalize(String(first)).count
            rest = rest.dropFirst()
        }
        return rest.trimmingCharacters(in: .whitespacesAndNewlines.union(.punctuationCharacters))
    }

    /// Stops the voice: nothing more is said, and what it got to say is
    /// kept as said.
    private func interrupt() {
        parts.audio.flush()
        speaking = false
        residue = false
        if let turn {
            turn.work?.cancel()
            turn.voice?.cancel()
            self.turn = nil
            // What the person heard of it, not all the model had written.
            let said = turn.spoken.trimmingCharacters(in: .whitespacesAndNewlines)
            if said.isEmpty {
                lines.removeAll { $0.id == turn.line }
            } else {
                update(turn.line) {
                    $0.text = said
                    $0.interrupted = true
                }
                history.append(ChatMessage(role: "assistant", content: said + text("（被打断了）", " (cut off)")))
            }
            lastSaid = turn.said
        }
        state = .listening
    }

    /// The voice talked over something it left alone: the recognizer
    /// starts clean, so that is not the start of what the person says next.
    private func dropResidue() {
        guard residue, partial.isEmpty else { return }
        residue = false
        echoed = ""
        parts.audio.newUtterance()
    }

    /// The person has finished; the voice answers. `cut`: they went
    /// quiet (the recognizer has not settled, and is told to start over).
    private func finishUtterance(cut: Bool = false) {
        let text = partial.trimmingCharacters(in: .whitespacesAndNewlines)
        partial = ""
        partialAt = nil
        endTimer?.cancel()
        endTimer = nil
        // The recognizer starts over (told to, or by itself): nothing of
        // what was heard so far is in its next text.
        residue = false
        echoed = ""
        if cut { parts.audio.newUtterance() }
        guard !text.isEmpty, !Self.backchannel.contains(Self.normalize(text)) else {
            if let last = lines.last, last.kind == .hearing { lines.removeLast() }
            return
        }
        if let last = lines.last, last.kind == .hearing {
            update(last.id) {
                $0.kind = .user
                $0.text = text
            }
        } else {
            add(.user, text)
        }
        history.append(ChatMessage(role: "user", content: text))
        startTurn()
    }

    // MARK: Answering

    /// Says hello once the call is on. A person speaking first interrupts
    /// this like any other turn, and a prior utterance is never greeted over.
    private func greet() {
        guard turn == nil, partial.isEmpty, history.isEmpty else { return }
        history.append(ChatMessage(role: "system", content: text(
            "（系统消息，不是对方说的）电话刚接通，你先开口：用一句很短的话打招呼，像接起电话那样，用第一人称（比如「我在，有什么要我做的？」）。你手上有活在做、或者在等对方回答，就顺带用半句话提一下；没有就不提。别说你是语音助手，别念对话原文。",
            "(System message, not said by the person.) The call has just connected and you speak first: one short sentence of hello, as when picking up the phone, in the first person (e.g. \"Hi, I'm here. What can I do for you?\"). If you have work in progress or are waiting on their answer, mention it in half a sentence; otherwise do not. Do not call yourself a voice assistant, and do not read out the chat.")))
        startTurn()
    }

    /// In a quiet moment, news about the tasks becomes a turn of its own.
    private func maybeTellNews() {
        guard state == .listening, !news.isEmpty, turn == nil, !speaking, partial.isEmpty, !muted else { return }
        guard clock.now - lastSound >= parts.quietBeforeNews else { return }
        let told = news.joined(separator: "\n")
        news = []
        history.append(ChatMessage(
            role: "system", content: text(
                "（系统消息，不是对方说的）你自己干活那边有了新情况。用第一人称（「我……」）、一两句自然的话告诉对方，别念原文：\n",
                "(System message, not said by the person.) There is news from your own work. Tell them in a sentence or two, in the first person (\"I...\"), in your own words; do not read it out:\n") + told))
        startTurn()
    }

    /// Runs the model on the talk so far and says what it writes.
    private func startTurn() {
        let turn = Turn(line: add(.assistant, ""))
        self.turn = turn
        state = .thinking
        let history = Self.trim(history)
        turn.work = Task { [weak self] in
            await self?.run(turn, history: history)
        }
    }

    /// Keeps the talk to its last messages, starting at a person's or a
    /// system line (never a tool's result without its call).
    nonisolated static func trim(_ history: [ChatMessage], keep: Int = 30) -> [ChatMessage] {
        guard history.count > keep else { return history }
        var kept = Array(history.suffix(keep))
        while let first = kept.first, first.role == "tool" || (first.role == "assistant" && !first.toolCalls.isEmpty) {
            kept.removeFirst()
        }
        return kept
    }

    /// The model's rounds (tools in between), each sentence said as soon
    /// as it is whole.
    private func run(_ turn: Turn, history: [ChatMessage]) async {
        // The conversation with the agent as it is now (it moves on its
        // own). After the voice passed something on or stopped the agent,
        // it is read from the workspace again: what it did shows there.
        let again = acted
        acted = false
        let conversation = await parts.hub.context(fresh: again)
        guard self.turn === turn else { return }
        var messages = [ChatMessage(
            role: "system",
            content: VoicePrompt.system(language: parts.language, agent: parts.hub.name, conversation: conversation) + parts.memo)]
        messages += history
        let (sentences, feed) = AsyncStream<String>.makeStream()
        let speech = parts.speech
        turn.voice = Task { [weak self] in
            for await sentence in sentences {
                guard let self, !Task.isCancelled, self.turn === turn else { return }
                self.state = .speaking
                turn.spoken += sentence
                do {
                    try await speech.speak(sentence) { [weak self] samples, rate in
                        guard let self, self.turn === turn, !samples.isEmpty else { return }
                        // From here until the speaker says it has played it all.
                        self.speaking = true
                        let length = Duration.seconds(Double(samples.count) / 2 / Double(max(rate, 1)))
                        self.playUntil = max(self.playUntil, self.clock.now) + length
                        self.parts.audio.play(samples, rate: rate)
                    }
                } catch {
                    guard !Task.isCancelled, self.turn === turn else { return }
                    self.add(.notice, self.text("说不出声：", "Could not speak: ") + Self.words(of: error))
                }
            }
        }
        var added: [ChatMessage] = []
        var failure: String?
        rounds: for _ in 0..<5 {
            guard !Task.isCancelled, self.turn === turn else { break }
            let reply: ChatMessage
            do {
                let language = parts.language
                reply = try await parts.model.stream(messages, tools: VoiceTools.all, onText: { [weak self] delta in
                    guard let self, self.turn === turn else { return }
                    for sentence in turn.splitter.add(delta) { self.say(sentence, in: turn, to: feed) }
                }, onTool: { [weak self] name in
                    // About to act with nothing said yet: a few words
                    // first, so the person is not left in silence while
                    // the model writes the message out.
                    guard let self, self.turn === turn,
                          turn.said.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                    else { return }
                    let rest = turn.splitter.flush()
                    self.say(rest.isEmpty ? VoicePrompt.toolAck(language: language, tool: name) : rest, in: turn, to: feed)
                })
            } catch {
                if !(error is CancellationError) { failure = Self.words(of: error) }
                break rounds
            }
            guard self.turn === turn else { break }
            say(turn.splitter.flush(), in: turn, to: feed)
            messages.append(reply)
            added.append(reply)
            if reply.toolCalls.isEmpty { break }
            for call in reply.toolCalls {
                // Not cut short with the turn: a message half sent is worse
                // than one the person talked over.
                let output = await Task { [weak self] in await self?.use(call) ?? "error: the call ended" }.value
                let result = ChatMessage(role: "tool", content: output, toolCallID: call.id)
                messages.append(result)
                added.append(result)
            }
        }
        feed.finish()
        await turn.voice?.value
        guard self.turn === turn else {
            // Cut off: `interrupt` kept what was said. What was done
            // meanwhile is still part of the talk, or it is done twice.
            if let done = added.lastIndex(where: { $0.role == "tool" }) { self.history += added[...done] }
            return
        }
        self.turn = nil
        if let failure { add(.notice, text("语音没能回答：", "The voice could not answer: ") + failure) }
        let said = turn.said.trimmingCharacters(in: .whitespacesAndNewlines)
        // Nothing said (only actions): no empty line.
        if said.isEmpty { lines.removeAll { $0.id == turn.line } }
        lastSaid = said
        // A tool's result always follows its call, even when the turn ended early.
        self.history += added
        if !speaking {
            state = .listening
            dropResidue()
        }
    }

    private func say(_ sentence: String, in turn: Turn, to feed: AsyncStream<String>.Continuation) {
        guard self.turn === turn, !sentence.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        turn.said += sentence
        let said = turn.said.trimmingCharacters(in: .whitespacesAndNewlines)
        // Its first words go under what it started meanwhile ("On it: ...").
        if let index = lines.lastIndex(where: { $0.id == turn.line }), lines[index].text.isEmpty, index != lines.count - 1 {
            lines.append(lines.remove(at: index))
        }
        update(turn.line) { $0.text = said }
        feed.yield(sentence)
    }

    /// The call's own words, in the language it is held in.
    private func text(_ zh: String, _ en: String) -> String {
        parts.language == .zh ? zh : en
    }

    nonisolated static func words(of error: Error) -> String {
        if let failure = error as? VoiceFailure { return failure.message }
        return error.localizedDescription
    }

    // MARK: What the model asks to have done

    /// Does what the model asked and says how it went (for the model).
    private func use(_ call: ToolCall) async -> String {
        switch call.name {
        case VoiceTools.tell:
            acted = true
            let arguments = call.texts
            let text = (arguments["text"] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !text.isEmpty else { return "error: text is required" }
            do {
                let (task, steered) = try await parts.hub.tell(ViaVoice.body(text, heard: arguments["their_words"] ?? ""))
                var what = (arguments["what"] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                if what.isEmpty { what = VoiceContext.clip(text, 20) }
                watch(task, what: what)
                if steered {
                    add(.action, self.text("加到手上的活里：\(what)", "Added to what I'm doing: \(what)"))
                    return "added to what you are working on"
                }
                add(.action, self.text("开始做：\(what)", "On it: \(what)"))
                return "started: you are working on it now"
            } catch {
                return "error: " + Self.words(of: error)
            }
        case VoiceTools.stop:
            acted = true
            do {
                let current = parts.hub.current
                try await parts.hub.stopCurrent()
                // Its ending is no news: the person asked for it.
                if let current { watched[current, default: Watched(what: "", toldAt: clock.now)].finished = true }
                add(.action, self.text("停下了手上的活", "Stopped what I was doing"))
                return "stopped"
            } catch {
                return "error: " + Self.words(of: error)
            }
        case VoiceTools.check:
            return await parts.hub.context(fresh: true)
        default:
            return "error: unknown tool " + call.name
        }
    }

    // MARK: Watching the agent's tasks

    private func watch(_ task: String, what: String) {
        if watched[task] == nil {
            watched[task] = Watched(what: what, toldAt: clock.now)
        } else if watched[task]?.what.isEmpty == true {
            watched[task]?.what = what
        }
    }

    /// Looks at the tasks handed out: an answer or a question is news.
    private func lookAtTasks() {
        guard state != .connecting else { return }
        // A task typed in during the call is watched like one spoken.
        if let current = parts.hub.current { watch(current, what: "") }
        for (task, entry) in watched where !entry.finished {
            guard let found = parts.hub.news(of: task) else { continue }
            let what = entry.what.isEmpty ? "" : text("「" + entry.what + "」", " \"" + entry.what + "\"")
            if found.finished {
                watched[task]?.finished = true
                watched[task]?.toldAt = clock.now
                news.append(text("你做完了\(what)，你在对话里的回复：\(found.summary)", "You finished\(what); your reply in the chat: \(found.summary)"))
            } else if !found.question.isEmpty, found.question != entry.asked {
                watched[task]?.asked = found.question
                watched[task]?.toldAt = clock.now
                news.append(text("你做\(what)时有个问题要问对方：\(found.question)（你在等对方回答；对方回答了就用 do_work 记下来）",
                                 "While working on\(what) you need to ask them: \(found.question) (you are waiting for the answer; when they answer, record it with do_work)"))
            } else if found.question.isEmpty, stillWorkingDue(entry) {
                watched[task]?.toldAt = clock.now
                if !found.progress.isEmpty { watched[task]?.progress = found.progress }
                news.append(Self.stillWorking(what: what, found, told: entry.progress, now: Date(), language: parts.language))
            }
        }
    }

    /// A task not done has gone unmentioned for a while, and the call has
    /// been quiet for a good part of it (not in the middle of talk about
    /// something else).
    private func stillWorkingDue(_ entry: Watched) -> Bool {
        let every = parts.stillWorkingEvery
        return clock.now - entry.toldAt >= every && clock.now - lastSound >= every / 2 && turn == nil && !speaking
    }

    /// The notice that a task is still going: how it stands, for how
    /// long, and its newest progress note if not told yet. The desktop
    /// app's `stillWorking`.
    nonisolated static func stillWorking(what: String, _ found: TaskNews, told: String, now: Date, language: AppLanguage = .zh) -> String {
        let en = language == .en
        func say(_ zh: String, _ english: String) -> String { en ? english : zh }
        var out = say("（不是结果，只是让对方知道你还在做）你手上的", "(Not a result, only so they know you are still at it) Your work")
            + what + say("：", ": ") + VoiceContext.statusWord(found.status, language: language)
        if let since = found.since {
            let minutes = Int(now.timeIntervalSince(since) / 60)
            if minutes >= 1 { out += say("，已经 \(minutes) 分钟了", ", for \(minutes) min now") }
        }
        if !found.progress.isEmpty, found.progress != told { out += say("；你最新写的进展：", "; your latest progress note: ") + found.progress }
        return out + say("。用一句很短的话、用「我」告诉对方你还在做（可以带上在做什么），别说做完了，也不用问对方要不要等。",
                         ". In one short sentence, in the first person, tell them you are still on it (you can say what you are doing); do not say it is done, and do not ask whether they want to wait.")
    }

    // MARK: Lines

    @discardableResult
    private func add(_ kind: VoiceLine.Kind, _ text: String) -> Int {
        nextLine += 1
        lines.append(VoiceLine(id: nextLine, kind: kind, text: text))
        if lines.count > 60 { lines.removeFirst(lines.count - 40) }
        return nextLine
    }

    private func update(_ id: Int, _ change: (inout VoiceLine) -> Void) {
        guard let index = lines.lastIndex(where: { $0.id == id }) else { return }
        change(&lines[index])
    }

    /// What the person is saying: one line that changes as they go on.
    private func showHearing(_ text: String) {
        if let last = lines.last, last.kind == .hearing {
            update(last.id) { $0.text = text }
        } else {
            add(.hearing, text)
        }
    }
}
