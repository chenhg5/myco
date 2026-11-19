package httpapi

import (
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// maxWebhookBody bounds what a webhook call may send.
const maxWebhookBody = 16 << 10

// SetAutomations enables /v1/console/automations and /hooks/{id}.
func (s *Server) SetAutomations(a *app.AutomationService) { s.automations = a }

func (s *Server) automationRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/console/automations", s.admin(s.listAutomations))
	m.HandleFunc("POST /v1/console/automations", s.admin(s.createAutomation))
	m.HandleFunc("GET /v1/console/automations/{id}", s.admin(s.getAutomation))
	m.HandleFunc("PATCH /v1/console/automations/{id}", s.admin(s.updateAutomation))
	m.HandleFunc("DELETE /v1/console/automations/{id}", s.admin(s.deleteAutomation))
	m.HandleFunc("POST /v1/console/automations/{id}/run", s.admin(s.runAutomation))
	m.HandleFunc("GET /v1/console/automations/{id}/runs/{run}", s.admin(s.getAutomationRun))
	// Public: the secret is the credential.
	m.HandleFunc("POST /hooks/{id}", s.hook)
}

func (s *Server) automationsEnabled(w http.ResponseWriter) bool {
	if s.automations == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "automations are not enabled on this hub"))
		return false
	}
	return true
}

func (s *Server) caller(r *http.Request) app.Caller {
	name, memberID := s.consoleIdentity(r)
	return app.Caller{Name: name, MemberID: memberID, Manages: s.actorOf(r).Manages()}
}

func automationInput(req api.AutomationRequest) app.AutomationInput {
	in := app.AutomationInput{Name: req.Name, To: req.To, Project: req.Project, Body: req.Body, ClearSchedule: req.ClearSchedule,
		Webhook: req.Webhook, RotateSecret: req.RotateSecret, SkipIfBusy: req.SkipIfBusy, Enabled: req.Enabled,
		Workflow: req.Workflow, Inputs: req.Inputs}
	if req.Schedule != nil {
		cron, tz := req.Schedule.Cron, req.Schedule.TZ
		in.Cron, in.TZ = &cron, &tz
	}
	return in
}

func automationRunDTO(r *domain.AutomationRun) *api.AutomationRun {
	if r == nil {
		return nil
	}
	out := &api.AutomationRun{ID: r.ID, AutomationID: r.AutomationID, At: r.At, Trigger: r.Trigger, Outcome: r.Outcome, MessageID: r.MessageID, Error: r.Error,
		WorkflowRun: r.WorkflowRun}
	if q := r.Request; q != nil {
		out.Request = &api.WebhookRequest{ContentType: q.ContentType, Headers: q.Headers, Body: q.Body, Size: q.Size}
	}
	return out
}

func (s *Server) automationDTO(r *http.Request, o *app.AutomationOut) api.Automation {
	a := o.Automation
	out := api.Automation{ID: a.ID, Name: a.Name, Owner: a.Owner, From: string(a.From), To: string(a.To), Project: a.Project, Body: a.Body,
		Webhook: a.WebhookHash != "", SkipIfBusy: a.SkipIfBusy, Enabled: a.Enabled, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		LastRun: automationRunDTO(o.LastRun), Workflow: a.Workflow, Inputs: a.Inputs}
	if a.Workflow != "" && s.workflows != nil {
		out.WorkflowName = s.workflows.NameOf(r.Context(), a.Workflow)
	}
	if a.Cron != "" {
		out.Schedule = &api.AutomationSchedule{Cron: a.Cron, TZ: a.TZ}
	}
	if out.Webhook {
		out.WebhookURL = s.publicBase(r) + "/hooks/" + a.ID
	}
	if !a.NextRunAt.IsZero() {
		t := a.NextRunAt
		out.NextRunAt = &t
	}
	return out
}

func (s *Server) listAutomations(w http.ResponseWriter, r *http.Request) {
	if !s.automationsEnabled(w) {
		return
	}
	list, err := s.automations.List(r.Context(), s.caller(r))
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.AutomationList{Automations: []api.Automation{}}
	for _, o := range list {
		out.Automations = append(out.Automations, s.automationDTO(r, o))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createAutomation(w http.ResponseWriter, r *http.Request) {
	if !s.automationsEnabled(w) {
		return
	}
	var req api.AutomationRequest
	if !decode(w, r, &req) {
		return
	}
	o, err := s.automations.Create(r.Context(), s.caller(r), automationInput(req))
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "automation.create", o.Automation.ID, o.Automation.Name+" → "+string(o.Automation.To))
	writeJSON(w, http.StatusCreated, api.AutomationResponse{Automation: s.automationDTO(r, o), WebhookSecret: o.Secret})
}

func (s *Server) getAutomation(w http.ResponseWriter, r *http.Request) {
	if !s.automationsEnabled(w) {
		return
	}
	o, runs, err := s.automations.Get(r.Context(), s.caller(r), r.PathValue("id"), domain.MaxAutomationRuns)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.AutomationDetail{Automation: s.automationDTO(r, o), Runs: []api.AutomationRun{}}
	for _, run := range runs {
		dto := automationRunDTO(run)
		if dto.Request != nil {
			dto.Request.Body = "" // (one run at a time: getAutomationRun)
		}
		out.Runs = append(out.Runs, *dto)
	}
	writeJSON(w, http.StatusOK, out)
}

// getAutomationRun is one run, with what its webhook call carried.
func (s *Server) getAutomationRun(w http.ResponseWriter, r *http.Request) {
	if !s.automationsEnabled(w) {
		return
	}
	run, err := s.automations.GetRun(r.Context(), s.caller(r), r.PathValue("id"), r.PathValue("run"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, automationRunDTO(run))
}

func (s *Server) updateAutomation(w http.ResponseWriter, r *http.Request) {
	if !s.automationsEnabled(w) {
		return
	}
	var req api.AutomationRequest
	if !decode(w, r, &req) {
		return
	}
	o, err := s.automations.Update(r.Context(), s.caller(r), r.PathValue("id"), automationInput(req))
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "automation.update", o.Automation.ID, o.Automation.Name)
	writeJSON(w, http.StatusOK, api.AutomationResponse{Automation: s.automationDTO(r, o), WebhookSecret: o.Secret})
}

func (s *Server) deleteAutomation(w http.ResponseWriter, r *http.Request) {
	if !s.automationsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.automations.Delete(r.Context(), s.caller(r), id); err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "automation.delete", id, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runAutomation(w http.ResponseWriter, r *http.Request) {
	if !s.automationsEnabled(w) {
		return
	}
	run, err := s.automations.RunNow(r.Context(), s.caller(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "automation.run", run.AutomationID, run.Outcome)
	writeJSON(w, http.StatusCreated, automationRunDTO(run))
}

// hook is POST /hooks/{id}: the secret in "Authorization: Bearer" (or
// ?token=), the body (at most 16 KiB) goes into the message.
func (s *Server) hook(w http.ResponseWriter, r *http.Request) {
	if !s.automationsEnabled(w) {
		return
	}
	secret := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if secret == "" {
		secret = r.URL.Query().Get("token")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSON(w, http.StatusRequestEntityTooLarge, api.Error{Code: "too_large", Message: "a webhook call's body is at most 16 KiB"})
			return
		}
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "reading the body: %v", err))
		return
	}
	id := r.PathValue("id")
	call := &domain.WebhookRequest{ContentType: r.Header.Get("Content-Type"), Headers: webhookHeaders(r.Header), Body: string(body), Size: len(body)}
	run, err := s.automations.Webhook(r.Context(), id, secret, call)
	if err != nil {
		if domain.IsCode(err, domain.CodeRateLimited) {
			w.Header().Set("Retry-After", strconv.Itoa(int(s.automations.RetryAfter(r.Context(), id)/time.Second)+1))
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, api.WebhookResponse{Run: *automationRunDTO(run)})
}

// webhookHeaders are the headers of a webhook call kept with its run: the
// ones that say what sent it and what it is (User-Agent, X-GitHub-Event,
// X-Request-Id…), never credentials, cookies, signatures or addresses. At
// most 24, each value at most 300 bytes.
func webhookHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(out) >= 24 {
			break
		}
		if !shownHeader(k) {
			continue
		}
		v := strings.Join(h.Values(k), ", ")
		if len(v) > 300 {
			v = v[:300]
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shownHeader(name string) bool {
	k := strings.ToLower(name)
	switch k {
	case "user-agent", "content-type", "content-encoding":
		return true
	}
	if !strings.HasPrefix(k, "x-") {
		return false
	}
	for _, bad := range []string{"auth", "token", "secret", "signature", "key", "cookie", "session", "password",
		"forwarded", "real-ip", "client-ip", "connecting-ip", "original", "proxy"} {
		if strings.Contains(k, bad) {
			return false
		}
	}
	return true
}
