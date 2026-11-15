import AVFoundation
import Foundation
import MycoCore

/// Says text with the phone's own voice, for a call with no Doubao key.
/// The sound is not played here: it is handed over as samples, like any
/// other voice's, so it goes out through the call's audio engine and is
/// taken out of what the microphone hears.
@MainActor
public final class SystemSpeech: NSObject, VoiceSpeech, AVSpeechSynthesizerDelegate {
    private let synthesizer = AVSpeechSynthesizer()
    private let voice: AVSpeechSynthesisVoice?
    /// The sentence being made into sound.
    private var feed: AsyncStream<Data>.Continuation?
    private var speakingID: ObjectIdentifier?

    public init(language: AppLanguage) {
        voice = Self.voice(for: language == .zh ? "zh-CN" : "en-US")
        super.init()
        synthesizer.delegate = self
    }

    /// The best voice the phone has for a language (people can download
    /// better ones in Settings › Accessibility › Spoken Content).
    static func voice(for language: String) -> AVSpeechSynthesisVoice? {
        let voices = AVSpeechSynthesisVoice.speechVoices().filter { $0.language == language }
        return voices.max { $0.quality.rawValue < $1.quality.rawValue } ?? AVSpeechSynthesisVoice(language: language)
    }

    public func speak(_ text: String, play: @escaping @MainActor @Sendable (Data, Int) -> Void) async throws {
        let text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        feed?.finish()
        let (sounds, feed) = AsyncStream<Data>.makeStream()
        self.feed = feed
        let utterance = AVSpeechUtterance(string: text)
        utterance.voice = voice
        speakingID = ObjectIdentifier(utterance)
        synthesizer.write(utterance, toBufferCallback: Self.callback(to: feed))
        await withTaskCancellationHandler {
            for await sound in sounds { play(sound, SpeechSamples.rate) }
        } onCancel: {
            feed.finish()
            Task { @MainActor in self.synthesizer.stopSpeaking(at: .immediate) }
        }
        try Task.checkCancellation()
    }

    /// What the synthesizer calls with each piece of sound, on a thread
    /// of its own: an empty piece is the end.
    private nonisolated static func callback(to feed: AsyncStream<Data>.Continuation) -> AVSpeechSynthesizer.BufferCallback {
        let samples = SpeechSamples()
        return { buffer in
            guard let sound = buffer as? AVAudioPCMBuffer, sound.frameLength > 0 else {
                feed.finish()
                return
            }
            if let data = samples.convert(sound) { feed.yield(data) }
        }
    }

    // The end of a sentence, should its empty last piece not come. Only
    // of the sentence being made: the one before may say so late.
    public nonisolated func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) {
        let id = ObjectIdentifier(utterance)
        Task { @MainActor in self.ended(id) }
    }

    public nonisolated func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didCancel utterance: AVSpeechUtterance) {
        let id = ObjectIdentifier(utterance)
        Task { @MainActor in self.ended(id) }
    }

    private func ended(_ id: ObjectIdentifier) {
        guard id == speakingID else { return }
        feed?.finish()
    }
}

/// Turns the synthesizer's sound (whatever rate and sample type its voice
/// has) into what a call plays: 16-bit mono samples, 24000 a second.
/// Used from the synthesizer's thread only, one piece after another.
final class SpeechSamples: @unchecked Sendable {
    static let rate = 24000
    private let target = AVAudioFormat(commonFormat: .pcmFormatInt16, sampleRate: Double(SpeechSamples.rate), channels: 1, interleaved: true)
    private var converter: AVAudioConverter?

    /// Whether the piece was handed to the converter yet.
    private final class Once: @unchecked Sendable {
        var given = false
    }

    func convert(_ sound: AVAudioPCMBuffer) -> Data? {
        guard let target else { return nil }
        if converter == nil || converter?.inputFormat != sound.format {
            converter = AVAudioConverter(from: sound.format, to: target)
        }
        guard let converter, sound.format.sampleRate > 0 else { return nil }
        let capacity = AVAudioFrameCount(Double(sound.frameLength) * Double(Self.rate) / sound.format.sampleRate) + 256
        guard let out = AVAudioPCMBuffer(pcmFormat: target, frameCapacity: capacity) else { return nil }
        let once = Once()
        nonisolated(unsafe) let input = sound
        var failure: NSError?
        let status = converter.convert(to: out, error: &failure) { _, state in
            if once.given {
                state.pointee = .noDataNow
                return nil
            }
            once.given = true
            state.pointee = .haveData
            return input
        }
        guard status != .error, out.frameLength > 0, let samples = out.int16ChannelData?[0] else { return nil }
        return Data(bytes: samples, count: Int(out.frameLength) * 2)
    }
}
