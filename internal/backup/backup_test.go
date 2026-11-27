package backup

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/infra/hubkey"
)

func makeHub(t *testing.T, dir, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "files", "ab"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "hub.db")+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{"CREATE TABLE t(v TEXT)", "INSERT INTO t VALUES ('" + value + "')"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "files", "ab", "blob1"), []byte("file of "+value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func valueOf(t *testing.T, db string) string {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+db+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var v string
	if err := d.QueryRow("SELECT v FROM t").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// A server with a workspace is backed up while its databases are open,
// and restored over changed data, which is kept aside.
func TestBackupAndRestoreServer(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	makeHub(t, data, "root")
	makeHub(t, filepath.Join(data, "workspaces", "team"), "team")
	// An open writer (the running hub) does not stop the snapshot.
	live, err := sql.Open("sqlite", "file:"+filepath.Join(data, "hub.db")+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if _, err := live.Exec("INSERT INTO t VALUES ('later')"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC)
	info, err := Write(ctx, filepath.Join(data, "backups"), Sources(filepath.Join(data, "hub.db"), "", true), "server", "v1", now)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "myco-backup-server-20261121-093000.tar.gz" || info.Size == 0 {
		t.Fatalf("backup: %+v", info)
	}
	// The data changes after the backup.
	if _, err := live.Exec("UPDATE t SET v='changed'"); err != nil {
		t.Fatal(err)
	}
	live.Close()
	os.WriteFile(filepath.Join(data, "files", "ab", "blob1"), []byte("changed"), 0o600)

	f, err := os.Open(filepath.Join(data, "backups", info.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tmp, _ := os.MkdirTemp(data, ".restore-")
	m, err := Extract(ctx, f, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if m.Scope != "server" || len(m.Hubs) != 2 || m.Files != 4 {
		t.Fatalf("manifest: %+v", m)
	}
	aside, err := Restore(tmp, data, m, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(aside) != 2 {
		t.Fatalf("kept aside: %v", aside)
	}
	if v := valueOf(t, filepath.Join(data, "hub.db")); v != "root" {
		t.Fatalf("restored root: %q", v)
	}
	if v := valueOf(t, filepath.Join(data, "workspaces", "team", "hub.db")); v != "team" {
		t.Fatalf("restored workspace: %q", v)
	}
	if b, _ := os.ReadFile(filepath.Join(data, "files", "ab", "blob1")); string(b) != "file of root" {
		t.Fatalf("restored file: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(aside[0], "files", "ab", "blob1")); string(b) != "changed" {
		t.Fatalf("what was there is kept: %q", b)
	}
}

// A damaged or foreign archive is refused before anything is written
// outside the extraction directory.
func TestExtractRefusesDamage(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	makeHub(t, data, "root")
	var buf bytes.Buffer
	if _, err := Create(ctx, &buf, Sources(filepath.Join(data, "hub.db"), "", false), "hub", "v1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(ctx, bytes.NewReader(buf.Bytes()), t.TempDir()); err != nil {
		t.Fatalf("a good backup: %v", err)
	}
	if _, err := Extract(ctx, strings.NewReader("not a backup"), t.TempDir()); err == nil {
		t.Fatal("garbage was accepted")
	}
	b := buf.Bytes()
	cut := b[:len(b)/2]
	if _, err := Extract(ctx, bytes.NewReader(cut), t.TempDir()); err == nil {
		t.Fatal("a cut backup was accepted")
	}
}

func TestPruneKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, FileName("hub", time.Unix(int64(1000+i), 0)))
		os.WriteFile(p, []byte("x"), 0o600)
		os.Chtimes(p, time.Unix(int64(1000+i), 0), time.Unix(int64(1000+i), 0))
	}
	os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x"), 0o600)
	if err := Prune(dir, 2); err != nil {
		t.Fatal(err)
	}
	l, _ := List(dir)
	if len(l) != 2 || !strings.Contains(l[0].Name, "19700101-001644") {
		t.Fatalf("kept: %+v", l)
	}
	if _, err := os.Stat(filepath.Join(dir, "other.txt")); err != nil {
		t.Fatal("pruned a file that is not a backup")
	}
	if ValidName("../x.tar.gz") || ValidName("myco-backup-a/b.tar.gz") || !ValidName(l[0].Name) {
		t.Fatal("ValidName")
	}
}

// A workspace's own key (i119) goes into its backup and comes back with
// it, for its user only; the one there before is kept aside.
func TestTheHubsKeyIsBackedUpAndRestored(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	team := filepath.Join(data, "workspaces", "team")
	makeHub(t, data, "root")
	makeHub(t, team, "team")
	key := filepath.Join(team, hubkey.FileName)
	if err := os.WriteFile(key, []byte(strings.Repeat("k", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 27, 9, 30, 0, 0, time.UTC)
	info, err := Write(ctx, filepath.Join(data, "backups"), Sources(filepath.Join(data, "hub.db"), "", true), "server", "v1", now)
	if err != nil {
		t.Fatal(err)
	}
	// a new key replaced it since (a hub moved without its key)
	if err := os.WriteFile(key, []byte(strings.Repeat("n", 32)), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(data, "backups", info.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tmp, _ := os.MkdirTemp(data, ".restore-")
	m, err := Extract(ctx, f, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.SHA256["workspaces/team/"+hubkey.FileName]; !ok {
		t.Fatalf("the key is not in the backup: %v", m.SHA256)
	}
	if _, ok := m.SHA256[hubkey.FileName]; ok {
		t.Fatal("a key the root hub never had")
	}
	aside, err := Restore(tmp, data, m, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(key)
	if b, _ := os.ReadFile(key); err != nil || string(b) != strings.Repeat("k", 32) || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
		t.Fatalf("the restored key: %q %v", b, err)
	}
	kept := ""
	for _, d := range aside {
		if b, err := os.ReadFile(filepath.Join(d, hubkey.FileName)); err == nil {
			kept = string(b)
		}
	}
	if kept != strings.Repeat("n", 32) {
		t.Fatalf("the key that was there is not kept aside: %q", kept)
	}
}
