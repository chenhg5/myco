package console

import (
	"github.com/myco-ai/myco/internal/client"
	"net/http"
)

func (b *Backend) usageRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/usage", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		out, err := c.Usage(r.Context(), client.UsageFilter{From: q.Get("from"), To: q.Get("to"), Project: q.Get("project")})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
}
