package loadtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/myco-ai/myco/internal/infra/sqlite"
	"strings"
	"testing"
	"time"
)

// The same executable/pipe protocol used by the standalone command, including
// cleanup on parent EOF. No mock Hub or global user configuration is involved.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "serve-child" {
		if e := serveChild(context.Background(), os.Stdin, os.Stdout, os.Stderr); e != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func testConfig(out string) Config {
	return Config{Users: []int{3}, Repeats: 1, Devices: 2, Agents: 5, SeedMessages: 30, Warmup: 100 * time.Millisecond, Duration: time.Second, Cooldown: 5 * time.Second, TaskEvery: 2 * time.Second, TaskWork: 0, AppEvery: 500 * time.Millisecond, RuntimeEvery: time.Second, RequestTimeout: 5 * time.Second, Workers: 8, Queue: 100, Seed: 7, Output: out}
}

func TestIsolatedRunUsesMemberDevicesAndValidatesTaskReplies(t *testing.T) {
	root := filepath.Join(t.TempDir(), "new-run")
	c := testConfig(root)
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	if e := Run(ctx, c, &output); e != nil && !errors.Is(e, errAcceptance) {
		data, _ := os.ReadFile(filepath.Join(root, "report.json"))
		t.Fatalf("run: %v; report=%s; progress=%s", e, data, output.String())
	}
	b, e := os.ReadFile(filepath.Join(root, "report.json"))
	if e != nil {
		t.Fatal(e)
	}
	var r []Report
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	if len(r) != 1 || (r[0].Status != "completed" && len(r[0].SLOFailures) == 0) || r[0].StopReason != "" || !r[0].ProbesPassed || r[0].Tasks.Scheduled == 0 || r[0].Tasks.Completed != r[0].Tasks.Scheduled || r[0].Tasks.DuplicateClaims != 0 || r[0].DBBytes == 0 {
		t.Fatalf("report: %+v", r)
	}
	for _, op := range r[0].Operations {
		if len(op.Errors) > 0 {
			t.Fatalf("operation failed: %+v", op)
		}
	}
	// This is a functional regression under race instrumentation, not a capacity
	// assertion about a shared CI host. SLOs are checked separately and reported.
	if len(r[0].TasksByPhase) == 0 {
		t.Fatal("missing phase task counts")
	}
	dir := filepath.Join(root, "users-003-run-01")
	for _, name := range []string{"hub.log", "samples.jsonl", "report.json", "progress.json"} {
		data, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			t.Fatal(e)
		}
		if regexp.MustCompile(`myco_(?:console_|enroll_|pair_)?[a-z2-7]{20,}`).Match(data) || strings.Contains(string(data), "Bearer ") {
			t.Fatalf("credential leaked to %s", name)
		}
	}
	// Historical data must exercise nonempty node pairs, large private bodies,
	// and valid roots, rather than benchmarking only empty result branches.
	db, err := sqlite.Open(filepath.Join(dir, "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`SELECT count(*) FROM messages WHERE id LIKE 'msg_seed_%' AND from_agent NOT LIKE '@console:%'`,
		`SELECT count(*) FROM messages WHERE project='u001' AND length(body)>16384`,
	} {
		var n int
		if e := db.QueryRow(query).Scan(&n); e != nil || n == 0 {
			t.Fatalf("fixture branch missing: count=%d err=%v", n, e)
		}
	}
	var orphan int
	if e := db.QueryRow(`SELECT count(*) FROM messages m LEFT JOIN messages root ON root.id=m.thread_id WHERE root.id IS NULL`).Scan(&orphan); e != nil || orphan != 0 {
		t.Fatalf("invalid thread roots: %d %v", orphan, e)
	}
	// The directory is not reused and is left intact for inspection.
	if e = Run(ctx, c, &output); e == nil {
		t.Fatal("existing directory reused")
	}
}
func TestSchedulerCountsDropsInsteadOfReducingArrivalRate(t *testing.T) {
	c := testConfig("unused")
	c.Warmup = 0
	c.Duration = 40 * time.Millisecond
	c.AppEvery = 2 * time.Millisecond
	c.TaskEvery = 5 * time.Millisecond
	c.RuntimeEvery = 5 * time.Millisecond
	f := &fixture{people: []*person{{name: "demo"}}, bots: []*bot{{name: "demo-bot", user: 0}}}
	w := &workload{config: c, f: f, start: time.Now(), tasks: map[string]*task{}}
	jobs := make(chan job, 1)
	w.schedule(t.Context(), jobs)
	if w.scheduled.Load() < 20 || w.admitted.Load() != 1 || w.dropped.Load() != w.scheduled.Load()-1 {
		t.Fatalf("scheduled=%d admitted=%d dropped=%d", w.scheduled.Load(), w.admitted.Load(), w.dropped.Load())
	}
	if w.summary.Failed == 0 {
		t.Fatal("dropped planned tasks disappeared")
	}
}
func TestHistogramKeepsErrorsAndBoundsPercentiles(t *testing.T) {
	var s statistics
	for i := 1; i <= 100; i++ {
		s.record("steady", "GET demo", time.Millisecond, time.Duration(i)*time.Millisecond, nil, false)
	}
	s.record("steady", "GET demo", 0, 200*time.Millisecond, context.DeadlineExceeded, false)
	s.record("steady", "poll", -1, time.Second, context.Canceled, true)
	a := s.take()
	if len(a) != 2 {
		t.Fatal(a)
	}
	for _, v := range a {
		if v.Name == "GET demo" {
			if v.Latency.Count != 101 || v.Latency.P95 < 95 || v.Latency.P95 > 106 || v.Errors["deadline"] != 1 {
				t.Fatal(v)
			}
		} else if v.Canceled != 1 || len(v.Errors) != 0 {
			t.Fatal(v)
		}
	}
	if len(s.snapshot()) != 0 {
		t.Fatal("interval did not reset")
	}
}
func TestConfigurationRejectsTargetsAndUnsafeOutput(t *testing.T) {
	var b bytes.Buffer
	if e := Main(t.Context(), []string{"--target", "http://example.com"}, strings.NewReader(""), &b, &b); e == nil {
		t.Fatal("external target flag accepted")
	}
	for _, bad := range []func(*Config){func(c *Config) { c.Duration = 0 }, func(c *Config) { c.Users = []int{0} }, func(c *Config) { c.Queue = 0 }, func(c *Config) { c.TaskEvery = 0 }, func(c *Config) { c.Repeats = 0 }} {
		c := testConfig("demo")
		bad(&c)
		if c.validate() == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	p := filepath.Join(t.TempDir(), "occupied")
	if e := os.WriteFile(p, []byte("retain"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := newRoot(p); e == nil {
		t.Fatal("existing file accepted")
	}
	data, _ := os.ReadFile(p)
	if string(data) != "retain" {
		t.Fatal("existing data changed")
	}
	if e := serveChild(t.Context(), strings.NewReader(`{"Dir":"missing","Marker":"x","Admin":"demo"}`), &b, &b); e == nil {
		t.Fatal("missing marker accepted")
	}
}
func TestAcceptanceDoesNotCountIntentionalWaitAsSlowRequest(t *testing.T) {
	a := []Operation{{Phase: "steady", Name: "POST /v1/inbox/pull [intentional wait]", Latency: Latency{Count: 100, P95: 30000, P99: 30000}}, {Phase: "warmup", Name: "GET /v1/overview", Latency: Latency{Count: 100, P95: 3000}}}
	if v := assess(a); len(v) != 0 {
		t.Fatal(v)
	}
	a = append(a, Operation{Phase: "steady", Name: "GET /v1/overview", Latency: Latency{Count: 100, P95: 1000}})
	if len(assess(a)) != 1 {
		t.Fatal("slow list was accepted")
	}
	if !errors.Is(safeErr("stage", context.Canceled), context.Canceled) {
		t.Fatal("cancellation lost")
	}
}
