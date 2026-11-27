package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/console"
	"github.com/myco-ai/myco/internal/e2e"
	"github.com/myco-ai/myco/internal/testutil"
)

// fakeClaude stands in for Claude Code: it starts or resumes a session and
// writes down where its requests would go (the address, the key and the
// model it was given: from --settings, as myco passes a provider), for the
// test to check. It sends nothing anywhere.
const fakeClaude = `#!/bin/sh
case " $* " in *" --version "*) echo "2.1.300 (Claude Code)"; exit 0;; esac
settings=""; model=""; resume=""; prev=""
for a in "$@"; do
  case "$prev" in --settings) settings="$a";; --model) model="$a";; --resume) resume="$a";; esac
  prev="$a"
done
cat >/dev/null
base="$ANTHROPIC_BASE_URL"; key="${ANTHROPIC_AUTH_TOKEN:-$ANTHROPIC_API_KEY}"; small=""
if [ -n "$settings" ]; then
  b=$(sed -n 's/.*"ANTHROPIC_BASE_URL":"\([^"]*\)".*/\1/p' "$settings"); [ -n "$b" ] && base="$b"
  k=$(sed -n 's/.*"ANTHROPIC_AUTH_TOKEN":"\([^"]*\)".*/\1/p' "$settings"); [ -n "$k" ] && key="$k"
  small=$(sed -n 's/.*"ANTHROPIC_SMALL_FAST_MODEL":"\([^"]*\)".*/\1/p' "$settings")
fi
sess="${resume:-cl-sess-$$}"
printf 'base=%s key=%s model=%s small=%s resume=%s\n' "$base" "$key" "$model" "$small" "$resume" >> "$FAKE_CLAUDE_LOG"
printf '{"type":"system","subtype":"init","session_id":"%s"}\n' "$sess"
printf '{"type":"result","subtype":"success","is_error":false,"result":"done via %s","session_id":"%s"}\n' "$base" "$sess"
`

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
func (s *memSecrets) Set(id, v string) error { s.mu.Lock(); s.m[id] = v; s.mu.Unlock(); return nil }
func (s *memSecrets) Delete(id string) error { s.mu.Lock(); delete(s.m, id); s.mu.Unlock(); return nil }

// appDevice is one of Glenn's Apps (the Mac's, the Windows PC's): its own
// local backend, with its own device keys.
type appDevice struct {
	t   *testing.T
	srv *httptest.Server
	hub string
}

func newAppDevice(t *testing.T, hubURL, token string) *appDevice {
	t.Helper()
	b := &console.Backend{Store: console.NewStore(filepath.Join(t.TempDir(), "hubs.json")), Secrets: &memSecrets{m: map[string]string{}}, Timeout: 20 * time.Second}
	d := &appDevice{t: t, srv: httptest.NewServer(b.Handler())}
	t.Cleanup(d.srv.Close)
	code, m, raw := d.do("POST", "/app/hubs", map[string]string{"url": hubURL, "token": token, "name": "Team"})
	if code/100 != 2 {
		t.Fatalf("add hub: %d %s", code, raw)
	}
	d.hub, _ = m["id"].(string)
	return d
}

func (d *appDevice) do(method, path string, body any) (int, map[string]any, []byte) {
	d.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, d.srv.URL+path, rd)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		d.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m, raw
}

// providers answers the App's list of the member's providers.
func (d *appDevice) providers() []map[string]any {
	d.t.Helper()
	code, m, raw := d.do("GET", "/app/hubs/"+d.hub+"/providers", nil)
	if code != 200 {
		d.t.Fatalf("providers: %d %s", code, raw)
	}
	var out []map[string]any
	for _, p := range m["providers"].([]any) {
		out = append(out, p.(map[string]any))
	}
	return out
}

// i115's acceptance: a MiniMax provider added on the Mac, opened on the
// Windows PC once the Mac sealed it again for it, and a Claude Code +
// MiniMax agent started on WSL that answers with it. The key never reaches
// the hub in the clear; a new key is pushed to the agent.
func TestModelProviderFromTheMacOntoAnAgentOnWSL(t *testing.T) {
	testutil.NeedsPOSIXShell(t) // fakeClaude
	home := isolatedHome(t)
	const key1, key2 = "mm-test-key-0123456789abcd", "mm-test-key-second-wxyz"
	var listed atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("x-api-key")
		if r.URL.Path != "/anthropic/v1/models" || (got != key1 && got != key2) {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		listed.Add(1)
		w.Write([]byte(`{"data":[{"id":"MiniMax-M2","display_name":"MiniMax M2"},{"id":"MiniMax-M2-lite"}]}`))
	}))
	defer vendor.Close()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(fakeClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	claudeLog := filepath.Join(t.TempDir(), "claude.log")
	t.Setenv("FAKE_CLAUDE_LOG", claudeLog)
	if err := os.MkdirAll(filepath.Join(home, "proj"), 0o700); err != nil {
		t.Fatal(err)
	}

	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	inv, err := admin.InviteMember(c, api.InviteRequest{Name: "glenn", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	macPair, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	// The Mac's App reaches the hub through a proxy that can lie about a
	// provider's address, as a hub (or its administrator token) could.
	var lie atomic.Bool
	var evilHits atomic.Int32
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { evilHits.Add(1); http.Error(w, "no", 401) }))
	defer evil.Close()
	hubURL, _ := url.Parse(h.url)
	rp := httputil.NewSingleHostReverseProxy(hubURL)
	rp.ModifyResponse = func(resp *http.Response) error {
		if !lie.Load() || resp.Request.URL.Path != "/v1/console/providers" {
			return nil
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var l api.ModelProviderList
		_ = json.Unmarshal(b, &l)
		for i := range l.Providers {
			l.Providers[i].AnthropicBase = evil.URL + "/anthropic"
		}
		b, _ = json.Marshal(l)
		resp.Body = io.NopCloser(bytes.NewReader(b))
		resp.ContentLength = int64(len(b))
		resp.Header.Set("Content-Length", strconv.Itoa(len(b)))
		return nil
	}
	lying := httptest.NewServer(rp)
	defer lying.Close()
	mac := newAppDevice(t, lying.URL, macPair.Token)

	// Without end-to-end encryption the hub keeps the key (i119; its own
	// test follows the rest of that way): here, put away again.
	body := map[string]any{"name": "MiniMax", "vendor": "minimax", "anthropic_base": vendor.URL + "/anthropic", "api_key": key1}
	if code, m, raw := mac.do("POST", "/app/hubs/"+mac.hub+"/providers", body); code != 201 || m["custody"] != "hub" || strings.Contains(string(raw), key1) {
		t.Fatalf("without e2e: %d %s", code, raw)
	} else if code, _, raw := mac.do("DELETE", "/app/hubs/"+mac.hub+"/providers/"+m["id"].(string), nil); code != 200 {
		t.Fatalf("delete: %d %s", code, raw)
	}
	if _, err := admin.SetHubSettings(c, api.HubSettings{E2E: true}); err != nil {
		t.Fatal(err)
	}
	code, m, raw := mac.do("POST", "/app/hubs/"+mac.hub+"/providers", body)
	if code != 201 || strings.Contains(string(raw), key1) || m["key_hint"] != key1[len(key1)-4:] || m["openai_base"] != "https://api.minimax.io/v1" {
		t.Fatalf("add on the Mac: %d %s", code, raw)
	}
	pid := m["id"].(string)
	// The hub keeps an envelope it cannot open.
	list, err := h.client(macPair.Token).ModelProviders(c)
	if err != nil || len(list.Providers) != 1 || strings.Contains(list.Providers[0].KeySealed, key1) || !e2e.IsSealed(list.Providers[0].KeySealed) {
		t.Fatalf("hub copy: %v %+v", err, list)
	}
	// Another member sees none of it; and cannot use it for an agent.
	other, _ := admin.InviteMember(c, api.InviteRequest{Name: "ana"})
	anaPair, _ := h.client("").Pair(c, other.Link[strings.Index(other.Link, "/c/")+3:], "ana-mac")
	if l, err := h.client(anaPair.Token).ModelProviders(c); err != nil || len(l.Providers) != 0 {
		t.Fatalf("another member's list: %v %+v", err, l)
	}
	if err := h.client(anaPair.Token).DeleteModelProvider(c, pid); err == nil {
		t.Fatal("another member deleted it")
	}

	// 测试: the vendor's models, read with the key; kept with the provider.
	code, m, raw = mac.do("POST", "/app/hubs/"+mac.hub+"/providers/"+pid+"/test", nil)
	if code != 200 || !strings.Contains(string(raw), "MiniMax-M2") || strings.Contains(string(raw), key1) {
		t.Fatalf("test: %d %s", code, raw)
	}
	// A hub that says the provider is elsewhere: the App says so, and still
	// uses only the address sealed with the key.
	lie.Store(true)
	if p := mac.providers()[0]; p["settings_changed"] != true || p["anthropic_base"] != vendor.URL+"/anthropic" {
		t.Fatalf("a changed address was not said: %v", p)
	}
	if code, _, raw := mac.do("POST", "/app/hubs/"+mac.hub+"/providers/"+pid+"/test", nil); code != 200 || evilHits.Load() != 0 {
		t.Fatalf("the key went to the hub's address: %d %s, %d", code, raw, evilHits.Load())
	}
	lie.Store(false)
	// Changing the address alone, without sealing it with the key: refused.
	evilBase := evil.URL + "/anthropic"
	if _, err := h.client(macPair.Token).UpdateModelProvider(c, pid, api.ModelProviderRequest{AnthropicBase: &evilBase}); err == nil || !strings.Contains(err.Error(), "sealed with the key") {
		t.Fatalf("an address changed without the key: %v", err)
	}
	// The hub's administrator token is no member: it has no providers.
	if _, err := admin.ModelProviders(c); err == nil {
		t.Fatal("the administrator token lists a member's providers")
	}
	// A wrong key says only where and what.
	code, m, raw = mac.do("POST", "/app/hubs/"+mac.hub+"/providers/test", map[string]any{"vendor": "custom", "anthropic_base": vendor.URL + "/anthropic", "api_key": "wrong-key-123456789"})
	if code != 502 || strings.Contains(string(raw), "wrong-key") || !strings.Contains(string(raw), "HTTP 401") {
		t.Fatalf("test with a wrong key: %d %s", code, raw)
	}

	// Glenn's Windows PC joins as a second device: it cannot open the key
	// until the Mac seals it again for both (rewrap), at its next look.
	link, err := h.client(macPair.Token).CreatePairing(c)
	if err != nil {
		t.Fatal(err)
	}
	winPair, err := h.client("").Pair(c, link.Link[strings.Index(link.Link, "/c/")+3:], "glenn-win")
	if err != nil {
		t.Fatal(err)
	}
	win := newAppDevice(t, h.url, winPair.Token)
	if p := win.providers()[0]; p["key_readable"] != false {
		t.Fatalf("the PC opened a key not sealed for it: %v", p)
	}
	if p := mac.providers()[0]; p["rewrapped"] != true || p["key_version"] != float64(1) {
		t.Fatalf("the Mac did not seal it again: %v", p)
	}
	if p := win.providers()[0]; p["key_readable"] != true || p["key_hint"] != key1[len(key1)-4:] {
		t.Fatalf("the PC still cannot open it: %v", p)
	}
	// (and the other way round: nothing more to seal)
	if p := win.providers()[0]; p["rewrapped"] == true {
		t.Fatalf("sealed again for nothing: %v", p)
	}

	// WSL joins; its runner serves its machine node (and the agent to come).
	// (Glenn's own link: the computer and the agents started on it are his)
	enr, err := h.client(macPair.Token).CreateEnrollment(c, api.CreateEnrollmentRequest{Kind: "link", MaxUses: 1, TTLSeconds: 3600}, false)
	if err != nil {
		t.Fatal(err)
	}
	box := newCLI(t, h, filepath.Join(home, ".myco", "config.yaml"))
	delete(box.env, "MYCO_HUB")
	delete(box.env, "MYCO_ADMIN_TOKEN")
	box.env["PATH"] = os.Getenv("PATH")
	box.env["FAKE_CLAUDE_LOG"] = claudeLog
	if out := box.must(c, "machine", "join", "--link", enr.JoinURL, "--machine", "wsl", "--no-service"); !strings.Contains(out, `"machine_node": "wsl-machine"`) {
		t.Fatalf("join: %s", out)
	}
	rctx, stop := context.WithCancel(c)
	done := make(chan struct{})
	go func() { defer close(done); box.run(rctx, "runner", "start", "--all", "--log-level", "error") }()
	t.Cleanup(func() { stop(); <-done })
	waitUntil(t, 30*time.Second, func() bool {
		node, err := admin.GetAgent(c, "wsl-machine")
		keys, kerr := admin.KeysOf(c, "wsl-machine")
		return err == nil && kerr == nil && node.Runtime != nil && slices.Contains(node.Runtime.Features, "sealed_spawn") && len(keys.Keys["wsl-machine"]) == 1
	})
	machines, err := admin.Machines(c)
	if err != nil {
		t.Fatal(err)
	}
	mid := ""
	for _, x := range machines {
		if x.Node == "wsl-machine" {
			mid = x.ID
		}
	}
	if mid == "" {
		t.Fatalf("no machine for the node: %+v", machines)
	}

	// An agent cannot plant a provider on a new agent: neither one it sealed
	// itself, nor one sealed by a person but sent by the agent.
	lead := h.client(h.register(api.RegisterRequest{Name: "planter"}))
	leadID := mustIdentity(t)
	lead.Sealer = &client.Sealer{Me: "planter", ID: leadID}
	if err := lead.RegisterKeys(c); err != nil {
		t.Fatal(err)
	}
	nodeKeys, err := admin.KeysOf(c, "wsl-machine")
	if err != nil {
		t.Fatal(err)
	}
	planted, _ := json.Marshal(map[string]string{"base_url": evil.URL + "/anthropic", "api_key": "x"})
	glennCLI := h.client("")
	cliLink, _ := h.client(macPair.Token).CreatePairing(c)
	cliPair, err := glennCLI.Pair(c, cliLink.Link[strings.Index(cliLink.Link, "/c/")+3:], "glenn-cli")
	if err != nil {
		t.Fatal(err)
	}
	glennDev := h.client(cliPair.Token)
	glennDev.Sealer = &client.Sealer{Me: "@console:glenn", ID: mustIdentity(t), Device: true}
	if err := glennDev.RegisterKeys(c); err != nil {
		t.Fatal(err)
	}
	for who, sealer := range map[string]*client.Sealer{"its own": lead.Sealer, "Glenn's": glennDev.Sealer} {
		body, err := e2e.Seal(sealer.ID, e2e.Header{From: sealer.Me, To: "wsl-machine", Type: e2e.ProviderSettingsType, ReplyTo: "pe-" + sealer.Me}, planted, []string{nodeKeys.Keys["wsl-machine"][0].EncKey})
		if err != nil {
			t.Fatal(err)
		}
		req, _ := json.Marshal(api.SpawnRequest{CLI: "claude", Dir: filepath.Join(home, "proj"), Name: "planted", ProviderEncrypted: &api.EncryptedProvider{ID: "pe-" + sealer.Me, From: sealer.Me, Body: body}})
		sent, err := lead.Send(c, api.SendRequest{To: "wsl-machine", Type: "task", Body: string(req)}, false)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := lead.WaitReply(c, sent.Message.ID, 30*time.Second)
		var res api.SpawnResult
		if err != nil || json.Unmarshal([]byte(reply.Body), &res) != nil || !res.Denied || res.Agent != "" {
			t.Fatalf("a provider %s sent by an agent was taken: %v %+v", who, err, reply)
		}
	}
	if _, err := admin.GetAgent(c, "wsl-planted"); err == nil || evilHits.Load() != 0 {
		t.Fatal("an agent planted a provider")
	}

	// From the PC: a Claude Code + MiniMax agent on WSL.
	code, m, raw = win.do("POST", "/app/hubs/"+win.hub+"/machines/"+mid+"/spawn", map[string]any{
		"cli": "claude", "dir": filepath.Join(home, "proj"), "name": "mm", "model": "MiniMax-M2", "provider": pid})
	if code != 200 || m["agent"] != "wsl-mm" || strings.Contains(string(raw), key1) {
		t.Fatalf("spawn: %d %s", code, raw)
	}
	first := logLines(t, claudeLog)
	want := "base=" + vendor.URL + "/anthropic key=" + key1 + " model=MiniMax-M2"
	if len(first) != 1 || !strings.HasPrefix(first[0], want) {
		t.Fatalf("the first turn did not run with the provider: %q", first)
	}
	a, err := admin.GetAgent(c, "wsl-mm")
	if err != nil || a.RunSettings == nil || a.RunSettings.Provider == nil || *a.RunSettings.Provider != "custom" ||
		a.RunSettings.ProviderRef == nil || a.RunSettings.ProviderRef.ID != pid || a.RunSettings.ProviderRef.KeyVersion != 1 {
		t.Fatalf("the agent's provider: %v %+v", err, a.RunSettings)
	}
	envFile := filepath.Join(home, ".myco", "env", "wsl-mm.yaml.env")
	if st, err := os.Stat(envFile); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("the agent's env file: %v %v", err, st)
	}

	// A message runs on it, with the same provider.
	sender := h.client(h.register(api.RegisterRequest{Name: "lead"}))
	sent, err := sender.Send(c, api.SendRequest{To: "wsl-mm", Type: "task", Body: "say hi"}, false)
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 30*time.Second, func() bool {
		got, err := sender.GetMessage(c, sent.Message.ID)
		return err == nil && got.Status == "done"
	})
	runs := logLines(t, claudeLog)
	if len(runs) != 2 || !strings.HasPrefix(runs[1], want) || !strings.Contains(runs[1], "resume=cl-sess-") {
		t.Fatalf("the message's run: %q", runs)
	}
	if listed.Load() == 0 {
		t.Fatal("the vendor's models were never read")
	}

	// A new key, from the PC: the agent has the older one; pushed to it.
	code, m, raw = win.do("PATCH", "/app/hubs/"+win.hub+"/providers/"+pid, map[string]any{"api_key": key2})
	if code != 200 || m["key_version"] != float64(2) || strings.Contains(string(raw), key2) {
		t.Fatalf("new key: %d %s", code, raw)
	}
	code, m, raw = win.do("POST", "/app/hubs/"+win.hub+"/providers/"+pid+"/push", nil)
	if code != 200 || !strings.Contains(string(raw), `"pushed":["wsl-mm"]`) {
		t.Fatalf("push: %d %s", code, raw)
	}
	sent, err = sender.Send(c, api.SendRequest{To: "wsl-mm", Type: "task", Body: "again"}, false)
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 30*time.Second, func() bool {
		got, err := sender.GetMessage(c, sent.Message.ID)
		return err == nil && got.Status == "done"
	})
	runs = logLines(t, claudeLog)
	if len(runs) != 3 || !strings.Contains(runs[2], "key="+key2) {
		t.Fatalf("the run after the push: %q", runs)
	}

	// A machine whose myco cannot take a sealed provider: the agent is
	// started without it, and gets it as soon as it has joined, before
	// anything is sent to it. The node here is a current one, which reports
	// sealed_spawn again whenever it reports anything (a run ending is
	// enough): the hub hears its reports as an older node's, without it.
	nodeCfg, err := config.Load(config.ProfilePath(filepath.Join(home, ".myco", "config.yaml"), "wsl-machine"))
	if err != nil {
		t.Fatal(err)
	}
	older := func(r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/me/runtime" || r.Header.Get("Authorization") != "Bearer "+nodeCfg.Token {
			return
		}
		body, _ := io.ReadAll(r.Body)
		var rt map[string]any
		if json.Unmarshal(body, &rt) == nil {
			if fs, ok := rt["features"].([]any); ok {
				rt["features"] = slices.DeleteFunc(fs, func(f any) bool { return f == "sealed_spawn" })
				body, _ = json.Marshal(rt)
			}
		}
		r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(body)), int64(len(body))
	}
	h.rewrite.Store(&older)
	if err := h.client(nodeCfg.Token).ReportRuntime(c, api.Runtime{CLI: "machine", Features: []string{"usage", "sealed_spawn"}}); err != nil {
		t.Fatal(err)
	}
	if node, err := admin.GetAgent(c, "wsl-machine"); err != nil || node.Runtime == nil || slices.Contains(node.Runtime.Features, "sealed_spawn") {
		t.Fatalf("the node still reads as one that takes a sealed provider: %v %+v", err, node.Runtime)
	}
	before := len(logLines(t, claudeLog))
	code, m, raw = mac.do("POST", "/app/hubs/"+mac.hub+"/machines/"+mid+"/spawn", map[string]any{
		"cli": "claude", "dir": filepath.Join(home, "proj"), "name": "mm2", "model": "MiniMax-M2", "provider": pid})
	if code != 200 || m["agent"] != "wsl-mm2" {
		t.Fatalf("spawn on an older machine: %d %s", code, raw)
	}
	// its first turn (a new session) is the one line since
	waitUntil(t, 30*time.Second, func() bool { return len(logLines(t, claudeLog)) > before })
	if runs := logLines(t, claudeLog)[before:]; len(runs) != 1 || strings.Contains(runs[0], vendor.URL) || !strings.Contains(runs[0], "model=MiniMax-M2") || !strings.HasSuffix(runs[0], "resume=") {
		t.Fatalf("an older machine's first turn got a provider it cannot take: %q", runs)
	}
	a, err = admin.GetAgent(c, "wsl-mm2")
	if err != nil || a.RunSettings == nil || a.RunSettings.ProviderEncrypted == nil || a.RunSettings.ProviderRef == nil || a.RunSettings.ProviderRef.KeyVersion != 2 {
		t.Fatalf("the provider did not follow the agent: %v %+v", err, a.RunSettings)
	}
	sent, err = sender.Send(c, api.SendRequest{To: "wsl-mm2", Type: "task", Body: "hello"}, false)
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 30*time.Second, func() bool {
		got, err := sender.GetMessage(c, sent.Message.ID)
		return err == nil && got.Status == "done"
	})
	if runs := logLines(t, claudeLog); !strings.HasPrefix(runs[len(runs)-1], "base="+vendor.URL+"/anthropic key="+key2) {
		t.Fatalf("the agent on an older machine did not run with the provider: %q", runs)
	}

	// Deleted: the agent keeps what is on its computer.
	if code, _, raw := mac.do("DELETE", "/app/hubs/"+mac.hub+"/providers/"+pid, nil); code != 200 {
		t.Fatalf("delete: %d %s", code, raw)
	}
	if b, err := os.ReadFile(envFile); err != nil || !strings.Contains(string(b), "ANTHROPIC_BASE_URL") {
		t.Fatalf("the agent lost its provider: %v", err)
	}

	// The key never reached the hub's database in the clear.
	stop()
	<-done
	h.stop()
	db, _ := os.ReadFile(h.db)
	if bytes.Contains(db, []byte(key1)) || bytes.Contains(db, []byte(key2)) {
		t.Fatal("a key is in the hub's database")
	}
}

func logLines(t *testing.T, path string) []string {
	t.Helper()
	b, _ := os.ReadFile(path)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func waitUntil(t *testing.T, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// The hub keeps only envelopes it can check: signed by one of the member's
// devices, readable by those devices only, for this provider and version.
func TestTheHubTakesOnlyAKeySealedForTheMembersOwnDevices(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	if _, err := admin.SetHubSettings(c, api.HubSettings{E2E: true}); err != nil {
		t.Fatal(err)
	}
	inv, _ := admin.InviteMember(c, api.InviteRequest{Name: "glenn"})
	pair, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	me, _ := h.client(pair.Token).Whoami(c)
	member := me.Member.ID
	glenn := h.client(pair.Token)
	glenn.Sealer = &client.Sealer{Me: "@console:glenn", ID: mustIdentity(t), Device: true}
	if err := glenn.RegisterKeys(c); err != nil {
		t.Fatal(err)
	}
	stranger := mustIdentity(t)
	name, vendor, two := "MiniMax", "minimax", 2
	base := "https://api.minimax.io/anthropic"
	create := func(body string, version int) error {
		_, err := glenn.CreateModelProvider(c, api.ModelProviderRequest{ID: "prv_abcdefgh12", Name: &name, Vendor: &vendor, AnthropicBase: &base, KeySealed: &body, KeyVersion: &version})
		return err
	}
	keyRefused := func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "key_sealed must be signed by one of your devices")
	}
	payload := []byte(`{"api_key":"k"}`)
	// Readable by someone else too: refused.
	foreign, _ := e2e.Seal(glenn.Sealer.ID, e2e.ProviderKeyHeader(member, "prv_abcdefgh12", 1), payload, []string{glenn.Sealer.ID.Public().Enc, stranger.Public().Enc})
	if err := create(foreign, 1); !keyRefused(err) {
		t.Fatal("a key a stranger can read was kept")
	}
	// Signed by someone who is not one of the member's devices: refused.
	unsigned, _ := e2e.Seal(stranger, e2e.ProviderKeyHeader(member, "prv_abcdefgh12", 1), payload, []string{glenn.Sealer.ID.Public().Enc})
	if err := create(unsigned, 1); !keyRefused(err) {
		t.Fatal("a key signed by a stranger was kept")
	}
	// For another provider or version: refused.
	other, _ := e2e.Seal(glenn.Sealer.ID, e2e.ProviderKeyHeader(member, "prv_other00000", 1), payload, []string{glenn.Sealer.ID.Public().Enc})
	if err := create(other, 1); !keyRefused(err) {
		t.Fatal("another provider's key was kept")
	}
	good, _ := glenn.SealProviderKey(c, member, "prv_abcdefgh12", 1, payload)
	if err := create(good, 2); err == nil || !strings.Contains(err.Error(), "key_version 1") {
		t.Fatal("a new provider at version 2 was kept")
	}
	if err := create(good, 1); err != nil {
		t.Fatal(err)
	}
	// A key version may only go up by one (or stay, sealed again).
	three := 3
	newer, _ := glenn.SealProviderKey(c, member, "prv_abcdefgh12", 3, payload)
	if _, err := glenn.UpdateModelProvider(c, "prv_abcdefgh12", api.ModelProviderRequest{KeySealed: &newer, KeyVersion: &three}); err == nil {
		t.Fatal("a key version jumped")
	}
	next, _ := glenn.SealProviderKey(c, member, "prv_abcdefgh12", 2, payload)
	if p, err := glenn.UpdateModelProvider(c, "prv_abcdefgh12", api.ModelProviderRequest{KeySealed: &next, KeyVersion: &two}); err != nil || p.KeyVersion != 2 {
		t.Fatalf("a new key: %v %+v", err, p)
	}
	if raw, err := glenn.OpenProviderKey(c, member, mustProvider(t, glenn)); err != nil || string(raw) != string(payload) {
		t.Fatalf("open: %v %s", err, raw)
	}
	// Encryption turned off: kept providers can be read and deleted, not
	// added to.
	if _, err := admin.SetHubSettings(c, api.HubSettings{E2E: false}); err != nil {
		t.Fatal(err)
	}
	if err := create(good, 1); err == nil || !strings.Contains(err.Error(), "encryption") {
		t.Fatalf("added without encryption: %v", err)
	}
	if err := glenn.DeleteModelProvider(c, "prv_abcdefgh12"); err != nil {
		t.Fatal(err)
	}
}

func mustIdentity(t *testing.T) *e2e.Identity {
	t.Helper()
	id, err := e2e.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustProvider(t *testing.T, c *client.Client) api.ModelProvider {
	t.Helper()
	l, err := c.ModelProviders(context.Background())
	if err != nil || len(l.Providers) != 1 {
		t.Fatalf("providers: %v %+v", err, l)
	}
	return l.Providers[0]
}
