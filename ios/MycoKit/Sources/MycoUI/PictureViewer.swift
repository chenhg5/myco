import SwiftUI
import UIKit

/// Pictures over the whole screen, as chat apps show them: pinch or
/// double-tap to zoom, swipe sideways to the next one, drag down to close
/// (the conversation shows through as it goes), and share. Files that are
/// not pictures open in the system's preview (`FilePreview`).
struct PictureViewer: View {
    let urls: [URL]
    let close: () -> Void

    @State private var index: Int
    @State private var drag: CGFloat = 0
    @State private var zoomed = false
    @State private var chrome = true

    /// How far down a drag closes it.
    static let closeAfter: CGFloat = 120

    init(urls: [URL], start: Int, close: @escaping () -> Void) {
        self.urls = urls
        self.close = close
        _index = State(initialValue: min(max(0, start), max(0, urls.count - 1)))
    }

    /// The background fades as the picture is dragged away.
    private var dim: Double { max(0, 1 - Double(abs(drag)) / 400) }

    var body: some View {
        ZStack {
            Color.black.opacity(dim).ignoresSafeArea()
            TabView(selection: $index) {
                ForEach(urls.indices, id: \.self) { i in
                    ZoomingPicture(url: urls[i], zoomed: i == index ? $zoomed : .constant(false)) {
                        withAnimation(.easeOut(duration: 0.15)) { chrome.toggle() }
                    }
                    .tag(i)
                    .accessibilityIdentifier("picture.image")
                }
            }
            .tabViewStyle(.page(indexDisplayMode: .never))
            .ignoresSafeArea()
            .offset(y: drag)
            .scaleEffect(1 - min(0.25, abs(drag) / 1600))
            .simultaneousGesture(
                DragGesture(minimumDistance: 12)
                    .onChanged { value in
                        // Down (or up), more than sideways, at its own size.
                        guard !zoomed, abs(value.translation.height) > abs(value.translation.width) else { return }
                        drag = value.translation.height
                    }
                    .onEnded { value in
                        guard !zoomed, drag != 0 else { return }
                        let going = value.predictedEndTranslation.height
                        if abs(drag) > Self.closeAfter || abs(going) > Self.closeAfter * 3 {
                            close()
                        } else {
                            withAnimation(.spring(duration: 0.25)) { drag = 0 }
                        }
                    })
            if chrome && drag == 0 {
                bar
            }
        }
        .statusBarHidden(!chrome)
        .accessibilityIdentifier("picture.viewer")
        .accessibilityAction(.escape) { close() }
    }

    /// Close, which one of how many, and share.
    private var bar: some View {
        VStack {
            HStack {
                Button(action: close) {
                    Image(systemName: "xmark").font(.system(size: 16, weight: .semibold))
                        .frame(width: 44, height: 44)
                        .background(.black.opacity(0.4), in: Circle())
                }
                .accessibilityLabel(FilePreview.words().close)
                .accessibilityIdentifier("picture.close")
                Spacer()
                if urls.count > 1 {
                    Text("\(index + 1) / \(urls.count)").font(.subheadline.monospacedDigit().weight(.medium))
                        .padding(.horizontal, 12).frame(height: 32)
                        .background(.black.opacity(0.4), in: Capsule())
                        .accessibilityIdentifier("picture.count")
                }
                Spacer()
                ShareLink(item: urls[index]) {
                    Image(systemName: "square.and.arrow.up").font(.system(size: 16, weight: .semibold))
                        .frame(width: 44, height: 44)
                        .background(.black.opacity(0.4), in: Circle())
                }
                .accessibilityLabel(FilePreview.words().share)
                .accessibilityIdentifier("picture.share")
            }
            .foregroundStyle(.white)
            .padding(.horizontal, 16)
            Spacer()
        }
        .transition(.opacity)
    }
}

/// One picture that zooms (pinch, double tap) in a scroll view of its own,
/// and says whether it is zoomed: only at its own size is a drag a close.
private struct ZoomingPicture: UIViewRepresentable {
    let url: URL
    @Binding var zoomed: Bool
    let onTap: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeUIView(context: Context) -> UIScrollView {
        let scroll = UIScrollView()
        scroll.delegate = context.coordinator
        scroll.minimumZoomScale = 1
        scroll.maximumZoomScale = 4
        scroll.showsVerticalScrollIndicator = false
        scroll.showsHorizontalScrollIndicator = false
        scroll.contentInsetAdjustmentBehavior = .never
        scroll.backgroundColor = .clear
        let image = UIImageView(image: UIImage(contentsOfFile: url.path))
        image.contentMode = .scaleAspectFit
        image.translatesAutoresizingMaskIntoConstraints = false
        image.isAccessibilityElement = true
        image.accessibilityTraits = .image
        image.accessibilityLabel = url.deletingPathExtension().lastPathComponent
        scroll.addSubview(image)
        NSLayoutConstraint.activate([
            image.widthAnchor.constraint(equalTo: scroll.frameLayoutGuide.widthAnchor),
            image.heightAnchor.constraint(equalTo: scroll.frameLayoutGuide.heightAnchor),
            image.leadingAnchor.constraint(equalTo: scroll.contentLayoutGuide.leadingAnchor),
            image.trailingAnchor.constraint(equalTo: scroll.contentLayoutGuide.trailingAnchor),
            image.topAnchor.constraint(equalTo: scroll.contentLayoutGuide.topAnchor),
            image.bottomAnchor.constraint(equalTo: scroll.contentLayoutGuide.bottomAnchor),
        ])
        context.coordinator.image = image
        let double = UITapGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.doubleTapped(_:)))
        double.numberOfTapsRequired = 2
        scroll.addGestureRecognizer(double)
        let single = UITapGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.tapped))
        single.require(toFail: double)
        scroll.addGestureRecognizer(single)
        return scroll
    }

    func updateUIView(_ scroll: UIScrollView, context: Context) {
        context.coordinator.parent = self
    }

    @MainActor
    final class Coordinator: NSObject, UIScrollViewDelegate {
        var parent: ZoomingPicture
        weak var image: UIImageView?

        init(_ parent: ZoomingPicture) {
            self.parent = parent
        }

        func viewForZooming(in scrollView: UIScrollView) -> UIView? { image }

        func scrollViewDidZoom(_ scrollView: UIScrollView) {
            let zoomed = scrollView.zoomScale > 1.01
            if parent.zoomed != zoomed { parent.zoomed = zoomed }
        }

        @objc func doubleTapped(_ tap: UITapGestureRecognizer) {
            guard let scroll = tap.view as? UIScrollView else { return }
            if scroll.zoomScale > 1.01 {
                scroll.setZoomScale(1, animated: true)
            } else {
                let at = tap.location(in: image)
                let size = CGSize(width: scroll.bounds.width / 2.5, height: scroll.bounds.height / 2.5)
                scroll.zoom(to: CGRect(x: at.x - size.width / 2, y: at.y - size.height / 2, width: size.width, height: size.height), animated: true)
            }
        }

        @objc func tapped() {
            parent.onTap()
        }
    }
}
