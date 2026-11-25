package console

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// An evaluation of the voice against a real model: fixed conversations run
// through a real call (the real instructions, tools, receipts and news),
// checking that the agent speaks of its work as its own, in the first
// person, and never of "the agent" or "it" as someone else. It runs only
// when a model is given (it costs a few requests per scenario):
//
//	VOICE_EVAL_BASE_URL=https://api.deepseek.com/v1 VOICE_EVAL_MODEL=deepseek-chat \
//	VOICE_EVAL_KEY=... scripts/voice-eval.sh
//
// VOICE_EVAL_RUNS repeats each scenario (the model is sampled, so a slip
// may show only now and then); VOICE_EVAL_LANG limits it to zh or en.
// Without a model, TestVoiceThirdPersonCheck still checks the checker.

// thirdPerson returns what in a spoken line talks about the agent as
// someone else (empty: the line speaks as the agent).
func thirdPerson(lang, text string) []string {
	var found []string
	if strings.HasPrefix(lang, "en") {
		low := strings.ToLower(text)
		for _, re := range enThirdPerson {
			if m := re.FindString(low); m != "" {
				found = append(found, m)
			}
		}
		return found
	}
	for _, w := range zhThirdPerson {
		if strings.Contains(text, w) {
			found = append(found, w)
		}
	}
	if strings.Contains(strings.ToLower(text), "agent") {
		found = append(found, "agent")
	}
	return found
}

// "它" alone is fine ("我把它修好了": the bug); these are it as a worker.
var zhThirdPerson = []string{
	"它在", "它正在", "它说", "它回", "它做", "它问", "它已经", "它手上", "它那边", "它还", "它刚",
	"让它", "跟它", "告诉它", "叫它", "给它", "转给", "转告", "转达", "我让", "助手",
	"后端那边", "后端说", "后端在", "后端已经", "后端回",
}

var enThirdPerson = []*regexp.Regexp{
	regexp.MustCompile(`\bthe agent\b`), regexp.MustCompile(`\bagent (is|has|was|says|said)\b`),
	regexp.MustCompile(`\b(tell|ask|let) it\b`), regexp.MustCompile(`\bpass (it|that|this) (on|along)\b`),
	regexp.MustCompile(`\bit('s| is| was| has) (still |now )?(working|running|on it)\b`),
	regexp.MustCompile(`\bit (finished|found|replied|says|said|asked|changed|fixed|wants)\b`),
	regexp.MustCompile(`\bassistant\b`), regexp.MustCompile(`\bbackend (is|has|says|said|found|finished)\b`),
}

func TestVoiceThirdPersonCheck(t *testing.T) {
	for _, tc := range []struct {
		lang, text string
		bad        bool
	}{
		{"zh-CN", "好，我跟它说。", true},
		{"zh-CN", "我让后端去修了。", true},
		{"zh-CN", "它回复了，测试都过了。", true},
		{"zh-CN", "agent 还在跑测试。", true},
		{"zh-CN", "好，我这就去修，修好告诉你。", false},
		{"zh-CN", "我把它修好了，测试都过了。", false},
		{"en-US", "Sure, I'll tell it.", true},
		{"en-US", "The agent is still working on it.", true},
		{"en-US", "It finished the tests.", true},
		{"en-US", "Sure, I'm on it. I'll let you know when it's done.", false},
		{"en-US", "I fixed it, and all the tests pass.", false},
	} {
		if got := thirdPerson(tc.lang, tc.text); (len(got) > 0) != tc.bad {
			t.Errorf("%s %q: found %v, want bad=%v", tc.lang, tc.text, got, tc.bad)
		}
	}
}

// evalHub is the agent's side for a scenario: its chat as read, the task at
// work, and the news about it.
type evalHub struct {
	mu      sync.Mutex
	name    string
	convo   string
	current *VoiceTaskRef
	news    taskNews
	told    []string
	stopped bool
}

func (h *evalHub) Name() string                                   { return h.name }
func (h *evalHub) Context(context.Context) (string, error)        { return h.convo, nil }
func (h *evalHub) Current(context.Context) (*VoiceTaskRef, error) { return h.current, nil }
func (h *evalHub) Tell(_ context.Context, body string) (VoiceTaskRef, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.told = append(h.told, body)
	return VoiceTaskRef{ID: "msg_e", Agent: "backend", Thread: "th_e"}, h.current != nil, nil
}
func (h *evalHub) StopCurrent(context.Context) (VoiceTaskRef, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopped = true
	return VoiceTaskRef{ID: "msg_e"}, nil
}
func (h *evalHub) TaskNews(context.Context, VoiceTaskRef) (taskNews, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.news, nil
}

type evalScenario struct {
	name string
	// The person's words (none: the turn is the hello or news).
	say   string
	greet bool
	hub   func() *evalHub
	// tool the model should use ("" for none checked).
	tool string
}

func evalScenarios(en bool) []evalScenario {
	L := func(zh, e string) string {
		if en {
			return e
		}
		return zh
	}
	name := L("后端", "backend")
	idle := func() *evalHub {
		return &evalHub{name: name, convo: L(
			"你是：backend（后端），修 bug。现在你空着\n\n对话里最近的消息（旧的在前；「你」是你自己，「对方」是正在和你通话的这个人）：\n- 09:40 对方（任务）：看看注册页为什么慢\n- 09:52 你（回复）：注册页慢是因为头像图片太大，我压缩了，现在 0.8 秒打开。\n\n你手上没有对方交的活。\n",
			"You are: backend, fixes bugs. Right now you are free\n\nThe latest messages in your chat (oldest first; \"You\" is you, \"They\" is the person on the call):\n- 09:40 They (task): find out why the sign-up page is slow\n- 09:52 You (reply): The sign-up page was slow because the avatar images were too big; I compressed them and it opens in 0.8 s now.\n\nYou have nothing of theirs at work.\n")}
	}
	ref := &VoiceTaskRef{ID: "msg_w", Agent: "backend", Thread: "th_w"}
	working := func() *evalHub {
		return &evalHub{name: name, current: ref, convo: L(
			"你是：backend（后端），修 bug。现在你在干活\n\n对话里最近的消息（旧的在前；「你」是你自己，「对方」是正在和你通话的这个人）：\n- 10:00 对方（任务）：把整个项目的测试跑一遍，挂了的修掉\n- 10:06 你（进展）：单元测试全过了，在跑集成测试，过了 30 个，还剩 12 个。\n\n你手上正在做对方交的：把整个项目的测试跑一遍，挂了的修掉（在做）\n",
			"You are: backend, fixes bugs. Right now you are at work\n\nThe latest messages in your chat (oldest first; \"You\" is you, \"They\" is the person on the call):\n- 10:00 They (task): run all the project's tests and fix any that fail\n- 10:06 You (progress): Unit tests all pass; running the integration tests, 30 passed, 12 to go.\n\nYou are working on what they asked: run all the project's tests and fix any that fail (in progress)\n")}
	}
	return []evalScenario{
		{name: "hello", greet: true, hub: working},
		{name: "new work", say: L("帮我把登录页那个报错修了", "Can you fix the error on the login page?"), hub: idle, tool: "do_work"},
		{name: "addition", say: L("对了，修完顺便把超时调成三十秒", "Oh, and when you're done, set the timeout to thirty seconds"), hub: working, tool: "do_work"},
		{name: "how far", say: L("做到哪了？", "How far along are you?"), hub: working},
		{name: "what changed", say: L("注册页那个你最后怎么弄的？", "What did you end up doing about the sign-up page?"), hub: idle},
		{name: "stop", say: L("先停一下吧，别跑了", "Stop for now, don't run them"), hub: working, tool: "stop_work"},
		{name: "finished", hub: func() *evalHub {
			h := working()
			h.news = taskNews{Finished: true, Summary: L("测试都跑完了：42 个全过，我顺手修了一个超时的用例。", "All tests ran: 42 pass, and I fixed one test that timed out along the way.")}
			return h
		}},
		{name: "question", hub: func() *evalHub {
			h := working()
			h.news = taskNews{Status: "acked", Question: L("有个旧接口 /v1/login 测试一直挂，要删掉它还是修好？", "An old endpoint, /v1/login, keeps failing its tests: should I remove it or fix it?")}
			return h
		}},
	}
}

func TestVoiceEvalRealModel(t *testing.T) {
	base, model, key := os.Getenv("VOICE_EVAL_BASE_URL"), os.Getenv("VOICE_EVAL_MODEL"), os.Getenv("VOICE_EVAL_KEY")
	if base == "" || model == "" || key == "" {
		t.Skip("set VOICE_EVAL_BASE_URL, VOICE_EVAL_MODEL and VOICE_EVAL_KEY to run the voice against a real model")
	}
	runs := 1
	if n, err := strconv.Atoi(os.Getenv("VOICE_EVAL_RUNS")); err == nil && n > 0 {
		runs = n
	}
	langs := []string{"zh-CN", "en-US"}
	switch os.Getenv("VOICE_EVAL_LANG") {
	case "zh":
		langs = langs[:1]
	case "en":
		langs = langs[1:]
	}
	m := &openAIModel{base: base, model: model, key: key, session: "myco-voice-eval", http: &http.Client{Timeout: 60 * time.Second}}
	for _, lang := range langs {
		for _, sc := range evalScenarios(strings.HasPrefix(lang, "en")) {
			for run := 1; run <= runs; run++ {
				t.Run(lang+"/"+sc.name+"/"+strconv.Itoa(run), func(t *testing.T) { runEvalScenario(t, m, lang, sc) })
			}
		}
	}
}

func runEvalScenario(t *testing.T, m voiceModel, lang string, sc evalScenario) {
	h := sc.hub()
	v := &voice{state: stIdle, changed: make(chan struct{})}
	a := &fakeAudio{}
	c := newVoiceCall(v, voiceDeps{audio: a, model: m, tts: fakeSpeech{}, hub: h, lang: lang, greet: sc.greet,
		endOfSpeech: 80 * time.Millisecond, quietBeforeNotice: 50 * time.Millisecond, watchEvery: 50 * time.Millisecond, stillWorkingEvery: time.Hour})
	v.call = c
	_ = a.Start(lang, c)
	c.begin()
	defer c.hangUp()
	if sc.say != "" {
		c.Heard(sc.say, true)
	}
	// A turn starts (now, or once the news is in) and ends.
	deadline := time.Now().Add(90 * time.Second)
	started := false
	for {
		if time.Now().After(deadline) {
			t.Fatalf("no answer in time: %+v", lines(v))
		}
		busy := make(chan bool, 1)
		c.do(func() {
			busy <- c.turn != nil
			c.speaking = false // fakeAudio does not say when it has played
		})
		// (a quick model can start and end a turn between two looks: a
		// line said counts as started)
		if <-busy {
			started = true
		} else if started || hasSaid(v) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	var said []string
	time.Sleep(100 * time.Millisecond) // the last sentence's update
	for _, e := range lines(v) {
		if e.Kind == vAssistant && strings.TrimSpace(e.Text) != "" {
			said = append(said, e.Text)
		}
		if e.Kind == vNotice {
			t.Errorf("notice: %s", e.Text)
		}
	}
	text := strings.Join(said, " ")
	t.Logf("said: %s", text)
	if text == "" {
		t.Fatalf("said nothing: %+v", lines(v))
	}
	if bad := thirdPerson(lang, text); len(bad) > 0 {
		t.Errorf("speaks of the agent as someone else %v: %s", bad, text)
	}
	h.mu.Lock()
	told, stopped := len(h.told), h.stopped
	h.mu.Unlock()
	switch sc.tool {
	case "do_work":
		if told == 0 {
			t.Errorf("did not take the work on (do_work): %s", text)
		}
	case "stop_work":
		if !stopped {
			t.Errorf("did not stop (stop_work): %s", text)
		}
	}
}

// The evaluation's own workings, with a scripted model standing in for a
// real one: every scenario gets an answer, and the tools are checked.
func TestVoiceEvalHarness(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en-US"} {
		en := strings.HasPrefix(lang, "en")
		for _, sc := range evalScenarios(en) {
			ok := "好，我去弄，弄好告诉你。"
			if en {
				ok = "Sure, I'm on it; I'll tell you when I'm done."
			}
			var replies []chatMsg
			if sc.tool != "" {
				call := toolCall{ID: "c1", Type: "function"}
				call.Function.Name, call.Function.Arguments = sc.tool, `{"text":"x","what":"x"}`
				replies = append(replies, chatMsg{Role: "assistant", ToolCalls: []toolCall{call}})
			}
			replies = append(replies, chatMsg{Role: "assistant", Content: ok})
			sc := sc
			t.Run(lang+"/"+sc.name, func(t *testing.T) { runEvalScenario(t, &scriptModel{replies: replies}, lang, sc) })
		}
	}
}

func hasSaid(v *voice) bool {
	for _, e := range lines(v) {
		if e.Kind == vAssistant && strings.TrimSpace(e.Text) != "" {
			return true
		}
	}
	return false
}
