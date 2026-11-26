package sqlite

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/perf"
	"github.com/myco-ai/myco/internal/requestid"
)

// Statements are named after the repository method that ran them; a slow
// one is logged with its name, duration and rows, never its parameters.
func TestTimedStatementsAreNamedAndLogged(t *testing.T) {
	var log bytes.Buffer
	rec := perf.New()
	db, err := OpenTimed(":memory:", Timing{Log: slog.New(slog.NewTextHandler(&log, nil)), Slow: time.Nanosecond, Perf: rec})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	agents := NewAgentRepo(db)
	for _, name := range []string{"alpha", "secret-name-xyz"} {
		if err := agents.Create(ctx, &domain.Agent{ID: "id-" + name, Name: domain.AgentName(name), TokenHash: "hash-" + name, Delivery: domain.Delivery{Mode: domain.DeliveryPull},
			CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := agents.Get(ctx, "secret-name-xyz"); err != nil {
		t.Fatal(err)
	}
	// (a statement run for a request names it: X-Request-Id on its context)
	list, err := agents.List(requestid.With(ctx, "req-list-1"))
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v %v", list, err)
	}
	if _, err := db.ExecContext(ctx, `SELECT 1`); err != nil { // (no repository method: "sqlite")
		t.Fatal(err)
	}

	byName := map[string]perf.Stat{}
	for _, s := range rec.Report(time.Minute, 0) {
		byName[s.Name] = s
	}
	find := func(prefix string) perf.Stat {
		t.Helper()
		for name, s := range byName {
			if strings.HasPrefix(name, prefix) {
				return s
			}
		}
		t.Fatalf("no statement named %s*: %v", prefix, byName)
		return perf.Stat{}
	}
	if s := find("AgentRepo.Create: INSERT agents #"); s.Count != 2 || s.MaxRows != 1 {
		t.Fatalf("create: %+v", s)
	}
	if s := find("AgentRepo.List: SELECT agents #"); s.Count != 1 || s.MaxRows != 2 {
		t.Fatalf("list counts its rows: %+v", s)
	}
	find("AgentRepo.Get: SELECT agents #")
	find("sqlite: SELECT #")

	out := log.String()
	if !strings.Contains(out, `msg="slow statement" name="AgentRepo.List: SELECT agents #`) || !strings.Contains(out, "rows=2 id=req-list-1") {
		t.Fatalf("log: %s", out)
	}
	for _, secret := range []string{"secret-name-xyz", "hash-alpha"} {
		if strings.Contains(out, secret) {
			t.Fatalf("a parameter value was logged: %s", out)
		}
	}
	if summary(`SELECT id FROM messages WHERE id IN (?, ?)`) != summary(`SELECT id  FROM messages WHERE id IN (?,?,?)`) {
		t.Fatal("placeholder lists are folded")
	}
}
