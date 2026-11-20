import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

/// A conversation's apps, pictures and files on one page (文件和应用): the
/// web pages linked in its messages first, once each, as the same cards as
/// in the conversation; then each message's pictures and files, newest
/// first, with who sent it and when; a tap opens one. Tabs: all, apps,
/// pictures, files. Older ones come with "再往前找". The desktop app's
/// `ChatFiles.tsx`.
struct ChatFilesSheet: View {
    let agent: String
    let project: String?
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router
    @State private var looking = false
    @State private var tab = Tab.all

    enum Tab: Hashable { case all, apps, pictures, files }

    private var model: ConversationModel { session.conversation(with: agent, project: project) }

    var body: some View {
        NavigationStack {
            ScrollView {
                let linked = model.linkedPages
                let pages = linked.compactMap { session.pages.pages[$0.id] }
                let carried = model.carried.compactMap { one -> ConversationModel.Carried? in
                    var kept = one
                    switch tab {
                    case .pictures: kept.files = one.files.filter(\.isPicture)
                    case .files: kept.files = one.files.filter { !$0.isPicture }
                    case .apps: kept.files = []
                    case .all: break
                    }
                    return kept.files.isEmpty ? nil : kept
                }
                let pictures = model.carried.reduce(0) { $0 + $1.files.filter(\.isPicture).count }
                let others = model.carried.reduce(0) { $0 + $1.files.filter { !$0.isPicture }.count }
                LazyVStack(alignment: .leading, spacing: Space.l) {
                    Picker(L("看什么", "Show"), selection: $tab) {
                        Text(L("全部", "All")).tag(Tab.all)
                        Text(L("应用 \(pages.count)", "Apps \(pages.count)")).tag(Tab.apps)
                        Text(L("图片 \(pictures)", "Pictures \(pictures)")).tag(Tab.pictures)
                        Text(L("文件 \(others)", "Files \(others)")).tag(Tab.files)
                    }
                    .pickerStyle(.segmented)
                    .accessibilityIdentifier("files.tabs")
                    if (tab == .all || tab == .apps), !pages.isEmpty {
                        VStack(alignment: .leading, spacing: 6) {
                            if tab == .all {
                                Text(L("应用", "Apps")).font(.footnote).foregroundStyle(MycoColor.muted)
                            }
                            ForEach(pages, id: \.id) { page in
                                PageCard(page: page, publisher: session.displayName(of: page.publisher)) {
                                    dismiss()
                                    Task {
                                        try? await Task.sleep(for: .milliseconds(450))
                                        router.page = PageRoute(id: page.id)
                                    }
                                }
                                .accessibilityIdentifier("files.app")
                            }
                        }
                    }
                    if carried.isEmpty, tab != .apps || pages.isEmpty {
                        EmptyState(
                            symbol: "photo.on.rectangle",
                            title: tab == .apps ? L("这段对话里还没有链接过应用", "No apps linked here yet") : L("这段对话里还没有图片和文件", "No pictures or files here yet"),
                            note: model.hasMoreEarlier ? L("更早的消息里可能有。", "Older messages may have some.") : "")
                            .padding(.top, 40)
                    }
                    ForEach(carried) { one in
                        VStack(alignment: .leading, spacing: 6) {
                            Text(session.displayName(of: one.from) + " · " + RelativeTime.short(one.at))
                                .font(.footnote).foregroundStyle(MycoColor.muted)
                            MessageFiles(
                                one.files.map { MessageFile(id: $0.fileId, name: $0.name, detail: Self.size($0.size), isPicture: $0.isPicture) },
                                failedText: L("拿不到了：可能已过期", "Not available: it may have expired")
                            ) { id in
                                guard let attachment = one.files.first(where: { $0.fileId == id }) else { throw CancellationError() }
                                return try await session.shelf.url(of: attachment)
                            }
                        }
                        .accessibilityElement(children: .contain)
                        .accessibilityIdentifier("files.message")
                    }
                    if model.hasMoreEarlier {
                        Button {
                            looking = true
                            Task {
                                await model.loadEarlier()
                                looking = false
                            }
                        } label: {
                            HStack(spacing: 8) {
                                if looking { ProgressView().controlSize(.small) }
                                Text(looking ? L("正在往前找…", "Looking further back…") : L("再往前找", "Look further back"))
                            }
                            .frame(maxWidth: .infinity)
                        }
                        .buttonStyle(QuietButtonStyle())
                        .disabled(looking)
                        .accessibilityIdentifier("files.earlier")
                    }
                }
                .padding(.horizontal, Space.page)
                .padding(.vertical, Space.m)
            }
            .background(MycoColor.bg)
            // (the pages linked here, read for their cards)
            .task(id: model.linkedPages.map(\.id)) { await session.pages.need(model.linkedPages.map(\.id)) }
            .navigationTitle(L("文件和应用", "Files and apps"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("完成", "Done")) { dismiss() }
                }
            }
        }
    }

    static func size(_ bytes: Int64) -> String {
        ByteCountFormatter.string(fromByteCount: bytes, countStyle: .file)
    }
}
