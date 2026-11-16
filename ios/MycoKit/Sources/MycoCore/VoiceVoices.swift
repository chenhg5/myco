import Foundation

/// One voice on a speech service's list, for the settings to choose from.
public struct SpeechVoice: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    /// The language it is made for: zh, en, ja, ... (Doubao's id prefix;
    /// mx is Mexican Spanish, tl Filipino).
    public var lang: String
    /// female | male
    public var gender: String

    public init(id: String, name: String, lang: String, gender: String) {
        self.id = id
        self.name = name
        self.lang = lang
        self.gender = gender
    }

    /// Doubao's voices for its 2.0 model, the official list, shipped with
    /// the app: Doubao lists a tenant's voices only to a Volcengine
    /// account's signing keys, not to the speech API key kept here. The
    /// same file as the desktop app's (internal/console/
    /// voice_doubao_voices.json; a Go test holds the two together). A
    /// voice not on it (a cloned one, S_...) is typed in by hand.
    public static let doubao: [SpeechVoice] = {
        guard let url = Bundle.module.url(forResource: "doubao-voices", withExtension: "json"),
              let data = try? Data(contentsOf: url),
              let voices = try? JSONDecoder().decode([SpeechVoice].self, from: data)
        else { return [] }
        return voices
    }()

    /// Voices by language: Chinese first, then English, then the rest in
    /// the list's order.
    public static func groups(_ voices: [SpeechVoice]) -> [(lang: String, voices: [SpeechVoice])] {
        var order: [String] = []
        var by: [String: [SpeechVoice]] = [:]
        for voice in voices {
            if by[voice.lang] == nil { order.append(voice.lang) }
            by[voice.lang, default: []].append(voice)
        }
        let first = ["zh", "en"]
        let sorted = first.filter { by[$0] != nil } + order.filter { !first.contains($0) }
        return sorted.map { ($0, by[$0] ?? []) }
    }

    private static let languages: [String: (String, String)] = [
        "zh": ("中文", "Chinese"), "en": ("英语", "English"), "ja": ("日语", "Japanese"), "ko": ("韩语", "Korean"),
        "es": ("西班牙语", "Spanish"), "mx": ("西班牙语（墨西哥）", "Spanish (Mexico)"), "pt": ("葡萄牙语（巴西）", "Portuguese (Brazil)"),
        "fr": ("法语", "French"), "de": ("德语", "German"), "it": ("意大利语", "Italian"), "ru": ("俄语", "Russian"),
        "id": ("印尼语", "Indonesian"), "ms": ("马来语", "Malay"), "th": ("泰语", "Thai"), "vi": ("越南语", "Vietnamese"),
        "ar": ("阿拉伯语", "Arabic"), "tl": ("菲律宾语", "Filipino"),
    ]

    /// A voice language's name ("中文", "Chinese"); its code when unknown.
    public static func languageName(_ code: String) -> String {
        guard let names = languages[code] else { return code }
        return L(names.0, names.1)
    }

    /// "团子 · 女".
    public var label: String {
        name + " · " + (gender == "female" ? L("女", "female") : L("男", "male"))
    }
}

/// What a voice says when it is tried, and how the sound is wrapped to
/// play it. The desktop app's (internal/console/voice_voices.go).
public enum VoiceSample {
    /// In the voice's own language (English when there is no line for it).
    static let lines: [String: String] = [
        "zh": "你好，我是你的语音助手。有什么要转告 agent 的，直接跟我说。",
        "en": "Hi, I'm your voice assistant. Tell me what to pass on to the agent.",
        "ja": "こんにちは、音声アシスタントです。エージェントに伝えたいことを話してください。",
        "ko": "안녕하세요, 음성 비서입니다. 에이전트에게 전할 말을 해 주세요.",
        "es": "Hola, soy tu asistente de voz. Dime qué le paso al agente.",
        "mx": "Hola, soy tu asistente de voz. Dime qué le paso al agente.",
        "pt": "Olá, sou seu assistente de voz. Diga o que devo passar ao agente.",
        "fr": "Bonjour, je suis votre assistant vocal. Dites-moi quoi transmettre à l'agent.",
        "de": "Hallo, ich bin dein Sprachassistent. Sag mir, was ich dem Agenten ausrichten soll.",
        "it": "Ciao, sono il tuo assistente vocale. Dimmi cosa riferire all'agente.",
        "ru": "Привет, я ваш голосовой помощник. Скажите, что передать агенту.",
        "id": "Halo, saya asisten suara Anda. Katakan apa yang harus saya sampaikan ke agen.",
        "ms": "Hai, saya pembantu suara anda. Beritahu saya apa yang perlu disampaikan kepada ejen.",
        "th": "สวัสดีค่ะ ฉันเป็นผู้ช่วยเสียงของคุณ บอกฉันได้เลยว่าจะให้ส่งต่ออะไรถึงเอเจนต์",
        "vi": "Xin chào, tôi là trợ lý giọng nói của bạn. Hãy nói điều cần chuyển cho tác tử.",
        "ar": "مرحبًا، أنا مساعدك الصوتي. أخبرني بما تريد أن أنقله إلى الوكيل.",
        "tl": "Kumusta, ako ang iyong voice assistant. Sabihin mo kung ano ang ipapasa ko sa agent.",
    ]

    /// The line a voice says when tried: by the list's language for it,
    /// else by its id's prefix, else in the app's language.
    public static func line(for speaker: String, in service: SpeechService, language: AppLanguage) -> String {
        var lang = service.voices.first { $0.id == speaker }?.lang ?? ""
        if lang.isEmpty, let cut = speaker.firstIndex(of: "_"), cut > speaker.startIndex { lang = String(speaker[..<cut]) }
        if let line = lines[lang] { return line }
        return lines[language == .zh ? "zh" : "en"] ?? ""
    }

    /// 16-bit little-endian mono samples as a WAV file, to hand to a player.
    public static func wav(_ samples: Data, rate: Int) -> Data {
        func le<T: FixedWidthInteger>(_ value: T) -> Data { withUnsafeBytes(of: value.littleEndian) { Data($0) } }
        var out = Data()
        out.append(Data("RIFF".utf8))
        out.append(le(UInt32(36 + samples.count)))
        out.append(Data("WAVEfmt ".utf8))
        out.append(le(UInt32(16)))
        out.append(le(UInt16(1)))
        out.append(le(UInt16(1)))
        out.append(le(UInt32(rate)))
        out.append(le(UInt32(rate * 2)))
        out.append(le(UInt16(2)))
        out.append(le(UInt16(16)))
        out.append(Data("data".utf8))
        out.append(le(UInt32(samples.count)))
        out.append(samples)
        return out
    }
}
