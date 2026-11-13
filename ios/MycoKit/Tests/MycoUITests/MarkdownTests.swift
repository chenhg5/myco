import Testing
@testable import MycoUI

struct MarkdownTests {
    @Test func splitsAnAnswerIntoItsBlocks() {
        let blocks = MarkdownBlock.parse("""
        ## Result

        Two lines
        of one paragraph.

        1. first
        2) second
           continued
        - [x] done
        - [ ] not yet

        ```swift
        let x = 1

        print(x)
        ```

        > quoted
        > twice

        ---
        """)
        #expect(blocks.count == 6)
        #expect(blocks[0] == .heading(level: 2, text: "Result"))
        #expect(blocks[1] == .paragraph("Two lines\nof one paragraph."))
        guard case .list(let items) = blocks[2] else {
            Issue.record("the third block is a list")
            return
        }
        #expect(items.map(\.marker) == ["1.", "2.", "", ""])
        #expect(items[1].text == "second\ncontinued")
        #expect(items.map(\.checked) == [nil, nil, true, false])
        // The step an agent is on ("[~]", "[-]") is a box too, in hand.
        guard case .list(let steps) = MarkdownBlock.parse("- [x] a\n- [~] b\n- [-] c\n- [ ] d").first else {
            Issue.record("a checklist is a list")
            return
        }
        #expect(steps.map(\.checked) == [true, false, false, false])
        #expect(steps.map(\.inHand) == [false, true, true, false])
        #expect(steps.map(\.text) == ["a", "b", "c", "d"])
        // A blank line inside a fence stays in the code.
        #expect(blocks[3] == .code(language: "swift", text: "let x = 1\n\nprint(x)"))
        #expect(blocks[4] == .quote("quoted\ntwice"))
        #expect(blocks[5] == .rule)
    }

    @Test func readsATable() {
        let blocks = MarkdownBlock.parse("| a | b |\n|---|:-:|\n| 1 | 2 |\n| 3 |\nafter")
        #expect(blocks == [.table(header: ["a", "b"], rows: [["1", "2"], ["3", ""]]), .paragraph("after")])
    }

    @Test func nestedItemsKeepTheirDepth() {
        guard case .list(let items) = MarkdownBlock.parse("- a\n  - b\n    - c").first else {
            Issue.record("a list")
            return
        }
        #expect(items.map(\.level) == [0, 1, 2])
    }

    @Test func whatItDoesNotKnowStaysText() {
        #expect(MarkdownBlock.parse("**bold** start\n#notaheading\na | b") == [.paragraph("**bold** start\n#notaheading\na | b")])
        // An unclosed fence takes the rest, and loses nothing.
        #expect(MarkdownBlock.parse("```\nabc") == [.code(language: "", text: "abc")])
        #expect(MarkdownBlock.parse("") == [])
    }
}

struct TimelineTests {
    @Test func theLineRunsFromTheFirstDotToTheLast() {
        #expect(ProgressLine.position(0, of: 1) == .only)
        #expect((0..<4).map { ProgressLine.position($0, of: 4) } == [.first, .middle, .middle, .last])
        #expect((0..<2).map { ProgressLine.position($0, of: 2) } == [.first, .last])
    }
}
