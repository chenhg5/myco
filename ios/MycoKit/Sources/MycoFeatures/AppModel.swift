import AVFoundation
import SwiftUI
import Observation
import MycoAPI
import MycoCore
import MycoStore
import MycoUI

/// The app as a whole: the workspaces this device joined, which one is on
/// screen, and where in it.
@MainActor
@Observable
public final class AppModel {
    /// The workspace on screen.
    public private(set) var session: WorkspaceSession?
    /// Every workspace joined, in the order they were joined.
    public private(set) var workspaces: [WorkspaceRecord] = []
    /// False until the app has looked for workspaces joined earlier.
    public private(set) var started = false
    public let router = Router()

    /// A session per workspace joined, kept while the app runs: switching
    /// back to one finds it as it was, and the ones off screen are looked
    /// at now and then, to say when something new came there.
    private var sessions: [String: WorkspaceSession] = [:]

    /// What the app needs from outside, put together at its entry point.
    public struct Doors: Sendable {
        /// Makes the demo workspace (no hub behind it).
        public var demo: @MainActor @Sendable () -> WorkspaceSession
        /// The workspaces this device joined; nil when none can be (a preview).
        public var book: (any WorkspaceBook)?
        /// Makes the session of a workspace joined.
        public var session: @MainActor @Sendable (WorkspaceRecord, any HubAPI) -> WorkspaceSession
        /// Forgets what was kept on this device for a workspace that was left.
        public var forget: @MainActor @Sendable (String) async -> Void
        /// Something to hear speech with: the system's recognition, or a
        /// script in tests.
        public var speech: @MainActor @Sendable () -> any SpeechListener
        /// What shows notifications: the system, or a list in tests.
        public var notifier: @MainActor @Sendable () -> any Notifier
        /// The app left the front: the place to ask the system for a
        /// moment in the background later.
        public var leftFront: @MainActor @Sendable () -> Void
        /// Where the voice's keys are kept: the keychain, or memory in
        /// the demo and in tests.
        public var secrets: any SecretStore
        /// A voice call's microphone, model and voice from outside (a
        /// script, where nobody speaks and there are no keys); nil makes
        /// them from what is set in Settings.
        public var voice: (@MainActor @Sendable () -> VoiceStandIn)?

        public init(
            demo: @escaping @MainActor @Sendable () -> WorkspaceSession,
            book: (any WorkspaceBook)? = nil,
            session: @escaping @MainActor @Sendable (WorkspaceRecord, any HubAPI) -> WorkspaceSession = { record, hub in
                WorkspaceSession(id: record.id, name: Workspaces.displayName(of: record), hub: hub)
            },
            forget: @escaping @MainActor @Sendable (String) async -> Void = { _ in },
            speech: @escaping @MainActor @Sendable () -> any SpeechListener = { AppleSpeechListener() },
            notifier: @escaping @MainActor @Sendable () -> any Notifier = { SystemNotifier() },
            leftFront: @escaping @MainActor @Sendable () -> Void = {},
            secrets: any SecretStore = MemoryStore(),
            voice: (@MainActor @Sendable () -> VoiceStandIn)? = nil
        ) {
            self.demo = demo
            self.book = book
            self.session = session
            self.forget = forget
            self.speech = speech
            self.notifier = notifier
            self.leftFront = leftFront
            self.secrets = secrets
            self.voice = voice
        }
    }

    /// What stands in for a call's microphone, model and voice.
    public struct VoiceStandIn {
        public var audio: any VoiceAudio
        public var model: any VoiceModel
        public var speech: any VoiceSpeech

        public init(audio: any VoiceAudio, model: any VoiceModel, speech: any VoiceSpeech) {
            self.audio = audio
            self.model = model
            self.speech = speech
        }
    }

    /// Why joining did not work, in words for the person.
    public struct JoinFailure: Error, Sendable {
        public var message: String
        /// What to do about it, and the raw reason, when there are any.
        public var advice: String?
        public var detail: String?

        public init(message: String, advice: String? = nil, detail: String? = nil) {
            self.message = message
            self.advice = advice
            self.detail = detail
        }
    }

    private let doors: Doors
    /// The id the demo workspace goes by: it is never "left" on a hub.
    public static let demoID = "demo"

    /// A listener for one input's microphone.
    public func speechListener() -> any SpeechListener {
        doors.speech()
    }

    /// - Parameter session: a workspace to show at once (the demo, in tests);
    ///   nil looks for the ones joined earlier.
    public init(session: WorkspaceSession? = nil, doors: Doors) {
        self.session = session
        self.doors = doors
        started = session != nil
        let notifier = doors.notifier()
        self.notifier = notifier
        let settings = Self.savedNotifications()
        notifications = settings
        desk = NotificationDesk(notifier: notifier, settings: settings)
        (notifier as? SystemNotifier)?.onOpen = { [weak self] workspace, page in
            Task { await self?.open(workspace: workspace, page: page) }
        }
        if let session { watch(session) }
        Task { await readVoiceKeys() }
    }

    // MARK: Voice calls

    /// What the person chose for voice calls (the keys are in the keychain).
    public private(set) var voice: VoiceSettings = AppModel.savedVoice()
    /// Whether the model's key and the chosen speech service's are on
    /// this device.
    public private(set) var voiceHasModelKey = false
    public private(set) var voiceHasSpeechKey = false
    /// How each workspace stood with the settings kept on it, at the last
    /// look: its name, and whether it has them.
    public private(set) var voiceSync: [VoiceSyncLine] = []
    /// The voice being tried in Settings, while it plays.
    @ObservationIgnored private var trial: AVAudioPlayer?

    /// A save and a look at the workspaces, one at a time.
    @ObservationIgnored private let voiceTurns = VoiceSharing.Turns()

    public struct VoiceSyncLine: Identifiable, Hashable, Sendable {
        public var id: String
        public var name: String
        public var state: VoiceSharing.State
    }

    // MARK: The network proxy

    /// How the app reaches its workspaces and the voice services: as the
    /// system has it, directly, or through a proxy set here.
    public private(set) var proxy: ProxySettings = AppModel.savedProxy()
    /// A password for the proxy is in the keychain.
    public private(set) var proxyHasPassword = false

    private static let proxyKey = "myco-network-proxy"

    private static func savedProxy() -> ProxySettings {
        guard let data = UserDefaults.standard.data(forKey: proxyKey),
              let settings = try? JSONDecoder().decode(ProxySettings.self, from: data)
        else { return ProxySettings() }
        return settings
    }

    /// Puts the saved setting in force: at launch, before anything is
    /// asked of a workspace, and when it is saved.
    private func applyProxy() async {
        let password = await secret(ProxySettings.passwordKey)
        proxyHasPassword = !password.isEmpty
        AppNetwork.shared.apply(proxy, password: password)
    }

    /// Keeps a proxy setting and puts it in force at once. A password
    /// left empty stays as it was; `forgetPassword`, or no user name,
    /// takes the saved one away.
    public func saveProxy(_ settings: ProxySettings, password: String = "", forgetPassword: Bool = false) async {
        var settings = settings
        settings.host = settings.host.trimmingCharacters(in: .whitespaces)
        settings.user = settings.user.trimmingCharacters(in: .whitespaces)
        guard settings.problem == nil else { return }
        proxy = settings
        UserDefaults.standard.set(try? JSONEncoder().encode(settings), forKey: Self.proxyKey)
        if forgetPassword || settings.user.isEmpty {
            try? await doors.secrets.setSecret(nil, for: ProxySettings.passwordKey)
        } else if !password.isEmpty {
            try? await doors.secrets.setSecret(password, for: ProxySettings.passwordKey)
        }
        await applyProxy()
    }

    /// One request through a setting, before it is saved: to a workspace
    /// that is not on the local network (those go direct, whatever is
    /// set), else to GitHub. With the password typed, else the one kept.
    public func tryProxy(_ settings: ProxySettings, password: String) async -> ProxyTrial {
        var password = password
        if password.isEmpty, !settings.user.isEmpty { password = await secret(ProxySettings.passwordKey) }
        var target = URL(string: "https://github.com/")!
        if let away = workspaces.first(where: { !ProxySettings.isLocal($0.url.host() ?? "") }) { target = away.url }
        // (`-myco-proxy-target <url>`: where a try goes, for tests.)
        if let given = UserDefaults.standard.string(forKey: "myco-proxy-target"), let url = URL(string: given) { target = url }
        return await ProxyTrial.run(settings, password: password, to: target)
    }

    private static let voiceKey = "myco-voice"

    private static func savedVoice() -> VoiceSettings {
        guard let data = UserDefaults.standard.data(forKey: voiceKey),
              let settings = try? JSONDecoder().decode(VoiceSettings.self, from: data)
        else { return VoiceSettings() }
        return settings
    }

    /// A call can start: the model is named and its key is here.
    public var voiceReady: Bool {
        doors.voice != nil || (voice.namesModel && voiceHasModelKey)
    }

    private func secret(_ name: String) async -> String {
        (try? await doors.secrets.secret(for: name)) ?? ""
    }

    private func readVoiceKeys() async {
        voiceHasModelKey = !(await secret(VoiceSettings.modelKey)).isEmpty
        voiceHasSpeechKey = !(await secret(VoiceSettings.speechKey(of: voice.provider))).isEmpty
    }

    /// What this device has for voice: the choices and every key.
    private func localVoice() async -> VoiceSharing.Local {
        var keys: [String: String] = [:]
        for service in SpeechService.all {
            keys[service.id] = await secret(VoiceSettings.speechKey(of: service.id))
        }
        return VoiceSharing.Local(settings: voice, modelKey: await secret(VoiceSettings.modelKey), speechKeys: keys)
    }

    /// Makes what is given this device's: the choices are kept, the keys
    /// go to the keychain (one that is gone is removed).
    private func keepVoice(_ local: VoiceSharing.Local) async {
        voice = local.settings
        UserDefaults.standard.set(try? JSONEncoder().encode(local.settings), forKey: Self.voiceKey)
        try? await doors.secrets.setSecret(local.modelKey.isEmpty ? nil : local.modelKey, for: VoiceSettings.modelKey)
        for service in SpeechService.all {
            let key = local.speechKeys[service.id] ?? ""
            try? await doors.secrets.setSecret(key.isEmpty ? nil : key, for: VoiceSettings.speechKey(of: service.id))
        }
        await readVoiceKeys()
    }

    /// Keeps what was set for voice, as changed now. A key left empty
    /// stays as it was; `forget` takes saved keys away. Then the person's
    /// other devices get it from the workspaces.
    public func saveVoice(_ settings: VoiceSettings, modelKey: String = "", speechKey: String = "", forgetModelKey: Bool = false, forgetSpeechKey: Bool = false) async {
        // In turn with a look at the workspaces: one that began before
        // this save cannot put back what it replaces.
        await voiceTurns.take { [self] in
            await saveVoiceNow(settings, modelKey: modelKey, speechKey: speechKey, forgetModelKey: forgetModelKey, forgetSpeechKey: forgetSpeechKey)
        }
    }

    private func saveVoiceNow(_ settings: VoiceSettings, modelKey: String, speechKey: String, forgetModelKey: Bool, forgetSpeechKey: Bool) async {
        var local = await localVoice()
        var settings = settings
        settings.baseURL = settings.baseURL.trimmingCharacters(in: .whitespacesAndNewlines)
        settings.model = settings.model.trimmingCharacters(in: .whitespacesAndNewlines)
        settings.speaker = settings.speaker.trimmingCharacters(in: .whitespacesAndNewlines)
        settings.rate = min(100, max(-50, settings.rate))
        if SpeechService.named(settings.provider) == nil { settings.provider = "" }
        let modelKey = modelKey.trimmingCharacters(in: .whitespacesAndNewlines)
        let speechKey = speechKey.trimmingCharacters(in: .whitespacesAndNewlines)
        let service = settings.service.id
        if forgetModelKey { local.modelKey = "" } else if !modelKey.isEmpty { local.modelKey = modelKey }
        if forgetSpeechKey { local.speechKeys[service] = nil } else if !speechKey.isEmpty { local.speechKeys[service] = speechKey }
        // Only a real change is a change: looking at the page is not.
        var before = local.settings
        before.updatedAt = settings.updatedAt
        let changed = before != settings || !modelKey.isEmpty || !speechKey.isEmpty || forgetModelKey || forgetSpeechKey
        if changed { settings.updatedAt = VoiceSharing.stamp(after: local.settings.updatedAt) }
        local.settings = settings
        await keepVoice(local)
        await syncVoiceNow()
    }

    /// Says one line in the voice chosen, to hear it before it is kept:
    /// with the key typed, else the one this device has. (The desktop
    /// app's "Try it".)
    public func tryVoice(_ settings: VoiceSettings, speechKey typed: String) async throws {
        let service = settings.service
        var key = typed.trimmingCharacters(in: .whitespacesAndNewlines)
        if key.isEmpty { key = await secret(VoiceSettings.speechKey(of: service.id)) }
        let stand = doors.voice?()
        guard stand != nil || !key.isEmpty else { throw VoiceFailure(L("先填 API Key", "Type the API key first")) }
        let speech = stand?.speech ?? service.speech(key: key, settings: settings)
        let heard = Heard()
        try await speech.speak(VoiceSample.line(for: settings.speakerInUse, in: service, language: AppLanguage.current)) { samples, rate in
            heard.samples.append(samples)
            heard.rate = rate
        }
        // (A script has no sound to play.)
        guard stand == nil else { return }
        guard !heard.samples.isEmpty, heard.rate > 0 else {
            throw VoiceFailure(L("语音合成服务没有返回声音", "The speech service sent no audio"))
        }
        trial?.stop()
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .default)
        try? AVAudioSession.sharedInstance().setActive(true)
        let player = try AVAudioPlayer(data: VoiceSample.wav(heard.samples, rate: heard.rate))
        trial = player
        player.play()
    }

    /// What a voice being tried has said so far.
    @MainActor private final class Heard {
        var samples = Data()
        var rate = 0
    }

    /// Brings this device and its workspaces to the newest voice settings
    /// (`VoiceSharing`): what was set on another device since is taken,
    /// and what was set here goes to the workspaces that are behind.
    public func syncVoice() async {
        await voiceTurns.take { [self] in await syncVoiceNow() }
    }

    private func syncVoiceNow() async {
        guard doors.voice == nil else { return }
        var all = Array(sessions.values)
        if let current = session, sessions[current.id] == nil, current.id != Self.demoID { all.append(current) }
        let names = Dictionary(all.map { ($0.id, $0.name) }, uniquingKeysWith: { first, _ in first })
        let local = await localVoice()
        let (next, states) = await VoiceSharing.sync(local, with: all.map { (id: $0.id, hub: $0.hub) })
        if next != local { await keepVoice(next) }
        voiceSync = states.map { VoiceSyncLine(id: $0.key, name: names[$0.key] ?? $0.key, state: $0.value) }
            .sorted { $0.name < $1.name }
    }

    /// A voice call with the agent of a conversation: the phone's
    /// microphone, the model set in Settings, and the speech service
    /// chosen there (the phone's own voice when there is no key for it).
    /// Nil when voice is not set up yet.
    public func call(in conversation: ConversationModel, of session: WorkspaceSession) async -> VoiceCall? {
        let hub = ConversationVoice(session: session, model: conversation)
        if let stand = doors.voice?() {
            return VoiceCall(.init(audio: stand.audio, model: stand.model, speech: stand.speech, hub: hub))
        }
        var key = await secret(VoiceSettings.modelKey)
        if !voice.namesModel || key.isEmpty {
            // Maybe set up on another device since.
            await syncVoice()
            key = await secret(VoiceSettings.modelKey)
        }
        guard voice.namesModel, !key.isEmpty else { return nil }
        let speechKey = await secret(VoiceSettings.speechKey(of: voice.provider))
        let speech: any VoiceSpeech = speechKey.isEmpty
            ? SystemSpeech(language: .current) : voice.service.speech(key: speechKey, settings: voice)
        let chat = OpenAIChat(
            base: voice.baseURL, model: voice.model, key: key,
            session: "myco-voice-" + UUID().uuidString.replacingOccurrences(of: "-", with: "").lowercased())
        // The end of the last call with this agent goes into this one's
        // instructions, and this one's end is kept for the next: on this
        // device only.
        let memos = FileMemoStore.standard
        let memoKey = VoiceMemo.key(workspace: session.id, agent: conversation.agent)
        let language = AppLanguage.current
        return VoiceCall(.init(
            audio: AppleVoiceAudio(), model: chat, speech: speech, hub: hub, language: language,
            memo: VoiceMemos.memo(for: memoKey, in: memos)?.prompt(language: language) ?? "",
            ended: { history in VoiceMemos.keep(history, for: memoKey, in: memos) }))
    }

    // MARK: Notifications

    private let notifier: any Notifier
    private let desk: NotificationDesk
    /// What the person chose about being told of new messages.
    public private(set) var notifications: NotificationSettings
    /// The person turned notifications on here, but the system does not
    /// allow them: Settings is where that is changed.
    public private(set) var notificationsRefused = false
    /// The app is in front.
    public var active = true {
        didSet { if !active, oldValue { doors.leftFront() } }
    }
    /// A notification tapped before the workspaces were read.
    private var pendingOpen: (workspace: String, page: String)?

    private static let notificationsKey = "myco-notifications"

    private static func savedNotifications() -> NotificationSettings {
        guard let data = UserDefaults.standard.data(forKey: notificationsKey),
              let settings = try? JSONDecoder().decode(NotificationSettings.self, from: data)
        else { return NotificationSettings() }
        return settings
    }

    /// Changes what the person chose, and keeps it.
    public func setNotifications(_ change: (inout NotificationSettings) -> Void) {
        change(&notifications)
        desk.settings = notifications
        UserDefaults.standard.set(try? JSONEncoder().encode(notifications), forKey: Self.notificationsKey)
    }

    /// Turns notifications on (the system asks the person the first time)
    /// or off.
    public func setNotificationsEnabled(_ on: Bool) async {
        guard on else {
            notificationsRefused = false
            setNotifications { $0.enabled = false }
            return
        }
        let allowed = await notifier.authorized(asking: true)
        notificationsRefused = !allowed
        setNotifications { $0.enabled = allowed }
    }

    public func isMuted(_ party: String, in workspace: String) -> Bool {
        notifications.muted.contains(NotificationSettings.muteKey(workspace: workspace, party: party))
    }

    public func setMuted(_ muted: Bool, _ party: String, in workspace: String) {
        let key = NotificationSettings.muteKey(workspace: workspace, party: party)
        setNotifications { if muted { $0.muted.insert(key) } else { $0.muted.remove(key) } }
    }

    /// Has a session tell when it refreshed, to announce what is new there.
    private func watch(_ session: WorkspaceSession) {
        session.refreshed = { [weak self] session in
            Task { await self?.announce(session) }
        }
    }

    /// The conversation on screen now: nothing in it needs announcing.
    private var onScreen: (workspace: String, party: String)? {
        guard let session, router.sheet == nil else { return nil }
        let path: [Route]
        switch router.tab {
        case .chats: path = router.chatsPath
        case .projects: path = router.projectsPath
        case .members: path = router.membersPath
        }
        switch path.last {
        case .agent(let name, _):
            return (session.id, name)
        case .project(let name):
            return session.project(name).flatMap { session.coordinator(of: $0) }.map { (session.id, $0) }
        default:
            return nil
        }
    }

    private func announce(_ session: WorkspaceSession) async {
        desk.onScreen = onScreen
        desk.severalWorkspaces = workspaces.count > 1
        await desk.check(session, active: active)
        var unread = sessions.values.reduce(0) { $0 + $1.unreadCount }
        if let current = self.session, sessions[current.id] == nil { unread += current.unreadCount }
        await notifier.setBadge(notifications.enabled ? unread : 0)
    }

    /// A notification was tapped: its workspace comes on screen, on the
    /// conversation it was about.
    public func open(workspace: String, page: String) async {
        guard started else {
            pendingOpen = (workspace, page)
            return
        }
        if session?.id != workspace { await switchTo(workspace) }
        guard session?.id == workspace else { return }
        _ = router.open(spec: page)
    }

    /// What the system lets the app do now and then while it is not in
    /// front: look at every workspace once, and announce what is new.
    public func refreshInBackground() async {
        if !started { await loadWorkspaces() }
        var all = Array(sessions.values)
        if let current = session, sessions[current.id] == nil { all.append(current) }
        for one in all {
            await one.restore()
            await one.refresh()
            await announce(one)
        }
    }

    /// Opens the workspace that was on screen last, or leaves the welcome
    /// page up.
    public func start(restoring: Bool) async {
        guard !started else { return }
        // The proxy set in Settings is in force before anything is asked.
        await applyProxy()
        if restoring { await loadWorkspaces() }
        started = true
        // What was set for voice on another device: here without asking.
        Task { await syncVoice() }
        if let pending = pendingOpen {
            pendingOpen = nil
            await open(workspace: pending.workspace, page: pending.page)
        }
    }

    /// Reads the workspaces joined and shows the one chosen last.
    private func loadWorkspaces() async {
        guard let book = doors.book else { return }
        workspaces = await book.all()
        for record in workspaces where sessions[record.id] == nil {
            guard let hub = await book.hub(of: record.id) else { continue }
            let session = doors.session(record, hub)
            sessions[record.id] = session
            watch(session)
        }
        // One whose token is gone cannot be opened: it is not listed.
        workspaces = workspaces.filter { sessions[$0.id] != nil }
        let chosen = await book.selected()
        session = chosen.flatMap { sessions[$0] } ?? workspaces.first.flatMap { sessions[$0.id] }
    }

    public func enterDemo() {
        router.reset()
        let demo = doors.demo()
        watch(demo)
        session = demo
    }

    /// Joins the workspace a link leads to and shows it. The ones joined
    /// before stay.
    public func join(link: String) async throws {
        guard let book = doors.book else {
            throw JoinFailure(message: L("这里不能加入工作区。", "A workspace cannot be joined here."))
        }
        let record: WorkspaceRecord
        do {
            record = try await book.join(link: link)
        } catch JoinError.notALink {
            throw JoinFailure(message: L("这不是添加设备的链接。它应该像 https://你的工作区/c/myco_pair_…", "This is not an \"add a device\" link. It looks like https://your-workspace/c/myco_pair_…"))
        } catch JoinError.refused(let reason) {
            throw JoinFailure(message: L("工作区没有接受这个链接：\(reason)", "The workspace did not accept this link: \(reason)"))
        } catch JoinError.unreachable(let detail) {
            // Not a bad link: the workspace did not answer.
            throw JoinFailure(
                message: L("暂时连不上这个工作区，或者它响应太慢。", "Cannot reach this workspace right now, or it is answering too slowly."),
                advice: L("链接还没用掉，等一下再点「加入」。一直这样的话，看看手机的网络，或者到设置 ›「代理」里看看。",
                          "The link was not used up: tap Join again in a moment. If it keeps happening, check this phone's network, or look at Settings › Proxy."),
                detail: detail)
        }
        guard let hub = await book.hub(of: record.id) else {
            throw JoinFailure(message: L("加入了，但没能保存在这台设备上。", "Joined, but it could not be saved on this device."))
        }
        // Joined again: a new session, with the new sign-in.
        let joined = doors.session(record, hub)
        sessions[record.id] = joined
        watch(joined)
        workspaces = await book.all().filter { sessions[$0.id] != nil }
        await show(record.id)
    }

    /// Puts another workspace on screen.
    public func switchTo(_ id: String) async {
        guard id != session?.id, sessions[id] != nil else { return }
        await show(id)
    }

    private func show(_ id: String) async {
        guard let next = sessions[id] else { return }
        router.reset()
        session = next
        await doors.book?.select(id)
    }

    /// Gives a workspace another name on this device.
    public func rename(_ id: String, to name: String) async {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, let book = doors.book else { return }
        await book.rename(id, to: name)
        workspaces = await book.all().filter { sessions[$0.id] != nil }
        sessions[id]?.rename(name)
    }

    /// Leaves a workspace. When it was the one on screen, another takes
    /// its place; with none left, the welcome page.
    public func leave(_ id: String) async {
        if id == Self.demoID {
            router.reset()
            session = workspaces.first.flatMap { sessions[$0.id] }
            return
        }
        await doors.book?.leave(id)
        await doors.forget(id)
        desk.forget(id)
        sessions[id] = nil
        workspaces.removeAll { $0.id == id }
        guard session?.id == id else { return }
        router.reset()
        if let next = workspaces.first {
            await show(next.id)
        } else {
            session = nil
        }
    }

    public func leaveWorkspace() async {
        if let session { await leave(session.id) }
    }

    // MARK: The workspaces off screen

    /// What a workspace is called on this device.
    public func name(of record: WorkspaceRecord) -> String {
        sessions[record.id]?.name ?? Workspaces.displayName(of: record)
    }

    /// How many conversations of a workspace hold something unread.
    public func unread(in id: String) -> Int {
        sessions[id]?.unreadCount ?? 0
    }

    /// Whether a workspace could be reached the last time it was asked.
    public func problem(in id: String) -> String? {
        sessions[id]?.problem
    }

    /// Something new came in a workspace that is not on screen.
    public var unreadElsewhere: Bool {
        workspaces.contains { $0.id != session?.id && unread(in: $0.id) > 0 }
    }

    /// How often the workspaces off screen are looked at.
    public static let othersInterval: Duration = .seconds(60)

    /// Looks at the workspaces off screen once: what is new there shows as
    /// a dot on the way to them.
    public func refreshOthers() async {
        let others = workspaces.compactMap { $0.id == session?.id ? nil : sessions[$0.id] }
        await withTaskGroup(of: Void.self) { group in
            for other in others {
                group.addTask {
                    await other.restore()
                    await other.refresh()
                }
            }
        }
    }

    /// Keeps looking at them, while the app is in front.
    public func watchOthers() async {
        while !Task.isCancelled {
            await refreshOthers()
            try? await Task.sleep(for: Self.othersInterval)
        }
    }
}

/// The top of the app: the workspace on screen, or the welcome page.
public struct AppRootView: View {
    let model: AppModel
    /// A place to open at launch (see `Router.open(spec:)`).
    let initialRoute: String?
    /// Whether to open the workspace joined earlier.
    let restoring: Bool
    /// A link to join with at launch (for trying the app from a script).
    let joinLink: String?

    @AppStorage("myco-appearance") private var appearance = "system"
    @AppStorage(AppLanguage.defaultsKey) private var language = "system"

    public init(model: AppModel, initialRoute: String? = nil, restoring: Bool = true, joinLink: String? = nil) {
        self.model = model
        self.initialRoute = initialRoute
        self.restoring = restoring
        self.joinLink = joinLink
    }

    public var body: some View {
        Group {
            if let session = model.session {
                // A new workspace is a new tree: nothing of the last one stays.
                RootView().environment(session).id(session.id)
            } else if model.started {
                WelcomeScreen()
            } else {
                MycoColor.bg.ignoresSafeArea()
            }
        }
        .environment(model)
        .environment(model.router)
        .preferredColorScheme(appearance == "dark" ? .dark : appearance == "light" ? .light : nil)
        // Strings are picked when a view is built: rebuild on a new language.
        .id(language)
        .task {
            await model.start(restoring: restoring)
            if let joinLink, !joinLink.isEmpty { try? await model.join(link: joinLink) }
            if let initialRoute { model.router.open(spec: initialRoute) }
        }
    }
}

/// The three tabs, each with its own stack of pages.
struct RootView: View {
    @Environment(Router.self) private var router
    @Environment(AppModel.self) private var app
    @Environment(WorkspaceSession.self) private var session
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        @Bindable var router = router
        TabView(selection: Binding(get: { router.tab }, set: { router.select($0) })) {
            // The system's tab bar is hidden: each tab's first page carries
            // the app's own, one row across (`AppTabBar`).
            NavigationStack(path: $router.chatsPath) {
                ChatsScreen().routed()
            }
            .tag(AppTab.chats)

            NavigationStack(path: $router.projectsPath) {
                ProjectsScreen().routed()
            }
            .tag(AppTab.projects)

            NavigationStack(path: $router.membersPath) {
                MembersScreen().routed()
            }
            .tag(AppTab.members)
        }
        .tint(MycoColor.fg)
        .sheet(item: $router.sheet) { sheet in
            switch sheet {
            case .workspace: WorkspaceSheet()
            case .memberCard(let name): MemberCardSheet(name: name)
            case .addAgent: AddAgentSheet()
            case .addComputer(let project): AddComputerSheet(project: project)
            case .export(let agent, let project): ExportSheet(agent: agent, project: project)
            }
        }
        .fullScreenCover(item: $router.page) { page in
            PageScreen(id: page.id)
        }
        // Refresh only while the app is in front.
        .task(id: scenePhase) {
            if scenePhase == .active { await session.run() }
        }
        // The workspaces off screen are looked at now and then, to say
        // when something new came there.
        .task(id: scenePhase) {
            if scenePhase == .active { await app.watchOthers() }
        }
        .onChange(of: scenePhase, initial: true) { _, phase in
            // In front, what is on screen is seen and is not announced.
            app.active = phase == .active
        }
    }
}

extension View {
    /// The pages a tab's stack can show.
    func routed() -> some View {
        navigationDestination(for: Route.self) { route in
            Group {
                switch route {
                case .project(let name): ProjectScreen(name: name)
                case .agent(let name, let focus): AgentScreen(name: name, focus: focus)
                case .exchange(let agent, let other, let focus): ExchangeScreen(agent: agent, other: other, focus: focus)
                case .projectInfo(let name): ProjectInfoScreen(name: name)
                case .automations: AutomationsScreen()
                }
            }
            // A pushed page takes the whole screen: no tab bar under its input.
            .toolbar(.hidden, for: .tabBar)
        }
    }
}

/// The current workspace's face at the top left of every tab: tap to switch
/// workspaces, join another, or open settings.
struct WorkspaceButton: View {
    @Environment(Router.self) private var router
    @Environment(AppModel.self) private var app
    @Environment(WorkspaceSession.self) private var session

    var body: some View {
        Button {
            router.sheet = .workspace
        } label: {
            WorkspaceBadge(name: session.name, size: 28)
                // Something new came in another workspace: a dot on the way there.
                .overlay(alignment: .topTrailing) {
                    if app.unreadElsewhere {
                        Circle().fill(MycoColor.unread).frame(width: 9, height: 9)
                            .overlay { Circle().stroke(MycoColor.bg, lineWidth: 1.5) }
                            .offset(x: 3, y: -3)
                            .accessibilityIdentifier("workspace.elsewhere")
                    }
                }
        }
        .accessibilityLabel(
            app.unreadElsewhere
                ? L("工作区：\(session.name)，别的工作区有新消息", "Workspace: \(session.name). Something new in another workspace")
                : L("工作区：\(session.name)", "Workspace: \(session.name)"))
        .accessibilityIdentifier("workspace.button")
    }
}

/// A workspace's face: the first letter of its name in a coloured square.
struct WorkspaceBadge: View {
    let name: String
    let size: CGFloat

    var body: some View {
        let look = MushroomLook(seed: name)
        RoundedRectangle(cornerRadius: size * 0.28, style: .continuous)
            .fill(Color(hex: look.capColor))
            .frame(width: size, height: size)
            .overlay {
                Text(name.trimmingCharacters(in: .whitespaces).prefix(1).uppercased())
                    .font(.system(size: size * 0.5, weight: .semibold))
                    .foregroundStyle(.white)
            }
    }
}
