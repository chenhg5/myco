import Foundation

/// Speech recognition hears names it does not know poorly, above all
/// English words in Chinese ("Claude", "Codex", "GitHub"). Two things help,
/// for voice calls and dictation alike (the desktop app's
/// internal/console/speech_words.go; the two must agree):
///
/// - the recognizer is told the words to expect (`contextualStrings`): the
///   workspace's agents, projects and members, words people here use all
///   the time, and the words the person added in Settings › Voice;
/// - what it heard is corrected, for those words only: a known word's
///   casing ("github" → "GitHub"), its spelling with spaces ("Deep Seek"),
///   and how a Chinese recognizer tends to write it ("克劳德", "cloud" →
///   "Claude"). Nothing else in what was said is touched.
public enum SpeechWords {
    /// Words people working with myco say all the time.
    public static let common = [
        "Claude", "Claude Code", "Codex", "myco", "Hub", "runner", "agent",
        "GitHub", "PR", "CI", "commit", "push", "pull", "merge", "rebase", "branch", "release", "tag",
        "DeepSeek", "OpenAI", "GPT", "Anthropic", "Opus", "Sonnet", "Haiku", "Gemini", "Kimi", "Qwen", "Doubao",
        "API", "SDK", "token", "bug", "README", "Markdown", "JSON", "React", "TypeScript", "Python",
        "Docker", "Xcode", "iOS", "Android", "macOS", "Windows", "Linux", "WSL", "VS Code", "Cursor", "Wails",
    ]

    /// How recognizers write some common words (Chinese ones mostly hear
    /// them by sound). Only forms nobody says for anything else: no real
    /// words ("cloud", "codecs", "cat", "句号"), which would change what
    /// people mean. Letters said apart ("P R") and casing are rules.
    static let aliases: [String: [String]] = [
        "Claude": ["克劳德", "克劳迪", "克洛德", "可劳德", "克老德", "Claud", "Clode"],
        "Codex": ["扣德克斯", "口德克斯", "寇德克斯", "科德克斯", "抠得克斯", "Kodex"],
        "myco": ["麦扣", "迈扣"],
        "DeepSeek": ["迪普西克", "迪普seek"],
        "GitHub": ["吉特哈布"],
    ]

    /// Everyday words an alias must not be, nor a casing rule change (a
    /// recognizer writes these when people mean them). The desktop's list.
    static let commonWords: Set<String> = Set("""
        句号 逗号 问号 妹妹 姐姐 哥哥 弟弟 妈妈 爸爸 宝宝 朋友 同事 老板 老师 我们 你们 他们 她们 它们 这个 那个 一个 什么 怎么 为什么 可以 没有 知道 觉得 现在 今天 明天 昨天
        时候 东西 事情 问题 工作 公司 项目 代码 电脑 手机 包裹 包子 口袋 扣子 克拉 卡通 猫咪 小猫 小狗 冰箱 蛋糕 咖啡 米饭 面条 苹果 香蕉 西瓜 云端 云朵 天气 下雨 开会 吃饭 睡觉 回家
        hub cursor windows python react docker opus sonnet haiku gemini markdown token bug tag branch release
        half cat dog cloud clouds code codes codec codecs cool cold call cloth close clock club deep seek sick sip dip git get hub hug pub up my co go do me be we see sea you ok okay yes no
        the and for are but not all any can had her was one our out day has him his how man new now old see two way who boy did its let put say she too use make like time just know take people year good some them other than then look only come over think also back after work first well even want because these give most
        """.split(whereSeparator: \.isWhitespace).map(String.init))

    /// Why a person's alias is not used: one character, or a word people
    /// say for other things (it would change what they mean). nil: fine.
    static func unsafeAlias(_ alias: String) -> String? {
        let alias = alias.trimmingCharacters(in: .whitespaces)
        if alias.count < 2 || (SpeechFixer.isLatin(alias) && alias.count < 3) { return "too short" }
        if commonWords.contains(alias.lowercased()) { return "a common word" }
        return nil
    }

    /// Apple suggests keeping contextual strings to about a hundred.
    public static let maxWords = 100

    /// The person's words, one per line: a word, or a word with how it is
    /// misheard ("Claude = 克劳德, cloud").
    public static func parse(_ lines: [String]) -> (words: [String], aliases: [String: [String]]) {
        var words: [String] = []
        var aliases: [String: [String]] = [:]
        for raw in lines {
            let line = raw.replacingOccurrences(of: "＝", with: "=").trimmingCharacters(in: .whitespaces)
            guard !line.isEmpty else { continue }
            let parts = line.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)
            let word = parts[0].trimmingCharacters(in: .whitespaces)
            guard !word.isEmpty else { continue }
            words.append(word)
            if parts.count > 1 {
                for alias in parts[1].split(whereSeparator: { ",，、;；".contains($0) }) {
                    let alias = alias.trimmingCharacters(in: .whitespaces)
                    if !alias.isEmpty, alias != word, unsafeAlias(alias) == nil { aliases[word, default: []].append(alias) }
                }
            }
        }
        return (words, aliases)
    }

    /// Names a person would say: ids and generated names
    /// ("invited-m9ixux9a", "cloud-hdkwmcki") are not words to listen for;
    /// a name with parts is listened for by its parts too ("pm-codex" →
    /// "pm codex").
    public static func speakable(_ names: [String]) -> [String] {
        var out: [String] = []
        for name in names where !looksGenerated(name) {
            out.append(name)
            if name.contains("-") || name.contains("_") {
                out.append(name.replacingOccurrences(of: "-", with: " ").replacingOccurrences(of: "_", with: " "))
            }
        }
        return out
    }

    /// A part with letters and digits mixed, or five consonants in a row
    /// ("hdkwmcki"), as random ids have.
    static func looksGenerated(_ name: String) -> Bool {
        for part in name.split(whereSeparator: { $0 == "-" || $0 == "_" || $0 == " " }) {
            let letters = part.filter { $0.isASCII && $0.isLetter }.count
            let digits = part.filter { $0.isASCII && $0.isNumber }.count
            if letters > 0, digits > 0, part.count >= 6 { return true }
            if consonantRun(String(part)) >= 5 { return true }
        }
        return false
    }

    static func consonantRun(_ text: String) -> Int {
        var most = 0, run = 0
        for c in text.lowercased() {
            if c >= "a", c <= "z", !"aeiouy".contains(c) {
                run += 1
                most = max(most, run)
            } else {
                run = 0
            }
        }
        return most
    }

    /// What the recognizer listens for and what is corrected: the person's
    /// words first, then the workspace's names, then the common words,
    /// each once, at most `maxWords`.
    public static func vocabulary(mine: [String], workspace: [String]) -> (words: [String], aliases: [String: [String]]) {
        let parsed = parse(mine)
        var seen: Set<String> = []
        var words: [String] = []
        for list in [parsed.words, speakable(workspace.filter { !$0.hasPrefix("@") }), common] {
            for word in list {
                let key = word.lowercased()
                guard !word.isEmpty, !seen.contains(key), words.count < maxWords else { continue }
                seen.insert(key)
                words.append(word)
            }
        }
        return (words, parsed.aliases)
    }
}

/// Corrects what was heard, for the vocabulary's words only.
public struct SpeechFixer: @unchecked Sendable {
    // (NSRegularExpression is immutable once made)
    private var rules: [(NSRegularExpression, String)] = []

    /// What may not touch an English word for it to be a word of its own
    /// ("pr" in "print" is not "PR"; "cloud" in "cloud-hdkwmcki" is a name).
    private static let latinEdge = #"A-Za-z0-9_\-.@/"#

    public init(language: String, words: [String], extra: [String: [String]] = [:]) {
        let zh = !language.lowercased().hasPrefix("en")
        let have = Set(words)
        var rules: [(NSRegularExpression, String)] = []
        func add(_ pattern: String, _ template: String, caseInsensitive: Bool = true) {
            if let regex = try? NSRegularExpression(pattern: pattern, options: caseInsensitive ? [.caseInsensitive] : []) {
                rules.append((regex, template))
            }
        }
        func latinRule(_ from: String, _ to: String) {
            // The words around it must not be English too: "cloud" alone in
            // Chinese is "Claude", "the cloud server" stays.
            add("(^|[^\(Self.latinEdge)])" + Self.spaced(NSRegularExpression.escapedPattern(for: from)) + "($|[^\(Self.latinEdge)])",
                "$1" + NSRegularExpression.escapedTemplate(for: to) + "$2")
        }
        // Longer words first: "Claude Code" before "Claude".
        let sorted = words.enumerated().sorted { a, b in
            a.element.utf8.count != b.element.utf8.count ? a.element.utf8.count > b.element.utf8.count : a.offset < b.offset
        }.map(\.element)
        for word in sorted where Self.isLatin(word) && word.count >= 2 {
            // Its casing, and its parts said apart ("Deep Seek", "Git Hub").
            // In English a capitalised word may be a plain one ("cursor",
            // "windows"): only odd casings ("GitHub", "macOS", "PR") are fixed.
            // Nor an everyday word ("hub", "python") in any language.
            if word != word.lowercased(), zh || !Self.titleCase(word), !SpeechWords.commonWords.contains(word.lowercased()) {
                latinRule(word, word)
            }
            let parts = Self.camelParts(word)
            if parts.count > 1 { latinRule(parts.joined(separator: " "), word) }
            // In capitals, as some recognizers write all English ("PUSH",
            // "IOS", "CLAUDE CODE"): the word as it is written.
            let upper = word.uppercased()
            if upper != word {
                add("(^|[^\(Self.latinEdge)])" + Self.spaced(NSRegularExpression.escapedPattern(for: upper)) + "($|[^\(Self.latinEdge)])",
                    "$1" + NSRegularExpression.escapedTemplate(for: word) + "$2", caseInsensitive: false)
            }
            // Letters said one by one: "P R", "a p i", "C.I." → the word.
            if let spelled = Self.spelledLetters(word) {
                add(spelled, "$1" + NSRegularExpression.escapedTemplate(for: word) + "$2")
            }
        }
        for word in sorted where have.contains(word) {
            let aliases = (zh ? SpeechWords.aliases[word] ?? [] : []) + (extra[word] ?? [])
            for alias in aliases {
                if Self.isLatin(alias) {
                    if !zh, alias.lowercased() == "cloud" { continue } // in English, "cloud" is a cloud
                    if zh {
                        // Standing alone between Chinese, spaces, punctuation
                        // or the ends: not next to other English words.
                        add(#"(^|[^A-Za-z0-9_\-.@/ ]\s*|^\s*)"# + Self.spaced(NSRegularExpression.escapedPattern(for: alias)) + #"(\s*[^A-Za-z0-9_\-.@/ ]|\s*$)"#,
                            "$1" + NSRegularExpression.escapedTemplate(for: word) + "$2")
                    } else {
                        latinRule(alias, word)
                    }
                } else {
                    add(NSRegularExpression.escapedPattern(for: alias), NSRegularExpression.escapedTemplate(for: word), caseInsensitive: false)
                }
            }
        }
        self.rules = rules
    }

    public func fix(_ text: String) -> String {
        guard !text.isEmpty, !rules.isEmpty else { return text }
        var text = text
        for (regex, template) in rules {
            // Twice: matches that share an edge ("cloud cloud") both change.
            for _ in 0..<2 {
                text = regex.stringByReplacingMatches(in: text, range: NSRange(text.startIndex..., in: text), withTemplate: template)
            }
        }
        return text
    }

    /// An all-capitals word of 2 to 5 letters said one letter at a time
    /// ("P R", "a p i", "C.I."), standing alone, as a pattern.
    static func spelledLetters(_ word: String) -> String? {
        guard (2...5).contains(word.count), word == word.uppercased(), word.allSatisfy({ $0 >= "A" && $0 <= "Z" }) else { return nil }
        return "(^|[^\(latinEdge)])" + word.map(String.init).joined(separator: #"[\s.]+"#) + #"\.?($|[^"# + latinEdge + "])"
    }

    /// The spaces in a phrase may be any run of spaces.
    private static func spaced(_ quoted: String) -> String { quoted.replacingOccurrences(of: " ", with: #"\s+"#) }

    static func isLatin(_ text: String) -> Bool { text.allSatisfy(\.isASCII) && text.contains(where: \.isLetter) }

    /// "DeepSeek" → "Deep", "Seek" (not "PR" or "macOS").
    static func camelParts(_ word: String) -> [String] {
        guard !word.contains(where: { " -_".contains($0) }) else { return [] }
        var parts: [String] = []
        var current = ""
        var previous: Character?
        for c in word {
            if let previous, c.isUppercase, previous.isLowercase {
                parts.append(current)
                current = ""
            }
            current.append(c)
            previous = c
        }
        parts.append(current)
        return parts.allSatisfy { $0.count >= 2 } ? parts : []
    }

    /// "Claude", "Cursor" (as opposed to "GitHub", "PR", "macOS").
    static func titleCase(_ word: String) -> Bool {
        guard let first = word.first, first.isUppercase else { return false }
        return !word.dropFirst().contains(where: \.isUppercase)
    }
}

/// The words the recognizers listen for now, and how what they hear is
/// corrected: set by the app (the workspace in front, the person's words),
/// read by the recognizers on their own threads when they start hearing.
public final class SpeechVocabulary: @unchecked Sendable {
    public static let shared = SpeechVocabulary()

    private let lock = NSLock()
    private var words: [String] = []
    private var aliases: [String: [String]] = [:]
    private var fixers: [String: SpeechFixer] = [:]

    /// Where the person's words are kept on this device (one per line).
    public static let defaultsKey = "myco-speech-words"

    private var mine: [String] = SpeechVocabulary.mine()
    private var workspace: [String] = []

    /// The names of the workspace in front: agents, projects, members.
    public func setWorkspace(_ names: [String]) {
        lock.lock()
        defer { lock.unlock() }
        workspace = names
        rebuild()
    }

    /// The person's own words (Settings › Voice), one per line.
    public func setMine(_ lines: [String]) {
        lock.lock()
        defer { lock.unlock() }
        mine = lines
        rebuild()
    }

    private func rebuild() {
        let vocabulary = SpeechWords.vocabulary(mine: mine, workspace: workspace)
        guard vocabulary.words != words || vocabulary.aliases != aliases else { return }
        words = vocabulary.words
        aliases = vocabulary.aliases
        fixers = [:]
    }

    /// What to listen for, and the corrector for this language.
    public func current(language: String) -> (words: [String], fixer: SpeechFixer) {
        lock.lock()
        defer { lock.unlock() }
        let key = language.lowercased().hasPrefix("en") ? "en" : "zh"
        if let fixer = fixers[key] { return (words, fixer) }
        let fixer = SpeechFixer(language: key, words: words, extra: aliases)
        fixers[key] = fixer
        return (words, fixer)
    }

    /// The person's own words, as kept on this device.
    public static func mine(in defaults: UserDefaults = .standard) -> [String] {
        (defaults.string(forKey: defaultsKey) ?? "").components(separatedBy: "\n")
    }
}
