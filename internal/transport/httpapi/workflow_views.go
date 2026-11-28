package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

func projectWorkflowRun(run *domain.WorkflowRun, steps []*domain.RunStep, view, selector string) (*api.WorkflowRunView, error) {
	if view != "compact" && view != "step" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "unknown workflow run view %q", view).WithSuggestion("use view=compact, or view=step&step=<instance-id>")
	}
	if view == "step" {
		if selector == "" {
			return nil, domain.Errf(domain.CodeInvalidArgument, "step view requires a step instance ID").WithSuggestion("select an instance from view=compact")
		}
		var chosen *domain.RunStep
		// Immutable instance IDs take precedence over optional logical names.
		for _, s := range steps {
			if s.RunID == run.ID && s.ID == selector {
				chosen = s
				break
			}
		}
		if chosen == nil {
			for _, s := range steps {
				if s.RunID == run.ID && s.StepID == selector && (chosen == nil || s.Attempt > chosen.Attempt || (s.Attempt == chosen.Attempt && !s.StartedAt.Before(chosen.StartedAt))) {
					chosen = s
				}
			}
		}
		if chosen == nil {
			return nil, domain.Errf(domain.CodeNotFound, "step instance %q not found in workflow run %s", selector, run.ID).WithSuggestion("select a step instance belonging to this run from view=compact")
		}
		steps = []*domain.RunStep{chosen}
	}
	out := &api.WorkflowRunView{ID: run.ID, WorkflowID: run.WorkflowID, Name: run.Name, Version: run.Version, Owner: run.Owner, Project: run.Project, Status: run.Status, Inputs: run.Inputs, By: run.By, Trigger: run.Trigger, Error: run.Error, View: view, Steps: []api.WorkflowStepView{}, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt, EndedAt: timePtr(run.EndedAt)}
	for _, s := range steps {
		if s.RunID != run.ID {
			continue
		}
		st := api.WorkflowStepView{ID: s.ID, Step: s.StepID, Attempt: s.Attempt, Status: s.Status, Error: s.Error, StartedAt: s.StartedAt, EndedAt: timePtr(s.EndedAt)}
		if s.Output != nil {
			data, err := json.Marshal(s.Output)
			if err != nil {
				return nil, domain.Errf(domain.CodeInternal, "cannot encode workflow step output")
			}
			sum := sha256.Sum256(data)
			st.OutputSHA256 = hex.EncodeToString(sum[:])
			if view == "step" {
				st.Output = &s.Output
			}
		}
		out.Steps = append(out.Steps, st)
	}
	return out, nil
}
func (s *Server) getRunView(w http.ResponseWriter, r *http.Request, c app.Caller) {
	view := r.URL.Query().Get("view")
	if view != "compact" && view != "step" {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "unknown workflow run view %q", view).WithSuggestion("use view=compact or view=step&step=<instance-id>"))
		return
	}
	run, steps, err := s.workflows.ReadRunSteps(r.Context(), c, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := projectWorkflowRun(run, steps, view, r.URL.Query().Get("step"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
