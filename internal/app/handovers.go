package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/myco-ai/myco/internal/domain"
)

// HandoverService hands a node over to another session: the name, its
// conversations, projects and aliases stay; the session behind it changes.
type HandoverService struct {
	repo     domain.HandoverRepository
	agents   domain.AgentRepository
	msgs     domain.MessageRepository
	projects domain.ProjectRepository
	machines domain.MachineRepository
	messages *MessageService
	tokens   TokenIssuer
	ids      IDGenerator
	clock    Clock
}

func NewHandoverService(repo domain.HandoverRepository, agents domain.AgentRepository, msgs domain.MessageRepository,
	tokens TokenIssuer, ids IDGenerator, clock Clock) *HandoverService {
	return &HandoverService{repo: repo, agents: agents, msgs: msgs, tokens: tokens, ids: ids, clock: clock}
}

// SetContext adds what the brief names (projects, machines) and the
// message service told when tasks go back to a queue. All optional.
func (s *HandoverService) SetContext(projects domain.ProjectRepository, machines domain.MachineRepository, messages *MessageService) {
	s.projects, s.machines, s.messages = projects, machines, messages
}

// Handable reports whether a node can be handed over: agents with a
// session, not shell nodes, machine nodes or consoles.
func Handable(a *domain.Agent) error {
	switch a.Binding.Harness {
	case "exec", domain.HarnessMachine:
		return domain.Errf(domain.CodeInvalidArgument, "%s is a %s node: it keeps no conversation to hand over", a.Name, map[string]string{"exec": "shell", domain.HarnessMachine: "machine"}[a.Binding.Harness]).
			WithSuggestion("add another shell node instead")
	}
	return nil
}

// LinkInput starts a handover.
type LinkInput struct {
	Agent string
	By    string
	Note  string
	// Back is the handover to undo (switch back to the session it replaced).
	Back   string
	DryRun bool
}

// CreateLink makes a waiting handover and its one-time take-over link (the
// plain token, shown once).
func (s *HandoverService) CreateLink(ctx context.Context, in LinkInput) (*domain.Handover, string, error) {
	a, err := s.agents.Get(ctx, domain.AgentName(in.Agent))
	if err != nil {
		return nil, "", err
	}
	if err := Handable(a); err != nil {
		return nil, "", err
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > 500 {
		return nil, "", domain.Errf(domain.CodeInvalidArgument, "the note is longer than 500 characters")
	}
	if in.Back != "" {
		b, err := s.repo.Get(ctx, in.Back)
		if err != nil || b.Agent != a.Name || b.State != domain.HandoverDone {
			return nil, "", domain.Errf(domain.CodeNotFound, "%s has no finished handover %s to switch back from", a.Name, in.Back).
				WithSuggestion("list them: myco agent handover-list --name %s", a.Name)
		}
	}
	now := s.clock.Now()
	h := &domain.Handover{ID: s.ids.New("hov"), Agent: a.Name, At: now, By: in.By, Note: note,
		From:  domain.HandoverSide{Binding: a.Binding, MachineID: a.MachineID, Runtime: a.Runtime},
		State: domain.HandoverWaiting, Back: in.Back, ExpiresAt: now.Add(domain.HandoverLinkTTL)}
	if in.DryRun {
		return h, "", nil
	}
	plain, _, err := s.tokens.Issue()
	if err != nil {
		return nil, "", err
	}
	plain = domain.HandoverTokenPrefix + strings.TrimPrefix(plain, "myco_")
	if err := s.repo.Create(ctx, h, s.tokens.Hash(plain)); err != nil {
		return nil, "", err
	}
	return h, plain, nil
}

func (s *HandoverService) checkToken(plain string) error {
	if !strings.HasPrefix(plain, domain.HandoverTokenPrefix) || strings.ContainsAny(plain, "/?# \t\r\n") {
		return domain.Errf(domain.CodeNotFound, "this take-over link has expired or was already used").
			WithSuggestion("ask the node's owner for a new one (myco agent handover-link --name <node>)")
	}
	return nil
}

// Peek returns the waiting handover of a link without using it.
func (s *HandoverService) Peek(ctx context.Context, plain string) (*domain.Handover, error) {
	if err := s.checkToken(plain); err != nil {
		return nil, err
	}
	return s.repo.PeekLink(ctx, s.tokens.Hash(plain), s.clock.Now())
}

// TakeOver binds the session b to the link's node with a new credential
// (returned plain, once) and returns the done handover.
func (s *HandoverService) TakeOver(ctx context.Context, plain string, b domain.Binding) (*domain.Handover, *domain.Agent, string, error) {
	if err := s.checkToken(plain); err != nil {
		return nil, nil, "", err
	}
	if err := b.Validate(); err != nil {
		return nil, nil, "", err
	}
	if b.Harness == "" || b.SessionID == "" {
		return nil, nil, "", domain.Errf(domain.CodeInvalidArgument, "binding harness and session_id are required")
	}
	if b.Harness == "exec" || b.Harness == domain.HarnessMachine {
		return nil, nil, "", domain.Errf(domain.CodeInvalidArgument, "a %s node cannot take a node over", b.Harness)
	}
	token, hash, err := s.tokens.Issue()
	if err != nil {
		return nil, nil, "", err
	}
	h, err := s.repo.TakeOver(ctx, domain.TakeOverInput{LinkHash: s.tokens.Hash(plain), Binding: b, TokenHash: hash, Now: s.clock.Now()})
	if err != nil {
		return nil, nil, "", err
	}
	a, err := s.agents.Get(ctx, h.Agent)
	if err != nil {
		return nil, nil, "", err
	}
	if len(h.OpenTasks) > 0 && s.messages != nil {
		s.messages.queued(a.Name)
	}
	return h, a, token, nil
}

// List returns an agent's handovers, newest first (at most 20).
func (s *HandoverService) List(ctx context.Context, agent string) ([]*domain.Handover, error) {
	a, err := s.agents.Get(ctx, domain.AgentName(agent))
	if err != nil {
		return nil, err
	}
	list, err := s.repo.List(ctx, a.Name, 20)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	for i, h := range list {
		if h.State == domain.HandoverWaiting && !now.Before(h.ExpiresAt) {
			h.State = domain.HandoverExpired
		}
		// The session that took it reports its machine later: the newest
		// done handover shows the node's current one.
		if h.State == domain.HandoverDone && h.To.MachineID == "" && h.To.Binding.SessionID == a.Binding.SessionID && isNewestDone(list, i) {
			h.To.MachineID, h.To.Runtime = a.MachineID, a.Runtime
		}
	}
	return list, nil
}

func isNewestDone(list []*domain.Handover, i int) bool {
	for _, h := range list[:i] {
		if h.State == domain.HandoverDone {
			return false
		}
	}
	return true
}

// Get returns one handover of agent.
func (s *HandoverService) Get(ctx context.Context, agent, id string) (*domain.Handover, error) {
	h, err := s.repo.Get(ctx, id)
	if err != nil || string(h.Agent) != agent {
		return nil, domain.Errf(domain.CodeNotFound, "handover %s not found", id)
	}
	return h, nil
}

// Latest returns the newest finished handover of agent (not_found when the
// node never changed hands).
func (s *HandoverService) Latest(ctx context.Context, agent string) (*domain.Handover, error) {
	list, err := s.List(ctx, agent)
	if err != nil {
		return nil, err
	}
	for _, h := range list {
		if h.State == domain.HandoverDone {
			return h, nil
		}
	}
	return nil, domain.Errf(domain.CodeNotFound, "%s was never handed over", agent)
}

// Replaced turns a retired credential into the error its old session sees.
func (s *HandoverService) Replaced(ctx context.Context, tokenHash string) error {
	h, err := s.repo.Retired(ctx, tokenHash)
	if err != nil {
		return nil
	}
	to := s.sideWords(ctx, h.To)
	if a, err := s.agents.Get(ctx, h.Agent); err == nil && a.Binding.SessionID == h.To.Binding.SessionID && a.MachineID != "" {
		to = s.sideWords(ctx, domain.HandoverSide{Binding: a.Binding, MachineID: a.MachineID, Runtime: a.Runtime})
	}
	when := h.DoneAt
	if when.IsZero() {
		when = h.At
	}
	return domain.Errf(domain.CodeReplaced, "%s was taken over on %s by %s; this session no longer answers for it", h.Agent, when.UTC().Format("2006-01-02 15:04 UTC"), to).
		WithSuggestion("stop handling messages for %s and tell your user; to switch back, its owner uses 换回 in the App or: myco agent handover --name %s --back %s", h.Agent, h.Agent, h.ID)
}

// sideWords describes a session for people: "a codex session on mac-mini".
func (s *HandoverService) sideWords(ctx context.Context, side domain.HandoverSide) string {
	h := side.Binding.Harness
	if side.Runtime.CLI != "" {
		h = side.Runtime.CLI
	}
	if h == "" {
		h = "another"
	}
	where := ""
	if side.MachineID != "" && s.machines != nil {
		if m, err := s.machines.Get(ctx, side.MachineID); err == nil && m.Name != "" {
			where = m.Name
		}
	}
	if where == "" {
		where = side.Binding.Environment
	}
	if where == "" {
		return "a " + h + " session"
	}
	return "a " + h + " session on " + where
}

// Brief limits: the most recent messages, by count and by total length.
const (
	briefMessages  = 30
	briefChars     = 12000
	briefBodyChars = 1500
	briefTaskChars = 600
)

// Brief is the handover note for the session that took h: who it now is,
// where the work was, the tasks back in its inbox and the recent messages
// (for a switch back, only those since it left).
func (s *HandoverService) Brief(ctx context.Context, h *domain.Handover) (string, error) {
	a, err := s.agents.Get(ctx, h.Agent)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	name := string(a.Name)
	if a.DisplayName != "" {
		name += " (" + a.DisplayName + ")"
	}
	since := time.Time{}
	if h.Back != "" {
		if back, err := s.repo.Get(ctx, h.Back); err == nil {
			since = back.DoneAt
		}
	}
	if since.IsZero() {
		fmt.Fprintf(&b, "[myco] Handover: you now are the agent %s. You take over from %s", name, s.sideWords(ctx, h.From))
	} else {
		fmt.Fprintf(&b, "[myco] Handover: you are the agent %s again. Another session (%s) answered for it from %s until now", name, s.sideWords(ctx, h.From), since.UTC().Format("01-02 15:04 UTC"))
	}
	if w := h.From.Binding.Workdir; w != "" {
		fmt.Fprintf(&b, ", which worked in %s", w)
	}
	if m := h.From.Runtime.Model; m != "" {
		fmt.Fprintf(&b, " (model %s)", m)
	}
	b.WriteString(".\n")
	if h.By != "" {
		fmt.Fprintf(&b, "Handed over by %s", h.By)
		if h.Note != "" {
			fmt.Fprintf(&b, ": %q", h.Note)
		}
		b.WriteString(".\n")
	}
	if a.Description != "" {
		fmt.Fprintf(&b, "Its role: %s\n", a.Description)
	}
	b.WriteString("Your name, conversations, projects and who may message you are unchanged; only the session behind the name changed. You do not have the previous session's memory: what follows is what you need to continue.\n")
	if wd := h.From.Binding.Workdir; wd != "" && (wd != h.To.Binding.Workdir || h.From.Binding.Environment != h.To.Binding.Environment) {
		fmt.Fprintf(&b, "The previous directory (%s) may not be on this machine. If the work was in a git repository, clone or pull it and check out the same branch before continuing.\n", wd)
	}
	if s.projects != nil {
		if ms, err := s.projects.MembershipsOf(ctx, a.Name); err == nil && len(ms) > 0 {
			all, _ := s.projects.List(ctx)
			byID := map[string]*domain.Project{}
			for _, p := range all {
				byID[p.ID] = p
			}
			b.WriteString("\nYour projects:\n")
			for _, m := range ms {
				p := byID[m.Project]
				if p == nil || p.Archived {
					continue
				}
				line := "- " + p.Name
				if p.Title != "" {
					line += " (" + p.Title + ")"
				}
				if g := oneLine(p.Goal, 200); g != "" {
					line += ": " + g
				}
				if m.Note != "" {
					line += " — your part: " + oneLine(m.Note, 200)
				}
				b.WriteString(line + "\n")
			}
			b.WriteString("Read a project's brief with: myco project show --name <project>\n")
		}
	}
	open := h.OpenTasks
	if h.State == domain.HandoverWaiting {
		// Not taken yet (the brief goes with the request that starts the
		// session): the tasks that will go back to the queue.
		open = nil
		if list, err := s.msgs.ListOpen(ctx, a.Name, 50); err == nil {
			for _, m := range list {
				if m.Type == domain.TypeTask {
					open = append(open, m.ID)
				}
			}
		}
	}
	if len(open) > 0 {
		b.WriteString("\nOpen tasks (back in your inbox; handle them as usual: ack, progress, reply):\n")
		for _, id := range open {
			m, err := s.msgs.Get(ctx, id)
			if err != nil || m.Status.Terminal() {
				continue
			}
			fmt.Fprintf(&b, "- %s from %s: %s\n", m.ID, m.From, hclip(oneLine(m.Body, 0), briefTaskChars))
			if last := s.lastProgress(ctx, m); last != "" {
				fmt.Fprintf(&b, "  last progress: %s\n", hclip(oneLine(last, 0), 300))
			}
		}
	}
	lines, threads := s.recent(ctx, a.Name, since)
	if len(lines) > 0 {
		if since.IsZero() {
			b.WriteString("\nRecent messages (oldest first):\n")
		} else {
			b.WriteString("\nMessages since you left (oldest first):\n")
		}
		for _, l := range lines {
			b.WriteString(l + "\n")
		}
	}
	if len(threads) > 0 {
		b.WriteString("\nRead more of a conversation: myco thread get --id <thread> (" + strings.Join(threads, ", ") + "), or myco thread export --id <thread> --as md.\n")
	}
	b.WriteString("\nDo this first: run `myco inbox wait` in the background if this session receives that way, then handle your inbox.\n")
	return b.String(), nil
}

func (s *HandoverService) lastProgress(ctx context.Context, task *domain.Message) string {
	list, err := s.msgs.ListThread(ctx, task.ThreadID)
	if err != nil {
		return ""
	}
	last := ""
	for _, m := range list {
		if m.Type == domain.TypeProgress && m.ReplyTo == task.ID {
			last = m.Body
		}
	}
	return last
}

// recent formats the newest messages to or from agent (since, if set):
// at most briefMessages, at most briefChars in all, oldest first; and the
// threads they are in, newest first.
func (s *HandoverService) recent(ctx context.Context, agent domain.AgentName, since time.Time) ([]string, []string) {
	list, err := s.msgs.ListRecent(ctx, agent, 300)
	if err != nil {
		return nil, nil
	}
	keep := []*domain.Message{}
	for _, m := range list {
		switch m.Type {
		case domain.TypeTask, domain.TypeReply, domain.TypeNote, domain.TypeSteer, domain.TypeProgress:
		default:
			continue
		}
		if !since.IsZero() && m.CreatedAt.Before(since) {
			continue
		}
		keep = append(keep, m)
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].CreatedAt.After(keep[j].CreatedAt) })
	var lines []string
	var threads []string
	seen := map[string]bool{}
	total := 0
	for _, m := range keep {
		if len(lines) >= briefMessages {
			break
		}
		body := hclip(strings.TrimSpace(m.Body), briefBodyChars)
		var files []string
		for _, f := range m.Attachments {
			files = append(files, fmt.Sprintf("%s (%d bytes)", f.Name, f.Size))
		}
		if len(files) > 0 {
			body += "\n  attachments: " + strings.Join(files, ", ")
		}
		l := fmt.Sprintf("[%s] %s → %s (%s, thread %s): %s", m.CreatedAt.UTC().Format("01-02 15:04"), m.From, m.To, m.Type, m.ThreadID, body)
		if total+len(l) > briefChars && len(lines) > 0 {
			break
		}
		total += len(l)
		lines = append(lines, l)
		if !seen[m.ThreadID] && len(threads) < 5 {
			seen[m.ThreadID] = true
			threads = append(threads, m.ThreadID)
		}
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines, threads
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 {
		return hclip(s, max)
	}
	return s
}

func hclip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// StillCurrent reports, for a request that authenticated a while ago (a
// long pull), that the agent's credential has not been replaced since.
func (s *HandoverService) StillCurrent(ctx context.Context, a *domain.Agent) error {
	if a == nil || a.TokenHash == "" {
		return nil
	}
	cur, err := s.agents.Get(ctx, a.Name)
	if err != nil || cur.TokenHash == a.TokenHash {
		return nil
	}
	if err := s.Replaced(ctx, a.TokenHash); err != nil {
		return err
	}
	return domain.Errf(domain.CodeUnauthenticated, "this credential of %s was replaced while the request waited", a.Name)
}
