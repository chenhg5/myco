package console_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/console"
)

// A page opens in a window of its own where the App can make one (the
// desktop App says so in /app/info); elsewhere the call says it cannot.
func TestPageOpensInAWindowOfItsOwn(t *testing.T) {
	r := newRig(t)
	code, h, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("t")})
	if code != 201 {
		t.Fatalf("add hub: %d", code)
	}
	hub := h["id"].(string)
	if code, _, raw := r.do("POST", "/app/hubs/"+hub+"/pages/pg_x/window", map[string]string{"name": "x"}); code != http.StatusNotImplemented || !strings.Contains(string(raw), "unsupported") {
		t.Fatalf("no windows here: %d %s", code, raw)
	}
	var got []string
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Timeout: 3 * time.Second,
		OpenPageWindow: func(hub, page, title string) error { got = []string{hub, page, title}; return nil }}
	app := httptest.NewServer(b.Handler())
	defer app.Close()
	res, err := http.Post(app.URL+"/app/hubs/"+hub+"/pages/pg_x/window", "application/json", strings.NewReader(`{"name":"报告"}`))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("open: %v %v", res, err)
	}
	res.Body.Close()
	if len(got) != 3 || got[0] != hub || got[1] != "pg_x" || got[2] != "报告" {
		t.Fatalf("window for: %v", got)
	}
	if res, _ := http.Post(app.URL+"/app/hubs/nope/pages/pg_x/window", "application/json", nil); res.StatusCode != 404 {
		t.Fatalf("no such hub: %d", res.StatusCode)
	}
	res, _ = http.Get(app.URL + "/app/info")
	var raw strings.Builder
	buf := make([]byte, 4096)
	n, _ := res.Body.Read(buf)
	raw.Write(buf[:n])
	res.Body.Close()
	if !strings.Contains(raw.String(), `"page_windows":true`) {
		t.Fatalf("info: %s", raw.String())
	}
}
