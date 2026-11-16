import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// Voice settings are the person's: kept on the workspaces, newest wins.
struct VoiceSharingTests {
    static let now = Date(timeIntervalSince1970: 1_800_000_000)

    static func local(model: String = "fast-1", at: Date? = nil, share: Bool = true) -> VoiceSharing.Local {
        VoiceSharing.Local(
            settings: VoiceSettings(baseURL: "https://api.example.com/v1", model: model, speaker: "zh_x", rate: 10, share: share, updatedAt: at),
            modelKey: "sk-model", speechKeys: ["doubao": "tts-key", "none": ""])
    }

    @Test func whatIsSetHereGoesToEveryWorkspaceThatCanKeepIt() async {
        let mine = FakeHub(scenario: .empty), team = FakeHub(scenario: .empty), old = FakeHub(scenario: .empty), away = FakeHub(scenario: .empty)
        await old.setTooOldForVoice(true)
        await away.setUnreachable(true)
        // Set up before settings were shared: they count as changed now.
        let (after, states) = await VoiceSharing.sync(
            Self.local(), with: [("mine", mine), ("team", team), ("old", old), ("away", away)], now: Self.now)
        #expect(after.settings.updatedAt == Self.now)
        #expect(states == ["mine": .synced, "team": .synced, "old": .unsupported, "away": .unreachable])
        for hub in [mine, team] {
            let kept = await hub.keptVoice()
            #expect(kept?.llmBaseUrl == "https://api.example.com/v1")
            #expect(kept?.llmModel == "fast-1")
            #expect(kept?.llmKey == "sk-model")
            #expect(kept?.ttsProvider == "doubao")
            #expect(kept?.ttsSpeaker == "zh_x")
            #expect(kept?.ttsRate == 10)
            #expect(kept?.ttsKeys == ["doubao": "tts-key"])
            #expect(kept?.updatedAt == Self.now)
        }
        #expect(await old.keptVoice() == nil)
    }

    @Test func whatAnotherDeviceSetSinceIsTakenAndPassedOn() async {
        let mine = FakeHub(scenario: .empty), team = FakeHub(scenario: .empty)
        let earlier = Self.now.addingTimeInterval(-3600)
        let desktop = VoiceDocument(
            llmBaseUrl: "https://api.other.com/v1", llmModel: "fast-2", llmKey: "sk-desktop", ttsProvider: "doubao",
            ttsSpeaker: "", ttsRate: 0, ttsKeys: nil, updatedAt: Self.now)
        await mine.setVoice(desktop)
        let (after, states) = await VoiceSharing.sync(Self.local(at: earlier), with: [("mine", mine), ("team", team)], now: Self.now)
        // The desktop's are this phone's now, keys too; a key it took away is gone here.
        #expect(after.settings.baseURL == "https://api.other.com/v1")
        #expect(after.settings.model == "fast-2")
        #expect(after.settings.speaker.isEmpty)
        #expect(after.settings.updatedAt == Self.now)
        #expect(after.modelKey == "sk-desktop")
        #expect(after.speechKeys.isEmpty)
        // Sharing is this device's own choice.
        #expect(after.settings.share)
        #expect(states == ["mine": .synced, "team": .synced])
        // The workspace that was behind has them too.
        #expect(await team.keptVoice() == desktop)
        // Nothing new: nothing changes.
        let (again, _) = await VoiceSharing.sync(after, with: [("mine", mine), ("team", team)], now: Self.now.addingTimeInterval(60))
        #expect(again == after)
    }

    @Test func whatIsNewerHereWins() async {
        let hub = FakeHub(scenario: .empty)
        await hub.setVoice(VoiceDocument(llmModel: "old", llmKey: "sk-old", updatedAt: Self.now.addingTimeInterval(-600)))
        let mine = Self.local(model: "newest", at: Self.now)
        let (after, _) = await VoiceSharing.sync(mine, with: [("hub", hub)], now: Self.now.addingTimeInterval(5))
        #expect(after == mine)
        #expect(await hub.keptVoice()?.llmModel == "newest")
        #expect(await hub.keptVoice()?.llmKey == "sk-model")
    }

    @Test func nothingLeavesThisDeviceWhenSharingIsOff() async {
        let hub = FakeHub(scenario: .empty)
        await hub.setVoice(VoiceDocument(llmModel: "theirs", updatedAt: Self.now))
        let mine = Self.local(at: Self.now.addingTimeInterval(-60), share: false)
        let (after, states) = await VoiceSharing.sync(mine, with: [("hub", hub)], now: Self.now)
        #expect(after == mine)
        #expect(states.isEmpty)
        #expect(await hub.keptVoice()?.llmModel == "theirs")
        // And with nothing set here and nothing there, nothing is made up.
        let empty = FakeHub(scenario: .empty)
        let (none, _) = await VoiceSharing.sync(VoiceSharing.Local(), with: [("hub", empty)], now: Self.now)
        #expect(none.settings.updatedAt == nil)
        #expect(await empty.keptVoice() == nil)
    }

    @Test func theWorkspacesWordsAreRead() throws {
        // What a hub answers (Go's JSON): snake_case, and the year 1 for "never".
        let never = try HubJSON.decoder().decode(VoiceDocument.self, from: Data(#"{"updated_at":"0001-01-01T00:00:00Z"}"#.utf8))
        #expect(never.neverSet)
        let json = #"{"llm_base_url":"https://api.example.com/v1","llm_model":"fast-1","llm_key":"sk","tts_provider":"doubao","tts_speaker":"zh_x","tts_rate":-10,"tts_keys":{"doubao":"k"},"updated_at":"2026-11-15T12:00:00.123456789Z"}"#
        let document = try HubJSON.decoder().decode(VoiceDocument.self, from: Data(json.utf8))
        #expect(!document.neverSet)
        #expect(document.llmBaseUrl == "https://api.example.com/v1")
        #expect(document.ttsRate == -10)
        #expect(document.ttsKeys == ["doubao": "k"])
        // And written back the way the hub reads it.
        let written = try JSONSerialization.jsonObject(with: HubJSON.encoder().encode(document)) as? [String: Any]
        #expect(written?["llm_base_url"] as? String == "https://api.example.com/v1")
        #expect((written?["tts_keys"] as? [String: String]) == ["doubao": "k"])
        #expect((written?["updated_at"] as? String)?.hasPrefix("2026-11-15T12:00:00") == true)
    }

    @Test func settingsSavedBeforeServicesExistedStillRead() throws {
        let old = #"{"baseURL":"https://api.example.com/v1","model":"m","speaker":"","rate":0}"#
        let settings = try JSONDecoder().decode(VoiceSettings.self, from: Data(old.utf8))
        #expect(settings.share)
        #expect(settings.updatedAt == nil)
        #expect(settings.service == .doubao)
        #expect(settings.speakerInUse == SpeechService.doubao.defaultSpeaker)
        // Each service's key has its own place; the first one keeps its old one.
        #expect(VoiceSettings.speechKey(of: "") == "voice:tts")
        #expect(VoiceSettings.speechKey(of: "doubao") == "voice:tts")
        #expect(VoiceSettings.speechKey(of: "other") == "voice:tts:other")
        #expect(SpeechService.named("nope") == nil)
    }

    /// What a device holds, as `AppModel` does.
    @MainActor final class Kept {
        var local: VoiceSharing.Local
        init(_ local: VoiceSharing.Local) { self.local = local }

        func look(at hub: FakeHub, now: Date) async {
            let before = local
            let (next, _) = await VoiceSharing.sync(before, with: [("hub", hub)], now: now)
            if next != before { local = next }
        }
    }

    @MainActor @Test func aLookThatBeganBeforeASaveDoesNotPutBackWhatItReplaced() async {
        // The workspace is slow, and holds what the desktop set a while ago.
        let hub = FakeHub(scenario: .empty, latency: .milliseconds(200))
        await hub.setVoice(VoiceDocument(llmModel: "desktop", llmKey: "sk-desktop", updatedAt: Self.now.addingTimeInterval(-600)))
        let kept = Kept(Self.local(model: "old", at: Self.now.addingTimeInterval(-3600)))
        let turns = VoiceSharing.Turns()

        // The app opens: a look at the workspace begins...
        let look = Task { @MainActor in
            await turns.take { await kept.look(at: hub, now: Self.now) }
        }
        try? await Task.sleep(for: .milliseconds(40))
        // ...and before it has its answer, the person saves a change.
        await turns.take {
            kept.local.settings.model = "phone"
            kept.local.settings.updatedAt = VoiceSharing.stamp(after: kept.local.settings.updatedAt, now: Self.now)
            await kept.look(at: hub, now: Self.now)
        }
        await look.value
        #expect(kept.local.settings.model == "phone")
        #expect(await hub.keptVoice()?.llmModel == "phone")
    }

    @Test func aChangeIsLaterThanTheOneItReplaces() {
        // Whole seconds, as the desktop counts and as a workspace gives back.
        let at = Date(timeIntervalSince1970: 1_800_000_000.75)
        #expect(VoiceSharing.stamp(after: nil, now: at) == Self.now)
        #expect(VoiceSharing.stamp(after: Self.now.addingTimeInterval(-5), now: at) == Self.now)
        // Twice within a second, or after settings from a clock a little ahead.
        #expect(VoiceSharing.stamp(after: Self.now, now: at) == Self.now.addingTimeInterval(1))
        #expect(VoiceSharing.stamp(after: Self.now.addingTimeInterval(90), now: at) == Self.now.addingTimeInterval(91))
    }
}
