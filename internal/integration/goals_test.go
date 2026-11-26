package integration_test

import (
	"encoding/json"
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/cli"
	"github.com/myco-ai/myco/internal/infra/sqlite"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestGoalSchedulingReportsAndSummary(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(time.Minute, 5))
	admin := h.admin()
	agent := h.client(h.register(api.RegisterRequest{Name: "worker"}))
	other := h.client(h.register(api.RegisterRequest{Name: "outsider"}))
	in := api.GoalCreateRequest{Agent: "worker", Objective: "Deliver a reviewed patch", DoneWhen: "Checks pass and the owner approves"}
	if _, e := agent.CreateGoal(c, in, false); e == nil {
		t.Fatal("target authorized itself")
	}
	if _, e := other.CreateGoal(c, in, false); e == nil {
		t.Fatal("unrelated agent created goal")
	}
	g, e := admin.CreateGoal(c, in, true)
	if e != nil {
		t.Fatal(e)
	}
	gs, _ := admin.Goals(c, "", "", false)
	if len(gs) != 0 {
		t.Fatal("dry persisted")
	}
	g, e = admin.CreateGoal(c, in, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = admin.CreateGoal(c, in, false); e == nil {
		t.Fatal("second live goal")
	}
	tick := func() {
		t.Helper()
		if e := h.h.Goals.Tick(c); e != nil {
			t.Fatal(e)
		}
	}
	due := func() {
		t.Helper()
		db, e := sqlite.Open(h.db)
		if e != nil {
			t.Fatal(e)
		}
		defer db.Close()
		r, e := sqlite.NewGoalRepo(db).Get(c, g.ID)
		if e != nil {
			t.Fatal(e)
		}
		past := time.Now().Add(-time.Minute)
		r.Goal.NextWakeAt = &past
		raw, _ := json.Marshal(r)
		if _, e = db.Exec(`UPDATE goals SET data=? WHERE id=?`, string(raw), g.ID); e != nil {
			t.Fatal(e)
		}
	}
	// A real acknowledged task defers goal dispatch.
	requester := h.client(h.register(api.RegisterRequest{Name: "requester"}))
	b, e := requester.Send(c, api.SendRequest{To: "worker", Body: "Existing work"}, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = agent.Pull(c, 1, 0); e != nil {
		t.Fatal(e)
	}
	if e = agent.Ack(c, b.Message.ID); e != nil {
		t.Fatal(e)
	}
	tick()
	d, _ := admin.Goal(c, g.ID)
	if d.Goal.Round != 0 {
		t.Fatal("woke busy agent")
	}
	if _, e = agent.Reply(c, b.Message.ID, "done", false); e != nil {
		t.Fatal(e)
	}
	startRound := func() string {
		t.Helper()
		var wg sync.WaitGroup
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if e := h.h.Goals.Tick(c); e != nil {
					t.Error(e)
				}
			}()
		}
		wg.Wait()
		msgs, e := agent.Pull(c, 10, 0)
		if e != nil || len(msgs) != 1 {
			t.Fatalf("one wake: %+v %v", msgs, e)
		}
		if msgs[0].Type != "task" || msgs[0].ThreadID != g.Thread {
			t.Fatal(msgs[0])
		}
		if e = agent.Ack(c, msgs[0].ID); e != nil {
			t.Fatal(e)
		}
		return msgs[0].ID
	}
	report := func(id string, progress, achieved bool) {
		t.Helper()
		req := api.GoalReportRequest{Message: id, Summary: "Checked the patch", Progressed: &progress, NextStep: "Review remaining checks", Achieved: achieved}
		if achieved {
			req.Reason = "All criteria satisfied"
		}
		got, e := agent.ReportGoal(c, g.ID, req, false)
		if e != nil {
			t.Fatal(e)
		}
		again, e := agent.ReportGoal(c, g.ID, req, false)
		if e != nil || again.Round != got.Round {
			t.Fatalf("retry: %+v %v", again, e)
		}
		bad := req
		bad.Summary = "Different result"
		if _, e = agent.ReportGoal(c, g.ID, bad, false); e == nil {
			t.Fatal("conflicting retry")
		}
	}
	mid := startRound()
	report(mid, true, false)
	if _, e = agent.StopGoal(c, g.ID, "", false); e == nil {
		t.Fatal("target stopped its goal")
	}
	tick()
	d, _ = admin.Goal(c, g.ID)
	if d.Goal.Round != 1 {
		t.Fatal("dispatched while reporting task still running")
	}
	if _, e = agent.Reply(c, mid, "round complete", false); e != nil {
		t.Fatal(e)
	}
	tick()
	for i := 0; i < 2; i++ {
		due()
		mid = startRound()
		report(mid, false, false)
		if _, e = agent.Reply(c, mid, "no change", false); e != nil {
			t.Fatal(e)
		}
		tick()
	}
	d, e = admin.Goal(c, g.ID)
	if e != nil || d.Goal.EverySec != 3600 || d.Goal.NoProgress != 2 {
		t.Fatalf("backoff %+v %v", d, e)
	}
	// A stale report from round one must not complete round four.
	due()
	mid = startRound()
	report(mid, true, true)
	d, e = admin.Goal(c, g.ID)
	if e != nil || d.Goal.State != "achieved" || len(d.Rounds) != 4 {
		t.Fatalf("achieved %+v %v", d, e)
	}
	tick()
	db, e := sqlite.Open(h.db)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var n int
	e = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE type='note' AND body LIKE 'Goal %: achieved%'`).Scan(&n)
	if e != nil || n != 1 {
		t.Fatalf("summary %d %v", n, e)
	}
	gs, e = admin.Goals(c, "worker", "", false)
	if e != nil || len(gs) != 0 {
		t.Fatal(gs, e)
	}
}
func TestGoalNoReportPauseExpiryAndStop(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(time.Minute, 5))
	a := h.admin()
	worker := h.client(h.register(api.RegisterRequest{Name: "worker"}))
	g, e := a.CreateGoal(c, api.GoalCreateRequest{Agent: "worker", Objective: "A bounded task", DoneWhen: "Reviewed"}, false)
	if e != nil {
		t.Fatal(e)
	}
	db, e := sqlite.Open(h.db)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	for i := 0; i < 4; i++ {
		if e = h.h.Goals.Tick(c); e != nil {
			t.Fatal(e)
		}
		msgs, e := worker.Pull(c, 10, 0)
		if e != nil || len(msgs) != 1 {
			t.Fatal(msgs, e)
		}
		if e = worker.Complete(c, msgs[0].ID); e != nil {
			t.Fatal(e)
		}
		if e = h.h.Goals.Tick(c); e != nil {
			t.Fatal(e)
		}
		r, e := sqlite.NewGoalRepo(db).Get(c, g.ID)
		if e != nil {
			t.Fatal(e)
		}
		if i < 3 {
			past := time.Now().Add(-time.Second)
			r.Goal.NextWakeAt = &past
			raw, _ := json.Marshal(r)
			if _, e = db.Exec(`UPDATE goals SET data=? WHERE id=?`, string(raw), g.ID); e != nil {
				t.Fatal(e)
			}
		}
	}
	d, e := a.Goal(c, g.ID)
	if e != nil || d.Goal.State != "paused" || d.Goal.LastSummary != "没有报告 / No report" {
		t.Fatal(d, e)
	}
	if _, e = a.ResumeGoal(c, g.ID, "Continue after advice", false); e != nil {
		t.Fatal(e)
	}
	if _, e = a.StopGoal(c, g.ID, "Owner stopped", false); e != nil {
		t.Fatal(e)
	}
	if _, e = a.ResumeGoal(c, g.ID, "", false); e == nil {
		t.Fatal("revived terminal goal")
	}
	g, e = a.CreateGoal(c, api.GoalCreateRequest{Agent: "worker", Objective: "A second task", DoneWhen: "Reviewed"}, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = h.h.Goals.Tick(c); e != nil {
		t.Fatal(e)
	}
	r, e := sqlite.NewGoalRepo(db).Get(c, g.ID)
	if e != nil {
		t.Fatal(e)
	}
	r.Goal.Deadline = time.Now().Add(-time.Second)
	raw, _ := json.Marshal(r)
	if _, e = db.Exec(`UPDATE goals SET data=? WHERE id=?`, string(raw), g.ID); e != nil {
		t.Fatal(e)
	}
	if e = h.h.Goals.Tick(c); e != nil {
		t.Fatal(e)
	}
	d, e = a.Goal(c, g.ID)
	if e != nil || d.Goal.State != "expired" {
		t.Fatal(d, e)
	}
	var n int
	if e = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE type='task' AND status='queued' AND to_agent='worker'`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("expired wake left queued %d %v", n, e)
	}
}

func TestGoalCoordinatorAndCLI(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(time.Minute, 5))
	admin := h.admin()
	token := h.register(api.RegisterRequest{Name: "worker"})
	worker := h.client(token)
	coordToken := h.register(api.RegisterRequest{Name: "coordinator"})
	coord := h.client(coordToken)
	h.register(api.RegisterRequest{Name: "outsider"})
	p, e := admin.CreateProject(c, api.CreateProjectRequest{Name: "demo"})
	if e != nil {
		t.Fatal(e)
	}
	role := "coordinator"
	if _, e = admin.PutProjectMember(c, p.Name, "coordinator", api.ProjectMemberRequest{Role: &role}); e != nil {
		t.Fatal(e)
	}
	if _, e = admin.PutProjectMember(c, p.Name, "worker", api.ProjectMemberRequest{}); e != nil {
		t.Fatal(e)
	}
	cmd := newCLI(t, h, filepath.Join(t.TempDir(), "coord.yaml"))
	cmd.env["MYCO_TOKEN"] = coordToken
	cmd.env["MYCO_NO_UPDATE_CHECK"] = "1"
	args := []string{"goal", "start", "--agent", "worker", "--project", "demo", "--objective", "Prepare a patch", "--done-when", "Review accepted"}
	dry := append(append([]string{}, args...), "--dry-run")
	code, _, stderr := cmd.run(c, dry...)
	if code != cli.ExitDryRun {
		t.Fatal(code, stderr)
	}
	var g api.Goal
	if e = json.Unmarshal([]byte(cmd.must(c, args...)), &g); e != nil {
		t.Fatal(e)
	}
	var detail api.GoalDetail
	if e = json.Unmarshal([]byte(cmd.must(c, "goal", "get", "--id", g.ID)), &detail); e != nil || detail.Goal.ID != g.ID {
		t.Fatal(detail, e)
	}
	cmd.must(c, "goal", "set", "--id", g.ID, "--every", "15m", "--max-rounds", "8")
	cmd.must(c, "goal", "pause", "--id", g.ID, "--reason", "Check constraints")
	cmd.must(c, "goal", "resume", "--id", g.ID)
	if e = h.h.Goals.Tick(c); e != nil {
		t.Fatal(e)
	}
	msgs, e := worker.Pull(c, 1, 0)
	if e != nil || len(msgs) != 1 {
		t.Fatal(msgs, e)
	}
	if e = worker.Ack(c, msgs[0].ID); e != nil {
		t.Fatal(e)
	}
	workerCLI := newCLI(t, h, filepath.Join(t.TempDir(), "worker.yaml"))
	workerCLI.env["MYCO_TOKEN"] = token
	workerCLI.env["MYCO_MESSAGE_ID"] = msgs[0].ID
	workerCLI.must(c, "goal", "report", "--id", g.ID, "--summary", "Need review", "--no-progress", "--blocked")
	updated, e := admin.Goal(c, g.ID)
	if e != nil || updated.Goal.State != "paused" || !updated.Rounds[0].Blocked {
		t.Fatal(updated, e)
	}
	// A creator remains able to stop, but another coordinator requires current membership.
	cmd.must(c, "goal", "stop", "--id", g.ID, "--reason", "No longer needed")
	list, e := coord.Goals(c, "worker", "demo", true)
	if e != nil || len(list) != 1 {
		t.Fatal(list, e)
	}
	if _, e = admin.RemoveProjectMember(c, p.Name, "worker"); e != nil {
		t.Fatal(e)
	}
	if _, e = coord.CreateGoal(c, api.GoalCreateRequest{Agent: "worker", Project: "demo", Objective: "Another", DoneWhen: "Reviewed"}, false); e == nil {
		t.Fatal("created for removed project member")
	}
}
