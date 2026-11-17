package cli

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// A live page: a dev server on this computer, forwarded through the hub
// while myco page serve runs. Plain requests and a WebSocket (hot reload)
// go through; the caller's token does not reach the server; stopped, the
// page says it is offline.
func TestPageServeForwardsALocalServer(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "agent.yaml")}}
	if code, _, e := h.run("agent", "register", "--name", "front"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	var mu sync.Mutex
	var sawAuth, sawHost string
	dev := http.NewServeMux()
	dev.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer c.CloseNow()
		typ, msg, err := c.Read(r.Context())
		if err == nil {
			_ = c.Write(r.Context(), typ, append([]byte("echo:"), msg...))
		}
	})
	dev.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sawAuth, sawHost = r.Header.Get("Authorization"), r.Host
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<h1>dev "+r.URL.Path+"?"+r.URL.RawQuery+"</h1>")
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: dev}
	go srv.Serve(ln)
	defer srv.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	ctx, stop := context.WithCancel(context.Background())
	var out, errb lockedBuf
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, []string{"page", "serve", "--port", strconv.Itoa(port), "--name", "前端预览", "--format", "json"}, strings.NewReader(""), &out, &errb, func(k string) string { return h.env[k] }, false)
	}()
	// The page is published, then forwarded.
	get := func(path, token string) (int, string) {
		req, _ := http.NewRequest("GET", hub+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	var id string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(errb.String(), "connected") {
			var p struct{ ID string }
			json.Unmarshal([]byte(strings.TrimSpace(out.String())), &p)
			id = p.ID
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if id == "" {
		t.Fatalf("not connected: %s %s", out.String(), errb.String())
	}
	// (the hub takes the tunnel up a moment after it connects)
	for i := 0; i < 100; i++ {
		if _, body := get("/v1/pages/"+id, "admin-token-123456"); strings.Contains(body, `"online":true`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code, body := get("/v1/pages/"+id, "admin-token-123456"); code != 200 || !strings.Contains(body, `"live":true`) || !strings.Contains(body, `"online":true`) {
		t.Fatalf("page: %d %s", code, body)
	}
	if code, body := get("/v1/pages/"+id+"/live/src/main.ts?v=1", "admin-token-123456"); code != 200 || body != "<h1>dev /src/main.ts?v=1</h1>" {
		t.Fatalf("forwarded: %d %s", code, body)
	}
	mu.Lock()
	auth, host := sawAuth, sawHost
	mu.Unlock()
	if auth != "" || host != "localhost:"+strconv.Itoa(port) {
		t.Fatalf("the server saw Authorization %q, Host %q", auth, host)
	}
	// A WebSocket (hot reload) through the hub.
	wsctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(wsctx, hub+"/v1/pages/"+id+"/live/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer admin-token-123456"}}})
	if err != nil {
		t.Fatalf("websocket: %v", err)
	}
	if err := ws.Write(wsctx, websocket.MessageText, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if _, msg, err := ws.Read(wsctx); err != nil || string(msg) != "echo:hi" {
		t.Fatalf("websocket echo: %q %v", msg, err)
	}
	ws.Close(websocket.StatusNormalClosure, "")

	// Stopped: offline, and it says how to take it up again.
	stop()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("serve exit %d: %s", code, errb.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop")
	}
	deadline = time.Now().Add(5 * time.Second)
	code, body := 0, ""
	for time.Now().Before(deadline) {
		if code, body = get("/v1/pages/"+id+"/live/", "admin-token-123456"); code == 503 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if code != 503 || !strings.Contains(body, "page serve --page "+id) {
		t.Fatalf("offline: %d %s", code, body)
	}
	// Another agent cannot take it over.
	h2 := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "b.yaml")}}
	h2.run("agent", "register", "--name", "other")
	if code, _, e := h2.run("page", "serve", "--port", strconv.Itoa(port), "--page", id); code == 0 || !strings.Contains(e, "forwarded by front") {
		t.Fatalf("taken over: %d %s", code, e)
	}
}

// lockedBuf is a buffer written by one goroutine and read by another.
type lockedBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
