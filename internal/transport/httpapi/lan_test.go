package httpapi_test

import (
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
	"github.com/chenhg5/myco/internal/osutil"
)

// A hub on the local network (no tunnel, every interface): links made from
// this machine over 127.0.0.1 name its LAN address, so other computers can
// use them; a public URL still wins; requests from elsewhere keep their host.
func TestLinksUseTheLANAddress(t *testing.T) {
	t.Setenv("MYCO_LAN_ADDR", "192.168.1.20")
	ip := osutil.LANAddr()
	h, err := hubserver.Build(hubserver.Options{DBPath: ":memory:", AdminToken: adminToken, Policy: app.DefaultMessagePolicy(),
		OnlineWindow: time.Minute, FilesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	invite := func(host, name string) string {
		req := httptest.NewRequest("POST", "/v1/members", strings.NewReader(`{"name":"`+name+`"}`))
		req.Host = host
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("invite: %d %s", w.Code, w.Body)
		}
		var out struct{ Link string }
		json.Unmarshal(w.Body.Bytes(), &out)
		return out.Link
	}
	if l := invite("127.0.0.1:7777", "a"); !strings.HasPrefix(l, "http://127.0.0.1:7777/c/") {
		t.Fatalf("not on the LAN: %s", l)
	}
	h.SetLAN(true)
	if l := invite("127.0.0.1:7777", "b"); !strings.HasPrefix(l, "http://"+ip+":7777/c/") {
		t.Fatalf("from this machine: %s", l)
	}
	if l := invite("localhost:7777", "c"); !strings.HasPrefix(l, "http://"+ip+":7777/c/") {
		t.Fatalf("localhost: %s", l)
	}
	if l := invite("10.9.8.7:7777", "d"); !strings.HasPrefix(l, "http://10.9.8.7:7777/c/") {
		t.Fatalf("from another computer: %s", l)
	}
	h.SetPublicURL("https://hub.example.com")
	if l := invite("127.0.0.1:7777", "e"); !strings.HasPrefix(l, "https://hub.example.com/c/") {
		t.Fatalf("public URL: %s", l)
	}
}
