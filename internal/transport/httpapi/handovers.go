package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/release"
)

// minHandoverMachine: machines whose myco is not newer than this ignore a
// take-over spawn request (they would start an unrelated new agent), so the
// hub refuses to send one. Dev builds (no version) are let through.
const minHandoverMachine = "v0.19.22"

// SetHandovers enables handing nodes over to other sessions.
func (s *Server) SetHandovers(v *app.HandoverService) {
	s.handovers = v
	if v != nil && s.agents != nil {
		s.agents.SetReplaced(v.Replaced)
	}
}

func (s *Server) handoverRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/agents/{name}/handover-link", s.admin(s.createHandoverLink))
	m.HandleFunc("POST /v1/agents/{name}/handover", s.admin(s.handOver))
	m.HandleFunc("GET /v1/agents/{name}/handovers", s.admin(s.listHandovers))
	m.HandleFunc("GET /v1/me/handover", s.agent(s.myHandover))
	// Public: the link token is the credential.
	m.HandleFunc("POST /v1/takeover", s.takeOver)
	m.HandleFunc("GET /h/{token}", s.handoverPage)
}

func (s *Server) handoversEnabled(w http.ResponseWriter) bool {
	if s.handovers == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "handovers are not available on this hub"))
		return false
	}
	return true
}

// handoverAllowed refuses what a first version cannot do: end-to-end
// encrypted workspaces (the hub cannot read what the note needs).
func (s *Server) handoverAllowed(ctx context.Context) error {
	if s.e2e != nil && s.e2e.Enabled(ctx) {
		return domain.Errf(domain.CodeInvalidState, "handing a node over is not yet available in end-to-end encrypted workspaces").
			WithSuggestion("add the new session as a new agent instead (myco join), or turn end-to-end encryption off for this workspace first")
	}
	return nil
}

func (s *Server) handoverDTO(ctx context.Context, h *domain.Handover) api.Handover {
	out := api.Handover{ID: h.ID, Agent: string(h.Agent), At: h.At, By: h.By, Note: h.Note,
		From: s.handoverSideDTO(ctx, h.From), State: string(h.State), DoneAt: timePtr(h.DoneAt),
		OpenTasks: nonNil(h.OpenTasks), Back: h.Back, ExpiresAt: h.ExpiresAt}
	if h.State == domain.HandoverDone {
		out.To = s.handoverSideDTO(ctx, h.To)
	}
	return out
}

func (s *Server) handoverSideDTO(ctx context.Context, side domain.HandoverSide) api.HandoverSide {
	b := side.Binding
	out := api.HandoverSide{Binding: api.Binding{Harness: b.Harness, SessionID: b.SessionID, Environment: b.Environment, Workdir: b.Workdir},
		MachineID: side.MachineID}
	if side.MachineID != "" && s.machines != nil {
		if m, err := s.machines.Get(ctx, side.MachineID); err == nil {
			out.MachineName = m.Name
		}
	}
	if rt := side.Runtime; rt != (domain.Runtime{}) {
		out.Runtime = &api.Runtime{CLI: rt.CLI, CLIVersion: rt.CLIVersion, Model: rt.Model, Effort: rt.Effort, Provider: rt.Provider}
	}
	return out
}

func (s *Server) handoverLinkOut(r *http.Request, h *domain.Handover, token string, dry bool) api.HandoverLink {
	out := api.HandoverLink{Handover: s.handoverDTO(r.Context(), h), DryRun: dry}
	if token != "" {
		out.URL = s.publicBase(r) + "/h/" + token
		out.Command = "curl -fsSL " + shq(out.URL) + " | sh"
		out.CommandPS1 = "irm " + psq(out.URL+"?format=ps1") + " | iex"
	}
	return out
}

// prepareHandover checks the caller and the node; it returns the agent.
func (s *Server) prepareHandover(w http.ResponseWriter, r *http.Request, verb string) (*domain.Agent, bool) {
	if !s.handoversEnabled(w) {
		return nil, false
	}
	name := r.PathValue("name")
	if !s.mayOwnAgent(w, r, name, verb) {
		return nil, false
	}
	if err := s.handoverAllowed(r.Context()); err != nil {
		writeError(w, err)
		return nil, false
	}
	a, err := s.agents.Get(r.Context(), name)
	if err != nil {
		writeError(w, err)
		return nil, false
	}
	if err := app.Handable(a); err != nil {
		writeError(w, err)
		return nil, false
	}
	return a, true
}

func (s *Server) createHandoverLink(w http.ResponseWriter, r *http.Request) {
	a, ok := s.prepareHandover(w, r, "hand over")
	if !ok {
		return
	}
	var in api.HandoverLinkRequest
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	dry := dryRun(r)
	h, token, err := s.handovers.CreateLink(r.Context(), app.LinkInput{Agent: string(a.Name), By: s.actorOf(r).Name(), Note: in.Note, Back: in.Back, DryRun: dry})
	if err != nil {
		writeError(w, err)
		return
	}
	if !dry {
		s.record(r, "agent.handover.link", string(a.Name), h.ID)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.handoverLinkOut(r, h, token, dry))
}

// resolveMachine finds a machine by id or name.
func (s *Server) resolveMachine(ctx context.Context, ref string) (*domain.Machine, error) {
	if m, err := s.machines.Get(ctx, ref); err == nil {
		return m, nil
	}
	views, _, err := s.machines.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		if strings.EqualFold(v.Name, ref) {
			return s.machines.Get(ctx, v.ID)
		}
	}
	return nil, domain.Errf(domain.CodeNotFound, "machine %s not found", ref).WithSuggestion("list them: myco machine list")
}

func (s *Server) handOver(w http.ResponseWriter, r *http.Request) {
	a, ok := s.prepareHandover(w, r, "hand over")
	if !ok || !s.machinesEnabled(w) {
		return
	}
	var in api.HandoverRequest
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	resume := ""
	if in.Back != "" {
		back, err := s.handovers.Get(ctx, string(a.Name), in.Back)
		if err != nil || back.State != domain.HandoverDone {
			writeError(w, domain.Errf(domain.CodeNotFound, "%s has no finished handover %s to switch back from", a.Name, in.Back).
				WithSuggestion("list them: myco agent handover-list --name %s", a.Name))
			return
		}
		old := back.From
		if old.MachineID == "" || old.Binding.SessionID == "" {
			writeError(w, domain.Errf(domain.CodeInvalidState, "the session %s replaced did not run on a known machine", in.Back).
				WithSuggestion("switch back from that session itself: myco agent handover-link --name %s --back %s, then run the command there", a.Name, in.Back))
			return
		}
		in.Machine, resume, in.Dir = old.MachineID, old.Binding.SessionID, old.Binding.Workdir
		if in.CLI == "" {
			in.CLI = orDefaultStr(old.Runtime.CLI, old.Binding.Harness)
		}
	}
	if strings.TrimSpace(in.Machine) == "" {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "say which machine starts the new session (machine id or name)"))
		return
	}
	m, err := s.resolveMachine(ctx, in.Machine)
	if err != nil {
		writeError(w, err)
		return
	}
	if !s.mayOwnMachineQuiet(r, m) {
		writeError(w, domain.Errf(domain.CodePermissionDenied, "you may not start sessions on machine %s", m.Name))
		return
	}
	if v := m.MycoVersion; v != "" {
		if _, ok := release.Parse(v); ok && !release.Newer(v, minHandoverMachine) {
			writeError(w, domain.Errf(domain.CodeInvalidState, "machine %s runs myco %s, which cannot take a node over", m.Name, v).
				WithSuggestion("upgrade myco on that machine (myco upgrade), or take over with a link: myco agent handover-link --name %s", a.Name))
			return
		}
	}
	dry := dryRun(r)
	h, token, err := s.handovers.CreateLink(ctx, app.LinkInput{Agent: string(a.Name), By: s.actorOf(r).Name(), Note: in.Note, Back: in.Back, DryRun: dry})
	if err != nil {
		writeError(w, err)
		return
	}
	if dry {
		writeJSON(w, http.StatusOK, api.HandoverResult{Handover: s.handoverDTO(ctx, h), DryRun: true})
		return
	}
	brief, err := s.handovers.Brief(ctx, h)
	if err != nil {
		writeError(w, err)
		return
	}
	sr := api.SpawnRequest{CLI: in.CLI, Dir: in.Dir, Model: in.Model, Access: in.Access, Name: string(a.Name),
		TakeOver: token, Brief: brief, ResumeSession: resume}
	body, _ := json.Marshal(sr)
	wait := waitParam(r)
	if wait <= 0 {
		wait = 5 * time.Minute
	}
	s.record(r, "agent.handover", string(a.Name), h.ID+" to "+orNone(in.CLI)+" on "+m.Name)
	reply, err := s.machines.Spawn(ctx, m.ID, body, wait)
	if err != nil {
		writeError(w, err)
		return
	}
	if reply == nil {
		writeError(w, domain.Errf(domain.CodeInvalidState, "the machine did not answer in time").
			WithSuggestion("the session may still be starting; check the handover in a minute: myco agent handover-list --name %s", a.Name))
		return
	}
	var res api.SpawnResult
	if err := json.Unmarshal([]byte(reply.Body), &res); err != nil {
		writeError(w, domain.Errf(domain.CodeInvalidState, "the machine could not start the session: %s", strings.TrimSpace(reply.Body)))
		return
	}
	switch {
	case res.Denied:
		writeError(w, domain.Errf(domain.CodePermissionDenied, "%s", res.Error))
		return
	case res.Error != "":
		writeError(w, domain.Errf(domain.CodeInvalidState, "%s", res.Error))
		return
	}
	if got, err := s.handovers.Get(ctx, string(a.Name), h.ID); err == nil {
		h = got
	}
	writeJSON(w, http.StatusOK, api.HandoverResult{Handover: s.handoverDTO(ctx, h), Spawn: &res})
}

func orDefaultStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// mayOwnMachineQuiet: the machine's owner or a hub manager.
func (s *Server) mayOwnMachineQuiet(r *http.Request, m *domain.Machine) bool {
	return s.actorOf(r).Owns(m.Owner)
}

func (s *Server) listHandovers(w http.ResponseWriter, r *http.Request) {
	if !s.handoversEnabled(w) {
		return
	}
	name := r.PathValue("name")
	if !s.mayOwnAgent(w, r, name, "see the handovers of") {
		return
	}
	list, err := s.handovers.List(r.Context(), name)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.HandoverList{Handovers: []api.Handover{}}
	for _, h := range list {
		out.Handovers = append(out.Handovers, s.handoverDTO(r.Context(), h))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) myHandover(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.handoversEnabled(w) {
		return
	}
	h, err := s.handovers.Latest(r.Context(), string(a.Name))
	if err != nil {
		writeError(w, err)
		return
	}
	brief, err := s.handovers.Brief(r.Context(), h)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.HandoverBrief{Handover: s.handoverDTO(r.Context(), h), Brief: brief})
}

func (s *Server) takeOver(w http.ResponseWriter, r *http.Request) {
	if !s.handoversEnabled(w) {
		return
	}
	var in api.TakeOverRequest
	if !decode(w, r, &in) {
		return
	}
	if err := s.handoverAllowed(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	h, a, token, err := s.handovers.TakeOver(r.Context(), strings.TrimSpace(in.Token), bindingFromAPI(in.Binding))
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, app.Actor{Agent: a}, "agent.handover.take", string(a.Name), h.ID+" by a "+orNone(in.Binding.Harness)+" session")
	brief, err := s.handovers.Brief(r.Context(), h)
	if err != nil {
		brief = "" // taken either way; `myco handover show` reads it again
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, api.TakeOverResponse{Agent: s.agentDTO(r, a), Token: token, Handover: s.handoverDTO(r.Context(), h), Brief: brief})
}

// handoverPage serves a take-over link: a script that installs myco and
// runs `myco join` with the link (sh, PowerShell with ?format=ps1), or a
// short guide (?format=md or a browser). Never echoes whether the token is
// valid to a browser, nor the token in an error.
func (s *Server) handoverPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex")
	if s.handovers == nil {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	h, err := s.handovers.Peek(r.Context(), token)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	hub := strings.TrimRight(s.publicBase(r), "/")
	link := hub + "/h/" + token
	switch r.URL.Query().Get("format") {
	case "md":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		fmt.Fprintf(w, "# Take over the myco agent %s\n\nThis session will answer as %s from now on; the session behind it until now stops. Keep this link private: it is a credential and works once, within 24 hours. Run, in this session:\n\n    curl -fsSL '%s' | sh\n\nIt installs myco if needed, takes the agent over and prints a handover note: read it first, then keep `myco inbox wait` running in the background if no runner wakes this session.\n", h.Agent, h.Agent, link)
	case "ps1":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, `& {
  $ErrorActionPreference = 'Stop'
  $env:MYCO_TAKEOVER = %s
  $env:MYCO_NO_DOCTOR = '1'
  try {
    Invoke-RestMethod 'https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-client.ps1' | Invoke-Expression
    $myco = (Get-Command myco.exe -ErrorAction SilentlyContinue).Source
    if (-not $myco) { $myco = Join-Path $env:LOCALAPPDATA 'Programs\myco\myco.exe' }
    & $myco join --hub %s --format text
  } finally {
    Remove-Item Env:MYCO_TAKEOVER -ErrorAction SilentlyContinue
    Remove-Item Env:MYCO_NO_DOCTOR -ErrorAction SilentlyContinue
  }
}
`, psq(token), psq(hub))
	default:
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		// The token reaches the CLI through the environment, not argv.
		fmt.Fprintf(w, `#!/usr/bin/env sh
# Take over the myco agent %s in this session. The token below is a
# credential: do not paste it into chats or repositories.
set -eu
curl -fsSL --retry 5 --retry-delay 2 https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-client.sh | MYCO_NO_DOCTOR=1 MYCO_ADD_PATH=1 bash
MYCO="$(command -v myco 2>/dev/null || true)"
if [ -z "$MYCO" ]; then MYCO="${MYCO_INSTALL_DIR:-$HOME/.local/bin}/myco"; fi
MYCO_TAKEOVER=%s "$MYCO" join --hub %s --format text
`, h.Agent, shq(token), shq(hub))
	}
}
