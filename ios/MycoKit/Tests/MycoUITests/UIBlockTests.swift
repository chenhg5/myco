import SwiftUI
import Testing
@testable import MycoUI

/// myco-ui blocks are read in either form, within the catalog's limits;
/// code is coloured where its language is known; the pictures a text
/// places are known, so they are not listed again.
struct UIBlockTests {
    @Test func theNestedFormIsRead() throws {
        let root = try #require(UIBlock.parse(#"""
        {"type": "Card", "props": {"title": "Release", "tone": "success"}, "children": [
          {"type": "Metric", "props": {"label": "Tests", "value": 412, "delta": "+12"}},
          {"type": "Steps", "props": {"items": [{"title": "Build", "status": "done"}]}}]}
        """#))
        #expect(root.type == "Card")
        #expect(root["title"]?.string == "Release")
        #expect(root.children.map(\.type) == ["Metric", "Steps"])
        // A number reads as words without ".0".
        #expect(root.children[0]["value"]?.string == "412")
        #expect(root.children[1]["items"]?.list?.first?["status"]?.string == "done")
    }

    @Test func anArrayIsAStackAndTheFlatFormIsRead() throws {
        let stack = try #require(UIBlock.parse(#"[{"type": "Heading", "props": {"text": "A"}}, {"type": "Badge", "props": {"text": "B"}}]"#))
        #expect(stack.type == "Stack")
        #expect(stack.children.map(\.type) == ["Heading", "Badge"])
        let flat = try #require(UIBlock.parse(#"""
        {"root": "c", "elements": {"c": {"type": "Card", "children": ["t", "x"]}, "t": {"type": "Text", "props": {"text": "hi"}}, "x": {"type": "Badge", "props": {"text": "ok", "tone": "success"}}}}
        """#))
        #expect(flat.type == "Card")
        #expect(flat.children.map(\.id) == ["t", "x"])
        #expect(flat.children[1]["tone"]?.string == "success")
    }

    @Test func whatCannotBeReadIsNotDrawn() {
        #expect(UIBlock.parse("not json") == nil)
        #expect(UIBlock.parse(#"{"props": {}}"#) == nil, "neither form")
        #expect(UIBlock.parse(#"{"type": "Card", "root": "a", "elements": {}}"#) == nil, "both forms")
        // Past the size limit.
        let big = #"{"type": "Text", "props": {"text": ""# + String(repeating: "x", count: 70_000) + #""}}"#
        #expect(UIBlock.parse(big) == nil)
        // An element that is its own ancestor is not followed for ever.
        let loop = UIBlock.parse(#"{"root": "a", "elements": {"a": {"type": "Stack", "children": ["a"]}}}"#)
        #expect(loop?.children.isEmpty == true)
    }

    @Test func picturesAreFoundInBlocksAndMarkdown() {
        let text = """
        Done.

        ```myco-ui
        {"type": "Gallery", "props": {"items": [{"src": "attachment:a.png"}, {"src": "https://example.com/b.png"}]}}
        ```

        ![the screen](attachment:screen.png)
        """
        #expect(MarkdownBlock.placedPictures(in: text) == ["attachment:a.png", "https://example.com/b.png", "attachment:screen.png"])
        // A picture on a line of its own is a block; one in a sentence is not.
        #expect(MarkdownBlock.parse("![x](attachment:x.png)") == [.image(alt: "x", src: "attachment:x.png")])
        #expect(MarkdownBlock.parse("see ![x](attachment:x.png) here").first == .paragraph("see ![x](attachment:x.png) here"))
    }

    @Test func aPictureIsFoundByNameOrID() {
        let pictures = MessagePictures(names: ["首页.png": "f_1", "f_1": "f_1"]) { _ in URL(fileURLWithPath: "/") }
        #expect(pictures.file(of: "attachment:首页.png") == "f_1")
        #expect(pictures.file(of: "attachment:%E9%A6%96%E9%A1%B5.png") == "f_1")
        #expect(pictures.file(of: "attachment:f_1") == "f_1")
        #expect(pictures.file(of: "attachment:other.png") == nil)
        #expect(pictures.file(of: "https://example.com/a.png") == nil)
    }

    @Test func codeIsColouredWhereItsLanguageIsKnown() {
        func colored(_ code: String, _ language: String) -> [String] {
            let text = CodeColors.highlight(code, language: language)
            return text.runs.compactMap { run in run.foregroundColor == nil ? nil : String(text[run.range].characters) }
        }
        let swift = colored("let title = \"if\" // note", "swift")
        #expect(swift.contains("let"))
        #expect(swift.contains("\"if\""), "a keyword inside a string stays a string")
        #expect(swift.contains("// note"))
        #expect(!swift.contains("if"))
        #expect(colored("SELECT name FROM users", "sql").contains("SELECT"))
        #expect(colored("{\"a\": 1}", "json").contains("\"a\":") == false)
        #expect(colored("{\"a\": 1}", "json").contains("\"a\""))
        // An unknown language stays plain.
        #expect(colored("let x = 1", "brainfuck").isEmpty)
    }
}
