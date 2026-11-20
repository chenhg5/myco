package oscap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// Event is something a job sent while running: progress, a piece of output.
type Event struct {
	N    int    `json:"n"`
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Job is a capability running (fs.scan, exec.spawn).
type Job struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	mu      sync.Mutex
	events  []Event
	first   int // N of events[0] (older ones are dropped)
	next    int
	changed chan struct{}
	done    bool
	result  any
	err     *Error
	ended   time.Time
	cancel  context.CancelFunc
	input   chan []byte
}

// The most events a job keeps for the page to read; older ones are dropped
// (a page that reads slower than a command prints loses the start).
const maxEvents = 4000

func (j *Job) emit(kind string, data any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return
	}
	j.events = append(j.events, Event{N: j.next, Type: kind, Data: data})
	j.next++
	if len(j.events) > maxEvents {
		drop := len(j.events) - maxEvents
		j.events = append([]Event(nil), j.events[drop:]...)
		j.first += drop
	}
	close(j.changed)
	j.changed = make(chan struct{})
}

func (j *Job) finish(v any, err *Error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.done, j.result, j.err, j.ended = true, v, err, time.Now()
	close(j.changed)
	j.changed = make(chan struct{})
}

// Batch is what a page reads of a job: the events after the ones it has,
// and, once it has ended, its value or error.
type Batch struct {
	Events []Event `json:"events"`
	// Next is the N to ask from next time.
	Next int `json:"next"`
	// Dropped: events between what was asked and Events[0] are gone.
	Dropped bool   `json:"dropped,omitempty"`
	Done    bool   `json:"done"`
	Value   any    `json:"value,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

func (j *Job) batch(after int) (Batch, <-chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	b := Batch{Next: j.next, Done: j.done}
	from := after - j.first
	if from < 0 {
		b.Dropped = true
		from = 0
	}
	if from < len(j.events) {
		b.Events = append([]Event(nil), j.events[from:]...)
	}
	if j.done {
		b.Value, b.Error = j.result, j.err
	}
	return b, j.changed
}

// Jobs are the jobs running or recently ended.
type Jobs struct {
	Env *Env
	mu  sync.Mutex
	m   map[string]*Job
}

// The most jobs running at once, and how long an ended one is kept to be read.
const (
	maxRunning = 32
	keepEnded  = 5 * time.Minute
)

// Start starts a capability that runs as a job.
func (js *Jobs) Start(name string, args json.RawMessage) (*Job, *Error) {
	c, ok := byName[name]
	if !ok {
		return nil, errf(CodeInvalid, "no such capability: %q", name)
	}
	if !c.Job {
		return nil, errf(CodeInvalid, "%s is a single call: call it (myco.os.call)", name)
	}
	js.mu.Lock()
	if js.m == nil {
		js.m = map[string]*Job{}
	}
	running := 0
	for id, j := range js.m {
		j.mu.Lock()
		if j.done && time.Since(j.ended) > keepEnded {
			delete(js.m, id)
		} else if !j.done {
			running++
		}
		j.mu.Unlock()
	}
	if running >= maxRunning {
		js.mu.Unlock()
		return nil, errf(CodeTooLarge, "%d jobs are running: wait for one to end, or cancel one", running)
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	j := &Job{ID: "job_" + hex.EncodeToString(b), Name: name, changed: make(chan struct{}), cancel: cancel, input: make(chan []byte, 64)}
	js.m[j.ID] = j
	js.mu.Unlock()

	go func() {
		defer cancel()
		v, err := run(withInput(ctx, j.input), js.Env, c, args, j.emit)
		if err != nil {
			e := AsError(err)
			if ctx.Err() != nil && e.Code == CodeFailed {
				e = &Error{Code: CodeCancelled, Message: "cancelled"}
			}
			j.finish(nil, e)
			return
		}
		j.finish(v, nil)
	}()
	return j, nil
}

func (js *Jobs) get(id string) (*Job, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	j, ok := js.m[id]
	return j, ok
}

// Read waits up to wait for events after the n-th (or the job's end), and
// returns what there is.
func (js *Jobs) Read(ctx context.Context, id string, after int, wait time.Duration) (Batch, *Error) {
	j, ok := js.get(id)
	if !ok {
		return Batch{}, errf(CodeNotFound, "no such job: %s (ended over %s ago?)", id, keepEnded)
	}
	b, changed := j.batch(after)
	if len(b.Events) > 0 || b.Done || b.Dropped || wait <= 0 {
		return b, nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-changed:
	case <-t.C:
	case <-ctx.Done():
	}
	b, _ = j.batch(after)
	return b, nil
}

// Cancel stops a job. It ends with the code "cancelled" (or what it had).
func (js *Jobs) Cancel(id string) *Error {
	j, ok := js.get(id)
	if !ok {
		return errf(CodeNotFound, "no such job: %s", id)
	}
	j.cancel()
	return nil
}

// CancelAll stops every job (the App is closing).
func (js *Jobs) CancelAll() {
	js.mu.Lock()
	defer js.mu.Unlock()
	for _, j := range js.m {
		j.cancel()
	}
}

// Input gives a running job (exec.spawn) what to read on its standard
// input; nil closes it.
func (js *Jobs) Input(id string, data []byte) *Error {
	j, ok := js.get(id)
	if !ok {
		return errf(CodeNotFound, "no such job: %s", id)
	}
	j.mu.Lock()
	done := j.done
	j.mu.Unlock()
	if done {
		return errf(CodeInvalid, "the job has ended")
	}
	select {
	case j.input <- data:
		return nil
	case <-time.After(5 * time.Second):
		return errf(CodeTimeout, "the job is not reading its input")
	}
}

type inputKey struct{}

func withInput(ctx context.Context, ch chan []byte) context.Context {
	return context.WithValue(ctx, inputKey{}, ch)
}

// inputOf is the job's input (nil outside a job).
func inputOf(ctx context.Context) chan []byte {
	ch, _ := ctx.Value(inputKey{}).(chan []byte)
	return ch
}
