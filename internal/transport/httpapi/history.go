package httpapi

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// A node's card (display name, description, labels, skills) can be changed
// by the agent itself and by people; every change is logged as agent.edit
// with one "field: …" line per field that changed, so its owner can see who
// changed what.

// cardOf is the part of an agent that agent.edit tracks.
type card struct {
	displayName, description string
	labels                   []string
	skills                   []domain.Skill
}

func cardOf(a *domain.Agent) card {
	return card{displayName: a.DisplayName, description: a.Description,
		labels: slices.Clone(a.Labels), skills: slices.Clone(a.Skills)}
}

// cardChanges lists what changed between two cards, one line per field.
func cardChanges(was, now card) []string {
	var out []string
	if was.displayName != now.displayName {
		out = append(out, fmt.Sprintf("display_name: %s → %s", orDash(was.displayName), orDash(now.displayName)))
	}
	if was.description != now.description {
		out = append(out, "description: "+orDash(clip(now.description, 120)))
	}
	if d := setDiff(was.labels, now.labels); d != "" {
		out = append(out, "labels: "+d)
	}
	wasNames, nowNames := domain.SkillNames(was.skills), domain.SkillNames(now.skills)
	d := setDiff(wasNames, nowNames)
	var edited []string
	for _, n := range now.skills {
		for _, w := range was.skills {
			if w.Name == n.Name && (w.Summary != n.Summary || !slices.Equal(w.Examples, n.Examples)) {
				edited = append(edited, "~"+n.Name)
			}
		}
	}
	if len(edited) > 0 {
		d = strings.TrimSpace(d + " " + strings.Join(edited, " "))
	}
	if d != "" {
		out = append(out, "skills: "+d)
	}
	return out
}

// setDiff is "+added −removed" between two lists ("" when equal as sets).
func setDiff(was, now []string) string {
	var parts []string
	for _, n := range now {
		if !slices.Contains(was, n) {
			parts = append(parts, "+"+n)
		}
	}
	for _, w := range was {
		if !slices.Contains(now, w) {
			parts = append(parts, "−"+w)
		}
	}
	return strings.Join(parts, " ")
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// cardBefore reads name's card before a change (writing the error if the
// agent cannot be read).
func (s *Server) cardBefore(w http.ResponseWriter, r *http.Request, name string) (card, bool) {
	a, err := s.agents.Get(r.Context(), name)
	if err != nil {
		writeError(w, err)
		return card{}, false
	}
	return cardOf(a), true
}

// recordEdit logs agent.edit when the card changed. by is the agent itself
// (its own token) or nil for a person (the request's actor).
func (s *Server) recordEdit(r *http.Request, by *domain.Agent, was card, now *domain.Agent) {
	lines := cardChanges(was, cardOf(now))
	if len(lines) == 0 || s.audit == nil {
		return
	}
	actor := s.actorOf(r)
	if by != nil {
		actor = app.Actor{Agent: by}
	}
	s.audit.Record(r.Context(), actor, "agent.edit", string(now.Name), strings.Join(lines, "\n"))
}

// agentHistory: GET /v1/agents/{name}/history — changes to a node (its
// card, visibility, owner, …), newest first, for its owner and admins.
func (s *Server) agentHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.membersEnabled(w) || !s.mayOwnAgent(w, r, name, "read the history of") {
		return
	}
	q := r.URL.Query()
	f := domain.AuditFilter{Target: name}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	list, err := s.audit.List(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.AuditList{Entries: make([]api.AuditEntry, 0, len(list))}
	for _, e := range list {
		if !strings.HasPrefix(e.Action, "agent.") {
			continue
		}
		out.Entries = append(out.Entries, api.AuditEntry{ID: e.ID, At: e.At, Actor: e.ActorName, ActorKind: e.ActorKind,
			Action: e.Action, Target: e.Target, Detail: e.Detail})
	}
	writeJSON(w, http.StatusOK, out)
}
