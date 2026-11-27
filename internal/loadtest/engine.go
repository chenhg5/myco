package loadtest

import (
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/infra/security"
)

var errAcceptance = errors.New("one or more load runs failed acceptance; see report.json")

type Report struct {
	ArrivalsStarted    time.Time `json:"arrivals_started_at"`
	SteadyStarted      time.Time `json:"steady_started_at"`
	PlannedArrivalsEnd time.Time `json:"planned_arrivals_end_at"`
	ArrivalsStopped    time.Time `json:"arrivals_stopped_at"`

	Users        int                    `json:"users"`
	Repeat       int                    `json:"repeat"`
	Config       Config                 `json:"config"`
	Started      time.Time              `json:"started_at"`
	Ended        time.Time              `json:"ended_at"`
	Status       string                 `json:"status"`
	StopReason   string                 `json:"stop_reason,omitempty"`
	Scheduled    uint64                 `json:"scheduled_operations"`
	Admitted     uint64                 `json:"admitted_operations"`
	Dropped      uint64                 `json:"dropped_operations"`
	Tasks        TaskSummary            `json:"tasks"`
	TasksByPhase map[string]TaskSummary `json:"tasks_by_phase"`
	SLOFailures  []string               `json:"slo_failures"`
	PeakWALBytes int64                  `json:"peak_wal_bytes"`
	Operations   []Operation            `json:"operations"`
	ProbesPassed bool                   `json:"permission_probes_passed"`
	DBBytes      int64                  `json:"db_bytes"`
	WALBytes     int64                  `json:"wal_bytes"`
	Limitations  []string               `json:"limitations"`
}
type TaskSummary struct {
	Scheduled        uint64 `json:"scheduled"`
	Sent             uint64 `json:"send_attempted"`
	Received         uint64 `json:"received"`
	Completed        uint64 `json:"completed"`
	Failed           uint64 `json:"failed"`
	Unfinished       uint64 `json:"unfinished"`
	DuplicateClaims  uint64 `json:"duplicate_claims"`
	ValidationErrors uint64 `json:"validation_errors"`
}
type validationError string

func (e validationError) Error() string { return string(e) }

type task struct {
	key, phase, target, from    string
	scheduled, sent             time.Time
	trace                       bool
	received, completed, failed bool
}
type job struct {
	due        time.Time
	kind       string
	index, seq int
	phase      string
	task       *task
}
type stream struct {
	due        time.Time
	every      time.Duration
	kind       string
	index, seq int
}
type streams []*stream

func (s streams) Len() int           { return len(s) }
func (s streams) Less(i, j int) bool { return s[i].due.Before(s[j].due) }
func (s streams) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s *streams) Push(v any)        { *s = append(*s, v.(*stream)) }
func (s *streams) Pop() any          { a := *s; v := a[len(a)-1]; *s = a[:len(a)-1]; return v }

type workload struct {
	config                       Config
	f                            *fixture
	start                        time.Time
	stats, interval              statistics
	mu                           sync.Mutex
	tasks                        map[string]*task
	summary                      TaskSummary
	scheduled, admitted, dropped atomic.Uint64
	activePolls                  atomic.Int64
	peakWAL                      atomic.Int64
	stopMu                       sync.Mutex
	reason                       string
	stopArrivals                 context.CancelFunc
}

func (w *workload) phase(t time.Time) string {
	if t.Before(w.start.Add(w.config.Warmup)) {
		return "warmup"
	}
	if t.Before(w.start.Add(w.config.Warmup + w.config.Duration)) {
		return "steady"
	}
	return "drain"
}
func (w *workload) record(phase, name string, lag, elapsed time.Duration, e error, shutdown bool) {
	w.stats.record(phase, name, lag, elapsed, e, shutdown)
	w.interval.record(phase, name, lag, elapsed, e, shutdown)
}
func (w *workload) call(ctx context.Context, phase, name string, fn func(context.Context) error) error {
	c, cancel := context.WithTimeout(ctx, w.config.RequestTimeout)
	defer cancel()
	at := time.Now()
	e := fn(c)
	var ve validationError
	if errors.As(e, &ve) {
		w.mu.Lock()
		w.summary.ValidationErrors++
		w.mu.Unlock()
	}
	w.record(phase, name, -1, time.Since(at), e, ctx.Err() != nil)
	return e
}
func (w *workload) stop(reason string) {
	w.stopMu.Lock()
	defer w.stopMu.Unlock()
	if w.reason == "" {
		w.reason = reason
		w.stopArrivals()
	}
}

func Run(ctx context.Context, c Config, out io.Writer) error {
	if e := c.validate(); e != nil {
		return e
	}
	root, e := newRoot(c.Output)
	if e != nil {
		return e
	}
	// This process only calls its own child. Disable configured HTTP proxies.
	client.SetProxy(func(*http.Request) (*url.URL, error) { return nil, nil })
	manifest := map[string]any{"config": c, "started_at": time.Now().UTC(), "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(), "gomaxprocs": runtime.GOMAXPROCS(0), "topology": "one workspace; separate local child Hub; same host generator", "percentiles": "log histogram upper bounds, 10% resolution; max exact", "limitations": []string{"not a production capacity claim", "no TLS/proxy or multiple workspaces", "no LLM calls or remote execution", "workload assumptions must be calibrated"}}
	if b, ok := debug.ReadBuildInfo(); ok {
		manifest["build_settings"] = b.Settings
	}
	if e = writeJSON(filepath.Join(root, "manifest.json"), manifest); e != nil {
		return e
	}
	var reports []Report
	var failed bool
	for _, users := range c.Users {
		for repeat := 1; repeat <= c.Repeats; repeat++ {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			dir := filepath.Join(root, fmt.Sprintf("users-%03d-run-%02d", users, repeat))
			if e = os.Mkdir(dir, 0700); e != nil {
				return e
			}
			_ = json.NewEncoder(out).Encode(map[string]any{"event": "start", "users": users, "repeat": repeat, "at": time.Now().UTC()})
			r, err := runOne(ctx, c, dir, users, repeat)
			reports = append(reports, r)
			if e = writeJSON(filepath.Join(root, "report.json"), reports); e != nil {
				return e
			}
			_ = json.NewEncoder(out).Encode(map[string]any{"event": "complete", "users": users, "repeat": repeat, "status": r.Status, "tasks": r.Tasks, "at": time.Now().UTC()})
			if err != nil {
				return err
			}
			if r.Status != "completed" {
				failed = true
			}
		}
	}
	if failed {
		return errAcceptance
	}
	return nil
}

func runOne(ctx context.Context, c Config, dir string, users, repeat int) (r Report, err error) {
	r = Report{Users: users, Repeat: repeat, Config: c, Started: time.Now().UTC(), Status: "setup_failed", Limitations: []string{"same-host local HTTP, not production capacity", "single workspace", "no attachments/workflows or synchronised restart storm in this version", "percentiles are approximate upper bounds; check sample counts"}}
	defer func() {
		r.Ended = time.Now().UTC()
		if err != nil && r.StopReason == "" {
			r.StopReason = err.Error()
		}
		r.DBBytes = fileBytes(filepath.Join(dir, "hub.db"))
		r.WALBytes = fileBytes(filepath.Join(dir, "hub.db-wal"))
		if e := writeJSON(filepath.Join(dir, "report.json"), r); e != nil && err == nil {
			err = e
		}
	}()
	tok, _, e := (security.Tokens{}).Issue()
	if e != nil {
		return r, e
	}
	marker := (security.IDs{}).New("load")
	if e = os.WriteFile(filepath.Join(dir, ".loadtest"), []byte(marker), 0600); e != nil {
		return r, e
	}
	cc := childConfig{Dir: dir, Marker: marker, Admin: tok}
	child, e := startChild(ctx, cc)
	if e != nil {
		return r, e
	}
	defer func() {
		if e := child.stop(); e != nil && err == nil {
			err = e
			r.Status = "shutdown_failed"
		}
	}()
	setup, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	f, e := prepare(setup, child.URL, tok, users)
	if e != nil {
		return r, e
	}
	if e = child.stop(); e != nil {
		return r, e
	}
	if e = seedHistory(ctx, dir, f, c.SeedMessages, c.Seed); e != nil {
		return r, safeErr("history fixture", e)
	}
	cc.Profile = c.Profile
	child, e = startChild(ctx, cc)
	if e != nil {
		return r, e
	}
	f.connect(child.URL)
	probe, cancelProbe := context.WithTimeout(ctx, 30*time.Second)
	e = probes(probe, f)
	cancelProbe()
	if e != nil {
		return r, e
	}
	w := &workload{config: c, f: f, start: time.Now(), tasks: map[string]*task{}}
	r.ArrivalsStarted = w.start.UTC()
	r.SteadyStarted = w.start.Add(c.Warmup).UTC()
	r.PlannedArrivalsEnd = w.start.Add(c.Warmup + c.Duration).UTC()
	arrivals, stopArrivals := context.WithCancel(ctx)
	defer stopArrivals()
	w.stopArrivals = stopArrivals
	runCtx, stopRun := context.WithCancel(ctx)
	defer stopRun()
	var bots sync.WaitGroup
	for _, b := range f.bots {
		bots.Go(func() { w.poll(runCtx, b) })
	}
	observeCtx, stopObserve := context.WithCancel(ctx)
	var observers sync.WaitGroup
	observers.Go(func() { w.observe(observeCtx, dir, child.cmd.Process.Pid) })
	jobs := make(chan job, c.Queue)
	var workers sync.WaitGroup
	for i := 0; i < c.Workers; i++ {
		workers.Go(func() {
			for j := range jobs {
				w.execute(runCtx, j)
			}
		})
	}
	w.schedule(arrivals, jobs)
	r.ArrivalsStopped = time.Now().UTC()
	close(jobs)
	workersDone := make(chan struct{})
	go func() { workers.Wait(); close(workersDone) }()
	drainDeadline := time.NewTimer(c.Cooldown)
	defer drainDeadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	draining := true
	for draining {
		select {
		case <-ctx.Done():
			draining = false
		case <-drainDeadline.C:
			w.stop("drain_deadline")
			draining = false
		case <-ticker.C:
			w.mu.Lock()
			pending := uint64(0)
			for _, t := range w.tasks {
				if !t.completed && !t.failed {
					pending++
				}
			}
			w.mu.Unlock()
			select {
			case <-workersDone:
				if pending == 0 {
					draining = false
				}
			default:
			}
		}
	}
	stopRun()
	workers.Wait()
	bots.Wait()
	stopObserve()
	observers.Wait()
	final, cancelFinal := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFinal()
	if e = probes(final, f); e == nil {
		r.ProbesPassed = true
	} else {
		w.stop("final_permission_probe_failed")
	}
	w.mu.Lock()
	r.Tasks = w.summary
	r.TasksByPhase = map[string]TaskSummary{}
	for _, t := range w.tasks {
		v := r.TasksByPhase[t.phase]
		v.Scheduled++
		if !t.sent.IsZero() {
			v.Sent++
		}
		if t.received {
			v.Received++
		}
		if t.completed {
			v.Completed++
		}
		if t.failed {
			v.Failed++
		}
		if !t.completed && !t.failed {
			r.Tasks.Unfinished++
			v.Unfinished++
		}
		r.TasksByPhase[t.phase] = v
	}
	w.mu.Unlock()
	r.Operations = w.stats.snapshot()
	r.Scheduled = w.scheduled.Load()
	r.Admitted = w.admitted.Load()
	r.Dropped = w.dropped.Load()
	r.PeakWALBytes = w.peakWAL.Load()
	r.SLOFailures = assess(r.Operations)
	w.stopMu.Lock()
	r.StopReason = w.reason
	w.stopMu.Unlock()
	r.Status = "completed"
	if ctx.Err() != nil {
		r.Status = "interrupted"
		err = ctx.Err()
	} else if r.StopReason != "" || r.Dropped > 0 || r.Tasks.Failed > 0 || r.Tasks.Unfinished > 0 || r.Tasks.ValidationErrors > 0 || r.Tasks.DuplicateClaims > 0 || len(r.SLOFailures) > 0 {
		r.Status = "failed"
	}
	// Request failures cannot be hidden by successful task retries or a quiet tail.
	for _, op := range r.Operations {
		for _, n := range op.Errors {
			if n > 0 {
				r.Status = "failed"
			}
		}
	}
	return r, err
}
func fileBytes(p string) int64 {
	s, e := os.Stat(p)
	if e != nil {
		return 0
	}
	return s.Size()
}

func (w *workload) schedule(ctx context.Context, jobs chan<- job) {
	rng := rand.New(rand.NewPCG(uint64(w.config.Seed), 2))
	var q streams
	add := func(kind string, index int, every time.Duration) {
		q = append(q, &stream{kind: kind, index: index, every: every, due: w.start.Add(time.Duration(rng.Int64N(int64(every))))})
	}
	for i := range w.f.bots {
		add("runtime", i, w.config.RuntimeEvery)
		add("me", i, w.config.RuntimeEvery)
		if i%5 == 0 {
			add("task", i, w.config.TaskEvery)
		}
	}
	for i := range w.f.people {
		add("overview", i, w.config.AppEvery)
		add("recent", i, w.config.AppEvery)
	}
	heap.Init(&q)
	end := w.start.Add(w.config.Warmup + w.config.Duration)
	for len(q) > 0 {
		s := heap.Pop(&q).(*stream)
		if !s.due.Before(end) {
			sleep(ctx, max(0, time.Until(end)))
			return
		}
		timer := time.NewTimer(max(0, time.Until(s.due)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		j := job{due: s.due, kind: s.kind, index: s.index, seq: s.seq, phase: w.phase(s.due)}
		w.scheduled.Add(1)
		if j.kind == "task" {
			b := w.f.bots[j.index]
			j.task = &task{key: fmt.Sprintf("load-%d-%d", j.index, j.seq), phase: j.phase, target: b.name, from: w.f.people[b.user].name, scheduled: j.due, trace: j.seq%10 == 0}
			w.mu.Lock()
			w.summary.Scheduled++
			w.tasks[j.task.key] = j.task
			w.mu.Unlock()
		}
		select {
		case jobs <- j:
			w.admitted.Add(1)
		default:
			w.dropped.Add(1)
			w.record(j.phase, "scheduler_drop", 0, 0, errors.New("queue_full"), false)
			if j.task != nil {
				w.mu.Lock()
				j.task.failed = true
				w.summary.Failed++
				w.mu.Unlock()
			}
		}
		s.due = s.due.Add(s.every)
		s.seq++
		heap.Push(&q, s)
	}
}
func (w *workload) execute(ctx context.Context, j job) {
	w.record(j.phase, "scheduler_dispatch", time.Since(j.due), 0, nil, false)
	if j.kind == "task" {
		b := w.f.bots[j.index]
		c := w.f.people[b.user].devices[j.seq%2].c
		w.mu.Lock()
		j.task.sent = time.Now()
		w.summary.Sent++
		w.mu.Unlock()
		e := w.call(ctx, j.phase, "POST /v1/console/messages", func(ctx context.Context) error {
			r, e := c.ConsoleSend(ctx, api.SendRequest{To: b.name, Body: "demo task " + j.task.key, Project: "shared", IdempotencyKey: j.task.key})
			if e == nil && r.Message.To != b.name {
				return validationError("wrong_recipient")
			}
			return e
		})
		if e != nil {
			w.mu.Lock()
			if !j.task.received {
				j.task.failed = true
				w.summary.Failed++
			}
			w.mu.Unlock()
		}
		return
	}
	if j.kind == "runtime" || j.kind == "me" {
		b := w.f.bots[j.index]
		if j.kind == "runtime" {
			_ = w.call(ctx, j.phase, "PUT /v1/me/runtime", func(ctx context.Context) error {
				return b.auth.c.ReportRuntime(ctx, api.Runtime{CLI: "shell", Model: "demo", RunnerInstance: "loadtest"})
			})
		} else {
			_ = w.call(ctx, j.phase, "GET /v1/me", func(ctx context.Context) error { _, e := b.auth.c.Me(ctx); return e })
		}
		return
	}
	u := w.f.people[j.index]
	c := u.devices[0].c
	if j.kind == "overview" {
		_ = w.call(ctx, j.phase, "GET /v1/overview", func(ctx context.Context) error { _, e := c.Overview(ctx, 0); return e })
		return
	}
	group, side, project, scope := "", "", "", ""
	switch j.seq % 6 {
	case 1:
		group = "pair"
		side = "me"
	case 2:
		group = "pair"
		side = "others"
	case 3:
		project = "shared"
	case 4:
		project = u.name
	case 5:
		scope = "mine"
	}
	_ = w.call(ctx, j.phase, "GET /v1/console/recent ["+fmt.Sprint(j.seq%6)+"]", func(ctx context.Context) error {
		_, e := c.RecentGrouped(ctx, group, side, project, scope, "", "", 0, 40)
		return e
	})
}
func (w *workload) poll(ctx context.Context, b *bot) {
	for ctx.Err() == nil {
		at := time.Now()
		phase := w.phase(at)
		w.activePolls.Add(1)
		msgs, e := b.auth.c.PullTypes(ctx, 1, 30*time.Second, []string{"task"})
		w.activePolls.Add(-1)
		w.record(phase, "POST /v1/inbox/pull [intentional wait]", -1, time.Since(at), e, ctx.Err() != nil)
		if e != nil {
			if ctx.Err() != nil {
				return
			}
			if !sleep(ctx, time.Second) {
				return
			}
			continue
		}
		for _, m := range msgs {
			w.processTask(ctx, b, m)
		}
	}
}
func (w *workload) processTask(ctx context.Context, b *bot, m api.Message) {
	w.mu.Lock()
	t := w.tasks[m.IdempotencyKey]
	if t == nil {
		w.summary.ValidationErrors++
		w.mu.Unlock()
		return
	}
	if t.received {
		w.summary.DuplicateClaims++
		w.mu.Unlock()
		return
	}
	t.received = true
	w.summary.Received++
	valid := t.target == b.name && m.To == b.name && m.From == "@console:"+t.from && m.Type == "task"
	due, sent, phase := t.scheduled, t.sent, t.phase
	w.mu.Unlock()
	w.record(phase, "task scheduled-to-received", -1, time.Since(due), nil, false)
	if !sent.IsZero() {
		w.record(phase, "task send-to-received", -1, time.Since(sent), nil, false)
	}
	fail := func() { w.mu.Lock(); t.failed = true; w.summary.Failed++; w.mu.Unlock() }
	if !valid {
		w.mu.Lock()
		w.summary.ValidationErrors++
		w.mu.Unlock()
		fail()
		return
	}
	call := func(name string, fn func(context.Context) error) bool { return w.call(ctx, phase, name, fn) == nil }
	c := b.auth.c
	sender := w.f.people[b.user].devices[1].c
	if !call("POST /v1/messages/{id}/ack", func(ctx context.Context) error { return c.Ack(ctx, m.ID) }) {
		fail()
		return
	}
	for step := 0; step < 2; step++ {
		if !call("POST /v1/messages/{id}/progress", func(ctx context.Context) error { _, e := c.Progress(ctx, m.ID, "demo progress", false); return e }) {
			fail()
			return
		}
		if t.trace {
			events := []api.TraceEvent{{Seq: step*2 + 1, Kind: "text", Data: json.RawMessage(`{"text":"demo trace"}`)}, {Seq: step*2 + 2, Kind: "text", Data: json.RawMessage(`{"text":"demo trace"}`)}}
			if !call("POST /v1/messages/{id}/trace", func(ctx context.Context) error { return c.PostTrace(ctx, m.ID, events) }) {
				fail()
				return
			}
		}
		if !sleep(ctx, w.config.TaskWork/2) {
			fail()
			return
		}
	}
	if !call("POST /v1/messages/{id}/reply", func(ctx context.Context) error {
		r, e := c.Reply(ctx, m.ID, "demo result "+t.key, false)
		if e == nil && (r.ReplyTo != m.ID || r.From != b.name || r.To != m.From) {
			return validationError("reply_identity_mismatch")
		}
		return e
	}) {
		fail()
		return
	}
	if !call("GET /v1/messages/{id}", func(ctx context.Context) error {
		r, e := c.GetMessage(ctx, m.ID)
		if e == nil && r.Status != "done" {
			return validationError("task_not_done")
		}
		return e
	}) {
		fail()
		return
	}
	if !call("GET /v1/threads/{id}", func(ctx context.Context) error {
		rows, e := c.Thread(ctx, m.ThreadID)
		if e != nil {
			return e
		}
		n := 0
		for _, r := range rows {
			if r.ReplyTo == m.ID && r.Type == "reply" {
				n++
			}
		}
		if n != 1 {
			return validationError("reply_count_mismatch")
		}
		return nil
	}) {
		fail()
		return
	}
	if !call("GET /v1/console/messages/{id}/trace", func(ctx context.Context) error {
		r, e := sender.ConsoleTrace(ctx, m.ID, 0)
		if e == nil && (r.Status != "done" || (t.trace && len(r.Events) != 4)) {
			return validationError("trace_mismatch")
		}
		return e
	}) {
		fail()
		return
	}
	w.mu.Lock()
	t.completed = true
	w.summary.Completed++
	w.mu.Unlock()
	w.record(phase, "task scheduled-to-validated", -1, time.Since(due), nil, false)
}
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
