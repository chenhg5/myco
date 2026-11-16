import Foundation

// The memo of the last call. What is said in a call is not in the chat
// (only what was passed on to the agent is), so a call that ended loses
// it. When a call ends, its last few lines are kept on this device, word
// for word, by workspace and agent; the next call with that agent within
// a day starts with them. No summary (no extra call to the model), and
// nothing on the workspace. The desktop app's, internal/console/voice_memo.go.

public struct VoiceMemo: Codable, Sendable, Hashable {
    public struct Line: Codable, Sendable, Hashable {
        /// person | assistant
        public var who: String
        public var text: String

        public init(who: String, text: String) {
            self.who = who
            self.text = text
        }
    }

    /// When the call ended.
    public var at: Date
    public var lines: [Line]

    public init(at: Date, lines: [Line]) {
        self.at = at
        self.lines = lines
    }

    static let linesKept = 8
    static let lineLength = 300
    public static let maxAge: TimeInterval = 24 * 3600

    public static func key(workspace: String, agent: String) -> String {
        workspace + " " + agent
    }

    /// The last lines said in a call: the person's and the voice's words,
    /// not what was done or what the system said.
    public static func lines(of history: [ChatMessage]) -> [Line] {
        var out: [Line] = []
        for message in history {
            let text = message.content.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !text.isEmpty else { continue }
            switch message.role {
            case "user": out.append(Line(who: "person", text: VoiceContext.clip(text, lineLength)))
            case "assistant": out.append(Line(who: "assistant", text: VoiceContext.clip(text, lineLength)))
            default: break
            }
        }
        return Array(out.suffix(linesKept))
    }

    /// The memo as a part of the model's instructions, after the
    /// conversation: for reference, not to be recited.
    public func prompt(language: AppLanguage, now: Date = Date()) -> String {
        let english = language == .en
        let ago = now.timeIntervalSince(at)
        let when: String
        if ago < 3600 {
            let minutes = max(1, Int(ago / 60))
            when = english ? "\(minutes) min ago" : "\(minutes) 分钟前"
        } else {
            let hours = Int(ago / 3600)
            when = english ? "\(hours) h ago" : "\(hours) 小时前"
        }
        var out = english
            ? "\nThe end of your last call with them (\(when)), for reference; the chat above is what counts. Bring it up only if it matters now:\n"
            : "\n上一通电话（\(when)）最后说的，供参考，以上面的对话为准；用得上再提，别主动复述：\n"
        for line in lines {
            let who = line.who == "assistant" ? (english ? "You" : "你") : (english ? "They" : "对方")
            out += "- " + who + "：" + line.text + "\n"
        }
        return out
    }
}

/// Where the memos are kept on this device: a file, or memory in tests.
public protocol VoiceMemoStore: Sendable {
    func read() -> Data?
    func write(_ data: Data)
}

public enum VoiceMemos {
    private static func all(in store: any VoiceMemoStore) -> [String: VoiceMemo] {
        guard let data = store.read() else { return [:] }
        return (try? HubJSONFree.decoder.decode([String: VoiceMemo].self, from: data)) ?? [:]
    }

    /// The last call's memo with this agent, if it is recent.
    public static func memo(for key: String, in store: any VoiceMemoStore, now: Date = Date()) -> VoiceMemo? {
        guard let memo = all(in: store)[key], !memo.lines.isEmpty, now.timeIntervalSince(memo.at) <= VoiceMemo.maxAge else { return nil }
        return memo
    }

    /// Keeps what was said in a call that ended. A call where nothing was
    /// said keeps the memo from before. Old memos are dropped.
    public static func keep(_ history: [ChatMessage], for key: String, in store: any VoiceMemoStore, now: Date = Date()) {
        let lines = VoiceMemo.lines(of: history)
        guard !lines.isEmpty else { return }
        var all = all(in: store).filter { now.timeIntervalSince($0.value.at) <= VoiceMemo.maxAge }
        all[key] = VoiceMemo(at: now, lines: lines)
        if let data = try? HubJSONFree.encoder.encode(all) { store.write(data) }
    }
}

/// Plain JSON for what stays on this device (dates as seconds).
enum HubJSONFree {
    static let decoder: JSONDecoder = {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .secondsSince1970
        return decoder
    }()

    static let encoder: JSONEncoder = {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .secondsSince1970
        encoder.outputFormatting = [.sortedKeys]
        return encoder
    }()
}

/// The memos in a file of the app's own, left out of backups: they are
/// what was said on this device and belong nowhere else.
public struct FileMemoStore: VoiceMemoStore {
    let url: URL

    public init(url: URL) {
        self.url = url
    }

    /// `voice-memo.json` in the app's Application Support folder.
    public static var standard: FileMemoStore {
        let folder = (try? FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true))
            ?? FileManager.default.temporaryDirectory
        return FileMemoStore(url: folder.appendingPathComponent("voice-memo.json"))
    }

    public func read() -> Data? {
        try? Data(contentsOf: url)
    }

    public func write(_ data: Data) {
        var url = url
        try? data.write(to: url, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? url.setResourceValues(values)
    }
}

/// Memos in memory (tests, and the demo).
public final class MemoryMemoStore: VoiceMemoStore, @unchecked Sendable {
    private let lock = NSLock()
    private var data: Data?

    public init() {}

    public func read() -> Data? {
        lock.lock()
        defer { lock.unlock() }
        return data
    }

    public func write(_ data: Data) {
        lock.lock()
        defer { lock.unlock() }
        self.data = data
    }
}
