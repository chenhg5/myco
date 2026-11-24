import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

/// The pictures a message's text places (`attachment:<name or id>` in a
/// Markdown picture or a myco-ui block): drawn there, not listed again.
enum PlacedFiles {
    /// The files not placed in the text, in their order.
    static func unplaced(_ attachments: [Attachment], in text: String) -> [Attachment] {
        guard !attachments.isEmpty, text.contains("attachment:") else { return attachments }
        let placed = Set(MarkdownBlock.placedPictures(in: text).compactMap { src -> String? in
            guard src.hasPrefix("attachment:") else { return nil }
            let name = String(src.dropFirst("attachment:".count)).trimmingCharacters(in: .whitespaces)
            return name.removingPercentEncoding ?? name
        })
        return attachments.filter { !(placed.contains($0.name) || placed.contains($0.fileId)) }
    }

    /// Where a text's pictures come from: these files, fetched from the shelf.
    @MainActor
    static func pictures(_ attachments: [Attachment], shelf: FileShelf) -> MessagePictures? {
        guard !attachments.isEmpty else { return nil }
        var names: [String: String] = [:]
        for attachment in attachments {
            names[attachment.name] = attachment.fileId
            names[attachment.fileId] = attachment.fileId
        }
        return MessagePictures(names: names) { id in
            guard let attachment = attachments.first(where: { $0.fileId == id }) else { throw CancellationError() }
            return try await shelf.url(of: attachment)
        }
    }
}
