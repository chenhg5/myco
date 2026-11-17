import Foundation
import MycoAPI

// A voice call's "thinking": a fast text model behind any OpenAI-compatible
// chat API (OpenAI, DeepSeek, OpenRouter, Doubao's ...), streamed, with
// tools. The same contract as the desktop app's
// (internal/console/voice_model.go).

/// One message of the talk with the model.
public struct ChatMessage: Sendable, Hashable {
    /// system | user | assistant | tool
    public var role: String
    public var content: String
    /// On an assistant's message: what it wants done.
    public var toolCalls: [ToolCall]
    /// On a tool's message: the call it answers.
    public var toolCallID: String?

    public init(role: String, content: String, toolCalls: [ToolCall] = [], toolCallID: String? = nil) {
        self.role = role
        self.content = content
        self.toolCalls = toolCalls
        self.toolCallID = toolCallID
    }

    /// As the API takes it.
    var json: [String: Any] {
        var out: [String: Any] = ["role": role, "content": content]
        if !toolCalls.isEmpty { out["tool_calls"] = toolCalls.map(\.json) }
        if let toolCallID { out["tool_call_id"] = toolCallID }
        return out
    }
}

/// Something the model asks to have done: a tool's name and its
/// arguments as JSON text.
public struct ToolCall: Sendable, Hashable {
    public var id: String
    public var name: String
    public var arguments: String

    public init(id: String, name: String, arguments: String) {
        self.id = id
        self.name = name
        self.arguments = arguments
    }

    var json: [String: Any] {
        ["id": id, "type": "function", "function": ["name": name, "arguments": arguments]]
    }

    /// The arguments that are text, by name.
    public var texts: [String: String] {
        guard let object = try? JSONSerialization.jsonObject(with: Data(arguments.utf8)) as? [String: Any] else { return [:] }
        return object.compactMapValues { $0 as? String }
    }
}

/// A tool the model may ask for: its name, what it is for, and its
/// arguments (all text).
public struct VoiceTool: Sendable, Hashable {
    public struct Argument: Sendable, Hashable {
        public var name: String
        public var description: String

        public init(_ name: String, _ description: String) {
            self.name = name
            self.description = description
        }
    }

    public var name: String
    public var description: String
    public var arguments: [Argument]
    public var required: [String]

    public init(name: String, description: String, arguments: [Argument] = [], required: [String] = []) {
        self.name = name
        self.description = description
        self.arguments = arguments
        self.required = required
    }

    var json: [String: Any] {
        var properties: [String: Any] = [:]
        for argument in arguments {
            properties[argument.name] = ["type": "string", "description": argument.description]
        }
        return [
            "type": "function",
            "function": [
                "name": name, "description": description,
                "parameters": ["type": "object", "properties": properties, "required": required],
            ],
        ]
    }
}

/// What went wrong with the model or the voice, in words for the person.
public struct VoiceFailure: Error, Sendable, Hashable, LocalizedError {
    public var message: String

    public init(_ message: String) {
        self.message = message
    }

    public var errorDescription: String? { message }
}

/// The model a voice call talks with. One reply at a time: its text comes
/// bit by bit to `onText`, the name of each thing it wants done to
/// `onTool` as soon as it starts asking for it (what exactly comes
/// later), and the whole reply at the end.
public protocol VoiceModel: Sendable {
    func stream(
        _ messages: [ChatMessage], tools: [VoiceTool], onText: @escaping @MainActor @Sendable (String) -> Void,
        onTool: @escaping @MainActor @Sendable (String) -> Void
    ) async throws -> ChatMessage
}

/// How a streamed answer is fetched: the status and the body's lines as
/// they come. The app's is `VoiceNetwork.lines`; tests give lines of
/// their own.
public typealias VoiceLines = @Sendable (URLRequest) async throws -> (status: Int, lines: AsyncThrowingStream<String, Error>)

public enum VoiceNetwork {
    /// Lines over the network, as they arrive.
    /// - Parameter session: a session of its own (tests); nil is the
    ///   app's, as its proxy setting has it at that moment.
    public static func lines(_ session: URLSession? = nil) -> VoiceLines {
        { request in
            let (bytes, response) = try await (session ?? AppNetwork.shared.session(for: request.url)).bytes(for: request)
            let status = (response as? HTTPURLResponse)?.statusCode ?? 0
            let (stream, continuation) = AsyncThrowingStream<String, Error>.makeStream()
            let reading = Task {
                do {
                    for try await line in bytes.lines { continuation.yield(line) }
                    continuation.finish()
                } catch {
                    continuation.finish(throwing: error)
                }
            }
            continuation.onTermination = { _ in reading.cancel() }
            return (status, stream)
        }
    }

    /// What a service said when it refused: the start of its answer.
    static func refusal(_ lines: AsyncThrowingStream<String, Error>, limit: Int = 600) async -> String {
        var text = ""
        do {
            for try await line in lines {
                text += line
                if text.count >= limit { break }
            }
        } catch {}
        return String(text.prefix(limit)).trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

/// Puts a streamed chat completion together, line by line (server-sent
/// events: `data: {json}`, ended by `data: [DONE]`).
struct ChatStreamReader {
    private(set) var text = ""
    private(set) var done = false
    private var calls: [Int: ToolCall] = [:]
    private var order: [Int] = []
    /// The tools whose calls began in the lines taken since last asked.
    private var began: [String] = []

    /// The names of the tools the model has just started to call (their
    /// arguments are still being written), each given once.
    mutating func takeBegun() -> [String] {
        defer { began = [] }
        return began
    }

    /// Takes one line. Answers the text it added, if any; throws what the
    /// model said went wrong.
    mutating func take(_ line: String) throws -> String? {
        let line = line.trimmingCharacters(in: .whitespaces)
        guard line.hasPrefix("data:") else { return nil }
        let data = line.dropFirst("data:".count).trimmingCharacters(in: .whitespaces)
        if data == "[DONE]" {
            done = true
            return nil
        }
        guard let chunk = try? JSONSerialization.jsonObject(with: Data(data.utf8)) as? [String: Any] else { return nil }
        if let error = chunk["error"] as? [String: Any] {
            throw VoiceFailure(L("模型：", "Model: ") + (error["message"] as? String ?? "error"))
        }
        var added = ""
        for choice in chunk["choices"] as? [[String: Any]] ?? [] {
            guard let delta = choice["delta"] as? [String: Any] else { continue }
            if let content = delta["content"] as? String { added += content }
            for call in delta["tool_calls"] as? [[String: Any]] ?? [] {
                let index = call["index"] as? Int ?? 0
                if calls[index] == nil {
                    calls[index] = ToolCall(id: "", name: "", arguments: "")
                    order.append(index)
                }
                if let id = call["id"] as? String, !id.isEmpty { calls[index]?.id = id }
                if let function = call["function"] as? [String: Any] {
                    let named = !(calls[index]?.name.isEmpty ?? true)
                    calls[index]?.name += function["name"] as? String ?? ""
                    calls[index]?.arguments += function["arguments"] as? String ?? ""
                    if !named, let name = calls[index]?.name, !name.isEmpty { began.append(name) }
                }
            }
        }
        text += added
        return added.isEmpty ? nil : added
    }

    /// The reply as heard so far.
    var reply: ChatMessage {
        var out = ChatMessage(role: "assistant", content: text)
        for (position, index) in order.enumerated() {
            guard var call = calls[index] else { continue }
            if call.id.isEmpty { call.id = "call_\(position)" }
            out.toolCalls.append(call)
        }
        return out
    }
}

/// An OpenAI-compatible chat API, as set in Settings › 语音.
public struct OpenAIChat: VoiceModel {
    /// The API's address ("https://api.openai.com/v1"); the whole
    /// endpoint pasted in works too.
    public var base: String
    public var model: String
    public var key: String
    /// Names the call: some gateways route and cache by it.
    public var session: String
    var lines: VoiceLines

    public init(base: String, model: String, key: String, session: String = "", lines: @escaping VoiceLines = VoiceNetwork.lines()) {
        self.base = base
        self.model = model
        self.key = key
        self.session = session
        self.lines = lines
    }

    /// Where chat completions are asked for.
    static func endpoint(_ base: String) -> URL? {
        var text = base.trimmingCharacters(in: .whitespacesAndNewlines)
        while text.hasSuffix("/") { text.removeLast() }
        if text.isEmpty { text = "https://api.openai.com/v1" }
        if text.hasSuffix("/chat/completions") { text.removeLast("/chat/completions".count) }
        guard let url = URL(string: text + "/chat/completions"), url.scheme?.hasPrefix("http") == true, url.host() != nil else { return nil }
        return url
    }

    public func stream(
        _ messages: [ChatMessage], tools: [VoiceTool], onText: @escaping @MainActor @Sendable (String) -> Void,
        onTool: @escaping @MainActor @Sendable (String) -> Void = { _ in }
    ) async throws -> ChatMessage {
        guard let url = Self.endpoint(base) else {
            throw VoiceFailure(L("模型的接口地址不对", "The model's address is not a web address"))
        }
        var request = URLRequest(url: url, timeoutInterval: 90)
        request.httpMethod = "POST"
        var body: [String: Any] = ["model": model, "messages": messages.map(\.json), "stream": true, "temperature": 0.7]
        if !tools.isEmpty { body["tools"] = tools.map(\.json) }
        request.httpBody = try JSONSerialization.data(withJSONObject: body)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer " + key, forHTTPHeaderField: "Authorization")
        request.setValue("myco-voice", forHTTPHeaderField: "User-Agent")
        if !session.isEmpty { request.setValue(session, forHTTPHeaderField: "x-opencode-session") }
        let (status, lines) = try await lines(request)
        guard status / 100 == 2 else {
            throw VoiceFailure("HTTP \(status): " + Self.said(await VoiceNetwork.refusal(lines)))
        }
        var reader = ChatStreamReader()
        for try await line in lines {
            if let added = try reader.take(line) { await onText(added) }
            for name in reader.takeBegun() { await onTool(name) }
            if reader.done { break }
        }
        try Task.checkCancellation()
        return reader.reply
    }

    /// A service's refusal in words: the message inside its JSON when it
    /// has one, else what it sent.
    static func said(_ refusal: String) -> String {
        guard let object = try? JSONSerialization.jsonObject(with: Data(refusal.utf8)) as? [String: Any] else { return refusal }
        if let error = object["error"] as? [String: Any], let message = error["message"] as? String { return message }
        if let error = object["error"] as? String { return error }
        if let message = object["message"] as? String { return message }
        return refusal
    }
}

/// Cuts streamed text into pieces worth saying on their own: at the end
/// of a sentence, or at a comma once a piece is long.
struct SentenceSplitter {
    private var buffer = ""

    mutating func add(_ delta: String) -> [String] {
        var out: [String] = []
        for character in delta {
            buffer.append(character)
            let count = buffer.count
            if "。！？!?；;\n".contains(character) {
                out.append(take())
            } else if "，,、：:".contains(character), count >= 18 {
                out.append(take())
            } else if character == ".", count >= 2 {
                out.append(take())
            }
        }
        return out
    }

    private mutating func take() -> String {
        defer { buffer = "" }
        return buffer
    }

    /// What is left when the text ends.
    mutating func flush() -> String {
        take().trimmingCharacters(in: .whitespacesAndNewlines)
    }
}
