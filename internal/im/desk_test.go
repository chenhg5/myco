package im

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

// scripted plays the model: each turn returns the next reply.
type scripted struct {
	mu      sync.Mutex
	replies []deskMsg
	seen    [][]deskMsg
	err     error
}

func (s *scripted) Chat(_ context.Context, msgs []deskMsg, _ []deskTool) (deskMsg, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, append([]deskMsg(nil), msgs...))
	if s.err != nil {
		return deskMsg{}, s.err
	}
	if len(s.replies) == 0 {
		return deskMsg{Role: "assistant", Content: "?"}, nil
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r, nil
}

func call(name, args string) deskMsg {
	var c deskToolCall
	c.ID, c.Type, c.Function.Name, c.Function.Arguments = "c_"+name, "function", name, args
	return deskMsg{Role: "assistant", ToolCalls: []deskToolCall{c}}
}

func say(text string) deskMsg { return deskMsg{Role: "assistant", Content: text} }

func TestDeskAnswersAsTheAgent(t *testing.T) {
	b, ad, hub, cancel := newTestBridge(t)
	defer cancel()
	m := &scripted{replies: []deskMsg{say("我是 coder，负责写代码。")}}
	b.SetModel(m)
	b.Handle(Inbound{ChatID: "oc_1", MessageID: "om_1", ThreadKey: "chat:oc_1", UserID: "ou_owner", Text: "你是谁"})
	eventually(t, "answer", func() bool { return ad.last("post").text == "我是 coder，负责写代码。" })
	if hub.nsent() != 0 {
		t.Fatal("a plain question should not become a task")
	}
	sys := m.seen[0][0].Content
	if !strings.Contains(sys, "你就是「coder」") || !strings.Contains(sys, "第一人称") {
		t.Fatalf("prompt %q", sys)
	}
	if c := b.conv("chat:oc_1"); c == nil || len(c.Desk) != 2 {
		t.Fatalf("history %+v", c)
	}
}

func TestDeskHandsWorkOver(t *testing.T) {
	b, ad, hub, cancel := newTestBridge(t)
	defer cancel()
	b.SetModel(&scripted{replies: []deskMsg{
		call("do_work", `{"text":"修好 CI 上失败的构建"}`), say("好，我来弄"),
		call("do_work", `{"text":"顺便更新 changelog"}`), say("收到，补上"),
		call("stop_work", `{}`), say("好，先停下"),
	}})
	in := func(id, text string) Inbound {
		return Inbound{ChatID: "oc_1", MessageID: id, ThreadKey: "chat:oc_1", UserID: "ou_owner", Text: text}
	}
	b.Handle(in("om_1", "构建挂了，帮我看看"))
	eventually(t, "said", func() bool { return ad.last("post").text == "好，我来弄" })
	if hub.nsent() != 1 {
		t.Fatal("no task")
	}
	body := hub.sent[0].Body
	if !strings.HasPrefix(body, "修好 CI 上失败的构建") || !strings.Contains(body, "构建挂了，帮我看看") || !strings.Contains(body, "glenn") {
		t.Fatalf("body %q", body)
	}
	if ad.last("status").text != "" {
		t.Fatal("the desk already answered: no status card until progress")
	}
	for _, p := range ad.posts {
		if strings.Contains(p.text, "coder") {
			t.Fatalf("third person: %q", p.text)
		}
	}
	b.Handle(in("om_2", "还有 changelog"))
	eventually(t, "steered", func() bool { hub.mu.Lock(); defer hub.mu.Unlock(); return len(hub.steers) == 1 })
	if !strings.Contains(hub.steers[0], "顺便更新 changelog") {
		t.Fatalf("steer %q", hub.steers[0])
	}
	// Progress opens the card; the result is posted and remembered.
	hub.prog["msg_1"] <- api.Message{Body: "running tests"}
	eventually(t, "card", func() bool { return strings.Contains(ad.last("status").text, "running tests") })
	b.Handle(in("om_3", "先停一下"))
	eventually(t, "stopped", func() bool { hub.mu.Lock(); defer hub.mu.Unlock(); return len(hub.stopped) == 1 })
	hub.replies["msg_1"] <- &api.Message{Body: "fixed in abc123"}
	eventually(t, "result", func() bool { return ad.last("post").text == "fixed in abc123" })
	eventually(t, "remembered", func() bool { c := b.conv("chat:oc_1"); return c != nil && c.LastAnswer == "fixed in abc123" })
}

func TestDeskFailsOverToTheAgent(t *testing.T) {
	b, ad, hub, cancel := newTestBridge(t)
	defer cancel()
	b.SetModel(&scripted{err: errors.New("down")})
	b.Handle(Inbound{ChatID: "oc_1", MessageID: "om_1", ThreadKey: "chat:oc_1", UserID: "ou_owner", Text: "fix it"})
	eventually(t, "task", func() bool { return hub.nsent() == 1 })
	eventually(t, "card", func() bool { return ad.last("status").text == "⏳ 收到，我看看" })
}

func TestLLMChat(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "no", 401)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"do_work","arguments":"{\"text\":\"x\"}"}}]}}]}`))
	}))
	defer srv.Close()
	l := LLM{BaseURL: srv.URL + "/v1/", Model: "fast", Key: "k"}
	r, err := l.Chat(context.Background(), []deskMsg{{Role: "user", Content: "hi"}}, deskTools)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Function.Name != "do_work" {
		t.Fatalf("reply %+v", r)
	}
	if got["model"] != "fast" || len(got["tools"].([]any)) != 3 {
		t.Fatalf("request %v", got)
	}
	if _, err := (LLM{BaseURL: srv.URL, Model: "m", Key: "bad"}).Chat(context.Background(), nil, nil); err == nil {
		t.Fatal("want an error")
	}
}

func TestRedactedHidesTheModelKey(t *testing.T) {
	c := Config{Name: "x", LLMModel: "m", LLMKey: "sk-secret"}
	b, _ := json.Marshal(c.Redacted())
	if strings.Contains(string(b), "sk-secret") || !strings.Contains(string(b), `"desk":true`) {
		t.Fatalf("%s", b)
	}
}

// The owner's instructions shape the front desk, and travel with tasks from
// other people so the agent keeps to them too.
func TestDeskOwnersInstructions(t *testing.T) {
	ad, hub := &fakeAdapter{}, newFakeHub()
	cfg := Config{Name: "fs", Platform: "feishu", Owner: "ou_owner", OwnerName: "glenn", Agent: "coder",
		AllowFrom: []string{"ou_guest"}, Prompt: "只回答产品使用问题，不透露内部信息。"}
	b, err := New(cfg, ad, hub, stateDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	b.waitEach = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx) }()
	eventually(t, "running", func() bool { return b.ctx.Load() != nil })
	m := &scripted{replies: []deskMsg{call("do_work", `{"text":"查一下导出功能怎么用"}`), say("好，我查一下"), say("你好")}}
	b.SetModel(m)

	b.Handle(Inbound{ChatID: "oc_g", MessageID: "om_1", ThreadKey: "chat:oc_g", UserID: "ou_guest", UserName: "访客", Text: "导出怎么用？"})
	eventually(t, "task", func() bool { return hub.nsent() == 1 })
	sys := m.seen[0][0].Content
	if !strings.Contains(sys, "只回答产品使用问题") || !strings.Contains(sys, "对方不是你的主人") {
		t.Fatalf("prompt %q", sys)
	}
	if body := hub.sent[0].Body; !strings.Contains(body, "只回答产品使用问题") || !strings.Contains(body, "不是主人") {
		t.Fatalf("a guest's task without the rules: %q", body)
	}

	b.Handle(Inbound{ChatID: "oc_o", MessageID: "om_2", ThreadKey: "chat:oc_o", UserID: "ou_owner", Text: "hi"})
	eventually(t, "answer", func() bool { return ad.last("post").text == "你好" })
	m.mu.Lock()
	last := m.seen[len(m.seen)-1][0].Content
	m.mu.Unlock()
	if !strings.Contains(last, "对方是你的主人") {
		t.Fatalf("owner prompt %q", last)
	}
}

func TestPromptLength(t *testing.T) {
	c := Config{Name: "x", Platform: "slack", BotToken: "xoxb-1", AppToken: "xapp-1", Profile: "p", Prompt: strings.Repeat("长", MaxPrompt+1)}
	if c.Validate() == nil {
		t.Fatal("too long")
	}
}
