package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
)

// The hub's owner makes and downloads a backup; nobody else can. Put back
// with `myco hub restore`, it brings the hub to where it was, and keeps
// what was there aside.
func TestHubBackupAndRestore(t *testing.T) {
	c := ctx(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "data", "hub.db")
	os.MkdirAll(filepath.Dir(db), 0o700)
	h := startHub(t, db, app.DefaultMessagePolicy())
	owner := h.admin()
	if _, err := owner.CreateProject(c, api.CreateProjectRequest{Name: "before", Goal: "kept"}); err != nil {
		t.Fatal(err)
	}
	enr, err := owner.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "worker", MaxUses: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: "worker", Binding: api.Binding{Harness: "claude", SessionID: "s-w"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	agent := h.client(res.Token)

	b, err := owner.CreateBackup(c)
	if err != nil || !strings.HasPrefix(b.Name, "myco-backup-hub-") || b.Size == 0 {
		t.Fatalf("backup: %v %+v", err, b)
	}
	list, err := owner.Backups(c)
	if err != nil || len(list.Backups) != 1 || list.Keep != 7 || !strings.Contains(list.Restore, "myco hub restore") {
		t.Fatalf("list: %v %+v", err, list)
	}
	if _, err := agent.CreateBackup(c); err == nil {
		t.Fatal("an agent made a backup")
	}
	if _, err := agent.DownloadBackup(c, b.Name, t.TempDir()); err == nil {
		t.Fatal("an agent downloaded a backup")
	}
	if _, err := owner.DownloadBackup(c, "../hub.db", t.TempDir()); err == nil {
		t.Fatal("a path outside the backups was served")
	}
	saved, err := owner.DownloadBackup(c, b.Name, filepath.Join(dir, "mine"))
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(saved); err != nil || st.Size() != b.Size {
		t.Fatalf("downloaded: %v", err)
	}

	// Things change after the backup; then it is put back.
	if _, err := owner.CreateProject(c, api.CreateProjectRequest{Name: "after"}); err != nil {
		t.Fatal(err)
	}
	h.stop()
	cl := newCLI(t, h, filepath.Join(dir, "cli.yaml"))
	if code, out, errb := cl.run(c, "hub", "restore", "--from", saved, "--db", db); code == 0 {
		t.Fatalf("restore without the hub's service or --yes went ahead: %s%s", out, errb)
	}
	if code, out, _ := cl.run(c, "hub", "restore", "--from", saved, "--db", db, "--dry-run"); code != 10 || !strings.Contains(out, `"hub_restore"`) {
		t.Fatalf("dry run: %d %s", code, out)
	}
	out := cl.must(c, "hub", "restore", "--from", saved, "--db", db, "--yes")
	if !strings.Contains(out, "before-restore-") {
		t.Fatalf("restore: %s", out)
	}
	h2 := startHub(t, db, app.DefaultMessagePolicy())
	ps, err := h2.admin().ListProjects(c, true)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, p := range ps {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "before" {
		t.Fatalf("projects after the restore: %v", names)
	}
	// The worker's credential from before the backup still works.
	if _, err := h2.client(res.Token).Me(c); err != nil {
		t.Fatalf("the worker after the restore: %v", err)
	}

	// The CLI on the server backs up the whole server.
	out = cl.must(c, "hub", "backup", "--db", db, "--out", filepath.Join(dir, "server.tar.gz"))
	if !strings.Contains(out, "server.tar.gz") {
		t.Fatalf("server backup: %s", out)
	}
}
