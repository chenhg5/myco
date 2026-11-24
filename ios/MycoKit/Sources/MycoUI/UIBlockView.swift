import SwiftUI
import Charts
import UIKit

/// Where a message's pictures come from: `attachment:<name or id>` of the
/// message a block or a Markdown picture is in. Set around a message's
/// text; nil where it has no files (an https:// picture still shows).
public struct MessagePictures: @unchecked Sendable {
    /// The files placed by name or id, and how to fetch one onto the device.
    public let ids: [String: String]
    public let load: @MainActor (String) async throws -> URL

    public init(names: [String: String], load: @escaping @MainActor (String) async throws -> URL) {
        ids = names
        self.load = load
    }

    /// The file a source names, as its id (`attachment:` by name or id).
    public func file(of src: String) -> String? {
        guard src.hasPrefix("attachment:") else { return nil }
        let name = String(src.dropFirst("attachment:".count)).trimmingCharacters(in: .whitespaces)
        let decoded = name.removingPercentEncoding ?? name
        return ids[name] ?? ids[decoded] ?? (ids.values.contains(name) ? name : nil)
    }
}

/// What a block's button does: put words in the input, open a link, copy.
public struct UIBlockActions: @unchecked Sendable {
    public let reply: @MainActor (String) -> Void
    public init(reply: @escaping @MainActor (String) -> Void) { self.reply = reply }
}

extension EnvironmentValues {
    @Entry public var messagePictures: MessagePictures? = nil
    @Entry public var uiBlockActions: UIBlockActions? = nil
}

/// The words blocks say themselves (set by the app, in its language).
public enum UIBlockWords {
    public struct Words: Sendable {
        public var cannotDraw = "This block could not be drawn; its source:"
        public var diagram = "Diagram (Mermaid)"
        public var passed = "passed"
        public var failed = "failed"
        public var skipped = "skipped"
        public var copied = "Copied"
        public var diff = "Changes"
        public init() {}
    }
    @MainActor public static var words: () -> Words = { Words() }
}

enum UITone: String {
    case neutral, info, success, warning, danger

    init(_ value: UIValue?) { self = value?.string.flatMap(UITone.init(rawValue:)) ?? .neutral }

    var color: Color {
        switch self {
        case .neutral: return MycoColor.muted
        case .info: return MycoColor.unread
        case .success: return MycoColor.green
        case .warning: return MycoColor.amber
        case .danger: return MycoColor.red
        }
    }

    var soft: Color {
        switch self {
        case .neutral: return MycoColor.pill
        case .info: return MycoColor.tinted(0x2F7BF6, 0.12)
        case .success: return MycoColor.greenSoft
        case .warning: return MycoColor.amberSoft
        case .danger: return MycoColor.redSoft
        }
    }

    var symbol: String {
        switch self {
        case .neutral: return "info.circle"
        case .info: return "info.circle.fill"
        case .success: return "checkmark.circle.fill"
        case .warning: return "exclamationmark.triangle.fill"
        case .danger: return "xmark.octagon.fill"
        }
    }
}

/// A ```myco-ui block in a message, drawn with native components; its
/// source when it cannot be read.
public struct UIBlockView: View {
    let source: String
    let root: UIElement?

    public init(source: String) {
        self.source = source
        root = UIBlock.parse(source)
    }

    public var body: some View {
        if let root {
            UIElementView(element: root)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("uiblock")
        } else {
            VStack(alignment: .leading, spacing: 6) {
                Text(UIBlockWords.words().cannotDraw).font(.caption).foregroundStyle(MycoColor.muted)
                CodeBlockView(code: source, language: "json")
            }
            .accessibilityIdentifier("uiblock.source")
        }
    }
}

/// A block of code: coloured by its language, scrolled sideways.
public struct CodeBlockView: View {
    let code: String
    let language: String
    var title: String? = nil

    public init(code: String, language: String, title: String? = nil) {
        self.code = code
        self.language = language
        self.title = title
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let title = title ?? (language.isEmpty ? nil : language) {
                Text(title).font(.caption.monospaced()).foregroundStyle(MycoColor.muted)
                    .padding(.horizontal, 14).padding(.top, 10)
            }
            ScrollView(.horizontal, showsIndicators: false) {
                Text(CodeColors.highlight(code, language: language))
                    .font(.footnote.monospaced())
                    .lineSpacing(3)
                    .foregroundStyle(MycoColor.fg)
                    .textSelection(.enabled)
                    .padding(14)
            }
        }
        .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .accessibilityIdentifier("code.block")
    }
}

/// A picture a message places: an attachment of it, or an https:// one.
struct PlacedPicture: View {
    let src: String
    var alt: String = ""
    var height: CGFloat? = nil
    @Environment(\.messagePictures) private var pictures
    @State private var local: UIImage?
    @State private var failed = false

    var body: some View {
        Group {
            if let local {
                Image(uiImage: local).resizable().scaledToFit()
            } else if let file = pictures?.file(of: src) {
                placeholder.task(id: file) {
                    do {
                        let url = try await pictures?.load(file)
                        if let url, let image = UIImage(contentsOfFile: url.path) { local = image } else { failed = true }
                    } catch { failed = true }
                }
            } else if let url = URL(string: src), url.scheme == "https" || url.scheme == "http" {
                AsyncImage(url: url) { phase in
                    if let image = phase.image { image.resizable().scaledToFit() } else { placeholder }
                }
            } else {
                // Not known here: its words, not a broken picture.
                Text(alt.isEmpty ? src : alt).font(.footnote).foregroundStyle(MycoColor.muted)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: height, alignment: .leading)
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .accessibilityLabel(alt.isEmpty ? "picture" : alt)
        .accessibilityIdentifier("placed.picture")
    }

    private var placeholder: some View {
        ZStack {
            MycoColor.pill
            Image(systemName: failed ? "photo.badge.exclamationmark" : "photo").font(.title3).foregroundStyle(MycoColor.faint)
        }
        .frame(height: height ?? 160)
    }
}

/// One element and what it holds.
struct UIElementView: View {
    let element: UIElement
    @Environment(\.uiBlockActions) private var actions
    @Environment(\.openURL) private var openURL
    @State private var tab = 0
    @State private var copied = false
    @State private var diffOpen = false

    private func string(_ key: String) -> String? { element[key]?.string.flatMap { $0.isEmpty ? nil : $0 } }
    private func items(_ key: String = "items") -> [UIValue] { element[key]?.list ?? [] }

    var body: some View {
        switch element.type {
        case "Card": card
        case "Stack": stack
        case "Grid": grid
        case "Tabs": tabs
        case "Heading":
            let level = Int(element["level"]?.number ?? 2)
            Text(MarkdownBlock.inline(string("text") ?? ""))
                .font(level <= 1 ? .title3.weight(.semibold) : level == 2 ? .headline : .subheadline.weight(.semibold))
        case "Text":
            ReplyText(string("text") ?? "", dim: string("tone") == "muted")
        case "Callout": callout
        case "Badge": badge(string("text") ?? "", UITone(element["tone"]))
        case "KeyValue": keyValue
        case "Table": table
        case "Metric": metric
        case "Progress": progress
        case "Steps": steps
        case "Image":
            VStack(alignment: .leading, spacing: 4) {
                PlacedPicture(src: string("src") ?? "", alt: string("alt") ?? "")
                if let caption = string("caption") { Text(caption).font(.caption).foregroundStyle(MycoColor.muted) }
            }
        case "Gallery": gallery
        case "MediaList": mediaList
        case "Diagram":
            VStack(alignment: .leading, spacing: 4) {
                CodeBlockView(code: string("source") ?? "", language: "", title: UIBlockWords.words().diagram)
                if let caption = string("caption") { Text(caption).font(.caption).foregroundStyle(MycoColor.muted) }
            }
        case "Chart": chart
        case "Code": CodeBlockView(code: string("code") ?? "", language: string("lang") ?? "", title: string("title"))
        case "FileChange": fileChange
        case "TestResults": testResults
        case "Button": button
        default:
            // Not in the catalog: what it holds, if anything.
            if !element.children.isEmpty { children(spacing: 10) }
        }
    }

    // MARK: Layout

    private func children(spacing: CGFloat) -> some View {
        VStack(alignment: .leading, spacing: spacing) {
            ForEach(element.children) { UIElementView(element: $0) }
        }
    }

    private var card: some View {
        VStack(alignment: .leading, spacing: 10) {
            if string("title") != nil || string("subtitle") != nil {
                VStack(alignment: .leading, spacing: 2) {
                    if let title = string("title") {
                        HStack(spacing: 6) {
                            if element["tone"] != nil { Circle().fill(UITone(element["tone"]).color).frame(width: 7, height: 7) }
                            Text(title).font(.subheadline.weight(.semibold)).foregroundStyle(MycoColor.fg)
                        }
                    }
                    if let subtitle = string("subtitle") { Text(subtitle).font(.caption).foregroundStyle(MycoColor.muted) }
                }
            }
            children(spacing: 10)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(MycoColor.line, lineWidth: 0.5))
    }

    private var gap: CGFloat {
        switch string("gap") { case "small": return 6; case "large": return 16; default: return 10 }
    }

    @ViewBuilder private var stack: some View {
        if string("direction") == "horizontal" {
            // Across while it fits; down on a narrow phone.
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .top, spacing: gap) { ForEach(element.children) { UIElementView(element: $0) } }
                children(spacing: gap)
            }
        } else {
            children(spacing: gap)
        }
    }

    private var grid: some View {
        // At most two columns on a phone.
        let columns = max(1, min(2, Int(element["columns"]?.number ?? 2)))
        return LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 10, alignment: .top), count: columns), alignment: .leading, spacing: 10) {
            ForEach(element.children) { UIElementView(element: $0) }
        }
    }

    @ViewBuilder private var tabs: some View {
        if !element.children.isEmpty {
            VStack(alignment: .leading, spacing: 10) {
                Picker("", selection: $tab) {
                    ForEach(Array(element.children.enumerated()), id: \.offset) { index, child in
                        Text(child.label ?? "\(index + 1)").tag(index)
                    }
                }
                .pickerStyle(.segmented)
                UIElementView(element: element.children[min(tab, element.children.count - 1)])
            }
        }
    }

    // MARK: Content

    private var callout: some View {
        let tone = UITone(element["tone"])
        return HStack(alignment: .top, spacing: 10) {
            Image(systemName: tone.symbol).foregroundStyle(tone.color).padding(.top, 2)
            VStack(alignment: .leading, spacing: 4) {
                if let title = string("title") { Text(title).font(.subheadline.weight(.semibold)).foregroundStyle(MycoColor.fg) }
                ReplyText(string("text") ?? "", compact: true)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(tone.soft, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private func badge(_ text: String, _ tone: UITone) -> some View {
        Text(text).font(.caption.weight(.medium)).foregroundStyle(tone == .neutral ? MycoColor.fg2 : tone.color)
            .padding(.horizontal, 8).padding(.vertical, 3)
            .background(tone.soft, in: Capsule())
    }

    private var keyValue: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(Array(items().enumerated()), id: \.offset) { _, item in
                HStack(alignment: .firstTextBaseline, spacing: 12) {
                    Text(item["label"]?.string ?? "").font(.subheadline).foregroundStyle(MycoColor.muted)
                    Spacer(minLength: 8)
                    Text(item["value"]?.string ?? "").font(.subheadline.weight(.medium))
                        .foregroundStyle(item["tone"] == nil ? MycoColor.fg : UITone(item["tone"]).color)
                        .multilineTextAlignment(.trailing)
                }
            }
        }
    }

    private var table: some View {
        let columns = items("columns").compactMap(\.string)
        let rows = items("rows").map { ($0.list ?? []).map { $0.string ?? "" } }
        let align = items("align").compactMap(\.string)
        func alignment(_ index: Int) -> HorizontalAlignment {
            switch index < align.count ? align[index] : "" { case "right": return .trailing; case "center": return .center; default: return .leading }
        }
        return ScrollView(.horizontal, showsIndicators: false) {
            Grid(alignment: .leading, horizontalSpacing: 16, verticalSpacing: 8) {
                GridRow {
                    ForEach(Array(columns.enumerated()), id: \.offset) { index, cell in
                        Text(MarkdownBlock.inline(cell)).font(.subheadline.weight(.semibold)).gridColumnAlignment(alignment(index))
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
    }

    private var metric: some View {
        let tone = UITone(element["tone"])
        return VStack(alignment: .leading, spacing: 2) {
            Text(string("label") ?? "").font(.caption).foregroundStyle(MycoColor.muted)
            Text(string("value") ?? "").font(.title2.weight(.semibold)).foregroundStyle(MycoColor.fg).minimumScaleFactor(0.6).lineLimit(1)
            if let delta = string("delta") { Text(delta).font(.caption.weight(.medium)).foregroundStyle(tone == .neutral ? MycoColor.muted : tone.color) }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var progress: some View {
        let value = max(0, min(100, element["value"]?.number ?? 0))
        let tone = UITone(element["tone"])
        return VStack(alignment: .leading, spacing: 4) {
            HStack {
                if let label = string("label") { Text(label).font(.subheadline) }
                Spacer()
                Text("\(Int(value))%").font(.caption.monospacedDigit()).foregroundStyle(MycoColor.muted)
            }
            ProgressView(value: value, total: 100).tint(tone == .neutral ? MycoColor.accent : tone.color)
        }
    }

    private var steps: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(Array(items().enumerated()), id: \.offset) { _, item in
                let status = item["status"]?.string ?? "todo"
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Image(systemName: Self.stepSymbol(status)).font(.subheadline).foregroundStyle(Self.stepColor(status))
                    VStack(alignment: .leading, spacing: 1) {
                        Text(item["title"]?.string ?? "").font(.subheadline)
                            .foregroundStyle(status == "done" || status == "skipped" ? MycoColor.muted : MycoColor.fg)
                            .strikethrough(status == "skipped")
                        if let note = item["note"]?.string, !note.isEmpty { Text(note).font(.caption).foregroundStyle(MycoColor.muted) }
                    }
                }
            }
        }
    }

    static func stepSymbol(_ status: String) -> String {
        switch status {
        case "done": return "checkmark.circle.fill"
        case "doing": return "circle.dotted.circle"
        case "failed": return "xmark.circle.fill"
        case "skipped": return "minus.circle"
        default: return "circle"
        }
    }

    static func stepColor(_ status: String) -> Color {
        switch status {
        case "done": return MycoColor.green
        case "doing": return MycoColor.accent
        case "failed": return MycoColor.red
        default: return MycoColor.faint
        }
    }

    // MARK: Media

    private var gallery: some View {
        LazyVGrid(columns: [GridItem(.flexible(), spacing: 10), GridItem(.flexible(), spacing: 10)], alignment: .leading, spacing: 10) {
            ForEach(Array(items().enumerated()), id: \.offset) { _, item in
                VStack(alignment: .leading, spacing: 4) {
                    PlacedPicture(src: item["src"]?.string ?? "", alt: item["title"]?.string ?? "", height: 120)
                    if let title = item["title"]?.string { Text(title).font(.caption.weight(.semibold)).lineLimit(2) }
                    if let text = item["text"]?.string { Text(text).font(.caption).foregroundStyle(MycoColor.muted).lineLimit(3) }
                    if let tag = item["badge"]?.string { badge(tag, UITone(item["tone"])) }
                }
            }
        }
    }

    private var mediaList: some View {
        VStack(alignment: .leading, spacing: 12) {
            ForEach(Array(items().enumerated()), id: \.offset) { _, item in
                HStack(alignment: .top, spacing: 10) {
                    if let image = item["image"]?.string {
                        PlacedPicture(src: image, height: 56).frame(width: 56, height: 56)
                    }
                    VStack(alignment: .leading, spacing: 3) {
                        let title = item["title"]?.string ?? ""
                        if let href = item["href"]?.string, let url = URL(string: href) {
                            Link(title, destination: url).font(.subheadline.weight(.semibold))
                        } else {
                            Text(title).font(.subheadline.weight(.semibold))
                        }
                        if let text = item["text"]?.string { Text(MarkdownBlock.inline(text)).font(.footnote).foregroundStyle(MycoColor.fg2) }
                        if let meta = item["meta"]?.string { Text(meta).font(.caption).foregroundStyle(MycoColor.muted) }
                        let tags = (item["badges"]?.list ?? []).compactMap(\.string)
                        if !tags.isEmpty {
                            HStack(spacing: 4) { ForEach(tags, id: \.self) { badge($0, .neutral) } }
                        }
                    }
                }
            }
        }
    }

    @ViewBuilder private var chart: some View {
        let labels = items("labels").compactMap(\.string)
        let series = items("series").map { (name: $0["name"]?.string ?? "", values: ($0["values"]?.list ?? []).compactMap(\.number)) }
        let points = series.flatMap { one in
            one.values.enumerated().compactMap { index, value in
                index < labels.count ? (series: one.name, label: labels[index], value: value, order: index) : nil
            }
        }
        VStack(alignment: .leading, spacing: 6) {
            if let title = string("title") { Text(title).font(.subheadline.weight(.semibold)) }
            Group {
                switch string("kind") {
                case "pie":
                    Chart(points.filter { $0.series == series.first?.name }, id: \.label) { point in
                        SectorMark(angle: .value(string("unit") ?? "", point.value), innerRadius: .ratio(0.5))
                            .foregroundStyle(by: .value("", point.label))
                    }
                case "line":
                    Chart(points, id: \.order) { point in
                        LineMark(x: .value("", point.label), y: .value(string("unit") ?? "", point.value))
                            .foregroundStyle(by: .value("", point.series))
                    }
                default:
                    Chart(points, id: \.order) { point in
                        BarMark(x: .value("", point.label), y: .value(string("unit") ?? "", point.value))
                            .foregroundStyle(by: .value("", point.series))
                            .position(by: .value("", point.series))
                    }
                }
            }
            .frame(height: 200)
        }
        .accessibilityIdentifier("uiblock.chart")
    }

    // MARK: Engineering

    private var fileChange: some View {
        let status = string("status") ?? "modified"
        let tone: UITone = status == "added" ? .success : status == "deleted" ? .danger : .info
        return VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: "doc.text").foregroundStyle(MycoColor.muted)
                Text(string("path") ?? "").font(.footnote.monospaced()).lineLimit(2).truncationMode(.middle)
                Spacer(minLength: 4)
                badge(status, tone)
                if let added = element["additions"]?.number { Text("+\(Int(added))").font(.caption.monospacedDigit()).foregroundStyle(MycoColor.green) }
                if let removed = element["deletions"]?.number { Text("−\(Int(removed))").font(.caption.monospacedDigit()).foregroundStyle(MycoColor.red) }
            }
            if let diff = string("diff") {
                DisclosureGroup(UIBlockWords.words().diff, isExpanded: $diffOpen) {
                    ScrollView(.horizontal, showsIndicators: false) {
                        VStack(alignment: .leading, spacing: 0) {
                            ForEach(Array(diff.components(separatedBy: "\n").enumerated()), id: \.offset) { _, line in
                                Text(line.isEmpty ? " " : line).font(.caption.monospaced())
                                    .foregroundStyle(line.hasPrefix("+") ? MycoColor.green : line.hasPrefix("-") ? MycoColor.red : line.hasPrefix("@@") ? MycoColor.muted : MycoColor.fg)
                            }
                        }
                        .padding(10)
                    }
                    .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
                }
                .font(.caption)
                .tint(MycoColor.muted)
            }
        }
        .padding(12)
        .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(MycoColor.line, lineWidth: 0.5))
    }

    private var testResults: some View {
        let words = UIBlockWords.words()
        let failed = Int(element["failed"]?.number ?? 0)
        return VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 12) {
                Label("\(Int(element["passed"]?.number ?? 0)) \(words.passed)", systemImage: "checkmark.circle.fill").foregroundStyle(MycoColor.green)
                Label("\(failed) \(words.failed)", systemImage: "xmark.circle.fill").foregroundStyle(failed > 0 ? MycoColor.red : MycoColor.muted)
                if let skipped = element["skipped"]?.number, skipped > 0 {
                    Label("\(Int(skipped)) \(words.skipped)", systemImage: "minus.circle").foregroundStyle(MycoColor.muted)
                }
                Spacer(minLength: 0)
                if let duration = string("duration") { Text(duration).foregroundStyle(MycoColor.muted) }
            }
            .font(.footnote.weight(.medium))
            // Failures first.
            let named = items().sorted { ($0["status"]?.string == "failed" ? 0 : 1) < ($1["status"]?.string == "failed" ? 0 : 1) }
            ForEach(Array(named.enumerated()), id: \.offset) { _, item in
                let status = item["status"]?.string ?? "passed"
                HStack(alignment: .firstTextBaseline, spacing: 6) {
                    Image(systemName: status == "failed" ? "xmark.circle.fill" : status == "skipped" ? "minus.circle" : "checkmark.circle.fill")
                        .foregroundStyle(status == "failed" ? MycoColor.red : status == "skipped" ? MycoColor.muted : MycoColor.green)
                    VStack(alignment: .leading, spacing: 1) {
                        Text(item["name"]?.string ?? "").font(.footnote.monospaced())
                        if let message = item["message"]?.string, !message.isEmpty {
                            Text(message).font(.caption).foregroundStyle(MycoColor.muted)
                        }
                    }
                }
            }
        }
        .padding(12)
        .background(MycoColor.card, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(MycoColor.line, lineWidth: 0.5))
    }

    // MARK: Actions

    private var button: some View {
        let label = string("label") ?? ""
        let value = string("value") ?? ""
        let primary = element["primary"]?.bool == true
        return Button {
            switch string("action") {
            case "open":
                if let url = URL(string: value), ["https", "http"].contains(url.scheme ?? "") { openURL(url) }
            case "copy":
                UIPasteboard.general.string = value
                copied = true
                Task { try? await Task.sleep(for: .seconds(1.5)); copied = false }
            default:
                actions?.reply(value)
            }
        } label: {
            Text(copied ? UIBlockWords.words().copied : label)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(primary ? MycoColor.buttonText : MycoColor.fg)
                .padding(.horizontal, 14).frame(minHeight: 36)
                .background(primary ? MycoColor.button : MycoColor.pill, in: Capsule())
        }
        .buttonStyle(PressScaleStyle())
        .accessibilityIdentifier("uiblock.button")
    }
}
