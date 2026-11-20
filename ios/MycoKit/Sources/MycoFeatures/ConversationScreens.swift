import SwiftUI
import PhotosUI
import QuickLook
import UniformTypeIdentifiers
import MycoAPI
import MycoCore
import MycoUI

/// An agent's conversation, as its own page.
struct AgentScreen: View {
    let name: String
    /// A message to land on.
    var focus: String?
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        ConversationHost(agent: name, project: nil, focus: focus)
            .navigationBarTitleDisplayMode(.inline)
            .solidTopBar()
            .toolbar {
                ToolbarItem(placement: .principal) {
                    Button {
                        router.sheet = .memberCard(name)
                    } label: {
                        BarTitle(session.displayName(of: name), subtitle: session.status(of: name).map(Words.member))
                    }
                    .accessibilityIdentifier("conversation.title")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Button(L("图片和文件", "Pictures and files"), systemImage: "photo.on.rectangle") {
                            router.sheet = .files(agent: name, project: nil)
                        }
                        if session.pins(with: name).supported {
                            Button(L("置顶一个网址", "Pin an address"), systemImage: "pin") {
                                router.sheet = .pinAddress(agent: name)
                            }
                        }
                        Button(L("聊天背景", "Chat background"), systemImage: "paintpalette") {
                            router.sheet = .background(key: ChatBackgrounds.key(workspace: session.id, agent: name))
                        }
                        Button(L("导出对话", "Export the conversation"), systemImage: "square.and.arrow.up") {
                            router.sheet = .export(agent: name, project: nil)
                        }
                    } label: {
                        Image(systemName: "ellipsis")
                    }
                    .accessibilityLabel(L("更多", "More"))
                    .accessibilityIdentifier("conversation.more")
                }
            }
    }
}

/// What two agents said to each other, to read: opened from a handoff
/// card. The one that gave the work is on the right, as I am in my own
/// conversations; the one that did it on the left, with how it worked.
/// There is no input: it is their conversation.
struct ExchangeScreen: View {
    let agent: String
    let other: String
    var focus: String?
    @Environment(WorkspaceSession.self) private var session
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        let model = session.exchange(of: agent, with: other)
        let front = scenePhase != .background
        ZStack {
            // Its own background, the default, or the page.
            ChatBackdrop(key: ChatBackgrounds.key(workspace: session.id, agent: agent))
            ConversationBody(model: model, focus: focus)
        }
        .task(id: front) {
            if front { await model.run() }
        }
        .navigationBarTitleDisplayMode(.inline)
        .solidTopBar()
        .toolbar {
            ToolbarItem(placement: .principal) {
                BarTitle(
                    session.displayName(of: other) + " → " + session.displayName(of: agent),
                    subtitle: L("它们之间的往来 · 只读", "What they said to each other · read-only"))
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("exchange.title")
            }
        }
    }
}

/// A project: the conversation with its coordinator, and above it a strip
/// of the members with what each is up to.
struct ProjectScreen: View {
    let name: String
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        Group {
            if let project = session.project(name) {
                if let lead = session.coordinator(of: project) {
                    VStack(spacing: 0) {
                        MemberStrip(project: project, lead: lead)
                        ConversationHost(agent: lead, project: name)
                    }
                } else {
                    EmptyState(
                        face: .mushroom(MushroomLook(seed: name)), title: L("项目建好了，拉几个 agent 进来吧", "The project is ready. Bring some agents in."),
                        buttonTitle: L("添加 agent", "Add an agent")) { router.sheet = .addAgent }
                }
            } else if session.loadedAt != nil {
                EmptyState(symbol: "questionmark.folder", title: L("找不到这个项目", "This project is gone"))
            } else {
                // The workspace has not answered yet.
                ConversationSkeleton(label: L("正在加载", "Loading"))
            }
        }
        .background(MycoColor.bg)
        .navigationBarTitleDisplayMode(.inline)
        .solidTopBar()
        .toolbar {
            ToolbarItem(placement: .principal) {
                let project = session.project(name)
                BarTitle(project.map { $0.title.isEmpty ? $0.name : $0.title } ?? name, subtitle: project?.goal)
            }
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button(L("项目资料", "About this project"), systemImage: "doc.text") { router.push(.projectInfo(name)) }
                    Button(L("给项目加一台电脑", "Add a computer to the project"), systemImage: "desktopcomputer") {
                        router.sheet = .addComputer(project: name)
                    }
                    if let project = session.project(name), let lead = session.coordinator(of: project) {
                        Button(L("图片和文件", "Pictures and files"), systemImage: "photo.on.rectangle") {
                            router.sheet = .files(agent: lead, project: name)
                        }
                        if session.pins(with: lead).supported {
                            Button(L("置顶一个网址", "Pin an address"), systemImage: "pin") {
                                router.sheet = .pinAddress(agent: lead)
                            }
                        }
                        Button(L("聊天背景", "Chat background"), systemImage: "paintpalette") {
                            router.sheet = .background(key: ChatBackgrounds.key(workspace: session.id, agent: lead))
                        }
                        Button(L("导出对话", "Export the conversation"), systemImage: "square.and.arrow.up") {
                            router.sheet = .export(agent: lead, project: name)
                        }
                    }
                } label: {
                    Image(systemName: "ellipsis")
                }
                .accessibilityIdentifier("project.more")
            }
        }
    }
}

/// A project's members in a row, the coordinator first: tap one to see
/// its work.
private struct MemberStrip: View {
    let project: Project
    let lead: String
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(alignment: .top, spacing: 4) {
                ForEach(session.memberStatuses(of: project)) { status in
                    Button {
                        router.push(.agent(status.agent.name, focus: nil))
                    } label: {
                        VStack(spacing: 4) {
                            AvatarView(.of(status.agent), size: 40, status: StatusTone.of(member: status), away: status.state == "offline")
                            Text(status.agent.name == lead ? L("协调者", "Coordinator") : session.displayName(of: status.agent.name))
                                .font(.caption2)
                                .foregroundStyle(MycoColor.muted)
                                .lineLimit(1)
                        }
                        .frame(width: 64)
                    }
                    .buttonStyle(PressScaleStyle())
                    .accessibilityIdentifier("strip." + status.agent.name)
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
        }
        .background(MycoColor.bg)
    }
}

/// Shows one conversation. Its model lives with the workspace, so coming
/// back to a conversation finds it as it was.
struct ConversationHost: View {
    let agent: String
    let project: String?
    var focus: String?
    @Environment(WorkspaceSession.self) private var session
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        let model = session.conversation(with: agent, project: project)
        // The app in the background asks for nothing.
        let front = scenePhase != .background
        ZStack {
            // Its own background, the default, or the page.
            ChatBackdrop(key: ChatBackgrounds.key(workspace: session.id, agent: agent))
            ConversationBody(model: model, focus: focus)
        }
        // Starts once the workspace has said who this device is, and
        // starts over when the app comes back to the front: what was
        // asked before it left may never be answered, and the page must
        // not wait for it.
        .task(id: session.me + (front ? "|front" : "|away")) {
            if front { await model.run() }
        }
        // On screen, and whenever something new arrives while it is: read.
        .onChange(of: model.items.last?.id, initial: true) { session.markRead(agent) }
        .onDisappear { session.markRead(agent) }
    }
}

/// What a conversation looks like: what was said, where the open task
/// stands, and the input. It draws the model and passes taps on.
struct ConversationBody: View {
    @Bindable var model: ConversationModel
    /// A task to land on once the conversation has loaded.
    var focus: String?
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(AppModel.self) private var app
    @Environment(\.scenePhase) private var scenePhase
    /// Speaking instead of typing: made the first time the microphone is tapped.
    @State private var dictation: Dictation?
    /// The voice call with this agent, while it is the one that is on.
    /// It lives in the app, not here: leaving this page does not hang up.
    private var call: VoiceCall? { app.call(with: model.agent, in: session.id) }
    /// Why the last call with this agent ended by itself (no microphone,
    /// no permission).
    private var callProblem: DictationProblem? {
        guard let last = app.callProblem, last.agent == model.agent, last.workspace == session.id else { return nil }
        return last.problem
    }
    /// A call could not be made from what is set: its settings are open.
    @State private var settingUpVoice = false
    /// A file about to go, opened to look at it first.
    @State private var lookingAt: URL?
    /// A pinned message to scroll to.
    @State private var jumpTo: String?
    /// What this conversation keeps above its messages (mine with the agent).
    private var pins: PinsModel? { model.readOnly ? nil : session.pins(with: model.agent) }
    /// The end was pulled up: the page is asking the workspace again.
    @State private var refreshingFromEnd = false
    /// Counts those pulls: a tap answers each.
    @State private var pulls = 0
    /// A setting: the keyboard's Return sends instead of breaking the line.
    @AppStorage("myco-return-sends") private var returnSends = false
    /// What was just taken out of view: for a moment it can be taken back.
    @State private var deleted: [String]?
    /// Which way of adding a file is open.
    @State private var pickingPhotos = false
    @State private var picked: [PhotosPickerItem] = []
    @State private var pickingFiles = false
    @State private var takingPhoto = false
    @State private var landed = false
    /// The end of the conversation is on screen.
    @State private var atBottom = true
    /// It was when the keyboard started to come up: keep it there.
    @State private var followKeyboard = false
    /// The conversation stays at its end as it grows. Only the person
    /// scrolling away lets go of it; content getting taller does not.
    @State private var pinned = true
    /// A finger moved the conversation a moment ago.
    @State private var touched = false
    /// How tall the part of the conversation on screen is.
    @State private var viewport: CGFloat = 800

    private static let space = "conversation"
    /// Counts what was sent or stopped from here: a light tap answers each.
    @State private var commits = 0
    /// The conversation has opened where it should and settled: from now
    /// on, reaching its top asks for older messages.
    @State private var settled = false
    /// The conversation is where it opens and can be seen. Until then it
    /// is laid out and scrolled out of sight, so opening never shows it
    /// jump into place.
    @State private var revealed = false
    /// Opening is taking long enough to say so: its shape is drawn meanwhile.
    @State private var waiting = false

    private static let bottomID = "conversation.bottom"
    /// How far past its end the conversation is pulled to refresh it.
    private static let pullToRefresh: CGFloat = 56

    private var name: String { session.displayName(of: model.agent) }

    /// The newest thing I said: saying something new brings it into view.
    private var newestMine: String? {
        model.items.last { if case .mine(_, _, nil) = $0.kind { return true } else { return false } }?.id
    }

    /// The newest answer: the one that carries the row of actions.
    private var newestReply: String? {
        model.items.last { if case .reply = $0.kind { return true } else { return false } }?.id
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                // Not lazy, on purpose. A conversation comes a page at a
                // time, so the rows are few enough to lay out all at once,
                // and then every one has its real height: where the page
                // is scrolled to is exact, which a lazy stack's estimates
                // (with rows as tall as an agent's run) could not promise.
                VStack(alignment: .leading, spacing: 0) {
                    // Older messages exist: coming near this row loads them.
                    if model.hasEarlier {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                            .padding(.bottom, Space.l)
                            .onGeometryChange(for: Bool.self) { proxy in
                                proxy.frame(in: .named(Self.space)).maxY > -240
                            } action: { near in
                                if near { loadEarlier(proxy) }
                            }
                            .accessibilityIdentifier("conversation.earlier")
                    }
                    ForEach(Array(model.items.enumerated()), id: \.element.id) { index, item in
                        row(item, at: index)
                            // What comes once the page is open arrives; what
                            // it opened with is simply there, and so is what
                            // was already shown while it was being sent.
                            .modifier(Arrival(animated: settled && !model.placed.contains(item.id)))
                            .padding(.top, index == 0 ? 0 : gap(from: model.items[index - 1], to: item))
                    }
                    // What I just sent, before the workspace hands it back:
                    // on the page at once, marked as on its way.
                    ForEach(model.outgoing) { sent in
                        VStack(alignment: .trailing, spacing: Space.s) {
                            if !sent.text.isEmpty { MyBubble(sent.text) }
                            files(sent.files, trailing: true)
                            HStack(spacing: 6) {
                                ProgressView().controlSize(.mini)
                                Text(L("发送中", "Sending")).font(.caption).foregroundStyle(MycoColor.muted)
                            }
                            .frame(maxWidth: .infinity, alignment: .trailing)
                            .padding(.trailing, 6)
                            .accessibilityElement(children: .combine)
                            .accessibilityIdentifier("message.sending")
                        }
                        .padding(.top, model.items.isEmpty ? 0 : Space.xl)
                    }
                    // Marks the end. It is also the room between the last
                    // thing said and the input, so "the end" includes it.
                    // On screen (or nearly) means "at the bottom".
                    Color.clear.frame(height: Space.xl)
                        // Pulled up past the end: the page asks again, and
                        // says so here, in the room under the last message.
                        .overlay {
                            if refreshingFromEnd {
                                ProgressView()
                                    .accessibilityLabel(L("正在刷新", "Refreshing"))
                                    .accessibilityIdentifier("conversation.refreshing")
                                    .transition(.opacity)
                            }
                        }
                        .onGeometryChange(for: Bool.self) { proxy in
                            // At rest the end sits on the lower edge of what
                            // can be seen (measured: the two are equal);
                            // pulled, it is further up than that.
                            viewport - proxy.frame(in: .named(Self.space)).maxY > Self.pullToRefresh
                        } action: { pulled in
                            // Only a finger pulls: not the keyboard, not the page growing.
                            if pulled, touched, revealed { refreshFromEnd() }
                        }
                        .id(Self.bottomID)
                        .onGeometryChange(for: Bool.self) { proxy in
                            proxy.frame(in: .named(Self.space)).minY <= viewport + 40
                        } action: { there in
                            atBottom = there
                            if there {
                                pinned = true
                            } else if touched {
                                // Gone because a finger scrolled away: let
                                // go. Gone because the page grew: pinned.
                                pinned = false
                            }
                        }
                }
                .padding(.horizontal, Space.page)
                .padding(.top, Space.l)
                .opacity(revealed ? 1 : 0)
            }
            .coordinateSpace(.named(Self.space))
            .onGeometryChange(for: CGFloat.self) { proxy in
                proxy.size.height
            } action: { height in
                viewport = height
            }
            .simultaneousGesture(
                DragGesture(minimumDistance: 8)
                    .onChanged { _ in touched = true }
                    .onEnded { _ in
                        // The glide after a flick still counts as the person's.
                        Task {
                            try? await Task.sleep(for: .seconds(1.2))
                            touched = false
                        }
                    })
            // Opens at the bottom, and stays there as content grows while
            // the newest message is in view.
            .defaultScrollAnchor(.bottom)
            .scrollDismissesKeyboard(.interactively)
            .modifier(SolidTopEdge())
            // A tap anywhere on the conversation puts the keyboard away.
            .simultaneousGesture(TapGesture().onEnded { Self.dismissKeyboard() })
            // Away from the end: one tap goes back to it.
            .overlay(alignment: .bottom) {
                if !atBottom, !model.items.isEmpty {
                    ScrollToEndButton(label: L("回到底部", "Back to the newest")) {
                        pinned = true
                        withAnimation(.snappy(duration: 0.3)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
                    }
                    .padding(.bottom, Space.m)
                    .transition(.scale(scale: 0.8).combined(with: .opacity))
                }
            }
            .animation(.snappy(duration: 0.2), value: atBottom)
            // What the page shows changed (an answer came, a task's steps
            // loaded, a colleague's exchange was merged in): a pinned
            // conversation goes to its end again. A few times, a moment
            // apart, as rows take their real heights; never in answer to
            // the layout itself, which would chase its own tail.
            .onChange(of: model.revision) {
                guard landed, pinned else { return }
                Task {
                    // Once the page is open, what comes at its end is
                    // followed with a glide, then set right as rows take
                    // their heights; while it opens, at once.
                    var pauses = [0, 80, 300]
                    if settled {
                        withAnimation(.easeOut(duration: 0.25)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
                        pauses = [320, 300]
                    }
                    for pause in pauses {
                        try? await Task.sleep(for: .milliseconds(pause))
                        guard pinned else { return }
                        proxy.scrollTo(Self.bottomID, anchor: .bottom)
                    }
                }
            }
            .onChange(of: model.outgoing.count) { old, new in
                // What I just said is where I look, wherever I had scrolled.
                guard new > old else { return }
                pinned = true
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
                Task {
                    // Once more when the keyboard has gone and the row has its height.
                    try? await Task.sleep(for: .milliseconds(320))
                    if pinned { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
                }
            }
            .onChange(of: newestMine) { old, new in
                // What I just said comes into view even if I had scrolled up.
                guard old != nil, new != nil else { return }
                pinned = true
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
            }
            // The keyboard takes the lower part of the screen. A
            // conversation that was at its end stays at its end; one being
            // read further up stays where it is.
            .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillShowNotification)) { _ in
                followKeyboard = pinned || atBottom
                guard followKeyboard else { return }
                withAnimation(.easeOut(duration: 0.25)) { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
            }
            .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardDidShowNotification)) { _ in
                if followKeyboard { proxy.scrollTo(Self.bottomID, anchor: .bottom) }
            }
            .onChange(of: model.loaded, initial: true) { _, loaded in
                // Where a conversation opens: on the task it was opened
                // for, else at its end. The bottom anchor alone does not get
                // a long, lazily laid out conversation there, so it is
                // sent there, and once more when the first rows have their
                // real heights.
                guard loaded, !landed else { return }
                landed = true
                let target = focus.flatMap { focus in model.items.first { $0.id == focus }?.id }
                if let target {
                    proxy.scrollTo(target, anchor: .center)
                    withAnimation(.easeOut(duration: 0.22)) { revealed = true }
                    settled = true
                    return
                }
                proxy.scrollTo(Self.bottomID, anchor: .bottom)
                Task {
                    try? await Task.sleep(for: .milliseconds(60))
                    proxy.scrollTo(Self.bottomID, anchor: .bottom)
                    // In place: now it fades in, whole.
                    withAnimation(.easeOut(duration: 0.22)) { revealed = true }
                    try? await Task.sleep(for: .milliseconds(400))
                    settled = true
                }
            }
            // A pinned message: scrolled to, in the middle.
            .onChange(of: jumpTo) { _, target in
                guard let target else { return }
                jumpTo = nil
                withAnimation(.snappy(duration: 0.3)) { proxy.scrollTo(target, anchor: .center) }
            }
        }
        // Still on its way after a moment: the shape of a conversation
        // stands in, breathing, until the real one fades in over it. One
        // kept on this device opens before that moment is up.
        .overlay {
            if !revealed, waiting {
                ConversationSkeleton(label: L("正在加载", "Loading")).transition(.opacity)
            }
        }
        .task {
            try? await Task.sleep(for: .milliseconds(160))
            if !revealed { withAnimation(.easeOut(duration: 0.15)) { waiting = true } }
        }
        .overlay {
            if model.loaded, model.items.isEmpty, model.outgoing.isEmpty, model.readOnly {
                EmptyState(symbol: "bubble.left.and.bubble.right", title: L("它们之间还没有说过什么", "They have not said anything to each other yet"))
            } else if model.loaded, model.items.isEmpty, model.outgoing.isEmpty {
                EmptyState(
                    face: session.agent(model.agent).map(AvatarView.Face.of),
                    title: model.project == nil
                        ? L("想让 \(name) 帮你做点什么？", "What should \(name) do for you?")
                        : L("你想在这个项目里做成什么？", "What do you want to get done here?"),
                    note: L("一句话就行，它会自己想办法。", "One sentence is enough. It will work out the rest."))
            }
        }
        // (Two agents' exchange is read, not written into.)
        .safeAreaInset(edge: .bottom, spacing: 0) { if !model.readOnly { bottom } }
        // The workspace does not answer: what is here is from before.
        .safeAreaInset(edge: .top, spacing: 0) {
            VStack(spacing: 0) {
                // (while it is at work, the line above the input says it)
                if model.unreachable != nil, model.openTask == nil {
                    StaleBanner(
                        L("连不上工作区，这里可能不是最新的。往上拉可以再试。", "Cannot reach the workspace: this may not be the latest. Pull up to try again."),
                        details: .of(model.unreachableDetail))
                        .accessibilityIdentifier("conversation.stale")
                }
                if let pins {
                    PinStrip(pins: pins, openFile: { file in
                        Task { lookingAt = try? await session.shelf.url(of: file) }
                    }, jump: { id in
                        if model.items.contains(where: { $0.id == id }) { jumpTo = id }
                    })
                }
            }
        }
        // (and read again now and then: what the agent pins comes later)
        .task(id: pins == nil) {
            if let pins { await pins.run() }
        }
        .sensoryFeedback(.impact(weight: .light), trigger: pulls)
        // (opened as UIKit apps do: a swipe down closes it)
        .onChange(of: lookingAt) { _, url in
            guard let url else { return }
            FilePreview.show([url], at: url)
            lookingAt = nil
        }
        // A link to a page of this workspace opens in the app, like its
        // card; any other link goes where links go.
        .environment(\.openURL, OpenURLAction { url in
            guard let id = PageLinks.id(of: url.absoluteString, in: session.pages.workspace) else { return .systemAction }
            router.page = PageRoute(id: id)
            return .handled
        })
        // Typing or sending while it listens stops it: nothing late is
        // written into a box that was emptied.
        .onChange(of: model.draft) { _, text in dictation?.textChanged(to: text) }
        // (A call goes on when this page is left: the app holds it, and
        // shows it in a strip at the top meanwhile.)
        .onDisappear {
            dictation?.stop()
        }
        .animation(.snappy(duration: 0.25), value: call != nil)
        .sheet(isPresented: $settingUpVoice) {
            NavigationStack { VoiceSettingsScreen(closes: true) }
        }
        .photosPicker(isPresented: $pickingPhotos, selection: $picked, maxSelectionCount: 9, matching: .images, preferredItemEncoding: .compatible)
        .onChange(of: picked) { _, items in
            guard !items.isEmpty else { return }
            picked = []
            Task { await addPhotos(items) }
        }
        .fileImporter(isPresented: $pickingFiles, allowedContentTypes: [.item], allowsMultipleSelection: true) { result in
            guard case .success(let urls) = result else { return }
            for url in urls { Task { await addFile(url) } }
        }
        .fullScreenCover(isPresented: $takingPhoto) {
            CameraPicker { image in
                guard let data = image.jpegData(compressionQuality: 0.9) else { return }
                Task { await addPicture(data, name: Self.pictureName(L("照片", "photo"), at: Date(), index: 0)) }
            }
            .ignoresSafeArea()
        }
    }

    // MARK: Refreshing

    /// The end was pulled up: the conversation and the lists are read
    /// again now. The spinner stays long enough to be seen.
    private func refreshFromEnd() {
        guard !refreshingFromEnd else { return }
        pulls += 1
        withAnimation(.easeOut(duration: 0.15)) { refreshingFromEnd = true }
        Task {
            async let seen: Void? = try? await Task.sleep(for: .milliseconds(600))
            await model.refresh()
            _ = await seen
            withAnimation(.easeOut(duration: 0.2)) { refreshingFromEnd = false }
        }
    }

    // MARK: Speaking

    private func toggleDictation() {
        app.forgetCallProblem()
        let dictation = dictation ?? Dictation(listener: app.speechListener())
        self.dictation = dictation
        dictation.toggle(typed: { model.draft }, write: { model.draft = $0 })
    }

    private var micState: MicState {
        guard let dictation else { return .idle }
        if dictation.listening { return .listening }
        return dictation.problem == nil ? .idle : .blocked
    }

    /// What the agent offers me to say next, while the input is empty and
    /// nothing else is going on in it.
    private var offered: String? {
        guard call == nil, dictation?.listening != true, model.pending.isEmpty,
              model.draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        else { return nil }
        return model.suggestion
    }

    /// Why it could not listen, and whether Settings is where to fix it.
    private var dictationNote: (text: String, settings: Bool)? {
        switch dictation?.problem ?? callProblem {
        case nil: return nil
        case .speechDenied:
            return (L("没有语音识别权限：在「设置 › myco」里打开「语音识别」", "No speech recognition access: allow it in Settings › myco"), true)
        case .microphoneDenied:
            return (L("没有麦克风权限：在「设置 › myco」里打开「麦克风」", "No microphone access: allow it in Settings › myco"), true)
        case .unavailable:
            return (L("这台设备现在没法识别这个语言", "This device cannot recognise this language right now"), false)
        case .microphone:
            return (L("麦克风打不开", "Could not open the microphone"), false)
        case .other(let text):
            return (text, false)
        }
    }

    // MARK: A voice call

    /// With nothing typed and nothing running, the round button starts a
    /// voice call with this agent (not with a shell or a person), once
    /// voice is set up in Settings. Until then it is Send, as always (the
    /// desktop app's rule).
    private var canCall: Bool {
        guard app.voiceReady, call == nil, model.action == .send, model.pending.isEmpty,
              model.draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        else { return false }
        return session.agent(model.agent).map { Naming.kind(of: $0) == .agent } ?? false
    }

    private func startCall() {
        dictation?.stop()
        app.forgetCallProblem()
        Task {
            guard call == nil else { return }
            // (a call with another agent is hung up first: one at a time)
            if !(await app.startCall(in: model, of: session)) {
                // Its key could not be read after all: its settings, then.
                settingUpVoice = true
            }
        }
    }

    private func voiceBar(_ call: VoiceCall) -> some View {
        let stage: VoiceBarView.Stage
        switch call.state {
        case .connecting, .ended: stage = .connecting
        case .listening: stage = .listening
        case .thinking: stage = .thinking
        case .speaking: stage = .speaking
        }
        let words = VoiceWords.state(of: call)
        // The talk, last few lines: what I said, what it said, what it did.
        // (An answer not begun yet shows only while it is the last line.)
        let newest = call.lines.last?.id
        let lines = call.lines.filter { line in
            switch line.kind {
            case .notice: return line.id == newest
            case .assistant: return !line.text.isEmpty || line.id == newest
            default: return true
            }
        }.suffix(3)
        return VoiceBarView(
            stage: stage, muted: call.muted, stageText: words,
            title: L("和 \(name) 语音聊天", "Voice chat with \(name)"),
            hint: L("说吧，比如「把登录那个 bug 修了」「做到哪了」。它说话时你随时可以打断。",
                    "Go ahead, e.g. “fix the login bug” or “how far along is it?”. Cut in any time."),
            lines: lines.map { line in
                let kind: VoiceBarView.Line.Kind
                switch line.kind {
                case .user: kind = .me(hearing: false)
                case .hearing: kind = .me(hearing: true)
                case .assistant: kind = .voice(cut: line.interrupted)
                case .action: kind = .action
                case .notice: kind = .notice
                }
                return VoiceBarView.Line(id: line.id, kind: kind, text: line.text)
            },
            meLabel: L("你", "You"), voiceLabel: L("语音", "Voice"),
            muteLabel: call.muted ? L("取消静音", "Unmute") : L("静音", "Mute"), hangUpLabel: L("挂断", "Hang up"),
            onMute: { call.setMuted(!call.muted) },
            onHangUp: { call.hangUp() })
    }

    // MARK: Adding files

    /// The ways to add a file, in the plus button's menu.
    private var attachChoices: [AttachChoice] {
        var choices = [
            AttachChoice(id: "photos", title: L("照片图库", "Photo Library"), systemImage: "photo.on.rectangle") { pickingPhotos = true },
        ]
        if UIImagePickerController.isSourceTypeAvailable(.camera) {
            choices.append(AttachChoice(id: "camera", title: L("拍照", "Take Photo"), systemImage: "camera") { takingPhoto = true })
        }
        choices.append(AttachChoice(id: "files", title: L("选文件", "Choose Files"), systemImage: "folder") { pickingFiles = true })
        return choices
    }

    /// Pictures picked from the library, in the order they were picked.
    private func addPhotos(_ items: [PhotosPickerItem]) async {
        let now = Date()
        for (index, item) in items.enumerated() {
            let name = Self.pictureName(L("照片", "photo"), at: now, index: index)
            guard let data = try? await item.loadTransferable(type: Data.self) else {
                model.couldNotRead(name)
                continue
            }
            await addPicture(data, name: name)
        }
    }

    /// A picture goes as something every agent can read: a phone's own
    /// format and its largest photos are made into a JPEG of sensible size.
    private func addPicture(_ data: Data, name: String) async {
        guard let prepared = await Task.detached(operation: { Pictures.prepared(data) }).value else {
            model.couldNotRead(name)
            return
        }
        // Each goes up on its own: the next one does not wait for it.
        Task { await model.attach(prepared.data, name: name + "." + prepared.fileExtension, contentType: prepared.contentType) }
    }

    /// A file picked in Files, as it is.
    private func addFile(_ url: URL) async {
        let name = url.lastPathComponent
        let read = await Task.detached { () -> Data? in
            // Files hands out what the person picked for as long as it is asked for.
            let scoped = url.startAccessingSecurityScopedResource()
            defer { if scoped { url.stopAccessingSecurityScopedResource() } }
            return try? Data(contentsOf: url, options: .mappedIfSafe)
        }.value
        guard let read else {
            model.couldNotRead(name)
            return
        }
        let type = UTType(filenameExtension: url.pathExtension)?.preferredMIMEType ?? "application/octet-stream"
        await model.attach(read, name: name, contentType: type)
    }

    /// "照片-153012", and "照片-153012-2" for the second picked at once.
    static func pictureName(_ word: String, at date: Date, index: Int) -> String {
        let parts = Calendar.current.dateComponents([.hour, .minute, .second], from: date)
        let time = String(format: "%02d%02d%02d", parts.hour ?? 0, parts.minute ?? 0, parts.second ?? 0)
        return index == 0 ? "\(word)-\(time)" : "\(word)-\(time)-\(index + 1)"
    }

    /// The files about to go, as the input shows them.
    private var trayFiles: [TrayFile] {
        model.pending.map { file in
            let state: TrayFile.State
            switch file.state {
            case .uploading: state = .uploading
            case .ready: state = .ready
            case .failed: state = .failed
            }
            return TrayFile(id: file.id, name: file.name, preview: file.preview, state: state)
        }
    }

    /// Why the first file that did not go up did not.
    private var fileNote: String? {
        for file in model.pending {
            if case .failed(let reason) = file.state { return reason }
        }
        return nil
    }

    /// The files a message came with, under it.
    private func files(of item: ConversationItem, trailing: Bool) -> some View {
        files(item.files, trailing: trailing)
    }

    @ViewBuilder private func files(_ attachments: [Attachment], trailing: Bool) -> some View {
        if !attachments.isEmpty {
            MessageFiles(
                attachments.map { MessageFile(id: $0.fileId, name: $0.name, detail: byteCount($0.size), isPicture: $0.isPicture) },
                trailing: trailing,
                failedText: L("拿不到了：可能已过期", "Not available: it may have expired")
            ) { id in
                guard let attachment = attachments.first(where: { $0.fileId == id }) else { throw CancellationError() }
                return try await session.shelf.url(of: attachment)
            }
        }
    }

    /// Loads the page of older messages, and keeps what was at the top of
    /// the screen where it was: the new rows go in above it.
    private func loadEarlier(_ proxy: ScrollViewProxy) {
        guard settled else { return }
        let anchor = model.items.first?.id
        Task {
            guard await model.loadEarlier(), let anchor else { return }
            proxy.scrollTo(anchor, anchor: .top)
        }
    }

    private static func dismissKeyboard() {
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    }

    /// How far a thing keeps from the one above it: close inside a turn,
    /// further between its parts, furthest where a new turn begins.
    private func gap(from above: ConversationItem, to item: ConversationItem) -> CGFloat {
        switch (above.kind, item.kind) {
        case (.time, _): return Space.m
        case (_, .time): return Space.xl
        case (.mine, .mine): return Space.s
        case (_, .mine): return Space.xl
        case (.handoff, .handoff): return Space.s
        case (.progress, .progress): return 0
        case (.process, .reply), (.pastProgress, .reply), (.progress, .reply): return Space.m
        case (.mine, .plan), (.plan, .process), (.plan, .progress): return Space.m
        default: return Space.l
        }
    }

    private func isProgress(_ index: Int) -> Bool {
        guard model.items.indices.contains(index), case .progress = model.items[index].kind else { return false }
        return true
    }

    /// Where an update sits in the run of updates around it: they are
    /// drawn as one timeline, joined by a line.
    private func timelinePosition(at index: Int) -> ProgressLine.Position {
        switch (isProgress(index - 1), isProgress(index + 1)) {
        case (false, false): return .only
        case (false, true): return .first
        case (true, true): return .middle
        case (true, false): return .last
        }
    }

    @ViewBuilder private func row(_ item: ConversationItem, at index: Int) -> some View {
        switch item.kind {
        case .time(let date):
            TimeBreak(Self.timeText(date))
        case .takenOver(let text):
            // Another session answers from here on: a line, not a message.
            TimeBreak(Self.timeText(item.at) + " · " + text)
                .accessibilityIdentifier("conversation.handover")
        case .mine(let text, let steer, let by):
            // A colleague's task carries their name; mine needs none.
            VStack(alignment: .trailing, spacing: Space.s) {
                // Added to a task further up: which one, quoted on top.
                if let quote = item.quote {
                    QuoteLine(L("补充：\(quote)", "Adding to: \(quote)"), trailing: true)
                }
                // Files sent with no words of my own: they are shown alone,
                // without the line that only names them.
                // What is added to a task is plain, like anything else I say.
                if steer || by != nil || !ConversationBuilder.saysOnlyItsFiles(text, files: item.files) {
                    // Passed on by a voice call: the words, and a small
                    // mark in place of the note the call put under them.
                    let voiced = ViaVoice.split(text)
                    MyBubble(voiced?.text ?? text, name: by.map { session.displayName(of: $0) })
                        .contextMenu {
                            copyButton(voiced?.text ?? text)
                            pinButton(item.id, text: voiced?.text ?? text)
                            deleteButton(item.id)
                        }
                    if let voiced {
                        VoiceMark(label: L("经语音", "By voice"), heard: voiced.heard.isEmpty ? "" : L("原话：", "As said: ") + voiced.heard)
                    }
                    // Sent for me by an automation, not typed just now.
                    if item.automated {
                        Label(L("自动", "Auto"), systemImage: "clock")
                            .labelStyle(.titleAndIcon)
                            .font(.caption2.weight(.medium))
                            .foregroundStyle(MycoColor.muted)
                            .padding(.horizontal, 6)
                            .accessibilityIdentifier("message.auto")
                    }
                }
                files(of: item, trailing: true)
                // Not started yet: a few words under it, like a receipt.
                // Gone once the agent starts on it.
                if let receipt = item.receipt {
                    Text(Self.words(for: receipt)).font(.caption).foregroundStyle(MycoColor.muted)
                        .frame(maxWidth: .infinity, alignment: .trailing)
                        .padding(.trailing, 6)
                        .accessibilityIdentifier("message.receipt")
                }
            }
        case .reply(let text, let taskID, let to):
            VStack(alignment: .leading, spacing: Space.s) {
                // Not right under what it answers: which one, quoted on top.
                if let quote = item.quote {
                    QuoteLine(L("回复：\(quote)", "In answer to: \(quote)"), trailing: false)
                }
                if let to {
                    // An answer to a colleague says whom it answers.
                    Text(L("回复 \(session.displayName(of: to))", "To \(session.displayName(of: to))"))
                        .font(.footnote.weight(.medium)).foregroundStyle(MycoColor.muted)
                }
                ReplyText(text)
                files(of: item, trailing: false)
                // A web page it published and links to: a card that opens it.
                PageCards(text: text, pins: pins)
                // The newest answer carries what can be done with it; the
                // others keep the page quiet (a long press copies any).
                if item.id == newestReply {
                    let took = taskID.flatMap { model.process(of: $0)?.usage }
                    ReplyActions(
                        copying: text,
                        detail: [item.at.formatted(date: .omitted, time: .shortened), took].compactMap { $0 }.joined(separator: " · "),
                        copyLabel: L("复制", "Copy"))
                }
            }
            .contextMenu {
                copyButton(text)
                pinButton(item.id, text: text)
                deleteButton(item.id)
            }
        case .progress(let text, let latest):
            VStack(alignment: .leading, spacing: Space.s) {
                ProgressLine(
                    text, time: item.at.formatted(date: .omitted, time: .shortened),
                    position: timelinePosition(at: index), latest: latest)
                PageCards(text: text, pins: pins)
            }
        case .plan(let plan, _):
            let done = plan.filter { $0.state == .done }.count
            PlanCard(
                title: L("计划", "Plan"), count: L("\(done) / \(plan.count) 完成", "\(done) of \(plan.count) done"),
                lines: plan.map { .init(text: $0.text, mark: $0.state == .done ? .done : ($0.state == .doing ? .doing : .todo)) })
        case .pastProgress(let texts):
            PastProgress(title: L("\(texts.count) 条进展", texts.count == 1 ? "1 update" : "\(texts.count) updates"), texts: texts)
        case .failure(let reason):
            // What happened, in words; and a task of mine that failed can
            // be sent again with one tap.
            let taskID = item.id.hasSuffix(".failed") ? String(item.id.dropLast(".failed".count)) : item.id
            let sending = model.resending == taskID
            NoticeLine(
                .failed, FailureWords.long(reason),
                actions: !model.canSendAgain(taskID) ? [] : [.init(sending ? L("正在发送…", "Sending…") : L("重新发送", "Send again")) {
                    guard model.resending == nil else { return }
                    Task { await model.sendAgain(taskID) }
                }])
            .accessibilityIdentifier("message.failed")
        case .stopped:
            NoticeLine(.idle, L("已停止", "Stopped"))
        case .status(let state, let taskID):
            status(state, taskID: taskID, saying: item.saying)
        case .process(let taskID, let live):
            VStack(alignment: .leading, spacing: Space.s) {
                // Not taken up yet: what shows here is the earlier task's,
                // still being done.
                if let earlier = model.carrying(taskID) {
                    Text(L("接着做上一件：\(earlier)", "Still on the one before: \(earlier)"))
                        .font(.caption).foregroundStyle(MycoColor.muted)
                        .accessibilityIdentifier("run.carrying")
                }
                ProcessView(parts: model.run(of: taskID), live: live) { attachments, trailing in
                    AnyView(files(attachments, trailing: trailing))
                }
            }
        case .handoff(let handoff):
            // What the agent and another agent handed each other: one
            // card, leading to their whole exchange.
            let name = session.displayName(of: handoff.other)
            let label = Self.label(of: handoff)
            let stateText = Self.words(for: handoff)
            let line = handoff.outgoing ? "\(label) \(name)" : "\(name) \(label)"
            HandoffCard(
                face: session.agent(handoff.other).map(AvatarView.Face.of) ?? .computer,
                name: name, label: label, labelFirst: handoff.outgoing, text: handoff.text, stateText: stateText,
                tone: Self.tone(of: handoff), result: handoff.result,
                spoken: line + L("：", ": ") + handoff.text + L("，", ", ") + stateText
                    + (handoff.result.map { L("。交回：", ". Came back: ") + $0 } ?? ""),
                // The one that gave the work on the right, the one that
                // did it on the left with its steps.
                action: {
                    router.push(.exchange(
                        agent: handoff.outgoing ? handoff.other : model.agent,
                        with: handoff.outgoing ? model.agent : handoff.other, focus: handoff.id))
                })
            .accessibilityIdentifier("handoff." + handoff.id)
        }
    }

    /// "To" / "From", by what was handed over: work, or a note.
    static func label(of handoff: Handoff) -> String {
        if handoff.outgoing { return handoff.isTask ? L("派给", "To") : L("发给", "To") }
        return handoff.isTask ? L("派来", "gave") : L("发来", "wrote")
    }

    /// Where a handoff stands. Done by the other one is "handed back";
    /// done by the agent itself is "done".
    static func words(for handoff: Handoff) -> String {
        switch handoff.state {
        case .note: return L("备注", "Note")
        case .queued: return L("排队中", "Queued")
        case .working: return L("在做", "Working")
        case .failed: return L("没做成", "Didn't work")
        case .done: return handoff.outgoing ? L("交回了", "Handed back") : L("做完了", "Done")
        }
    }

    static func tone(of handoff: Handoff) -> StatusTone {
        switch handoff.state {
        case .working: return .working
        case .failed: return .failed
        case .queued: return .away
        case .done, .note: return .idle
        }
    }

    /// What stands under a task of mine that has not started.
    static func words(for receipt: TaskReceipt) -> String {
        switch receipt {
        case .notOpened: return L("还没开始看", "Not opened yet")
        case .behind: return L("排在后面", "Queued behind the current one")
        case .offline: return L("对方离线，上线后再看", "Offline; it reads it when back")
        case .sessionBusy: return L("会话开在别处，空出来再看", "Its session is busy elsewhere")
        case .later: return L("稍后回复", "Will answer later")
        case .delivered: return L("已送到", "Delivered")
        }
    }

    /// Takes a message out of my view (the agents and other people keep
    /// it); for a few seconds it can be taken back.
    @ViewBuilder private func deleteButton(_ id: String) -> some View {
        // (Two agents' exchange is theirs: nothing is taken out of it here.)
        if !model.readOnly {
            Button(L("删除（只从你这边去掉）", "Delete (from your view only)"), systemImage: "trash", role: .destructive) {
                deleted = [id]
                Task {
                    await model.setHidden([id])
                    try? await Task.sleep(for: .seconds(6))
                    if deleted == [id] { withAnimation(.easeOut(duration: 0.2)) { deleted = nil } }
                }
            }
        }
    }

    private func copyButton(_ text: String) -> some View {
        Button(L("复制", "Copy"), systemImage: "square.on.square") { UIPasteboard.general.string = text }
    }

    /// Keeps a message at the top of the conversation, or takes it away.
    @ViewBuilder private func pinButton(_ id: String, text: String) -> some View {
        if let pins, pins.supported {
            if let pinned = pins.pin(message: id) {
                Button(L("取消置顶", "Unpin"), systemImage: "pin.slash") { Task { await pins.remove(pinned) } }
            } else {
                Button(L("置顶", "Pin"), systemImage: "pin") {
                    Task { await pins.pinMessage(id, title: PinsModel.title(of: text)) }
                }
            }
        }
    }

    /// "14:32" today, "昨天 14:32", else "11月12日 14:32".
    private static func timeText(_ date: Date) -> String {
        let time = date.formatted(date: .omitted, time: .shortened)
        let calendar = Calendar.current
        if calendar.isDateInToday(date) { return time }
        if calendar.isDateInYesterday(date) { return L("昨天 \(time)", "Yesterday \(time)") }
        let locale = Locale(identifier: AppLanguage.current == .zh ? "zh-Hans" : "en")
        return date.formatted(.dateTime.month().day().locale(locale)) + " " + time
    }

    @ViewBuilder private func status(_ state: RunState, taskID: String, saying: String? = nil) -> some View {
        let look = MushroomLook.of(session.agent(model.agent))
        switch state {
        case .queued:
            // Not begun: the message has a receipt under it instead.
            EmptyView()
        case .working(let since):
            // What it says it is doing; when it has not said, a playful
            // word that changes every few seconds, as on the desktop.
            // What it says it is doing now (its newest update, when it has
            // not answered since), else its status.
            let text = saying ?? session.agent(model.agent)?.statusText
            TimelineView(.periodic(from: .now, by: 3.6)) { context in
                WorkingLine(
                    look: look, text: (text?.isEmpty == false ? text : nil) ?? RelativeTime.workingVerb(now: context.date),
                    since: since, elapsed: { RelativeTime.working(since: $0) })
            }
        case .maybeStuck(let quietSince):
            NoticeLine(
                .attention,
                L("\(RelativeTime.elapsed(since: quietSince))没有动静，可能卡住了", "Quiet for \(RelativeTime.elapsed(since: quietSince)). It may be stuck."),
                actions: stuckActions(taskID))
        case .done, .stopped, .failed:
            EmptyView()
        }
    }

    /// What can be done about a task that may be stuck, as on the desktop:
    /// it was done (close it), ask where it stands, or stop it.
    private func stuckActions(_ taskID: String) -> [NoticeLine.Action] {
        var actions: [NoticeLine.Action] = []
        if model.canMarkDone {
            actions.append(.init(L("标记完成", "Mark done")) { Task { await model.markDone(taskID) } })
        }
        actions.append(.init(model.nudged.contains(taskID) ? L("已经催了", "Asked") : L("再催一下", "Nudge")) {
            Task { await model.nudge(taskID) }
        })
        actions.append(.init(L("停止", "Stop")) { Task { await model.submitStop(taskID) } })
        return actions
    }

    private var bottom: some View {
        VStack(spacing: 0) {
            // What else it holds, given by others and not done: said here,
            // handled in its profile (the desktop app's 查看并处理).
            let elsewhere = model.elsewhere
            if !elsewhere.isEmpty {
                NoticeLine(
                    .idle,
                    OpenWork.words(elsewhere, status: session.status(of: model.agent), me: session.me, name: session.displayName(of:)),
                    actions: [.init(L("查看并处理", "See and handle")) { router.sheet = .memberCard(model.agent) }])
                    .padding(.horizontal, Space.page).padding(.top, Space.s)
                    .accessibilityIdentifier("conversation.elsewhere")
            }
            // Only while it is at work: a conversation that has ended has
            // nothing to wait for, whatever the network.
            if model.unreachable != nil, model.openTask != nil, model.failure == nil {
                PoorNetworkLine(since: model.answeredAt) { Task { await model.refresh() } }
                    .padding(.horizontal, Space.page).padding(.top, Space.s)
            }
            if let failure = model.failure {
                FailureLine(failure: failure)
                    .padding(.horizontal, Space.page).padding(.top, Space.s)
            }
            if let ids = deleted {
                HStack(spacing: Space.s) {
                    Text(L("已删除", "Deleted")).font(.footnote).foregroundStyle(MycoColor.fg2)
                    Spacer(minLength: 0)
                    Button(L("撤销", "Undo")) {
                        deleted = nil
                        Task { await model.setHidden(ids, hidden: false) }
                    }
                    .font(.footnote.weight(.medium)).tint(MycoColor.accent)
                    .accessibilityIdentifier("delete.undo")
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 9)
                .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                .padding(.horizontal, 14)
                .padding(.top, Space.s)
                .transition(.opacity)
            }
            if let note = dictationNote {
                HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                    Text(note.text).font(.footnote).foregroundStyle(MycoColor.amber)
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("dictation.note")
                    Spacer(minLength: 0)
                    if note.settings, let url = URL(string: UIApplication.openSettingsURLString) {
                        Button(L("去设置", "Settings")) { UIApplication.shared.open(url) }
                            .font(.footnote.weight(.medium)).tint(MycoColor.accent)
                    }
                }
                .padding(.horizontal, Space.page).padding(.top, Space.s)
            }
            // What waits behind the task being worked on: not part of the
            // conversation yet, shown here until its turn comes.
            ForEach(model.waiting) { item in
                HStack(spacing: 10) {
                    Text(L("排队中：", "Queued: ") + ConversationBuilder.firstLine(of: item.body))
                        .font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(1)
                    Spacer(minLength: 4)
                    Button(L("编辑", "Edit")) { Task { await model.withdraw(item, edit: true) } }
                    Button(L("撤回", "Withdraw")) { Task { await model.withdraw(item, edit: false) } }
                }
                .font(.footnote)
                .tint(MycoColor.accent)
                .padding(.horizontal, 14)
                .padding(.vertical, 9)
                .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                .padding(.horizontal, 14)
                .padding(.top, Space.s)
                .accessibilityIdentifier("queue.row")
            }
            // What the agent offers me to say next, above an empty input:
            // a tap writes it there, to edit or send. (The desktop app
            // shows it in the input and Tab takes it; a phone has no Tab.)
            if let offered {
                Button {
                    model.takeSuggestion()
                } label: {
                    HStack(spacing: 8) {
                        Text(L("建议：", "Suggested: ") + ConversationBuilder.firstLine(of: offered))
                            .font(.footnote).foregroundStyle(MycoColor.fg2).lineLimit(1)
                        Spacer(minLength: 4)
                        Image(systemName: "arrow.up.left")
                            .font(.footnote.weight(.medium)).foregroundStyle(MycoColor.muted)
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 9)
                    .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                    .contentShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                }
                .buttonStyle(.plain)
                .padding(.horizontal, 14)
                .padding(.top, Space.s)
                .accessibilityLabel(L("填入建议：", "Use the suggestion: ") + offered)
                .accessibilityIdentifier("suggestion.row")
                .transition(.opacity)
            }
            if let call {
                voiceBar(call)
                    .padding(.horizontal, 12)
                    .padding(.top, Space.s)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
            ComposerView(
                text: $model.draft,
                placeholder: model.openTask == nil ? L("发给 \(name)", "Message \(name)") : L("补充给 \(name)", "Add to what \(name) is doing"),
                mode: mode, enabled: canCall || model.canSubmit, submitLabel: submitLabel,
                queueTitle: L("排到下一条", "Send after this one"),
                files: trayFiles, fileNote: fileNote,
                attachLabel: L("加图片或文件", "Add pictures or files"), removeLabel: L("不发这个", "Remove"), retryLabel: L("再传一次", "Try again"),
                attachChoices: attachChoices,
                // The microphone is the call's while one is on, with
                // this agent or another.
                mic: app.ongoing == nil ? micState : nil,
                micLabel: micState == .listening ? L("停止听写", "Stop listening") : L("语音输入", "Dictate"),
                returnSends: returnSends,
                openLabel: L("看一下", "Look at"),
                // The model and effort it runs with, switched here by
                // whoever may. Out of the way while a call is on.
                aside: call == nil ? session.runChoice(of: model.agent).map { AnyView(RunChoiceButton(agent: model.agent, choice: $0)) } : nil,
                onSubmit: {
                    commits += 1
                    if canCall {
                        startCall()
                    } else {
                        // What is sent shows on the page at once; the
                        // keyboard gets out of its way. (Stopping a task
                        // leaves the keyboard alone.)
                        if model.action != .stop { Self.dismissKeyboard() }
                        Task { await model.submit() }
                    }
                },
                onQueue: {
                    commits += 1
                    Self.dismissKeyboard()
                    Task { await model.queueNext() }
                },
                onRemoveFile: { model.removePending($0) },
                onRetryFile: { id in Task { await model.retryPending(id) } },
                onOpenFile: { id in
                    Task {
                        guard let place = await model.preview(ofPending: id) else { return }
                        Self.dismissKeyboard()
                        lookingAt = place
                    }
                },
                onMic: toggleDictation)
        }
        .background(.bar)
        .sensoryFeedback(.impact(weight: .light), trigger: commits)
        .sensoryFeedback(.selection, trigger: micState == .listening)
    }

    private var submitLabel: String {
        if canCall {
            // A call with someone else is on: this one takes its place.
            if let other = app.ongoing, other.call.state != .ended {
                return L("挂断和 \(other.name) 的通话，改和 \(name) 语音聊天", "Hang up with \(other.name) and voice chat with \(name)")
            }
            return L("语音聊天", "Voice chat")
        }
        switch model.action {
        case .send: return L("发送", "Send")
        case .steer: return L("补充", "Add")
        case .stop: return L("停止", "Stop")
        }
    }

    private var mode: ComposerView.Mode {
        if canCall { return .call }
        switch model.action {
        case .send: return .send
        case .steer: return .steer
        case .stop: return .stop
        }
    }
}

/// How a task was worked on, in the order it happened: what the agent
/// said, each run of steps and thoughts folded into one line, the updates
/// it reported and what was added to the task where each came.
struct ProcessView: View {
    let parts: [RunPart]
    /// Still being worked on: a step without a result is still going, and
    /// its updates stay open as a timeline.
    let live: Bool
    /// Draws the files that came with an update or an addition.
    let files: ([Attachment], Bool) -> AnyView

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            ForEach(Array(parts.enumerated()), id: \.element.id) { index, part in
                switch part {
                case .said(_, let text):
                    ReplyText(text)
                case .steps(_, let steps):
                    StepGroupView(summary: RunProcess.summary(of: steps), steps: steps.map(line))
                case .failure(_, let text):
                    NoticeLine(.failed, text)
                case .reports(_, let reports):
                    if live {
                        // While it works, each update shows in full, joined
                        // by a line; the newest of the run has the green dot.
                        VStack(alignment: .leading, spacing: 0) {
                            ForEach(Array(reports.enumerated()), id: \.element.id) { at, report in
                                VStack(alignment: .leading, spacing: Space.s) {
                                    ProgressLine(
                                        report.text, time: report.at.formatted(date: .omitted, time: .shortened),
                                        position: ProgressLine.position(at, of: reports.count),
                                        latest: index == parts.count - 1 && at == reports.count - 1)
                                    if !report.files.isEmpty { files(report.files, false).padding(.leading, 22) }
                                }
                            }
                        }
                    } else {
                        // Once it is answered, they fold into a line that opens.
                        PastProgress(
                            title: L("\(reports.count) 条进展", reports.count == 1 ? "1 update" : "\(reports.count) updates"),
                            texts: reports.map(\.text))
                    }
                case .added(let report):
                    // What I added, where I sent it: what it did before is
                    // above, what it did about it below.
                    VStack(alignment: .trailing, spacing: Space.s) {
                        let voiced = ViaVoice.split(report.text)
                        MyBubble(voiced?.text ?? report.text)
                        if let voiced {
                            VoiceMark(label: L("经语音", "By voice"), heard: voiced.heard.isEmpty ? "" : L("原话：", "As said: ") + voiced.heard)
                        }
                        if !report.files.isEmpty { files(report.files, true) }
                    }
                }
            }
        }
    }

    private func line(_ step: ToolStep) -> StepLine {
        StepLine(
            id: step.id, symbol: symbol(for: step.tool), label: RunProcess.label(for: step.tool),
            detail: step.input, failed: step.failed, running: live && !step.finished,
            prose: step.tool == RunProcess.thinking)
    }

    private func symbol(for tool: String) -> String {
        switch tool {
        case RunProcess.thinking: return "brain"
        case "Bash": return "terminal"
        case "Read", "NotebookRead": return "doc.text"
        case "Edit", "Write", "MultiEdit", "NotebookEdit": return "pencil"
        case "Grep", "Glob", "LS": return "magnifyingglass"
        case "WebFetch", "WebSearch": return "globe"
        default: return "wrench.and.screwdriver"
        }
    }
}

/// The camera, to take one photo and hand it over.
private struct CameraPicker: UIViewControllerRepresentable {
    let onImage: (UIImage) -> Void
    @Environment(\.dismiss) private var dismiss

    func makeUIViewController(context: Context) -> UIImagePickerController {
        let picker = UIImagePickerController()
        picker.sourceType = .camera
        picker.delegate = context.coordinator
        return picker
    }

    func updateUIViewController(_ controller: UIImagePickerController, context: Context) {}

    func makeCoordinator() -> Coordinator {
        Coordinator(onImage: onImage, dismiss: { dismiss() })
    }

    final class Coordinator: NSObject, UIImagePickerControllerDelegate, UINavigationControllerDelegate {
        let onImage: (UIImage) -> Void
        let dismiss: () -> Void

        init(onImage: @escaping (UIImage) -> Void, dismiss: @escaping () -> Void) {
            self.onImage = onImage
            self.dismiss = dismiss
        }

        func imagePickerController(_ picker: UIImagePickerController, didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]) {
            if let image = info[.originalImage] as? UIImage { onImage(image) }
            dismiss()
        }

        func imagePickerControllerDidCancel(_ picker: UIImagePickerController) {
            dismiss()
        }
    }
}

/// Where text scrolls under the top bar, the bar stays solid enough to
/// read its title: a conversation's words do not show through it.
private struct SolidTopEdge: ViewModifier {
    func body(content: Content) -> some View {
        if #available(iOS 26.0, *) {
            content.scrollEdgeEffectStyle(.hard, for: .top)
        } else {
            content
        }
    }
}

extension View {
    /// A top bar with a background of its own on pages of running text: a
    /// conversation's words pass under it without showing through the title.
    func solidTopBar() -> some View {
        toolbarBackground(MycoColor.bg, for: .navigationBar)
            .toolbarBackground(.visible, for: .navigationBar)
    }
}

/// The workspace has not answered for a while, while the agent is at
/// work: it may well be going on. The desktop app's 网络不太好.
private struct PoorNetworkLine: View {
    let since: Date?
    let retry: () -> Void
    @State private var retrying = false

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: "wifi.exclamationmark").font(.footnote).foregroundStyle(MycoColor.amber)
            TimelineView(.periodic(from: .now, by: 15)) { context in
                (Text(L("网络不太好 ", "Poor connection ")).fontWeight(.semibold) + Text(words(now: context.date)))
                    .font(.footnote).foregroundStyle(MycoColor.fg2)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 0)
            Button(retrying ? L("正在重试…", "Retrying…") : L("重试", "Retry")) {
                retrying = true
                retry()
                Task {
                    try? await Task.sleep(for: .seconds(3))
                    retrying = false
                }
            }
            .font(.footnote.weight(.medium)).tint(MycoColor.accent)
            .disabled(retrying)
        }
        .padding(.horizontal, 14).padding(.vertical, 9)
        .background(MycoColor.amberSoft, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("conversation.poorNetwork")
    }

    private func words(now: Date) -> String {
        guard let since, now.timeIntervalSince(since) >= 60 else {
            return L("这里的消息有一会儿没更新了，agent 那边可能还在继续。正在自动重试。", "This chat hasn't updated for a little while; the agent may well be at work. Retrying by itself.")
        }
        let quiet = RelativeTime.elapsed(since: since)
        return L("这里的消息 \(quiet)没更新了，agent 那边可能还在继续。正在自动重试。", "This chat hasn't updated for \(quiet); the agent may well be at work. Retrying by itself.")
    }
}
