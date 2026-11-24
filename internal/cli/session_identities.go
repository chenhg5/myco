package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
)

// One CLI session can be an agent of several workspaces: a profile on each
// hub, the same session id. Inside such a session,
//   - myco inbox wait and inbox pull serve every identity at once;
//   - a command on a message (--id msg_…) acts as the identity that owns it;
//   - other commands act as the first identity (by file name) and say so;
//     MYCO_AGENT=<name> picks another.
// A runner-woken turn has MYCO_CONFIG set to its profile and is not affected.

// pickSessionProfile chooses the profile for session sessionID: MYCO_AGENT
// when it names one of them, else the first. all is every profile of the
// session (nil when it has none).
func pickSessionProfile(getenv func(string) string, def, sessionID string) (path string, all []config.Profile, byEnv bool) {
	all = config.SessionProfiles(config.ProfilesDir(def), sessionID)
	if len(all) == 0 {
		return "", nil, false
	}
	if name := strings.TrimSpace(getenv(EnvAgent)); name != "" {
		for _, p := range all {
			if p.Config.Agent == name {
				return p.Path, all, true
			}
		}
	}
	return all[0].Path, all, false
}

// sessionIdentities are this session's identities when it has several and
// nothing chose one (no --config / MYCO_CONFIG, MYCO_TOKEN or MYCO_AGENT):
// the commands that can serve them all do.
func (e *env) sessionIdentities() []config.Profile {
	if len(e.sessionProfiles) < 2 || e.explicitConfig || e.pickedByEnv || e.in.Getenv(config.EnvToken) != "" {
		return nil
	}
	var live []config.Profile
	for _, p := range e.sessionProfiles {
		if p.Config.Replaced == "" && p.Config.Token != "" {
			live = append(live, p)
		}
	}
	if len(live) < 2 {
		return nil
	}
	return live
}

// identityLabel is "<agent> on <hub host and path>" (workspaces on one
// server differ by their path, /w/<name>).
func identityLabel(c *config.Config) string {
	return c.Agent + " on " + hubLabel(c.Hub)
}

func hubLabel(hub string) string {
	if u, err := url.Parse(hub); err == nil && u.Host != "" {
		return u.Host + strings.TrimSuffix(u.Path, "/")
	}
	return hub
}

// useProfile makes p this command's identity.
func (e *env) useProfile(p config.Profile) error {
	cfg, err := config.Load(p.Path)
	if err != nil {
		return err
	}
	cfg.ApplyEnv(e.in.Getenv)
	e.cfgPath, e.cfg = p.Path, cfg
	return nil
}

// clientFor is a client authenticated as profile p.
func (e *env) clientFor(p config.Profile) (*client.Client, error) {
	c, err := client.New(p.Config.Hub, p.Config.Token)
	if err != nil {
		return nil, err
	}
	return e.withSealer(c, p.Path, p.Config), nil
}

// sessionIdentityChoice runs before an agent client is made: a command on a
// message acts as the identity that owns it; any other command says which
// identity it acts as, once, on stderr.
func (e *env) sessionIdentityChoice() {
	ids := e.sessionIdentities()
	if len(ids) == 0 || e.identityChosen {
		return
	}
	e.identityChosen = true
	if id := strings.TrimSpace(e.in.String("id")); strings.HasPrefix(id, "msg_") {
		if p, ok := e.ownerOf(id, ids); ok {
			e.useProfile(p)
			return
		}
	}
	var others []string
	for _, p := range ids {
		if p.Path != e.cfgPath {
			others = append(others, identityLabel(p.Config))
		}
	}
	fmt.Fprintf(e.in.Stderr, "myco: this session is also %s; acting as %s. Choose with %s=<agent> myco …\n",
		strings.Join(others, ", "), identityLabel(e.cfg), EnvAgent)
}

// ownerOf finds which of ids owns message id: the one that pulled or saw it
// here (remembered), else the one whose hub knows it.
func (e *env) ownerOf(id string, ids []config.Profile) (config.Profile, bool) {
	if path := lookupMessageOwner(e.defaultPath, id); path != "" {
		for _, p := range ids {
			if p.Path == path {
				return p, true
			}
		}
	}
	ctx, cancel := context.WithTimeout(e.in.Ctx, 10*time.Second)
	defer cancel()
	type hit struct {
		i  int
		ok bool
	}
	ch := make(chan hit, len(ids))
	for i, p := range ids {
		go func(i int, p config.Profile) {
			c, err := e.clientFor(p)
			if err != nil {
				ch <- hit{i, false}
				return
			}
			_, err = c.GetMessage(ctx, id)
			ch <- hit{i, err == nil}
		}(i, p)
	}
	found := -1
	for range ids {
		if h := <-ch; h.ok && (found < 0 || h.i < found) {
			found = h.i
		}
	}
	if found < 0 {
		return config.Profile{}, false
	}
	rememberMessageOwners(e.defaultPath, ids[found].Path, []string{id})
	return ids[found], true
}

// --- which identity a message came to (a small local cache) ---------------

const messageOwnersMax = 500

var messageOwnersMu sync.Mutex

func messageOwnersPath(def string) string {
	return filepath.Join(config.Dir(def), "state", "message-owners.json")
}

type messageOwners struct {
	// Owner maps a message id to the profile file it came to; Order keeps
	// the newest last, for trimming.
	Owner map[string]string `json:"owner"`
	Order []string          `json:"order"`
}

func loadMessageOwners(def string) messageOwners {
	var m messageOwners
	if b, err := os.ReadFile(messageOwnersPath(def)); err == nil {
		json.Unmarshal(b, &m)
	}
	if m.Owner == nil {
		m.Owner = map[string]string{}
	}
	return m
}

func lookupMessageOwner(def, id string) string {
	messageOwnersMu.Lock()
	defer messageOwnersMu.Unlock()
	return loadMessageOwners(def).Owner[id]
}

// rememberMessageOwners notes that ids came to the identity in profile path
// (best effort: a failed write only means a lookup later).
func rememberMessageOwners(def, path string, ids []string) {
	if len(ids) == 0 {
		return
	}
	messageOwnersMu.Lock()
	defer messageOwnersMu.Unlock()
	m := loadMessageOwners(def)
	for _, id := range ids {
		if _, ok := m.Owner[id]; !ok {
			m.Order = append(m.Order, id)
		}
		m.Owner[id] = path
	}
	if over := len(m.Order) - messageOwnersMax; over > 0 {
		for _, id := range m.Order[:over] {
			delete(m.Owner, id)
		}
		m.Order = append([]string(nil), m.Order[over:]...)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	file := messageOwnersPath(def)
	if os.MkdirAll(filepath.Dir(file), 0o700) != nil {
		return
	}
	tmp := file + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, file)
	}
}

// --- inbox wait and pull for every identity -------------------------------

// messageOwner says which identity a message came to.
type messageOwner struct {
	Agent string `json:"agent"`
	Hub   string `json:"hub"`
	// Config is the profile to act as for it (MYCO_CONFIG); commands on the
	// message find it by themselves.
	Config string `json:"config"`
}

func ownerOf(p config.Profile) *messageOwner {
	return &messageOwner{Agent: p.Config.Agent, Hub: p.Config.Hub, Config: p.Path}
}

// ownedMessage is a message with the identity it came to.
type ownedMessage struct {
	api.Message
	As *messageOwner `json:"as,omitempty"`
}

type inboxWaitAllResult struct {
	Messages []api.Message `json:"messages"`
	Hint     string        `json:"hint,omitempty"`
	Timeout  string        `json:"timeout,omitempty"`
	// For: the identity these messages came to (the others may have some too:
	// myco inbox pull takes them all).
	For        *messageOwner   `json:"for"`
	Identities []*messageOwner `json:"identities"`
}

// inboxWaitAll is myco inbox wait in a session that is an agent of several
// workspaces: one long poll per identity, done when any has messages.
func (e *env) inboxWaitAll(ids []config.Profile) error {
	e.identityChosen = true
	duration, raw, label, err := inboxWaitTimeout(e, nil)
	if err != nil {
		return err
	}
	clients := make([]*client.Client, len(ids))
	var labels []string
	all := make([]*messageOwner, len(ids))
	for i, p := range ids {
		if clients[i], err = e.clientFor(p); err != nil {
			return err
		}
		labels = append(labels, identityLabel(p.Config))
		all[i] = ownerOf(p)
		if e.in.Getenv != nil {
			reportSessionRuntime(e.in.Ctx, clients[i], e.in.Getenv)
		}
	}
	targets := make([]sessionUsageTarget, 0, len(ids))
	for i, p := range ids {
		targets = append(targets, sessionUsageTarget{Hub: p.Config.Hub, Agent: p.Config.Agent, Client: clients[i]})
	}
	usage := e.sessionUsage(targets)
	if e.p.Format == FormatText {
		fmt.Fprintf(e.in.Stderr, "waiting up to %s (%s) as %s\n", raw, label, strings.Join(labels, " and "))
	}
	ctx, cancel := context.WithCancel(e.in.Ctx)
	defer cancel()
	type result struct {
		i    int
		msgs []api.Message
		err  error
	}
	ch := make(chan result, len(ids))
	for i, c := range clients {
		go func(i int, c *client.Client) {
			msgs, err := c.WaitPending(ctx, duration)
			ch <- result{i, msgs, err}
		}(i, c)
	}
	var firstErr error
	for range ids {
		r := <-ch
		if r.err != nil {
			if e.in.Ctx.Err() == nil && ctx.Err() == nil {
				fmt.Fprintf(e.in.Stderr, "myco: waiting as %s failed: %v\n", labels[r.i], r.err)
				if firstErr == nil {
					firstErr = r.err
				}
			}
			continue
		}
		if len(r.msgs) == 0 {
			continue
		}
		cancel()
		var mids []string
		for _, m := range r.msgs {
			mids = append(mids, m.ID)
		}
		rememberMessageOwners(e.defaultPath, ids[r.i].Path, mids)
		usage.remember(ids[r.i].Config.Hub, ids[r.i].Config.Agent, r.msgs)
		out := inboxWaitAllResult{Messages: nonNilMsgs(r.msgs), Hint: inboxWaitHint(inboxTaskHint(r.msgs)), Timeout: raw,
			For: all[r.i], Identities: all}
		return e.p.Print(out, func(w io.Writer) {
			fmt.Fprintf(w, "%d message(s) waiting for %s; run: myco inbox pull (it takes every workspace's)\n", len(r.msgs), labels[r.i])
			if taskHint := inboxTaskHint(r.msgs); taskHint != "" {
				fmt.Fprintln(w, taskHint)
			}
			fmt.Fprintln(w, waitAgainHint)
		})
	}
	if e.in.Ctx.Err() != nil {
		return &Error{Code: "interrupted", Message: "the wait was stopped before it finished (e.g. a background-command time limit)",
			Retryable: true, Exit: ExitTimeout}
	}
	_ = firstErr // said on stderr as it happened; the others timed out
	return &Error{Code: "timeout", Message: "no messages arrived", Retryable: true, Exit: ExitTimeout}
}

type inboxPullAllResult struct {
	Messages []ownedMessage `json:"messages"`
	Hint     string         `json:"hint,omitempty"`
	// Errors: identities that could not be pulled for (the others were).
	Errors []string `json:"errors,omitempty"`
}

// inboxPullAll is myco inbox pull for every identity of the session; each
// message says which identity it came to.
func (e *env) inboxPullAll(ids []config.Profile, limit int, wait time.Duration) error {
	e.identityChosen = true
	type result struct {
		msgs []api.Message
		err  error
	}
	res := make([]result, len(ids))
	var wg sync.WaitGroup
	for i, p := range ids {
		wg.Add(1)
		go func(i int, p config.Profile) {
			defer wg.Done()
			c, err := e.clientFor(p)
			if err == nil {
				res[i].msgs, err = c.Pull(e.in.Ctx, limit, wait)
			}
			res[i].err = err
		}(i, p)
	}
	wg.Wait()
	out := inboxPullAllResult{Messages: []ownedMessage{}}
	var plain []api.Message
	failed := 0
	for i, r := range res {
		if r.err != nil {
			failed++
			out.Errors = append(out.Errors, identityLabel(ids[i].Config)+": "+r.err.Error())
			continue
		}
		var mids []string
		for _, m := range r.msgs {
			out.Messages = append(out.Messages, ownedMessage{Message: m, As: ownerOf(ids[i])})
			plain = append(plain, m)
			mids = append(mids, m.ID)
		}
		rememberMessageOwners(e.defaultPath, ids[i].Path, mids)
	}
	if failed == len(ids) {
		return res[0].err
	}
	out.Hint = inboxTaskHint(plain)
	return e.p.Print(out, func(w io.Writer) {
		for _, s := range out.Errors {
			fmt.Fprintln(w, "could not pull as "+s)
		}
		if len(out.Messages) == 0 {
			fmt.Fprintln(w, "inbox empty")
			return
		}
		for i := range out.Messages {
			fmt.Fprintf(w, "[as %s]\n", identityLabel(&config.Config{Agent: out.Messages[i].As.Agent, Hub: out.Messages[i].As.Hub}))
			printMessage(w, &out.Messages[i].Message)
			fmt.Fprintln(w)
		}
		if out.Hint != "" {
			fmt.Fprintln(w, out.Hint)
		}
		fmt.Fprintln(w, "Commands on these messages (ack, progress, reply, …) act as the right agent by themselves.")
	})
}

// --- joining a second workspace -------------------------------------------

// sessionElsewhere are the identities session sessionID has on hubs other
// than hub (those another session took over left out).
func sessionElsewhere(def, sessionID, hub string) []config.Profile {
	var out []config.Profile
	for _, p := range config.SessionProfiles(config.ProfilesDir(def), sessionID) {
		if p.Config.Replaced == "" && !config.SameHub(p.Config.Hub, hub) {
			out = append(out, p)
		}
	}
	return out
}

// copyOnHub is the copy of session sessionID that joined hub before.
func copyOnHub(def, sessionID, hub string) (config.Profile, bool) {
	ps, _ := config.ListProfiles(config.ProfilesDir(def))
	for _, p := range ps {
		if p.Config.CopyOf == sessionID && p.Config.Replaced == "" && config.SameHub(p.Config.Hub, hub) {
			return p, true
		}
	}
	return config.Profile{}, false
}

// forkable: CLIs whose sessions myco can copy.
func forkable(harness string) bool { return harness == "claude" || harness == "codex" }

func identitiesOf(ps []config.Profile) []*messageOwner {
	out := make([]*messageOwner, len(ps))
	for i, p := range ps {
		out[i] = ownerOf(p)
	}
	return out
}

// alsoInNote says, after joining, that the session is another workspace's
// agent too and what that means.
func alsoInNote(elsewhere []config.Profile, mode, agent string) string {
	var labels []string
	for _, p := range elsewhere {
		labels = append(labels, identityLabel(p.Config))
	}
	already := "this session is already " + strings.Join(labels, " and ")
	if mode == "copy" {
		return already + ": a copy of it joined this workspace as " + agent +
			", so each workspace has its own session (the runner wakes the copy for this workspace's messages). To share one session instead: myco join --same-session"
	}
	return already + ": it now answers for both. In this session, myco inbox wait and inbox pull serve every workspace, and a command on a message acts as the agent that got it; other commands act as " +
		identityLabel(elsewhere[0].Config) + " unless " + EnvAgent + "=<agent> picks another. The runner runs one turn at a time on the session."
}
