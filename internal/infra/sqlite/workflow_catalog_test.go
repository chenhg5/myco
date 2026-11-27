package sqlite_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestWorkflowCatalogCountsUseCoveringIndexAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a database created before the catalog index was added.
	if _, err = db.Exec(`DROP INDEX idx_workflow_runs_counts`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT status, COUNT(*) FROM workflow_runs WHERE workflow_id=? GROUP BY status`, "wf_demo")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "COVERING INDEX idx_workflow_runs_counts") {
		t.Fatalf("catalog counts must not read run payloads: %s", plan)
	}
}
