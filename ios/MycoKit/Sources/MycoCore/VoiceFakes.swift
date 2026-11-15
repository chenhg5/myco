import Foundation

// A voice call with nobody speaking and no keys: for the demo and for
// tests. The microphone hears a script, the model answers from one, and
// the voice makes no sound.

/// A microphone that hears what it is told to, and a speaker that only
/// counts what it is given.
@MainActor
public final class ScriptedVoiceAudio: VoiceAudio {
    /// Something the person says: after a pause, word by word.
    public struct Line: Sendable {
        public var after: Duration
        public var partials: [String]
        /// The recognizer settles it (else the person just goes quiet).
        public var settles: Bool

        public init(after: Duration, _ partials: [String], settles: Bool = false) {
            self.after = after
            self.partials = partials
            self.settles = settles
        }
    }

    private let script: [Line]
    private let pace: Duration
    private let problem: DictationProblem?
    private var continuation: AsyncThrowingStream<VoiceAudioEvent, Error>.Continuation?
    private var draining: Task<Void, Never>?
    private var round = 0

    /// How much sound was queued, how often it was cut off, how often the
    /// recognizer was told to start over, and whether it was closed.
    public private(set) var queued = 0
    public private(set) var flushes = 0
    public private(set) var cuts = 0
    public private(set) var stopped = false

    /// - Parameters:
    ///   - script: what is heard, in order, once the microphone is open.
    ///   - pace: the time between two words of a line.
    ///   - problem: the microphone does not open, for this reason.
    public init(_ script: [Line] = [], pace: Duration = .milliseconds(220), problem: DictationProblem? = nil) {
        self.script = script
        self.pace = pace
        self.problem = problem
    }

    public func start(language: AppLanguage) -> AsyncThrowingStream<VoiceAudioEvent, Error> {
        let (stream, continuation) = AsyncThrowingStream<VoiceAudioEvent, Error>.makeStream()
        self.continuation = continuation
        round += 1
        let round = round
        Task {
            try? await Task.sleep(for: .milliseconds(30))
            if let problem {
                continuation.finish(throwing: problem)
                return
            }
            continuation.yield(.ready)
            for line in script {
                try? await Task.sleep(for: line.after)
                for (index, text) in line.partials.enumerated() {
                    guard self.round == round, !stopped else { return }
                    continuation.yield(.heard(text, final: line.settles && index == line.partials.count - 1))
                    try? await Task.sleep(for: pace)
                }
            }
        }
        return stream
    }

    /// Hears something now (tests).
    public func hear(_ text: String, final: Bool = false) {
        continuation?.yield(.heard(text, final: final))
    }

    public func play(_ samples: Data, rate: Int) {
        queued += samples.count
        // "Played" a moment after the last of it was queued.
        draining?.cancel()
        draining = Task {
            try? await Task.sleep(for: .milliseconds(120))
            guard !Task.isCancelled else { return }
            continuation?.yield(.played)
        }
    }

    public func flush() {
        flushes += 1
        draining?.cancel()
    }

    public func newUtterance() {
        cuts += 1
    }

    public func stop() {
        stopped = true
        draining?.cancel()
        continuation?.finish()
        continuation = nil
    }
}

/// A model that answers from a script: each time it is asked, the next
/// answer, its text a few characters at a time.
public actor ScriptedVoiceModel: VoiceModel {
    public struct Answer: Sendable {
        public var text: String
        public var calls: [ToolCall]

        public init(_ text: String = "", calls: [ToolCall] = []) {
            self.text = text
            self.calls = calls
        }
    }

    private let answer: @Sendable ([ChatMessage]) -> Answer
    private let pace: Duration
    /// What it was asked, each time.
    public private(set) var asked: [[ChatMessage]] = []

    /// - Parameter answer: what to say to a talk so far.
    public init(pace: Duration = .milliseconds(25), answer: @escaping @Sendable ([ChatMessage]) -> Answer) {
        self.answer = answer
        self.pace = pace
    }

    /// The answers in order; after the last, nothing more is said.
    public init(_ answers: [Answer], pace: Duration = .zero) {
        let left = Remaining(answers)
        self.answer = { _ in left.next() }
        self.pace = pace
    }

    private final class Remaining: @unchecked Sendable {
        private let lock = NSLock()
        private var answers: [Answer]

        init(_ answers: [Answer]) {
            self.answers = answers
        }

        func next() -> Answer {
            lock.lock()
            defer { lock.unlock() }
            return answers.isEmpty ? Answer() : answers.removeFirst()
        }
    }

    public func stream(
        _ messages: [ChatMessage], tools: [VoiceTool], onText: @escaping @MainActor @Sendable (String) -> Void
    ) async throws -> ChatMessage {
        asked.append(messages)
        let reply = self.answer(messages)
        var rest = Substring(reply.text)
        while !rest.isEmpty {
            let piece = rest.prefix(3)
            rest = rest.dropFirst(piece.count)
            if pace > .zero { try await Task.sleep(for: pace) }
            try Task.checkCancellation()
            await onText(String(piece))
        }
        return ChatMessage(role: "assistant", content: reply.text, toolCalls: reply.calls)
    }
}

/// A voice that makes no sound: each sentence takes as long as saying it
/// would, and gives silence to play.
public struct SilentSpeech: VoiceSpeech {
    /// How long a character takes to say.
    public var perCharacter: Duration
    /// Fails instead, with these words.
    public var failure: String?

    public init(perCharacter: Duration = .milliseconds(70), failure: String? = nil) {
        self.perCharacter = perCharacter
        self.failure = failure
    }

    public func speak(_ text: String, play: @escaping @MainActor @Sendable (Data, Int) -> Void) async throws {
        if let failure { throw VoiceFailure(failure) }
        await play(Data(count: 480), 24000)
        try await Task.sleep(for: perCharacter * text.count)
    }
}

extension ScriptedVoiceModel {
    /// The demo's model: it answers a question about progress itself and
    /// passes a request on, as the real one is told to.
    public static func demo() -> ScriptedVoiceModel {
        ScriptedVoiceModel { messages in
            let last = messages.last
            if last?.role == "tool" {
                return Answer(L("好，已经转给它了。", "Done, I passed that on."))
            }
            if last?.role == "system" {
                return Answer(L("它那边有新消息了，详细的放在对话里了。", "There is news from it; the details are in the chat."))
            }
            let said = last?.content ?? ""
            if said.contains("到哪") || said.lowercased().contains("how far") {
                return Answer(L("它还在做，刚把下载页的版式改完，正在核对链接。", "Still at it: the download page's layout is done, and it is checking the links."))
            }
            let arguments = ["text": said, "what": String(said.prefix(12)), "their_words": said]
            let json = (try? JSONSerialization.data(withJSONObject: arguments)).flatMap { String(data: $0, encoding: .utf8) } ?? "{}"
            return Answer(calls: [ToolCall(id: "call_demo", name: VoiceTools.tell, arguments: json)])
        }
    }
}

extension ScriptedVoiceAudio {
    /// The demo's microphone: a question about progress, then a request.
    public static func demo() -> ScriptedVoiceAudio {
        ScriptedVoiceAudio([
            Line(after: .milliseconds(900), ["做到", "做到哪了"]),
            Line(after: .seconds(5), ["顺便", "顺便把标题", "顺便把标题也改短一点"]),
        ])
    }
}
