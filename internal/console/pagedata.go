package console

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
)

// maxPageDownload bounds a file a page saves to Downloads (myco.download);
// a video or a recording (视频剪辑's export) may be larger.
const (
	maxPageDownload = 20 << 20
	maxPageMedia    = 4 << 30
)

// What a page may save to Downloads: documents, pictures, video and sound,
// not programs.
var pageDownloadTypes = map[string]bool{
	".csv": true, ".tsv": true, ".xlsx": true, ".xls": true, ".pdf": true, ".docx": true, ".pptx": true,
	".json": true, ".md": true, ".txt": true, ".html": true, ".png": true, ".jpg": true, ".jpeg": true,
	".svg": true, ".gif": true, ".webp": true, ".zip": true, ".ics": true,
	".mp4": true, ".webm": true, ".mov": true, ".m4a": true, ".mp3": true, ".wav": true,
}

var pageMediaTypes = map[string]bool{".mp4": true, ".webm": true, ".mov": true, ".m4a": true, ".mp3": true, ".wav": true}

// errTooLarge is what a capped reader answers past its limit (the file
// being written is then removed).
var errTooLarge = errors.New("too large")

type capReader struct {
	r    io.Reader
	left int64
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		var one [1]byte
		if n, _ := c.r.Read(one[:]); n > 0 {
			return 0, errTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	return n, err
}

// pageDataRoutes: a page's records and change feed (templates.md), and
// files a page saves to Downloads.
func (b *Backend) pageDataRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/hubs/{id}/pages/{pid}/data", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		l, err := c.PageRecords(r.Context(), r.PathValue("pid"), r.URL.Query().Get("collection"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, l)
	}))
	put := b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.PageRecordWrite
		if !decode(w, r, &in) {
			return
		}
		rec, err := c.PutPageRecord(r.Context(), r.PathValue("pid"), r.PathValue("coll"), r.PathValue("rid"), in.Data, in.IfVersion)
		if err != nil {
			if rec != nil {
				e, _ := client.AsError(err)
				msg := "the record changed"
				if e != nil {
					msg = e.Message
				}
				writeJSON(w, http.StatusConflict, map[string]any{"error": "conflict", "message": msg, "latest": rec})
				return
			}
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rec)
	})
	m.HandleFunc("PUT /app/hubs/{id}/pages/{pid}/data/{coll}/{rid}", put)
	m.HandleFunc("POST /app/hubs/{id}/pages/{pid}/data/{coll}", put)
	m.HandleFunc("DELETE /app/hubs/{id}/pages/{pid}/data/{coll}/{rid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		v, _ := strconv.Atoi(r.URL.Query().Get("if_version"))
		if err := c.DeletePageRecord(r.Context(), r.PathValue("pid"), r.PathValue("coll"), r.PathValue("rid"), v); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("GET /app/hubs/{id}/pages/{pid}/changes", b.withHubTimeout(90*time.Second, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		after, _ := strconv.ParseInt(q.Get("after"), 10, 64)
		wait, _ := strconv.Atoi(q.Get("wait"))
		ch, err := c.PageChanges(r.Context(), r.PathValue("pid"), after, min(max(wait, 0), 30))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, ch)
	}))
	// The workspace's templates (myco's own first) and pages made from one.
	m.HandleFunc("GET /app/hubs/{id}/templates", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		ts, err := c.Templates(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.TemplateList{Templates: ts})
	}))
	m.HandleFunc("GET /app/hubs/{id}/templates/{tid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		t, err := c.Template(r.Context(), r.PathValue("tid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, t)
	}))
	m.HandleFunc("POST /app/hubs/{id}/templates/sync", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		res, err := c.SyncApps(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	m.HandleFunc("POST /app/hubs/{id}/pages/{pid}/upgrade", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		p, err := c.UpgradePage(r.Context(), r.PathValue("pid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}))
	m.HandleFunc("POST /app/hubs/{id}/templates/{tid}/use", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.TemplateUse
		if !decode(w, r, &in) {
			return
		}
		u, err := c.UseTemplate(r.Context(), r.PathValue("tid"), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, u)
	}))
	// A file a page made (an export: CSV, Excel, PDF…), saved to Downloads
	// and shown in Finder / Explorer.
	m.HandleFunc("POST /app/page-download", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(strings.TrimSpace(r.URL.Query().Get("name")))
		if name == "" || name == "." || strings.HasPrefix(name, ".") || !pageDownloadTypes[strings.ToLower(filepath.Ext(name))] {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument",
				Message: "a page saves documents and pictures (csv, xlsx, pdf, docx, pptx, json, md, txt, png, …), with a name"})
			return
		}
		// (streamed to the file: a video can be large)
		limit := int64(maxPageDownload)
		if pageMediaTypes[strings.ToLower(filepath.Ext(name))] {
			limit = maxPageMedia
		}
		path, err := writeToDownloads(name, &capReader{r: r.Body, left: limit})
		if errors.Is(err, errTooLarge) {
			writeErr(w, &client.Error{Status: http.StatusRequestEntityTooLarge, Code: "too_large", Message: fmt.Sprintf("the file is larger than %d MB", limit>>20)})
			return
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		revealExport(path)
		writeJSON(w, http.StatusOK, map[string]string{"path": path})
	})
}
