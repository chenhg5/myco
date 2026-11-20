import QuickLook
import SwiftUI
import UniformTypeIdentifiers
import UIKit

/// Opens what a message carries: pictures in a viewer of our own (zoom,
/// swipe to the next, drag down to close, share), other files in the
/// system's preview. (SwiftUI's quickLookPreview closes only with Done.)
@MainActor
public enum FilePreview {
    /// The viewer's words (close, share), in the app's language: set by
    /// the app, which knows it.
    public static var words: @MainActor () -> (close: String, share: String) = { ("Close", "Share or save") }
    /// The Markdown viewer's two ways to read a file, in the app's language.
    public static var markdownWords: @MainActor () -> (rendered: String, source: String) = { ("Formatted", "Source") }

    public static func show(_ urls: [URL], at url: URL) {
        let all = urls.contains(url) ? urls : [url]
        guard let from = topController() else { return }
        let pictures = all.filter(isPicture)
        if isPicture(url), !pictures.isEmpty {
            // Over the conversation, which shows through as it is dragged away.
            weak var host: UIViewController?
            let viewer = PictureViewer(urls: pictures, start: pictures.firstIndex(of: url) ?? 0) {
                host?.dismiss(animated: true)
            }
            let controller = UIHostingController(rootView: viewer)
            controller.modalPresentationStyle = .overFullScreen
            controller.modalTransitionStyle = .crossDissolve
            controller.view.backgroundColor = .clear
            host = controller
            from.present(controller, animated: true)
            return
        }
        // Markdown, laid out as the conversation lays out replies (the
        // system's preview shows it as plain text).
        if isMarkdown(url), let text = try? String(contentsOf: url, encoding: .utf8) {
            weak var host: UIViewController?
            let viewer = MarkdownFileViewer(url: url, text: text) { host?.dismiss(animated: true) }
            let controller = UIHostingController(rootView: viewer)
            controller.modalPresentationStyle = .pageSheet
            host = controller
            from.present(controller, animated: true)
            return
        }
        let controller = Controller(urls: all)
        controller.currentPreviewItemIndex = all.firstIndex(of: url) ?? 0
        from.present(controller, animated: true)
    }

    static func isMarkdown(_ url: URL) -> Bool {
        ["md", "markdown", "mdown", "mkd"].contains(url.pathExtension.lowercased())
    }

    static func isPicture(_ url: URL) -> Bool {
        guard let type = UTType(filenameExtension: url.pathExtension) else { return false }
        return type.conforms(to: .image) && UIImage(contentsOfFile: url.path) != nil
    }

    private static func topController() -> UIViewController? {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        let window = scenes.flatMap(\.windows).first { $0.isKeyWindow } ?? scenes.first?.windows.first
        var top = window?.rootViewController
        while let next = top?.presentedViewController, !next.isBeingDismissed { top = next }
        return top
    }

    private final class Controller: QLPreviewController, QLPreviewControllerDataSource {
        let urls: [URL]

        init(urls: [URL]) {
            self.urls = urls
            super.init(nibName: nil, bundle: nil)
            dataSource = self
        }

        required init?(coder: NSCoder) { nil }

        nonisolated func numberOfPreviewItems(in controller: QLPreviewController) -> Int {
            MainActor.assumeIsolated { urls.count }
        }

        nonisolated func previewController(_ controller: QLPreviewController, previewItemAt index: Int) -> any QLPreviewItem {
            MainActor.assumeIsolated { urls[index] as NSURL }
        }
    }
}

/// A Markdown file, laid out (headings, lists, code, tables, links) as
/// replies are, or as it is written; shared or saved from the top.
struct MarkdownFileViewer: View {
    let url: URL
    let text: String
    let close: () -> Void
    @State private var source = false

    var body: some View {
        NavigationStack {
            ScrollView {
                Group {
                    if source {
                        Text(text).font(.system(.footnote, design: .monospaced)).foregroundStyle(MycoColor.fg)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    } else {
                        ReplyText(text).frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                .textSelection(.enabled)
                .padding(.horizontal, Space.page).padding(.vertical, Space.m)
                .accessibilityIdentifier(source ? "markdown.source" : "markdown.rendered")
            }
            .background(MycoColor.bg)
            .navigationTitle(url.lastPathComponent)
            .navigationBarTitleDisplayMode(.inline)
            .safeAreaInset(edge: .top, spacing: 0) {
                Picker("", selection: $source) {
                    Text(FilePreview.markdownWords().rendered).tag(false)
                    Text(FilePreview.markdownWords().source).tag(true)
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, Space.page).padding(.vertical, Space.s)
                .background(MycoColor.bg)
                .accessibilityIdentifier("markdown.mode")
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(action: close) { Image(systemName: "xmark") }
                        .accessibilityLabel(FilePreview.words().close)
                        .accessibilityIdentifier("markdown.close")
                }
                ToolbarItem(placement: .primaryAction) {
                    ShareLink(item: url) { Image(systemName: "square.and.arrow.up") }
                        .accessibilityLabel(FilePreview.words().share)
                }
            }
        }
        .accessibilityIdentifier("markdown.viewer")
    }
}
