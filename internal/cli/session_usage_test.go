package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
)

func usageLine(id, model string, at time.Time) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":%q,"model":%q,"usage":{"input_tokens":3,"cache_read_input_tokens":7,"cache_creation_input_tokens":2,"output_tokens":5}}}`, at.Format(time.RFC3339), id, model) + "\n"
}
func TestSessionUsageGroupsAndResumes(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	file := filepath.Join(t.TempDir(), "claude.jsonl")
	data := usageLine("old", "opus", now.AddDate(0, 0, -8)) + usageLine("a", "opus", now) + usageLine("a", "opus", now) + usageLine("synthetic", "<synthetic>", now) + usageLine("b", "sonnet", now.Add(-24*time.Hour))
	if err := os.WriteFile(file, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	rows, c, err := readSessionUsage(file, usageCursor{}, "project", now)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows %v %v", rows, err)
	}
	for _, r := range rows {
		if r.Calls != 1 || r.InputTokens != 3 || r.CacheReadTokens != 7 || r.CacheWriteTokens != 2 || r.OutputTokens != 5 || r.Project != "project" || r.Key == "" {
			t.Fatalf("bad aggregation %+v", r)
		}
	}
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString(usageLine("b", "sonnet", now.Add(-24*time.Hour)) + usageLine("c", "opus", now))
	f.Close()
	rows, next, err := readSessionUsage(file, c, "next-project", now)
	if err != nil || len(rows) != 1 || rows[0].Calls != 1 || rows[0].Project != "next-project" || next.Offset <= c.Offset {
		t.Fatalf("resume %v %+v %v", rows, next, err)
	}
	// A tail still being written is left for the next wait.
	partial := strings.TrimSuffix(usageLine("d", "opus", now), "\n")
	f, _ = os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString(partial)
	f.Close()
	rows, c, err = readSessionUsage(file, next, "", now)
	if err != nil || len(rows) != 0 || c.Offset != next.Offset {
		t.Fatal("incomplete JSONL was consumed")
	}
	f, _ = os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString("\n")
	f.Close()
	rows, _, err = readSessionUsage(file, c, "", now)
	if err != nil || len(rows) != 1 || rows[0].Calls != 1 {
		t.Fatalf("completed tail %v %v", rows, err)
	}
}
func TestSessionUsagePendingSurvivesUncertainAckAndNewTail(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "session.jsonl")
	now := time.Now()
	if err := os.WriteFile(file, []byte(usageLine("a", "opus", now)), 0600); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	calls := int64(0)
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.UsageRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			t.Error("bad request")
		}
		for _, p := range req.Reports {
			if !seen[p.Key] {
				seen[p.Key] = true
				calls += p.Calls
			}
		}
		if fail {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"recorded":0}`)
	}))
	defer srv.Close()
	cl, _ := client.New(srv.URL, "fixture")
	tracker := &sessionUsageTracker{path: filepath.Join(dir, "agent.usage"), file: file, targets: []sessionUsageTarget{{Hub: srv.URL, Agent: "agent", Client: cl}}}
	tracker.report(context.Background())
	state, err := tracker.load()
	if err != nil || state.Pending == nil || state.Offset != 0 || calls != 1 {
		t.Fatalf("uncertain ack state %+v calls=%d %v", state, calls, err)
	}
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString(usageLine("b", "opus", now))
	f.Close()
	fail = false
	tracker.report(context.Background())
	state, err = tracker.load()
	if err != nil || state.Pending != nil || state.Offset == 0 || calls != 2 {
		t.Fatalf("retry duplicated old usage: calls=%d state=%+v %v", calls, state, err)
	}
	tracker.remember(srv.URL, "agent", []api.Message{{Project: "one"}, {Project: "two"}})
	state, _ = tracker.load()
	if state.Project != "" {
		t.Fatal("mixed projects attributed to one project")
	}
	tracker.remember(srv.URL, "agent", []api.Message{{Project: "one"}, {Project: "one"}})
	state, _ = tracker.load()
	if state.Project != "one" {
		t.Fatal("same project not remembered")
	}
}

func TestSessionUsageRangeKeyStableAndCrossProcessLock(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "transcript")
	if err := os.WriteFile(file, []byte(usageLine("a", "model", time.Now())), 0600); err != nil {
		t.Fatal(err)
	}
	first, _, err := readSessionUsage(file, usageCursor{}, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := readSessionUsage(file, usageCursor{}, "", time.Now().Add(time.Second))
	if err != nil || len(first) != 1 || len(again) != 1 || first[0].Key != again[0].Key {
		t.Fatal("retry key was not stable")
	}
	path := filepath.Join(dir, "state.lock")
	unlock, err := lockUsageState(path)
	if err != nil {
		t.Fatal(err)
	}
	if release, err := lockUsageState(path); err == nil {
		release()
		unlock()
		t.Fatal("concurrent updater acquired the same lock")
	}
	unlock()
	release, err := lockUsageState(path)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestInboxWaitAutomaticallyReportsClaudeSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	claudeDir := filepath.Join(dir, "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	projectDir := filepath.Join(claudeDir, "projects", "fixture")
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(projectDir, "usage-session.jsonl")
	line := usageLine("assistant-one", "opus", time.Now())
	if err := os.WriteFile(file, []byte(line+line), 0600); err != nil {
		t.Fatal(err)
	}
	hub := newHub(t)
	alice := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "alice.yaml")}}
	bob := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "bob.yaml")}}
	if code, _, err := alice.run("agent", "register", "--name", "alice", "--harness", "claude", "--session-id", "usage-session"); code != 0 {
		t.Fatal(err)
	}
	if code, _, err := bob.run("agent", "register", "--name", "bob"); code != 0 {
		t.Fatal(err)
	}
	if code, _, err := bob.run("message", "send", "--to", "alice", "--body", "work"); code != 0 {
		t.Fatal(err)
	}
	alice.env["CLAUDE_CODE_SESSION_ID"] = "usage-session"
	for range 2 {
		if code, _, err := alice.run("inbox", "wait", "--timeout", "1s"); code != 0 {
			t.Fatal(err)
		}
	}
	code, body, stderr := bob.run("usage", "show")
	if code != 0 {
		t.Fatal(stderr)
	}
	var out api.UsageList
	if err := json.Unmarshal([]byte(body), &out); err != nil || len(out.Rows) != 1 || out.Rows[0].Source != "session" || out.Rows[0].Calls != 1 || out.Rows[0].InputTokens != 3 {
		t.Fatalf("automatic session accounting %s %v", body, err)
	}
	// Token-only configurations recover their authoritative node name from Me.
	cfg, err := config.Load(alice.env["MYCO_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	cfg.Agent = ""
	if err := config.Save(alice.env["MYCO_CONFIG"], cfg); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(usageLine("assistant-two", "opus", time.Now()))
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if code, _, err := alice.run("inbox", "wait", "--timeout", "1s"); code != 0 {
		t.Fatal(err)
	}
	code, body, stderr = bob.run("usage", "show")
	if code != 0 {
		t.Fatal(stderr)
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || len(out.Rows) != 1 || out.Rows[0].Calls != 2 {
		t.Fatalf("token-only accounting %s %v", body, err)
	}
	// Accounting opt-out or an unavailable transcript cannot break receiving.
	alice.env["MYCO_NO_USAGE"] = "1"
	if code, _, err := alice.run("inbox", "wait", "--timeout", "1s"); code != 0 {
		t.Fatal(err)
	}
	alice.env["MYCO_NO_USAGE"] = ""
	alice.env["CLAUDE_CODE_SESSION_ID"] = "missing-session"
	if code, _, err := alice.run("inbox", "wait", "--timeout", "1s"); code != 0 {
		t.Fatal(err)
	}
}
