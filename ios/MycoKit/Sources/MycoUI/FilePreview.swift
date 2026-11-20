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
        let controller = Controller(urls: all)
        controller.currentPreviewItemIndex = all.firstIndex(of: url) ?? 0
        from.present(controller, animated: true)
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
