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

/// A speech service the voice can speak with. Which ones there are is
/// this list (one today), as on the desktop (`internal/console/voice_sync.go`):
/// the settings name the one chosen and keep a key for each, and the
/// settings page draws what the list says.
public struct SpeechService: Sendable, Hashable, Identifiable {
    /// Its id, as the workspace keeps it. No underscores (see `VoiceDocument`).
    public var id: String
    public var name: String
    public var englishName: String
    /// The voice used when none is chosen.
    public var defaultSpeaker: String
    /// How to get a key and a voice.
    public var help: String
    public var englishHelp: String

    public static let doubao = SpeechService(
        id: "doubao", name: "豆包语音", englishName: "Doubao Speech",
        defaultSpeaker: "zh_male_wennuanahu_uranus_bigtts",
        help: "在火山引擎控制台「豆包语音」里开通语音合成 2.0、创建 API Key。音色从列表里选，选好点「试听」；复刻的声音在列表最下面填 S_ 开头的 ID。",
        englishHelp: "Turn on speech synthesis 2.0 under Doubao Speech in the Volcengine console and create an API key. Pick a voice from the list and try it; for a cloned voice, type its ID (it starts with S_) at the end of the list.")

    /// Every service this app can speak with.
    public static let all: [SpeechService] = [.doubao]
    public static let standard = "doubao"

    /// The service by id; nil for one this app does not know (chosen on a
    /// newer desktop app).
    public static func named(_ id: String) -> SpeechService? {
        all.first { $0.id == (id.isEmpty ? standard : id) }
    }

    /// The voices to choose from; none when the service has no list (the
    /// voice is then typed in). Another may always be typed in, like a
    /// cloned one.
    public var voices: [SpeechVoice] {
        id == Self.doubao.id ? SpeechVoice.doubao : []
    }

    /// Makes its voice, with its key and what was chosen for it.
    public func speech(key: String, settings: VoiceSettings) -> any VoiceSpeech {
        switch id {
        default: return DoubaoSpeech(key: key, speaker: settings.speakerInUse, rate: settings.rate)
        }
    }
}

/// What the person chose for voice calls (no keys: those are in the
/// keychain, under `modelKey` and `speechKey(of:)`).
public struct VoiceSettings: Codable, Sendable, Hashable {
    /// The model's OpenAI-compatible API ("https://api.deepseek.com/v1").
    public var baseURL = ""
    public var model = ""
    /// The speech service; empty is the standard one.
    public var provider = ""
    /// Its voice; empty is its default one.
    public var speaker = ""
    /// How fast it talks, -50...100.
    public var rate = 0
    /// Kept on the workspaces too, keys and all, so the person's other
    /// devices have them. On unless turned off.
    public var share = true
    /// When the person last changed them, on whichever device.
    public var updatedAt: Date?

    public init(baseURL: String = "", model: String = "", provider: String = "", speaker: String = "", rate: Int = 0, share: Bool = true, updatedAt: Date? = nil) {
        self.baseURL = baseURL
        self.model = model
        self.provider = provider
        self.speaker = speaker
        self.rate = rate
        self.share = share
        self.updatedAt = updatedAt
    }

    // What was saved before a field existed still reads.
    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        baseURL = try values.decodeIfPresent(String.self, forKey: .baseURL) ?? ""
        model = try values.decodeIfPresent(String.self, forKey: .model) ?? ""
        provider = try values.decodeIfPresent(String.self, forKey: .provider) ?? ""
        speaker = try values.decodeIfPresent(String.self, forKey: .speaker) ?? ""
        rate = try values.decodeIfPresent(Int.self, forKey: .rate) ?? 0
        share = try values.decodeIfPresent(Bool.self, forKey: .share) ?? true
        updatedAt = try values.decodeIfPresent(Date.self, forKey: .updatedAt)
    }

    /// The desktop app's default voice.
    public static let defaultSpeaker = SpeechService.doubao.defaultSpeaker
    /// Where the keys are kept in the keychain. The standard service's
    /// keeps the name it had when it was the only one.
    public static let modelKey = "voice:llm"
    public static let speechKey = "voice:tts"

    public static func speechKey(of provider: String) -> String {
        provider.isEmpty || provider == SpeechService.standard ? speechKey : speechKey + ":" + provider
    }

    /// The service in use: the one chosen, when this app knows it.
    public var service: SpeechService {
        SpeechService.named(provider) ?? .doubao
    }

    public var speakerInUse: String {
        let chosen = speaker.trimmingCharacters(in: .whitespacesAndNewlines)
        return chosen.isEmpty ? service.defaultSpeaker : chosen
    }

    /// The model is named: with its key, a call can start.
    public var namesModel: Bool {
        OpenAIChat.endpoint(baseURL) != nil && !baseURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && !model.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }
}
