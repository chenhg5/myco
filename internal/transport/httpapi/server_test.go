package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/hubserver"
)

const adminToken = "admin-token-123456"

type hub struct {
	t   *testing.T
	url string
}

func newHub(t *testing.T, policy app.MessagePolicy) *hub {
	t.Helper()
	h, err := hubserver.Build(hubserver.Options{DBPath: ":memory:", AdminToken: adminToken, Policy: policy,
		OnlineWindow: time.Minute, FilesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Handler)
	t.Cleanup(func() { srv.Close(); h.Close() })
	return &hub{t: t, url: srv.URL}
}

// resp is a decoded HTTP response.
type resp struct {
	status int
	header http.Header
	raw    []byte
	body   map[string]any
}

// do sends a request with an optional bearer token and JSON body (a string
// is sent verbatim, anything else is JSON-encoded).
func (h *hub) do(method, path, token string, body any) resp {
	h.t.Helper()
	return h.doReq(method, path, token, body, nil)
}

func (h *hub) doReq(method, path, token string, body any, hdr map[string]string) resp {
	h.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			h.t.Fatal(err)
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, h.url+path, rd)
	if err != nil {
		h.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if rd != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	out := resp{status: res.StatusCode, header: res.Header, raw: raw}
	if strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(raw, &out.body); err != nil {
			h.t.Fatalf("%s %s: bad JSON %q: %v", method, path, raw, err)
		}
	}
	return out
}

func (r resp) want(t *testing.T, status int) resp {
	t.Helper()
	if r.status != status {
		t.Fatalf("status %d, want %d: %s", r.status, status, r.raw)
	}
	return r
}

// wantErr checks the error envelope: status, code and the field set.
func (r resp) wantErr(t *testing.T, status int, code string) {
	t.Helper()
	r.want(t, status)
	if ct := r.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("error Content-Type %q", ct)
	}
	if r.body["error"] != code {
		t.Errorf("error code %v, want %s (%s)", r.body["error"], code, r.raw)
	}
	if msg, _ := r.body["message"].(string); msg == "" {
		t.Errorf("error without message: %s", r.raw)
	}
	if retry, ok := r.body["retryable"].(bool); !ok || retry != (code == "internal") {
		t.Errorf("retryable = %v (present %v) for %s", retry, ok, code)
	}
	for k := range r.body {
		switch k {
		case "error", "message", "suggestion", "retryable":
		default:
			t.Errorf("unexpected error field %q", k)
		}
	}
}

func str(v any, path ...string) string {
	for _, p := range path {
		m, _ := v.(map[string]any)
		v = m[p]
	}
	s, _ := v.(string)
	return s
}

func messages(t *testing.T, r resp) []map[string]any {
	t.Helper()
	list, ok := r.body["messages"].([]any)
	if !ok {
		t.Fatalf("no messages array: %s", r.raw)
	}
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, m.(map[string]any))
	}
	return out
}

// register creates an agent (admin) and returns its token.
func (h *hub) register(name string, extra map[string]any) string {
	h.t.Helper()
	body := map[string]any{"name": name}
	for k, v := range extra {
		body[k] = v
	}
	r := h.do("POST", "/v1/agents", adminToken, body).want(h.t, http.StatusCreated)
	tok := str(r.body, "token")
	if tok == "" || str(r.body, "agent", "name") != name || r.body["created"] != true {
		h.t.Fatalf("register %s: %s", name, r.raw)
	}
	return tok
}

func (h *hub) send(from, to, body string) string {
	h.t.Helper()
	r := h.do("POST", "/v1/messages", from, map[string]any{"to": to, "body": body}).want(h.t, http.StatusCreated)
	return str(r.body, "message", "id")
}

func shortPolicy() app.MessagePolicy {
	return app.MessagePolicy{Lease: 100 * time.Millisecond, MaxAttempts: 5, MaxWait: 3 * time.Second}
}

// ---- tests ----

func TestHealth(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	r := h.do("GET", "/v1/health", "", nil).want(t, http.StatusOK)
	if r.body["ok"] != true || r.body["version"] != "v1" {
		t.Fatalf("health: %s", r.raw)
	}
	if _, ok := r.body["public_url"]; ok {
		t.Errorf("public_url should be omitted when unknown: %s", r.raw)
	}
}

func TestAuth(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	h.register("carol", nil)
	tests := []struct {
		name, method, path, token string
		body                      any
		status                    int
		code                      string
	}{
		{"agent route without token", "GET", "/v1/me", "", nil, 401, "unauthenticated"},
		{"agent route with bad token", "GET", "/v1/me", "myco_bogus", nil, 401, "unauthenticated"},
		{"agent route with admin token", "GET", "/v1/me", adminToken, nil, 401, "unauthenticated"},
		{"lowercase bearer scheme is not accepted", "GET", "/v1/me", "", nil, 401, "unauthenticated"},
		{"discovery without token", "GET", "/v1/agents", "", nil, 401, "unauthenticated"},
		{"discovery with bad token", "GET", "/v1/agents", "nope", nil, 401, "unauthenticated"},
		// Admin routes: no/unknown token is 401 (not authenticated); a valid
		// agent token is 403 (authenticated, but admin only).
		{"admin route without token", "POST", "/v1/agents", "", map[string]any{"name": "x"}, 401, "unauthenticated"},
		{"admin route with wrong admin token", "POST", "/v1/agents", adminToken + "x", map[string]any{"name": "x"}, 401, "unauthenticated"},
		{"admin route with an unknown token", "DELETE", "/v1/agents/alice", "myco_notarealtoken", nil, 401, "unauthenticated"},
		{"register with agent token", "POST", "/v1/agents", alice, map[string]any{"name": "x"}, 403, "permission_denied"},
		{"delete agent with agent token", "DELETE", "/v1/agents/alice", alice, nil, 403, "permission_denied"},
		{"create enrollment with agent token", "POST", "/v1/enrollments", alice, map[string]any{"name_prefix": "c-"}, 403, "permission_denied"},
		{"list enrollments with agent token", "GET", "/v1/enrollments", alice, nil, 403, "permission_denied"},
		{"set another agent's alias", "PUT", "/v1/aliases/boss", alice, map[string]any{"target": "carol"}, 403, "permission_denied"},
		{"admin labels with agent token", "PUT", "/v1/agents/alice/labels", alice, map[string]any{"add": []string{"x"}}, 403, "permission_denied"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var hdr map[string]string
			if strings.HasPrefix(tc.name, "lowercase") {
				hdr = map[string]string{"Authorization": "bearer " + alice}
			}
			h.t = t
			h.doReq(tc.method, tc.path, tc.token, tc.body, hdr).wantErr(t, tc.status, tc.code)
		})
	}
	h.t = t
	// The valid cases.
	if r := h.do("GET", "/v1/me", alice, nil).want(t, 200); r.body["name"] != "alice" {
		t.Fatalf("me: %s", r.raw)
	}
	h.do("GET", "/v1/agents", adminToken, nil).want(t, 200)
	h.do("GET", "/v1/agents", alice, nil).want(t, 200)
	// Admin-only failures must not have created anything.
	if r := h.do("GET", "/v1/agents/x", adminToken, nil); r.status != 404 {
		t.Fatalf("agent x created by a rejected request: %s", r.raw)
	}
}

func TestErrorMapping(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", map[string]any{"allow_from": []string{"carol"}})
	carol := h.register("carol", nil)
	queued := h.send(carol, "bob", "hi")

	tests := []struct {
		name, method, path, token string
		body                      any
		status                    int
		code                      string
	}{
		{"unknown agent", "GET", "/v1/agents/ghost", adminToken, nil, 404, "not_found"},
		{"unknown message", "GET", "/v1/messages/msg_nope", alice, nil, 404, "not_found"},
		{"someone else's message reads as not found", "GET", "/v1/messages/" + queued, alice, nil, 404, "not_found"},
		{"unknown route", "GET", "/v1/nope", "", nil, 404, "not_found"},
		{"send to unknown agent", "POST", "/v1/messages", alice, map[string]any{"to": "ghost", "body": "x"}, 404, "not_found"},
		{"duplicate register", "POST", "/v1/agents", adminToken, map[string]any{"name": "alice"}, 409, "conflict"},
		{"alias shadowing an agent", "PUT", "/v1/aliases/alice", adminToken, map[string]any{"target": "carol"}, 409, "conflict"},
		{"ack a queued message (invalid state)", "POST", "/v1/messages/" + queued + "/ack", bob, nil, 409, "invalid_state"},
		{"invalid agent name", "POST", "/v1/agents", adminToken, map[string]any{"name": "Bad Name"}, 400, "invalid_argument"},
		{"malformed JSON", "POST", "/v1/messages", alice, `{"to":`, 400, "invalid_argument"},
		{"unknown JSON field", "POST", "/v1/messages", alice, `{"to":"carol","body":"x","bogus":1}`, 400, "invalid_argument"},
		{"empty body", "POST", "/v1/messages", alice, map[string]any{"to": "carol", "body": "  "}, 400, "invalid_argument"},
		{"unsendable type", "POST", "/v1/messages", alice, map[string]any{"to": "carol", "body": "x", "type": "reply"}, 400, "invalid_argument"},
		{"allow_from", "POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "x"}, 403, "permission_denied"},
		{"ack someone else's message", "POST", "/v1/messages/" + queued + "/ack", alice, nil, 403, "permission_denied"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h.t = t
			h.do(tc.method, tc.path, tc.token, tc.body).wantErr(t, tc.status, tc.code)
		})
	}
	h.t = t

	// Suggestions are part of the contract where the domain provides one.
	r := h.do("GET", "/v1/agents/ghost", adminToken, nil)
	if !strings.Contains(str(r.body, "suggestion"), "myco agent list") {
		t.Errorf("not-found suggestion: %s", r.raw)
	}
	r = h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "x"})
	if !strings.Contains(str(r.body, "suggestion"), "allow-from") {
		t.Errorf("allow_from suggestion: %s", r.raw)
	}
}

func TestMessageLifecycle(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", map[string]any{"capabilities": []string{"code"}})

	// Send.
	r := h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "ping", "idempotency_key": "k1"}).want(t, 201)
	msg := r.body["message"].(map[string]any)
	id := msg["id"].(string)
	if msg["from"] != "alice" || msg["to"] != "bob" || msg["type"] != "task" || msg["status"] != "queued" || msg["thread_id"] != id || r.body["duplicate"] != false {
		t.Fatalf("send: %s", r.raw)
	}
	if _, ok := msg["lease_until"]; ok {
		t.Errorf("queued message has lease_until: %s", r.raw)
	}
	// Same idempotency key: 200, same message, duplicate=true.
	r = h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "ping again", "idempotency_key": "k1"}).want(t, 200)
	if str(r.body, "message", "id") != id || r.body["duplicate"] != true {
		t.Fatalf("idempotent resend: %s", r.raw)
	}
	// Dry run: 200, nothing stored.
	r = h.do("POST", "/v1/messages?dry_run=true", alice, map[string]any{"to": "bob", "body": "dry"}).want(t, 200)
	if r.body["dry_run"] != true {
		t.Fatalf("dry run: %s", r.raw)
	}

	// Pull: exactly the one real message, leased.
	r = h.do("POST", "/v1/inbox/pull", bob, map[string]any{}).want(t, 200)
	ms := messages(t, r)
	if len(ms) != 1 || ms[0]["id"] != id || ms[0]["status"] != "delivered" || ms[0]["attempts"] != float64(1) || ms[0]["lease_until"] == nil {
		t.Fatalf("pull: %s", r.raw)
	}
	// Leased: a second pull (empty body allowed) returns nothing.
	r = h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)
	if len(messages(t, r)) != 0 {
		t.Fatalf("leased message pulled again: %s", r.raw)
	}

	// Ack (idempotent), reply.
	for range 2 {
		r = h.do("POST", "/v1/messages/"+id+"/ack", bob, nil).want(t, 200)
		if str(r.body, "message", "status") != "acked" {
			t.Fatalf("ack: %s", r.raw)
		}
	}
	r = h.do("POST", "/v1/messages/"+id+"/reply", bob, map[string]any{"body": "pong"}).want(t, 201)
	reply := r.body["message"].(map[string]any)
	if reply["type"] != "reply" || reply["reply_to"] != id || reply["to"] != "alice" || reply["thread_id"] != id || reply["body"] != "pong" {
		t.Fatalf("reply: %s", r.raw)
	}
	// The task is closed: replying again is an invalid state.
	h.do("POST", "/v1/messages/"+id+"/reply", bob, map[string]any{"body": "again"}).wantErr(t, 409, "invalid_state")
	if r := h.do("GET", "/v1/messages/"+id, alice, nil).want(t, 200); r.body["status"] != "done" {
		t.Fatalf("task after reply: %s", r.raw)
	}

	// The sender waits for the reply; only the sender may.
	r = h.do("GET", "/v1/messages/"+id+"/reply?wait_seconds=1", alice, nil).want(t, 200)
	if str(r.body, "reply", "body") != "pong" || r.body["timed_out"] != false {
		t.Fatalf("wait reply: %s", r.raw)
	}
	h.do("GET", "/v1/messages/"+id+"/reply", bob, nil).wantErr(t, 403, "permission_denied")

	// Thread: participants and the admin see it in order; others do not.
	for _, tok := range []string{alice, bob, adminToken} {
		r = h.do("GET", "/v1/threads/"+id, tok, nil).want(t, 200)
		ms := messages(t, r)
		if len(ms) != 2 || ms[0]["id"] != id || ms[1]["type"] != "reply" {
			t.Fatalf("thread: %s", r.raw)
		}
	}
	carol := h.register("carol", nil)
	h.do("GET", "/v1/threads/"+id, carol, nil).wantErr(t, 404, "not_found")
	h.do("GET", "/v1/threads/"+id, "", nil).wantErr(t, 401, "unauthenticated")
	h.do("GET", "/v1/threads/nope", adminToken, nil).wantErr(t, 404, "not_found")
}

func TestAllowFrom(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	carol := h.register("carol", nil)
	bob := h.register("bob", map[string]any{"allow_from": []string{"carol"}})

	h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "x"}).wantErr(t, 403, "permission_denied")
	// Nothing was queued for bob by the refused send.
	if r := h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200); len(messages(t, r)) != 0 {
		t.Fatalf("refused message delivered: %s", r.raw)
	}
	id := h.send(carol, "bob", "allowed")
	// bob's allow_from does not restrict who bob can send to, or replies.
	h.send(bob, "alice", "outgoing is unrestricted")
	h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)
	h.do("POST", "/v1/messages/"+id+"/reply", bob, map[string]any{"body": "ok"}).want(t, 201)
	// The admin can open bob up again ([] = anyone).
	h.do("PUT", "/v1/agents/bob/profile", adminToken, map[string]any{"allow_from": []string{}}).want(t, 200)
	h.send(alice, "bob", "now allowed")
	// An agent cannot change its own allow_from.
	h.do("PUT", "/v1/me/profile", bob, map[string]any{"allow_from": []string{"carol"}}).wantErr(t, 403, "permission_denied")
}

func TestLongPollPullReturnsWhenMessageArrives(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)

	type result struct {
		r       resp
		elapsed time.Duration
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		// Use a separate request so the helper's t.Fatal is not called off
		// the test goroutine.
		req, _ := http.NewRequest("POST", h.url+"/v1/inbox/pull", strings.NewReader(`{"wait_seconds":10}`))
		req.Header.Set("Authorization", "Bearer "+bob)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- result{}
			return
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		done <- result{resp{status: res.StatusCode, raw: raw, body: body}, time.Since(start)}
	}()
	time.Sleep(150 * time.Millisecond) // let the pull park
	select {
	case res := <-done:
		t.Fatalf("pull returned before any message: %s", res.r.raw)
	default:
	}
	id := h.send(alice, "bob", "wake up")

	select {
	case res := <-done:
		if res.r.status != 200 {
			t.Fatalf("pull: %d %s", res.r.status, res.r.raw)
		}
		ms := messages(t, res.r)
		if len(ms) != 1 || ms[0]["id"] != id {
			t.Fatalf("pull: %s", res.r.raw)
		}
		if res.elapsed > 3*time.Second {
			t.Fatalf("long poll did not return early: %v", res.elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("long poll did not return after the message arrived")
	}
}

func TestPullTypesAndLeaseRedelivery(t *testing.T) {
	h := newHub(t, shortPolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	task := h.send(alice, "bob", "task")
	h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "note", "type": "note"}).want(t, 201)

	r := h.do("POST", "/v1/inbox/pull", bob, map[string]any{"types": []string{"note"}}).want(t, 200)
	if ms := messages(t, r); len(ms) != 1 || ms[0]["type"] != "note" {
		t.Fatalf("types filter: %s", r.raw)
	}
	r = h.do("POST", "/v1/inbox/pull", bob, map[string]any{"limit": 5}).want(t, 200)
	if ms := messages(t, r); len(ms) != 1 || ms[0]["id"] != task {
		t.Fatalf("task pull: %s", r.raw)
	}
	// Not acked: after the (100ms) lease the task is redelivered.
	time.Sleep(150 * time.Millisecond)
	r = h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)
	ms := messages(t, r)
	if len(ms) == 0 {
		t.Fatalf("no redelivery after lease: %s", r.raw)
	}
	found := false
	for _, m := range ms {
		if m["id"] == task {
			found = true
			if m["attempts"] != float64(2) {
				t.Errorf("redelivered attempts: %v", m["attempts"])
			}
		}
	}
	if !found {
		t.Fatalf("task not redelivered: %s", r.raw)
	}
	// Acked: never again.
	h.do("POST", "/v1/messages/"+task+"/ack", bob, nil).want(t, 200)
	time.Sleep(150 * time.Millisecond)
	r = h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)
	for _, m := range messages(t, r) {
		if m["id"] == task {
			t.Fatalf("acked task redelivered: %s", r.raw)
		}
	}
}

func TestAliasesAndCascadeOnAgentRemoval(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	h.do("PUT", "/v1/aliases/self-one", alice, map[string]any{"target": "alice"}).want(t, 200)
	h.do("PUT", "/v1/aliases/self-two", alice, map[string]any{"target": "alice"}).want(t, 200)
	h.do("PUT", "/v1/aliases/self-three", alice, map[string]any{"target": "alice"}).want(t, 200)
	h.do("PUT", "/v1/aliases/self-four", alice, map[string]any{"target": "alice"}).wantErr(t, 409, "conflict")
	h.do("PUT", "/v1/aliases/self-one", bob, map[string]any{"target": "bob"}).wantErr(t, 409, "conflict")
	h.do("PUT", "/v1/aliases/bob", alice, map[string]any{"target": "alice"}).wantErr(t, 409, "conflict")
	h.do("DELETE", "/v1/aliases/self-one", bob, nil).wantErr(t, 403, "permission_denied")
	h.do("DELETE", "/v1/aliases/self-one", alice, nil).want(t, 200)
	h.do("DELETE", "/v1/aliases/self-two", alice, nil).want(t, 200)
	h.do("DELETE", "/v1/aliases/self-three", alice, nil).want(t, 200)

	h.do("PUT", "/v1/aliases/reviewer", adminToken, map[string]any{"target": "ghost"}).wantErr(t, 404, "not_found")
	r := h.do("PUT", "/v1/aliases/reviewer", adminToken, map[string]any{"target": "bob"}).want(t, 200)
	if str(r.body, "alias", "alias") != "reviewer" || str(r.body, "alias", "target") != "bob" {
		t.Fatalf("set alias: %s", r.raw)
	}
	r = h.do("POST", "/v1/messages", alice, map[string]any{"to": "reviewer", "body": "review pls"}).want(t, 201)
	if str(r.body, "message", "to") != "bob" || r.body["via_alias"] != "reviewer" {
		t.Fatalf("send via alias: %s", r.raw)
	}
	if r := h.do("GET", "/v1/agents/reviewer", bob, nil).want(t, 200); r.body["name"] != "bob" {
		t.Fatalf("get via alias: %s", r.raw)
	}

	r = h.do("DELETE", "/v1/agents/bob", adminToken, nil).want(t, 200)
	if r.body["removed"] != true {
		t.Fatalf("remove: %s", r.raw)
	}
	r = h.do("GET", "/v1/aliases", alice, nil).want(t, 200)
	if list, _ := r.body["aliases"].([]any); len(list) != 0 {
		t.Fatalf("alias survived its target's removal: %s", r.raw)
	}
	h.do("POST", "/v1/messages", alice, map[string]any{"to": "reviewer", "body": "x"}).wantErr(t, 404, "not_found")
	// The removed agent's token no longer authenticates.
	h.do("GET", "/v1/me", bob, nil).wantErr(t, 401, "unauthenticated")
	h.do("DELETE", "/v1/aliases/reviewer", adminToken, nil).wantErr(t, 404, "not_found")
}

func TestTaskFollowUpsAndFailure(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	id := h.send(alice, "bob", "long job")

	// Pending (not leased) shows up in inbox/wait without leasing it.
	r := h.do("GET", "/v1/inbox/wait?wait_seconds=0", bob, nil).want(t, 200)
	if p, _ := r.body["pending"].([]any); len(p) != 1 || r.body["timed_out"] != false {
		t.Fatalf("inbox wait: %s", r.raw)
	}
	h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)

	// Release with no body: back to due immediately, attempts not counted.
	r = h.do("POST", "/v1/messages/"+id+"/release", bob, nil).want(t, 200)
	if str(r.body, "message", "status") != "delivered" {
		t.Fatalf("release: %s", r.raw)
	}
	h.do("POST", "/v1/messages/"+id+"/release", bob, map[string]any{"delay_seconds": 7200}).wantErr(t, 400, "invalid_argument")
	r = h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)
	if ms := messages(t, r); len(ms) != 1 || ms[0]["attempts"] != float64(1) {
		t.Fatalf("re-pull after release: %s", r.raw)
	}

	h.do("POST", "/v1/messages/"+id+"/hold", bob, nil).want(t, 200)
	// Progress from the recipient, steer from the sender (not vice versa).
	r = h.do("POST", "/v1/messages/"+id+"/progress", bob, map[string]any{"body": "50%"}).want(t, 201)
	if str(r.body, "message", "type") != "progress" || str(r.body, "message", "status") != "done" {
		t.Fatalf("progress: %s", r.raw)
	}
	h.do("POST", "/v1/messages/"+id+"/progress", alice, map[string]any{"body": "x"}).wantErr(t, 403, "permission_denied")
	r = h.do("POST", "/v1/messages/"+id+"/steer", alice, map[string]any{"body": "go faster"}).want(t, 201)
	if str(r.body, "message", "type") != "steer" || str(r.body, "message", "to") != "bob" {
		t.Fatalf("steer: %s", r.raw)
	}
	h.do("POST", "/v1/messages/"+id+"/steer", bob, map[string]any{"body": "x"}).wantErr(t, 403, "permission_denied")

	// The sender's wait sees progress before the reply.
	r = h.do("GET", "/v1/messages/"+id+"/reply?wait_seconds=0&progress_since=0", alice, nil).want(t, 200)
	if p, _ := r.body["progress"].([]any); len(p) != 1 || r.body["reply"] != nil || r.body["timed_out"] != false {
		t.Fatalf("wait with progress: %s", r.raw)
	}
	h.do("GET", "/v1/messages/"+id+"/reply?progress_since=yesterday", alice, nil).wantErr(t, 400, "invalid_argument")
	r = h.do("GET", "/v1/messages/"+id+"/reply?wait_seconds=0", alice, nil).want(t, 200)
	if r.body["timed_out"] != true || r.body["reply"] != nil {
		t.Fatalf("wait timeout: %s", r.raw)
	}

	// Fail: the task closes and the sender gets an event as the "reply".
	r = h.do("POST", "/v1/messages/"+id+"/fail", bob, map[string]any{"reason": "disk full"}).want(t, 200)
	if str(r.body, "message", "status") != "failed" || str(r.body, "message", "error") != "disk full" {
		t.Fatalf("fail: %s", r.raw)
	}
	r = h.do("GET", "/v1/messages/"+id+"/reply", alice, nil).want(t, 200)
	if str(r.body, "reply", "type") != "event" || !strings.Contains(str(r.body, "reply", "body"), "disk full") {
		t.Fatalf("failure event: %s", r.raw)
	}
	h.do("POST", "/v1/messages/"+id+"/done", bob, nil).wantErr(t, 409, "invalid_state")
}

func TestProfileLabelsStatusAndListing(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", map[string]any{"labels": []string{"team=x"}, "capabilities": []string{"code"},
		"delivery": map[string]any{"mode": "webhook", "url": "https://hook.example/x?token=secret", "secret": "s3cret"}})
	bob := h.register("bob", nil)

	r := h.do("GET", "/v1/me", alice, nil).want(t, 200)
	if str(r.body, "delivery", "url") != "https://hook.example/x?…" || r.body["delivery"].(map[string]any)["has_secret"] != true || strings.Contains(string(r.raw), "s3cret") {
		t.Fatalf("delivery not redacted: %s", r.raw)
	}
	if r.body["online"] != true {
		t.Errorf("authenticated agent not online: %s", r.raw)
	}

	r = h.do("PUT", "/v1/me/labels", alice, map[string]any{"add": []string{"gpu"}, "remove": []string{"team"}}).want(t, 200)
	if labels, _ := r.body["labels"].([]any); len(labels) != 1 || labels[0] != "gpu" {
		t.Fatalf("labels: %s", r.raw)
	}
	h.do("PUT", "/v1/me/labels", alice, map[string]any{"add": []string{"Bad Label"}}).wantErr(t, 400, "invalid_argument")
	r = h.do("PUT", "/v1/me/status", alice, map[string]any{"text": "busy"}).want(t, 200)
	if r.body["status_text"] != "busy" || r.body["status_at"] == nil {
		t.Fatalf("status: %s", r.raw)
	}
	h.do("PUT", "/v1/me/status", alice, map[string]any{"text": "two\nlines"}).wantErr(t, 400, "invalid_argument")
	r = h.do("PUT", "/v1/me/profile", alice, map[string]any{"display_name": "Alice A."}).want(t, 200)
	if r.body["display_name"] != "Alice A." {
		t.Fatalf("profile: %s", r.raw)
	}
	r = h.do("PUT", "/v1/me/binding", alice, map[string]any{"harness": "claude", "session_id": "s-1"}).want(t, 200)
	if str(r.body, "binding", "session_id") != "s-1" {
		t.Fatalf("binding: %s", r.raw)
	}

	r = h.do("GET", "/v1/agents?capability=code", bob, nil).want(t, 200)
	if list, _ := r.body["agents"].([]any); len(list) != 1 || str(list[0], "name") != "alice" {
		t.Fatalf("filter by capability: %s", r.raw)
	}
	r = h.do("GET", "/v1/agents?selector=gpu", adminToken, nil).want(t, 200)
	if list, _ := r.body["agents"].([]any); len(list) != 1 || str(list[0], "name") != "alice" {
		t.Fatalf("filter by selector: %s", r.raw)
	}

	r = h.do("GET", "/v1/overview", bob, nil).want(t, 200)
	if str(r.body, "viewer", "kind") != "agent" || str(r.body, "viewer", "name") != "bob" {
		t.Fatalf("overview viewer: %s", r.raw)
	}
	r = h.do("GET", "/v1/overview?recent=5", adminToken, nil).want(t, 200)
	if str(r.body, "viewer", "kind") != "admin" || r.body["totals"].(map[string]any)["agents"] != float64(2) {
		t.Fatalf("overview admin: %s", r.raw)
	}
	h.do("GET", "/v1/overview", "", nil).wantErr(t, 401, "unauthenticated")
}

func TestPulledTaskIsPendingUntilAck(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	id := h.send(alice, "bob", "work")
	h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)
	check := func(wantState string, wantQueued, wantProgress float64) {
		t.Helper()
		r := h.do("GET", "/v1/overview", adminToken, nil).want(t, 200)
		var found bool
		for _, raw := range r.body["agents"].([]any) {
			st := raw.(map[string]any)
			if str(st, "agent", "name") != "bob" {
				continue
			}
			found = true
			if st["state"] != wantState || st["queued"] != wantQueued || st["in_progress"] != wantProgress {
				t.Fatalf("overview: %s", r.raw)
			}
		}
		if !found {
			t.Fatalf("bob absent from overview: %s", r.raw)
		}
		r = h.do("GET", "/v1/console/recent", adminToken, nil).want(t, 200)
		items := r.body["items"].([]any)
		if len(items) != 1 || str(items[0], "state") != map[string]string{"pending": "queued", "working": "working"}[wantState] {
			t.Fatalf("console recent: %s", r.raw)
		}
	}
	check("pending", 1, 0)
	h.do("POST", "/v1/messages/"+id+"/ack", bob, nil).want(t, 200)
	check("working", 0, 1)
}

func TestFilesAndBroadcast(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", map[string]any{"labels": []string{"team=x"}})
	carol := h.register("carol", map[string]any{"labels": []string{"team=x"}})

	req, _ := http.NewRequest("POST", h.url+"/v1/files?name=notes.txt", strings.NewReader("hello file"))
	req.Header.Set("Authorization", "Bearer "+alice)
	req.Header.Set("Content-Type", "text/plain")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]any
	_ = json.NewDecoder(res.Body).Decode(&f)
	res.Body.Close()
	if res.StatusCode != 201 || f["name"] != "notes.txt" || f["size"] != float64(10) || f["owner"] != "alice" {
		t.Fatalf("upload: %d %v", res.StatusCode, f)
	}
	fid := f["id"].(string)

	// Not shared yet: bob cannot see it.
	h.do("GET", "/v1/files/"+fid+"/meta", bob, nil).wantErr(t, 404, "not_found")
	// Broadcast with the attachment grants every recipient access.
	r := h.do("POST", "/v1/broadcasts", alice, map[string]any{"selector": "team=x", "body": "read this", "attachments": []string{fid}}).want(t, 201)
	if ms, _ := r.body["messages"].([]any); len(ms) != 2 {
		t.Fatalf("broadcast: %s", r.raw)
	}
	bcID := str(r.body, "broadcast_id")
	for _, tok := range []string{bob, carol} {
		h.do("GET", "/v1/files/"+fid+"/meta", tok, nil).want(t, 200)
	}
	r = h.doReq("GET", "/v1/files/"+fid, bob, nil, nil).want(t, 200)
	if string(r.raw) != "hello file" || r.header.Get("X-Myco-SHA256") != f["sha256"] || r.header.Get("X-Myco-File-Name") != "notes.txt" ||
		r.header.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(r.header.Get("Content-Type"), "text/plain") {
		t.Fatalf("download: %v %q", r.header, r.raw)
	}

	r = h.do("GET", "/v1/broadcasts/"+bcID, alice, nil).want(t, 200)
	if r.body["total"] != float64(2) || r.body["answered"] != float64(0) {
		t.Fatalf("broadcast status: %s", r.raw)
	}
	h.do("POST", "/v1/broadcasts", alice, map[string]any{"selector": "team=x", "body": " "}).wantErr(t, 400, "invalid_argument")
}

// The app uploads a file as its sender (@console:…) and attaches it to a
// task: the recipient may read it, other agents may not attach it.
func TestConsoleFiles(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)

	req, _ := http.NewRequest("POST", h.url+"/v1/console/files?name=shot.png", strings.NewReader("png bytes"))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "image/png")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]any
	_ = json.NewDecoder(res.Body).Decode(&f)
	res.Body.Close()
	owner, _ := f["owner"].(string)
	if res.StatusCode != 201 || f["name"] != "shot.png" || !strings.HasPrefix(owner, "@console:") {
		t.Fatalf("console upload: %d %v", res.StatusCode, f)
	}
	fid := f["id"].(string)

	// An agent cannot upload as the console, nor attach the console's file.
	h.doReq("POST", "/v1/console/files?name=x", alice, nil, nil).wantErr(t, 403, "permission_denied")
	h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "look", "attachments": []string{fid}}).wantErr(t, 404, "not_found")

	r := h.do("POST", "/v1/console/messages", adminToken, map[string]any{"to": "bob", "body": "看看这张图", "attachments": []string{fid}}).want(t, 201)
	if m, _ := r.body["message"].(map[string]any); m == nil || len(m["attachments"].([]any)) != 1 {
		t.Fatalf("console send with attachment: %s", r.raw)
	}
	r = h.doReq("GET", "/v1/files/"+fid, bob, nil, nil).want(t, 200)
	if string(r.raw) != "png bytes" {
		t.Fatalf("recipient download: %q", r.raw)
	}
}
