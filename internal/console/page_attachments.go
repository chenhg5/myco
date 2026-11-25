package console

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/domain"
)

// These routes use the signed-in person's Hub client. The page bridge derives
// pid from the open page, never from untrusted iframe arguments.
func (b *Backend) pageAttachmentRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/pages/{pid}/attachments", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		if name := q.Get("name"); name != "" {
			a, err := c.ResolvePageAttachment(r.Context(), r.PathValue("pid"), name)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, a)
			return
		}
		limit := 100
		if raw := q.Get("limit"); raw != "" {
			var err error
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > 1000 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_argument", "message": "limit must be 1..1000"})
				return
			}
		}
		out, err := c.PageAttachments(r.Context(), r.PathValue("pid"), q.Get("after"), limit)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("POST /app/hubs/{id}/pages/{pid}/attachments", b.withHubTimeout(5*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		out, err := c.PutPageAttachment(r.Context(), client.PageAttachmentUpload{Page: r.PathValue("pid"), Collection: q.Get("collection"), Name: q.Get("name"), LegacyName: q.Get("legacy_name"), ContentType: r.Header.Get("Content-Type"), Key: r.Header.Get("Idempotency-Key")}, r.Body, r.ContentLength)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/pages/{pid}/attachments/{file}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.DeletePageAttachment(r.Context(), r.PathValue("pid"), r.PathValue("file")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("GET /app/hubs/{id}/pages/{pid}/attachments/{file}/content", b.withHubTimeout(5*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		response, err := c.OpenPageAttachment(r.Context(), r.PathValue("pid"), r.PathValue("file"), r.Header.Get("Range"))
		if err != nil {
			writeErr(w, err)
			return
		}
		defer response.Body.Close()
		for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Content-Disposition", "X-Content-Type-Options", "Content-Security-Policy", "Cache-Control"} {
			if value := response.Header.Get(key); value != "" {
				w.Header().Set(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	m.HandleFunc("POST /app/hubs/{id}/pages/{pid}/attachments/{file}/save", b.withHubTimeout(5*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		a, err := c.PageAttachment(r.Context(), r.PathValue("pid"), r.PathValue("file"))
		if err != nil {
			writeErr(w, err)
			return
		}
		response, err := c.OpenPageAttachment(r.Context(), a.Page, a.ID, "")
		if err != nil {
			writeErr(w, err)
			return
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, domain.MaxPageAttachment+1))
		if err != nil {
			writeErr(w, err)
			return
		}
		sum := sha256.Sum256(data)
		if int64(len(data)) != a.Size || hex.EncodeToString(sum[:]) != a.SHA256 {
			writeErr(w, &client.Error{Status: http.StatusBadGateway, Code: "invalid_state", Message: "page attachment content verification failed", Suggestion: "download again"})
			return
		}
		path, err := writeToDownloads(a.Name, bytes.NewReader(data))
		if err != nil {
			writeErr(w, err)
			return
		}
		revealFile(path)
		writeJSON(w, http.StatusOK, map[string]string{"path": path})
	}))
}
