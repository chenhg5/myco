package console

import (
	"net/http"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// Superagents (hubs from v0.21.1), passed to the hub as the person signed in
// there; older hubs answer 404 and the App leaves the entry out.
//
//	GET    /app/hubs/{id}/superagent                     {agent?, pending}
//	PUT    /app/hubs/{id}/superagent                     {agent} → {agent}
//	DELETE /app/hubs/{id}/superagent
//	GET    /app/hubs/{id}/approvals?status=pending       {approvals}
//	POST   /app/hubs/{id}/approvals/{aid}/approve|deny   → the approval
func (b *Backend) superagentRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/superagent", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		out, err := c.Superagent(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("PUT /app/hubs/{id}/superagent", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.SuperagentSet
		if !decode(w, r, &in) {
			return
		}
		out, err := c.SetSuperagent(r.Context(), in.Agent, false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/superagent", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.ClearSuperagent(r.Context()); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("GET /app/hubs/{id}/approvals", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		list, err := c.Approvals(r.Context(), r.URL.Query().Get("status"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if list == nil {
			list = []api.Approval{}
		}
		writeJSON(w, http.StatusOK, api.ApprovalList{Approvals: list})
	}))
	m.HandleFunc("POST /app/hubs/{id}/approvals/{aid}/{action}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		action := r.PathValue("action")
		if action != "approve" && action != "deny" {
			http.NotFound(w, r)
			return
		}
		out, err := c.DecideApproval(r.Context(), r.PathValue("aid"), action == "approve")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
}
