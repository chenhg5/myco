import SwiftUI
import UIKit
import MycoCore
import MycoUI

/// Writing a conversation out as a Markdown file: the whole of it, with
/// or without the agents' work steps, handed to the system's share sheet
/// (save to Files, AirDrop, send to someone). The desktop app's export.
struct ExportSheet: View {
    let agent: String
    let project: String?
    @Environment(\.dismiss) private var dismiss
    @Environment(WorkspaceSession.self) private var session

    @State private var steps = false
    @State private var busy = false
    @State private var failure: Failure?
    /// The file last written, and what is in it.
    @State private var done: Written?
    @State private var sharing: Written?

    struct Written: Identifiable, Hashable {
        var url: URL
        var messages: Int
        var id: URL { url }
    }

    private var name: String { session.displayName(of: agent) }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack {
                Text(L("导出对话", "Export the conversation")).font(.title3.weight(.semibold)).foregroundStyle(MycoColor.fg)
                Spacer()
                Button {
                    dismiss()
                } label: {
                    Image(systemName: "xmark")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundStyle(MycoColor.muted)
                        .frame(width: 30, height: 30)
                        .background(MycoColor.pill, in: Circle())
                }
                .buttonStyle(PressScaleStyle())
                .accessibilityLabel(L("关闭", "Close"))
            }
            Text(L("和 \(name) 的整段对话存成一个 Markdown 文件：按时间排，写着谁说的、什么时候、附件名。可以存到「文件」，或者发给别人。",
                   "Your whole conversation with \(name) as one Markdown file: in time order, with who said it, when, and attachment names. Save it to Files, or send it to someone."))
                .font(.subheadline).lineSpacing(3).foregroundStyle(MycoColor.fg2)
                .fixedSize(horizontal: false, vertical: true)
            Toggle(isOn: $steps) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(L("带上 agent 干活的过程", "Include the agents' work steps")).font(.body).foregroundStyle(MycoColor.fg)
                    Text(L("每个任务下面列出它用了哪些工具：读了什么、跑了什么命令。", "Under each task: the tools it used, what it read, which commands it ran."))
                        .font(.footnote).foregroundStyle(MycoColor.muted)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .tint(MycoColor.green)
            .disabled(busy)
            .accessibilityIdentifier("export.steps")
            if let failure {
                FailureLine(failure: failure) { export() }
            } else if let done {
                Text(L("已导出 \(done.messages) 条消息：\(done.url.lastPathComponent)", "Exported \(done.messages) messages: \(done.url.lastPathComponent)"))
                    .font(.footnote).foregroundStyle(MycoColor.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("export.done")
            }
            Button(busy ? L("正在导出…", "Exporting…") : L("导出", "Export")) { export() }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(busy)
                .accessibilityIdentifier("export.submit")
        }
        .padding(.horizontal, 20)
        .padding(.top, 26)
        .padding(.bottom, 12)
        .background(MycoColor.bg)
        .fittedSheet()
        .sheet(item: $sharing) { file in
            ActivityView(items: [file.url])
                .presentationDetents([.medium, .large])
                .ignoresSafeArea()
        }
    }

    private func export() {
        guard !busy else { return }
        busy = true
        failure = nil
        let model = session.conversation(with: agent, project: project)
        Task {
            defer { busy = false }
            do {
                let exported = try await model.export(
                    steps: steps, title: L("和 \(name) 的对话", "Conversation with \(name)"), with: name,
                    name: { session.displayName(of: $0) })
                let written = Written(url: try Self.write(exported), messages: exported.messages)
                done = written
                sharing = written
            } catch is CancellationError {
                // the sheet was closed
            } catch {
                failure = Failure.of(error, canRetry: true)
            }
        }
    }

    /// Writes the file where the share sheet can read it. The one before
    /// it is cleared away: an export is handed on, not kept here.
    private static func write(_ exported: ConversationModel.Exported) throws -> URL {
        let folder = FileManager.default.temporaryDirectory.appendingPathComponent("myco-export", isDirectory: true)
        try? FileManager.default.removeItem(at: folder)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appendingPathComponent(exported.name)
        try Data(exported.text.utf8).write(to: url, options: [.atomic, .completeFileProtection])
        return url
    }
}

/// The system's share sheet.
struct ActivityView: UIViewControllerRepresentable {
    let items: [Any]

    func makeUIViewController(context: Context) -> UIActivityViewController {
        UIActivityViewController(activityItems: items, applicationActivities: nil)
    }

    func updateUIViewController(_ controller: UIActivityViewController, context: Context) {}
}
