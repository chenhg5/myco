import Foundation
import Testing
@testable import MycoCore

/// The words speech recognition listens for, and how what it hears is
/// corrected: the same cases as the desktop app's speech_words_test.go.
struct SpeechWordsTests {
    @Test func thePersonsWordsAreRead() {
        let parsed = SpeechWords.parse(["  ", "Claude = 克劳德， cloud", "前端小王", "Kettu＝凯图、kettle"])
        #expect(parsed.words == ["Claude", "前端小王", "Kettu"])
        // "cloud" is a word people say: not taken as an alias.
        #expect(parsed.aliases["Claude"] == ["克劳德"])
        #expect(parsed.aliases["Kettu"] == ["凯图", "kettle"])
    }

    @Test func chineseIsCorrectedForTheWordsOnly() {
        let fixer = SpeechFixer(language: "zh-CN", words: ["Kettu", "pm codex"] + SpeechWords.common, extra: ["Kettu": ["凯图"]])
        let cases: [String: String] = [
            "让克劳德看一下": "让Claude看一下",
            "让 P R 合进去": "让 PR 合进去",
            "a p i 文档": "API 文档",
            "C.I. 挂了": "CI 挂了",
            "把代码推到 github 上": "把代码推到 GitHub 上",
            "用 deep seek 跑一下": "用 DeepSeek 跑一下",
            "问问扣德克斯": "问问Codex",
            "pr 合了吗，ci 过了没": "PR 合了吗，CI 过了没",
            "凯图那边怎么样": "Kettu那边怎么样",
            "打开 macos 的设置": "打开 macOS 的设置",
            "claude code 装好了": "Claude Code 装好了",
            "IOS 上 PUSH 一下": "iOS 上 push 一下",
            // Not words of their own: left alone.
            "cloud-hdkwmcki 在干嘛": "cloud-hdkwmcki 在干嘛",
            "print 一下 prompt": "print 一下 prompt",
            "部署到 cloud server 上": "部署到 cloud server 上",
            "今天天气不错": "今天天气不错",
        ]
        for (said, meant) in cases {
            #expect(fixer.fix(said) == meant, "\(said)")
        }
    }

    @Test func englishLeavesPlainWordsAlone() {
        let fixer = SpeechFixer(language: "en-US", words: SpeechWords.common)
        let cases: [String: String] = [
            "push it to github and open a pr": "push it to GitHub and open a PR",
            "ask deep seek": "ask DeepSeek",
            "move the cursor to the windows": "move the cursor to the windows",
            "store it in the cloud": "store it in the cloud",
            "let's go": "let's go",
        ]
        for (said, meant) in cases {
            #expect(fixer.fix(said) == meant, "\(said)")
        }
        // The person's own aliases apply in English too.
        let mine = SpeechFixer(language: "en-US", words: ["Claude"], extra: ["Claude": ["clod"]])
        #expect(mine.fix("ask clod about it") == "ask Claude about it")
    }

    /// What people mean is not changed: real words, however much they
    /// sound like a name (the desktop's negative set and its six controls,
    /// internal/console/testdata/speech-negatives.json).
    static let negatives = [
        "句号", "我妹妹明天来", "half an hour", "那只 cat 很可爱", "a cat on the mat",
        "上传到 cloud 里", "cloud server 挂了", "store it in the cloud", "my co-worker 说的",
        "把 codec 换一下", "codecs 不支持", "git 提交一下", "hub 上的灯",
        "go 一下", "print 一下 prompt", "让他 call 我", "这个 cool 吧",
        "今天天气不错", "我们开会吧", "包裹到了", "扣子掉了", "克拉的钻石", "猫咪睡着了",
        "这里加一个句号", "The cloud server is running", "Open Michael and check the report",
        "先准备一下妹妹的作业", "The cat sat on the mat", "Give me half of the result",
    ]

    @Test func whatPeopleMeanIsLeftAlone() {
        let words = ["Kettu", "pm codex", "前端小王"] + SpeechWords.common
        for language in ["zh-CN", "en-US"] {
            let fixer = SpeechFixer(language: language, words: words)
            for said in Self.negatives {
                #expect(fixer.fix(said) == said, "\(language): \(said)")
            }
        }
    }

    @Test func unsafeAliasesAreNotUsed() {
        for alias in ["句号", "妹妹", "half", "cat", "cloud", "猫", "ab", "my"] {
            #expect(SpeechWords.unsafeAlias(alias) != nil, "\(alias)")
        }
        for alias in ["克劳德", "凯图", "kettle", "CLOU"] {
            #expect(SpeechWords.unsafeAlias(alias) == nil, "\(alias)")
        }
    }

    @Test func onlyNamesAPersonSaysAreListenedFor() {
        #expect(SpeechWords.speakable(["cloud-hdkwmcki", "invited-m9ixux9a", "pm-codex", "后端", "Review (Codex 云端)"])
            == ["pm-codex", "pm codex", "后端", "Review (Codex 云端)"])
    }

    @Test func thePersonsWordsComeFirstEachOnceAtMostAHundred() {
        let vocabulary = SpeechWords.vocabulary(mine: ["Kettu = 凯图", "claude"], workspace: ["@console:glenn", "运维", "myco-ios"])
        #expect(Array(vocabulary.words.prefix(4)) == ["Kettu", "claude", "运维", "myco-ios"])
        #expect(vocabulary.aliases["Kettu"] == ["凯图"])
        #expect(!vocabulary.words.dropFirst(2).contains { $0.lowercased() == "claude" }, "a word twice")
        #expect(!vocabulary.words.contains("@console:glenn"))
        let many = SpeechWords.vocabulary(mine: (0..<150).map { "词\($0)" }, workspace: [])
        #expect(many.words.count == SpeechWords.maxWords)
    }

    @Test func theRecognizersGetTheWordsAndTheirCorrector() {
        let vocabulary = SpeechVocabulary()
        vocabulary.setMine(["Kettu = 凯图"])
        vocabulary.setWorkspace(["运维"])
        let zh = vocabulary.current(language: "zh-CN")
        #expect(zh.words.prefix(2) == ["Kettu", "运维"])
        #expect(zh.fixer.fix("问问凯图和克劳德") == "问问Kettu和Claude")
        // In English, "cloud" stays a cloud.
        #expect(vocabulary.current(language: "en-US").fixer.fix("the cloud") == "the cloud")
    }
}
