package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/client"
)

func TestProxyConfigCheck(t *testing.T) {
	ok := []ProxyConfig{
		{Mode: "system"}, {Mode: "none"},
		{Mode: "manual", Type: "socks5", Host: "127.0.0.1", Port: 1080},
		{Mode: "manual", Type: "http", Host: "proxy.corp", Port: 8080, User: "me"},
	}
	for _, c := range ok {
		if err := c.check(); err != nil {
			t.Errorf("%+v: %v", c, err)
		}
	}
	bad := []ProxyConfig{
		{Mode: "auto"},
		{Mode: "manual", Type: "ftp", Host: "x", Port: 1},
		{Mode: "manual", Type: "http", Host: "http://x", Port: 80},
		{Mode: "manual", Type: "http", Host: "", Port: 80},
		{Mode: "manual", Type: "http", Host: "x", Port: 0},
	}
	for _, c := range bad {
		if c.check() == nil {
			t.Errorf("%+v should be refused", c)
		}
	}
}

func TestProxyForGoesDirectOnTheLocalNetwork(t *testing.T) {
	f := proxyFor(ProxyConfig{Mode: "manual", Type: "socks5", Host: "127.0.0.1", Port: 1080, User: "me"}, "pw")
	at := func(raw string) string {
		u, _ := url.Parse(raw)
		p, err := f(&http.Request{URL: u})
		if err != nil {
			t.Fatal(err)
		}
		if p == nil {
			return "direct"
		}
		return p.Redacted()
	}
	if got := at("https://hub.example.com/v1/x"); got != "socks5://me:xxxxx@127.0.0.1:1080" {
		t.Fatalf("a remote hub goes through the proxy: %s", got)
	}
	for _, local := range []string{"http://127.0.0.1:7780/", "http://localhost:8080/", "http://192.168.1.20/", "http://mac.local/"} {
		if got := at(local); got != "direct" {
			t.Fatalf("%s should go direct, got %s", local, got)
		}
	}
	none := proxyFor(ProxyConfig{Mode: "none"}, "")
	if p, _ := none(&http.Request{URL: &url.URL{Scheme: "https", Host: "x.example"}}); p != nil {
		t.Fatalf("no proxy: %v", p)
	}
}

func TestProxyRoutesSaveApplyAndTest(t *testing.T) {
	defer client.SetProxy(nil)
	// A proxy that answers every request it is asked to make.
	var asked []string
	px := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.String())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer px.Close()
	pu, _ := url.Parse(px.URL)
	host, port := pu.Hostname(), pu.Port()

	keys := &memKeys{m: map[string]string{}}
	b := &Backend{Store: NewStore(filepath.Join(t.TempDir(), "hubs.json")), Secrets: keys}
	h := b.Handler()
	do := func(method, path, body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, out := do("GET", "/app/proxy", ""); code != 200 || out["mode"] != "system" {
		t.Fatalf("default: %d %v", code, out)
	}
	cfg := `{"mode":"manual","type":"http","host":"` + host + `","port":` + port + `,"user":"me","password":"s3cret","url":"http://example.test/"}`
	// Tried before it is saved: the request goes through the proxy.
	if code, out := do("POST", "/app/proxy/test", cfg); code != 200 || out["ok"] != true {
		t.Fatalf("test: %d %v", code, out)
	}
	if len(asked) != 1 || asked[0] != "http://example.test/" {
		t.Fatalf("the proxy was asked %v", asked)
	}
	if code, out := do("PUT", "/app/proxy", cfg); code != 200 || out["has_password"] != true {
		t.Fatalf("save: %d %v", code, out)
	}
	if keys.m[proxySecret] != "s3cret" {
		t.Fatal("the password goes to the keychain")
	}
	// In force for the hub clients at once (the local network goes direct).
	u, _ := url.Parse("https://hub.example.com/")
	if p, _ := client.Proxy(&http.Request{URL: u}); p == nil || p.Host != host+":"+port {
		t.Fatalf("hub clients follow the setting: %v", p)
	}
	if code, out := do("GET", "/app/proxy", ""); out["password"] != nil || out["host"] != host || code != 200 {
		t.Fatalf("read back: %d %v", code, out)
	}
	if code, _ := do("PUT", "/app/proxy", `{"mode":"manual","type":"ftp","host":"x","port":1}`); code != http.StatusBadRequest {
		t.Fatalf("a bad setting: %d", code)
	}
	// Back to the environment's: the password is let go.
	if code, _ := do("PUT", "/app/proxy", `{"mode":"system"}`); code != 200 || keys.m[proxySecret] != "" {
		t.Fatalf("system: %d %v", code, keys.m)
	}
}
