package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/infra/hubkey"
	"github.com/myco-ai/myco/internal/testutil"
)

// i119: a workspace with end-to-end encryption off keeps model providers
// too. The hub keeps the key, sealed with the workspace's own key, and
// hands it only to the runner or machine node it was set for, and to its
// owner: added on the Mac, tested, an agent started with it on WSL from
// the PC, a message run on it, a new key pushed; nobody else gets the key,
// the database never holds it in the clear, and it survives turning
// encryption on and off again.
func TestAModelProviderTheHubKeepsWithEncryptionOff(t *testing.T) {
	testutil.NeedsPOSIXShell(t) // fakeClaude
	home := isolatedHome(t)
	const key1, key2 = "mm-hub-key-0123456789abcd", "mm-hub-key-second-wxyz"
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
	dataDir := t.TempDir()
	h := startHub(t, filepath.Join(dataDir, "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	inv, err := admin.InviteMember(c, api.InviteRequest{Name: "glenn", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	macPair, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	mac := newAppDevice(t, h.url, macPair.Token)
	keyFile := filepath.Join(dataDir, hubkey.FileName)

	// Added on the Mac, encryption off: the hub keeps it, and says so.
	body := map[string]any{"name": "MiniMax", "vendor": "custom", "anthropic_base": vendor.URL + "/anthropic", "api_key": key1}
	code, m, raw := mac.do("POST", "/app/hubs/"+mac.hub+"/providers", body)
	if code != 201 || m["custody"] != "hub" || strings.Contains(string(raw), key1) || m["key_hint"] != key1[len(key1)-4:] {
		t.Fatalf("add: %d %s", code, raw)
	}
	pid := m["id"].(string)
	if st, err := os.Stat(keyFile); err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
		t.Fatalf("the workspace's key: %v %v", err, st)
	}
	list, err := h.client(macPair.Token).ModelProviders(c)
	if err != nil || list.E2E || len(list.Providers) != 1 || list.Providers[0].KeySealed != "" || list.Providers[0].Custody != "hub" {
		t.Fatalf("the hub's copy: %v %+v", err, list)
	}
	if strings.Contains(string(mustJSON(t, list)), key1) {
		t.Fatal("the list carries the key")
	}
	// 测试: from the Mac, with the key its owner reads.
	code, _, raw = mac.do("POST", "/app/hubs/"+mac.hub+"/providers/"+pid+"/test", nil)
	if code != 200 || !strings.Contains(string(raw), "MiniMax-M2") || strings.Contains(string(raw), key1) || listed.Load() == 0 {
		t.Fatalf("test: %d %s", code, raw)
	}

	// Nobody else reads the key: another member, the administrator token.
	other, _ := admin.InviteMember(c, api.InviteRequest{Name: "ana"})
	anaPair, _ := h.client("").Pair(c, other.Link[strings.Index(other.Link, "/c/")+3:], "ana-mac")
	if _, err := h.client(anaPair.Token).ModelProviderKey(c, pid); err == nil {
		t.Fatal("another member read the key")
	}
	if _, err := admin.ModelProviderKey(c, pid); err == nil {
		t.Fatal("the administrator token read the key")
	}

	// WSL joins; its runner serves its machine node.
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
		return err == nil && node.Runtime != nil && slices.Contains(node.Runtime.Features, "hub_provider")
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

	// From the PC: a Claude Code + MiniMax agent on WSL. Its first turn
	// already runs on the provider the node fetched with the spawn's grant.
	link, err := h.client(macPair.Token).CreatePairing(c)
	if err != nil {
		t.Fatal(err)
	}
	winPair, err := h.client("").Pair(c, link.Link[strings.Index(link.Link, "/c/")+3:], "glenn-win")
	if err != nil {
		t.Fatal(err)
	}
	win := newAppDevice(t, h.url, winPair.Token)
	if p := win.providers()[0]; p["key_readable"] != true || p["key_hint"] != key1[len(key1)-4:] {
		t.Fatalf("the PC's view: %v", p)
	}
	code, m, raw = win.do("POST", "/app/hubs/"+win.hub+"/machines/"+mid+"/spawn", map[string]any{
		"cli": "claude", "dir": filepath.Join(home, "proj"), "name": "mm", "model": "MiniMax-M2", "provider": pid})
	if code != 200 || m["agent"] != "wsl-mm" || strings.Contains(string(raw), key1) {
		t.Fatalf("spawn: %d %s", code, raw)
	}
	want := "base=" + vendor.URL + "/anthropic key=" + key1 + " model=MiniMax-M2"
	if first := logLines(t, claudeLog); len(first) != 1 || !strings.HasPrefix(first[0], want) {
		t.Fatalf("the first turn did not run with the provider: %q", first)
	}
	a, err := admin.GetAgent(c, "wsl-mm")
	if err != nil || a.RunSettings == nil || a.RunSettings.ProviderRef == nil || a.RunSettings.ProviderRef.ID != pid || a.RunSettings.ProviderHub != nil {
		t.Fatalf("the agent's provider: %v %+v", err, a.RunSettings)
	}
	envFile := filepath.Join(home, ".myco", "env", "wsl-mm.yaml.env")
	if st, err := os.Stat(envFile); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("the agent's env file: %v %v", err, st)
	}
	// The grant was for that request alone: gone with it.
	nodeCfg := nodeConfig(t, home)
	if _, err := h.client(nodeCfg.Token).ProviderMaterial(c, "", "pgr_anything"); err == nil {
		t.Fatal("a made-up grant was taken")
	}

	// A message runs on it.
	sender := h.client(h.register(api.RegisterRequest{Name: "lead"}))
	runTask := func(body string) {
		t.Helper()
		sent, err := sender.Send(c, api.SendRequest{To: "wsl-mm", Type: "task", Body: body}, false)
		if err != nil {
			t.Fatal(err)
		}
		waitUntil(t, 30*time.Second, func() bool {
			got, err := sender.GetMessage(c, sent.Message.ID)
			return err == nil && got.Status == "done"
		})
	}
	runTask("say hi")
	if runs := logLines(t, claudeLog); len(runs) != 2 || !strings.HasPrefix(runs[1], want) {
		t.Fatalf("the message's run: %q", runs)
	}

	// A new key from the PC, pushed: the runner fetches it from the hub.
	code, m, raw = win.do("PATCH", "/app/hubs/"+win.hub+"/providers/"+pid, map[string]any{"api_key": key2})
	if code != 200 || m["key_version"] != float64(2) || strings.Contains(string(raw), key2) || m["key_hint"] != key2[len(key2)-4:] {
		t.Fatalf("new key: %d %s", code, raw)
	}
	code, _, raw = win.do("POST", "/app/hubs/"+win.hub+"/providers/"+pid+"/push", nil)
	if code != 200 || !strings.Contains(string(raw), `"pushed":["wsl-mm"]`) {
		t.Fatalf("push: %d %s", code, raw)
	}
	a, _ = admin.GetAgent(c, "wsl-mm")
	if a.RunSettings.ProviderHub == nil || strings.Contains(string(mustJSON(t, a)), key2) {
		t.Fatalf("the agent's settings: %+v", a.RunSettings)
	}
	held := a.RunSettings.ProviderHub.ID
	// Only that agent's runner gets it: not another agent with its id.
	if _, err := sender.ProviderMaterial(c, held, ""); err == nil {
		t.Fatal("another agent fetched the agent's provider")
	}
	runTask("again")
	if runs := logLines(t, claudeLog); len(runs) != 3 || !strings.Contains(runs[2], "key="+key2) {
		t.Fatalf("the run after the push: %q", runs)
	}
	if a, _ = admin.GetAgent(c, "wsl-mm"); a.RunSettings.ProviderHub != nil || a.RunSettings.ProviderRef.KeyVersion != 2 {
		t.Fatalf("not acknowledged: %+v", a.RunSettings)
	}

	// A computer may start many agents with it in a minute: each spawn's
	// grant is its own limit (L2). Refusals are held to a few a minute.
	for i := 0; i < 7; i++ {
		code, _, raw := win.do("POST", "/app/hubs/"+win.hub+"/machines/"+mid+"/spawn", map[string]any{
			"cli": "claude", "dir": filepath.Join(home, "proj"), "name": "burst" + string(rune('a'+i)), "provider": pid})
		if code != 200 {
			t.Fatalf("spawn %d in a minute: %d %s", i+1, code, raw)
		}
	}
	nodeClient := h.client(nodeCfg.Token)
	refused := 0
	for i := 0; i < 8; i++ {
		if _, err := nodeClient.ProviderMaterial(c, "", "pgr_made_up"); err != nil && strings.Contains(err.Error(), "too many") {
			refused++
		}
	}
	if refused == 0 {
		t.Fatal("made-up grants were not held to a few a minute")
	}

	// Encryption on: the Mac seals the key for Glenn's devices, and the hub
	// drops its copy (the same key: version 2 still).
	if _, err := admin.SetHubSettings(c, api.HubSettings{E2E: true}); err != nil {
		t.Fatal(err)
	}
	if p := mac.providers()[0]; p["custody"] != "e2e" || p["key_version"] != float64(2) || p["key_hint"] != key2[len(key2)-4:] {
		t.Fatalf("not moved to the devices: %v", p)
	}
	if l, _ := h.client(macPair.Token).ModelProviders(c); l.Providers[0].Custody != "e2e" || l.Providers[0].KeySealed == "" {
		t.Fatalf("the hub's copy after: %+v", l.Providers[0])
	}
	if _, err := h.client(macPair.Token).ModelProviderKey(c, pid); err == nil {
		t.Fatal("the hub still reads the key")
	}
	// And off again: the hub keeps it once Glenn agrees (the App asks).
	if _, err := admin.SetHubSettings(c, api.HubSettings{E2E: false}); err != nil {
		t.Fatal(err)
	}
	if p := mac.providers()[0]; p["move_to_hub"] != true || p["key_readable"] != true {
		t.Fatalf("the App does not offer to hand it to the hub: %v", p)
	}
	code, m, raw = mac.do("POST", "/app/hubs/"+mac.hub+"/providers/"+pid+"/custody", map[string]any{"custody": "hub"})
	if code != 200 || m["custody"] != "hub" || m["key_version"] != float64(2) || strings.Contains(string(raw), key2) {
		t.Fatalf("to the hub: %d %s", code, raw)
	}
	if k, err := h.client(macPair.Token).ModelProviderKey(c, pid); err != nil || k.APIKey != key2 {
		t.Fatalf("the key after the moves: %v", err)
	}

	// The key never reached the database in the clear.
	stop()
	<-done
	h.stop()
	for _, f := range []string{"hub.db", "hub.db-wal"} {
		b, _ := os.ReadFile(filepath.Join(dataDir, f))
		if bytes.Contains(b, []byte(key1)) || bytes.Contains(b, []byte(key2)) {
			t.Fatalf("a key is in %s", f)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// nodeConfig is the machine node's profile on the joined computer.
func nodeConfig(t *testing.T, home string) *config.Config {
	t.Helper()
	cfg, err := config.Load(config.ProfilePath(filepath.Join(home, ".myco", "config.yaml"), "wsl-machine"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// The workspace's key file gone (a hub moved without it): the keys it kept
// are lost, the App is told so, and the hub does not make a new key while
// they are kept, which would hide that they cannot be opened.
func TestAKeyTheHubLostIsSaidAndNotMadeAgain(t *testing.T) {
	c := ctx(t)
	dataDir := t.TempDir()
	db := filepath.Join(dataDir, "hub.db")
	h := startHub(t, db, app.DefaultMessagePolicy())
	inv, err := h.admin().InviteMember(c, api.InviteRequest{Name: "glenn", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	name, vendor, base, key := "MiniMax", "custom", "https://api.example.test/anthropic", "mm-lost-key-0123456789"
	if _, err := h.client(pair.Token).CreateModelProvider(c, api.ModelProviderRequest{ID: "prv_lostkey00001", Name: &name, Vendor: &vendor, AnthropicBase: &base, APIKey: &key}); err != nil {
		t.Fatal(err)
	}
	h.stop()
	keyFile := filepath.Join(dataDir, hubkey.FileName)
	if err := os.Remove(keyFile); err != nil {
		t.Fatal(err)
	}
	h = startHub(t, db, app.DefaultMessagePolicy())
	mac := newAppDevice(t, h.url, pair.Token)
	if p := mac.providers()[0]; p["key_lost"] != true || p["key_readable"] != false {
		t.Fatalf("a lost key was not said: %v", p)
	}
	if _, err := h.client(pair.Token).ModelProviderKey(c, "prv_lostkey00001"); err == nil || !strings.Contains(err.Error(), "lost") {
		t.Fatalf("reading a lost key: %v", err)
	}
	other := "mm-other-key-0123456789"
	if _, err := h.client(pair.Token).CreateModelProvider(c, api.ModelProviderRequest{ID: "prv_lostkey00002", Name: &name, Vendor: &vendor, AnthropicBase: &base, APIKey: &other}); err == nil {
		t.Fatal("a new key was made while the lost one's values are kept")
	}
	if _, err := os.Stat(keyFile); err == nil {
		t.Fatal("the hub made a new key")
	}
	// Its owner entering the key again is the one act that makes a new
	// workspace key; then the rest works again.
	if _, err := h.client(pair.Token).UpdateModelProvider(c, "prv_lostkey00001", api.ModelProviderRequest{APIKey: &key}); err != nil {
		t.Fatalf("entered again: %v", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		t.Fatalf("no new key: %v", err)
	}
	if p := mac.providers()[0]; p["key_lost"] == true || p["key_readable"] != true {
		t.Fatalf("still lost: %v", p)
	}
	if _, err := h.client(pair.Token).CreateModelProvider(c, api.ModelProviderRequest{ID: "prv_lostkey00003", Name: &name, Vendor: &vendor, AnthropicBase: &base, APIKey: &other}); err != nil {
		t.Fatalf("a new one after: %v", err)
	}
}

// L3 and L1 (i119 review): with the hub keeping the key, a new address,
// vendor or small model is a new key version too (agents are offered it,
// as with a sealed key); a name alone is not. And a short key shows none
// of itself.
func TestAHubKeptProvidersNewAddressIsANewVersion(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	inv, err := h.admin().InviteMember(c, api.InviteRequest{Name: "glenn", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	glenn := h.client(pair.Token)
	name, vendor, base, key := "MiniMax", "custom", "https://api.example.test/anthropic", "mm-version-key-0123456789"
	p, err := glenn.CreateModelProvider(c, api.ModelProviderRequest{ID: "prv_version00001", Name: &name, Vendor: &vendor, AnthropicBase: &base, APIKey: &key})
	if err != nil || p.KeyVersion != 1 {
		t.Fatalf("add: %v %+v", err, p)
	}
	renamed := "MiniMax (work)"
	if p, err = glenn.UpdateModelProvider(c, p.ID, api.ModelProviderRequest{Name: &renamed}); err != nil || p.KeyVersion != 1 {
		t.Fatalf("a new name is no new version: %v %+v", err, p)
	}
	moved := "https://api2.example.test/anthropic"
	if p, err = glenn.UpdateModelProvider(c, p.ID, api.ModelProviderRequest{AnthropicBase: &moved}); err != nil || p.KeyVersion != 2 || p.AnthropicBase != moved {
		t.Fatalf("a new address is a new version: %v %+v", err, p)
	}
	small := "mini-1"
	if p, err = glenn.UpdateModelProvider(c, p.ID, api.ModelProviderRequest{SmallModel: &small}); err != nil || p.KeyVersion != 3 {
		t.Fatalf("a new small model is a new version: %v %+v", err, p)
	}
	if k, err := glenn.ModelProviderKey(c, p.ID); err != nil || k.APIKey != key || k.KeyVersion != 3 {
		t.Fatalf("the key after: %v %+v", err, k)
	}
	short := "abcd1234"
	q, err := glenn.CreateModelProvider(c, api.ModelProviderRequest{ID: "prv_version00002", Name: &name, Vendor: &vendor, AnthropicBase: &base, APIKey: &short})
	if err != nil || q.KeyHint != "" {
		t.Fatalf("a short key's hint: %v %+v", err, q)
	}
}
