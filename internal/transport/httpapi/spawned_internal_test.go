package httpapi

import (
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

// A machine's answer to a spawn names the new agent; the hub records a
// provider on it only when it really is the one just made there.
func TestASpawnAnswerIsNotTakenOnItsWord(t *testing.T) {
	since := time.Date(2026, 11, 27, 1, 0, 0, 0, time.UTC)
	fresh := &domain.Agent{Name: "wsl-mm", CreatedAt: since.Add(2 * time.Second), Owner: "mem_glenn", Binding: domain.Binding{SessionID: "cl-1"}}
	res := api.SpawnResult{Agent: "wsl-mm", SessionID: "cl-1"}
	if !spawnedHere(fresh, res, since, "mem_glenn") {
		t.Fatal("the agent just made is refused")
	}
	older := *fresh
	older.CreatedAt = since.Add(-time.Hour)
	other := *fresh
	other.Binding.SessionID = "someone-elses"
	theirs := *fresh
	theirs.Owner = "mem_ana"
	for name, a := range map[string]*domain.Agent{"an older agent": &older, "another session's": &other, "another member's": &theirs} {
		if spawnedHere(a, res, since, "mem_glenn") {
			t.Errorf("%s is taken for the new one", name)
		}
	}
	if spawnedHere(fresh, api.SpawnResult{Agent: "wsl-mm"}, since, "mem_glenn") {
		t.Error("an answer without its session is taken")
	}
	if spawnedHere(fresh, api.SpawnResult{Agent: "wsl-other", SessionID: "cl-1"}, since, "mem_glenn") {
		t.Error("an answer naming another agent is taken")
	}
}
