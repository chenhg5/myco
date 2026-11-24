package console

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// Superagents (hubs from v0.21.1), passed to the hub as the person signed in
// there; older hubs answer 404 and the App leaves the entry out.
//
//	GET    /app/hubs/{id}/superagent                     {agent?, pending}
//	PUT    /app/hubs/{id}/superagent                     {agent} → {agent}
//	DELETE /app/hubs/{id}/superagent
//	GET    /app/hubs/{id}/approvals?status=pending       {approvals}
//	POST   /app/hubs/{id}/approvals/{aid}/approve|deny   → the approval ({permissions?, ttl_seconds?} narrows an action_grant)
//	GET    /app/hubs/{id}/action-grants?state=active     {grants}
//	POST   /app/hubs/{id}/action-grants/{gid}/revoke     → the grant
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
		// Approving an agent's request to act on a computer may allow less
		// than it asked ({permissions?, ttl_seconds?}).
		var narrow api.ActionGrantNarrow
		_ = json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&narrow)
		var out *api.Approval
		var err error
		if action == "approve" && (len(narrow.Permissions) > 0 || narrow.TTLSeconds > 0) {
			out, err = c.DecideActionGrant(r.Context(), r.PathValue("aid"), narrow)
		} else {
			out, err = c.DecideApproval(r.Context(), r.PathValue("aid"), action == "approve")
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	// What agents may do on computers now (v0.21.21 hubs), and taking it
	// back.
	m.HandleFunc("GET /app/hubs/{id}/action-grants", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		list, err := c.OwnerActionGrants(r.Context(), r.URL.Query().Get("state"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if list == nil {
			list = []api.ActionGrant{}
		}
		writeJSON(w, http.StatusOK, api.ActionGrantList{Grants: list})
	}))
	m.HandleFunc("POST /app/hubs/{id}/action-grants/{gid}/revoke", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		out, err := c.RevokeActionGrant(r.Context(), r.PathValue("gid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
}
