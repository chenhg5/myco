package oscap

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/dbstudio"
)

func TestDatabaseCapabilityScopeCancelAndDefaultReadOnly(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sample.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE data(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO data VALUES(1,'before')"); err != nil {
		t.Fatal(err)
	}
	e := &Env{Home: home, Scope: "hub\x00page"}
	call := func(e *Env, name string, args any) (any, *Error) {
		raw, _ := json.Marshal(args)
		return Call(context.Background(), e, name, raw)
	}
	if _, err := call(&Env{Home: home}, "db.profile", map[string]string{"action": "list"}); err == nil {
		t.Fatal("accepted an unscoped profile call")
	}
	v, err2 := call(e, "db.profile", map[string]any{"action": "save", "profile": map[string]any{"name": "Test", "engine": "sqlite", "path": path}})
	if err2 != nil {
		t.Fatal(err2)
	}
	p := v.(dbstudio.Profile)
	if !p.ReadOnly {
		t.Fatal("read_only must default to true")
	}
	other := *e
	other.Scope = "hub\x00other"
	if _, err := call(&other, "db.rows", map[string]any{"connection": p.ID, "table": "data", "action": "browse"}); err == nil {
		t.Fatal("connection crossed a page boundary")
	}
	if _, err := call(e, "db.query", map[string]any{"connection": p.ID, "mode": "execute", "sql": "DELETE FROM data"}); err == nil {
		t.Fatal("default connection allowed writes")
	}
	done := make(chan *Error, 1)
	go func() {
		_, err := call(e, "db.query", map[string]any{"connection": p.ID, "request_id": "long", "mode": "query", "sql": "WITH RECURSIVE x(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM x) SELECT SUM(n) FROM x"})
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		databaseRuns.Lock()
		n := len(databaseRuns.active)
		databaseRuns.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("query never became active")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := call(e, "db.cancel", map[string]string{"request_id": "long"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || err.Code != CodeCancelled {
			t.Fatal("cancel did not stop query", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel stalled")
	}
	databaseRuns.Lock()
	n := len(databaseRuns.active)
	databaseRuns.Unlock()
	if n != 0 {
		t.Fatal("cancel left a registered request")
	}
	call(e, "db.cancel", map[string]string{"request_id": "early"})
	if _, err := call(e, "db.query", map[string]any{"connection": p.ID, "request_id": "early", "mode": "query", "sql": "SELECT 1"}); err == nil || err.Code != CodeCancelled {
		t.Fatal("early cancellation was lost", err)
	}
}
