package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
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
	m.HandleFunc("GET /v1/workflows/{ref}/stats", s.flow(s.workflowStats))
	m.HandleFunc("GET /v1/workflow-runs", s.flow(s.listRuns))
	m.HandleFunc("GET /v1/workflow-runs/compare", s.flow(s.compareRuns))
	m.HandleFunc("GET /v1/workflow-runs/{id}/steps/{step}/log", s.flow(s.stepLog))
	m.HandleFunc("GET /v1/workflow-runs/{id}/evaluations", s.flow(s.listEvaluations))
	m.HandleFunc("POST /v1/workflow-runs/{id}/evaluations", s.flow(s.evaluateRun))
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
	if o.RunCounts != nil {
		counts := &api.WorkflowRunCounts{Running: o.RunCounts["running"], Waiting: o.RunCounts["waiting"], Succeeded: o.RunCounts["succeeded"], Failed: o.RunCounts["failed"], Cancelled: o.RunCounts["cancelled"]}
		for _, n := range o.RunCounts {
			counts.Total += n
		}
		out.RunCounts = counts
	}
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
	end := r.EndedAt
	if end.IsZero() {
		end = time.Now()
	}
	if end.After(r.CreatedAt) {
		out.DurationMS = end.Sub(r.CreatedAt).Milliseconds()
	}
	if d == nil {
		return out
	}
	out.Definition = defBytes(d.Def)
	m := metricsDTO(d.Metrics)
	out.Metrics = &m
	out.DurationMS = m.DurationMS
	out.Artifacts = []api.RunArtifact{}
	for _, a := range d.Artifacts {
		out.Artifacts = append(out.Artifacts, api.RunArtifact{Step: a.Step, StepRun: a.StepRun, Try: a.Try, Kind: a.Kind, Message: a.Message,
			From: string(a.From), At: a.At, FileID: a.File.FileID, Name: a.File.Name, Size: a.File.Size, ContentType: a.File.ContentType})
	}
	out.Evaluations = []api.RunEvaluation{}
	for _, e := range d.Evaluations {
		out.Evaluations = append(out.Evaluations, evaluationDTO(e))
	}
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
		Bounced: st.Bounced, StartedAt: st.StartedAt, EndedAt: timePtr(st.EndedAt), DueAt: timePtr(st.DueAt),
		Bounces: st.BounceCount(), Attempts: st.Attempts(), Tasks: st.TaskIDs(), DurationMS: domain.StepDurationMS(st, time.Now()), Usage: usageDTO(st.Usage)}
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
	runs, err := s.workflows.RunsWhere(r.Context(), c, q.Get("workflow"), domain.RunFilter{Active: q.Get("active") == "true", Status: q.Get("status"), Limit: limit})
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
	if r.URL.Query().Has("view") {
		s.getRunView(w, r, c)
		return
	}
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

func usageDTO(u *domain.Usage) *api.Usage {
	if u.Zero() {
		return nil
	}
	return &api.Usage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, CostUSD: u.CostUSD}
}

func metricsDTO(m domain.RunMetrics) api.RunMetrics {
	return api.RunMetrics{DurationMS: m.DurationMS, ActiveMS: m.ActiveMS, WaitingMS: m.WaitingMS, Steps: m.Steps, Done: m.Done, Failed: m.Failed,
		Attempts: m.Attempts, Retries: m.Retries, Bounces: m.Bounces, Usage: usageDTO(&m.Usage)}
}

func evaluationDTO(e *domain.RunEvaluation) api.RunEvaluation {
	return api.RunEvaluation{ID: e.ID, Run: e.RunID, Step: e.Step, Name: e.Name, Value: e.Value, Unit: e.Unit, Direction: e.Direction,
		Note: e.Note, By: e.By, At: e.At}
}

func distDTO(d domain.Dist) api.Dist {
	return api.Dist{N: d.N, Avg: d.Avg, Min: d.Min, P50: d.P50, P90: d.P90, P95: d.P95, Max: d.Max}
}

func pointDTO(p app.RunPoint) api.WorkflowRunPoint {
	return api.WorkflowRunPoint{ID: p.ID, Status: p.Status, Version: p.Version, CreatedAt: p.CreatedAt, DurationMS: p.DurationMS, Error: p.Error,
		FailedStep: p.FailedStep, CostUSD: p.Usage.CostUSD}
}

func (s *Server) workflowStats(w http.ResponseWriter, r *http.Request, c app.Caller) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	version, _ := strconv.Atoi(q.Get("version"))
	st, err := s.workflows.Stats(r.Context(), c, r.PathValue("ref"), limit, version)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.WorkflowStats{Workflow: st.Workflow, Name: st.Name, Total: st.Total, ByStatus: st.ByStatus, SuccessRate: st.SuccessRate,
		Sample: st.Sample, Version: st.Version, Duration: distDTO(st.Duration), Active: distDTO(st.Active), Waiting: distDTO(st.Waiting),
		Usage: usageDTO(&st.Usage), CostPerRun: st.CostPerRun, Steps: []api.WorkflowStepStats{}, Versions: []api.WorkflowVersionStats{},
		Evaluations: []api.WorkflowEvalStats{}, Recent: []api.WorkflowRunPoint{}, Failures: []api.WorkflowRunPoint{}}
	for _, x := range st.Steps {
		out.Steps = append(out.Steps, api.WorkflowStepStats{Step: x.Step, Title: x.Title, Kind: x.Kind, Runs: x.Runs, Done: x.Done, Failed: x.Failed,
			Retries: x.Retries, Bounces: x.Bounces, Duration: distDTO(x.Duration), Usage: usageDTO(&x.Usage), LastError: x.LastError,
			LastErrorRun: x.LastErrorRun, LastErrorAt: timePtr(x.LastErrorAt)})
	}
	for _, v := range st.Versions {
		out.Versions = append(out.Versions, api.WorkflowVersionStats{Version: v.Version, Runs: v.Runs, Succeeded: v.Succeeded, Failed: v.Failed,
			Cancelled: v.Cancelled, Duration: distDTO(v.Duration)})
	}
	for _, e := range st.Evaluations {
		out.Evaluations = append(out.Evaluations, api.WorkflowEvalStats{Name: e.Name, Unit: e.Unit, Direction: e.Direction, Values: distDTO(e.Values),
			Last: e.Last, LastRun: e.LastRun})
	}
	for _, p := range st.Recent {
		out.Recent = append(out.Recent, pointDTO(p))
	}
	for _, p := range st.Failures {
		out.Failures = append(out.Failures, pointDTO(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) stepLog(w http.ResponseWriter, r *http.Request, c app.Caller) {
	l, err := s.workflows.StepLog(r.Context(), c, r.PathValue("id"), r.PathValue("step"), r.URL.Query().Get("trace") != "false")
	if err != nil {
		writeError(w, err)
		return
	}
	var def *domain.StepDef
	if d, err := s.workflows.GetRun(r.Context(), c, l.Run.ID); err == nil {
		def = d.Def.Step(l.Step.StepID)
	}
	out := api.StepLog{Run: l.Run.ID, Step: runStepDTO(l.Step, def), Entries: []api.StepLogEntry{}, Clipped: l.Clipped}
	for _, e := range l.Entries {
		x := api.StepLogEntry{At: e.At, Kind: e.Kind, Sub: e.Sub, Try: e.Try, From: e.From, Text: e.Text}
		for _, a := range e.Attachments {
			x.Attachments = append(x.Attachments, api.Attachment{FileID: a.FileID, Name: a.Name, Size: a.Size, SHA256: a.SHA256, ContentType: a.ContentType})
		}
		out.Entries = append(out.Entries, x)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listEvaluations(w http.ResponseWriter, r *http.Request, c app.Caller) {
	d, err := s.workflows.GetRun(r.Context(), c, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := []api.RunEvaluation{}
	for _, e := range d.Evaluations {
		out = append(out, evaluationDTO(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"evaluations": out})
}

func (s *Server) evaluateRun(w http.ResponseWriter, r *http.Request, c app.Caller) {
	var req api.RunEvaluate
	if !decode(w, r, &req) {
		return
	}
	if dryRun(r) {
		e := &domain.RunEvaluation{Name: req.Name, Value: req.Value, Unit: req.Unit, Direction: req.Direction, Note: req.Note}
		if err := e.Validate(); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "evaluation": api.RunEvaluation{Run: r.PathValue("id"), Step: req.Step,
			Name: req.Name, Value: req.Value, Unit: req.Unit, Direction: req.Direction, Note: req.Note}})
		return
	}
	e, err := s.workflows.Evaluate(r.Context(), c, r.PathValue("id"), app.EvaluateInput{Step: req.Step, Name: req.Name, Value: req.Value,
		Unit: req.Unit, Direction: req.Direction, Note: req.Note})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, evaluationDTO(e))
}

func compareSide(x app.RunSide) api.RunCompareSide {
	return api.RunCompareSide{Run: runDTO(x.Run, nil), Metrics: metricsDTO(x.Metrics)}
}

func compareStep(x *app.StepSide) *api.RunCompareStep {
	if x == nil {
		return nil
	}
	return &api.RunCompareStep{Status: x.Status, Runs: x.Runs, Attempts: x.Attempts, DurationMS: x.DurationMS, Usage: usageDTO(&x.Usage),
		Summary: x.Summary, Error: x.Error}
}

func (s *Server) compareRuns(w http.ResponseWriter, r *http.Request, c app.Caller) {
	q := r.URL.Query()
	if q.Get("a") == "" || q.Get("b") == "" {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "name the two runs: ?a=<run>&b=<run>"))
		return
	}
	cmp, err := s.workflows.Compare(r.Context(), c, q.Get("a"), q.Get("b"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.RunComparison{A: compareSide(cmp.A), B: compareSide(cmp.B), SameVersion: cmp.SameVersion,
		Steps: []api.RunCompareStepDelta{}, Evaluations: []api.RunCompareEval{}}
	for _, d := range cmp.Steps {
		out.Steps = append(out.Steps, api.RunCompareStepDelta{Step: d.Step, Title: d.Title, A: compareStep(d.A), B: compareStep(d.B),
			FasterIn: d.FasterIn, DeltaMS: d.DeltaMS, DeltaCost: d.DeltaCost})
	}
	for _, e := range cmp.Evaluations {
		out.Evaluations = append(out.Evaluations, api.RunCompareEval{Name: e.Name, Unit: e.Unit, Direction: e.Direction, A: e.A, B: e.B,
			Delta: e.Delta, Better: e.Better})
	}
	writeJSON(w, http.StatusOK, out)
}
