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
        ear?.endAudio()
    }
}

/// One listening. The system calls back on threads of its own, so nothing
/// here belongs to the main actor: its state is only touched on its queue.
private final class Ear: @unchecked Sendable {
    private let queue = DispatchQueue(label: "myco.dictation")
    private let locale: Locale
    private let continuation: AsyncThrowingStream<String, Error>.Continuation
    private var recognizer: SFSpeechRecognizer?
    private var request: SFSpeechAudioBufferRecognitionRequest?
    private var task: SFSpeechRecognitionTask?
    private var engine: AVAudioEngine?
    private var heardAnything = false
    private var stopped = false
    private var ended = false

    init(locale: Locale, continuation: AsyncThrowingStream<String, Error>.Continuation) {
        self.locale = locale
        self.continuation = continuation
    }

    /// Asks for what it needs (the first time, the system asks the
    /// person), then listens.
    func begin() {
        SFSpeechRecognizer.requestAuthorization { status in
            guard status == .authorized else {
                self.finish(.speechDenied)
                return
            }
            AVAudioApplication.requestRecordPermission { granted in
                guard granted else {
                    self.finish(.microphoneDenied)
                    return
                }
                self.queue.async { self.listen() }
            }
        }
    }

    private func listen() {
        guard !ended, !stopped else {
            finish(nil)
            return
        }
        guard let recognizer = SFSpeechRecognizer(locale: locale), recognizer.isAvailable else {
            finish(.unavailable)
            return
        }
        let request = SFSpeechAudioBufferRecognitionRequest()
        request.shouldReportPartialResults = true
        request.addsPunctuation = true
        // What is said stays on the phone when the language can be heard there.
        if recognizer.supportsOnDeviceRecognition { request.requiresOnDeviceRecognition = true }
        let engine = AVAudioEngine()
        do {
            let session = AVAudioSession.sharedInstance()
            try session.setCategory(.record, mode: .measurement, options: .duckOthers)
            try session.setActive(true, options: .notifyOthersOnDeactivation)
            let input = engine.inputNode
            let format = input.outputFormat(forBus: 0)
            // No microphone to hear with (a simulator without one): a tap
            // on such an input does not fail, it crashes.
            guard format.sampleRate > 0, format.channelCount > 0 else {
                finish(.microphone)
                return
            }
            nonisolated(unsafe) let hearing = request
            input.installTap(onBus: 0, bufferSize: 1024, format: format) { buffer, _ in
                hearing.append(buffer)
            }
            engine.prepare()
            try engine.start()
        } catch {
            engine.inputNode.removeTap(onBus: 0)
            finish(.microphone)
            return
        }
        self.recognizer = recognizer
        self.request = request
        self.engine = engine
        task = recognizer.recognitionTask(with: request) { result, error in
            let text = result?.bestTranscription.formattedString
            let final = result?.isFinal ?? false
            let failure = error.map { ($0 as NSError).code == Ear.noSpeech ? "" : $0.localizedDescription }
            self.queue.async {
                if let text {
                    self.heardAnything = self.heardAnything || !text.isEmpty
                    self.continuation.yield(text)
                }
                guard final || failure != nil else { return }
                // An error with nothing heard, before anyone stopped it,
                // is worth saying; one at the end of a listening is not.
                if let failure, !failure.isEmpty, !final, !self.heardAnything, !self.stopped {
                    self.end(.other(failure))
                } else {
                    self.end(nil)
                }
            }
        }
    }

    /// "No speech detected": the person said nothing, which is no error.
    private static let noSpeech = 1110

    /// Stops the microphone; the recognizer then gives its last words.
    func endAudio() {
        queue.async {
            guard !self.ended, !self.stopped else { return }
            self.stopped = true
            self.stopEngine()
            // Not listening yet (the person is still being asked): `listen`
            // sees it was stopped and ends.
            guard let request = self.request, self.task != nil else { return }
            request.endAudio()
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

    private func finish(_ problem: DictationProblem?) {
        queue.async { self.end(problem) }
    }

    private func stopEngine() {
        guard let engine else { return }
        engine.stop()
        engine.inputNode.removeTap(onBus: 0)
        self.engine = nil
    }

    private func end(_ problem: DictationProblem?) {
        guard !ended else { return }
        ended = true
        stopEngine()
        task = nil
        request = nil
        recognizer = nil
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        if let problem {
            continuation.finish(throwing: problem)
        } else {
            continuation.finish()
        }
    }
}
