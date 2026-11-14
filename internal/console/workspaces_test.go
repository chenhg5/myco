package console_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/console"
	"github.com/chenhg5/myco/internal/hubserver"
)

// The app creates a workspace on its hub's server and adds it, signed in as
// its owner; a hub without workspaces says so.
func TestAppCreatesWorkspaceOnItsServer(t *testing.T) {
	o := hubserver.Options{DBPath: filepath.Join(t.TempDir(), "hub.db"), AdminToken: adminToken,
		Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h, err := hubserver.Build(o)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	handler, closeWS, err := hubserver.Serve(ctx, o, h)
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(handler)
	t.Cleanup(func() { hs.Close(); closeWS(); cancel(); h.Close() })
	admin, _ := client.New(hs.URL, adminToken)
	link, err := admin.CreatePairing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, hubURL: hs.URL, admin: admin, secrets: &memSecrets{m: map[string]string{}}, store: filepath.Join(t.TempDir(), "app", "hubs.json")}
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Timeout: 3 * time.Second}
	r.app = httptest.NewServer(b.Handler())
	t.Cleanup(r.app.Close)

	code, home, raw := r.do("POST", "/app/hubs", map[string]string{"link": link.Link})
	if code != 201 {
		t.Fatalf("add hub: %d %s", code, raw)
	}
	id := home["id"].(string)
	if code, m, raw := r.do("GET", "/app/hubs/"+id+"/workspaces", nil); code != 200 || m["supported"] != true || m["allowed"] != true {
		t.Fatalf("workspaces: %d %s", code, raw)
	}
	code, ws, raw := r.do("POST", "/app/hubs/"+id+"/workspaces", map[string]string{"name": "team-b", "display_name": "B 组"})
	if code != 201 || ws["url"] != hs.URL+"/w/team-b" || ws["name"] != "B 组" {
		t.Fatalf("create: %d %s", code, raw)
	}
	_, _, list := r.do("GET", "/app/hubs", nil)
	if !strings.Contains(string(list), hs.URL+"/w/team-b") {
		t.Fatalf("the new workspace should be in the app: %s", list)
	}
	// From inside the new workspace, the server is still reachable as its
	// owner (through the default workspace's entry).
	if code, m, raw := r.do("GET", "/app/hubs/"+ws["id"].(string)+"/workspaces", nil); code != 200 || m["allowed"] != true {
		t.Fatalf("workspaces from the new one: %d %s", code, raw)
	}
	if code, m, raw := r.do("GET", "/app/hubs/"+ws["id"].(string)+"/server-machines", nil); code != 200 || len(m["machines"].([]any)) != 0 {
		t.Fatalf("server machines: %d %s", code, raw)
	}
	if code, _, _ := r.do("POST", "/app/hubs/"+id+"/workspaces", map[string]string{"name": "team-b"}); code != 409 {
		t.Fatalf("a taken name: %d", code)
	}

	// A hub without workspaces (older): not supported, no error.
	old := newRig(t)
	ol, err := old.admin.CreatePairing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, oh, _ := old.do("POST", "/app/hubs", map[string]string{"link": ol.Link})
	if code, m, raw := old.do("GET", "/app/hubs/"+oh["id"].(string)+"/workspaces", nil); code != 200 || m["supported"] != false {
		t.Fatalf("old hub: %d %s", code, raw)
	}
}
