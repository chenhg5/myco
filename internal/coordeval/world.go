// Package coordeval is the coordination test: fixed scenarios that check
// how well a project coordinator (an agent) splits a person's goal among
// other agents and puts their results together. Each scenario runs against
// a real hub started in this process, with scripted fake agents that are
// slow, queued, go offline, answer wrongly or edit the same thing; a
// checker then reads the hub's records and scores the run (see
// coordinator-tools, section 7, and docs/coord-eval.md).
//
// The coordinator under test is either a real Claude Code or Codex session
// (RunCLI, `make coord-eval`; it spends tokens) or a scripted one
// (Baseline), which the regular tests use to check the harness itself.
package coordeval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

// Names in the world. Neutral on purpose: this is a public repository.
const (
	// Lead is the agent under test.
	Lead = "lead"
	// UserName is the person who sends the goals (a member, at the App).
	UserName = "tester"
	// User is how the person is addressed.
	User = app.ConsoleSenderPrefix + UserName
	// OtherTeam is an agent outside the drill that keeps a busy agent busy.
	OtherTeam = "other-team"
	// Project holds the drill: the coordinator and the fake agents.
	Project    = "drill"
	adminToken = "coordeval-admin-token"
)

// Options configure a world.
type Options struct {
	// RequestDelay simulates slow transport/storage in the in-process harness.
	RequestDelay time.Duration
	clock        *scenarioClock
	// Dir holds the hub's database (a temporary directory).
	Dir string
	// Unit is how long one unit of scenario time is: scenario latencies
	// and deadlines are in units. Small for the scripted baseline (tens of
	// milliseconds), seconds for a real CLI that thinks between commands.
	Unit time.Duration
	// Seed makes the fake agents' latencies the same on every run.
	Seed int64
	// Log, when set, receives the hub's log.
	Log io.Writer
}

// World is a hub with a person, a coordinator and fake agents.
type World struct {
	opts  Options
	srv   *httptest.Server
	hub   *hubserver.Hub
	db    string
	admin *client.Client
	// user is the person's device (the App).
	user *client.Client
	// CoordToken is the coordinator's token.
	CoordToken string
	// URL is the hub's address.
	URL string

	mu       sync.Mutex
	rng      *rand.Rand
	fakes    []*Fake
	events   []Event
	goals    []Goal
	catalog  map[string]Item
	answered map[string]int
	start    time.Time
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// Goal is one goal the person sent the coordinator.
type Goal struct {
	ID       string
	Body     string
	SentAt   time.Time
	Priority string // "high" for an urgent one
}

// Item is a piece of work fake agents recognise in a brief by its name:
// a list to add up (Numbers, the answer is its total) or a deliverable
// (the answer is Token).
type Item struct {
	Name    string
	Numbers []int
	Token   string
	// Units is how long it takes (before an agent's own speed).
	Units float64
	// Resource, when set, is what doing it edits (two at once collide).
	Resource string
	// Skill is the kind of agent that can do it ("" = any).
	Skill string
	// WrongFirst makes the first answer for a list off by this much (a
	// plausible, wrong total), whoever gives it.
	WrongFirst int
}

// Answer is the item's correct result, as a reply should carry it.
func (it Item) Answer() string {
	if len(it.Numbers) > 0 {
		sum := 0
		for _, n := range it.Numbers {
			sum += n
		}
		return fmt.Sprint(sum)
	}
	return it.Token
}

// Event is something a fake agent did, for the checkers.
type Event struct {
	At     time.Time `json:"at"`
	Agent  string    `json:"agent"`
	Kind   string    `json:"kind"`
	Item   string    `json:"item,omitempty"`
	Msg    string    `json:"message,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

// Kinds of event.
const (
	EvStart     = "start"     // began an item
	EvFinish    = "finish"    // answered an item
	EvOffline   = "offline"   // went away in the middle of one
	EvStopped   = "stopped"   // its task was stopped while it worked
	EvAskBack   = "ask_back"  // the brief lacked what it needed
	EvEditStart = "edit"      // started editing a resource
	EvEditEnd   = "edit_end"  // finished editing it
	EvViolation = "violation" // did what the person said not to
)

var buildMu sync.Mutex

// NewWorld starts a hub, the person and the coordinator's identity.
func NewWorld(ctx context.Context, opts Options) (*World, error) {
	if opts.Unit <= 0 {
		opts.Unit = time.Second
	}
	if opts.Dir == "" {
		return nil, errors.New("coordeval: Options.Dir is required")
	}
	logw := opts.Log
	if logw == nil {
		logw = io.Discard
	}
	w := &World{opts: opts, db: filepath.Join(opts.Dir, "hub.db"), rng: rand.New(rand.NewSource(opts.Seed)), catalog: map[string]Item{}, answered: map[string]int{}}
	policy := app.DefaultMessagePolicy()
	// (Build sets a few package-level things: one world at a time)
	buildMu.Lock()
	var serviceClock app.Clock
	if opts.clock != nil {
		serviceClock = opts.clock
	}
	h, err := hubserver.Build(hubserver.Options{DBPath: w.db, AdminToken: adminToken, Policy: policy,
		// an agent that stopped asking for work is away after four units
		Clock: serviceClock, OnlineWindow: 4 * opts.Unit, Log: slog.New(slog.NewTextHandler(logw, nil))})
	buildMu.Unlock()
	if err != nil {
		return nil, err
	}
	w.hub = h
	w.srv = httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		requestCtx := r.Context()
		if opts.clock != nil {
			ctx, done := scenarioActorContext(context.WithValue(requestCtx, scenarioClockKey{}, opts.clock))
			defer done()
			r = r.WithContext(ctx)
		}
		if opts.RequestDelay > 0 && !sleepCtx(requestCtx, opts.RequestDelay) {
			return
		}
		h.Handler.ServeHTTP(out, r)
	}))
	w.URL = w.srv.URL
	if w.admin, err = client.New(w.URL, adminToken); err != nil {
		w.Close()
		return nil, err
	}
	inv, err := w.admin.InviteMember(ctx, api.InviteRequest{Name: UserName, Role: "admin"})
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("the person: %w", err)
	}
	anon, _ := client.New(w.URL, "")
	pair, err := anon.Pair(ctx, inv.Link[strings.Index(inv.Link, "/c/")+3:], "tester-app")
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("the person's device: %w", err)
	}
	if w.user, err = client.New(w.URL, pair.Token); err != nil {
		w.Close()
		return nil, err
	}
	reg, err := w.admin.Register(ctx, api.RegisterRequest{Name: Lead, DisplayName: "Lead",
		Description:  "Project coordinator: splits the person's goals among the team and puts the results together",
		Capabilities: []string{"coordinate"}}, false)
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("the coordinator: %w", err)
	}
	w.CoordToken = reg.Token
	w.start = w.now()
	return w, nil
}

// Unit is the world's unit of scenario time.
func (w *World) Unit() time.Duration { return w.opts.Unit }

// Units is n units of scenario time.
func (w *World) Units(n float64) time.Duration { return time.Duration(n * float64(w.opts.Unit)) }

// Admin is a client with the hub's administrator token.
func (w *World) Admin() *client.Client { return w.admin }

// CoordClient is a client acting as the coordinator.
func (w *World) CoordClient() *client.Client {
	c, _ := client.New(w.URL, w.CoordToken)
	return c
}

// AddItems makes items known to the fake agents.
func (w *World) AddItems(items ...Item) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, it := range items {
		w.catalog[strings.ToLower(it.Name)] = it
	}
}

// Items are the items known, by name.
func (w *World) Items() map[string]Item {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]Item, len(w.catalog))
	for k, v := range w.catalog {
		out[k] = v
	}
	return out
}

// SetupProject puts the coordinator and the fake agents in the drill's
// project, with the coordinator as its coordinator.
func (w *World) SetupProject(ctx context.Context, goal string) error {
	if _, err := w.admin.CreateProject(ctx, api.CreateProjectRequest{Name: Project, Title: "Coordination drill", Goal: goal}); err != nil {
		return err
	}
	coordinator, member := "coordinator", "member"
	if _, err := w.admin.PutProjectMember(ctx, Project, Lead, api.ProjectMemberRequest{Role: &coordinator}); err != nil {
		return err
	}
	for _, f := range w.fakes {
		if _, err := w.admin.PutProjectMember(ctx, Project, f.Name, api.ProjectMemberRequest{Role: &member}); err != nil {
			return err
		}
	}
	return nil
}

// SetAlertAfter sets how long a task of the drill may be quiet before the
// hub alerts its coordinator (whole seconds; none below one).
func (w *World) SetAlertAfter(ctx context.Context, d time.Duration) error {
	if d < time.Second {
		return nil
	}
	_, err := w.admin.TaskAlertPolicy(ctx, Project, &api.TaskAlertPolicyRequest{StaleSeconds: int(d / time.Second)}, false)
	return err
}

// SendGoal sends the person's goal to the coordinator.
func (w *World) SendGoal(ctx context.Context, body, priority string) (Goal, error) {
	res, err := w.user.ConsoleSend(ctx, api.SendRequest{To: Lead, Type: "task", Body: body, Project: Project})
	if err != nil {
		return Goal{}, err
	}
	g := Goal{ID: res.Message.ID, Body: body, SentAt: w.now(), Priority: priority}
	w.mu.Lock()
	w.goals = append(w.goals, g)
	w.mu.Unlock()
	return g, nil
}

// Goals are the goals sent so far.
func (w *World) Goals() []Goal {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]Goal(nil), w.goals...)
}

// answerQuestions answers what the coordinator asks the person, as the
// person would at the App: a question on one of their goals (agents reach
// a person only on that person's own tasks). The drill has nothing for
// them to decide, so they say to go on as asked (the question still counts
// against the coordinator).
func (w *World) answerQuestions(ctx context.Context) {
	defer w.wg.Done()
	const goOn = "Go on as I asked; there is nothing more for me to decide."
	answered := map[string]bool{}
	for ctx.Err() == nil {
		for _, g := range w.Goals() {
			m, err := w.goal(ctx, g.ID)
			if err != nil || m.Asking == "" || answered[m.Asking] {
				continue
			}
			answered[m.Asking] = true
			_, _ = w.user.ConsoleSteer(ctx, g.ID, goOn)
		}
		sleepCtx(ctx, w.opts.Unit)
	}
}

// goal is a goal message as the person sees it now.
func (w *World) goal(ctx context.Context, id string) (*api.Message, error) {
	msgs, err := w.user.Thread(ctx, id)
	if err != nil {
		return nil, err
	}
	for i := range msgs {
		if msgs[i].ID == id {
			return &msgs[i], nil
		}
	}
	return nil, fmt.Errorf("goal %s not found", id)
}

func (w *World) record(e Event) {
	e.At = w.now()
	w.mu.Lock()
	w.events = append(w.events, e)
	w.mu.Unlock()
}

// Events are what the fake agents did, in order.
func (w *World) Events() []Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := append([]Event(nil), w.events...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// Run starts the fake agents, the person's side (answering questions) and
// the hub's coordinator alerts (a served hub runs them itself).
func (w *World) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.wg.Add(1)
	goScenario(ctx, w.answerQuestions)
	if w.hub.TaskMonitor != nil {
		w.wg.Add(1)
		goScenario(ctx, func(ctx context.Context) {
			defer w.wg.Done()
			for ctx.Err() == nil {
				_ = w.hub.TaskMonitor.Tick(ctx)
				sleepCtx(ctx, w.opts.Unit)
			}
		})
	}
	for _, f := range w.fakes {
		w.wg.Add(1)
		goScenario(ctx, func(ctx context.Context) {
			defer w.wg.Done()
			f.run(ctx)
		})
	}
}

// Stop ends the fake agents (the hub keeps running for the checkers).
func (w *World) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

// Close stops everything and the hub.
func (w *World) Close() {
	w.Stop()
	if w.srv != nil {
		w.srv.Close()
		w.srv = nil
	}
	if w.hub != nil {
		w.hub.Close()
		w.hub = nil
	}
}

// Messages are every message the hub keeps, oldest first: what the
// checkers score.
func (w *World) Messages(ctx context.Context) ([]*domain.Message, error) {
	db, err := sqlite.Open(w.db)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	threads, err := threadIDs(ctx, db)
	if err != nil {
		return nil, err
	}
	repo := sqlite.NewMessageRepo(db)
	var out []*domain.Message
	for _, id := range threads {
		ms, err := repo.ListThread(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, ms...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func threadIDs(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT thread_id FROM messages`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Agents are the agents the hub knows now.
func (w *World) Agents(ctx context.Context) ([]api.Agent, error) {
	return w.admin.ListAgents(ctx, "", false)
}

func (w *World) now() time.Time {
	if w.opts.clock != nil {
		return w.opts.clock.Now()
	}
	return time.Now()
}
