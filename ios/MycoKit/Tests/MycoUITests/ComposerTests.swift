import Testing
@testable import MycoUI

/// The input's rules that are not drawing.
@MainActor
struct ComposerTests {
    @Test func tellsATypedReturnFromAnyOtherChange() {
        // A Return typed at the end, or in the middle: the text before it.
        #expect(ComposerView.withoutTypedReturn(old: "hello", new: "hello\n") == "hello")
        #expect(ComposerView.withoutTypedReturn(old: "ab", new: "a\nb") == "ab")
        #expect(ComposerView.withoutTypedReturn(old: "", new: "\n") == "")
        // A line that already had breaks keeps them.
        #expect(ComposerView.withoutTypedReturn(old: "a\nb", new: "a\nb\n") == "a\nb")
        // Typing a letter, deleting, pasting several lines, or words
        // arriving from dictation is not a Return.
        #expect(ComposerView.withoutTypedReturn(old: "hello", new: "hello!") == nil)
        #expect(ComposerView.withoutTypedReturn(old: "hello", new: "hell") == nil)
        #expect(ComposerView.withoutTypedReturn(old: "", new: "one\ntwo") == nil)
        #expect(ComposerView.withoutTypedReturn(old: "帮我看", new: "帮我看一下") == nil)
    }
}
