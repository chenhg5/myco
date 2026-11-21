package oscap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type proxySecrets struct {
	mu   sync.Mutex
	data map[string]string
}

func (s *proxySecrets) Get(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[k]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}
func (s *proxySecrets) Set(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[k] = v
	return nil
}
func (s *proxySecrets) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, k)
	return nil
}
func proxyCallTest(t *testing.T, e *Env, cap string, args any) (any, *Error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	return Call(context.Background(), e, cap, raw)
}
func TestProxyProfileAndController(t *testing.T) {
	const secret = "DO-NOT-LEAK-THIS"
	var mu sync.Mutex
	var actions []string
	mode := "rule"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("missing credential")
			w.WriteHeader(401)
			return
		}
		mu.Lock()
		actions = append(actions, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version":
			w.Write([]byte(`{"version":"test"}`))
		case "/configs":
			if r.Method == "PATCH" {
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				mode = body["mode"]
				w.WriteHeader(204)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"mode": mode, "mixed-port": 7890, "secret": secret, "authentication": []string{"also-secret"}})
		case "/proxies":
			w.Write([]byte(`{"proxies":{"choose":{"name":"choose","type":"Selector","now":"node","all":["node"],"password":"DO-NOT-LEAK-THIS"}}}`))
		case "/connections":
			w.Write([]byte(`{"downloadTotal":123,"uploadTotal":45,"connections":[{"id":"abc","metadata":{"host":"example.com","secret":"DO-NOT-LEAK-THIS"}}]}`))
		case "/proxies/choose":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "node" {
				t.Errorf("selection=%v", body)
			}
			w.WriteHeader(204)
		case "/proxies/node/delay":
			w.Write([]byte(`{"delay":23}`))
		case "/connections/abc":
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	store := &proxySecrets{data: map[string]string{}}
	e := &Env{Secrets: store, Scope: "hub\x00page-one"}
	v, err := proxyCallTest(t, e, "proxy.profile", map[string]any{"action": "save", "endpoint": srv.URL, "engine": "mihomo", "secret": secret})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(v)
	if strings.Contains(string(encoded), secret) || !strings.Contains(string(encoded), `"has_secret":true`) {
		t.Fatal(string(encoded))
	}
	other := *e
	other.Scope = "hub\x00page-two"
	if _, err := proxyCallTest(t, &other, "proxy.profile", nil); err == nil || err.Code != CodeNotFound {
		t.Fatal("profiles must be page scoped", err)
	}
	v, err = proxyCallTest(t, e, "proxy.control", map[string]string{"action": "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(v)
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "also-secret") {
		t.Fatalf("snapshot leaked credentials: %s", encoded)
	}
	for _, a := range []map[string]string{{"action": "mode", "mode": "global"}, {"action": "select", "group": "choose", "name": "node"}, {"action": "delay", "name": "node"}, {"action": "close", "id": "abc"}} {
		if _, err := proxyCallTest(t, e, "proxy.control", a); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	n := len(actions)
	mu.Unlock()
	if n != 9 {
		t.Fatalf("requests=%d", n)
	}
	if _, err := proxyCallTest(t, e, "proxy.control", map[string]string{"action": "mode", "mode": "invalid"}); err == nil || err.Code != CodeInvalid {
		t.Fatal(err)
	}
	// Keep a credential for the same endpoint; do not forward it to a new one.
	proxyCallTest(t, e, "proxy.profile", map[string]string{"action": "save", "endpoint": srv.URL, "engine": "sing-box"})
	p, _ := loadProxy(e)
	if p.Secret != secret {
		t.Fatal("lost saved credential")
	}
	proxyCallTest(t, e, "proxy.profile", map[string]string{"action": "save", "endpoint": "http://127.0.0.1:9999", "engine": "mihomo"})
	p, _ = loadProxy(e)
	if p.Secret != "" {
		t.Fatal("credential followed endpoint change")
	}
	proxyCallTest(t, e, "proxy.profile", map[string]string{"action": "forget"})
	if _, err := loadProxy(e); err == nil {
		t.Fatal("forget did not remove profile")
	}
}
func TestProxyRejectsUnsafeEndpoints(t *testing.T) {
	for _, s := range []string{"http://localhost:9090", "http://192.168.1.1:9090", "http://example.com:9090", "http://user:secret@127.0.0.1:9090", "file:///etc/passwd", "http://127.0.0.1:9090?token=x", "http://127.0.0.1:9090/configs", "http://127.0.0.1", "http://[::ffff:192.168.1.2]:9090"} {
		if _, err := proxyEndpoint(s); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
	for _, s := range []string{"http://127.0.0.1:9090", "http://[::1]:9090/"} {
		if _, err := proxyEndpoint(s); err != nil {
			t.Fatal(s, err)
		}
	}
}
func TestProxyErrorsAndNoRedirect(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	for _, status := range []int{401, 403, 404, 500, 302} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				w.Write([]byte("private error secret"))
			}))
			defer srv.Close()
			err := proxyRequest(context.Background(), proxyProfile{Endpoint: srv.URL, Secret: "secret"}, "GET", "/version", nil, &struct{}{})
			if err == nil || strings.Contains(err.Error(), "private") {
				t.Fatal(err)
			}
		})
	}
	if reached {
		t.Fatal("controller redirect was followed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := proxyRequest(ctx, proxyProfile{Endpoint: target.URL}, "GET", "/version", nil, &struct{}{}); err == nil {
		t.Fatal("cancel ignored")
	}
}
func TestProxyPartialSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			w.Write([]byte(`{"version":"sing-box"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	e := &Env{Secrets: &proxySecrets{data: map[string]string{}}, Scope: "x"}
	proxyCallTest(t, e, "proxy.profile", map[string]string{"action": "save", "endpoint": srv.URL, "engine": "sing-box"})
	v, err := proxyCallTest(t, e, "proxy.control", map[string]string{"action": "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.(map[string]any)["warnings"].([]string)) != 3 {
		t.Fatal(v)
	}
}

func TestProxyReportsIgnoredMode(t *testing.T) {
	// Some sing-box configurations accept PATCH but ignore an undefined mode.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			w.WriteHeader(204)
			return
		}
		w.Write([]byte(`{"mode":"rule","mode-list":["rule"]}`))
	}))
	defer srv.Close()
	e := &Env{Secrets: &proxySecrets{data: map[string]string{}}, Scope: "ignored-mode"}
	proxyCallTest(t, e, "proxy.profile", map[string]string{"action": "save", "endpoint": srv.URL, "engine": "sing-box"})
	_, err := proxyCallTest(t, e, "proxy.control", map[string]string{"action": "mode", "mode": "global"})
	if err == nil || err.Code != CodeUnsupported {
		t.Fatal("reported success for ignored mode", err)
	}
}
