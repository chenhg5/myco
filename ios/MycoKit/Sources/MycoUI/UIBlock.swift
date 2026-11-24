import Foundation

/// A value in a myco-ui block's props: what JSON holds.
public enum UIValue: Hashable, Sendable {
    case string(String)
    case number(Double)
    case bool(Bool)
    case list([UIValue])
    case object([String: UIValue])
    case null

    init(json: Any) {
        switch json {
        case let value as String: self = .string(value)
        case let value as NSNumber:
            // (JSONSerialization gives booleans as NSNumber too)
            if CFGetTypeID(value) == CFBooleanGetTypeID() { self = .bool(value.boolValue) } else { self = .number(value.doubleValue) }
        case let value as [Any]: self = .list(value.map(UIValue.init(json:)))
        case let value as [String: Any]: self = .object(value.mapValues(UIValue.init(json:)))
        default: self = .null
        }
    }

    /// As words: a string, or a number written plainly (12, not 12.0).
    public var string: String? {
        switch self {
        case .string(let value): return value
        case .number(let value): return value == value.rounded() && abs(value) < 1e15 ? String(Int64(value)) : String(value)
        case .bool(let value): return value ? "true" : "false"
        default: return nil
        }
    }

    public var number: Double? {
        switch self {
        case .number(let value): return value
        case .string(let value): return Double(value)
        default: return nil
        }
    }

    public var bool: Bool? { if case .bool(let value) = self { return value } else { return nil } }
    public var list: [UIValue]? { if case .list(let value) = self { return value } else { return nil } }
    public var object: [String: UIValue]? { if case .object(let value) = self { return value } else { return nil } }

    public subscript(key: String) -> UIValue? { object?[key] }
}

/// One component of a myco-ui block, with what it holds.
public struct UIElement: Identifiable, Hashable, Sendable {
    public let id: String
    public let type: String
    public let props: [String: UIValue]
    public let children: [UIElement]

    public subscript(key: String) -> UIValue? { props[key] }
    /// A child of Tabs: its tab's name.
    public var label: String? { props["label"]?.string }
}

/// A myco-ui block: structured parts of a message (a ```myco-ui fence of
/// JSON), drawn with the components of the shared catalog (the desktop's
/// internal/uispec/catalog.json, version 1). Either form is read: nested
/// elements, an array of them (a vertical Stack), or the flat form
/// {"root": id, "elements": {id: {...}}}.
public enum UIBlock {
    public static let fence = "myco-ui"
    /// The catalog's limits: a block past them is not drawn.
    static let maxBytes = 65_536
    static let maxElements = 300
    static let maxDepth = 12

    public static let components: Set<String> = [
        "Card", "Stack", "Grid", "Tabs", "Heading", "Text", "Callout", "Badge", "KeyValue", "Table", "Metric",
        "Progress", "Steps", "Image", "Gallery", "MediaList", "Diagram", "Chart", "Code", "FileChange", "TestResults", "Button",
    ]

    /// The block's root element, or nil when it cannot be read (not JSON,
    /// neither form, past the limits): then its source is shown instead.
    public static func parse(_ source: String) -> UIElement? {
        let data = Data(source.utf8)
        guard data.count <= maxBytes, let top = try? JSONSerialization.jsonObject(with: data, options: [.fragmentsAllowed]) else { return nil }
        var count = 0
        var next = 0
        func newID() -> String { next += 1; return "e\(next)" }

        func nested(_ value: Any, depth: Int) -> UIElement? {
            guard depth <= maxDepth, let object = value as? [String: Any], let type = object["type"] as? String else { return nil }
            count += 1
            guard count <= maxElements else { return nil }
            let id = newID()
            let props = (object["props"] as? [String: Any] ?? [:]).mapValues(UIValue.init(json:))
            let children = (object["children"] as? [Any] ?? []).compactMap { nested($0, depth: depth + 1) }
            return UIElement(id: id, type: type, props: props, children: children)
        }

        func flat(_ object: [String: Any]) -> UIElement? {
            guard let root = object["root"] as? String, let elements = object["elements"] as? [String: Any] else { return nil }
            var seen: Set<String> = []
            func element(_ id: String, depth: Int) -> UIElement? {
                // (an element that is its own ancestor is drawn once)
                guard depth <= maxDepth, !seen.contains(id), let one = elements[id] as? [String: Any], let type = one["type"] as? String else { return nil }
                seen.insert(id)
                defer { seen.remove(id) }
                count += 1
                guard count <= maxElements else { return nil }
                let props = (one["props"] as? [String: Any] ?? [:]).mapValues(UIValue.init(json:))
                let children = (one["children"] as? [Any] ?? []).compactMap { ($0 as? String).flatMap { element($0, depth: depth + 1) } }
                return UIElement(id: id, type: type, props: props, children: children)
            }
            return element(root, depth: 1)
        }

        let root: UIElement?
        switch top {
        case let list as [Any]:
            let children = list.compactMap { nested($0, depth: 2) }
            root = children.isEmpty ? nil : UIElement(id: "root", type: "Stack", props: [:], children: children)
        case let object as [String: Any]:
            if object["type"] != nil, object["root"] == nil, object["elements"] == nil {
                root = nested(object, depth: 1)
            } else if object["type"] == nil {
                root = flat(object)
            } else {
                root = nil
            }
        default:
            root = nil
        }
        guard count <= maxElements else { return nil }
        return root
    }

    /// The pictures a block places (`src` and `image` props), as written.
    public static func pictures(in element: UIElement) -> [String] {
        var found: [String] = []
        for key in ["src", "image"] { if let src = element[key]?.string { found.append(src) } }
        for item in element["items"]?.list ?? [] {
            for key in ["src", "image"] { if let src = item[key]?.string { found.append(src) } }
        }
        return found + element.children.flatMap(pictures(in:))
    }
}
