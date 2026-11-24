package console

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReadChatStreamTextAndTools(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"好的，"}}]}`,
		`data: {"choices":[{"delta":{"content":"交给后端。"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"send_task","arguments":"{\"to\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"backend\"}"}}]}}]}`,
		`data: [DONE]`,
	}, "\n\n")
	var got []string
	var tools []string
	m, err := readChatStream(strings.NewReader(stream), func(s string) { got = append(got, s) }, func(n string) { tools = append(tools, n) })
	if err != nil || m.Content != "好的，交给后端。" || strings.Join(got, "|") != "好的，|交给后端。" {
		t.Fatalf("text: %v %+v %v", err, m, got)
	}
	if strings.Join(tools, "|") != "send_task" {
		t.Fatalf("a tool's start is told once: %v", tools)
	}
	if len(m.ToolCalls) != 1 || m.ToolCalls[0].ID != "c1" || m.ToolCalls[0].Function.Name != "send_task" || m.ToolCalls[0].Function.Arguments != `{"to":"backend"}` {
		t.Fatalf("tools: %+v", m.ToolCalls)
	}
}

func TestReadDoubaoStreamKeepsSamplesWhole(t *testing.T) {
	enc := base64.StdEncoding.EncodeToString
	stream := `{"code":0,"data":"` + enc([]byte{1, 2, 3}) + `"}` + "\n" + `{"code":0,"data":"` + enc([]byte{4, 5, 6, 7, 8}) + `"}` + "\n" + `{"code":20000000}` + "\n"
	var all []byte
	err := readDoubaoStream(strings.NewReader(stream), func(pcm []byte, rate int) {
		if len(pcm)%2 != 0 || rate != doubaoRate {
			t.Fatalf("chunk %d bytes at %d", len(pcm), rate)
		}
		all = append(all, pcm...)
	})
	if err != nil || string(all) != string([]byte{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Fatalf("%v %v", err, all)
	}
	if err := readDoubaoStream(strings.NewReader(`{"code":45000000,"message":"bad key"}`+"\n"), func([]byte, int) {}); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("error frame: %v", err)
	}
	if err := readDoubaoStream(strings.NewReader(""), func([]byte, int) {}); err == nil {
		t.Fatal("a stream that never ends must be an error")
	}
}

func TestSentenceSplitter(t *testing.T) {
	s := &sentenceSplitter{}
	var out []string
	for _, d := range []string{"好的，我这就", "交给后端。它修完会", "告诉你！还有别的吗"} {
		out = append(out, s.add(d)...)
	}
	out = append(out, s.flush())
	if strings.Join(out, "|") != "好的，我这就交给后端。|它修完会告诉你！|还有别的吗" {
		t.Fatalf("%q", out)
	}
}

// --- a call with fakes ---

type fakeAudio struct {
	mu      sync.Mutex
	played  int
	flushes int
	ev      VoiceEvents
}

func (a *fakeAudio) Start(lang string, ev VoiceEvents) error { a.ev = ev; return nil }
func (a *fakeAudio) Play(pcm []byte, rate int) {
	a.mu.Lock()
	a.played += len(pcm)
	a.mu.Unlock()
}
func (a *fakeAudio) Flush() { a.mu.Lock(); a.flushes++; a.mu.Unlock() }
func (a *fakeAudio) Stop()  {}

type fakeSpeech struct{}

func (fakeSpeech) Speak(ctx context.Context, text string, play func([]byte, int)) error {
	play(make([]byte, 2*len([]rune(text))), 24000)
	return nil
}

// scriptModel answers each turn from a script; a reply may be streamed slowly.
type scriptModel struct {
	mu      sync.Mutex
	replies []chatMsg
	slow    time.Duration
	seen    [][]chatMsg
}

func (m *scriptModel) Stream(ctx context.Context, msgs []chatMsg, tools []toolDef, onText func(string), onTool func(string)) (chatMsg, error) {
	m.mu.Lock()
	m.seen = append(m.seen, append([]chatMsg(nil), msgs...))
	var r chatMsg
	if len(m.replies) > 0 {
		r, m.replies = m.replies[0], m.replies[1:]
	} else {
		r = chatMsg{Role: "assistant", Content: "嗯。"}
	}
	slow := m.slow
	m.mu.Unlock()
	for _, ch := range []rune(r.Content) {
		if slow > 0 {
			select {
			case <-time.After(slow):
			case <-ctx.Done():
				return chatMsg{}, ctx.Err()
			}
		}
		onText(string(ch))
	}
	for _, tc := range r.ToolCalls {
		if onTool != nil {
			onTool(tc.Function.Name)
		}
	}
	return r, nil
}

type fakeHub struct {
	mu      sync.Mutex
	told    []string
	working bool // a task at work: what is told is added to it
	news    taskNews
}

func (h *fakeHub) Name() string { return "后端" }
func (h *fakeHub) Context(context.Context) (string, error) {
	return "这个 agent：backend（后端），修 bug\n- 10:00 你（任务）：看看登录页", nil
}
func (h *fakeHub) Tell(_ context.Context, body string) (VoiceTaskRef, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.told = append(h.told, body)
	return VoiceTaskRef{ID: "msg_1", Agent: "backend", Thread: "th_1"}, h.working, nil
}
func (h *fakeHub) StopCurrent(context.Context) (VoiceTaskRef, error) {
	return VoiceTaskRef{ID: "msg_1"}, nil
}
func (h *fakeHub) Current(context.Context) (*VoiceTaskRef, error) { return nil, nil }
func (h *fakeHub) TaskNews(context.Context, VoiceTaskRef) (taskNews, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.news, nil
}

func newTestCall(t *testing.T, m *scriptModel, h *fakeHub, tweak ...func(*voiceDeps)) (*voice, *voiceCall, *fakeAudio) {
	t.Helper()
	v := &voice{state: stIdle, changed: make(chan struct{})}
	a := &fakeAudio{}
	d := voiceDeps{audio: a, model: m, tts: fakeSpeech{}, hub: h, lang: "zh-CN",
		endOfSpeech: 80 * time.Millisecond, quietBeforeNotice: 50 * time.Millisecond, watchEvery: 30 * time.Millisecond}
	for _, f := range tweak {
		f(&d)
	}
	c := newVoiceCall(v, d)
	v.call = c
	_ = a.Start("zh-CN", c)
	c.begin()
	t.Cleanup(c.hangUp)
	return v, c, a
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func lines(v *voice) []VoiceEvent {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]VoiceEvent(nil), v.events...)
}

func has(v *voice, kind, sub string) bool {
	for _, e := range lines(v) {
		if e.Kind == kind && strings.Contains(e.Text, sub) {
			return true
		}
	}
	return false
}

func TestVoiceCallPassesWorkOnAndSaysSo(t *testing.T) {
	send := toolCall{ID: "c1", Type: "function"}
	send.Function.Name = "tell_agent"
	send.Function.Arguments = `{"text":"修好登录页的 bug，修完跑一遍测试","what":"修登录 bug","their_words":"把登录那个 bug 修了"}`
	m := &scriptModel{replies: []chatMsg{
		{Role: "assistant", ToolCalls: []toolCall{send}},
		{Role: "assistant", Content: "好，交给后端了，修好我告诉你。"},
	}}
	h := &fakeHub{}
	v, c, a := newTestCall(t, m, h)

	c.Heard("把登录那个", false)
	c.Heard("把登录那个 bug 修了", true)
	waitFor(t, "the answer", func() bool { return has(v, vAssistant, "交给后端了") })
	// It said a few words before it acted, not only after.
	if !has(v, vAssistant, "好，我跟它说。好，交给后端了") {
		t.Fatalf("no word before acting: %+v", lines(v))
	}
	h.mu.Lock()
	told := append([]string(nil), h.told...)
	h.mu.Unlock()
	if len(told) != 1 || !strings.HasPrefix(told[0], "修好登录页的 bug") || !strings.Contains(told[0], "经语音转述") || !strings.Contains(told[0], "原话：把登录那个 bug 修了") {
		t.Fatalf("told: %v", told)
	}
	if !has(v, vUser, "把登录那个 bug 修了") || !has(v, vAction, "交给 后端：修登录 bug") {
		t.Fatalf("lines: %+v", lines(v))
	}
	for _, e := range lines(v) {
		if e.Kind == vHearing {
			t.Fatalf("the partial hearing should give way to the final line: %+v", lines(v))
		}
	}
	a.mu.Lock()
	played := a.played
	a.mu.Unlock()
	if played == 0 {
		t.Fatal("nothing was said out loud")
	}
	// The model reads the conversation with the agent.
	if p := m.seen[0][0].Content; !strings.Contains(p, "后端") || !strings.Contains(p, "看看登录页") {
		t.Fatalf("system prompt: %s", m.seen[0][0].Content)
	}
	c.Played()
	waitFor(t, "listening again", func() bool { v.mu.Lock(); defer v.mu.Unlock(); return v.state == stListening })
}

func TestVoiceCallStopsWhenThePersonSpeaks(t *testing.T) {
	long := "这个问题有几个方面，我们一个一个来看。首先是登录，然后是注册，最后是找回密码，每一个都要单独测一遍。"
	m := &scriptModel{replies: []chatMsg{{Role: "assistant", Content: long}}, slow: 15 * time.Millisecond}
	v, c, a := newTestCall(t, m, &fakeHub{})
	c.Heard("说说登录的问题", true)
	waitFor(t, "it starts talking", func() bool { return has(v, vAssistant, "这个问题") })

	// Listening noises and its own voice coming back do not cut it off.
	c.Heard("嗯", false)
	c.Heard("这个问题有几个方面", false)
	time.Sleep(60 * time.Millisecond)
	a.mu.Lock()
	flushes := a.flushes
	a.mu.Unlock()
	if flushes != 0 {
		t.Fatal("a backchannel or its own echo interrupted it")
	}

	// Real words do.
	c.Heard("等一下，先别说了", false)
	waitFor(t, "it stops", func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.flushes > 0 })
	waitFor(t, "the line marked cut off", func() bool {
		for _, e := range lines(v) {
			if e.Kind == vAssistant && e.Interrupted {
				return !strings.Contains(e.Text, "找回密码")
			}
		}
		return false
	})
	time.Sleep(200 * time.Millisecond)
	if has(v, vAssistant, "找回密码") {
		t.Fatal("it kept talking after being cut off")
	}
}

func TestVoiceCallAddsToTheTaskAtWork(t *testing.T) {
	add := toolCall{ID: "c1", Type: "function"}
	add.Function.Name = "tell_agent"
	add.Function.Arguments = `{"text":"顺便把注册页也看一下","what":"也看注册页"}`
	m := &scriptModel{replies: []chatMsg{
		{Role: "assistant", ToolCalls: []toolCall{add}},
		{Role: "assistant", Content: "好，跟它说了。"},
	}}
	h := &fakeHub{working: true}
	v, c, _ := newTestCall(t, m, h)
	c.Heard("顺便把注册页也看一下", true)
	waitFor(t, "the answer", func() bool { return has(v, vAssistant, "跟它说了") })
	if !has(v, vAction, "补充给 后端：也看注册页") {
		t.Fatalf("lines: %+v", lines(v))
	}
}

func TestVoiceCallTellsNewsInAPause(t *testing.T) {
	send := toolCall{ID: "c1", Type: "function"}
	send.Function.Name = "tell_agent"
	send.Function.Arguments = `{"text":"跑测试","what":"跑测试"}`
	m := &scriptModel{replies: []chatMsg{
		{Role: "assistant", ToolCalls: []toolCall{send}},
		{Role: "assistant", Content: "交给后端了。"},
		{Role: "assistant", Content: "后端那边测试都过了。"},
	}}
	h := &fakeHub{}
	v, c, _ := newTestCall(t, m, h)
	c.Heard("跑一下测试", true)
	waitFor(t, "handed out", func() bool { return has(v, vAssistant, "交给后端了") })
	c.Played()
	h.mu.Lock()
	h.news = taskNews{Finished: true, Summary: "全部 42 个测试通过"}
	h.mu.Unlock()
	waitFor(t, "the news told", func() bool { return has(v, vAssistant, "测试都过了") })
	m.mu.Lock()
	last := m.seen[len(m.seen)-1]
	m.mu.Unlock()
	if s := last[len(last)-1]; s.Role != "system" || !strings.Contains(s.Content, "全部 42 个测试通过") {
		t.Fatalf("the model should get the news as a system line: %+v", s)
	}
}

type memKeys struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memKeys) Get(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[id]; ok {
		return v, nil
	}
	return "", errNoKey
}
func (s *memKeys) Set(id, v string) error { s.mu.Lock(); s.m[id] = v; s.mu.Unlock(); return nil }
func (s *memKeys) Delete(id string) error { s.mu.Lock(); delete(s.m, id); s.mu.Unlock(); return nil }

var errNoKey = context.Canceled

func TestVoiceSettingsKeepKeysOutOfSight(t *testing.T) {
	dir := t.TempDir()
	keys := &memKeys{m: map[string]string{}}
	b := &Backend{Store: NewStore(dir + "/hubs.json"), Secrets: keys}
	srv := httptestServer(t, b)
	code, body := srv("PUT", "/app/voice/config", `{"llm_base_url":"https://api.example.com/v1","llm_model":"fast","llm_key":"sk-secret-1","tts_speaker":"zh_male_x","tts_key":"tts-secret-2"}`)
	if code != 200 || strings.Contains(body, "secret") || !strings.Contains(body, `"has_llm_key":true`) || !strings.Contains(body, `"has_tts_key":true`) || !strings.Contains(body, `"ready":false`) {
		t.Fatalf("put: %d %s", code, body)
	}
	if keys.m[voiceLLMSecret] != "sk-secret-1" || keys.m[voiceTTSSecret] != "tts-secret-2" {
		t.Fatalf("keys not in the keychain: %v", keys.m)
	}
	code, body = srv("GET", "/app/voice", "")
	if code != 200 || strings.Contains(body, "secret") || !strings.Contains(body, `"tts_speaker":"zh_male_x"`) || !strings.Contains(body, `"available":false`) {
		t.Fatalf("get: %d %s", code, body)
	}
	// A call is with one agent.
	if code, body := srv("POST", "/app/voice/start", `{"hub_id":"x"}`); code != 400 || !strings.Contains(body, "agent is required") {
		t.Fatalf("start without an agent: %d %s", code, body)
	}
	// No microphone here (a browser): starting says so.
	if code, body := srv("POST", "/app/voice/start", `{"hub_id":"x","agent":"backend"}`); code != 409 || !strings.Contains(body, "unsupported") {
		t.Fatalf("start without audio: %d %s", code, body)
	}
	if code, _ := srv("PUT", "/app/voice/config", `{"tts_rate":500}`); code != 400 {
		t.Fatalf("bad rate: %d", code)
	}
}

// httptestServer serves b and returns a function that makes a request.
func httptestServer(t *testing.T, b *Backend) func(method, path, body string) (int, string) {
	t.Helper()
	s := httptest.NewServer(b.Handler())
	t.Cleanup(func() {
		s.Close()
		// Server.Close joins handlers, but not the initial settings sync they
		// launch. Join it before the hubs and TempDirs are removed.
		v := b.voiceState()
		v.sync.once.Do(func() {})
		v.sync.background.Wait()
	})
	return func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, s.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
}

// The model's address may be given as the whole endpoint.
func TestVoiceModelTakesTheWholeEndpoint(t *testing.T) {
	var path, session string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, session = r.URL.Path, r.Header.Get("x-opencode-session")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	m := &openAIModel{base: srv.URL + "/v1/chat/completions", model: "fast", key: "k", session: "myco-voice-1"}
	out, err := m.Stream(context.Background(), []chatMsg{{Role: "user", Content: "嗨"}}, nil, nil, nil)
	if err != nil || out.Content != "好" || path != "/v1/chat/completions" || session != "myco-voice-1" {
		t.Fatalf("got %q %v at %s (session %q)", out.Content, err, path, session)
	}
}

// The settings offer Doubao's voices, and a voice can be tried before it is
// saved: one line in its own language, back as a WAV.
func TestVoiceListAndPreview(t *testing.T) {
	var gotKey, gotSpeaker, gotText string
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Req struct {
				Text    string `json:"text"`
				Speaker string `json:"speaker"`
			} `json:"req_params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		gotKey, gotSpeaker, gotText = r.Header.Get("X-Api-Key"), in.Req.Speaker, in.Req.Text
		enc := base64.StdEncoding.EncodeToString
		fmt.Fprintf(w, "{\"code\":0,\"data\":%q}\n{\"code\":20000000}\n", enc([]byte{1, 0, 2, 0}))
	}))
	defer tts.Close()
	was := doubaoTTSURL
	doubaoTTSURL = tts.URL
	defer func() { doubaoTTSURL = was }()

	keys := &memKeys{m: map[string]string{}}
	b := &Backend{Store: NewStore(t.TempDir() + "/hubs.json"), Secrets: keys}
	srv := httptestServer(t, b)
	code, body := srv("GET", "/app/voice", "")
	if code != 200 || !strings.Contains(body, `"id":"zh_female_vv_uranus_bigtts"`) || !strings.Contains(body, `"lang":"ja"`) {
		t.Fatalf("voices: %d %.300s", code, body)
	}
	if len(doubaoVoices) < 100 {
		t.Fatalf("only %d voices", len(doubaoVoices))
	}
	for _, v := range doubaoVoices {
		if v.ID == "" || v.Name == "" || v.Lang == "" || (v.Gender != "female" && v.Gender != "male") {
			t.Fatalf("voice %+v", v)
		}
	}
	if code, body := srv("POST", "/app/voice/preview", `{"tts_speaker":"zh_female_vv_uranus_bigtts"}`); code != 400 || !strings.Contains(body, "no_key") {
		t.Fatalf("no key: %d %s", code, body)
	}
	// The key typed (not saved yet) is used, and the line is in the voice's language.
	code, body = srv("POST", "/app/voice/preview", `{"tts_speaker":"ja_female_x_uranus_bigtts","tts_key":"typed-key"}`)
	if code != 200 || !strings.HasPrefix(body, "RIFF") || !strings.HasSuffix(body, string([]byte{1, 0, 2, 0})) {
		t.Fatalf("preview: %d %q", code, body)
	}
	if gotKey != "typed-key" || gotSpeaker != "ja_female_x_uranus_bigtts" || gotText != voiceSampleLines["ja"] {
		t.Fatalf("sent %q %q %q", gotKey, gotSpeaker, gotText)
	}
	// Else the kept key; no voice is the default one.
	keys.m[voiceTTSSecret] = "kept-key"
	if code, _ := srv("POST", "/app/voice/preview", `{"lang":"en"}`); code != 200 || gotKey != "kept-key" || gotSpeaker != defaultVoiceSpeaker || gotText != voiceSampleLines["zh"] {
		t.Fatalf("kept key: %d %q %q %q", code, gotKey, gotSpeaker, gotText)
	}
}

// Words the model says before a tool are enough: no second line is added.
func TestVoiceCallAckOnlyWhenSilent(t *testing.T) {
	send := toolCall{ID: "c1", Type: "function"}
	send.Function.Name = "tell_agent"
	send.Function.Arguments = `{"text":"加一个暗色模式"}`
	m := &scriptModel{replies: []chatMsg{
		{Role: "assistant", Content: "好，让它加暗色模式。", ToolCalls: []toolCall{send}},
		{Role: "assistant", Content: "转过去了。"},
	}}
	v, c, _ := newTestCall(t, m, &fakeHub{})
	c.Heard("加个暗色模式", true)
	waitFor(t, "the answer", func() bool { return has(v, vAssistant, "转过去了") })
	if has(v, vAssistant, "我跟它说") {
		t.Fatalf("an ack on top of the model's own words: %+v", lines(v))
	}
	if toolAck("en-US", "stop_agent") != "OK, I'll stop it. " || toolAck("zh-CN", "check_agent") != "我看一眼。" {
		t.Fatal("toolAck")
	}
}

// A long task is not minutes of silence: now and then the call says it is
// still going, with its newest progress note (told once).
func TestVoiceCallSaysALongTaskIsStillGoing(t *testing.T) {
	send := toolCall{ID: "c1", Type: "function"}
	send.Function.Name = "tell_agent"
	send.Function.Arguments = `{"text":"把整个项目的测试跑一遍","what":"跑全部测试"}`
	m := &scriptModel{replies: []chatMsg{
		{Role: "assistant", ToolCalls: []toolCall{send}},
		{Role: "assistant", Content: "转过去了。"},
	}}
	h := &fakeHub{news: taskNews{Status: "acked", Since: time.Now().Add(-6 * time.Minute), Progress: "在跑集成测试"}}
	v, c, _ := newTestCall(t, m, h, func(d *voiceDeps) { d.stillWorkingEvery = 300 * time.Millisecond })
	c.Heard("把测试都跑一遍", true)
	waitFor(t, "the answer", func() bool { return has(v, vAssistant, "转过去了") })
	c.Played()
	notice := func() []string {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []string
		for _, msgs := range m.seen {
			for _, x := range msgs {
				if x.Role == "system" && strings.Contains(x.Content, "还在进行") && !contains(out, x.Content) {
					out = append(out, x.Content)
				}
			}
		}
		return out
	}
	waitFor(t, "a word that it is still going", func() bool { return len(notice()) > 0 })
	first := notice()[0]
	if !strings.Contains(first, "跑全部测试") || !strings.Contains(first, "在做") || !strings.Contains(first, "6 分钟") || !strings.Contains(first, "在跑集成测试") {
		t.Fatalf("notice: %s", first)
	}
	// Its spoken reply ends (on a slow machine it may start only after
	// this; so the playback ends again now and then while waiting).
	c.Played()
	deadline, played := time.Now().Add(8*time.Second), time.Now()
	for len(notice()) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for a second word")
		}
		if time.Since(played) > time.Second {
			c.Played()
			played = time.Now()
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Contains(notice()[1], "在跑集成测试") {
		t.Fatalf("the same progress note told twice: %s", notice()[1])
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// A call ended leaves its last lines for the next call with that agent:
// kept per workspace and agent, word for word, for a day.
func TestVoiceMemoCarriesTheLastCallOver(t *testing.T) {
	v := &voice{state: stIdle, changed: make(chan struct{}), memoPath: t.TempDir() + "/voice-memo.json"}
	var history []chatMsg
	for i := 0; i < 6; i++ {
		history = append(history, chatMsg{Role: "user", Content: fmt.Sprintf("第 %d 句", i)},
			chatMsg{Role: "assistant", ToolCalls: []toolCall{{ID: "x"}}}, chatMsg{Role: "tool", Content: "sent"},
			chatMsg{Role: "system", Content: "（系统消息）"}, chatMsg{Role: "assistant", Content: fmt.Sprintf("回答 %d", i)})
	}
	v.keepMemo(memoKey("h1", "backend"), history)
	m := v.memo(memoKey("h1", "backend"))
	if m == nil || len(m.Lines) != memoLines || m.Lines[0].Text != "第 2 句" || m.Lines[7] != (memoLine{Who: "assistant", Text: "回答 5"}) {
		t.Fatalf("memo: %+v", m)
	}
	if v.memo(memoKey("h1", "frontend")) != nil || v.memo(memoKey("h2", "backend")) != nil {
		t.Fatal("a memo is per workspace and agent")
	}
	// A call where nothing was said keeps the memo from before.
	v.keepMemo(memoKey("h1", "backend"), []chatMsg{{Role: "system", Content: "x"}})
	if m := v.memo(memoKey("h1", "backend")); m == nil || m.Lines[0].Text != "第 2 句" {
		t.Fatalf("an empty call wiped the memo: %+v", m)
	}
	p := memoPrompt("zh-CN", m)
	if !strings.Contains(p, "上一通电话（1 分钟前）") || !strings.Contains(p, "- 对方：第 2 句") || !strings.Contains(p, "- 你：回答 5") {
		t.Fatalf("prompt: %s", p)
	}
	if memoPrompt("zh-CN", nil) != "" {
		t.Fatal("no memo, no words")
	}

	// A call: the memo goes into its instructions, and what was said is
	// handed back when it ends.
	ms := &scriptModel{replies: []chatMsg{{Role: "assistant", Content: "下午再说，记住了。"}}}
	got := make(chan []chatMsg, 1)
	_, c, _ := newTestCall(t, ms, &fakeHub{}, func(d *voiceDeps) {
		d.memo = p
		d.ended = func(h []chatMsg) { got <- h }
	})
	c.Heard("这个先不急，下午再说", true)
	waitFor(t, "a turn", func() bool { ms.mu.Lock(); defer ms.mu.Unlock(); return len(ms.seen) > 0 })
	ms.mu.Lock()
	sys := ms.seen[0][0].Content
	ms.mu.Unlock()
	if !strings.Contains(sys, "上一通电话") || !strings.Contains(sys, "回答 5") {
		t.Fatalf("instructions without the memo: %s", sys)
	}
	time.Sleep(100 * time.Millisecond)
	c.hangUp()
	select {
	case h := <-got:
		if l := memoOf(h); len(l) == 0 || l[0].Text != "这个先不急，下午再说" {
			t.Fatalf("ended with %+v", h)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the call's end was not handed over")
	}
}
