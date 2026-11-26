package console

import (
	"context"
	"sync"
	"testing"
	"time"
)

// polishModel answers every correction with the same text, after a while.
type polishModel struct {
	mu     sync.Mutex
	answer string
	after  time.Duration
	asked  []string
	err    error
}

func (m *polishModel) Stream(ctx context.Context, msgs []chatMsg, _ []toolDef, _ func(string), _ func(string)) (chatMsg, error) {
	m.mu.Lock()
	m.asked = append(m.asked, msgs[len(msgs)-1].Content)
	answer, after, err := m.answer, m.after, m.err
	m.mu.Unlock()
	if after > 0 {
		select {
		case <-time.After(after):
		case <-ctx.Done():
			return chatMsg{}, ctx.Err()
		}
	}
	return chatMsg{Role: "assistant", Content: answer}, err
}

var polishWords = []string{"Claude", "Codex", "GitHub", "DeepSeek", "myco", "Kettu", "PR", "前端小王"}

func TestOnlyNameFixes(t *testing.T) {
	for _, tc := range []struct{ heard, model, want string }{
		// Names put where the recognizer wrote something else.
		{"让包扣看一下这个 PR", "让Codex看一下这个 PR", "让Codex看一下这个 PR"},
		{"让 CLOU 看一下", "让 Claude 看一下", "让 Claude 看一下"},
		// A word people say is not replaced, even when the model would ("句号"
		// is in the negative set; Doubao's hot words are for this one).
		{"把代码推到句号上", "把代码推到GitHub上", "把代码推到句号上"},
		{"Open Michael and check the report", "Open myco and check the report", "Open Michael and check the report"},
		{"The cloud server is running", "The Claude server is running", "The cloud server is running"},
		{"在 MICHAEL 里交给 CAT TO", "在 myco 里交给 Kettu", "在 myco 里交给 Kettu"},
		{"用 DIP SIP 跑一下", "用 DeepSeek 跑一下", "用 DeepSeek 跑一下"},
		{"发给 CAT TO", "发给 Kettu", "发给 Kettu"},
		// Part of a name changed: the name as a whole, with what is the same.
		{"让前端小玩把说明写一下", "让前端小王把说明写一下", "让前端小王把说明写一下"},
		// Everything else the model did is dropped.
		{"让包扣看一下这个 PR", "请让Codex帮忙看一下这个 PR。", "让Codex看一下这个 PR"},
		{"我妹妹明天来", "我 Kettu 明天来", "我妹妹明天来"},
		{"今天天气不错", "今天天气很好", "今天天气不错"},
		{"把代码推上去", "把代码推到 GitHub 上去", "把代码推上去"},
		{"看一下这个", "", "看一下这个"},
		// A long run replaced by a short name: not a misheard name.
		{"这是一段很长很长的说明文字结尾", "这是Claude结尾", "这是一段很长很长的说明文字结尾"},
	} {
		if got := onlyNameFixes(tc.heard, tc.model, polishWords); got != tc.want {
			t.Errorf("%q + model %q → %q, want %q", tc.heard, tc.model, got, tc.want)
		}
	}
}

func TestSpeechPolishFix(t *testing.T) {
	m := &polishModel{answer: "让Codex看一下"}
	p := &speechPolish{model: m, words: polishWords, lang: "zh-CN"}
	if got, ok := p.Fix(context.Background(), "让包扣看一下"); !ok || got != "让Codex看一下" {
		t.Fatalf("fix: %q %v", got, ok)
	}
	// Too slow: what was heard stays, and nothing is said.
	m.after = polishTimeout + 500*time.Millisecond
	start := time.Now()
	if got, ok := p.Fix(context.Background(), "让包扣看一下"); ok || got != "让包扣看一下" {
		t.Fatalf("slow: %q %v", got, ok)
	}
	if d := time.Since(start); d > polishTimeout+300*time.Millisecond {
		t.Fatalf("waited %v", d)
	}
	// No corrector: nothing.
	var none *speechPolish
	if got, ok := none.Fix(context.Background(), "让包扣看一下"); ok || got != "让包扣看一下" {
		t.Fatal("nil corrector")
	}
}

func TestSpeechPolisherFollowsSettings(t *testing.T) {
	b := &Backend{Secrets: &memKeys{m: map[string]string{}}}
	v := b.voiceState()
	v.mem = VoiceConfig{LLMBaseURL: "https://api.example.com/v1", LLMModel: "fast"}
	if b.speechPolisher("zh-CN", polishWords) != nil {
		t.Fatal("no model key: no corrector")
	}
	_ = b.Secrets.Set(voiceLLMSecret, "k")
	if b.speechPolisher("zh-CN", polishWords) == nil {
		t.Fatal("a model: a corrector (on unless turned off)")
	}
	off := false
	v.mem.Polish = &off
	if b.speechPolisher("zh-CN", polishWords) != nil {
		t.Fatal("turned off")
	}
}

// A call's line is out (and answered) at once; the correction follows.
func TestVoiceCallPolishesInTheBackground(t *testing.T) {
	m := &scriptModel{replies: []chatMsg{{Role: "assistant", Content: "好的。"}}}
	pm := &polishModel{answer: "让Codex看一下", after: 150 * time.Millisecond}
	v, c, _ := newTestCall(t, m, &fakeHub{}, func(d *voiceDeps) {
		d.polish = &speechPolish{model: pm, words: polishWords, lang: "zh-CN"}
	})
	c.Heard("让包扣看一下", true)
	waitFor(t, "the line as heard", func() bool { return has(v, vUser, "让包扣看一下") })
	waitFor(t, "the corrected line", func() bool {
		for _, e := range lines(v) {
			if e.Kind == vUser && e.Text == "让Codex看一下" && e.Corrected == "让包扣看一下" {
				return true
			}
		}
		return false
	})
	// The answer did not wait for it: the model saw the line as heard.
	m.mu.Lock()
	first := m.seen[0]
	m.mu.Unlock()
	if first[len(first)-1].Content != "让包扣看一下" {
		t.Fatalf("the turn waited for the correction: %+v", first[len(first)-1])
	}
}

func TestHearingPolishes(t *testing.T) {
	h := newHearing()
	h.polish = &speechPolish{model: &polishModel{answer: "问一下Claude", after: 50 * time.Millisecond}, words: polishWords, lang: "zh-CN"}
	h.Heard("问一下 CLOU", false)
	if st := h.state(); st.Polishing || st.Corrected != nil {
		t.Fatalf("not final: %+v", st)
	}
	h.Heard("问一下CLOU", true)
	h.End("")
	if st := h.state(); !st.Polishing || st.Text != "问一下CLOU" {
		t.Fatalf("heard text out at once, correction under way: %+v", st)
	}
	st := h.wait(context.Background(), h.state().Seq, 2*time.Second)
	if st.Polishing || st.Corrected == nil || st.Corrected.From != "问一下CLOU" || st.Corrected.To != "问一下Claude" {
		t.Fatalf("corrected: %+v", st)
	}
}
