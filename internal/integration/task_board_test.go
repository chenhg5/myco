package integration_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestTaskBoardCLIAndCoordinatorAlertsRealHub(t *testing.T) {
	c := ctx(t)
	path := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, path, app.DefaultMessagePolicy())
	senderToken := h.register(api.RegisterRequest{Name: "lead"})
	lead := h.client(senderToken)
	worker := h.client(h.register(api.RegisterRequest{Name: "worker"}))
	other := h.client(h.register(api.RegisterRequest{Name: "other"}))
	if _, err := h.admin().CreateProject(c, api.CreateProjectRequest{Name: "acme"}); err != nil {
		t.Fatal(err)
	}
	coord := "coordinator"
	for _, name := range []string{"lead", "worker", "other"} {
		r := api.ProjectMemberRequest{}
		if name == "lead" {
			r.Role = &coord
		}
		if _, err := h.admin().PutProjectMember(c, "acme", name, r); err != nil {
			t.Fatal(err)
		}
	}
	// Mutations have a real server-side dry run; unauthorized callers cannot use it.
	if _, err := lead.TaskAlertPolicy(c, "acme", &api.TaskAlertPolicyRequest{StaleSeconds: 1}, true); err != nil {
		t.Fatal(err)
	}
	p, err := lead.TaskAlertPolicy(c, "acme", nil, false)
	if err != nil || p.StaleSeconds != 900 {
		t.Fatalf("dry run changed threshold %+v %v", p, err)
	}
	if _, err = worker.TaskAlertPolicy(c, "acme", &api.TaskAlertPolicyRequest{StaleSeconds: 1}, true); err == nil {
		t.Fatal("ordinary member set alert policy")
	}
	if _, err = lead.TaskAlertPolicy(c, "acme", &api.TaskAlertPolicyRequest{StaleSeconds: 1}, false); err != nil {
		t.Fatal(err)
	}
	sent, err := lead.Send(c, api.SendRequest{To: "worker", Body: "secret task body", Project: "acme"}, false)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := other.Send(c, api.SendRequest{To: "worker", Body: "other private body", Project: "acme"}, false)
	if err != nil {
		t.Fatal(err)
	}
	// Work is in progress (acked) and held work is also shown, but neither
	// metadata listing nor --all returns the bodies or credentials.
	if _, err = worker.Pull(c, 10, 0); err != nil {
		t.Fatal(err)
	}
	if err = worker.Ack(c, sent.Message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Hold(c, theirs.Message.ID, "", false); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := time.Now().Add(-2 * time.Second).UnixMilli()
	if _, err = db.ExecContext(c, `UPDATE messages SET active_at=?,updated_at=?,created_at=? WHERE id IN (?,?)`, old, old, time.Now().Add(-3*time.Minute).UnixMilli(), sent.Message.ID, theirs.Message.ID); err != nil {
		t.Fatal(err)
	}
	own, err := lead.TaskBoard(c, "", false, 0)
	if err != nil || len(own.Tasks) != 1 || !own.Tasks[0].Stale {
		t.Fatalf("own board %+v %v", own, err)
	}
	all, err := lead.TaskBoard(c, "acme", true, 0)
	if err != nil || len(all.Tasks) != 2 {
		t.Fatalf("project board %+v %v", all, err)
	}
	raw, _ := json.Marshal(all)
	if strings.Contains(string(raw), "private body") || strings.Contains(string(raw), "secret") || strings.Contains(string(raw), senderToken) {
		t.Fatal("board leaked contents")
	}
	if _, err = worker.TaskBoard(c, "acme", true, 0); err == nil {
		t.Fatal("member enumerated everyone's tasks")
	}
	if _, err = lead.TaskBoard(c, "", true, 0); err == nil {
		t.Fatal("unscoped all accepted")
	}
	env := newCLI(t, h, filepath.Join(t.TempDir(), "config.yaml"))
	env.env["MYCO_TOKEN"] = senderToken
	delete(env.env, "MYCO_ADMIN_TOKEN")
	env.env["HOME"] = t.TempDir()
	code, out, stderr := env.run(c, "task", "board", "--project", "acme", "--all")
	if code != 0 || !strings.Contains(out, sent.Message.ID) || strings.Contains(out, "secret task body") {
		t.Fatalf("CLI board: %d %s %s", code, out, stderr)
	}
	code, out, stderr = env.run(c, "task", "alerts", "set", "--project", "acme", "--stale", "2s", "--dry-run")
	if code != 10 {
		t.Fatalf("CLI policy dry-run: %d %s %s", code, out, stderr)
	}
	p, err = lead.TaskAlertPolicy(c, "acme", nil, false)
	if err != nil || p.StaleSeconds != 1 {
		t.Fatal("CLI dry run saved policy", p, err)
	}
	if err = h.h.TaskMonitor.Tick(c); err != nil {
		t.Fatal(err)
	}
	notes, err := lead.Pull(c, 10, 0)
	if err != nil || len(notes) != 1 || notes[0].Type != "note" || !strings.Contains(notes[0].Body, sent.Message.ID) || !strings.Contains(notes[0].Body, "myco message steer") {
		t.Fatalf("alert inbox %+v %v", notes, err)
	}
	if strings.Contains(notes[0].Body, theirs.Message.ID) || strings.Contains(notes[0].Body, "secret") {
		t.Fatal("alert leaked another sender's task")
	}
	// Persisted dedup survives a hub restart and does not wake the sender twice.
	if err = lead.Complete(c, notes[0].ID); err != nil {
		t.Fatal(err)
	}
	h.stop()
	h = startHub(t, path, app.DefaultMessagePolicy())
	lead = h.client(senderToken)
	if err = h.h.TaskMonitor.Tick(c); err != nil {
		t.Fatal(err)
	}
	notes, err = lead.Pull(c, 10, 0)
	if err != nil || len(notes) != 0 {
		t.Fatalf("restart duplicate %+v %v", notes, err)
	}
	// Heartbeat expiry is independently deduped, but shares the same 5m budget.
	if _, err = db.ExecContext(c, `UPDATE agents SET last_seen_at=? WHERE name='worker'`, time.Now().Add(-2*time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err = h.h.TaskMonitor.Tick(c); err != nil {
		t.Fatal(err)
	}
	notes, err = lead.Pull(c, 10, 0)
	if err != nil || len(notes) != 0 {
		t.Fatal("offline alert bypassed sender cooldown", notes, err)
	}
	// Move the persisted cooldown to its boundary rather than waiting five minutes.
	if _, err = db.ExecContext(c, `UPDATE task_alert_deliveries SET sent_at=? WHERE sender='lead'`, time.Now().Add(-5*time.Minute-time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err = h.h.TaskMonitor.Tick(c); err != nil {
		t.Fatal(err)
	}
	notes, err = lead.Pull(c, 10, 0)
	if err != nil || len(notes) != 1 || !strings.Contains(notes[0].Body, "heartbeat expired") {
		t.Fatalf("offline alert %+v %v", notes, err)
	}
	if strings.Contains(notes[0].Body, "no recipient activity") {
		t.Fatal("stale alert repeated", notes[0].Body)
	}
}

func TestTaskMachineIncidentRealHub(t *testing.T) {
	c := ctx(t)
	path := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, path, app.DefaultMessagePolicy())
	lead := h.client(h.register(api.RegisterRequest{Name: "lead"}))
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.ExecContext(c, `INSERT INTO machines(id,fingerprint_hash,name,hostname,os,arch,myco_version,created_at,last_seen_at,owner)VALUES('demo-machine','demo-fingerprint','demo','','','','',1,1,'owner')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("worker-%d", i)
		worker := h.client(h.register(api.RegisterRequest{Name: name}))
		if _, err = db.ExecContext(c, `UPDATE agents SET machine_id='demo-machine',owner='owner' WHERE name=?`, name); err != nil {
			t.Fatal(err)
		}
		sent, err := lead.Send(c, api.SendRequest{To: name, Body: "synthetic task"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = worker.Pull(c, 10, 0); err != nil {
			t.Fatal(err)
		}
		if _, err = worker.Fail(c, sent.Message.ID, "synthetic execution failure", false); err != nil {
			t.Fatal(err)
		}
		if err = h.h.TaskMonitor.Tick(c); err != nil {
			t.Fatal(err)
		}
	}
	notes, err := lead.Pull(c, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	alerts := 0
	for _, n := range notes {
		if n.Type == "note" && strings.Contains(n.Body, "same-machine incident") {
			alerts++
			if !strings.Contains(n.Body, "at least 3 agents") || strings.Contains(n.Body, "synthetic execution failure") {
				t.Fatal(n.Body)
			}
		}
	}
	if alerts != 1 {
		t.Fatalf("machine incident alerts=%d; inbox=%+v", alerts, notes)
	}
	if err = h.h.TaskMonitor.Tick(c); err != nil {
		t.Fatal(err)
	}
	notes, err = lead.Pull(c, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if n.Type == "note" {
			t.Fatal("incident repeated", n)
		}
	}
}

func TestTaskBoardConsoleIdentitySurvivesRename(t *testing.T) {
	c := ctx(t)
	path := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, path, app.DefaultMessagePolicy())
	admin := h.admin()
	h.register(api.RegisterRequest{Name: "worker"})
	first, err := admin.ConsoleSend(c, api.SendRequest{To: "worker", Body: "person's synthetic task"})
	if err != nil {
		t.Fatal(err)
	}
	board, err := admin.TaskBoard(c, "", false, 0)
	if err != nil || len(board.Tasks) != 1 || board.Tasks[0].ID != first.Message.ID {
		t.Fatalf("master owner sender identity: %+v %v", board, err)
	}
	if _, err = admin.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "new-owner"}); err != nil {
		t.Fatal(err)
	}
	board, err = admin.TaskBoard(c, "", false, 0)
	if err != nil || len(board.Tasks) != 1 {
		t.Fatalf("renaming hid sent task: %+v %v", board, err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	replacement := &domain.Member{ID: "replacement-account", Name: "owner", Role: domain.RoleMember, CreatedAt: time.Now()}
	if err = sqlite.NewMemberRepo(db).Create(c, replacement); err != nil {
		t.Fatal(err)
	}
	replacementBoard, err := h.h.TaskMonitor.Board(c, app.Actor{Member: replacement}, "", false, 0)
	if err != nil || len(replacementBoard.Tasks) != 0 {
		t.Fatalf("reused handle inherited another account's sent tasks: %+v %v", replacementBoard, err)
	}
	if _, err = db.ExecContext(c, `UPDATE messages SET created_at=?,updated_at=?,active_at=? WHERE id=?`, time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli(), first.Message.ID); err != nil {
		t.Fatal(err)
	}
	if err = h.h.TaskMonitor.Tick(c); err != nil {
		t.Fatal(err)
	}
	notes, err := sqlite.NewMessageRepo(db).ListRecent(c, app.ConsoleSender("new-owner"), 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range notes {
		if n.Type == "note" {
			found = true
			if n.To != app.ConsoleSender("new-owner") {
				t.Fatal("alert used previous handle", n)
			}
		}
	}
	if !found {
		t.Fatal("no note to renamed owner")
	}
	second, err := admin.ConsoleSend(c, api.SendRequest{To: "worker", Body: "second synthetic task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.UpdateMember(c, "new-owner", api.MemberUpdateRequest{Name: "renamed-owner"}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(c, `UPDATE messages SET created_at=?,updated_at=?,active_at=? WHERE id=?`, time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli(), second.Message.ID); err != nil {
		t.Fatal(err)
	}
	if err = h.h.TaskMonitor.Tick(c); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRowContext(c, `SELECT COUNT(*) FROM messages WHERE type='note' AND from_agent=?`, app.AdminSender).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rename bypassed sender cooldown: %d %v", count, err)
	}
	if _, err = db.ExecContext(c, `UPDATE task_alert_deliveries SET sent_at=?`, time.Now().Add(-6*time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err = h.h.TaskMonitor.Tick(c); err != nil {
		t.Fatal(err)
	}
	notes, err = sqlite.NewMessageRepo(db).ListRecent(c, app.ConsoleSender("renamed-owner"), 20)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, n := range notes {
		if n.Type == "note" && strings.Contains(n.Body, second.Message.ID) {
			found = true
		}
	}
	if !found {
		t.Fatal("delayed alert delivered to previous handle")
	}
}
