import Foundation
import MycoAPI

/// The page side of the myco bridge, as the desktop app answers it
/// (`pages.tsx`): a page opened in the app asks for its shared state
/// (`myco.ready`, `myco.update`), its records (`myco.data`), to send its
/// agent a message as me (`myco.send`, once I allow it), to keep a little
/// of its own (`myco.storage`), and to hand a file over (`myco.download`).
/// What others change comes to it while it is open (`poll`).
@MainActor
public final class PageBridge {
    /// Why a call was not answered: said to the page as an error with a
    /// code, and the latest when someone wrote first.
    public struct Refusal: Error {
        public var message: String
        public var code: String?
        /// JSON: the record or state as it is now.
        public var latest: Data?
    }

    public struct Doors {
        /// Asks me whether this page may send its agent (named) this text.
        public var askToSend: @MainActor (_ text: String, _ to: String) async -> Bool
        /// Asks before connecting this page to an authorized computer.
        public var askToConnect: @MainActor (_ computer: RemoteComputer, _ control: Bool) async -> Bool
        public var remoteConnectionChanged: @MainActor (Bool) -> Void
        /// Hands a file the page made to the system (save, send).
        public var save: @MainActor (_ name: String, _ data: Data, _ type: String) async -> Void
        /// Sends a task as me; answers its id.
        public var send: @MainActor (_ to: String, _ body: String, _ project: String?) async throws -> String

        public init(
            askToSend: @escaping @MainActor (String, String) async -> Bool,
            save: @escaping @MainActor (String, Data, String) async -> Void,
            send: @escaping @MainActor (String, String, String?) async throws -> String,
            askToConnect: @escaping @MainActor (RemoteComputer, Bool) async -> Bool = { _, _ in false },
            remoteConnectionChanged: @escaping @MainActor (Bool) -> Void = { _ in }
        ) {
            self.askToSend = askToSend
            self.askToConnect = askToConnect
            self.remoteConnectionChanged = remoteConnectionChanged
            self.save = save
            self.send = send
        }
    }

    private let hub: any HubAPI
    public private(set) var page: Page
    private let me: String
    private let names: @MainActor (String) -> String
    private let doors: Doors
    private let remote: PageRemoteDesktop
    private let network: PageNetwork
    private var preferences = ["language": AppLanguage.current.rawValue, "theme": "light"]
    private let storage: UserDefaults
    private let storageKey: String
    /// The state's version the page has.
    private var known = 0
    private var bridged = false
    /// The latest change of its records the page has, once it listens.
    private var dataSeq = 0
    private var watching = false
    private var allowedToSend = false
    private var sends: [Date] = []

    public init(
        hub: any HubAPI, page: Page, me: String, workspace: String, names: @escaping @MainActor (String) -> String, doors: Doors,
        storage: UserDefaults = .standard, network: PageNetwork? = nil
    ) {
        self.hub = hub
        self.page = page
        self.me = me
        self.names = names
        self.doors = doors
        self.remote = PageRemoteDesktop(hub: hub, ask: doors.askToConnect, changed: doors.remoteConnectionChanged)
        self.network = network ?? PageNetwork()
        self.storage = storage
        storageKey = "myco-page-store:\(workspace):\(page.id)"
    }

    /// The page as it is now (a newer version, its name).
    public func update(_ page: Page) {
        self.page = page
    }

    private static func json(_ value: Any) -> Data {
        (try? JSONSerialization.data(withJSONObject: value, options: [.fragmentsAllowed])) ?? Data("null".utf8)
    }

    private static func object(_ data: Data) -> Any {
        (try? JSONSerialization.jsonObject(with: data, options: [.fragmentsAllowed])) ?? NSNull()
    }

    private func call(_ method: String, _ path: String, query: [String: String] = [:], body: Any? = nil) async throws -> PageAnswer {
        try await hub.pageRequest(method, page: page.id, path: path, query: query, body: body.map(Self.json))
    }

    private static func segment(_ text: String) -> String {
        text.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-_."))) ?? text
    }

    /// Answers one call: its value as JSON, or a refusal.
    public func answer(_ method: String, _ args: [String: Any]) async throws -> Data {
        do {
            return try await answering(method, args)
        } catch let refusal as Refusal {
            throw refusal
        } catch let error as HubError {
            throw Refusal(message: error.message, code: error.error)
        }
    }

    private func answering(_ method: String, _ args: [String: Any]) async throws -> Data {
        let collection = (args["collection"] as? String) ?? ""
        switch method {
        case "ready":
            let state = Self.object(try await call("GET", "state").data) as? [String: Any] ?? [:]
            known = state["version"] as? Int ?? 0
            bridged = true
            return Self.json([
                "page": ["id": page.id, "name": page.name, "version": page.version],
                "me": ["name": names(me)],
                "state": state["state"] ?? NSNull(),
                "version": known,
                "preferences": preferences,
            ] as [String: Any])
        case "net.fetch":
            return try await network.answer(args)
        case "remote.call":
            return try await remote.answer((args["action"] as? String) ?? "", (args["args"] as? [String: Any]) ?? [:])
        case "update":
            let answer = try await call("PUT", "state", body: ["state": args["state"] ?? NSNull(), "if_version": args["version"] as? Int ?? 0])
            let body = Self.object(answer.data) as? [String: Any] ?? [:]
            if answer.status == 409 {
                // Someone wrote first: not ok, with the latest.
                let latest = body["latest"] as? [String: Any] ?? [:]
                let version = latest["version"] as? Int ?? known
                known = max(known, version)
                return Self.json(["ok": false, "state": latest["state"] ?? NSNull(), "version": version] as [String: Any])
            }
            let version = body["version"] as? Int ?? known
            known = max(known, version)
            return Self.json(["ok": true, "state": body["state"] ?? NSNull(), "version": version] as [String: Any])
        case "send":
            return Self.json(["id": try await send(((args["text"] as? String) ?? "").trimmingCharacters(in: .whitespacesAndNewlines))])
        case "storage.get":
            return Self.json(stored()[(args["key"] as? String) ?? ""] ?? NSNull())
        case "storage.set":
            var all = stored()
            let key = (args["key"] as? String) ?? ""
            if let value = args["value"], !(value is NSNull) { all[key] = value } else { all[key] = nil }
            let raw = Self.json(all)
            guard raw.count <= 1 << 20 else {
                throw Refusal(message: L("这个网页的存储满了（1 MB）", "This page's storage is full (1 MB)"), code: "too_large")
            }
            storage.set(raw, forKey: storageKey)
            return Self.json(true)
        case "data.list":
            let answer = try await call("GET", "data", query: collection.isEmpty ? [:] : ["collection": collection])
            if let seq = (Self.object(answer.data) as? [String: Any])?["seq"] as? Int { dataSeq = max(dataSeq, seq) }
            return answer.data
        case "data.get":
            return try await call("GET", "data/\(Self.segment(collection))/\(Self.segment((args["id"] as? String) ?? ""))").data
        case "data.put":
            let id = (args["id"] as? String) ?? ""
            var body: [String: Any] = ["data": args["data"] ?? [String: Any]()]
            if let version = args["ifVersion"] as? Int { body["if_version"] = version }
            let answer = id.isEmpty
                ? try await call("POST", "data/\(Self.segment(collection))", body: body)
                : try await call("PUT", "data/\(Self.segment(collection))/\(Self.segment(id))", body: body)
            if answer.status == 409 {
                let latest = (Self.object(answer.data) as? [String: Any])?["latest"]
                throw Refusal(
                    message: L("别人先改了这一条", "Someone changed this record first"), code: "conflict", latest: latest.map(Self.json))
            }
            return answer.data
        case "data.remove":
            let version = args["ifVersion"] as? Int ?? 0
            _ = try await call(
                "DELETE", "data/\(Self.segment(collection))/\(Self.segment((args["id"] as? String) ?? ""))",
                query: version > 0 ? ["if_version": String(version)] : [:])
            return Self.json(true)
        case "data.watch":
            if dataSeq == 0, let seq = (Self.object(try await call("GET", "data").data) as? [String: Any])?["seq"] as? Int { dataSeq = seq }
            watching = true
            return Self.json(true)
        case "download":
            let name = (args["name"] as? String).flatMap { $0.isEmpty ? nil : $0 } ?? "download"
            guard let data = Data(base64Encoded: (args["base64"] as? String) ?? "") else {
                throw Refusal(message: L("文件内容读不出来", "The file could not be read"), code: "invalid_argument")
            }
            await doors.save(name, data, (args["type"] as? String) ?? "")
            return Self.json(true)
        default:
            throw Refusal(message: "myco: no such call: \(method)", code: "not_found")
        }
    }

    public func setPreferences(language: String, theme: String) {
        preferences = ["language": language == "zh" ? "zh" : "en", "theme": theme == "dark" ? "dark" : "light"]
    }
    public func cancelNetwork() { network.cancel() }
    public func resetRemote() { remote.resetNow() }
    public func closeRemote() { remote.closeNow() }
    public func suspendRemote() { remote.suspendNow() }
    public func resumeRemote() { remote.resume() }

    private func stored() -> [String: Any] {
        storage.data(forKey: storageKey).flatMap { try? JSONSerialization.jsonObject(with: $0) as? [String: Any] } ?? [:]
    }

    /// A task to the page's agent, as me: asked once a page, at most five
    /// a minute, with where it came from under it.
    private func send(_ text: String) async throws -> String {
        guard !text.isEmpty else { throw Refusal(message: L("消息是空的", "The message is empty"), code: "invalid_argument") }
        let to = page.publisher
        guard !to.isEmpty, to != me else {
            throw Refusal(message: L("不知道这个网页是哪个 agent 发布的", "Which agent published this page is not known"), code: "invalid_state")
        }
        sends = sends.filter { Date().timeIntervalSince($0) < 60 }
        guard sends.count < 5 else { throw Refusal(message: L("发得太快了：一分钟最多 5 条", "Too many: at most 5 a minute"), code: "rate_limited") }
        if !allowedToSend {
            guard await doors.askToSend(text, names(to)) else {
                throw Refusal(message: L("你没有允许这个网页发消息", "You did not let this page send messages"), code: "denied")
            }
            allowedToSend = true
        }
        sends.append(Date())
        let body = text + "\n\n" + L("（来自网页「\(page.name)」\(page.url ?? "")，状态第 \(known) 版）", "(from the page \"\(page.name)\" \(page.url ?? ""), state version \(known))")
        return try await doors.send(to, body, page.project)
    }

    /// What others changed since the page last heard, as events to hand
    /// it (JSON): its state (once it is bridged), and its records (once it
    /// listens). Nothing when nothing changed, or the workspace is away.
    public func poll() async -> [Data] {
        var events: [Data] = []
        if bridged, let answer = try? await call("GET", "state"),
           let state = Self.object(answer.data) as? [String: Any], let version = state["version"] as? Int, version > known {
            known = version
            let by = (state["updated_by"] as? String).map(names) ?? ""
            events.append(Self.json(["myco": "change", "state": state["state"] ?? NSNull(), "version": version, "by": by] as [String: Any]))
        }
        if watching, let answer = try? await call("GET", "changes", query: ["after": String(dataSeq)]),
           let feed = Self.object(answer.data) as? [String: Any] {
            let changes = (feed["changes"] as? [[String: Any]]) ?? []
            dataSeq = max(dataSeq, feed["seq"] as? Int ?? dataSeq)
            let reset = feed["reset"] as? Bool ?? false
            if !changes.isEmpty || reset {
                let named = changes.map { change -> [String: Any] in
                    var one = change
                    if let by = change["by"] as? String { one["by"] = names(by) }
                    return one
                }
                events.append(Self.json(["myco": "data", "changes": named, "reset": reset] as [String: Any]))
            }
        }
        return events
    }
}
