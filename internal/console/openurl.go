package console

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/myco-ai/myco/internal/client"
)

// openURL: POST /app/open-url {url} opens a link in the default browser
// (the desktop app: its web view does not open new windows, so a link in a
// chat did nothing). Only web and mail links: nothing on this computer, no
// other app's scheme.
func (b *Backend) openURL(w http.ResponseWriter, r *http.Request) {
	if b.OpenURL == nil {
		writeErr(w, &client.Error{Code: "not_supported", Message: "only the desktop app opens links in the browser", Status: http.StatusNotFound})
		return
	}
	var in struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		writeErr(w, &client.Error{Code: "invalid_argument", Message: "a url is required", Status: http.StatusBadRequest})
		return
	}
	if !openable(in.URL) {
		writeErr(w, &client.Error{Code: "invalid_argument", Message: "only http, https and mailto links open in the browser", Status: http.StatusBadRequest})
		return
	}
	if err := b.OpenURL(in.URL); err != nil {
		writeErr(w, &client.Error{Code: "unavailable", Message: "could not open the browser", Status: http.StatusConflict})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// fullscreen: PUT /app/window/fullscreen {on} puts the desktop app's
// window in full screen or takes it out (专注模式). 404 not_supported in a
// browser, which uses its own full screen.
func (b *Backend) fullscreen(w http.ResponseWriter, r *http.Request) {
	if b.Fullscreen == nil {
		writeErr(w, &client.Error{Code: "not_supported", Message: "only the desktop app sets its window's full screen", Status: http.StatusNotFound})
		return
	}
	var in struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil {
		writeErr(w, &client.Error{Code: "invalid_argument", Message: "on (true or false) is required", Status: http.StatusBadRequest})
		return
	}
	if err := b.Fullscreen(in.On); err != nil {
		writeErr(w, &client.Error{Code: "unavailable", Message: "the window is not ready", Status: http.StatusConflict})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// openable: an absolute http(s) link with a host, or a mailto link.
func openable(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 8<<10 || strings.ContainsAny(raw, "\r\n") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return u.Host != ""
	case "mailto":
		return u.Opaque != "" || u.Path != ""
	}
	return false
}
