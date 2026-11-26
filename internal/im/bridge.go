package im

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// Hub is the myco side of a bridge: the node it speaks as.
type Hub interface {
	Send(ctx context.Context, req api.SendRequest) (*api.Message, error)
	Steer(ctx context.Context, task, body string) error
	Stop(ctx context.Context, task string) error
	Get(ctx context.Context, id string) (*api.Message, error)
	// WaitReply waits up to d for the task's reply, calling progress for
	// each update; nil, nil when none came in time.
	WaitReply(ctx context.Context, task string, d time.Duration, progress func(api.Message)) (*api.Message, error)
	Pull(ctx context.Context, wait time.Duration) ([]api.Message, error)
	Close(ctx context.Context, id string) error
	Reply(ctx context.Context, id, body string) error
}

// ClientHub is a Hub backed by a myco client.
type ClientHub struct{ C *client.Client }

func (h ClientHub) Send(ctx context.Context, req api.SendRequest) (*api.Message, error) {
	res, err := h.C.Send(ctx, req, false)
	if err != nil {
		return nil, err
	}
	return &res.Message, nil
}
func (h ClientHub) Steer(ctx context.Context, task, body string) error {
	_, err := h.C.Steer(ctx, task, body, false)
	return err
}
func (h ClientHub) Stop(ctx context.Context, task string) error {
	_, err := h.C.StopTask(ctx, task, false)
	return err
}
func (h ClientHub) Get(ctx context.Context, id string) (*api.Message, error) {
	return h.C.GetMessage(ctx, id)
}
func (h ClientHub) WaitReply(ctx context.Context, task string, d time.Duration, progress func(api.Message)) (*api.Message, error) {
	return h.C.WaitReplyProgress(ctx, task, d, progress)
}
func (h ClientHub) Pull(ctx context.Context, wait time.Duration) ([]api.Message, error) {
	return h.C.PullTypes(ctx, 20, wait, []string{"task", "note", "event", "progress"})
}
func (h ClientHub) Close(ctx context.Context, id string) error { return h.C.Complete(ctx, id) }
func (h ClientHub) Reply(ctx context.Context, id, body string) error {
	_, err := h.C.Reply(ctx, id, body, false)
	return err
}

// Bridge passes messages between one chat-app bot and myco.
type Bridge struct {
	cfg  Config
	ad   Adapter
	hub  Hub
	st   *state
	log  *slog.Logger
	lang string

	ctx   atomic.Value // context.Context, set by Run
	locks sync.Map     // conversation key → *sync.Mutex
	seen  seenSet
	// throttle for status updates while a task reports progress
	statusEvery time.Duration
	waitEach    time.Duration

	// OnConfig saves the config when the bridge changes it (an owner
	// claimed it).
	OnConfig func(Config) error
	cfgMu    sync.Mutex
}

// New makes a bridge; dir is where its state is kept ("" keeps it in memory).
func New(cfg Config, ad Adapter, hub Hub, dir string, log *slog.Logger) (*Bridge, error) {
	st := &state{Convs: map[string]*conversation{}}
	if dir != "" {
		var err error
		if st, err = loadState(dir, cfg.Name); err != nil {
			return nil, err
		}
	}
	if log == nil {
		log = slog.Default()
	}
	lang := cfg.Lang
	if lang == "" {
		lang = DefaultLang(cfg.Platform)
	}
	return &Bridge{cfg: cfg, ad: ad, hub: hub, st: st, log: log, lang: lang,
		statusEvery: 2 * time.Second, waitEach: 5 * time.Minute}, nil
}

// Run serves until ctx ends: the chat app's messages, the node's inbox, and
// the tasks still open from before.
func (b *Bridge) Run(ctx context.Context) error {
	b.ctx.Store(ctx)
	b.st.mu.Lock()
	for key, c := range b.st.Convs {
		if c.Task != "" {
			go b.watch(key, c.Task)
		}
	}
	b.st.mu.Unlock()
	go b.inbox(ctx)
	return b.ad.Run(ctx, b.Handle)
}

// Handle takes one message from the chat app (it returns at once).
func (b *Bridge) Handle(in Inbound) {
	if in.MessageID != "" && !b.seen.add(in.MessageID) {
		return // the app delivered it again
	}
	go func() {
		mu, _ := b.locks.LoadOrStore(in.ThreadKey, &sync.Mutex{})
		mu.(*sync.Mutex).Lock()
		defer mu.(*sync.Mutex).Unlock()
		b.process(b.context(), in)
	}()
}

func (b *Bridge) context() context.Context {
	if ctx, ok := b.ctx.Load().(context.Context); ok {
		return ctx
	}
	return context.Background()
}

func (b *Bridge) conv(key string) *conversation {
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	if c := b.st.Convs[key]; c != nil {
		cp := *c
		return &cp
	}
	return nil
}

func (b *Bridge) update(key string, f func(c *conversation)) {
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	c := b.st.Convs[key]
	if c == nil {
		c = &conversation{}
		b.st.Convs[key] = c
	}
	f(c)
	c.At = time.Now()
	if err := b.st.save(); err != nil {
		b.log.Warn("im: saving state failed", "err", err)
	}
}

func (b *Bridge) say(ctx context.Context, ref Ref, text string) {
	if _, err := b.ad.Post(ctx, ref, text); err != nil {
		b.log.Warn("im: posting failed", "err", err)
	}
}

func (b *Bridge) process(ctx context.Context, in Inbound) {
	ref := Ref{ChatID: in.ChatID, ReplyTo: orText(in.ReplyTo, in.MessageID), Thread: in.Group}
	if strings.HasPrefix(ref.ReplyTo, "cmd:") {
		ref.ReplyTo = "" // a slash command has no message to answer
	}
	known := b.conv(in.ThreadKey)
	if in.Group && !in.Mentioned && known == nil {
		return // group talk not meant for the bot
	}
	if b.claim(ctx, in, ref) {
		return
	}
	b.cfgMu.Lock()
	allowed := b.cfg.Allowed(in.UserID)
	b.cfgMu.Unlock()
	if !allowed {
		if !in.Group || in.Mentioned {
			b.say(ctx, ref, b.t("not_allowed", b.cfg.Name, in.UserID))
		}
		return
	}
	agent := b.cfg.Agent
	cmd := parseCommand(in.Text)
	switch cmd.name {
	case "help":
		b.say(ctx, ref, b.t("help", agent))
		return
	case "whoami":
		b.say(ctx, ref, b.t("whoami", in.UserID))
		return
	case "new":
		b.st.mu.Lock()
		delete(b.st.Convs, in.ThreadKey)
		_ = b.st.save()
		b.st.mu.Unlock()
		if cmd.rest == "" {
			b.say(ctx, ref, b.t("new"))
			return
		}
		known = nil
	case "stop":
		if known == nil || known.Task == "" {
			b.say(ctx, ref, b.t("no_task"))
			return
		}
		if err := b.hub.Stop(ctx, known.Task); err != nil {
			b.say(ctx, ref, b.t("send_failed", agent, err.Error()))
			return
		}
		b.say(ctx, ref, b.t("stopped", agent))
		return
	}
	text := strings.TrimSpace(cmd.rest)
	if text == "" {
		b.say(ctx, ref, b.t("help", agent))
		return
	}
	if agent == "" {
		b.say(ctx, ref, b.t("no_agent"))
		return
	}
	who := in.UserName
	if who == "" && in.UserID == b.owner() {
		who = b.cfg.OwnerName
	}
	body := text + "\n\n——\n" + b.t("via", platformName(b.cfg.Platform, b.lang), orText(who, in.UserID))

	// An open task: what they say is added to it (and answers its question).
	if known != nil && known.Task != "" {
		err := b.hub.Steer(ctx, known.Task, body)
		if err == nil {
			b.say(ctx, ref, b.t("steered", agent))
			return
		}
		if !closedTask(err) {
			b.say(ctx, ref, b.t("send_failed", agent, err.Error()))
			return
		}
		// It closed meanwhile: a new task in the same thread.
	}
	req := api.SendRequest{To: agent, Type: "task", Body: body,
		IdempotencyKey: "im:" + b.cfg.Name + ":" + in.MessageID}
	if known != nil && known.ThreadID != "" {
		req.ThreadID = known.ThreadID
	} else {
		req.Project = b.cfg.Project
	}
	msg, err := b.hub.Send(ctx, req)
	if err != nil {
		b.say(ctx, ref, b.t("send_failed", agent, err.Error()))
		return
	}
	status, err := b.ad.Status(ctx, ref, b.t("queued", agent))
	if err != nil {
		b.log.Warn("im: status message failed", "err", err)
	}
	b.update(in.ThreadKey, func(c *conversation) {
		c.Ref, c.ThreadID, c.Task, c.Agent, c.Status, c.User = ref, msg.ThreadID, msg.ID, agent, status, in.UserID
	})
	go b.watch(in.ThreadKey, msg.ID)
}

func (b *Bridge) owner() string {
	b.cfgMu.Lock()
	defer b.cfgMu.Unlock()
	return b.cfg.Owner
}

// claim handles a bot that has no owner yet: in a direct chat, "/claim
// <code>" with the code from myco im connect makes the sender its owner.
func (b *Bridge) claim(ctx context.Context, in Inbound, ref Ref) bool {
	b.cfgMu.Lock()
	defer b.cfgMu.Unlock()
	if b.cfg.Owner != "" {
		return false
	}
	cmd := parseCommand(in.Text)
	if cmd.name != "claim" || in.Group {
		b.say(ctx, ref, b.t("unclaimed"))
		return true
	}
	if !b.cfg.ClaimUsable(time.Now()) {
		b.say(ctx, ref, b.t("claim_gone"))
		return true
	}
	if subtle.ConstantTimeCompare([]byte(cmd.arg), []byte(b.cfg.Claim)) != 1 {
		cfg := b.cfg
		cfg.ClaimFails++
		if b.OnConfig != nil {
			if err := b.OnConfig(cfg); err != nil {
				b.log.Warn("im: saving a failed claim", "err", err)
			}
		}
		b.cfg = cfg
		if cfg.ClaimUsable(time.Now()) {
			b.say(ctx, ref, b.t("bad_claim"))
		} else {
			b.say(ctx, ref, b.t("claim_gone"))
		}
		return true
	}
	cfg := b.cfg
	cfg.Owner, cfg.Claim, cfg.ClaimExpires, cfg.ClaimFails = in.UserID, "", time.Time{}, 0
	if b.OnConfig != nil {
		if err := b.OnConfig(cfg); err != nil {
			b.say(ctx, ref, b.t("send_failed", "config", err.Error()))
			return true
		}
	}
	b.cfg = cfg
	b.say(ctx, ref, b.t("claimed"))
	return true
}

// watch follows a task until it closes: progress and questions update its
// status message; its answer is posted as a new message.
func (b *Bridge) watch(key, task string) {
	ctx := b.context()
	c := b.conv(key)
	if c == nil {
		return
	}
	setStatus := func(text string) {
		if c.Status == "" {
			if id, err := b.ad.Status(ctx, c.Ref, text); err == nil {
				c.Status = id
				b.update(key, func(cc *conversation) { cc.Status = id })
			}
			return
		}
		if err := b.ad.Update(ctx, c.Ref, c.Status, text); err != nil {
			b.log.Debug("im: status update failed", "err", err)
		}
	}
	var last time.Time
	progress := func(p api.Message) {
		if p.Ask {
			setStatus(b.t("asking", c.Agent, clip(p.Body, 1500)))
			// A new message too: an edit does not notify anyone.
			b.say(ctx, c.Ref, b.t("asking", c.Agent, clip(p.Body, 3000)))
			return
		}
		if time.Since(last) < b.statusEvery {
			return
		}
		last = time.Now()
		if s := clip(p.Body, 600); s != "" {
			setStatus(b.t("working", c.Agent, s))
		} else {
			setStatus(b.t("working0", c.Agent))
		}
	}
	finish := func(text string) {
		setStatus(text)
		b.update(key, func(cc *conversation) {
			if cc.Task == task {
				cc.Task, cc.Status = "", ""
			}
		})
	}
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		if cur := b.conv(key); cur == nil || cur.Task != task {
			return // forgotten (/new) or replaced
		}
		reply, err := b.hub.WaitReply(ctx, task, b.waitEach, progress)
		if err == nil && reply != nil {
			finish(b.t("done", c.Agent))
			b.say(ctx, c.Ref, clip(reply.Body, 20000))
			return
		}
		if err == nil {
			// No answer yet: closed some other way?
			m, gerr := b.hub.Get(ctx, task)
			err = gerr
			if gerr == nil {
				switch m.Status {
				case "failed":
					finish(b.t("failed", c.Agent, clip(orText(m.Error, "?"), 600)))
					return
				case "done":
					finish(b.t("closed"))
					return
				}
				continue
			}
		}
		if ctx.Err() != nil {
			return
		}
		b.log.Warn("im: following task failed", "task", task, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 2*time.Minute)
	}
}

// inbox handles what reaches the node itself: events about its tasks (the
// watchers see those) are closed; anything else is passed to the owner.
func (b *Bridge) inbox(ctx context.Context) {
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		msgs, err := b.hub.Pull(ctx, 50*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			b.log.Warn("im: inbox failed", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 2*time.Minute)
			continue
		}
		backoff = 5 * time.Second
		for _, m := range msgs {
			b.fromHub(ctx, m)
		}
	}
}

func (b *Bridge) fromHub(ctx context.Context, m api.Message) {
	if b.tracked(m.ReplyTo) || m.Type == "event" || m.Type == "progress" {
		_ = b.hub.Close(ctx, m.ID)
		return
	}
	from := orText(m.FromDisplay, m.From)
	b.cfgMu.Lock()
	owner := b.cfg.Owner
	b.cfgMu.Unlock()
	if owner != "" {
		if err := b.ad.Direct(ctx, owner, b.t("from", from, clip(m.Body, 20000))); err != nil {
			b.log.Warn("im: forwarding to the owner failed", "err", err)
			return // leave it for the next pull
		}
	}
	if m.Type == "task" {
		_ = b.hub.Reply(ctx, m.ID, b.t("forwarded", platformName(b.cfg.Platform, b.lang)))
		return
	}
	_ = b.hub.Close(ctx, m.ID)
}

func (b *Bridge) tracked(task string) bool {
	if task == "" {
		return false
	}
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	for _, c := range b.st.Convs {
		if c.Task == task {
			return true
		}
	}
	return false
}

// closedTask says a steer was refused because the task is no longer open.
func closedTask(err error) bool {
	var ae *client.Error
	return errors.As(err, &ae) && ae.Code == "invalid_state"
}

func orText(s, alt string) string {
	if strings.TrimSpace(s) == "" {
		return alt
	}
	return s
}

// seenSet remembers recent message ids (chat apps redeliver).
type seenSet struct {
	mu    sync.Mutex
	ids   map[string]struct{}
	order []string
}

func (s *seenSet) add(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ids == nil {
		s.ids = map[string]struct{}{}
	}
	if _, ok := s.ids[id]; ok {
		return false
	}
	s.ids[id] = struct{}{}
	s.order = append(s.order, id)
	if len(s.order) > 2000 {
		delete(s.ids, s.order[0])
		s.order = s.order[1:]
	}
	return true
}
