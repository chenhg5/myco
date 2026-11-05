package wake

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

type recorded struct {
	method, path string
	header       http.Header
	body         []byte
}

func recorder(t *testing.T, respond func(r *http.Request) (int, string)) (*httptest.Server, *[]recorded) {
	t.Helper()
	var (
		mu  sync.Mutex
		got []recorded
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, recorded{r.Method, r.URL.Path, r.Header.Clone(), b})
		mu.Unlock()
		code, body := 200, "{}"
		if respond != nil {
			code, body = respond(r)
		}
		w.WriteHeader(code)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func agent(mode, u, secret string, params map[string]string) *domain.Agent {
	return &domain.Agent{Name: "worker", Binding: domain.Binding{Harness: "claude", SessionID: "s1", Environment: "vps"},
		Delivery: domain.Delivery{Mode: mode, URL: u, Secret: secret, Params: params}}
}

func pending() []*domain.Message {
	m, _ := domain.NewMessage("msg_aaaaaaaaaaaaaaaa", "alice", "worker", domain.TypeTask, "fix it", "", "", "", app.SystemClock{}.Now())
	return []*domain.Message{m}
}

func TestWebhookSignsPayload(t *testing.T) {
	srv, got := recorder(t, nil)
	a := agent("webhook", srv.URL+"/hook", "k3y", nil)
	if err := (Webhook{}).Validate(a); err != nil {
		t.Fatal(err)
	}
	if err := (Webhook{}).Wake(context.Background(), app.WakeRequest{Agent: a, Pending: pending(), Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	r := (*got)[0]
	if !Verify("k3y", r.body, r.header.Get("X-Myco-Signature")) {
		t.Fatal("signature does not verify")
	}
	var ev WebhookEvent
	json.Unmarshal(r.body, &ev)
	if ev.SessionID != "s1" || ev.MessageIDs[0] != "msg_aaaaaaaaaaaaaaaa" || ev.Text != "hello" {
		t.Fatalf("event: %+v", ev)
	}
}

func TestValidationRejectsInsecureURLs(t *testing.T) {
	for _, u := range []string{"", "http://example.com/x", "https://user:pw@example.com/x", "ftp://x"} {
		if err := (ClaudeRoutine{}).Validate(agent("claude_routine", u, "t", nil)); err == nil {
			t.Errorf("url %q should be rejected", u)
		}
	}
	if err := (GitHubDispatch{}).Validate(agent("github_dispatch", "", "t", map[string]string{"repo": "bad repo"})); err == nil {
		t.Error("bad repo should be rejected")
	}
}

func TestClaudeRoutineFire(t *testing.T) {
	srv, got := recorder(t, nil)
	a := agent("claude_routine", srv.URL+"/v1/claude_code/routines/trig_1/fire", "sk-ant-oat01-x", nil)
	if err := (ClaudeRoutine{}).Wake(context.Background(), app.WakeRequest{Agent: a, Text: "wake up"}); err != nil {
		t.Fatal(err)
	}
	r := (*got)[0]
	if r.header.Get("Authorization") != "Bearer sk-ant-oat01-x" || r.header.Get("anthropic-beta") == "" {
		t.Fatalf("headers: %v", r.header)
	}
	var body map[string]string
	json.Unmarshal(r.body, &body)
	if body["text"] != "wake up" {
		t.Fatalf("body: %s", r.body)
	}
}

func TestGitHubIssueCreatesIssueAndMention(t *testing.T) {
	srv, got := recorder(t, func(r *http.Request) (int, string) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search"):
			return 200, `{"items":[]}`
		case r.URL.Path == "/repos/me/app/issues":
			return 201, `{"number":7}`
		}
		return 201, `{}`
	})
	a := agent("github_issue", "", "ghp_x", map[string]string{"repo": "me/app", "api_url": srv.URL})
	if err := (GitHubIssue{}).Wake(context.Background(), app.WakeRequest{Agent: a, Pending: pending()}); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 3 || (*got)[2].path != "/repos/me/app/issues/7/comments" {
		t.Fatalf("calls: %+v", *got)
	}
	if !strings.Contains(string((*got)[1].body), "[myco] msg_aaaaaaaaaaaaaaaa") || !strings.Contains(string((*got)[2].body), "@codex") {
		t.Fatalf("issue/comment bodies: %s / %s", (*got)[1].body, (*got)[2].body)
	}
	if !(GitHubIssue{}).Consumes() {
		t.Fatal("github_issue hands messages off")
	}
}
