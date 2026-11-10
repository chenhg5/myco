package console_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/console"
	"github.com/chenhg5/myco/internal/hubserver"
	"github.com/chenhg5/myco/internal/osutil"
	"github.com/zalando/go-keyring"
)

const adminToken = "console-test-admin-token-0123456789"

type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSecrets) Get(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[id]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}
func (s *memSecrets) Set(id, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = v
	return nil
}
func (s *memSecrets) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	return nil
}

type rig struct {
	t       *testing.T
	hubURL  string
	admin   *client.Client
	app     *httptest.Server
	secrets *memSecrets
	store   string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	h, err := hubserver.Build(hubserver.Options{DBPath: filepath.Join(t.TempDir(), "hub.db"), AdminToken: adminToken,
		Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(h.Handler)
	t.Cleanup(func() { hs.Close(); h.Close() })
	admin, _ := client.New(hs.URL, adminToken)
	r := &rig{t: t, hubURL: hs.URL, admin: admin, secrets: &memSecrets{m: map[string]string{}}, store: filepath.Join(t.TempDir(), "app", "hubs.json")}
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Timeout: 3 * time.Second}
	r.app = httptest.NewServer(b.Handler())
	t.Cleanup(r.app.Close)
	return r
}

func (r *rig) do(method, path string, body any) (int, map[string]any, []byte) {
	r.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, r.app.URL+path, rd)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m, raw
}

func (r *rig) consoleToken(name string) string {
	r.t.Helper()
	res, err := r.admin.CreateConsoleToken(context.Background(), name)
	if err != nil {
		r.t.Fatal(err)
	}
	return res.Token
}

func TestBackendManagesHubsAndAgents(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	reg, err := r.admin.Register(ctx, api.RegisterRequest{Name: "worker", Description: "old"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.admin.Register(ctx, api.RegisterRequest{Name: "lead"}, false); err != nil {
		t.Fatal(err)
	}

	// No hubs yet: an empty list (App mode with the first-run guide).
	if code, _, raw := r.do("GET", "/app/hubs", nil); code != 200 || strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("empty hubs: %d %s", code, raw)
	}

	// Bad input and the wrong kind of token are refused, and nothing is saved.
	if code, m, _ := r.do("POST", "/app/hubs", map[string]string{"name": "x", "url": "ftp://x", "token": "t"}); code != 400 || m["error"] != "invalid_argument" {
		t.Fatalf("bad url: %d %v", code, m)
	}
	if code, m, _ := r.do("POST", "/app/hubs", map[string]string{"name": "x", "url": r.hubURL, "token": reg.Token}); code != 403 || !strings.Contains(m["message"].(string), "agent token") {
		t.Fatalf("agent token: %d %v", code, m)
	}
	if code, m, _ := r.do("POST", "/app/hubs", map[string]string{"name": "x", "url": r.hubURL, "token": app.ConsoleTokenPrefix + "nope"}); code != 401 {
		t.Fatalf("made-up token: %d %v", code, m)
	}
	if len(r.secrets.m) != 0 {
		t.Fatal("a refused hub left a secret behind")
	}

	// Add the hub with a console token: saved in the store, token only in secrets.
	tok := r.consoleToken("test-mac")
	code, m, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL + "/", "token": tok})
	if code != 201 || m["name"] != "Main" || m["url"] != r.hubURL {
		t.Fatalf("add hub: %d %v", code, m)
	}
	id := m["id"].(string)
	stored, _ := os.ReadFile(r.store)
	if strings.Contains(string(stored), tok) || r.secrets.m[id] != tok {
		t.Fatalf("token must be in secrets only; store: %s", stored)
	}
	if st, _ := os.Stat(r.store); osutil.UnixPermissions && st.Mode().Perm() != 0o600 {
		t.Fatalf("store mode %v", st.Mode())
	}

	// A second hub that is down: listed with an error, the first still works.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	downTok := r.consoleToken("down")
	if code, _, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Down", "url": deadURL, "token": downTok}); code != 502 {
		t.Fatalf("adding an unreachable hub: %d", code)
	}
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets}
	if _, err := b.Store.Add("Down", deadURL); err != nil {
		t.Fatal(err)
	}

	var hubs []console.HubStatus
	_, _, raw := r.do("GET", "/app/hubs", nil)
	if err := json.Unmarshal(raw, &hubs); err != nil || len(hubs) != 2 || !hubs[0].OK || hubs[1].OK || hubs[1].Error == "" {
		t.Fatalf("hubs: %v %s", err, raw)
	}

	var ov struct {
		Hubs []console.HubOverview `json:"hubs"`
	}
	_, _, raw = r.do("GET", "/app/overview", nil)
	if err := json.Unmarshal(raw, &ov); err != nil || len(ov.Hubs) != 2 || !ov.Hubs[0].OK || ov.Hubs[0].Overview.Viewer.Kind != "admin" ||
		len(ov.Hubs[0].Overview.Agents) != 2 || ov.Hubs[1].OK || ov.Hubs[1].Error == "" {
		t.Fatalf("overview: %v %s", err, raw)
	}

	// Agent operations go to the right hub with the console token.
	if code, m, _ := r.do("PUT", "/app/hubs/"+id+"/agents/worker/profile", map[string]any{"display_name": "Worker", "description": "new"}); code != 200 || m["display_name"] != "Worker" {
		t.Fatalf("profile: %d %v", code, m)
	}
	if code, m, _ := r.do("PUT", "/app/hubs/"+id+"/agents/worker/allow_from", map[string]any{"allow_from": []string{"lead"}}); code != 200 {
		t.Fatalf("allow_from: %d %v", code, m)
	}
	if code, _, _ := r.do("PUT", "/app/hubs/"+id+"/agents/worker/allow_from", map[string]any{}); code != 400 {
		t.Fatalf("allow_from missing: %d", code)
	}
	// Group and tags: move to another group (the app removes the old one), add a tag.
	r.do("PUT", "/app/hubs/"+id+"/agents/worker/labels", map[string]any{"add": []string{"group=旧"}})
	code, m, _ = r.do("PUT", "/app/hubs/"+id+"/agents/worker/labels", map[string]any{"add": []string{"group=官网改版", "前端"}, "remove": []string{"group=旧"}})
	if code != 200 || fmt.Sprint(m["labels"]) != "[group=官网改版 前端]" {
		t.Fatalf("labels: %d %v", code, m)
	}
	if code, m, _ := r.do("PUT", "/app/hubs/"+id+"/agents/worker/labels", map[string]any{"add": []string{"Bad Tag"}}); code != 400 {
		t.Fatalf("bad label: %d %v", code, m)
	}
	a, err := r.admin.GetAgent(ctx, "worker")
	if err != nil || a.DisplayName != "Worker" || a.Description != "new" || len(a.AllowFrom) != 1 {
		t.Fatalf("agent after edits: %v %+v", err, a)
	}
	if code, m, _ := r.do("POST", "/app/hubs/"+id+"/agents/worker/kick", nil); code != 200 || m["kicked"] != true {
		t.Fatalf("kick: %d %v", code, m)
	}
	wc, _ := client.New(r.hubURL, reg.Token)
	if _, err := wc.Me(ctx); err == nil {
		t.Fatal("kicked agent's token still works")
	}
	if code, m, _ := r.do("GET", "/app/hubs/"+id+"/threads/msg_nope", nil); code != 404 || m["error"] != "not_found" {
		t.Fatalf("hub errors keep their status and shape: %d %v", code, m)
	}
	if code, _, _ := r.do("DELETE", "/app/hubs/"+id+"/agents/worker", nil); code != 200 {
		t.Fatalf("remove agent: %d", code)
	}
	if _, err := r.admin.GetAgent(ctx, "worker"); err == nil {
		t.Fatal("agent not removed")
	}
	if code, _, _ := r.do("POST", "/app/hubs/hub_nope/agents/lead/kick", nil); code != 404 {
		t.Fatalf("unknown hub: %d", code)
	}

	// A revoked console token shows up as an error on that hub.
	toks, _ := r.admin.ListConsoleTokens(ctx)
	for _, ct := range toks {
		if ct.Name == "test-mac" {
			if _, err := r.admin.RevokeConsoleToken(ctx, ct.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, _, raw = r.do("GET", "/app/overview", nil)
	_ = json.Unmarshal(raw, &ov)
	if ov.Hubs[0].OK || ov.Hubs[0].Error == "" {
		t.Fatalf("revoked token must show as an error: %s", raw)
	}

	// Removing a hub deletes its token.
	if code, _, _ := r.do("DELETE", "/app/hubs/"+id, nil); code != 200 {
		t.Fatalf("remove hub: %d", code)
	}
	if _, ok := r.secrets.m[id]; ok {
		t.Fatal("removing a hub left its token in the keychain")
	}
	if code, _, _ := r.do("DELETE", "/app/hubs/"+id, nil); code != 404 {
		t.Fatalf("removing twice: %d", code)
	}
}

func TestKeychainRoundTrip(t *testing.T) {
	keyring.MockInit()
	k := console.Keychain{}
	if err := k.Set("hub_1", "myco_console_x"); err != nil {
		t.Fatal(err)
	}
	if v, err := k.Get("hub_1"); err != nil || v != "myco_console_x" {
		t.Fatalf("get: %q %v", v, err)
	}
	if err := k.Delete("hub_1"); err != nil {
		t.Fatal(err)
	}
	if err := k.Delete("hub_1"); err != nil {
		t.Fatalf("deleting twice must be fine: %v", err)
	}
	if _, err := k.Get("hub_1"); err == nil {
		t.Fatal("deleted token still there")
	}
}

func TestBackendMachinesAndLinks(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	reg, err := r.admin.Register(ctx, api.RegisterRequest{Name: "mac-claude", Binding: api.Binding{Harness: "claude", Environment: "glenn-mac"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	wc, _ := client.New(r.hubURL, reg.Token)
	if err := wc.ReportMachine(ctx, api.MachineReport{Fingerprint: strings.Repeat("ef", 16), Name: "glenn-mac", OS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	ms, err := r.admin.Machines(ctx)
	if err != nil || len(ms) != 1 {
		t.Fatalf("machines: %v %+v", err, ms)
	}
	code, m, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("t")})
	if code != 201 {
		t.Fatalf("add hub: %d %v", code, m)
	}
	id := m["id"].(string)

	if code, m, _ := r.do("PATCH", "/app/hubs/"+id+"/machines/"+ms[0].ID, map[string]string{"name": "Glenn 的 Mac"}); code != 200 || m["name"] != "Glenn 的 Mac" {
		t.Fatalf("rename: %d %v", code, m)
	}
	// No machine node on it: the hub's reason and suggestion come through.
	if code, m, _ := r.do("POST", "/app/hubs/"+id+"/machines/"+ms[0].ID+"/spawn", map[string]string{"cli": "claude", "dir": "~/code"}); code != 409 || m["error"] != "invalid_state" || m["suggestion"] == nil {
		t.Fatalf("spawn without a machine node: %d %v", code, m)
	}
	code, m, _ = r.do("POST", "/app/hubs/"+id+"/links", map[string]any{})
	if code != 200 || !strings.Contains(m["join_url"].(string), "/j/") || !strings.HasPrefix(m["command"].(string), "curl -fsSL ") {
		t.Fatalf("links: %d %v", code, m)
	}
}

// A hub from before the recent list answers 404: the app falls back to the
// conversations list instead of showing an error.
func TestRecentFallsBackOnOldHub(t *testing.T) {
	old := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(old.Close)
	r := newRig(t)
	h, err := console.NewStore(r.store).Add("old hub", old.URL)
	if err != nil {
		t.Fatal(err)
	}
	r.secrets.m[h.ID] = "myco_console_x"
	code, m, raw := r.do("GET", "/app/hubs/"+h.ID+"/recent", nil)
	if code != 200 || m["fallback"] != true {
		t.Fatalf("old hub: %d %s", code, raw)
	}
}

// The recent list through the app: a shell command that failed shows as
// failed, while the hub only knows the task was answered.
func TestRecentThroughTheApp(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	reg := func(name, harness string, allow ...string) *client.Client {
		res, err := r.admin.Register(ctx, api.RegisterRequest{Name: name, AllowFrom: allow, Binding: api.Binding{Harness: harness, SessionID: "s-" + name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := client.New(r.hubURL, res.Token)
		return c
	}
	lead := reg("lead", "claude")
	box := reg("box", "exec", "lead")
	m, err := lead.Send(ctx, api.SendRequest{To: "box", Body: "go vet ./..."}, false)
	if err != nil {
		t.Fatal(err)
	}
	box.Pull(ctx, 1, 0)
	box.Reply(ctx, m.Message.ID, `{"exit_code":1,"stderr":"vet: bad"}`, false)

	code, h, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("test-mac")})
	if code != 201 {
		t.Fatalf("add hub: %d %v", code, h)
	}
	var out console.RecentResponse
	_, _, raw := r.do("GET", "/app/hubs/"+h["id"].(string)+"/recent", nil)
	if err := json.Unmarshal(raw, &out); err != nil || out.Fallback || len(out.Items) != 1 {
		t.Fatalf("recent: %v %s", err, raw)
	}
	if it := out.Items[0]; it.Kind != "pair" || it.Other != "lead" || it.Node != "box" || it.State != "failed" {
		t.Fatalf("row: %+v", it)
	}
}
