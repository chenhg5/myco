package console

import (
	"strings"
	"testing"
)

func TestParseSpeechWords(t *testing.T) {
	words, aliases := parseSpeechWords([]string{"  ", "Claude = 克劳德， cloud", "前端小王", "Kettu＝凯图、kettle"})
	if strings.Join(words, "|") != "Claude|前端小王|Kettu" {
		t.Fatalf("words: %v", words)
	}
	if strings.Join(aliases["Claude"], "|") != "克劳德|cloud" || strings.Join(aliases["Kettu"], "|") != "凯图|kettle" {
		t.Fatalf("aliases: %v", aliases)
	}
}

func TestSpeechFixerZh(t *testing.T) {
	words := append([]string{"Kettu", "pm codex"}, commonSpeechWords...)
	f := newSpeechFixer("zh-CN", words, map[string][]string{"Kettu": {"凯图"}})
	for in, want := range map[string]string{
		"让克劳德看一下":          "让Claude看一下",
		"让 cloud 看一下这个 PR": "让 Claude 看一下这个 PR",
		"Cloud帮我提交":        "Claude帮我提交",
		"把代码推到 github 上":   "把代码推到 GitHub 上",
		"用 deep seek 跑一下":  "用 DeepSeek 跑一下",
		"问问扣德克斯":           "问问Codex",
		"pr 合了吗，ci 过了没":    "PR 合了吗，CI 过了没",
		"在 my co 里发给它":     "在 myco 里发给它",
		"凯图那边怎么样":          "Kettu那边怎么样",
		"打开 macos 的设置":     "打开 macOS 的设置",
		"claude code 装好了":  "Claude Code 装好了",
		// Not words of their own: left alone.
		"cloud-hdkwmcki 在干嘛": "cloud-hdkwmcki 在干嘛",
		"print 一下 prompt":    "print 一下 prompt",
		"部署到 cloud server 上": "部署到 cloud server 上",
		"今天天气不错":             "今天天气不错",
	} {
		if got := f.Fix(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestSpeechFixerEnLeavesPlainWords(t *testing.T) {
	f := newSpeechFixer("en-US", commonSpeechWords, nil)
	for in, want := range map[string]string{
		"push it to github and open a pr": "push it to GitHub and open a PR",
		"ask deep seek":                   "ask DeepSeek",
		"move the cursor to the windows":  "move the cursor to the windows",
		"store it in the cloud":           "store it in the cloud",
		"let's go":                        "let's go",
	} {
		if got := f.Fix(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	// The person's own aliases apply in English too.
	f = newSpeechFixer("en-US", []string{"Claude"}, map[string][]string{"Claude": {"clod"}})
	if got := f.Fix("ask clod about it"); got != "ask Claude about it" {
		t.Fatalf("alias: %q", got)
	}
}

func TestSpeakableNames(t *testing.T) {
	got := speakable([]string{"cloud-hdkwmcki", "invited-m9ixux9a", "pm-codex", "后端", "Review (Codex 云端)"})
	if strings.Join(got, "|") != "pm-codex|pm codex|后端|Review (Codex 云端)" {
		t.Fatalf("speakable: %v", got)
	}
}

func TestSpeechVocabOrderAndLimit(t *testing.T) {
	b := &Backend{}
	v := b.voiceState()
	v.mem.Words = []string{"Kettu = 凯图", "claude"}
	words, aliases := b.speechVocab(t.Context())
	if words[0] != "Kettu" || words[1] != "claude" || len(aliases["Kettu"]) != 1 {
		t.Fatalf("the person's words first: %v %v", words[:3], aliases)
	}
	for _, w := range words[2:] {
		if strings.EqualFold(w, "claude") {
			t.Fatal("a word twice")
		}
	}
	if len(words) > maxSpeechWords {
		t.Fatalf("%d words", len(words))
	}
}

// The words reach the platform (VoiceHints) and what it hears is corrected.
func TestVoiceCallHintsAndFixes(t *testing.T) {
	m := &scriptModel{replies: []chatMsg{{Role: "assistant", Content: "好的。"}}}
	v, c, _ := newTestCall(t, m, &fakeHub{}, func(d *voiceDeps) {
		d.hints = []string{"Claude", "GitHub"}
		d.fix = newSpeechFixer("zh-CN", d.hints, nil)
	})
	var ev VoiceEvents = c
	if h, ok := ev.(VoiceHints); !ok || strings.Join(h.Hints(), "|") != "Claude|GitHub" {
		t.Fatal("the call should give the platform its words")
	}
	c.Heard("让克劳德看看 github", true)
	waitFor(t, "the line", func() bool { return has(v, vUser, "让Claude看看 GitHub") })
}

func TestHearingFixes(t *testing.T) {
	h := newHearing()
	h.fix = newSpeechFixer("zh-CN", commonSpeechWords, nil)
	h.Heard("问一下克劳德", true)
	if s := h.state(); s.Text != "问一下Claude" {
		t.Fatalf("dictation: %q", s.Text)
	}
}
