package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/console"
)

// fakeCLI stands in for the myco binary: `hub up` writes hub.env pointing
// at the rig's real hub (as myco does) and reports a quick-tunnel URL;
// `console token create` issues a real console token with the admin token.
type fakeCLI struct {
	r        *rig
	home     string
	calls    [][]string
	failUp   bool
	installs int
}

func (f *fakeCLI) run(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if !slices.ContainsFunc(env, func(kv string) bool {
		return strings.HasPrefix(kv, "PATH=") && strings.Contains(kv, filepath.Join(".local", "bin"))
	}) {
		f.r.t.Errorf("myco must run with the usual install dirs on PATH: %v", env)
	}
	switch strings.Join(args[:2], " ") {
	case "version --offline":
		return []byte(`{"version":"v0.16.9"}`), nil
	case "hub up":
		if f.failUp {
			return []byte(`{"error":"tunnel_not_ready","message":"Funnel is not enabled for this device","suggestion":"enable it: tailscale funnel 7777"}`), errors.New("exit status 2")
		}
		dir := filepath.Join(f.home, ".myco", "hub")
		os.MkdirAll(dir, 0o700)
		os.WriteFile(filepath.Join(dir, "hub.env"), []byte("MYCO_ADMIN_TOKEN="+adminToken+"\nMYCO_HUB="+f.r.hubURL+"\n"), 0o600)
		return []byte(`{"public_url":"https://abc.trycloudflare.com","reachable":false,"join_url":"https://abc.trycloudflare.com/j/myco_enroll_x"}`), nil
	case "console token":
		if !slices.Contains(args, "--hub") || !slices.Contains(args, "test-mac-app") {
			f.r.t.Errorf("console token create args: %v", args)
		}
		res, err := f.r.admin.CreateConsoleToken(context.Background(), "test-mac-app")
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"token": res.Token})
	case "hub down":
		return []byte(`{"stopped":true}`), nil
	}
	f.r.t.Fatalf("unexpected myco call: %v", args)
	return nil, nil
}

func TestLocalHub(t *testing.T) {
	r := newRig(t)
	home := t.TempDir()
	f := &fakeCLI{r: r, home: home}
	mycoPath := ""
	l := &console.Local{Home: home, Device: "test-mac", Run: f.run, Dirs: []string{filepath.Join(home, ".local", "bin")},
		LookPath: func(name string) (string, error) {
			if name == "myco" && mycoPath != "" {
				return mycoPath, nil
			}
			if name == "cloudflared" {
				return "/opt/homebrew/bin/cloudflared", nil
			}
			return "", errors.New("not found")
		},
		Install: func(_ context.Context, dir string) (string, error) {
			f.installs++
			mycoPath = filepath.Join(dir, "myco")
			return mycoPath, nil
		}}
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Local: l, Shell: "mac", Version: "v1"}
	r.app.Config.Handler = b.Handler()

	if code, m, _ := r.do("GET", "/app/info", nil); code != 200 || m["shell"] != "mac" {
		t.Fatalf("info: %d %v", code, m)
	}
	var st console.LocalStatus
	_, _, raw := r.do("GET", "/app/local", nil)
	if err := json.Unmarshal(raw, &st); err != nil || st.Myco != nil || st.Hub != nil || st.Tools["cloudflared"] == nil || st.Tools["tailscale"] != nil || st.Device != "test-mac" {
		t.Fatalf("status before: %v %s", err, raw)
	}
	if code, m, _ := r.do("POST", "/app/local/myco", nil); code != 200 || !strings.HasPrefix(filepath.Base(m["path"].(string)), "myco") || filepath.Base(filepath.Dir(m["path"].(string))) != "bin" || f.installs != 1 {
		t.Fatalf("install: %d %v", code, m)
	}
	r.do("POST", "/app/local/myco", nil)
	if f.installs != 1 {
		t.Fatal("installing again must not download again")
	}

	for _, bad := range []map[string]string{{"tunnel": "ngrok"}, {"tunnel": "tailscale", "hostname": "x.example.com"}} {
		if code, _, _ := r.do("POST", "/app/local/hub", bad); code != 400 {
			t.Fatalf("bad request %v: %d", bad, code)
		}
	}
	f.failUp = true
	if code, m, _ := r.do("POST", "/app/local/hub", map[string]string{"tunnel": "tailscale"}); code != 502 || m["error"] != "tunnel_not_ready" || !strings.Contains(m["suggestion"].(string), "funnel") {
		t.Fatalf("the CLI's error must come through: %d %v", code, m)
	}
	f.failUp = false

	var res console.StartLocalResult
	code, _, raw := r.do("POST", "/app/local/hub", map[string]string{"tunnel": "cloudflare"})
	if err := json.Unmarshal(raw, &res); err != nil || code != 201 || res.Hub.URL != r.hubURL || res.Hub.Name != "本机 Hub" ||
		res.PublicURL != "https://abc.trycloudflare.com" || res.JoinURL == "" || len(res.Warnings) != 2 {
		t.Fatalf("start: %d %v %s", code, err, raw)
	}
	if r.secrets.m[res.Hub.ID] == "" {
		t.Fatal("console token not stored")
	}
	_, _, raw = r.do("GET", "/app/local", nil)
	if err := json.Unmarshal(raw, &st); err != nil || st.Myco == nil || st.Myco.Version != "v0.16.9" || st.Hub == nil || !st.Hub.Running || !st.Hub.Added || st.Hub.LocalURL != r.hubURL {
		t.Fatalf("status after: %v %s", err, raw)
	}
	// The overview now includes the local hub, through its console token.
	var ov struct{ Hubs []console.HubOverview }
	_, _, raw = r.do("GET", "/app/overview", nil)
	if json.Unmarshal(raw, &ov) != nil || len(ov.Hubs) != 1 || !ov.Hubs[0].OK {
		t.Fatalf("overview with the local hub: %s", raw)
	}
	// Starting again reuses it: no second entry, no second token.
	tokens := len(r.secrets.m)
	if code, _, _ := r.do("POST", "/app/local/hub", map[string]string{"tunnel": "cloudflare"}); code != 200 || len(r.secrets.m) != tokens {
		t.Fatalf("second start: %d (tokens %d -> %d)", code, tokens, len(r.secrets.m))
	}
	if hubs, _ := console.NewStore(r.store).List(); len(hubs) != 1 {
		t.Fatalf("hubs after a second start: %+v", hubs)
	}
	if code, m, _ := r.do("POST", "/app/local/hub/stop", nil); code != 200 || m["stopped"] != true {
		t.Fatalf("stop: %d %v", code, m)
	}
	if last := f.calls[len(f.calls)-1]; strings.Join(last[:2], " ") != "hub down" || !slices.Contains(last, "json") {
		t.Fatalf("stop ran %v", last)
	}
}

func TestNoLocalRoutesWithoutLocal(t *testing.T) {
	b := &console.Backend{Store: console.NewStore(filepath.Join(t.TempDir(), "h.json")), Secrets: &memSecrets{m: map[string]string{}}}
	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/app/local", nil))
	if rec.Code != 404 {
		t.Fatalf("/app/local without Local: %d", rec.Code)
	}
}
