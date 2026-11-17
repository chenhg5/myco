package console

import (
	"net/http"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// pinRoutes: what a conversation keeps at hand (置顶), on its hub.
func (b *Backend) pinRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/pins", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		l, err := c.Pins(r.Context(), q.Get("a"), q.Get("with"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, l)
	}))
	m.HandleFunc("POST /app/hubs/{id}/pins", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.PinCreate
		if !decode(w, r, &in) {
			return
		}
		p, err := c.AddPin(r.Context(), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}))
	m.HandleFunc("PUT /app/hubs/{id}/pins/order", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.PinOrder
		if !decode(w, r, &in) {
			return
		}
		l, err := c.OrderPins(r.Context(), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, l)
	}))
	m.HandleFunc("PATCH /app/hubs/{id}/pins/{pin}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.PinUpdate
		if !decode(w, r, &in) {
			return
		}
		p, err := c.RenamePin(r.Context(), r.PathValue("pin"), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/pins/{pin}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		if err := c.RemovePin(r.Context(), q.Get("a"), q.Get("with"), r.PathValue("pin")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
