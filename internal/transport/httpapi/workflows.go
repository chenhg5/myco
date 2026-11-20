package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// SetWorkflows enables /v1/workflows and /v1/workflow-runs.
func (s *Server) SetWorkflows(w *app.WorkflowService) { s.workflows = w }

func (s *Server) workflowRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/workflows", s.flow(s.listWorkflows))
	m.HandleFunc("POST /v1/workflows", s.flow(s.applyWorkflow))
	m.HandleFunc("POST /v1/workflows/validate", s.flow(s.validateWorkflow))
	m.HandleFunc("GET /v1/workflows/{ref}", s.flow(s.getWorkflow))
	m.HandleFunc("PATCH /v1/workflows/{ref}", s.flow(s.updateWorkflow))
	m.HandleFunc("DELETE /v1/workflows/{ref}", s.flow(s.deleteWorkflow))
	m.HandleFunc("POST /v1/workflows/{ref}/runs", s.flow(s.startWorkflow))
	m.HandleFunc("GET /v1/workflow-runs", s.flow(s.listRuns))
	m.HandleFunc("GET /v1/workflow-runs/{id}", s.flow(s.getRun))
	m.HandleFunc("POST /v1/workflow-runs/{id}/cancel", s.flow(s.cancelRun))
	m.HandleFunc("POST /v1/workflow-runs/{id}/retry", s.flow(s.retryRun))
	m.HandleFunc("POST /v1/workflow-runs/{id}/decide", s.flow(s.decideRun))
	m.HandleFunc("GET /v1/workflow-decisions", s.flow(s.listDecisions))
}

// flow lets both the App (a console or admin token) and agents (their own
// token, acting for their person) use workflows.
func (s *Server) flow(h func(http.ResponseWriter, *http.Request, app.Caller)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.workflows == nil {
			writeError(w, domain.Errf(domain.CodeNotFound, "workflows are not enabled on this hub"))
			return
		}
		if s.isAdmin(r) {
			h(w, r, s.caller(r))
			return
		}
		a, err := s.agents.Authenticate(r.Context(), bearer(r))
		if err != nil {
			writeError(w, err)
			return
		}
		h(w, r, s.agentCaller(r, a))
	}
}

// agentCaller: an agent acts for the member who added it; on hubs without
// members, for the workspace's owner (it may see everything).
func (s *Server) agentCaller(r *http.Request, a *domain.Agent) app.Caller {
	c := app.Caller{Name: "admin", MemberID: a.Owner, Agent: a.Name, Manages: a.Owner == ""}
	if a.Owner != "" && s.members != nil {
		if m, err := s.members.Get(r.Context(), a.Owner); err == nil {
			c.Name = m.Name
		}
	}
	return c
}

func defBytes(d *domain.WorkflowDef) json.RawMessage {
	b, _ := json.Marshal(d)
	return b
}

func workflowDTO(o *app.WorkflowOut, withDef bool) api.Workflow {
	w := o.Workflow
	out := api.Workflow{ID: w.ID, Name: w.Name, Title: w.Def.Title, Tags: w.Def.Tags, Group: w.Def.Group, Description: w.Def.Description, Project: w.Project, Owner: w.Owner, Version: w.Version,
		Enabled: w.Enabled, Steps: len(w.Def.Steps), Active: o.Active, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt}
	if o.LastRun != nil {
		r := runDTO(o.LastRun, nil)
		out.LastRun = &r
	}
	if withDef {
		out.Definition = defBytes(w.Def)
	}
	return out
}

func runDTO(r *domain.WorkflowRun, d *app.RunDetail) api.WorkflowRun {
	out := api.WorkflowRun{ID: r.ID, WorkflowID: r.WorkflowID, Name: r.Name, Version: r.Version, Project: r.Project, Status: r.Status,
		Inputs: r.Inputs, Owner: r.Owner, By: r.By, Trigger: r.Trigger, Error: r.Error, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		EndedAt: timePtr(r.EndedAt)}
	if d == nil {
		return out
	}
	out.Definition = defBytes(d.Def)
	for _, st := range d.Steps {
		out.Steps = append(out.Steps, runStepDTO(st, d.Def.Step(st.StepID)))
		if !domain.StepOver(st.Status) {
			out.Current = append(out.Current, st.StepID)
		}
	}
	for _, e := range d.Events {
		out.Events = append(out.Events, api.RunEvent{At: e.At, Kind: e.Kind, Step: e.Step, Text: e.Text})
	}
	return out
}

func runStepDTO(st *domain.RunStep, def *domain.StepDef) api.RunStep {
	out := api.RunStep{ID: st.ID, Step: st.StepID, Kind: st.Kind, Attempt: st.Attempt, Status: st.Status, Target: st.Target,
		MessageID: st.MessageID, Input: st.Input, Payload: st.Payload, Output: st.Output, Summary: st.Summary, Error: st.Error, Retries: st.Retries,
		Bounced: st.Bounced, StartedAt: st.StartedAt, EndedAt: timePtr(st.EndedAt), DueAt: timePtr(st.DueAt)}
	if def != nil {
		out.Title = def.Title
		out.Options = def.Options
	}
	return out
}

func (s *Server) listWorkflows(w http.ResponseWriter, r *http.Request, c app.Caller) {
	list, err := s.workflows.List(r.Context(), c)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.WorkflowList{Workflows: []api.Workflow{}}
	for _, o := range list {
		out.Workflows = append(out.Workflows, workflowDTO(o, false))
	}
	writeJSON(w, http.StatusOK, out)
}

func parseDef(w http.ResponseWriter, r *http.Request) (*domain.WorkflowDef, bool) {
	var req api.WorkflowApplyRequest
	if !decode(w, r, &req) {
		return nil, false
	}
	if len(req.Definition) == 0 {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "definition is required"))
		return nil, false
	}
	var d domain.WorkflowDef
	dec := json.NewDecoder(bytes.NewReader(req.Definition))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "the workflow is not valid: %v", err))
		return nil, false
	}
	return &d, true
}

func (s *Server) validateWorkflow(w http.ResponseWriter, r *http.Request, _ app.Caller) {
	d, ok := parseDef(w, r)
	if !ok {
		return
	}
	chk := s.workflows.Check(r.Context(), d)
	writeJSON(w, http.StatusOK, api.WorkflowCheck{Valid: len(chk.Problems) == 0, Problems: chk.Problems, Warnings: chk.Warnings})
}

func (s *Server) applyWorkflow(w http.ResponseWriter, r *http.Request, c app.Caller) {
	d, ok := parseDef(w, r)
	if !ok {
		return
	}
	dry := dryRun(r)
	out, err := s.workflows.Apply(r.Context(), c, d, dry)
	if err != nil {
		writeError(w, err)
		return
	}
	if out.Changed && !dry {
		s.record(r, "workflow.apply", out.Workflow.ID, out.Workflow.Name+" v"+strconv.Itoa(out.Workflow.Version))
	}
	status := http.StatusOK
	if out.Created && !dry {
		status = http.StatusCreated
	}
	writeJSON(w, status, api.WorkflowApplyResponse{Workflow: workflowDTO(&app.WorkflowOut{Workflow: out.Workflow}, true),
		Created: out.Created, Changed: out.Changed, Warnings: out.Check.Warnings, DryRun: dry})
}

func (s *Server) getWorkflow(w http.ResponseWriter, r *http.Request, c app.Caller) {
	o, err := s.workflows.Get(r.Context(), c, r.PathValue("ref"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workflowDTO(o, true))
}

func (s *Server) updateWorkflow(w http.ResponseWriter, r *http.Request, c app.Caller) {
	var req api.WorkflowUpdate
	if !decode(w, r, &req) {
		return
	}
	ref := r.PathValue("ref")
	if req.Enabled != nil {
		if _, err := s.workflows.SetEnabled(r.Context(), c, ref, *req.Enabled); err != nil {
			writeError(w, err)
			return
		}
	}
	s.getWorkflow(w, r, c)
}

func (s *Server) deleteWorkflow(w http.ResponseWriter, r *http.Request, c app.Caller) {
	ref := r.PathValue("ref")
	if dryRun(r) {
		o, err := s.workflows.Get(r.Context(), c, ref)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workflowDTO(o, false))
		return
	}
	if err := s.workflows.Delete(r.Context(), c, ref); err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "workflow.remove", ref, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startWorkflow(w http.ResponseWriter, r *http.Request, c app.Caller) {
	var req api.WorkflowStart
	if !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	d, pre, err := s.workflows.Start(r.Context(), c, r.PathValue("ref"), app.StartInput{Inputs: req.Inputs, DryRun: dry})
	if err != nil {
		writeError(w, err)
		return
	}
	if pre != nil {
		writeJSON(w, http.StatusOK, api.WorkflowPreview{Inputs: pre.Inputs, Step: pre.Step, Target: pre.Target, Body: pre.Body, DryRun: true})
		return
	}
	writeJSON(w, http.StatusCreated, runDTO(d.Run, d))
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request, c app.Caller) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	runs, err := s.workflows.Runs(r.Context(), c, q.Get("workflow"), q.Get("active") == "true", limit)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.WorkflowRunList{Runs: []api.WorkflowRun{}}
	for _, run := range runs {
		out.Runs = append(out.Runs, runDTO(run, nil))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request, c app.Caller) {
	d, err := s.workflows.GetRun(r.Context(), c, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runDTO(d.Run, d))
}

func (s *Server) runAfter(w http.ResponseWriter, r *http.Request, c app.Caller, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	s.getRun(w, r, c)
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request, c app.Caller) {
	_, err := s.workflows.Cancel(r.Context(), c, r.PathValue("id"))
	s.runAfter(w, r, c, err)
}

func (s *Server) retryRun(w http.ResponseWriter, r *http.Request, c app.Caller) {
	var req api.WorkflowRetry
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	_, err := s.workflows.Retry(r.Context(), c, r.PathValue("id"), req.From)
	s.runAfter(w, r, c, err)
}

func (s *Server) decideRun(w http.ResponseWriter, r *http.Request, c app.Caller) {
	var req api.WorkflowDecide
	if !decode(w, r, &req) {
		return
	}
	_, err := s.workflows.Decide(r.Context(), c, r.PathValue("id"), req.Step, req.Choice, req.Note)
	s.runAfter(w, r, c, err)
}

func (s *Server) listDecisions(w http.ResponseWriter, r *http.Request, c app.Caller) {
	ds, err := s.workflows.Decisions(r.Context(), c)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.WorkflowDecisions{Decisions: []api.WorkflowDecision{}}
	for _, d := range ds {
		out.Decisions = append(out.Decisions, api.WorkflowDecision{Run: runDTO(d.Run, nil), Step: runStepDTO(d.Step, d.Def)})
	}
	writeJSON(w, http.StatusOK, out)
}
