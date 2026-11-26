package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/harness"
)

type fakeHTTP struct {
	waitForCancel bool
	events        chan map[string]any
	permission    chan struct{}
	cancelled     chan struct{}
	deleted       atomic.Bool
	badHeaders    atomic.Bool
}

func newFakeHTTP() *fakeHTTP {
	return &fakeHTTP{events: make(chan map[string]any, 16), permission: make(chan struct{}, 1), cancelled: make(chan struct{}, 1)}
}
func (f *fakeHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-codebuddy-request") != "1" {
		f.badHeaders.Store(true)
		http.Error(w, "missing security header", 403)
		return
	}
	if r.URL.Path == "/api/v1/acp/connect" {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"connectionId":"test-connection","sessionToken":"test-secret"}`)
		return
	}
	if r.Header.Get("acp-connection-id") != "test-connection" || r.Header.Get("acp-session-token") != "test-secret" {
		f.badHeaders.Store(true)
		http.Error(w, "missing connection headers", 403)
		return
	}
	if r.Method == http.MethodDelete {
		f.deleted.Store(true)
		w.WriteHeader(204)
		return
	}
	sse := func(m map[string]any) {
		m["jsonrpc"] = "2.0"
		b, _ := json.Marshal(m)
		fmt.Fprintf(w, "event: message\r\ndata: %s\r\n\r\n", b)
		w.(http.Flusher).Flush()
	}
	w.Header().Set("Content-Type", "text/event-stream")
	if r.Method == http.MethodGet {
		w.(http.Flusher).Flush()
		for {
			select {
			case e := <-f.events:
				sse(e)
			case <-r.Context().Done():
				return
			}
		}
	}
	var m message
	if json.NewDecoder(r.Body).Decode(&m) != nil {
		w.WriteHeader(400)
		return
	}
	if m.Method == "" {
		f.permission <- struct{}{}
		w.WriteHeader(202)
		return
	}
	reply := func(result any) { sse(map[string]any{"id": m.ID, "result": result}) }
	switch m.Method {
	case "initialize":
		reply(map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"loadSession": true}})
	case "session/new":
		reply(map[string]any{"sessionId": "http-session"})
	case "session/load", "session/set_config_option":
		reply(map[string]any{})
	case "session/cancel":
		f.cancelled <- struct{}{}
		w.WriteHeader(202)
	case "session/prompt":
		f.events <- map[string]any{"method": "session/update", "params": map[string]any{"sessionId": "http-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "from GET "}}}}
		f.events <- map[string]any{"id": 999, "method": "session/request_permission", "params": map[string]any{"toolCall": map[string]any{"toolCallId": "test-tool", "kind": "execute", "title": "echo ready"}, "options": []map[string]string{{"optionId": "no", "kind": "reject_once"}}}}
		select {
		case <-f.permission:
		case <-r.Context().Done():
			return
		}
		// The permission reply proves the preceding GET update was dispatched.
		if f.waitForCancel {
			select {
			case <-f.cancelled:
				reply(map[string]any{"stopReason": "cancelled"})
			case <-r.Context().Done():
			}
			return
		}

		sse(map[string]any{"method": "session/update", "params": map[string]any{"sessionId": "http-session", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "and POST"}}}})
		reply(map[string]any{"stopReason": "end_turn"})
	default:
		reply(map[string]any{})
	}
}

func TestHTTPRPCConcurrentPermissionAndNotifications(t *testing.T) {
	f := newFakeHTTP()
	srv := httptest.NewServer(f)
	defer srv.Close()
	h, err := newHTTPRPC(t.Context(), t.Context(), srv.URL, &tailBuffer{max: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer h.close()
	var mu sync.Mutex
	var text strings.Builder
	h.c.onNotify = func(method string, p json.RawMessage) {
		var v struct {
			Update struct{ Content struct{ Text string } }
		}
		_ = json.Unmarshal(p, &v)
		mu.Lock()
		text.WriteString(v.Update.Content.Text)
		mu.Unlock()
	}
	h.c.onRequest = func(method string, p json.RawMessage) (any, *rpcError) {
		return map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, method := range []string{"initialize", "session/new", "session/load", "session/set_config_option", "session/prompt"} {
		var reply map[string]any
		if err = h.c.call(ctx, method, map[string]any{}, &reply); err != nil {
			h.close()
			t.Fatalf("%s: %v", method, err)
		}
	}
	if err = h.c.notify("session/cancel", map[string]string{"sessionId": "http-session"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.cancelled:
	case <-ctx.Done():
		t.Fatal("cancel was not sent")
	}
	h.close()
	mu.Lock()
	got := text.String()
	mu.Unlock()
	if got != "from GET and POST" {
		t.Fatalf("notifications: %q", got)
	}
	if !f.deleted.Load() || f.badHeaders.Load() {
		t.Fatal("connection teardown/headers failed")
	}
}

func TestHTTPTransportRejectsRedirectsAndNonlocalEndpoints(t *testing.T) {
	for _, bad := range []string{"https://127.0.0.1:1", "http://example.com:80", "http://127.0.0.1:0", "http://127.0.0.1:65536", "http://user:secret@127.0.0.1:1", "http://127.0.0.1:1/?token=secret"} {
		if _, err := localEndpoint(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	var followed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Store(true) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer srv.Close()
	if _, err := newHTTPRPC(t.Context(), t.Context(), srv.URL, &tailBuffer{max: 1024}); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("redirect: %v", err)
	}
	if followed.Load() {
		t.Fatal("followed redirect")
	}
}

func TestHTTPTransportSSEFramingAndBounds(t *testing.T) {
	var frames []string
	err := readSSE(strings.NewReader(": keepalive\r\nevent: message\r\ndata: {\"id\":1,\r\ndata: \"result\":{}}\r\n\r\ndata: {\"id\":2}\n\n"), func(b []byte) error { frames = append(frames, string(b)); return nil })
	if err != nil || len(frames) != 2 || !json.Valid([]byte(frames[0])) {
		t.Fatalf("SSE: %v %q", err, frames)
	}
	if err = readSSE(strings.NewReader("data: "+strings.Repeat("x", maxHTTPFrame+1)), func([]byte) error { return nil }); err == nil {
		t.Fatal("accepted unbounded SSE line")
	}
	w := &endpointWriter{found: make(chan string, 1)}
	w.Write([]byte("HTTP server http://127.0.0."))
	w.Write([]byte("1:12345/api/v1/acp\n"))
	select {
	case endpoint := <-w.found:
		if endpoint != "http://127.0.0.1:12345" {
			t.Fatal(endpoint)
		}
	default:
		t.Fatal("split announcement lost")
	}
}

func fakeHTTPAgent() {
	f := newFakeHTTP()
	f.waitForCancel = os.Getenv("MYCO_FAKE_ACP") == "http-cancel"
	srv := httptest.NewServer(f)
	defer srv.Close()
	fmt.Fprintln(os.Stderr, "HTTP server ready at "+srv.URL)
	// Like WorkBuddy, the child lives while stdin stays open.
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}

func TestHTTPHarnessProcessLifecycle(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	a := &agent{name: "http-test", spec: Spec{Binary: binary, Transport: "streamable-http"}}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	result, err := a.Run(ctx, harness.Request{Workdir: t.TempDir(), Prompt: "hello", Env: []string{"MYCO_FAKE_ACP=http-test"}})
	if err != nil || result.SessionID != "http-session" || result.Output != "and POST" {
		t.Fatalf("result %+v error %v", result, err)
	}
}

func TestHTTPHarnessCancelsOverProtocol(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	a := &agent{name: "http-test", spec: Spec{Binary: binary, Transport: "streamable-http"}}
	parent, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	cause := errors.New("test interrupted")
	result, err := a.Run(ctx, harness.Request{Workdir: t.TempDir(), Prompt: "wait", Env: []string{"MYCO_FAKE_ACP=http-cancel"}, OnEvent: func(e harness.Event) {
		if e.Kind == "text" {
			cancel(cause)
		}
	}})
	if err == nil || !strings.Contains(err.Error(), cause.Error()) || result.SessionID != "http-session" {
		t.Fatalf("cancel: result=%+v err=%v", result, err)
	}
}

func TestHTTPJSONResponsesAndSafeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/acp/connect" {
			fmt.Fprint(w, `{"connectionId":"test"}`)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		var m message
		_ = json.NewDecoder(r.Body).Decode(&m)
		if m.Method == "initialize" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"protocolVersion": 1}})
			return
		}
		http.Error(w, "fixture-private-response", 500)
	}))
	defer srv.Close()
	diag := &tailBuffer{max: 1024}
	h, err := newHTTPRPC(t.Context(), t.Context(), srv.URL, diag)
	if err != nil {
		t.Fatal(err)
	}
	defer h.close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var reply struct{ ProtocolVersion int }
	if err = h.c.call(ctx, "initialize", nil, &reply); err != nil || reply.ProtocolVersion != 1 {
		t.Fatalf("JSON response %v %+v", err, reply)
	}
	if err = h.c.call(ctx, "session/new", nil, nil); !errors.Is(err, errClosed) {
		t.Fatalf("failed HTTP must release pending calls: %v", err)
	}
	if strings.Contains(diag.String(), "fixture-private-response") || !strings.Contains(diag.String(), "500") {
		t.Fatal("unsafe or missing HTTP diagnostic")
	}
}

func TestHTTPTruncatedReplyReleasesPendingCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/acp/connect" {
			fmt.Fprint(w, `{"connectionId":"test"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		if r.Method == http.MethodGet {
			<-r.Context().Done()
			return
		}
		fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"session/update\",\"params\":{}}\n\n")
	}))
	defer srv.Close()
	h, err := newHTTPRPC(t.Context(), t.Context(), srv.URL, &tailBuffer{max: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer h.close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err = h.c.call(ctx, "initialize", nil, nil); !errors.Is(err, errClosed) {
		t.Fatalf("truncated reply: %v", err)
	}
}
