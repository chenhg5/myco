package sqlite_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestAgentForkMigrationAndPrivateProvenanceSurviveReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hub.db")
	db, err := sqlite.Open(path)
	mustNoErr(t, err)
	agent := newAgent(t, "source", "source-hash", domain.AgentSpec{})
	mustNoErr(t, sqlite.NewAgentRepo(db).Create(ctx, agent))
	// An existing Hub has all its ordinary tables, but no managed-copy table.
	_, err = db.Exec(`DROP TABLE agent_forks`)
	mustNoErr(t, err)
	mustNoErr(t, db.Close())
	db, err = sqlite.Open(path)
	mustNoErr(t, err)
	f := &domain.AgentFork{ID: "fork-demo", Name: "copy", SourceID: agent.ID, ForkOf: "source", CreatorID: "agent:creator-identity", ForkedBy: "lead", Batch: "one", MachineID: "machine", NodeID: "node", State: "planned", CreatedAt: t0}
	_, err = sqlite.NewForkRepo(db).CreateBatch(ctx, []*domain.AgentFork{f}, 1)
	mustNoErr(t, err)
	mustNoErr(t, db.Close())
	db, err = sqlite.Open(path)
	mustNoErr(t, err)
	defer db.Close()
	got, err := sqlite.NewForkRepo(db).Get(ctx, f.ID)
	mustNoErr(t, err)
	if got.CreatorID != f.CreatorID || got.SourceID != agent.ID {
		t.Fatal("trusted identities were lost on restart")
	}
	b, err := json.Marshal(got)
	mustNoErr(t, err)
	if strings.Contains(string(b), f.CreatorID) {
		t.Fatal("private creator ID leaked into public JSON")
	}
	_, err = sqlite.NewAgentRepo(db).Get(ctx, agent.Name)
	mustNoErr(t, err)
}
