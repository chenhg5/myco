package console

import (
	"net/http"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// Handovers (「换一个来接着做」, hubs after v0.19.22): another session takes
// a node over, passed to the hub as the person signed in there. Older hubs
// answer 404, and the App hides the entry.
//
//	GET  /app/hubs/{id}/agents/{name}/handovers      {handovers}
//	POST /app/hubs/{id}/agents/{name}/handover-link  HandoverLinkRequest → HandoverLink
//	POST /app/hubs/{id}/agents/{name}/handover       HandoverRequest → HandoverResult (waits for the new session's first turn)
func (b *Backend) handoverRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/agents/{name}/handovers", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		list, err := c.Handovers(r.Context(), r.PathValue("name"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if list == nil {
			list = []api.Handover{}
		}
		writeJSON(w, http.StatusOK, api.HandoverList{Handovers: list})
	}))
	m.HandleFunc("POST /app/hubs/{id}/agents/{name}/handover-link", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.HandoverLinkRequest
		if !decode(w, r, &in) {
			return
		}
		out, err := c.CreateHandoverLink(r.Context(), r.PathValue("name"), in, false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}))
	m.HandleFunc("POST /app/hubs/{id}/agents/{name}/handover", b.withHubTimeout(6*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.HandoverRequest
		if !decode(w, r, &in) {
			return
		}
		out, err := c.HandOver(r.Context(), r.PathValue("name"), in, 5*time.Minute, false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
}
