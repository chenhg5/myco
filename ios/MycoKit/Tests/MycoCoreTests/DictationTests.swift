import Foundation
import Testing
@testable import MycoCore

/// Speaking instead of typing: where heard words go, and what stops it.
@MainActor
struct DictationTests {
    @MainActor
    final class Box {
        var text: String
        init(_ text: String = "") { self.text = text }
    }

    /// Waits until something is true, a second at most.
    func until(_ condition: @MainActor () -> Bool) async {
        for _ in 0..<100 where !condition() {
            try? await Task.sleep(for: .milliseconds(10))
        }
    }

    /// The same cases as the desktop app's (internal/console/dictation_test.go).
    @Test func aSentenceAfterAPauseGoesAfterWhatCameBefore() {
        var chinese = Utterances()
        for (heard, all) in [
            ("帮", "帮"), ("帮我看", "帮我看"), ("帮我看一下下载页", "帮我看一下下载页"),
            // A pause: the recognizer starts over with the next sentence.
            ("然后", "帮我看一下下载页然后"), ("然后改一下标题", "帮我看一下下载页然后改一下标题"),
        ] {
            #expect(chinese.add(heard) == all)
        }
        var english = Utterances()
        for (heard, all) in [
            ("Please", "Please"), ("Please check the", "Please check the"),
            ("Please check the download page", "Please check the download page"),
            ("And", "Please check the download page And"), ("And fix the title", "Please check the download page And fix the title"),
            // A revision of the same sentence is not a new one.
            ("And fix the titles", "Please check the download page And fix the titles"),
        ] {
            #expect(english.add(heard) == all)
        }
    }

    @Test func aSessionThatEndedKeepsItsWords() {
        // The recognizer's session ended at a pause; the next one starts
        // from nothing, even with the same first words.
        var heard = Utterances()
        #expect(heard.add("Check the page") == "Check the page")
        heard.commit()
        #expect(heard.add("Check") == "Check the page Check")
        #expect(heard.add("Check the title") == "Check the page Check the title")
        heard.commit()
        heard.commit()
        #expect(heard.add("好") == "Check the page Check the title好")
        var chinese = Utterances()
        _ = chinese.add("帮我看一下")
        chinese.commit()
        #expect(chinese.add("帮我改一下") == "帮我看一下帮我改一下")
    }

    /// The desktop app's cases (TestUtterancesFinalRewritesTheStart,
    /// TestUtterancesRevisionIsNotANewSentence).
    @Test func stoppingDoesNotWriteTheSentenceTwice() {
        var heard = Utterances()
        _ = heard.add("我们先看一下日志")
        heard.commit()
        _ = heard.add("妹那边")
        _ = heard.add("妹那边那妹妹那边有做了一些改动")
        _ = heard.add("妹那边那妹妹那边有做了一些改动然后可能你可以参照着Mac那边的改动看移动端这边也能不能一起改一下")
        // On stopping, the sentence as the recognizer settled on it, its
        // first words rewritten: it takes the place of what was shown.
        let settled = "妹可那边有做了一些改动然后可能你可以参照着Mac那边的改动看移动端这边也能不能一起改一下"
        #expect(heard.add(settled, final: true) == "我们先看一下日志" + settled)
        // Its session ends, and the same sentence comes once more.
        heard.commit()
        #expect(heard.add(settled, final: true) == "我们先看一下日志" + settled)

        // A text that rewrites the start of a long sentence and is about
        // as long is the same sentence, not a new one.
        var revised = Utterances()
        _ = revised.add("那妹妹那边有做了一些改动")
        #expect(revised.add("妹可那边有做了一些改动吧") == "妹可那边有做了一些改动吧")
    }

    @Test func heardWordsGoAfterWhatWasTyped() async {
        let listener = ScriptedSpeechListener(
            ["帮", "帮我看一下下载页", "然后", "然后改一下标题"], pause: .milliseconds(15), endsItself: true)
        let dictation = Dictation(listener: listener)
        let box = Box("先记一下：")
        AppLanguage.$forced.withValue(.zh) {
            dictation.toggle(typed: { box.text }, write: { box.text = $0 })
        }
        #expect(dictation.listening)
        await until { !dictation.listening }
        #expect(box.text == "先记一下：帮我看一下下载页然后改一下标题")
        #expect(dictation.problem == nil)

        // In a language that puts spaces between words, one goes in first.
        let english = Dictation(listener: ScriptedSpeechListener(["check", "check the page"], pause: .milliseconds(15), endsItself: true))
        let note = Box("Note:")
        AppLanguage.$forced.withValue(.en) {
            english.toggle(typed: { note.text }, write: { note.text = $0 })
        }
        await until { !english.listening }
        #expect(note.text == "Note: check the page")
        #expect(Dictation.base(after: "", language: .en) == "")
        #expect(Dictation.base(after: "done. ", language: .en) == "done. ")
    }

    @Test func typingWhileItListensStopsIt() async {
        let listener = ScriptedSpeechListener(["一", "一二", "一二三", "一二三四", "一二三四五", "一二三四五六"], pause: .milliseconds(40))
        let dictation = Dictation(listener: listener)
        let box = Box()
        dictation.toggle(typed: { box.text }, write: { box.text = $0 })
        await until { !box.text.isEmpty }
        // The person takes over: sends, or types something else.
        box.text = ""
        dictation.textChanged(to: box.text)
        #expect(!dictation.listening)
        #expect(listener.stops >= 1)
        try? await Task.sleep(for: .milliseconds(150))
        // Nothing late is written into the emptied box.
        #expect(box.text.isEmpty)
        // What it writes itself is not "typing".
        let again = Dictation(listener: ScriptedSpeechListener(["好的"], pause: .milliseconds(15)))
        again.toggle(typed: { box.text }, write: { box.text = $0 })
        await until { box.text == "好的" }
        again.textChanged(to: box.text)
        #expect(again.listening)
    }

    @Test func tappingAgainStopsAndKeepsWhatWasHeard() async {
        let listener = ScriptedSpeechListener(["你好"], pause: .milliseconds(15))
        let dictation = Dictation(listener: listener)
        let box = Box()
        dictation.toggle(typed: { box.text }, write: { box.text = $0 })
        await until { box.text == "你好" }
        #expect(dictation.listening)
        dictation.toggle(typed: { box.text }, write: { box.text = $0 })
        await until { !dictation.listening }
        #expect(!dictation.listening)
        #expect(box.text == "你好")
    }

    @Test func aMissingPermissionSaysSo() async {
        let dictation = Dictation(listener: ScriptedSpeechListener([], problem: .microphoneDenied))
        let box = Box("hi")
        dictation.toggle(typed: { box.text }, write: { box.text = $0 })
        await until { dictation.problem != nil }
        #expect(dictation.problem == .microphoneDenied)
        #expect(!dictation.listening)
        #expect(box.text == "hi")
    }
}
