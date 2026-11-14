import AVFoundation
import Foundation
import Speech
import MycoCore

/// Listens with Apple's speech recognition, on the device when the
/// language allows, in the language chosen for the app. The system asks
/// for the microphone and for speech recognition the first time.
@MainActor
public final class AppleSpeechListener: SpeechListener {
    private var ear: Ear?

    public init() {}

    public func start(language: AppLanguage) -> AsyncThrowingStream<String, Error> {
        ear?.cancel()
        let (stream, continuation) = AsyncThrowingStream<String, Error>.makeStream()
        let ear = Ear(locale: Locale(identifier: language == .zh ? "zh-CN" : "en-US"), continuation: continuation)
        self.ear = ear
        // Nobody reads it any more: let go of the microphone.
        continuation.onTermination = { _ in ear.cancel() }
        ear.begin()
        return stream
    }

    public func stop() {
        ear?.stop()
    }
}

/// One listening: the microphone runs from `begin` to `stop`, and the
/// recognizer hears it in sessions. The recognizer ends a session when the
/// person pauses (on the device it finalises the sentence), so while they
/// have not stopped, the next one starts at once on the running
/// microphone: a pause does not end the listening. What was said stays,
/// and the next words go after it. (The desktop app's rule,
/// cmd/myco-app/dictation_darwin.go.)
///
/// The system calls back on threads of its own, so nothing here belongs
/// to the main actor: its state is only touched on its queue, and the
/// request the audio thread feeds is behind a lock.
private final class Ear: @unchecked Sendable {
    private let queue = DispatchQueue(label: "myco.dictation")
    private let locale: Locale
    private let continuation: AsyncThrowingStream<String, Error>.Continuation
    private var recognizer: SFSpeechRecognizer?
    private var task: SFSpeechRecognitionTask?
    private var engine: AVAudioEngine?
    /// The request of the session being heard; the audio thread appends to it.
    private let feeding = NSLock()
    private var request: SFSpeechAudioBufferRecognitionRequest?
    /// Sessions are numbered: a late answer from an old one is dropped.
    private var session = 0
    /// Everything heard, session after session.
    private var heard = Utterances()
    /// The person tapped to stop: the session's end is the listening's end.
    private var stopping = false
    /// Sessions in a row that heard nothing: after a few, it stops.
    private var silent = 0
    /// Whether anything was heard at all, and why the last session that
    /// heard nothing failed (when it was not for silence).
    private var everHeard = false
    private var lastFailure: String?
    private var ended = false

    /// This many silent sessions in a row end the listening.
    private static let silentSessions = 6
    /// "No speech detected": the person said nothing, which is no error.
    private static let noSpeech = 1110

    init(locale: Locale, continuation: AsyncThrowingStream<String, Error>.Continuation) {
        self.locale = locale
        self.continuation = continuation
    }

    /// Asks for what it needs (the first time, the system asks the
    /// person), then listens.
    func begin() {
        SFSpeechRecognizer.requestAuthorization { status in
            guard status == .authorized else {
                self.queue.async { self.end(.speechDenied) }
                return
            }
            AVAudioApplication.requestRecordPermission { granted in
                self.queue.async {
                    if granted {
                        self.openMicrophone()
                    } else {
                        self.end(.microphoneDenied)
                    }
                }
            }
        }
    }

    /// Stops the microphone; the recognizer then gives its last words.
    func stop() {
        queue.async {
            guard !self.ended, !self.stopping else { return }
            self.stopping = true
            // Not listening yet (the person is still being asked): it ends
            // when the answer comes.
            guard self.engine != nil else { return }
            self.closeMicrophone()
            self.endAudio()
            // The last words come within a moment; if they do not, it ends anyway.
            self.queue.asyncAfter(deadline: .now() + 2) { self.end(nil) }
        }
    }

    /// Lets go of everything at once: nothing more is wanted.
    func cancel() {
        queue.async {
            self.task?.cancel()
            self.end(nil)
        }
    }

    private func openMicrophone() {
        guard !ended, !stopping else {
            end(nil)
            return
        }
        guard let recognizer = SFSpeechRecognizer(locale: locale), recognizer.isAvailable else {
            end(.unavailable)
            return
        }
        let engine = AVAudioEngine()
        do {
            let audio = AVAudioSession.sharedInstance()
            try audio.setCategory(.record, mode: .measurement, options: .duckOthers)
            try audio.setActive(true, options: .notifyOthersOnDeactivation)
            let input = engine.inputNode
            let format = input.outputFormat(forBus: 0)
            // No microphone to hear with (a simulator without one): a tap
            // on such an input does not fail, it crashes.
            guard format.sampleRate > 0, format.channelCount > 0 else {
                end(.microphone)
                return
            }
            input.installTap(onBus: 0, bufferSize: 1024, format: format) { buffer, _ in
                self.feeding.lock()
                self.request?.append(buffer)
                self.feeding.unlock()
            }
            engine.prepare()
            try engine.start()
        } catch {
            engine.inputNode.removeTap(onBus: 0)
            end(.microphone)
            return
        }
        self.recognizer = recognizer
        self.engine = engine
        hear()
    }

    /// One recognition session on the running microphone.
    private func hear() {
        guard let recognizer else { return }
        let request = SFSpeechAudioBufferRecognitionRequest()
        request.shouldReportPartialResults = true
        request.addsPunctuation = true
        // What is said stays on the phone when the language can be heard there.
        if recognizer.supportsOnDeviceRecognition { request.requiresOnDeviceRecognition = true }
        feeding.lock()
        self.request?.endAudio()
        self.request = request
        feeding.unlock()
        session += 1
        let session = session
        task = recognizer.recognitionTask(with: request) { result, error in
            let text = result?.bestTranscription.formattedString ?? ""
            let final = result?.isFinal ?? false
            let failure = error.map { ($0 as NSError).code == Ear.noSpeech ? "" : $0.localizedDescription }
            self.queue.async { self.heard(text, final: final, failure: failure, in: session) }
        }
    }

    private func heard(_ text: String, final: Bool, failure: String?, in session: Int) {
        guard !ended, session == self.session else { return }
        if !text.isEmpty {
            everHeard = true
            continuation.yield(heard.add(text))
        }
        guard final || failure != nil else { return }
        // The session is over. What it said stays; the next goes after it.
        let any = !text.isEmpty
        heard.commit()
        if let failure, !failure.isEmpty, !any { lastFailure = failure }
        if engine != nil, !stopping {
            silent = any ? 0 : silent + 1
            if silent < Self.silentSessions {
                hear()
                return
            }
        }
        // It never heard a word and the recognizer kept failing: worth
        // saying why. An error at the end of a listening is not.
        if !stopping, !everHeard, let lastFailure {
            end(.other(lastFailure))
        } else {
            end(nil)
        }
    }

    private func endAudio() {
        feeding.lock()
        request?.endAudio()
        feeding.unlock()
    }

    private func closeMicrophone() {
        guard let engine else { return }
        engine.stop()
        engine.inputNode.removeTap(onBus: 0)
        self.engine = nil
    }

    private func end(_ problem: DictationProblem?) {
        guard !ended else { return }
        ended = true
        closeMicrophone()
        feeding.lock()
        request?.endAudio()
        request = nil
        feeding.unlock()
        task = nil
        recognizer = nil
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        if let problem {
            continuation.finish(throwing: problem)
        } else {
            continuation.finish()
        }
    }
}
