package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/harness/builtin"
)

func startupEnv(t *testing.T, ctx context.Context) (*env, *slog.Logger) {
	t.Helper()
	return &env{defaultPath: filepath.Join(t.TempDir(), "config.yaml"), in: &cmdkit.Invocation{Ctx: ctx, Getenv: func(string) string { return "" }}}, slog.New(slog.NewTextHandler(io.Discard, nil))
}

func startupProfile(t *testing.T, e *env, name, hub string) (string, *config.Config) {
	t.Helper()
	path := config.ProfilePath(e.defaultPath, name)
	c := &config.Config{Hub: hub, Token: "test-token", Agent: name, Harness: "exec", Runner: config.Runner{Harness: "exec", Workdir: t.TempDir()}}
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	return path, c
}

// No real CLI is launched. A successful first inbox poll proves that binding
// finished and the worker is serving its Hub rather than just being queued.
func startupHub(poll func(http.ResponseWriter, *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/me", "/v1/me/binding":
			_ = json.NewEncoder(w).Encode(api.Agent{Name: "healthy", AllowFrom: []string{"owner"}, Binding: api.Binding{Harness: "exec"}})
		case "/v1/inbox/pull":
			poll(w, r)
		default:
			_, _ = io.WriteString(w, "{}")
		}
	}))
}

func TestRunnerAllStartsHealthyHubWhileAnotherHubIsBlocked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e, log := startupEnv(t, ctx)
	var active, maxActive, meCalls atomic.Int32
	blocked := make(chan struct{}, 16)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me" {
			meCalls.Add(1)
			n := active.Add(1)
			defer active.Add(-1)
			for old := maxActive.Load(); n > old; old = maxActive.Load() {
				if maxActive.CompareAndSwap(old, n) {
					break
				}
			}
		}
		_, _ = io.Copy(io.Discard, r.Body)
		blocked <- struct{}{}
		<-r.Context().Done()
	}))
	defer func() { cancel(); slow.Close() }()
	polled := make(chan struct{})
	var once sync.Once
	healthy := startupHub(func(w http.ResponseWriter, r *http.Request) { once.Do(func() { close(polled) }); <-r.Context().Done() })
	defer func() { cancel(); healthy.Close() }()
	for i := range 8 {
		startupProfile(t, e, fmt.Sprintf("a-slow-%d", i), slow.URL)
	}
	startupProfile(t, e, "z-healthy", healthy.URL)
	if err := config.SaveMachine(e.defaultPath, &config.Machine{MachineHub: config.MachineHub{Hub: slow.URL, Machine: "test", EnrollToken: "test-enroll", NamePrefix: "test"}}); err != nil {
		t.Fatal(err)
	}
	sh := runnerShared{registry: builtin.Registry(), rescan: make(chan struct{}, 1)}
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- e.runAll(sh, log) }()
	select {
	case <-polled:
		t.Logf("healthy Hub served in %s while the other Hub remained blocked", time.Since(start))
	case <-time.After(3 * time.Second):
		t.Fatal("slow Hub blocked the healthy worker")
	}
	for range 5 {
		select {
		case <-blocked:
		case <-time.After(time.Second):
			t.Fatal("expected four pending profiles and machine enrollment")
		}
	}
	// A scan while handshakes are pending must not duplicate those workers.
	sh.rescan <- struct{}{}
	if n := maxActive.Load(); n > 4 {
		t.Fatalf("unbounded startup concurrency: %d", n)
	}
	if n := meCalls.Load(); n != 4 {
		t.Fatalf("expected four pending handshakes, got %d", n)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not wait for/cancel pending startup")
	}
	for _, name := range []string{"a-slow-0", "z-healthy"} {
		c, err := config.Load(config.ProfilePath(e.defaultPath, name))
		if err != nil || c.Token != "test-token" || c.Replaced != "" {
			t.Fatal("cancellation changed saved credentials", err)
		}
	}
}

func TestRunnerStartupDeadlineCoversMeAndBind(t *testing.T) {
	for _, endpoint := range []string{"/v1/me", "/v1/me/binding"} {
		t.Run(endpoint, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e, log := startupEnv(t, ctx)
			var reached atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == endpoint {
					reached.Store(true)
					_, _ = io.Copy(io.Discard, r.Body)
					<-r.Context().Done()
					return
				}
				_ = json.NewEncoder(w).Encode(api.Agent{Name: "healthy", AllowFrom: []string{"owner"}, Binding: api.Binding{Harness: "exec"}})
			}))
			defer func() { cancel(); srv.Close() }()
			path, c := startupProfile(t, e, "healthy", srv.URL)
			startup, stop := context.WithTimeout(ctx, 300*time.Millisecond)
			defer stop()
			start := time.Now()
			_, err := e.buildRunnerContext(startup, path, c, runnerShared{registry: builtin.Registry()}, runnerOverrides{}, log)
			if err == nil {
				t.Fatal("blocked handshake succeeded")
			}
			if !reached.Load() {
				t.Fatal("target endpoint was not exercised")
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("startup deadline did not bound the request")
			}
			if ctx.Err() != nil {
				t.Fatal("startup deadline cancelled service lifetime")
			}
		})
	}
}

func TestRunnerKeepsServingAfterStartupContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e, log := startupEnv(t, ctx)
	polled := make(chan struct{})
	var once sync.Once
	srv := startupHub(func(w http.ResponseWriter, r *http.Request) { once.Do(func() { close(polled) }); <-r.Context().Done() })
	defer func() { cancel(); srv.Close() }()
	path, c := startupProfile(t, e, "healthy", srv.URL)
	startup, stop := context.WithCancel(ctx)
	r, err := e.buildRunnerContext(startup, path, c, runnerShared{registry: builtin.Registry()}, runnerOverrides{}, log)
	stop()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	select {
	case <-polled:
	case <-time.After(3 * time.Second):
		t.Fatal("worker inherited ended startup context")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop")
	}
}
