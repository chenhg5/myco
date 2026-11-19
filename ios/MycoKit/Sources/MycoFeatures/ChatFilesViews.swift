import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

/// A conversation's pictures and files on one page, newest first: each
/// message's, with who sent it and when; a tap opens one full size. Older
/// ones come with "再往前找". The desktop app's `ChatFiles.tsx`.
struct ChatFilesSheet: View {
    let agent: String
    let project: String?
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session
    @State private var looking = false

    private var model: ConversationModel { session.conversation(with: agent, project: project) }

    var body: some View {
        NavigationStack {
            ScrollView {
                let carried = model.carried
                LazyVStack(alignment: .leading, spacing: Space.l) {
                    if carried.isEmpty {
                        EmptyState(
                            symbol: "photo.on.rectangle", title: L("这段对话里还没有图片和文件", "No pictures or files here yet"),
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
            .navigationTitle(L("图片和文件", "Pictures and files"))
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
