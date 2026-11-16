import AVFoundation
import Foundation
import os
import Speech
import MycoCore

/// A voice call's microphone and speaker on the phone. One audio engine
/// does both, with the system's voice processing on (echo cancellation,
/// noise suppression, automatic gain): what the voice says is played
/// through the same engine, so it is taken out of what the microphone
/// hears, and the person can cut in while it talks. What is heard goes to
/// Apple's speech recognition (on the phone when the language allows),
/// one session after another for as long as the call is on. The desktop
/// app's, cmd/myco-app/voice_darwin.go.
@MainActor
public final class AppleVoiceAudio: VoiceAudio {
    private var line: CallLine?

    public init() {}

    public func start(language: AppLanguage) -> AsyncThrowingStream<VoiceAudioEvent, Error> {
        line?.close(nil)
        let (stream, continuation) = AsyncThrowingStream<VoiceAudioEvent, Error>.makeStream()
        let line = CallLine(locale: Locale(identifier: language == .zh ? "zh-CN" : "en-US"), continuation: continuation)
        self.line = line
        // Nobody listens any more: let go of the microphone.
        continuation.onTermination = { _ in line.close(nil) }
        line.open()
        return stream
    }

    public func play(_ samples: Data, rate: Int) {
        line?.play(samples, rate: rate)
    }

    public func flush() {
        line?.flush()
    }

    public func newUtterance() {
        line?.startOver()
    }

    public func stop() {
        line?.close(nil)
        line = nil
    }
}

/// One call's audio. The system calls back on threads of its own, so
/// nothing here belongs to the main actor: its state is only touched on
/// its queue, and the request the audio thread feeds is behind a lock.
private final class CallLine: @unchecked Sendable {
    private let queue = DispatchQueue(label: "myco.voice")
    private let locale: Locale
    private let continuation: AsyncThrowingStream<VoiceAudioEvent, Error>.Continuation

    private var engine: AVAudioEngine?
    private var player: AVAudioPlayerNode?
    private var recognizer: SFSpeechRecognizer?
    private var task: SFSpeechRecognitionTask?
    /// The request of the session being heard; the audio thread appends to it.
    private let feeding = NSLock()
    private var request: SFSpeechAudioBufferRecognitionRequest?
    /// What the recognizer gets: the microphone's first channel alone.
    private var heardFormat: AVAudioFormat?
    /// Sessions are numbered: a late answer from an old one is dropped.
    private var session = 0
    private var sessionBegan = Date()
    /// What this session heard: the recognizer may start its text over
    /// after a short pause, and the words before it stay.
    private var words = Utterances()
    /// Sessions in a row that failed at once: after a few, the call ends.
    private var failures = 0
    /// Sound queued and not yet played, and which flush it belongs to (a
    /// flush makes the older ones not count).
    private var pending = 0
    private var generation = 0
    private var observers: [NSObjectProtocol] = []
    private var closed = false
    /// The system's voice processing is on (echo cancellation); off when
    /// this phone would not start the engine with it.
    private var processing = true
    /// When the engine was last built again, and how often in a row: a
    /// system that keeps changing its mind ends the call, not the app.
    private var rebuilt: [Date] = []

    /// What the voice is played at; anything else is left out.
    private static let playRate = 24000.0
    private let playFormat = AVAudioFormat(commonFormat: .pcmFormatFloat32, sampleRate: CallLine.playRate, channels: 1, interleaved: false)
    /// "No speech detected": the person said nothing, which is no error.
    private static let noSpeech = 1110
    /// This many sessions failing at once, in a row, end the call.
    private static let failuresAllowed = 6
    /// What the audio did, step by step, in the system's log (Console,
    /// category "voice"): never what was said.
    private static let log = Logger(subsystem: "dev.myco.ios", category: "voice")

    private func note(_ text: String) {
        Self.log.notice("\(text, privacy: .public)")
    }

    private static func said(_ error: Error) -> String {
        let error = error as NSError
        return "\(error.domain) \(error.code)"
    }

    init(locale: Locale, continuation: AsyncThrowingStream<VoiceAudioEvent, Error>.Continuation) {
        self.locale = locale
        self.continuation = continuation
    }

    /// Asks for what it needs (the first time, the system asks the
    /// person), then opens the microphone and the speaker.
    func open() {
        SFSpeechRecognizer.requestAuthorization { status in
            guard status == .authorized else {
                self.queue.async { self.end(.speechDenied) }
                return
            }
            AVAudioApplication.requestRecordPermission { granted in
                self.queue.async {
                    if granted {
                        self.start()
                    } else {
                        self.end(.microphoneDenied)
                    }
                }
            }
        }
    }

    func close(_ problem: DictationProblem?) {
        queue.async { self.end(problem) }
    }

    private func start() {
        guard !closed else { return }
        guard let recognizer = SFSpeechRecognizer(locale: locale), recognizer.isAvailable else {
            note("no recognizer for \(locale.identifier)")
            end(.unavailable)
            return
        }
        self.recognizer = recognizer
        do {
            let audio = AVAudioSession.sharedInstance()
            // Talking and listening at once, out of the loudspeaker unless
            // headphones are on.
            try audio.setCategory(.playAndRecord, mode: .voiceChat, options: [.defaultToSpeaker, .allowBluetooth])
            try audio.setActive(true, options: .notifyOthersOnDeactivation)
            note("session on: \(audio.sampleRate) Hz, in \(audio.currentRoute.inputs.map(\.portType.rawValue)), out \(audio.currentRoute.outputs.map(\.portType.rawValue))")
        } catch {
            note("session failed: \(Self.said(error))")
            end(Self.wouldNotOpen(L("音频会话", "audio session"), error))
            return
        }
        do {
            try build()
        } catch {
            // Some phones and routes will not start the engine with the
            // system's voice processing: without it there is still a call
            // (what the voice says is told apart by its words).
            note("engine failed with voice processing: \(Self.said(error))")
            processing = false
            do {
                try build()
            } catch {
                note("engine failed: \(Self.said(error))")
                end(Self.wouldNotOpen(L("音频引擎", "audio engine"), error))
                return
            }
        }
        watchTheSystem()
        continuation.yield(.ready)
        hear()
    }

    /// "The microphone would not open", with what refused and the
    /// system's number for it: something to tell whoever can fix it.
    private static func wouldNotOpen(_ what: String, _ error: Error) -> DictationProblem {
        if let problem = error as? DictationProblem, problem != .microphone { return problem }
        let detail = (error as? DictationProblem) == .microphone ? L("没有可用的输入", "no input to hear with") : said(error)
        return .other(L("麦克风打不开（", "Could not open the microphone (") + what + L("：", ": ") + detail + L("）", ")"))
    }

    /// The engine: the microphone tapped for the recognizer, and a player
    /// for the voice. One engine for the whole call: when the system stops
    /// it, the same one is wired and started again (`restart`).
    private func build() throws {
        let engine = AVAudioEngine()
        // Echo cancellation and noise suppression: before the formats are read.
        if processing {
            do {
                try engine.inputNode.setVoiceProcessingEnabled(true)
            } catch {
                note("voice processing refused: \(Self.said(error))")
                processing = false
            }
        }
        let player = AVAudioPlayerNode()
        engine.attach(player)
        self.engine = engine
        self.player = player
        do {
            try wire()
        } catch {
            engine.stop()
            self.engine = nil
            self.player = nil
            throw error
        }
    }

    /// Taps the microphone as it is now, connects the player and starts
    /// the engine: when it is new, and again each time the system stopped it.
    private func wire() throws {
        guard let engine, let player, let playFormat else { throw DictationProblem.microphone }
        let input = engine.inputNode
        input.removeTap(onBus: 0)
        let format = input.outputFormat(forBus: 0)
        // No microphone to hear with (a simulator without one, or a phone
        // just after its audio changed): a tap on such an input does not
        // fail, it crashes.
        guard format.sampleRate > 0, format.channelCount > 0 else {
            note("no input: \(format.sampleRate) Hz, \(format.channelCount) channels")
            throw DictationProblem.microphone
        }
        // With voice processing on, the microphone may give several
        // channels (the processed voice first); the recognizer hears
        // nothing in that, so it gets the first channel alone.
        heardFormat = AVAudioFormat(commonFormat: .pcmFormatFloat32, sampleRate: format.sampleRate, channels: 1, interleaved: false)
        input.installTap(onBus: 0, bufferSize: 1024, format: format) { buffer, _ in
            self.feed(buffer)
        }
        engine.connect(player, to: engine.mainMixerNode, format: playFormat)
        engine.prepare()
        do {
            try engine.start()
        } catch {
            input.removeTap(onBus: 0)
            throw error
        }
        player.play()
        let audio = AVAudioSession.sharedInstance()
        note("engine on: input \(format.sampleRate) Hz, \(format.channelCount) channels, voice processing \(processing ? "on" : "off"), session \(audio.sampleRate) Hz, out \(audio.currentRoute.outputs.map(\.portType.rawValue))")
    }

    private func feed(_ buffer: AVAudioPCMBuffer) {
        feeding.lock()
        defer { feeding.unlock() }
        guard let request else { return }
        if buffer.format.channelCount == 1, buffer.format.commonFormat == .pcmFormatFloat32 {
            request.append(buffer)
            return
        }
        guard let heardFormat, let source = buffer.floatChannelData,
              let mono = AVAudioPCMBuffer(pcmFormat: heardFormat, frameCapacity: buffer.frameLength), let target = mono.floatChannelData
        else { return }
        mono.frameLength = buffer.frameLength
        let stride = buffer.format.isInterleaved ? Int(buffer.format.channelCount) : 1
        for frame in 0..<Int(buffer.frameLength) { target[0][frame] = source[0][frame * stride] }
        request.append(mono)
    }

    /// A phone call or another app takes the audio: the call ends. The
    /// system stops the engine (headphones on or off, another sample
    /// rate): it is built again.
    private func watchTheSystem() {
        let center = NotificationCenter.default
        observers.append(center.addObserver(forName: AVAudioSession.interruptionNotification, object: nil, queue: nil) { note in
            let began = (note.userInfo?[AVAudioSessionInterruptionTypeKey] as? UInt) == AVAudioSession.InterruptionType.began.rawValue
            let reason = note.userInfo?[AVAudioSessionInterruptionReasonKey] as? UInt
            self.queue.async {
                self.note("interruption: began \(began), reason \(reason.map(String.init) ?? "none")")
                guard began else { return }
                self.end(.other(L("别的通话或 App 用了麦克风，语音聊天结束了", "A call or another app took the microphone: the voice chat ended")))
            }
        })
        observers.append(center.addObserver(forName: AVAudioSession.mediaServicesWereResetNotification, object: nil, queue: nil) { _ in
            self.queue.async {
                self.note("media services were reset")
                self.end(.other(L("系统的音频服务重启了，语音聊天结束了", "The system's audio restarted: the voice chat ended")))
            }
        })
        observers.append(center.addObserver(forName: .AVAudioEngineConfigurationChange, object: nil, queue: nil) { note in
            let changed = note.object as AnyObject?
            let which = changed.map(ObjectIdentifier.init)
            self.queue.async {
                guard !self.closed, let engine = self.engine, which == ObjectIdentifier(engine) else { return }
                self.note("configuration change: engine running \(engine.isRunning)")
                // Only an engine the system has stopped needs starting
                // again; a notice for one that runs is an old one.
                guard !engine.isRunning else { return }
                self.restart()
            }
        })
    }

    /// This many restarts within a few seconds: the system will not settle.
    private static let restartsAllowed = 4

    /// The system stopped the engine: its audio changed (headphones on or
    /// off, another sample rate), and on a phone always once just after a
    /// call starts, when voice processing settles in. The same engine is
    /// wired and started again. A new engine instead would begin without
    /// voice processing and turn it on, which is such a change itself:
    /// the phone would stop engine after engine and never settle.
    private func restart(attempt: Int = 0) {
        guard !closed, engine != nil else { return }
        // What was queued went when the engine stopped.
        generation += 1
        if pending > 0 {
            pending = 0
            continuation.yield(.played)
        }
        if attempt == 0 {
            let now = Date()
            rebuilt = rebuilt.filter { now.timeIntervalSince($0) < 5 } + [now]
        }
        guard rebuilt.count <= Self.restartsAllowed else {
            note("restarted \(rebuilt.count) times in a few seconds")
            startWithoutProcessing(.other(L("音频设备一直在变，语音聊天结束了", "The audio route keeps changing: the voice chat ended")))
            return
        }
        do {
            try wire()
        } catch {
            note("restart \(attempt) failed: \(Self.said(error))")
            // Just after a change the microphone may have no format yet: a
            // few more tries, a little later each time.
            guard attempt < 5 else {
                startWithoutProcessing(Self.wouldNotOpen(L("音频引擎", "audio engine"), error))
                return
            }
            queue.asyncAfter(deadline: .now() + 0.2 * Double(attempt + 1)) {
                guard !self.closed, self.engine?.isRunning == false else { return }
                self.restart(attempt: attempt + 1)
            }
            return
        }
        // The microphone may give another format now: a new session hears it.
        hear()
    }

    /// The engine will not stay up with the system's voice processing: a
    /// new one without it, once (what the voice says is then told apart
    /// by its words). If that was tried already, the call ends.
    private func startWithoutProcessing(_ otherwise: DictationProblem) {
        engine?.inputNode.removeTap(onBus: 0)
        engine?.stop()
        engine = nil
        player = nil
        guard processing else {
            end(otherwise)
            return
        }
        note("going on without voice processing")
        processing = false
        rebuilt = []
        do {
            try build()
        } catch {
            note("engine failed: \(Self.said(error))")
            end(Self.wouldNotOpen(L("音频引擎", "audio engine"), error))
            return
        }
        hear()
    }

    // MARK: Hearing

    /// One recognition session on the running microphone; the next starts
    /// when it ends (a pause), while the call is on.
    private func hear() {
        guard !closed, let recognizer else { return }
        let request = SFSpeechAudioBufferRecognitionRequest()
        request.shouldReportPartialResults = true
        request.addsPunctuation = true
        // What is said stays on the phone when the language can be heard there.
        if recognizer.supportsOnDeviceRecognition { request.requiresOnDeviceRecognition = true }
        feeding.lock()
        self.request?.endAudio()
        self.request = request
        feeding.unlock()
        task?.cancel()
        session += 1
        sessionBegan = Date()
        words = Utterances()
        let session = session
        task = recognizer.recognitionTask(with: request) { result, error in
            let text = result?.bestTranscription.formattedString ?? ""
            let final = result?.isFinal ?? false
            let failure = error.map { ($0 as NSError).code == CallLine.noSpeech ? "" : $0.localizedDescription }
            self.queue.async { self.heard(text, final: final, failure: failure, in: session) }
        }
    }

    private func heard(_ text: String, final: Bool, failure: String?, in session: Int) {
        guard !closed, session == self.session else { return }
        let over = final || failure != nil
        if !text.isEmpty {
            failures = 0
            continuation.yield(.heard(words.add(text), final: over))
        }
        guard over else { return }
        // A session that fails as soon as it starts, again and again: the
        // recognizer is not working, and asking faster does not help.
        if let failure, !failure.isEmpty, text.isEmpty, Date().timeIntervalSince(sessionBegan) < 1 {
            failures += 1
            guard failures < Self.failuresAllowed else {
                note("recognition keeps failing: \(failure)")
                end(.other(failure))
                return
            }
            queue.asyncAfter(deadline: .now() + 0.6) {
                guard session == self.session else { return }
                self.hear()
            }
            return
        }
        hear()
    }

    /// What was heard has been taken: the next words are a new utterance.
    func startOver() {
        queue.async {
            guard !self.closed, self.engine?.isRunning == true else { return }
            self.hear()
        }
    }

    // MARK: Speaking

    func play(_ samples: Data, rate: Int) {
        let count = samples.count / 2
        guard count > 0, Double(rate) == Self.playRate else { return }
        queue.async {
            guard !self.closed, let player = self.player, let format = self.playFormat,
                  let buffer = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: AVAudioFrameCount(count)),
                  let out = buffer.floatChannelData?[0]
            else { return }
            // Stopped by the system and not yet started again: this piece
            // is not heard (a player may not play on a stopped engine).
            guard self.engine?.isRunning == true else {
                if self.pending == 0 { self.continuation.yield(.played) }
                return
            }
            buffer.frameLength = AVAudioFrameCount(count)
            samples.withUnsafeBytes { raw in
                for index in 0..<count {
                    let sample = Int16(littleEndian: raw.loadUnaligned(fromByteOffset: index * 2, as: Int16.self))
                    out[index] = Float(sample) / 32768
                }
            }
            let generation = self.generation
            self.pending += 1
            player.scheduleBuffer(buffer, at: nil, options: [], completionCallbackType: .dataPlayedBack) { _ in
                self.queue.async {
                    guard !self.closed, generation == self.generation else { return }
                    self.pending -= 1
                    if self.pending == 0 { self.continuation.yield(.played) }
                }
            }
            if !player.isPlaying { player.play() }
        }
    }

    /// Stops what is playing and drops what is queued (the person cut in).
    func flush() {
        queue.async {
            self.generation += 1
            self.pending = 0
            guard let player = self.player, self.engine?.isRunning == true else { return }
            player.stop()
            player.play()
        }
    }

    private func end(_ problem: DictationProblem?) {
        guard !closed else { return }
        closed = true
        note("ended\(problem.map { ": \($0)" } ?? "")")
        for observer in observers { NotificationCenter.default.removeObserver(observer) }
        observers = []
        generation += 1
        pending = 0
        player?.stop()
        engine?.inputNode.removeTap(onBus: 0)
        engine?.stop()
        engine = nil
        player = nil
        feeding.lock()
        request?.endAudio()
        request = nil
        feeding.unlock()
        task?.cancel()
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

/// `-myco-voice probe`: opens a call's microphone and speaker alone for a
/// few seconds, plays a short tone, and says in the system's log
/// (category "voice") what happened and never what was said: to see on a
/// real phone why a call would not start.
@MainActor
public enum VoiceProbe {
    public static func run(seconds: Double = 8) async {
        let log = Logger(subsystem: "dev.myco.ios", category: "voice")
        let audio = AppleVoiceAudio()
        log.notice("probe: start")
        let listening = Task { @MainActor in
            do {
                for try await event in audio.start(language: .zh) {
                    switch event {
                    case .ready:
                        log.notice("probe: ready")
                        audio.play(tone(), rate: 24000)
                    case .heard(let text, let final):
                        log.notice("probe: heard \(text.count, privacy: .public) characters, final \(final, privacy: .public)")
                    case .played:
                        log.notice("probe: played")
                    }
                }
                log.notice("probe: ended")
            } catch {
                log.notice("probe: failed: \(String(describing: error), privacy: .public)")
            }
        }
        try? await Task.sleep(for: .seconds(seconds))
        audio.stop()
        await listening.value
        log.notice("probe: done")
    }

    /// Half a second of a quiet 440 Hz tone, as the voice's sound comes.
    private static func tone() -> Data {
        var data = Data()
        for index in 0..<12000 {
            let sample = Int16(sin(Double(index) * 2 * .pi * 440 / 24000) * 6000)
            withUnsafeBytes(of: sample.littleEndian) { data.append(contentsOf: $0) }
        }
        return data
    }
}
