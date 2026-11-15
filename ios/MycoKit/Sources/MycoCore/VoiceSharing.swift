import Foundation
import MycoAPI

/// Voice settings are the person's, not this phone's: they are kept on
/// the workspaces this device joined (`GET`/`PUT /v1/console/voice`), so
/// what was set on the desktop app is here without typing it again, and
/// the other way round. The newest change wins, by the time the person
/// made it. The desktop app's rule, `internal/console/voice_sync.go`.
public enum VoiceSharing {
    /// What this device has: the choices, and the keys from its keychain.
    public struct Local: Sendable, Hashable {
        public var settings: VoiceSettings
        public var modelKey: String
        /// The speech services' keys, by service.
        public var speechKeys: [String: String]

        public init(settings: VoiceSettings = VoiceSettings(), modelKey: String = "", speechKeys: [String: String] = [:]) {
            self.settings = settings
            self.modelKey = modelKey
            self.speechKeys = speechKeys.filter { !$0.value.isEmpty }
        }

        /// Nothing was ever set here.
        var isEmpty: Bool {
            settings.baseURL.isEmpty && settings.model.isEmpty && settings.speaker.isEmpty && settings.rate == 0
                && modelKey.isEmpty && speechKeys.isEmpty
        }

        /// As the workspace keeps it.
        func document(at: Date) -> VoiceDocument {
            VoiceDocument(
                llmBaseUrl: settings.baseURL, llmModel: settings.model, llmKey: modelKey,
                ttsProvider: settings.provider.isEmpty ? SpeechService.standard : settings.provider,
                ttsSpeaker: settings.speaker, ttsRate: settings.rate, ttsKeys: speechKeys.isEmpty ? nil : speechKeys, updatedAt: at)
        }

        /// A workspace's settings, made this device's. Sharing is this
        /// device's own choice and stays.
        func adopting(_ document: VoiceDocument) -> Local {
            var next = self
            next.settings.baseURL = document.llmBaseUrl ?? ""
            next.settings.model = document.llmModel ?? ""
            next.settings.provider = document.ttsProvider ?? ""
            next.settings.speaker = document.ttsSpeaker ?? ""
            next.settings.rate = document.ttsRate ?? 0
            next.settings.updatedAt = document.updatedAt
            next.modelKey = document.llmKey ?? ""
            next.speechKeys = (document.ttsKeys ?? [:]).filter { !$0.value.isEmpty }
            return next
        }
    }

    /// How one workspace stands with the settings.
    public enum State: Sendable, Hashable {
        case synced
        /// Its hub is from before settings were kept.
        case unsupported
        case unreachable
    }

    /// Brings this device and its workspaces to the newest settings: reads
    /// every workspace's, takes the newest (a workspace's, when newer than
    /// this device's), and sends it to those that are behind. It never
    /// fails: a workspace that cannot be reached is tried again next time.
    /// - Returns: what this device has then, and how each workspace stands.
    public static func sync(
        _ local: Local, with hubs: [(id: String, hub: any HubAPI)], now: Date = Date()
    ) async -> (local: Local, states: [String: State]) {
        guard local.settings.share, !hubs.isEmpty else { return (local, [:]) }
        // What each workspace holds, all asked at once.
        let held = await withTaskGroup(of: (String, Result<VoiceDocument, Error>).self) { group in
            for (id, hub) in hubs {
                group.addTask {
                    do {
                        return (id, .success(try await hub.voiceSettings()))
                    } catch {
                        return (id, .failure(error))
                    }
                }
            }
            var answers: [String: Result<VoiceDocument, Error>] = [:]
            for await (id, answer) in group { answers[id] = answer }
            return answers
        }
        var mine = local
        var newest: VoiceDocument?
        for case .success(let document) in held.values where !document.neverSet {
            if document.updatedAt > (newest?.updatedAt ?? mine.settings.updatedAt ?? .distantPast) { newest = document }
        }
        if let newest {
            mine = mine.adopting(newest)
        } else if mine.settings.updatedAt == nil, !mine.isEmpty {
            // Set up before settings were shared: they count as changed now.
            mine.settings.updatedAt = now
        }
        var states: [String: State] = [:]
        var behind: [(id: String, hub: any HubAPI)] = []
        for (id, hub) in hubs {
            switch held[id] {
            case .success(let document)?:
                states[id] = .synced
                if let at = mine.settings.updatedAt, document.neverSet || document.updatedAt < at { behind.append((id, hub)) }
            case .failure(let error as HubError)? where error.error == "not_found":
                states[id] = .unsupported
            default:
                states[id] = .unreachable
            }
        }
        if let at = mine.settings.updatedAt, !behind.isEmpty {
            let document = mine.document(at: at)
            let failed = await withTaskGroup(of: String?.self) { group in
                for (id, hub) in behind {
                    group.addTask { (try? await hub.putVoiceSettings(document)) == nil ? id : nil }
                }
                var ids: [String] = []
                for await id in group { if let id { ids.append(id) } }
                return ids
            }
            for id in failed { states[id] = .unreachable }
        }
        return (mine, states)
    }
}
