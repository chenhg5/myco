import SwiftUI

/// A block of an agent's answer. Agents write Markdown; this covers what
/// they actually use: paragraphs, headings, lists, code, quotes, tables.
public enum MarkdownBlock: Hashable, Sendable {
    public struct ListItem: Hashable, Sendable {
        /// "•", "1.", or a task list's box.
        public var marker: String
        public var text: String
        /// Nesting depth, from 0.
        public var level: Int
        /// A task list item: ticked or not. Nil for an ordinary item.
        public var checked: Bool?
    }

    case paragraph(String)
    case heading(level: Int, text: String)
    case list([ListItem])
    case code(language: String, text: String)
    case quote(String)
    case table(header: [String], rows: [[String]])
    case rule

    /// Splits Markdown into blocks, line by line. Anything it does not
    /// recognise stays a paragraph, so nothing an agent wrote is lost.
    public static func parse(_ source: String) -> [MarkdownBlock] {
        var blocks: [MarkdownBlock] = []
        var paragraph: [String] = []
        let lines = source.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n")
        var i = 0

        func flush() {
            if !paragraph.isEmpty { blocks.append(.paragraph(paragraph.joined(separator: "\n"))) }
            paragraph = []
        }

        while i < lines.count {
            let line = lines[i]
            let trimmed = line.trimmingCharacters(in: .whitespaces)

            if trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~") {
                flush()
                let fence = String(trimmed.prefix(3))
                let language = String(trimmed.dropFirst(3)).trimmingCharacters(in: .whitespaces)
                var body: [String] = []
                i += 1
                while i < lines.count, !lines[i].trimmingCharacters(in: .whitespaces).hasPrefix(fence) {
                    body.append(lines[i])
                    i += 1
                }
                i += 1 // the closing fence, or the end
                blocks.append(.code(language: language, text: body.joined(separator: "\n")))
                continue
            }
            if trimmed.isEmpty {
                flush()
                i += 1
                continue
            }
            if let match = trimmed.wholeMatch(of: /(#{1,6})\s+(.+?)\s*#*/) {
                flush()
                blocks.append(.heading(level: match.1.count, text: String(match.2)))
                i += 1
                continue
            }
            if trimmed.wholeMatch(of: /(-\s*){3,}|(\*\s*){3,}|(_\s*){3,}/) != nil {
                flush()
                blocks.append(.rule)
                i += 1
                continue
            }
            if trimmed.hasPrefix(">") {
                flush()
                var quoted: [String] = []
                while i < lines.count, lines[i].trimmingCharacters(in: .whitespaces).hasPrefix(">") {
                    let inner = lines[i].trimmingCharacters(in: .whitespaces).dropFirst()
                    quoted.append(inner.hasPrefix(" ") ? String(inner.dropFirst()) : String(inner))
                    i += 1
                }
                blocks.append(.quote(quoted.joined(separator: "\n")))
                continue
            }
            if trimmed.contains("|"), i + 1 < lines.count, isTableRule(lines[i + 1]) {
                flush()
                let header = cells(of: trimmed)
                var rows: [[String]] = []
                i += 2
                while i < lines.count, lines[i].contains("|"), !lines[i].trimmingCharacters(in: .whitespaces).isEmpty {
                    var row = cells(of: lines[i])
                    // Every row as wide as the header, so columns line up.
                    while row.count < header.count { row.append("") }
                    rows.append(Array(row.prefix(header.count)))
                    i += 1
                }
                blocks.append(.table(header: header, rows: rows))
                continue
            }
            if listItem(line) != nil {
                flush()
                var items: [ListItem] = []
                while i < lines.count {
                    if let item = listItem(lines[i]) {
                        items.append(item)
                    } else if !lines[i].trimmingCharacters(in: .whitespaces).isEmpty, lines[i].hasPrefix(" "), !items.isEmpty {
                        // An indented line continues the item above it.
                        items[items.count - 1].text += "\n" + lines[i].trimmingCharacters(in: .whitespaces)
                    } else {
                        break
                    }
                    i += 1
                }
                blocks.append(.list(items))
                continue
            }
            paragraph.append(trimmed)
            i += 1
        }
        flush()
        return blocks
    }

    static func isTableRule(_ line: String) -> Bool {
        let trimmed = line.trimmingCharacters(in: .whitespaces)
        return trimmed.contains("-") && trimmed.wholeMatch(of: /\|?[\s:|-]*-{3,}[\s:|-]*\|?/) != nil
    }

    static func cells(of line: String) -> [String] {
        var trimmed = line.trimmingCharacters(in: .whitespaces)
        if trimmed.hasPrefix("|") { trimmed.removeFirst() }
        if trimmed.hasSuffix("|") { trimmed.removeLast() }
        return trimmed.components(separatedBy: "|").map { $0.trimmingCharacters(in: .whitespaces) }
    }

    static func listItem(_ line: String) -> ListItem? {
        guard let match = line.wholeMatch(of: /(\s*)([-*+]|\d{1,9}[.)])\s+(.*)/) else { return nil }
        let level = match.1.reduce(0) { $0 + ($1 == "\t" ? 4 : 1) } / 2
        var text = String(match.3)
        var marker = match.2.first!.isNumber ? String(match.2).replacingOccurrences(of: ")", with: ".") : "•"
        var checked: Bool?
        if let box = text.wholeMatch(of: /\[( |x|X)\]\s+(.*)/) {
            checked = box.1 != " "
            marker = ""
            text = String(box.2)
        }
        return ListItem(marker: marker, text: text, level: min(level, 4), checked: checked)
    }

    /// Inline Markdown (bold, italics, code, links) as attributed text.
    static func inline(_ text: String) -> AttributedString {
        let options = AttributedString.MarkdownParsingOptions(interpretedSyntax: .inlineOnlyPreservingWhitespace)
        return (try? AttributedString(markdown: text, options: options)) ?? AttributedString(text)
    }
}

/// An agent's answer: no bubble, the full width, Markdown.
public struct ReplyText: View {
    let blocks: [MarkdownBlock]
    let dim: Bool
    let compact: Bool

    /// - Parameters:
    ///   - dim: a quieter colour, for what it thought.
    ///   - compact: a size smaller and a shade lighter, for an update
    ///     along the way: it must not read as the answer.
    public init(_ text: String, dim: Bool = false, compact: Bool = false) {
        blocks = MarkdownBlock.parse(text)
        self.dim = dim
        self.compact = compact
    }

    private var color: Color { dim ? MycoColor.muted : (compact ? MycoColor.fg2 : MycoColor.fg) }
    private var textFont: Font { compact ? .subheadline : .body }

    public var body: some View {
        VStack(alignment: .leading, spacing: compact ? Space.s : Space.m) {
            ForEach(Array(blocks.enumerated()), id: \.offset) { _, block in
                view(for: block)
            }
        }
        .foregroundStyle(color)
        .tint(MycoColor.accent)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder private func view(for block: MarkdownBlock) -> some View {
        switch block {
        case .paragraph(let text):
            Text(MarkdownBlock.inline(text)).font(textFont).lineSpacing(compact ? 3 : 5)
        case .heading(let level, let text):
            Text(MarkdownBlock.inline(text))
                .font(level <= 1 ? .title3.weight(.semibold) : .headline)
                .lineSpacing(3)
                .padding(.top, 6)
        case .list(let items):
            VStack(alignment: .leading, spacing: compact ? 6 : 9) {
                ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        if let checked = item.checked {
                            Image(systemName: checked ? "checkmark.square.fill" : "square")
                                .font(.subheadline).foregroundStyle(checked ? MycoColor.green : MycoColor.faint)
                        } else {
                            Text(item.marker).font(textFont.monospacedDigit()).foregroundStyle(MycoColor.muted)
                        }
                        Text(MarkdownBlock.inline(item.text)).font(textFont).lineSpacing(compact ? 3 : 4)
                    }
                    .padding(.leading, CGFloat(item.level) * 20)
                }
            }
        case .code(_, let text):
            ScrollView(.horizontal, showsIndicators: false) {
                Text(text)
                    .font(.footnote.monospaced())
                    .lineSpacing(3)
                    .foregroundStyle(MycoColor.fg)
                    .textSelection(.enabled)
                    .padding(14)
            }
            .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        case .quote(let text):
            HStack(spacing: 10) {
                RoundedRectangle(cornerRadius: 1.5).fill(MycoColor.line).frame(width: 3)
                Text(MarkdownBlock.inline(text)).font(.body).lineSpacing(5).foregroundStyle(MycoColor.fg2)
            }
            .fixedSize(horizontal: false, vertical: true)
        case .table(let header, let rows):
            ScrollView(.horizontal, showsIndicators: false) {
                Grid(alignment: .leading, horizontalSpacing: 16, verticalSpacing: 8) {
                    GridRow {
                        ForEach(Array(header.enumerated()), id: \.offset) { _, cell in
                            Text(MarkdownBlock.inline(cell)).font(.subheadline.weight(.semibold))
                        }
                    }
                    Divider().gridCellUnsizedAxes(.horizontal)
                    ForEach(Array(rows.enumerated()), id: \.offset) { _, row in
                        GridRow {
                            ForEach(Array(row.enumerated()), id: \.offset) { _, cell in
                                Text(MarkdownBlock.inline(cell)).font(.subheadline)
                            }
                        }
                    }
                }
                .padding(12)
            }
            .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(MycoColor.line, lineWidth: 0.5))
        case .rule:
            Divider()
        }
    }
}
