// Package notices shows a person's notices on this computer: it follows
// every workspace's notice feed (GET /v1/console/notices) while the App
// runs, window shown or not, and hands what is to be announced to a
// Presenter (the system's notification centre, in the desktop app).
//
// What is announced was decided on the hub (Notice.Announce: tiers, mutes,
// quiet hours, pause); this package adds only what this computer alone
// knows: the person is looking at that conversation right now (then it is
// not announced, and is marked read on the hub, so other devices do not
// announce it either). It also merges a burst from one conversation into
// one banner, and keeps the badge.
//
// It knows nothing of the console or the desktop: workspaces come from
// Hubs, the system from Presenter. Both are small interfaces, so a new
// platform is a new Presenter and nothing else.
package notices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/myco-ai/myco/internal/domain"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/uilocale"
)

// Feed is a workspace's notice feed (a *client.Client).
type Feed interface {
	Notices(ctx context.Context, after int64, limit int, wait time.Duration) (*api.NoticeFeed, error)
	ReadNotices(ctx context.Context, target string, through int64) (int, error)
}

// Hub is one workspace to follow.
type Hub struct {
	ID, Name string
	// Feed returns a client for it (built anew after errors: a token
	// changed, a key arrived).
	Feed func(ctx context.Context) (Feed, error)
}

// Hubs lists the workspaces to follow; asked again now and then, so one
// added or removed is picked up.
type Hubs func() []Hub

// Toast is one banner.
type Toast struct {
	// ID replaces an earlier banner with the same ID (one per
	// conversation: a burst stays one banner).
	ID       string
	Title    string
	Subtitle string
	Body     string
	Sound    bool
	// Hub and Target are where a click leads (Target as a notice's);
	// Actor is who it is from (the chat to open when the target is not
	// one, an approval: the superagent's).
	Hub, Target, Actor string
}

// Presenter is the system's notifications on this computer.
type Presenter interface {
	// Show shows t, replacing a shown one with its ID.
	Show(t Toast) error
	// Withdraw removes the banners with these IDs (seen in the App).
	Withdraw(ids ...string)
	// Badge sets the number on the App's icon (0: none).
	Badge(n int)
}

// Focus is what the person is looking at in the App.
type Focus struct {
	// Front: the App's window is in front (and the screen awake).
	Front bool `json:"front"`
	// Hub and Target: the conversation on screen ("" for none).
	Hub    string `json:"hub,omitempty"`
	Target string `json:"target,omitempty"`
}

// State is what the App shows of this: per workspace, whether it can tell
// (an older hub cannot) and its badge.
type State struct {
	Hubs  map[string]HubState `json:"hubs"`
	Badge int                 `json:"badge"`
}

// HubState is one workspace's.
type HubState struct {
	// Supported: the hub keeps notices (v0.21.21+).
	Supported bool   `json:"supported"`
	Badge     int    `json:"badge"`
	Error     string `json:"error,omitempty"`
}

// Watcher follows the workspaces' feeds. Start it once; Stop ends it.
type Watcher struct {
	hubs      Hubs
	present   Presenter
	lang      func() string
	statePath string
	log       *slog.Logger
	// now and the waits, for tests.
	now      func() time.Time
	wait     time.Duration
	retryMin time.Duration
	retryMax time.Duration
	oldRetry time.Duration
	rescan   time.Duration
	merge    time.Duration

	mu      sync.Mutex
	focus   Focus
	loops   map[string]*loop
	cursors map[string]int64
	states  map[string]HubState
	groups  map[string]*group
	// seen is how many waited when the App was last in front: the Dock
	// counts only what came after (looking at the App clears it).
	seen   int
	cancel context.CancelFunc
	done   chan struct{}
}

type loop struct {
	hub    Hub
	cancel context.CancelFunc
	done   chan struct{}
}

// group is a conversation's banner while it is shown.
type group struct {
	count int
	at    time.Time
}

// Options configure a Watcher.
type Options struct {
	Hubs    Hubs
	Present Presenter
	// Lang is the App's supported interface language.
	Lang func() string
	// StatePath keeps where each feed was read up to, so a restart does
	// not announce again (nor miss) what came meanwhile.
	StatePath string
	Log       *slog.Logger
}

// New returns a watcher; nothing runs until Start.
func New(o Options) *Watcher {
	if o.Lang == nil {
		o.Lang = func() string { return "en" }
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	return &Watcher{
		hubs: o.Hubs, present: o.Present, lang: o.Lang, statePath: o.StatePath, log: o.Log,
		now: time.Now, wait: 50 * time.Second, retryMin: 2 * time.Second, retryMax: time.Minute,
		oldRetry: 10 * time.Minute, rescan: 30 * time.Second, merge: 2 * time.Minute,
		loops: map[string]*loop{}, cursors: map[string]int64{}, states: map[string]HubState{}, groups: map[string]*group{},
	}
}

// Start follows the workspaces until Stop.
func (w *Watcher) Start() {
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return
	}
	w.loadCursors()
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel, w.done = cancel, make(chan struct{})
	w.mu.Unlock()
	go w.run(ctx)
}

// Stop ends every feed.
func (w *Watcher) Stop() {
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.cancel = nil
	w.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (w *Watcher) run(ctx context.Context) {
	defer close(w.done)
	t := time.NewTicker(w.rescan)
	defer t.Stop()
	for {
		w.sync(ctx)
		select {
		case <-ctx.Done():
			w.mu.Lock()
			var dones []chan struct{}
			for id, l := range w.loops {
				l.cancel()
				dones = append(dones, l.done)
				delete(w.loops, id)
			}
			w.mu.Unlock()
			// Wait for the feeds to stop: one may still be writing the
			// state file, and Stop's caller may be removing its directory.
			for _, done := range dones {
				<-done
			}
			return
		case <-t.C:
		}
	}
}

// sync starts a feed for each workspace new to the list and stops those
// removed.
func (w *Watcher) sync(ctx context.Context) {
	want := map[string]Hub{}
	if w.hubs != nil {
		for _, h := range w.hubs() {
			want[h.ID] = h
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, l := range w.loops {
		if _, ok := want[id]; !ok {
			l.cancel()
			delete(w.loops, id)
			delete(w.states, id)
		}
	}
	for id, h := range want {
		if l, ok := w.loops[id]; ok {
			l.hub.Name = h.Name
			continue
		}
		lctx, cancel := context.WithCancel(ctx)
		l := &loop{hub: h, cancel: cancel, done: make(chan struct{})}
		w.loops[id] = l
		go func() {
			defer close(l.done)
			w.follow(lctx, h)
		}()
	}
}

// follow reads one workspace's feed until ctx ends.
func (w *Watcher) follow(ctx context.Context, h Hub) {
	retry := w.retryMin
	for ctx.Err() == nil {
		f, err := h.Feed(ctx)
		if err == nil {
			err = w.read(ctx, h, f)
		}
		if ctx.Err() != nil {
			return
		}
		pause := retry
		if isOld(err) {
			w.setState(h.ID, HubState{Supported: false})
			pause = w.oldRetry
		} else {
			w.setState(h.ID, HubState{Supported: w.stateOf(h.ID).Supported, Badge: w.stateOf(h.ID).Badge, Error: errText(err)})
			retry = min(retry*2, w.retryMax)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
	}
}

// read follows f until an error. The first read on a workspace never seen
// starts after what is already there: that is not news.
func (w *Watcher) read(ctx context.Context, h Hub, f Feed) error {
	w.mu.Lock()
	after, known := w.cursors[h.ID]
	w.mu.Unlock()
	if !known {
		feed, err := f.Notices(ctx, 0, 1, 0)
		if err != nil {
			return err
		}
		after = feed.Last
		w.setCursor(h.ID, after)
		w.setState(h.ID, HubState{Supported: true, Badge: feed.Badge})
	}
	for ctx.Err() == nil {
		feed, err := f.Notices(ctx, after, 100, w.wait)
		if err != nil {
			return err
		}
		w.setState(h.ID, HubState{Supported: true, Badge: feed.Badge})
		for i := range feed.Notices {
			w.tell(ctx, h, f, &feed.Notices[i])
		}
		if feed.Last > after || len(feed.Notices) > 0 {
			after = max(feed.Last, after)
			w.setCursor(h.ID, after)
		}
	}
	return ctx.Err()
}

// tell announces n unless the person is looking at it (then it is read).
func (w *Watcher) tell(ctx context.Context, h Hub, f Feed, n *api.Notice) {
	if n.ReadAt != nil || n.ResolvedAt != nil {
		return
	}
	w.mu.Lock()
	focus := w.focus
	w.mu.Unlock()
	if focus.Front && focus.Hub == h.ID && sameTarget(focus.Target, n.Target) {
		if _, err := f.ReadNotices(ctx, n.Target, n.Seq); err != nil {
			w.log.Debug("notices: read", "err", err)
		}
		return
	}
	if !n.Announce || w.present == nil {
		return
	}
	t := w.toast(h, n)
	if err := w.present.Show(t); err != nil {
		w.log.Warn("notices: show", "err", err)
	}
}

func (w *Watcher) toast(h Hub, n *api.Notice) Toast {
	language := w.lang()
	id := h.ID + "|" + targetKey(n.Target)
	t := Toast{ID: id, Hub: h.ID, Target: n.Target, Actor: n.Actor, Sound: n.Tier == "needs_you"}
	t.Title = n.Actor
	if n.Message != nil && n.Message.FromDisplay != "" {
		t.Title = n.Message.FromDisplay
	}
	if t.Title == "" {
		t.Title = h.Name
	}
	w.mu.Lock()
	if len(w.loops) > 1 {
		t.Subtitle = h.Name
	}
	g := w.groups[id]
	now := w.now()
	if g == nil || now.Sub(g.at) > w.merge {
		g = &group{}
		w.groups[id] = g
	}
	g.count++
	g.at = now
	count := g.count
	w.mu.Unlock()
	t.Body = WordsForLanguage(n, language)
	if count > 1 {
		t.Body = fmt.Sprintf(uilocale.Text(language, "%s 条新消息 · ", "%s new · "), itoa(count)) + t.Body
	}

	return t
}

// shownBadge is the number for the Dock out of total (the hub's open
// needs-you notices): what came since the App was last in front; none
// while it is. (Called with w.mu held.)
func (w *Watcher) shownBadge(total int) int {
	if w.focus.Front || total < w.seen {
		w.seen = total
	}
	return total - w.seen
}

func (w *Watcher) totalLocked() int {
	total := 0
	for _, x := range w.states {
		total += x.Badge
	}
	return total
}

// SetFocus says what the person is looking at. Coming to a conversation
// marks its notices read on the hub (every device stops announcing them)
// and takes its banners away.
func (w *Watcher) SetFocus(ctx context.Context, f Focus) {
	w.mu.Lock()
	wasFront := w.focus.Front
	w.focus = f
	// The App in front: the Dock's number goes (what came before is seen).
	clear := f.Front && !wasFront
	if clear {
		w.shownBadge(w.totalLocked())
	}
	l := w.loops[f.Hub]
	var id string
	if f.Front && f.Target != "" {
		id = f.Hub + "|" + targetKey(f.Target)
		delete(w.groups, id)
	}
	w.mu.Unlock()
	if clear && w.present != nil {
		w.present.Badge(0)
	}
	if id == "" || l == nil {
		return
	}
	if w.present != nil {
		w.present.Withdraw(id)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if fd, err := l.hub.Feed(ctx); err == nil {
			_, _ = fd.ReadNotices(ctx, f.Target, 0)
		}
	}()
}

// State is what the App shows: per workspace, and the badge.
func (w *Watcher) State() State {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := State{Hubs: map[string]HubState{}}
	for id, s := range w.states {
		out.Hubs[id] = s
		out.Badge += s.Badge
	}
	return out
}

func (w *Watcher) stateOf(id string) HubState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.states[id]
}

func (w *Watcher) setState(id string, s HubState) {
	w.mu.Lock()
	if _, ok := w.loops[id]; !ok {
		w.mu.Unlock()
		return
	}
	w.states[id] = s
	shown := w.shownBadge(w.totalLocked())
	w.mu.Unlock()
	if w.present != nil {
		w.present.Badge(shown)
	}
}

// --- where each feed was read up to ---

func (w *Watcher) loadCursors() {
	if w.statePath == "" {
		return
	}
	data, err := os.ReadFile(w.statePath)
	if err != nil {
		return
	}
	var c struct {
		Cursors map[string]int64 `json:"cursors"`
	}
	if json.Unmarshal(data, &c) == nil && c.Cursors != nil {
		w.cursors = c.Cursors
	}
}

func (w *Watcher) setCursor(id string, seq int64) {
	w.mu.Lock()
	w.cursors[id] = seq
	cur := map[string]int64{}
	for k, v := range w.cursors {
		if _, ok := w.loops[k]; ok {
			cur[k] = v
		}
	}
	w.mu.Unlock()
	if w.statePath == "" {
		return
	}
	data, _ := json.Marshal(struct {
		Cursors map[string]int64 `json:"cursors"`
	}{cur})
	if err := os.MkdirAll(filepath.Dir(w.statePath), 0o700); err != nil {
		return
	}
	tmp := w.statePath + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, w.statePath)
	}
}

// --- words ---

// Words is a notice's line, in Chinese or English: what kind of thing it
// is, then its first line.
func Words(n *api.Notice, zh bool) string {
	language := "en"
	if zh {
		language = "zh"
	}
	return WordsForLanguage(n, language)
}

// WordsForLanguage localizes only the UI prefix; subjects and bodies stay intact.
func WordsForLanguage(n *api.Notice, language string) string {
	choose := func(zh, en string) string { return uilocale.Text(language, zh, en) }
	text := n.Subject
	if n.Message != nil {
		if n.Message.Body != "" && !looksSealed(n.Message.Body) {
			text = firstLine(n.Message.Body)
		} else if text == "" {
			text = choose("有一条新消息", "A new message")
		}
	}
	lead := ""
	switch n.Kind {
	case "asking":
		lead = choose("问你：", "Asks you: ")
	case "decision":
		lead = choose("等你决定：", "Waiting for your decision: ")
	case "approval":
		lead = choose("请你批准：", "Needs your OK: ")
	case "failed":
		lead = choose("没做成：", "Didn't get done: ")
	case "run_failed":
		lead = choose("运行失败：", "Run failed: ")
	case "stuck":
		lead = choose("可能卡住了：", "May be stuck: ")
	case "answered":
		lead = choose("回复：", "Replied: ")
	case "progress":
		lead = choose("进展：", "Update: ")
	}
	return lead + text
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if utf8.RuneCountInString(s) > 140 {
		r := []rune(s)
		s = string(r[:139]) + "…"
	}
	return s
}

// looksSealed: a body this computer could not open (end-to-end
// encryption without the key) is not shown as gibberish.
func looksSealed(body string) bool { return strings.HasPrefix(body, domain.SealedPrefix) }

func targetKey(t string) string {
	if i := strings.IndexByte(t, '?'); i >= 0 {
		return t[:i]
	}
	return t
}

func sameTarget(onScreen, target string) bool {
	return onScreen != "" && targetKey(onScreen) == targetKey(target)
}

// isOld: a hub from before notices (no such route, or it says it keeps
// none).
func isOld(err error) bool {
	var e *client.Error
	return errors.As(err, &e) && e.Status == 404
}

func errText(err error) string {
	if err == nil || errors.Is(err, context.Canceled) {
		return ""
	}
	return err.Error()
}

func itoa(n int) string { return strconv.Itoa(n) }
