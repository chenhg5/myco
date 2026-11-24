import SwiftUI

/// Code that names its language, coloured: keywords, strings, comments and
/// numbers, as the desktop colours code blocks. A small tokenizer, not a
/// parser: what it does not know stays plain, so nothing is lost.
public enum CodeColors {
    enum Family { case cLike, python, shell, sql, json, yaml, markup }

    static func family(of language: String) -> Family? {
        switch language.lowercased() {
        case "swift", "go", "golang", "js", "javascript", "jsx", "ts", "typescript", "tsx", "java", "kotlin", "kt",
             "c", "h", "cpp", "c++", "cc", "objc", "rust", "rs", "csharp", "cs", "dart", "scala", "php":
            return .cLike
        case "python", "py": return .python
        case "sh", "bash", "zsh", "shell", "console", "fish": return .shell
        case "sql", "postgres", "mysql", "sqlite": return .sql
        case "json", "jsonc": return .json
        case "yaml", "yml", "toml": return .yaml
        case "html", "xml", "svg", "vue": return .markup
        default: return nil
        }
    }

    static let keywords: [Family: Set<String>] = [
        .cLike: ["func", "function", "fn", "let", "var", "const", "if", "else", "for", "while", "return", "import", "package", "struct",
                 "class", "enum", "interface", "type", "public", "private", "static", "async", "await", "try", "catch", "throw", "throws",
                 "new", "switch", "case", "default", "break", "continue", "go", "defer", "chan", "map", "range", "select", "nil", "null",
                 "true", "false", "self", "this", "guard", "in", "of", "export", "from", "extends", "implements", "impl", "mut", "pub",
                 "use", "mod", "match", "where", "protocol", "extension", "override", "final", "void", "int", "string", "bool",
                 "undefined", "typeof", "instanceof", "do", "fileprivate", "internal", "init", "deinit", "some", "any", "trait", "crate"],
        .python: ["def", "class", "if", "elif", "else", "for", "while", "return", "import", "from", "as", "with", "try", "except",
                  "finally", "raise", "lambda", "yield", "None", "True", "False", "and", "or", "not", "in", "is", "pass", "break",
                  "continue", "async", "await", "global", "nonlocal", "self", "assert", "del"],
        .shell: ["if", "then", "else", "elif", "fi", "for", "do", "done", "while", "case", "esac", "function", "export", "local",
                 "return", "in", "echo", "cd", "sudo", "set", "unset"],
        .sql: ["select", "from", "where", "and", "or", "not", "insert", "into", "values", "update", "set", "delete", "create", "table",
               "index", "on", "join", "left", "right", "inner", "outer", "group", "by", "order", "limit", "as", "null", "primary",
               "key", "alter", "drop", "distinct", "having", "union", "asc", "desc", "is", "in", "like", "count", "default", "references"],
        .json: ["true", "false", "null"],
        .yaml: ["true", "false", "null", "yes", "no", "on", "off"],
    ]

    /// Colours of the four kinds of token.
    static let keyword = MycoColor.dynamic(0x8E44AD, 0xC792EA)
    static let string = MycoColor.green
    static let comment = MycoColor.muted
    static let number = MycoColor.amber
    static let tag = MycoColor.unread

    /// The code as attributed text: coloured where its language is known.
    public static func highlight(_ code: String, language: String) -> AttributedString {
        var text = AttributedString(code)
        guard let family = family(of: language), code.utf8.count <= 40_000 else { return text }
        let nsCode = code as NSString
        // Ranges already coloured: a keyword inside a string stays a string.
        var taken = IndexSet()
        func paint(_ pattern: String, _ color: Color, options: NSRegularExpression.Options = [], group: Int = 0) {
            guard let regex = try? NSRegularExpression(pattern: pattern, options: options) else { return }
            for match in regex.matches(in: code, range: NSRange(location: 0, length: nsCode.length)) {
                let range = match.range(at: group)
                guard range.location != NSNotFound, range.length > 0,
                      !taken.intersects(integersIn: range.location..<(range.location + range.length)),
                      let swiftRange = Range(range, in: code),
                      let lower = AttributedString.Index(swiftRange.lowerBound, within: text),
                      let upper = AttributedString.Index(swiftRange.upperBound, within: text) else { continue }
                text[lower..<upper].foregroundColor = color
                taken.insert(integersIn: range.location..<(range.location + range.length))
            }
        }
        switch family {
        case .cLike:
            paint(#"//[^\n]*|/\*[\s\S]*?\*/"#, comment)
        case .python, .shell, .yaml:
            paint(#"(?m)(^|\s)(#[^\n]*)"#, comment, group: 2)
        case .sql:
            paint(#"--[^\n]*"#, comment)
        case .markup:
            paint(#"<!--[\s\S]*?-->"#, comment)
        case .json:
            break
        }
        // (a JSON key before the strings, or it would read as one)
        if family == .json { paint(#""(?:[^"\\]|\\.)*"(?=\s*:)"#, tag) }
        paint(#""(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|`(?:[^`\\]|\\.)*`"#, string)
        switch family {
        case .markup:
            paint(#"</?[A-Za-z][\w:-]*"#, tag)
        case .yaml:
            paint(#"(?m)^\s*-?\s*([\w.-]+)\s*:"#, tag, group: 1)
        default:
            break
        }
        paint(#"\b-?\d+(?:\.\d+)?\b"#, number)
        if let words = keywords[family], !words.isEmpty {
            let alternatives = words.map { NSRegularExpression.escapedPattern(for: $0) }.joined(separator: "|")
            paint("\\b(?:\(alternatives))\\b", keyword, options: family == .sql ? [.caseInsensitive] : [])
        }
        return text
    }
}
