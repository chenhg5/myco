import QuickLook
import UIKit

/// Opens files in the system's preview (zoom, page through, share, save),
/// presented the way UIKit apps do, so it closes with a swipe down as well
/// as with Done. (SwiftUI's quickLookPreview closes only with Done.)
@MainActor
public enum FilePreview {
    public static func show(_ urls: [URL], at url: URL) {
        let all = urls.contains(url) ? urls : [url]
        guard let from = topController() else { return }
        let controller = Controller(urls: all)
        controller.currentPreviewItemIndex = all.firstIndex(of: url) ?? 0
        from.present(controller, animated: true)
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
