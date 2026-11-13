import Foundation
import Observation

/// Why speaking instead of typing did not work.
public enum DictationProblem: Error, Hashable, Sendable {
    /// The person has not allowed speech recognition.
    case speechDenied
    /// The person has not allowed the microphone.
    case microphoneDenied
    /// The device cannot recognise this language right now.
    case unavailable
    /// The microphone would not open.
    case microphone
    /// Something else, in the system's words.
    case other(String)
}

/// Something that listens and turns speech into text: Apple's speech
/// recognition in the app, a script in the demo and in tests.
@MainActor
public protocol SpeechListener: AnyObject {
    /// Starts listening in a language. The stream gives the recognizer's
    /// text each time it changes, and ends when listening does; it throws
    /// a `DictationProblem` when listening could not start.
    func start(language: AppLanguage) -> AsyncThrowingStream<String, Error>
    /// Stops the microphone. The last words still come, then the stream ends.
    func stop()
}

/// Keeps what a recognizer heard when it starts over after a pause
/// (Apple's does, on the device: after a pause its text is only the new
/// sentence): what came before stays, the new sentence goes after it. The
/// same rule as the desktop app's (`internal/console/dictation.go`).
public struct Utterances: Sendable {
    private var done = ""
    private var last = ""

    public init() {}

    /// Takes the recognizer's text now and answers all of it so far.
    public mutating func add(_ text: String) -> String {
        if !last.isEmpty, Self.startsOver(last, text) { done = Self.join(done, last) }
        last = text
        return Self.join(done, text)
    }

    /// `next` is a new sentence, not `previous` revised or carried on. A
    /// revision keeps the start (two letters at least, or all of a short
    /// beginning); a new sentence shares nothing with it.
    static func startsOver(_ previous: String, _ next: String) -> Bool {
        let p = Array(previous.unicodeScalars), n = Array(next.unicodeScalars)
        var same = 0
        while same < p.count, same < n.count, p[same] == n[same] { same += 1 }
        if same >= 2 || same == p.count || same == n.count { return false }
        // The first word or two may still change as more is heard.
        return p.count > 3
    }

    /// Puts `b` after `a`: words in a Latin script get a space between.
    static func join(_ a: String, _ b: String) -> String {
        guard let end = a.unicodeScalars.last, let start = b.unicodeScalars.first else { return a + b }
        return latin(end) && latin(start) ? a + " " + b : a + b
    }

    private static func latin(_ scalar: Unicode.Scalar) -> Bool {
        guard scalar.value < 0x2E80 else { return false }
        let properties = scalar.properties
        if properties.isAlphabetic || properties.numericType != nil { return true }
        switch properties.generalCategory {
        case .connectorPunctuation, .dashPunctuation, .openPunctuation, .closePunctuation, .initialPunctuation, .finalPunctuation, .otherPunctuation:
            return true
        default:
            return false
        }
    }
}

/// Speaking instead of typing, for one input: the microphone button's
/// state, and the words going into the box after what was typed. Typing
/// or sending while it listens stops it, and nothing more is written.
@MainActor
@Observable
public final class Dictation {
    public private(set) var listening = false
    /// Why the last try did not work.
    public private(set) var problem: DictationProblem?

    private let listener: any SpeechListener
    /// The text this listening last put in the box: any other text means
    /// the person took over.
    private var wrote: String?
    /// Counts listenings, so one that ends late does not undo the next.
    private var round = 0
    private var quiet: Task<Void, Never>?

    /// With nothing new heard for this long, the microphone is let go.
    public static let quietLimit: Duration = .seconds(30)

    public init(listener: any SpeechListener) {
        self.listener = listener
    }

    /// The microphone button: starts listening, or stops.
    /// - Parameters:
    ///   - typed: what is in the box now.
    ///   - write: puts new text in the box.
    public func toggle(typed: @escaping @MainActor () -> String, write: @escaping @MainActor (String) -> Void) {
        if listening {
            stop()
            return
        }
        problem = nil
        let language = AppLanguage.current
        let start = typed()
        let base = Self.base(after: start, language: language)
        round += 1
        let round = round
        wrote = start
        listening = true
        let stream = listener.start(language: language)
        restartQuietTimer()
        Task {
            var heard = Utterances()
            var failure: DictationProblem?
            do {
                for try await text in stream {
                    // Typed or sent since: this listening is over.
                    guard self.round == round, let wrote = self.wrote, typed() == wrote else { break }
                    let all = base + heard.add(text)
                    guard all != wrote else { continue }
                    self.wrote = all
                    write(all)
                    self.restartQuietTimer()
                }
            } catch let problem as DictationProblem {
                failure = problem
            } catch {
                failure = .other(error.localizedDescription)
            }
            guard self.round == round else { return }
            self.listener.stop()
            self.quiet?.cancel()
            self.listening = false
            self.wrote = nil
            self.problem = failure
        }
    }

    /// Stops the microphone; the last words still come.
    public func stop() {
        listener.stop()
    }

    /// The box's text changed. When it is not what was heard, the person
    /// typed or sent: stop, and write nothing more.
    public func textChanged(to text: String) {
        guard listening, let wrote, text != wrote else { return }
        self.wrote = nil
        listening = false
        quiet?.cancel()
        listener.stop()
    }

    /// What heard words are put after: what was typed, with a space when
    /// the language puts one between words.
    static func base(after typed: String, language: AppLanguage) -> String {
        guard let last = typed.last, !last.isWhitespace else { return typed }
        return language == .zh ? typed : typed + " "
    }

    private func restartQuietTimer() {
        quiet?.cancel()
        quiet = Task { [weak self] in
            try? await Task.sleep(for: Self.quietLimit)
            guard !Task.isCancelled else { return }
            self?.stop()
        }
    }
}

/// A listener that hears what it is told to: for the demo and for tests.
/// Each text is given a moment after the one before, like a recognizer
/// that revises as it goes.
@MainActor
public final class ScriptedSpeechListener: SpeechListener {
    private let script: [String]
    private let pause: Duration
    private let problem: DictationProblem?
    private var current: (id: Int, continuation: AsyncThrowingStream<String, Error>.Continuation)?
    private var count = 0
    public private(set) var stops = 0

    /// - Parameters:
    ///   - script: the recognizer's text, each time it changes.
    ///   - endsItself: ends after the last text (else it listens until stopped).
    ///   - problem: fails to start with this instead.
    public init(_ script: [String], pause: Duration = .milliseconds(250), endsItself: Bool = false, problem: DictationProblem? = nil) {
        self.script = script
        self.pause = pause
        self.problem = problem
        self.endsItself = endsItself
    }

    private let endsItself: Bool

    public func start(language: AppLanguage) -> AsyncThrowingStream<String, Error> {
        current?.continuation.finish()
        count += 1
        let id = count
        let (stream, continuation) = AsyncThrowingStream<String, Error>.makeStream()
        current = (id, continuation)
        Task {
            if let problem {
                continuation.finish(throwing: problem)
                return
            }
            for text in script {
                try? await Task.sleep(for: pause)
                guard current?.id == id else { return }
                continuation.yield(text)
            }
            if endsItself, current?.id == id {
                continuation.finish()
                current = nil
            }
        }
        return stream
    }

    public func stop() {
        stops += 1
        current?.continuation.finish()
        current = nil
    }
}
