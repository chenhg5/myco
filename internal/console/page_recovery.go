package console

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/client"
)

//go:embed page_recovery.js
var recoveryScript string

//go:embed page_recovery.html
var recoveryDocument string

func (s *pageServer) hasRecovery() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.legacyPort != 0
}

// The retired origin serves only the App's recovery UI. It never serves old
// page files or injects the bridge. The ticket is issued by the console API.
func (s *pageServer) openRecovery() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.legacyPort == 0 {
		return "", &client.Error{Status: http.StatusNotFound, Code: "not_found", Message: "this profile has no old page storage"}
	}
	if s.recovery == nil {
		ln, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(s.legacyPort))
		if err != nil {
			return "", &client.Error{Status: http.StatusServiceUnavailable, Code: "page_origin_unavailable", Message: "the old page storage address is occupied; close the other App and retry"}
		}
		s.recovery = &http.Server{Handler: http.HandlerFunc(s.serveRecovery), ReadHeaderTimeout: 10 * time.Second}
		go func(server *http.Server) { _ = server.Serve(ln) }(s.recovery)
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	s.recoveryTicket = hex.EncodeToString(random[:])
	s.recoveryExpiry = time.Now().Add(10 * time.Minute)
	return fmt.Sprintf("http://127.0.0.1:%d/__myco/recover/%s/", s.legacyPort, s.recoveryTicket), nil
}

func (s *pageServer) serveRecovery(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	host := "127.0.0.1:" + strconv.Itoa(s.legacyPort)
	prefix := "/__myco/recover/" + s.recoveryTicket + "/"
	fresh := s.recoveryTicket != "" && time.Now().Before(s.recoveryExpiry)
	s.mu.Unlock()
	file, valid := strings.CutPrefix(r.URL.Path, prefix)
	if !fresh || r.Host != host || !valid || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; form-action 'none'; base-uri 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	switch file {
	case "":
		h.Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, recoveryDocument)
	case "recover.js":
		h.Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = io.WriteString(w, recoveryScript)
	default:
		http.NotFound(w, r)
	}
}
