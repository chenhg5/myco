package cli

import (
	"context"
	"encoding/json"
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRunnerRestartAnnouncesAllLocalProfilesAndGoodbye(t *testing.T) {
	var mu sync.Mutex
	reports := []api.Runtime{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(api.Agent{Runtime: &api.Runtime{CLI: "exec", RunnerInstance: "old"}})
			return
		}
		var rt api.Runtime
		json.NewDecoder(r.Body).Decode(&rt)
		mu.Lock()
		reports = append(reports, rt)
		mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "config.yaml")
	for _, name := range []string{"idle-agent", "shell-node", "machine-node"} {
		if err := config.Save(config.ProfilePath(file, name), &config.Config{Agent: name, Hub: srv.URL, Token: "test-token", Harness: "exec"}); err != nil {
			t.Fatal(err)
		}
	}
	e := &env{defaultPath: file}
	e.announceRunnerRestart()
	if runnerRestartUntil(runnerRestartPath(file)).IsZero() {
		t.Fatal("restart marker missing")
	}
	mu.Lock()
	got := append([]api.Runtime{}, reports...)
	mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("announced%d profiles", len(got))
	}
	for _, rt := range got {
		if rt.Away != "updating" || rt.RestartingUntil.IsZero() || rt.RunnerInstance != "old" {
			t.Fatalf("bad announcement: %+v", rt)
		}
	}
	c, _ := client.New(srv.URL, "test-token")
	rep := &runtimeReporter{c: c, log: slog.New(slog.NewTextHandler(io.Discard, nil)), base: &api.Runtime{CLI: "codex", RunnerInstance: "old"}, restartPath: runnerRestartPath(file)}
	rep.Goodbye()
	mu.Lock()
	last := reports[len(reports)-1]
	mu.Unlock()
	if last.Away != "updating" {
		t.Fatal("goodbye replaced updating with stopped")
	}
}
func TestRunnerStopRequestCancelsWithoutConsoleSignal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner-stop-42")
	os.WriteFile(path, []byte("stale"), 0600)
	ctx, cancel := watchRunnerStop(context.Background(), path)
	defer cancel()
	select {
	case <-ctx.Done():
		t.Fatal("stale PID request stopped replacement")
	default:
	}
	os.WriteFile(path, []byte("stop"), 0600)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop request did not cancel")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("request was not consumed")
	}
}
