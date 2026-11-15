import Foundation

/// A voice call's "speaking": says text. Sound goes to `play` as it
/// arrives: 16-bit little-endian mono samples and how many a second.
public protocol VoiceSpeech: Sendable {
    func speak(_ text: String, play: @escaping @MainActor @Sendable (Data, Int) -> Void) async throws
}

/// Doubao (Volcengine) speech synthesis over its streaming HTTP API, with
/// the key from Settings › 语音 (an API key from the Volcengine console).
/// A cloned voice (S_...) uses the seed-icl-2.0 resource, the others
/// seed-tts-2.0. The desktop app's, internal/console/voice_tts.go.
public struct DoubaoSpeech: VoiceSpeech {
    public var key: String
    public var speaker: String
    /// Doubao's speech_rate, -50...100 (0 is normal).
    public var rate: Int
    var lines: VoiceLines

    public static let url = URL(string: "https://openspeech.bytedance.com/api/v3/tts/unidirectional")!
    public static let sampleRate = 24000

    public init(key: String, speaker: String, rate: Int = 0, lines: @escaping VoiceLines = VoiceNetwork.lines()) {
        self.key = key
        self.speaker = speaker
        self.rate = rate
        self.lines = lines
    }

    public func speak(_ text: String, play: @escaping @MainActor @Sendable (Data, Int) -> Void) async throws {
        let text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        var request = URLRequest(url: Self.url, timeoutInterval: 60)
        request.httpMethod = "POST"
        request.httpBody = try JSONSerialization.data(withJSONObject: [
            "req_params": [
                "text": text, "speaker": speaker,
                "audio_params": ["format": "pcm", "sample_rate": Self.sampleRate, "speech_rate": rate],
            ],
        ])
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue(key, forHTTPHeaderField: "X-Api-Key")
        request.setValue(speaker.hasPrefix("S_") ? "seed-icl-2.0" : "seed-tts-2.0", forHTTPHeaderField: "X-Api-Resource-Id")
        request.setValue(UUID().uuidString.replacingOccurrences(of: "-", with: "").lowercased(), forHTTPHeaderField: "X-Api-Request-Id")
        let (status, lines) = try await lines(request)
        guard status / 100 == 2 else {
            throw VoiceFailure(L("豆包语音 ", "Doubao speech ") + "HTTP \(status): " + OpenAIChat.said(await VoiceNetwork.refusal(lines)))
        }
        var reader = DoubaoStreamReader()
        for try await line in lines {
            if let sound = try reader.take(line) { await play(sound, Self.sampleRate) }
            if reader.finished { break }
        }
        try Task.checkCancellation()
        guard reader.finished else { throw VoiceFailure(L("豆包语音没有正常结束", "Doubao speech ended early")) }
    }
}

/// Reads Doubao's answer: one JSON object per line; `data` is base64
/// sound; code 20000000 ends it. An odd byte waits for the next piece
/// (a sample is two bytes).
struct DoubaoStreamReader {
    private var carry = Data()
    private(set) var finished = false

    mutating func take(_ line: String) throws -> Data? {
        let line = line.trimmingCharacters(in: .whitespaces)
        guard !line.isEmpty, let frame = try? JSONSerialization.jsonObject(with: Data(line.utf8)) as? [String: Any] else { return nil }
        let code = frame["code"] as? Int ?? 0
        if code == 20_000_000 {
            finished = true
            return nil
        }
        guard code == 0 else {
            throw VoiceFailure(L("豆包语音 ", "Doubao speech ") + "\(code): " + (frame["message"] as? String ?? ""))
        }
        guard let text = frame["data"] as? String, !text.isEmpty, let sound = Data(base64Encoded: text) else { return nil }
        var samples = Data(carry + sound)
        carry = Data()
        if samples.count % 2 == 1 {
            carry = Data(samples.suffix(1))
            samples = Data(samples.dropLast())
        }
        return samples.isEmpty ? nil : samples
    }
}

/// What the person chose for voice calls (no keys: those are in the
/// keychain, under `modelKey` and `speechKey`).
public struct VoiceSettings: Codable, Sendable, Hashable {
    /// The model's OpenAI-compatible API ("https://api.deepseek.com/v1").
    public var baseURL = ""
    public var model = ""
    /// Doubao's voice; empty is the default one.
    public var speaker = ""
    /// Doubao's speech_rate, -50...100.
    public var rate = 0

    public init(baseURL: String = "", model: String = "", speaker: String = "", rate: Int = 0) {
        self.baseURL = baseURL
        self.model = model
        self.speaker = speaker
        self.rate = rate
    }

    /// The desktop app's default voice.
    public static let defaultSpeaker = "zh_male_wennuanahu_uranus_bigtts"
    /// Where the keys are kept in the keychain.
    public static let modelKey = "voice:llm"
    public static let speechKey = "voice:tts"

    public var speakerInUse: String {
        let chosen = speaker.trimmingCharacters(in: .whitespacesAndNewlines)
        return chosen.isEmpty ? Self.defaultSpeaker : chosen
    }

    /// The model is named: with its key, a call can start.
    public var namesModel: Bool {
        OpenAIChat.endpoint(baseURL) != nil && !baseURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && !model.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }
}
