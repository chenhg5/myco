package console

import (
	"net/http"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// Automations (hubs from v0.19.22): the App's 自动化 page, passed to the
// hub's /v1/console/automations as the person signed in there. Older hubs
// answer 404, and the page says so.
//
//	GET    /app/hubs/{id}/automations           {automations}
//	POST   /app/hubs/{id}/automations           AutomationRequest → {automation, webhook_secret?}
//	GET    /app/hubs/{id}/automations/{aid}     {automation, runs}
//	PATCH  /app/hubs/{id}/automations/{aid}     AutomationRequest → {automation, webhook_secret?}
//	DELETE /app/hubs/{id}/automations/{aid}
//	POST   /app/hubs/{id}/automations/{aid}/run AutomationRun
func (b *Backend) automationRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/automations", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		list, err := c.Automations(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		if list == nil {
			list = []api.Automation{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"automations": list})
	}))
	m.HandleFunc("POST /app/hubs/{id}/automations", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.AutomationRequest
		if !decode(w, r, &in) {
			return
		}
		out, err := c.CreateAutomation(r.Context(), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}))
	m.HandleFunc("GET /app/hubs/{id}/automations/{aid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		out, err := c.Automation(r.Context(), r.PathValue("aid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("PATCH /app/hubs/{id}/automations/{aid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.AutomationRequest
		if !decode(w, r, &in) {
			return
		}
		out, err := c.UpdateAutomation(r.Context(), r.PathValue("aid"), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/automations/{aid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.DeleteAutomation(r.Context(), r.PathValue("aid")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("POST /app/hubs/{id}/automations/{aid}/run", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		out, err := c.RunAutomation(r.Context(), r.PathValue("aid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}))
}
