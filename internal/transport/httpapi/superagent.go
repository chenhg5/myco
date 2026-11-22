package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// A member's superagent uses its own agent
// token on the App's routes. Each route the App uses is listed here:
// allowed (it acts as its owner, marked as through it), or held for its
// owner's approval and carried out once they approve. A route not listed
// is untouched: the superagent is then an ordinary agent there (most such
// routes refuse agent tokens). Nothing here widens what its owner can do.

// SetSuperagents enables superagents.
func (s *Server) SetSuperagents(svc *app.SuperagentService) { s.superagents = svc }

type superTier int

const (
	superAllow superTier = iota + 1
	superAsk
)

// superRule: what a route lets a superagent do, and in words what it is
// ({x} is the route's value x).
type superRule struct {
	tier superTier
	what string
}

func allow(what string) superRule { return superRule{superAllow, what} }
func ask(what string) superRule   { return superRule{superAsk, what} }

// superRules, by route pattern. Sending as its owner (console messages,
// steers, uploads) is never allowed: it speaks as itself. Tokens, invites,
// pairing, keys, members and handovers are not listed: never.
var superRules = map[string]superRule{
	// Reading what its owner sees.
	"GET /v1/overview":                              allow("看全局"),
	"GET /v1/console/recent":                        allow("看最近的对话"),
	"GET /v1/console/conversations":                 allow("看对话"),
	"GET /v1/console/exchanges":                     allow("看 agent 之间的往来"),
	"GET /v1/console/agents/{name}/peers":           allow("看 {name} 和谁来往"),
	"GET /v1/console/messages/{id}/trace":           allow("看任务 {id} 的过程"),
	"GET /v1/console/files/{id}":                    allow("下载文件 {id}"),
	"GET /v1/console/automations":                   allow("看自动化"),
	"GET /v1/console/automations/{id}":              allow("看自动化 {id}"),
	"GET /v1/console/automations/{id}/runs/{run}":   allow("看自动化的一次运行"),
	"GET /v1/console/hidden":                        allow("看隐藏的对话"),
	"GET /v1/machines":                              allow("看电脑"),
	"GET /v1/members":                               allow("看成员"),
	"GET /v1/audit":                                 allow("看审计日志"),
	"GET /v1/agents/{name}/history":                 allow("看 {name} 的变更"),
	"GET /v1/agents/{name}/handovers":               allow("看 {name} 的交接"),
	"GET /v1/projects":                              allow("看项目"),
	"GET /v1/projects/{name}":                       allow("看项目 {name}"),
	"GET /v1/projects/{name}/docs/{doc}":            allow("看项目 {name} 的文档 {doc}"),
	"GET /v1/projects/{name}/files/{file}":          allow("看项目 {name} 的文件 {file}"),
	"POST /v1/console/hidden":                       allow("隐藏对话"),
	"POST /v1/console/messages/{id}/dismiss":        allow("收起任务 {id}"),
	"POST /v1/console/automations/{id}/run":         allow("运行一次自动化 {id}"),
	"PATCH /v1/machines/{id}":                       allow("给电脑 {id} 改名"),
	"POST /v1/machines/{id}/spawn":                  allow("在电脑 {id} 上开一个 agent"),
	"PUT /v1/agents/{name}/run-settings":            allow("改 {name} 的模型和权限"),
	"POST /v1/projects":                             allow("建项目"),
	"PATCH /v1/projects/{name}":                     allow("改项目 {name}"),
	"PUT /v1/projects/{name}/members/{agent}":       allow("把 {agent} 拉进项目 {name}"),
	"GET /v1/projects/{name}/machines":              allow("看项目 {name} 能在哪些电脑上开 agent"),
	"PUT /v1/projects/{name}/machines/{machine}":    allow("让项目 {name} 能在电脑 {machine} 上开 agent"),
	"DELETE /v1/projects/{name}/machines/{machine}": allow("不让项目 {name} 在电脑 {machine} 上开 agent"),
	"PUT /v1/projects/{name}/docs/{doc}":            allow("写项目 {name} 的文档 {doc}"),
	"PUT /v1/projects/{name}/files/{file}":          allow("放文件 {file} 到项目 {name}"),
	"POST /v1/console/automations":                  ask("新建一个自动化"),
	"PATCH /v1/console/automations/{id}":            ask("改自动化 {id}"),
	"DELETE /v1/console/automations/{id}":           ask("删除自动化 {id}"),
	"POST /v1/console/messages/{id}/stop":           ask("停下任务 {id}"),
	"DELETE /v1/projects/{name}":                    ask("删除项目 {name}"),
	"DELETE /v1/projects/{name}/members/{agent}":    ask("把 {agent} 移出项目 {name}"),
	"DELETE /v1/projects/{name}/docs/{doc}":         ask("删除项目 {name} 的文档 {doc}"),
	"DELETE /v1/projects/{name}/files/{file}":       ask("删除项目 {name} 的文件 {file}"),
	"DELETE /v1/agents/{name}":                      ask("删除 agent {name}"),
	"POST /v1/agents/cleanup":                       ask("清理不用的 agent"),
	"POST /v1/agents/{name}/kick":                   ask("重启 {name}"),
	"PUT /v1/agents/{name}/visibility":              ask("改 {name} 谁能看到"),
	"PUT /v1/agents/{name}/owner":                   ask("把 {name} 交给别人"),
	"DELETE /v1/machines/{id}":                      ask("移除电脑 {id}"),
	"PUT /v1/machines/{id}/owner":                   ask("把电脑 {id} 交给别人"),
	"POST /v1/admin/upgrade":                        ask("升级工作区"),
	"PUT /v1/hub/settings":                          ask("改工作区设置"),
}

// superCall is a superagent acting for its owner (on an allowed route, or
// one its owner approved).
type superCall struct {
	agent  *domain.Agent
	member *domain.Member
}

type superKey struct{}

func superOf(r *http.Request) *superCall {
	c, _ := r.Context().Value(superKey{}).(*superCall)
	return c
}

// superagentGate handles a superagent's request on a listed route: it
// returns the request to serve (acting for its owner), or nil when it
// answered already (held for approval).
func (s *Server) superagentGate(w http.ResponseWriter, r *http.Request) *http.Request {
	if s.superagents == nil || superOf(r) != nil || bearer(r) == "" {
		return r
	}
	_, pattern := s.mux.Handler(r)
	what := ""
	rule, ok := superRules[pattern]
	if !ok {
		return r
	}
	a, err := s.agents.Authenticate(r.Context(), bearer(r))
	if err != nil {
		return r // a person's token, or none: as ever
	}
	owner, err := s.superagents.OwnerOf(r.Context(), a.Name)
	if err != nil || owner == nil {
		return r // an ordinary agent
	}
	call := &superCall{agent: a, member: owner}
	if rule.tier == superAllow {
		return r.WithContext(context.WithValue(r.Context(), superKey{}, call))
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "could not read the request: %v", err))
		return nil
	}
	if dryRun(r) {
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "needs_approval": true, "what": whatOf(rule.what, pattern, r.URL.Path)})
		return nil
	}
	what = whatOf(rule.what, pattern, r.URL.Path)
	ap, err := s.superagents.Ask(r.Context(), owner, a.Name, what, r.Header.Get("X-Myco-Reason"), r.Method,
		r.URL.RequestURI(), r.Header.Get("Content-Type"), body)
	if err != nil {
		writeError(w, err)
		return nil
	}
	writeJSON(w, http.StatusConflict, api.Error{
		Code:       "approval_pending",
		Message:    "held for " + owner.Name + "'s approval: " + ap.What + " (" + ap.ID + "). It is carried out when they approve; you get a note either way.",
		Suggestion: "do not try another way; carry on with other work, or end your turn and wait for the note (myco approval list shows it)",
	})
	return nil
}

// whatOf fills a rule's words with the route's values ({x} in pattern),
// read from the path (the router has not filled them in yet).
func whatOf(what, pattern, path string) string {
	_, pat, _ := strings.Cut(pattern, " ")
	ps, xs := strings.Split(strings.Trim(pat, "/"), "/"), strings.Split(strings.Trim(path, "/"), "/")
	// A workspace's routes may sit under a prefix: match from the end.
	for i := 1; i <= len(ps) && i <= len(xs); i++ {
		p, x := ps[len(ps)-i], xs[len(xs)-i]
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			if v, err := url.PathUnescape(x); err == nil {
				x = v
			}
			what = strings.ReplaceAll(what, p, x)
		}
	}
	return what
}

// replayApproval carries out an approved request once, as its owner through
// the superagent.
func (s *Server) replayApproval(ctx context.Context, ap *domain.Approval, owner *domain.Member, agent *domain.Agent) (int, string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	ctx = context.WithValue(ctx, superKey{}, &superCall{agent: agent, member: owner})
	req, err := http.NewRequestWithContext(ctx, ap.Method, ap.Path, bytes.NewReader(ap.Body))
	if err != nil {
		return http.StatusInternalServerError, err.Error()
	}
	if ap.ContentType != "" {
		req.Header.Set("Content-Type", ap.ContentType)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// ---- routes ----

func (s *Server) superagentRoutes(m *http.ServeMux) {
	// The person: who their superagent is; name one; take it away.
	m.HandleFunc("GET /v1/console/superagent", s.admin(s.getSuperagent))
	m.HandleFunc("PUT /v1/console/superagent", s.admin(s.setSuperagent))
	m.HandleFunc("DELETE /v1/console/superagent", s.admin(s.clearSuperagent))
	m.HandleFunc("GET /v1/console/approvals", s.admin(s.listApprovals))
	m.HandleFunc("POST /v1/console/approvals/{id}/approve", s.admin(func(w http.ResponseWriter, r *http.Request) { s.decideApproval(w, r, true) }))
	m.HandleFunc("POST /v1/console/approvals/{id}/deny", s.admin(func(w http.ResponseWriter, r *http.Request) { s.decideApproval(w, r, false) }))
	// The agent: whether it is a superagent, and what it asked for.
	m.HandleFunc("GET /v1/me/superagent", s.agent(s.meSuperagent))
	m.HandleFunc("GET /v1/me/approvals", s.agent(s.myApprovals))
}

func (s *Server) superagentsEnabled(w http.ResponseWriter) bool {
	if s.superagents == nil || s.members == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "superagents are not enabled on this hub").WithSuggestion("upgrade the hub to v0.21.1 or later"))
		return false
	}
	return true
}

func (s *Server) superagentOut(r *http.Request, a *domain.Agent) api.Superagent {
	if a == nil {
		return api.Superagent{}
	}
	ag := s.agentDTO(a)
	return api.Superagent{Agent: &ag}
}

func (s *Server) getSuperagent(w http.ResponseWriter, r *http.Request) {
	if !s.superagentsEnabled(w) {
		return
	}
	actor := s.actorOf(r)
	if actor.Member == nil {
		writeJSON(w, http.StatusOK, api.Superagent{})
		return
	}
	a, err := s.superagents.Of(r.Context(), actor.Member.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	out := s.superagentOut(r, a)
	if list, err := s.superagents.Approvals(r.Context(), app.Actor{Member: actor.Member}, domain.ApprovalPending); err == nil {
		out.Pending = len(list)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setSuperagent(w http.ResponseWriter, r *http.Request) {
	if !s.superagentsEnabled(w) {
		return
	}
	var req api.SuperagentSet
	if !decode(w, r, &req) {
		return
	}
	if dryRun(r) {
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "agent": req.Agent})
		return
	}
	a, err := s.superagents.Set(r.Context(), s.actorOf(r), domain.AgentName(req.Agent))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.superagentOut(r, a))
}

func (s *Server) clearSuperagent(w http.ResponseWriter, r *http.Request) {
	if !s.superagentsEnabled(w) {
		return
	}
	if err := s.superagents.Clear(r.Context(), s.actorOf(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func approvalDTO(a *domain.Approval) api.Approval {
	out := api.Approval{ID: a.ID, Agent: string(a.Agent), What: a.What, Reason: a.Reason, Method: a.Method, Path: a.Path, Kind: a.Kind,
		Status: a.Status, Result: a.Result, ResultBody: a.ResultBody, DecidedBy: a.DecidedBy, CreatedAt: a.CreatedAt,
		ExpiresAt: a.CreatedAt.Add(domain.ApprovalTTL), DecidedAt: timePtr(a.DecidedAt)}
	if len(a.Body) > 0 && len(a.Body) <= 2000 && strings.Contains(a.ContentType, "json") {
		out.Body = string(a.Body)
	}
	if a.Kind != domain.ApprovalKindRequest && json.Valid([]byte(a.Payload)) {
		out.Payload = json.RawMessage(a.Payload)
	}
	return out
}

func (s *Server) listApprovals(w http.ResponseWriter, r *http.Request) {
	if !s.superagentsEnabled(w) {
		return
	}
	list, err := s.superagents.Approvals(r.Context(), s.actorOf(r), r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.ApprovalList{Approvals: []api.Approval{}}
	for _, a := range list {
		out.Approvals = append(out.Approvals, approvalDTO(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request, approve bool) {
	if !s.superagentsEnabled(w) {
		return
	}
	// Approving a typed request may say what to allow of it (less than
	// asked); an empty body allows all of it.
	narrow, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "could not read the request: %v", err))
		return
	}
	a, err := s.superagents.DecideWith(r.Context(), s.actorOf(r), r.PathValue("id"), approve, narrow, s.replayApproval)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, approvalDTO(a))
}

func (s *Server) meSuperagent(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.superagentsEnabled(w) {
		return
	}
	owner, err := s.superagents.OwnerOf(r.Context(), a.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.MeSuperagent{}
	if owner != nil {
		out.Superagent, out.Owner = true, owner.Name
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) myApprovals(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.superagentsEnabled(w) {
		return
	}
	list, err := s.superagents.AgentApprovals(r.Context(), a.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.ApprovalList{Approvals: []api.Approval{}}
	for _, x := range list {
		out.Approvals = append(out.Approvals, approvalDTO(x))
	}
	writeJSON(w, http.StatusOK, out)
}
