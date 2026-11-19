package console

import (
	"net/http"
	"strconv"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// Workflows (hubs from v0.21): the App's 工作流 page, passed to the hub's
// /v1/workflows as the person signed in there. Older hubs answer 404, and
// the page says so.
//
//	GET    /app/hubs/{id}/workflows                 {workflows}
//	POST   /app/hubs/{id}/workflows                 {definition} → WorkflowApplyResponse
//	POST   /app/hubs/{id}/workflows/validate        {definition} → WorkflowCheck
//	GET    /app/hubs/{id}/workflows/{ref}           Workflow (with definition)
//	PATCH  /app/hubs/{id}/workflows/{ref}           {enabled} → Workflow
//	DELETE /app/hubs/{id}/workflows/{ref}
//	POST   /app/hubs/{id}/workflows/{ref}/runs      {inputs} → WorkflowRun
//	GET    /app/hubs/{id}/workflow-runs?workflow=&active=&limit=  {runs}
//	GET    /app/hubs/{id}/workflow-runs/{rid}       WorkflowRun (steps, events, definition)
//	POST   /app/hubs/{id}/workflow-runs/{rid}/cancel|retry|decide
//	GET    /app/hubs/{id}/workflow-decisions        {decisions}
func (b *Backend) workflowRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/workflows", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		list, err := c.Workflows(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		if list == nil {
			list = []api.Workflow{}
		}
		writeJSON(w, http.StatusOK, api.WorkflowList{Workflows: list})
	}))
	m.HandleFunc("POST /app/hubs/{id}/workflows", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.WorkflowApplyRequest
		if !decode(w, r, &in) {
			return
		}
		out, err := c.ApplyWorkflow(r.Context(), in.Definition, false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("POST /app/hubs/{id}/workflows/validate", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.WorkflowApplyRequest
		if !decode(w, r, &in) {
			return
		}
		out, err := c.ValidateWorkflow(r.Context(), in.Definition)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("GET /app/hubs/{id}/workflows/{ref}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		out, err := c.Workflow(r.Context(), r.PathValue("ref"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("PATCH /app/hubs/{id}/workflows/{ref}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.WorkflowUpdate
		if !decode(w, r, &in) {
			return
		}
		if in.Enabled == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled is required"})
			return
		}
		out, err := c.SetWorkflowEnabled(r.Context(), r.PathValue("ref"), *in.Enabled)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/workflows/{ref}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.DeleteWorkflow(r.Context(), r.PathValue("ref"), false); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("POST /app/hubs/{id}/workflows/{ref}/runs", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.WorkflowStart
		if !decode(w, r, &in) {
			return
		}
		run, _, err := c.StartWorkflow(r.Context(), r.PathValue("ref"), in.Inputs, false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, run)
	}))
	m.HandleFunc("GET /app/hubs/{id}/workflow-runs", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		runs, err := c.WorkflowRuns(r.Context(), q.Get("workflow"), q.Get("active") == "true", limit)
		if err != nil {
			writeErr(w, err)
			return
		}
		if runs == nil {
			runs = []api.WorkflowRun{}
		}
		writeJSON(w, http.StatusOK, api.WorkflowRunList{Runs: runs})
	}))
	m.HandleFunc("GET /app/hubs/{id}/workflow-runs/{rid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		out, err := c.WorkflowRun(r.Context(), r.PathValue("rid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("POST /app/hubs/{id}/workflow-runs/{rid}/{action}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		id := r.PathValue("rid")
		var (
			out *api.WorkflowRun
			err error
		)
		switch r.PathValue("action") {
		case "cancel":
			out, err = c.CancelWorkflowRun(r.Context(), id)
		case "retry":
			var in api.WorkflowRetry
			if r.ContentLength != 0 && !decode(w, r, &in) {
				return
			}
			out, err = c.RetryWorkflowRun(r.Context(), id, in.From)
		case "decide":
			var in api.WorkflowDecide
			if !decode(w, r, &in) {
				return
			}
			out, err = c.DecideWorkflowRun(r.Context(), id, in)
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("GET /app/hubs/{id}/workflow-decisions", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		ds, err := c.WorkflowDecisions(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		if ds == nil {
			ds = []api.WorkflowDecision{}
		}
		writeJSON(w, http.StatusOK, api.WorkflowDecisions{Decisions: ds})
	}))
}
