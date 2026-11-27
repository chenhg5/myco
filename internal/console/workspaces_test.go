package console_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/console"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/testutil"
)

// The app creates a workspace on its hub's server and adds it, signed in as
// its owner; a hub without workspaces says so.
func TestAppCreatesWorkspaceOnItsServer(t *testing.T) {
	// This tests workspace routing and ownership, independently of the size
	// of the prepared production application catalogue.
	o := hubserver.Options{OfficialDir: testutil.OfficialDir(t), DBPath: filepath.Join(t.TempDir(), "hub.db"), AdminToken: adminToken,
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
	// These observations are server entry/return, not client completion or
	// response-body timing. Keep incomplete requests in the case-local snapshot.
	type phase struct {
		name                string
		entered, returned   time.Duration
		entryCtx, returnCtx string
		done                bool
	}
	var phasesMu sync.Mutex
	var phases []phase
	started := time.Now()
	contextState := func(ctx context.Context) string {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return "deadline"
		case errors.Is(ctx.Err(), context.Canceled):
			return "canceled"
		default:
			return "active"
		}
	}
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := ""
		// Inspect paths only to classify these fixture routes; never emit them.
		path := strings.TrimPrefix(r.URL.Path, "/w/team-b")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/server/workspaces":
			name = "create"
		case r.Method == http.MethodPost && path == "/v1/console/pair":
			name = "pair"
		case r.Method == http.MethodGet && path == "/v1/overview":
			name = "overview"
		}
		if name == "" {
			handler.ServeHTTP(w, r)
			return
		}
		phasesMu.Lock()
		index := len(phases)
		phases = append(phases, phase{name: name, entered: time.Since(started), entryCtx: contextState(r.Context())})
		phasesMu.Unlock()
		defer func() {
			phasesMu.Lock()
			defer phasesMu.Unlock()
			phases[index].returned = time.Since(started)
			phases[index].returnCtx = contextState(r.Context())
			phases[index].done = true
		}()
		handler.ServeHTTP(w, r)
	}))
	defer func() {
		phasesMu.Lock()
		snapshot := append([]phase(nil), phases...)
		phasesMu.Unlock()
		// Only the test goroutine logs; handlers may still be finishing after
		// this snapshot, so absence/unfinished entries are not causal proof.
		for i, p := range snapshot {
			t.Logf("workspace_server_phase seq=%d phase=%s event=enter at_ms=%d server_context=%s", i+1, p.name, p.entered.Milliseconds(), p.entryCtx)
			if p.done {
				t.Logf("workspace_server_phase seq=%d phase=%s event=return at_ms=%d elapsed_ms=%d server_context=%s", i+1, p.name, p.returned.Milliseconds(), (p.returned - p.entered).Milliseconds(), p.returnCtx)
			} else {
				t.Logf("workspace_server_phase seq=%d phase=%s event=incomplete snapshot_ms=%d server_context=unknown", i+1, p.name, time.Since(started).Milliseconds())
			}
		}
	}()
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
		t.Logf("workspace_create_failed at_ms=%d", time.Since(started).Milliseconds())
		// Separate, fresh diagnostic budgets do not extend or retry creation.
		// Neither a failed probe nor an expired probe proves absence.
		serverExists := func() string {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			list, err := admin.Workspaces(ctx)
			if err != nil || ctx.Err() != nil {
				return "unknown"
			}
			for _, item := range list {
				if item.Name == "team-b" {
					return "yes"
				}
			}
			return "no"
		}()
		locallyAdded := func() string {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			// Store.List has no context API. Bound the caller's wait and leave
			// a late read-only result buffered; the worker never calls t.Log.
			result := make(chan string, 1)
			go func() {
				list, err := b.Store.List()
				if err != nil {
					result <- "unknown"
					return
				}
				for _, item := range list {
					if item.URL == hs.URL+"/w/team-b" {
						result <- "yes"
						return
					}
				}
				result <- "no"
			}()
			select {
			case state := <-result:
				if ctx.Err() != nil {
					return "unknown"
				}
				return state
			case <-ctx.Done():
				return "unknown"
			}
		}()
		t.Logf("workspace_readonly_check server_exists=%s locally_added=%s", serverExists, locallyAdded)
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
