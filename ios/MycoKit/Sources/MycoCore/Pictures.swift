import Foundation
import ImageIO
import UniformTypeIdentifiers

/// Pictures on their way out: made into something every agent can read,
/// and into the small previews shown before they are sent.
public enum Pictures {
    /// A picture as it is sent.
    public struct Prepared: Sendable, Hashable {
        public var data: Data
        /// "jpg", "png", …: what its file name ends with.
        public var fileExtension: String
        public var contentType: String
    }

    /// The longest side a picture is sent with. A phone's camera makes far
    /// more than an agent looks at.
    public static let longestSide = 3072
    /// Kept as they are below this size; larger ones are made smaller.
    static let keepBelow = 6 * 1024 * 1024

    /// The kinds agents read as they are, with their file endings.
    private static let kept: [String: (ext: String, type: String)] = [
        "public.png": ("png", "image/png"),
        "public.jpeg": ("jpg", "image/jpeg"),
        "com.compuserve.gif": ("gif", "image/gif"),
        "org.webmproject.webp": ("webp", "image/webp"),
    ]

    /// Makes a picture ready to send: a PNG, JPEG, GIF or WebP of a
    /// sensible size goes as it is (a screenshot stays sharp); anything
    /// else (a phone's HEIC, a 48-megapixel photo) becomes a JPEG no
    /// longer than `longestSide`, turned the right way up. Nil: not a
    /// picture this device can read.
    public static func prepared(_ data: Data) -> Prepared? {
        guard let source = CGImageSourceCreateWithData(data as CFData, nil), CGImageSourceGetCount(source) > 0 else { return nil }
        let kind = (CGImageSourceGetType(source) as String?) ?? ""
        let size = pixelSize(of: source)
        if let keep = kept[kind], data.count <= keepBelow, max(size.width, size.height) <= longestSide + 1024 {
            return Prepared(data: data, fileExtension: keep.ext, contentType: keep.type)
        }
        guard let jpeg = jpeg(from: source, longestSide: longestSide, quality: 0.85) else { return nil }
        return Prepared(data: jpeg, fileExtension: "jpg", contentType: "image/jpeg")
    }

    /// A small JPEG of a picture, for a tile: nil when it is not a picture.
    public static func thumbnail(_ data: Data, longestSide: Int = 240) -> Data? {
        guard let source = CGImageSourceCreateWithData(data as CFData, nil), CGImageSourceGetCount(source) > 0 else { return nil }
        return jpeg(from: source, longestSide: longestSide, quality: 0.8)
    }

    /// How large a picture is, in pixels; nil when it is not a picture.
    public static func pixelSize(of data: Data) -> (width: Int, height: Int)? {
        guard let source = CGImageSourceCreateWithData(data as CFData, nil), CGImageSourceGetCount(source) > 0 else { return nil }
        let size = pixelSize(of: source)
        return size.width > 0 ? size : nil
    }

    private static func pixelSize(of source: CGImageSource) -> (width: Int, height: Int) {
        let properties = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any]
        let width = (properties?[kCGImagePropertyPixelWidth] as? NSNumber)?.intValue ?? 0
        let height = (properties?[kCGImagePropertyPixelHeight] as? NSNumber)?.intValue ?? 0
        return (width, height)
    }

    private static func jpeg(from source: CGImageSource, longestSide: Int, quality: Double) -> Data? {
        let options: [CFString: Any] = [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceThumbnailMaxPixelSize: longestSide,
        ]
        guard let image = CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary) else { return nil }
        let out = NSMutableData()
        guard let destination = CGImageDestinationCreateWithData(out, UTType.jpeg.identifier as CFString, 1, nil) else { return nil }
        CGImageDestinationAddImage(destination, image, [kCGImageDestinationLossyCompressionQuality: quality] as CFDictionary)
        guard CGImageDestinationFinalize(destination) else { return nil }
        return out as Data
    }
}

/// How large a file is, as people write it: "812 B", "3.4 KB", "12.0 MB"
/// (the desktop app's wording).
public func byteCount(_ bytes: Int64) -> String {
    if bytes < 1024 { return "\(bytes) B" }
    if bytes < 1024 * 1024 {
        let kb = Double(bytes) / 1024
        return String(format: bytes < 10 * 1024 ? "%.1f KB" : "%.0f KB", kb)
    }
    return String(format: "%.1f MB", Double(bytes) / 1024 / 1024)
}
