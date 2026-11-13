import SwiftUI
import UIKit
import ImageIO
import QuickLook

/// A file of a message, as a page shows it.
public struct MessageFile: Identifiable, Hashable, Sendable {
    public var id: String
    public var name: String
    /// How large it is ("3.4 KB").
    public var detail: String
    public var isPicture: Bool

    public init(id: String, name: String, detail: String, isPicture: Bool) {
        self.id = id
        self.name = name
        self.detail = detail
        self.isPicture = isPicture
    }
}

/// The files a message carries, as chat apps show them: pictures as
/// thumbnails (one larger, several in a grid), other files as rows. A tap
/// opens the system's preview, which zooms, pages through a message's
/// pictures, and shares or saves. A picture that cannot be read (an older
/// workspace, an expired file) falls back to a row.
///
/// Every tile has its size before its picture is here, so a conversation
/// does not move as pictures come in.
public struct MessageFiles: View {
    let files: [MessageFile]
    let trailing: Bool
    let failedText: String
    let load: @MainActor (String) async throws -> URL
    /// Pictures that would not load: shown as rows instead.
    @State private var broken: Set<String> = []
    @State private var preview: URL?
    @State private var previewList: [URL] = []

    static let single: CGFloat = 196
    static let tile: CGFloat = 96
    static let gap: CGFloat = 4
    static let perRow = 3

    /// - Parameters:
    ///   - trailing: on the right, under something I said.
    ///   - failedText: what a row says when its file cannot be fetched.
    ///   - load: fetches a file by id and answers where it is on this device.
    public init(_ files: [MessageFile], trailing: Bool = false, failedText: String, load: @escaping @MainActor (String) async throws -> URL) {
        self.files = files
        self.trailing = trailing
        self.failedText = failedText
        self.load = load
    }

    private var pictures: [MessageFile] { files.filter { $0.isPicture && !broken.contains($0.id) } }
    private var others: [MessageFile] { files.filter { !$0.isPicture || broken.contains($0.id) } }

    public var body: some View {
        VStack(alignment: trailing ? .trailing : .leading, spacing: Space.s) {
            if pictures.count == 1, let picture = pictures.first {
                tile(picture, side: Self.single, radius: 16)
            } else if !pictures.isEmpty {
                VStack(alignment: trailing ? .trailing : .leading, spacing: Self.gap) {
                    ForEach(Array(stride(from: 0, to: pictures.count, by: Self.perRow)), id: \.self) { start in
                        HStack(spacing: Self.gap) {
                            ForEach(pictures[start..<min(start + Self.perRow, pictures.count)]) { picture in
                                tile(picture, side: Self.tile, radius: 12)
                            }
                        }
                    }
                }
            }
            ForEach(others) { file in
                FileRow(file: file, failedText: failedText, load: load) { url in
                    previewList = [url]
                    preview = url
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: trailing ? .trailing : .leading)
        .quickLookPreview($preview, in: previewList)
    }

    private func tile(_ picture: MessageFile, side: CGFloat, radius: CGFloat) -> some View {
        PictureTile(file: picture, side: side, radius: radius, load: load, onBroken: { broken.insert(picture.id) }) {
            open(picture)
        }
    }

    /// Opens a picture full size, with the message's other pictures a
    /// swipe away.
    private func open(_ picture: MessageFile) {
        Task {
            var urls: [URL] = []
            var chosen: URL?
            for other in pictures {
                guard let url = try? await load(other.id) else { continue }
                urls.append(url)
                if other.id == picture.id { chosen = url }
            }
            guard let chosen else { return }
            previewList = urls
            preview = chosen
        }
    }
}

/// One picture of a message: a square thumbnail that opens it.
private struct PictureTile: View {
    let file: MessageFile
    let side: CGFloat
    let radius: CGFloat
    let load: @MainActor (String) async throws -> URL
    let onBroken: () -> Void
    let action: () -> Void
    @State private var image: UIImage?

    init(
        file: MessageFile, side: CGFloat, radius: CGFloat, load: @escaping @MainActor (String) async throws -> URL,
        onBroken: @escaping () -> Void, action: @escaping () -> Void
    ) {
        self.file = file
        self.side = side
        self.radius = radius
        self.load = load
        self.onBroken = onBroken
        self.action = action
        // Seen before: there at once, no flash of grey.
        _image = State(initialValue: Thumbnails.shared.cached(Self.key(file.id, side)))
    }

    private static func key(_ id: String, _ side: CGFloat) -> String { "\(id)@\(Int(side))" }

    var body: some View {
        Button(action: action) {
            ZStack {
                MycoColor.pill
                if let image {
                    Image(uiImage: image).resizable().scaledToFill()
                        .frame(width: side, height: side)
                        .transition(.opacity)
                } else {
                    Image(systemName: "photo").font(.system(size: 20)).foregroundStyle(MycoColor.faint)
                }
            }
            .frame(width: side, height: side)
            .clipShape(RoundedRectangle(cornerRadius: radius, style: .continuous))
            .overlay { RoundedRectangle(cornerRadius: radius, style: .continuous).stroke(MycoColor.line, lineWidth: 0.5) }
            .contentShape(RoundedRectangle(cornerRadius: radius, style: .continuous))
        }
        .buttonStyle(PressScaleStyle())
        .accessibilityLabel(file.name)
        .accessibilityIdentifier("attachment.picture")
        .task(id: file.id) {
            guard image == nil else { return }
            do {
                let url = try await load(file.id)
                let made = await Thumbnails.shared.image(at: url, key: Self.key(file.id, side), side: side)
                guard !Task.isCancelled else { return }
                if let made {
                    withAnimation(.easeOut(duration: 0.18)) { image = made }
                } else {
                    onBroken()
                }
            } catch {
                if !Task.isCancelled { onBroken() }
            }
        }
    }
}

/// A file that is not a picture: its kind, name and size. A tap fetches it
/// and opens the system's preview, where it can be shared or saved.
private struct FileRow: View {
    let file: MessageFile
    let failedText: String
    let load: @MainActor (String) async throws -> URL
    let onOpen: (URL) -> Void
    @State private var loading = false
    @State private var failed = false

    var body: some View {
        Button {
            guard !loading else { return }
            loading = true
            failed = false
            Task {
                defer { loading = false }
                do {
                    onOpen(try await load(file.id))
                } catch {
                    failed = true
                }
            }
        } label: {
            HStack(spacing: 10) {
                ZStack {
                    RoundedRectangle(cornerRadius: 9, style: .continuous).fill(MycoColor.card)
                    if loading {
                        ProgressView().controlSize(.small)
                    } else {
                        Image(systemName: FileKind.symbol(for: file.name))
                            .font(.system(size: 16)).foregroundStyle(MycoColor.fg2)
                    }
                }
                .frame(width: 36, height: 36)
                VStack(alignment: .leading, spacing: 2) {
                    Text(file.name).font(.subheadline.weight(.medium)).foregroundStyle(MycoColor.fg)
                        .lineLimit(1).truncationMode(.middle)
                    Text(failed ? failedText : file.detail).font(.caption)
                        .foregroundStyle(failed ? MycoColor.red : MycoColor.muted).lineLimit(1)
                }
                Spacer(minLength: 0)
            }
            .padding(8)
            .padding(.trailing, 6)
            .frame(width: 260, alignment: .leading)
            .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
            .contentShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        }
        .buttonStyle(PressScaleStyle())
        .accessibilityLabel(file.name + ", " + file.detail)
        .accessibilityIdentifier("attachment.file")
    }
}

/// Which symbol stands for a file, by its name's ending.
public enum FileKind {
    public static func symbol(for name: String) -> String {
        switch (name as NSString).pathExtension.lowercased() {
        case "pdf": return "doc.richtext"
        case "zip", "gz", "tgz", "tar", "rar", "7z": return "doc.zipper"
        case "png", "jpg", "jpeg", "gif", "webp", "heic", "bmp", "avif", "svg": return "photo"
        case "mp4", "mov", "m4v", "webm": return "film"
        case "mp3", "m4a", "wav", "aac", "flac": return "waveform"
        case "csv", "xls", "xlsx", "numbers": return "tablecells"
        case "md", "txt", "log", "json", "yaml", "yml", "toml", "xml", "html", "css",
             "go", "swift", "ts", "tsx", "js", "jsx", "py", "rs", "java", "kt", "c", "h", "cpp", "sh", "sql":
            return "doc.text"
        default: return "doc"
        }
    }
}

/// Small pictures of the pictures in messages: made from the file once
/// (never the whole picture in memory), then kept while there is room.
@MainActor
final class Thumbnails {
    static let shared = Thumbnails()
    private let cache = NSCache<NSString, UIImage>()

    init() {
        cache.countLimit = 300
    }

    func cached(_ key: String) -> UIImage? {
        cache.object(forKey: key as NSString)
    }

    func image(at url: URL, key: String, side: CGFloat) async -> UIImage? {
        if let known = cached(key) { return known }
        let pixels = Int(side * 3)
        let made = await Task.detached(priority: .userInitiated) { Self.downsample(url, longestSide: pixels) }.value
        if let made { cache.setObject(made, forKey: key as NSString) }
        return made
    }

    /// A picture file made small. The shorter side gets `longestSide`
    /// pixels at least, so a square tile filled with it stays sharp.
    nonisolated static func downsample(_ url: URL, longestSide: Int) -> UIImage? {
        guard let source = CGImageSourceCreateWithURL(url as CFURL, [kCGImageSourceShouldCache: false] as CFDictionary) else { return nil }
        var target = longestSide
        if let properties = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any],
           let width = (properties[kCGImagePropertyPixelWidth] as? NSNumber)?.doubleValue,
           let height = (properties[kCGImagePropertyPixelHeight] as? NSNumber)?.doubleValue, min(width, height) > 0 {
            // Up to three times longer than it is wide; beyond that the tile shows its middle a little soft.
            target = Int(Double(longestSide) * min(3, max(width, height) / min(width, height)))
        }
        let options: [CFString: Any] = [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceShouldCacheImmediately: true,
            kCGImageSourceThumbnailMaxPixelSize: target,
        ]
        guard let image = CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary) else { return nil }
        return UIImage(cgImage: image)
    }
}

// MARK: The input's tray

/// A file about to be sent, as the input shows it.
public struct TrayFile: Identifiable, Hashable, Sendable {
    public enum State: Hashable, Sendable {
        case uploading, ready, failed
    }

    public var id: String
    public var name: String
    /// A small picture of it, when it is one.
    public var preview: Data?
    public var state: State

    public init(id: String, name: String, preview: Data? = nil, state: State) {
        self.id = id
        self.name = name
        self.preview = preview
        self.state = state
    }
}

/// The files about to go, above the text: small tiles of one size. A
/// picture is its thumbnail alone; another file is an icon and its name.
/// One still going up is dimmed under a spinner; one that did not go up
/// offers to try again.
struct AttachmentTray: View {
    let files: [TrayFile]
    let removeLabel: String
    let retryLabel: String
    let onRemove: (String) -> Void
    let onRetry: (String) -> Void

    static let side: CGFloat = 64

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(files) { file in
                    TrayTile(file: file, removeLabel: removeLabel, retryLabel: retryLabel, onRemove: { onRemove(file.id) }, onRetry: { onRetry(file.id) })
                        .transition(.scale(scale: 0.8).combined(with: .opacity))
                }
            }
            .padding(.horizontal, 10)
            // Room for the remove buttons, which hang over the tiles' corners.
            .padding(.top, 6)
        }
        .animation(.snappy(duration: 0.2), value: files.map(\.id))
    }
}

private struct TrayTile: View {
    let file: TrayFile
    let removeLabel: String
    let retryLabel: String
    let onRemove: () -> Void
    let onRetry: () -> Void
    @State private var image: UIImage?

    private var side: CGFloat { AttachmentTray.side }

    var body: some View {
        ZStack {
            MycoColor.card
            if let image {
                Image(uiImage: image).resizable().scaledToFill().frame(width: side, height: side)
            } else {
                VStack(spacing: 3) {
                    Image(systemName: FileKind.symbol(for: file.name)).font(.system(size: 17)).foregroundStyle(MycoColor.fg2)
                    Text(file.name).font(.system(size: 10)).foregroundStyle(MycoColor.fg2)
                        .lineLimit(2).truncationMode(.middle).multilineTextAlignment(.center)
                }
                .padding(.horizontal, 5)
            }
            if file.state == .uploading {
                Color.black.opacity(0.32)
                ProgressView().controlSize(.small).tint(.white)
            }
            if file.state == .failed {
                MycoColor.red.opacity(0.78)
                Button(action: onRetry) {
                    Image(systemName: "arrow.clockwise").font(.system(size: 17, weight: .semibold)).foregroundStyle(.white)
                        .frame(width: side, height: side)
                }
                .accessibilityLabel(retryLabel)
            }
        }
        .frame(width: side, height: side)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay { RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(MycoColor.line, lineWidth: 0.5) }
        .overlay(alignment: .topTrailing) {
            Button(action: onRemove) {
                Image(systemName: "xmark").font(.system(size: 9, weight: .bold)).foregroundStyle(.white)
                    .frame(width: 20, height: 20)
                    .background(Color.black.opacity(0.62), in: Circle())
                    .overlay { Circle().stroke(MycoColor.pill, lineWidth: 1.5) }
                    // Easier to hit than it looks.
                    .padding(6)
                    .contentShape(Rectangle())
            }
            .offset(x: 11, y: -11)
            .accessibilityLabel(removeLabel + " " + file.name)
            .accessibilityIdentifier("composer.file.remove")
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("composer.file")
        .task(id: file.id) {
            if let preview = file.preview { image = UIImage(data: preview) }
        }
    }
}
