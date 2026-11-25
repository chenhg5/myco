package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/cli"
)

func TestWatchHoldCompletionAndExpiry(t *testing.T) {
	c := ctx(t)
	db := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, db, policy(time.Minute, 5))
	sender := h.client(h.register(api.RegisterRequest{Name: "requester"}))
	token := h.register(api.RegisterRequest{Name: "watcher"})
	watcher := h.client(token)
	worker := h.client(h.register(api.RegisterRequest{Name: "worker"}))
	stranger := h.client(h.register(api.RegisterRequest{Name: "stranger"}))
	makeHeld := func() (string, string) {
		t.Helper()
		parent, err := sender.Send(c, api.SendRequest{To: "watcher", Body: "Review the result"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = watcher.Pull(c, 10, 0); err != nil {
			t.Fatal(err)
		}
		child, err := watcher.Send(c, api.SendRequest{To: "worker", Body: "Run the checks"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = watcher.Hold(c, parent.Message.ID, child.Message.ID, false); err != nil {
			t.Fatal(err)
		}
		return parent.Message.ID, child.Message.ID
	}
	parent, child := makeHeld()
	ws, err := watcher.Watches(c, "", "", false)
	if err != nil || len(ws) != 1 {
		t.Fatalf("watches: %v %+v", err, ws)
	}
	w := ws[0]
	if w.Task != parent || w.Target != child || w.Kind != "task" || w.Deadline.Sub(w.StartedAt) != 30*time.Minute {
		t.Fatalf("automatic watch: %+v", w)
	}
	if _, err = stranger.StopWatch(c, w.ID, "", false); err == nil {
		t.Fatal("unrelated agent stopped watch")
	}
	if _, err = watcher.ExtendWatch(c, w.ID, 60, true); err != nil {
		t.Fatal(err)
	}
	unchanged, err := watcher.Watch(c, w.ID)
	if err != nil || !unchanged.Deadline.Equal(w.Deadline) {
		t.Fatalf("dry extend changed watch: %+v %v", unchanged, err)
	}
	agents, err := h.admin().ListAgents(c, "", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range agents {
		if a.Name == "watcher" {
			found = true
			if a.Watching != 1 || a.Held != 1 {
				t.Fatalf("counts: %+v", a)
			}
		}
	}
	if !found {
		t.Fatal("missing agent")
	}
	if _, err = worker.Pull(c, 10, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Reply(c, child, "All checks passed", false); err != nil {
		t.Fatal(err)
	}
	// Race the original runner's WakeHeld path against Hub evaluation. Only
	// the original task is claimed, never its reply or an extra wake event.
	watcher.Via, watcher.WakeHeld = "runner", true
	var wg sync.WaitGroup
	var mu sync.Mutex
	claimed := []api.Message{}
	errs := []error{}
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if e := h.h.Watches.Tick(c); e != nil {
				mu.Lock()
				errs = append(errs, e)
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			ms, e := watcher.Pull(c, 10, 0)
			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				errs = append(errs, e)
			}
			claimed = append(claimed, ms...)
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(claimed) != 1 || claimed[0].ID != parent || claimed[0].HoldWake == "" {
		t.Fatalf("duplicate or missing wake: %+v", claimed)
	}
	ended, err := watcher.Watch(c, w.ID)
	if err != nil || ended.State != "met" {
		t.Fatalf("not met: %+v %v", ended, err)
	}
	// Complete the resumed task before constructing a second held task.
	if _, err = watcher.Reply(c, parent, "Done", false); err != nil {
		t.Fatal(err)
	}
	parent2, child2 := makeHeld()
	auto, err := watcher.Watches(c, "", "", false)
	if err != nil || len(auto) != 1 {
		t.Fatalf("second automatic watch: %+v %v", auto, err)
	}
	// Set a short deadline via the public API; wait for that exact deadline.
	short, err := watcher.ExtendWatch(c, auto[0].ID, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	h.stop()
	h = startHub(t, db, policy(time.Minute, 5))
	watcher = h.client(token)
	watcher.Via, watcher.WakeHeld = "runner", true
	if d := time.Until(short.Deadline); d > 0 {
		select {
		case <-c.Done():
			t.Fatal(c.Err())
		case <-time.After(d):
		}
	}
	if err = h.h.Watches.Tick(c); err != nil {
		t.Fatal(err)
	}
	if err = h.h.Watches.Tick(c); err != nil {
		t.Fatal(err)
	}
	expired, err := watcher.Watch(c, short.ID)
	if err != nil || expired.State != "expired" {
		t.Fatalf("expiry after restart: %+v %v", expired, err)
	}
	ms, err := watcher.Pull(c, 10, 0)
	if err != nil || len(ms) != 1 || ms[0].ID != parent2 {
		t.Fatalf("expiry wake: %+v %v", ms, err)
	}
	notice, err := watcher.GetMessage(c, ms[0].HoldWake)
	if err != nil || notice.Body == "" {
		t.Fatalf("expiry notice: %+v %v", notice, err)
	}
	if _, err = watcher.ExtendWatch(c, short.ID, 30, false); err == nil {
		t.Fatal("extended ended watch")
	}
	_ = child2
}

func TestWatchStandaloneDryRunAndStop(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(time.Minute, 5))
	a := h.client(h.register(api.RegisterRequest{Name: "watcher"}))
	h.register(api.RegisterRequest{Name: "worker"})
	sent, err := a.Send(c, api.SendRequest{To: "worker", Body: "Check the build"}, false)
	if err != nil {
		t.Fatal(err)
	}
	req := api.WatchStartRequest{What: "Build result", Kind: "reply", Target: sent.Message.ID}
	if _, err = a.StartWatch(c, req, true); err != nil {
		t.Fatal(err)
	}
	ws, err := a.Watches(c, "", "", true)
	if err != nil || len(ws) != 0 {
		t.Fatalf("dry start persisted: %+v %v", ws, err)
	}
	w, err := a.StartWatch(c, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.StopWatch(c, w.ID, "no longer needed", true); err != nil {
		t.Fatal(err)
	}
	if _, err = h.admin().StopWatch(c, w.ID, "owner stopped", false); err != nil {
		t.Fatal(err)
	}
	ms, err := a.Pull(c, 10, 0)
	if err != nil || len(ms) != 1 {
		t.Fatalf("standalone stop wake: %+v %v", ms, err)
	}
	stopped, err := a.Watch(c, w.ID)
	if err != nil || stopped.State != "stopped" {
		t.Fatalf("stop: %+v %v", stopped, err)
	}
	for _, bad := range []api.WatchStartRequest{{What: "bad", Kind: "command", Target: sent.Message.ID}, {What: "bad", Kind: "task", Target: sent.Message.ID, ForSec: 86401}} {
		if _, err = a.StartWatch(c, bad, false); err == nil {
			t.Fatal(fmt.Sprint("accepted invalid watch", bad))
		}
	}
}

func TestWatchProjectPermissionsAndWorkflow(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(time.Minute, 5))
	admin := h.admin()
	watcher := h.client(h.register(api.RegisterRequest{Name: "watcher"}))
	worker := h.client(h.register(api.RegisterRequest{Name: "worker"}))
	coord := h.client(h.register(api.RegisterRequest{Name: "coordinator"}))
	outsider := h.client(h.register(api.RegisterRequest{Name: "outsider"}))
	p, err := admin.CreateProject(c, api.CreateProjectRequest{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"watcher", "worker", "coordinator"} {
		req := api.ProjectMemberRequest{}
		if name == "coordinator" {
			role := "coordinator"
			req.Role = &role
		}
		if _, err = admin.PutProjectMember(c, p.Name, name, req); err != nil {
			t.Fatal(err)
		}
	}
	task, err := watcher.Send(c, api.SendRequest{To: "worker", Body: "Run checks", Project: p.Name}, false)
	if err != nil {
		t.Fatal(err)
	}
	w, err := watcher.StartWatch(c, api.WatchStartRequest{What: "Project checks", Kind: "task", Target: task.Message.ID, Project: p.Name}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = outsider.ExtendWatch(c, w.ID, 60, false); err == nil {
		t.Fatal("outsider extended watch")
	}
	if _, err = coord.ExtendWatch(c, w.ID, 60, false); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.RemoveProjectMember(c, p.Name, "watcher"); err != nil {
		t.Fatal(err)
	}
	if _, err = coord.StopWatch(c, w.ID, "", false); err == nil {
		t.Fatal("coordinator managed former member's watch")
	}
	if _, err = admin.StopWatch(c, w.ID, "owner stopped", false); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequestWithContext(c, http.MethodGet, h.url+"/v1/audit?target="+w.ID, nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	var audit api.AuditList
	e = json.NewDecoder(response.Body).Decode(&audit)
	response.Body.Close()
	if e != nil {
		t.Fatal(e)
	}
	if len(audit.Entries) != 2 {
		t.Fatalf("watch changes not audited exactly once: %+v", audit)
	}
	actions := map[string]bool{}
	for _, entry := range audit.Entries {
		actions[entry.Action] = true
		if entry.Actor == "" || entry.At.IsZero() {
			t.Fatal(entry)
		}
	}
	if !actions["watch.stop"] || !actions["watch.extend"] {
		t.Fatal(audit)
	}
	// Workflow conditions use the same read permissions as workflow-runs.
	def := []byte(`{"name":"watch-workflow","steps":[{"id":"work","agent":"worker","prompt":"Run checks"}],"edges":[{"from":"work","to":"$end"}]}`)
	if _, err = watcher.ApplyWorkflow(c, def, false); err != nil {
		t.Fatal(err)
	}
	run, _, err := watcher.StartWorkflow(c, "watch-workflow", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	parent, e := outsider.Send(c, api.SendRequest{To: "watcher", Body: "Wait on workflow"}, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = watcher.Pull(c, 10, 0); e != nil {
		t.Fatal(e)
	}
	rw, err := watcher.StartWatch(c, api.WatchStartRequest{What: "Workflow completion", Kind: "workflow", Target: run.ID, Task: parent.Message.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = watcher.CancelWorkflowRun(c, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = h.h.Watches.Tick(c); err != nil {
		t.Fatal(err)
	}
	done, err := watcher.Watch(c, rw.ID)
	if err != nil || done.State != "met" || done.LastResult != "workflow cancelled" {
		t.Fatalf("workflow watch: %+v %v", done, err)
	}
	// A third party cannot start watches against a private task.
	if _, err = outsider.StartWatch(c, api.WatchStartRequest{What: "Private", Kind: "task", Target: task.Message.ID}, false); err == nil {
		t.Fatal("unrelated agent watched task")
	}
	_ = worker
}

func TestWatchStartBindsTaskAtomically(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(time.Minute, 5))
	sender := h.client(h.register(api.RegisterRequest{Name: "requester"}))
	a := h.client(h.register(api.RegisterRequest{Name: "watcher"}))
	h.register(api.RegisterRequest{Name: "worker"})
	p, err := sender.Send(c, api.SendRequest{To: "watcher", Body: "Wait on checks"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Pull(c, 10, 0); err != nil {
		t.Fatal(err)
	}
	child, err := a.Send(c, api.SendRequest{To: "worker", Body: "Run checks"}, false)
	if err != nil {
		t.Fatal(err)
	}
	req := api.WatchStartRequest{What: "Checks", Kind: "task", Target: child.Message.ID, Task: p.Message.ID}
	if _, err = a.StartWatch(c, req, true); err != nil {
		t.Fatal(err)
	}
	before, err := a.GetMessage(c, p.Message.ID)
	if err != nil || before.Status == "held" {
		t.Fatalf("dry start held task: %+v %v", before, err)
	}
	w, err := a.StartWatch(c, req, false)
	if err != nil {
		t.Fatal(err)
	}
	held, err := a.GetMessage(c, p.Message.ID)
	if err != nil || held.Status != "held" {
		t.Fatalf("start did not hold task: %+v %v", held, err)
	}
	again, err := a.StartWatch(c, req, false)
	if err != nil || again.ID != w.ID {
		t.Fatalf("duplicate watch on retry: %+v %v", again, err)
	}
	if _, err = a.StopWatch(c, w.ID, "stop", false); err != nil {
		t.Fatal(err)
	}
	ms, err := a.Pull(c, 10, 0)
	if err != nil || len(ms) != 1 || ms[0].ID != p.Message.ID {
		t.Fatalf("stop wake: %+v %v", ms, err)
	}
}

func TestWatchCLIRealHub(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(time.Minute, 5))
	token := h.register(api.RegisterRequest{Name: "watcher"})
	a := h.client(token)
	h.register(api.RegisterRequest{Name: "worker"})
	target, err := a.Send(c, api.SendRequest{To: "worker", Body: "Build"}, false)
	if err != nil {
		t.Fatal(err)
	}
	cmd := newCLI(t, h, filepath.Join(t.TempDir(), "config.yaml"))
	cmd.env["MYCO_TOKEN"] = token
	cmd.env["MYCO_NO_UPDATE_CHECK"] = "1"
	code, _, stderr := cmd.run(c, "watch", "start", "--what", "Build reply", "--until", "reply:"+target.Message.ID, "--dry-run")
	if code != cli.ExitDryRun {
		t.Fatalf("dry CLI: %d %s", code, stderr)
	}
	var list api.WatchList
	if err = json.Unmarshal([]byte(cmd.must(c, "watch", "list")), &list); err != nil || len(list.Watches) != 0 {
		t.Fatalf("dry CLI persisted: %+v %v", list, err)
	}
	var w api.Watch
	if err = json.Unmarshal([]byte(cmd.must(c, "watch", "start", "--what", "Build reply", "--until", "reply:"+target.Message.ID)), &w); err != nil || w.State != "watching" {
		t.Fatalf("start CLI: %+v %v", w, err)
	}
	if err = json.Unmarshal([]byte(cmd.must(c, "watch", "extend", "--id", w.ID, "--for", "1h")), &w); err != nil || w.Deadline.Sub(w.StartedAt) < time.Hour {
		t.Fatalf("extend CLI: %+v %v", w, err)
	}
	if err = json.Unmarshal([]byte(cmd.must(c, "watch", "stop", "--id", w.ID, "--reason", "No longer needed")), &w); err != nil || w.State != "stopped" {
		t.Fatalf("stop CLI: %+v %v", w, err)
	}
	if err = json.Unmarshal([]byte(cmd.must(c, "watch", "list", "--all")), &list); err != nil || len(list.Watches) != 1 {
		t.Fatalf("list CLI: %+v %v", list, err)
	}
}
