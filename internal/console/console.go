// Package console is the backend of the myco console app: the hubs the
// user added (names and URLs in a JSON file, console tokens in the system
// keychain) and a same-origin HTTP API (/app/...) that the web UI calls.
// Requests are forwarded to each hub with its console token; the token
// never reaches the web UI.
package console

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/release"
)

// Hub is one hub the user added. The token is kept in Secrets, not here.
type Hub struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Secrets stores console tokens (the system keychain in the app).
type Secrets interface {
	Get(hubID string) (string, error)
	Set(hubID, token string) error
	Delete(hubID string) error
}

// Store keeps the hub list in a JSON file (mode 600).
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore returns a store backed by path (created on first save).
func NewStore(path string) *Store { return &Store{path: path} }

// DefaultStorePath is hubs.json in the user's config directory
// (~/Library/Application Support/myco on macOS).
func DefaultStorePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "myco", "console-hubs.json"), nil
}

// List returns the hubs in the order they were added.
func (s *Store) List() ([]Hub, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Store) load() ([]Hub, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Hub{}, nil
	}
	if err != nil {
		return nil, err
	}
	var hubs []Hub
	if err := json.Unmarshal(b, &hubs); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path, err)
	}
	if hubs == nil {
		hubs = []Hub{}
	}
	return hubs, nil
}

func (s *Store) save(hubs []Hub) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(hubs, "", "  ")
	f, err := os.CreateTemp(filepath.Dir(s.path), ".console-hubs-*")
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		os.Remove(f.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(f.Name(), s.path); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

// Add appends a hub and returns it with a new ID.
func (s *Store) Add(name, u string) (Hub, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hubs, err := s.load()
	if err != nil {
		return Hub{}, err
	}
	h := Hub{ID: "hub_" + randomHex(6), Name: name, URL: u}
	if err := s.save(append(hubs, h)); err != nil {
		return Hub{}, err
	}
	return h, nil
}

// Remove deletes a hub; false if there was none with that ID.
func (s *Store) Remove(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hubs, err := s.load()
	if err != nil {
		return false, err
	}
	i := slices.IndexFunc(hubs, func(h Hub) bool { return h.ID == id })
	if i < 0 {
		return false, nil
	}
	return true, s.save(slices.Delete(hubs, i, i+1))
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Backend serves the /app/ API.
type Backend struct {
	Store   *Store
	Secrets Secrets
	// Timeout bounds each call to a hub (default 5s; adding a hub 15s).
	Timeout time.Duration
	// NewClient builds a hub client (tests replace it); default client.New.
	NewClient func(hubURL, token string) (*client.Client, error)
}

func (b *Backend) timeout() time.Duration {
	if b.Timeout > 0 {
		return b.Timeout
	}
	return 5 * time.Second
}

func (b *Backend) client(h Hub) (*client.Client, error) {
	tok, err := b.Secrets.Get(h.ID)
	if err != nil {
		return nil, &client.Error{Code: "not_configured", Message: "no console token for hub " + h.Name + " in the keychain",
			Suggestion: "remove the hub and add it again with a console token"}
	}
	nc := b.NewClient
	if nc == nil {
		nc = client.New
	}
	return nc(h.URL, tok)
}

// Handler returns the /app/ API.
func (b *Backend) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /app/hubs", b.listHubs)
	m.HandleFunc("POST /app/hubs", b.addHub)
	m.HandleFunc("DELETE /app/hubs/{id}", b.removeHub)
	m.HandleFunc("GET /app/overview", b.overview)
	m.HandleFunc("GET /app/hubs/{id}/threads/{tid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		msgs, err := c.Thread(r.Context(), r.PathValue("tid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if msgs == nil {
			msgs = []api.Message{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
	}))
	m.HandleFunc("PUT /app/hubs/{id}/agents/{name}/profile", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in struct {
			DisplayName *string `json:"display_name"`
			Description *string `json:"description"`
		}
		if !decode(w, r, &in) {
			return
		}
		a, err := c.UpdateProfile(r.Context(), r.PathValue("name"), api.ProfileRequest{DisplayName: in.DisplayName, Description: in.Description})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	m.HandleFunc("PUT /app/hubs/{id}/agents/{name}/allow_from", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in struct {
			AllowFrom *[]string `json:"allow_from"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.AllowFrom == nil {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "allow_from is required ([] = anyone)"})
			return
		}
		a, err := c.UpdateProfile(r.Context(), r.PathValue("name"), api.ProfileRequest{AllowFrom: in.AllowFrom})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	m.HandleFunc("POST /app/hubs/{id}/agents/{name}/kick", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		res, err := c.KickAgent(r.Context(), r.PathValue("name"), false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/agents/{name}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		res, err := c.RemoveAgent(r.Context(), r.PathValue("name"), false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	return m
}

// withHub resolves {id} to a hub client and bounds the call.
func (b *Backend) withHub(h func(http.ResponseWriter, *http.Request, *client.Client)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hub, err := b.find(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		c, err := b.client(hub)
		if err != nil {
			writeErr(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), b.timeout())
		defer cancel()
		h(w, r.WithContext(ctx), c)
	}
}

func (b *Backend) find(id string) (Hub, error) {
	hubs, err := b.Store.List()
	if err != nil {
		return Hub{}, err
	}
	for _, h := range hubs {
		if h.ID == id {
			return h, nil
		}
	}
	return Hub{}, &client.Error{Status: http.StatusNotFound, Code: "not_found", Message: "no hub " + id + " in this app"}
}

// HubStatus is one entry of GET /app/hubs.
type HubStatus struct {
	Hub
	OK              bool   `json:"ok"`
	Error           string `json:"error,omitempty"`
	HubVersion      string `json:"hub_version,omitempty"`
	UpdateAvailable bool   `json:"update_available,omitempty"`
}

func (b *Backend) listHubs(w http.ResponseWriter, r *http.Request) {
	hubs, err := b.Store.List()
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]HubStatus, len(hubs))
	each(hubs, func(i int, h Hub) {
		out[i] = HubStatus{Hub: h}
		c, err := b.client(h)
		if err != nil {
			out[i].Error = errText(err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), b.timeout())
		defer cancel()
		hl, err := c.Health(ctx)
		if err != nil {
			out[i].Error = errText(err)
			return
		}
		out[i].OK = true
		out[i].HubVersion = hl.ServerVersion
		out[i].UpdateAvailable = release.Newer(hl.LatestVersion, hl.ServerVersion)
	})
	writeJSON(w, http.StatusOK, out)
}

func (b *Backend) addHub(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, URL, Token string }
	if !decode(w, r, &in) {
		return
	}
	in.Name, in.Token = strings.TrimSpace(in.Name), strings.TrimSpace(in.Token)
	u, err := normalizeURL(in.URL)
	if err != nil {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: err.Error(),
			Suggestion: "use the hub's address, e.g. https://hub.example.com"})
		return
	}
	if in.Token == "" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "a console token is required",
			Suggestion: "on the hub machine run: myco console token create --name <device>"})
		return
	}
	if in.Name == "" {
		in.Name = hostOf(u)
	}
	nc := b.NewClient
	if nc == nil {
		nc = client.New
	}
	c, err := nc(u, in.Token)
	if err != nil {
		writeErr(w, err)
		return
	}
	// The token must work, and as an admin (an agent token also reads the
	// overview, but only its own view).
	ctx, cancel := context.WithTimeout(r.Context(), 3*b.timeout())
	defer cancel()
	ov, err := c.Overview(ctx, 1)
	if err != nil {
		writeErr(w, err)
		return
	}
	if ov.Viewer.Kind != "admin" {
		writeErr(w, &client.Error{Status: http.StatusForbidden, Code: "permission_denied",
			Message:    "this is an agent token, not a console token",
			Suggestion: "on the hub machine run: myco console token create --name <device>"})
		return
	}
	h, err := b.Store.Add(in.Name, u)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := b.Secrets.Set(h.ID, in.Token); err != nil {
		_, _ = b.Store.Remove(h.ID)
		writeErr(w, fmt.Errorf("cannot save the token in the keychain: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, HubStatus{Hub: h, OK: true})
}

func (b *Backend) removeHub(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := b.Store.Remove(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		writeErr(w, &client.Error{Status: http.StatusNotFound, Code: "not_found", Message: "no hub " + id + " in this app"})
		return
	}
	_ = b.Secrets.Delete(id) // already gone is fine
	writeJSON(w, http.StatusOK, map[string]any{"removed": id})
}

// HubOverview is one hub's part of GET /app/overview.
type HubOverview struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	OK       bool          `json:"ok"`
	Error    string        `json:"error,omitempty"`
	Overview *api.Overview `json:"overview,omitempty"`
}

func (b *Backend) overview(w http.ResponseWriter, r *http.Request) {
	hubs, err := b.Store.List()
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]HubOverview, len(hubs))
	each(hubs, func(i int, h Hub) {
		out[i] = HubOverview{ID: h.ID, Name: h.Name}
		c, err := b.client(h)
		if err != nil {
			out[i].Error = errText(err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), b.timeout())
		defer cancel()
		ov, err := c.Overview(ctx, 50)
		if err != nil {
			out[i].Error = errText(err)
			return
		}
		out[i].OK, out[i].Overview = true, ov
	})
	writeJSON(w, http.StatusOK, map[string]any{"generated_at": time.Now().UTC(), "hubs": out})
}

// each runs fn for every hub concurrently and waits.
func each(hubs []Hub, fn func(int, Hub)) {
	var wg sync.WaitGroup
	for i, h := range hubs {
		wg.Add(1)
		go func() { defer wg.Done(); fn(i, h) }()
	}
	wg.Wait()
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", errors.New("the hub address is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("not a hub address: %q", raw)
	}
	if u.User != nil {
		return "", errors.New("the hub address must not contain a user name or password")
	}
	return u.String(), nil
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Hostname() != "" {
		return p.Hostname()
	}
	return u
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err == nil {
		err = json.Unmarshal(b, v)
	}
	if err != nil {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "invalid JSON body: " + err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr answers in the hub's error shape, keeping the hub's status code.
func writeErr(w http.ResponseWriter, err error) {
	status, body := http.StatusInternalServerError, map[string]string{"error": "internal", "message": err.Error()}
	var ce *client.Error
	if errors.As(err, &ce) {
		body = map[string]string{"error": ce.Code, "message": ce.Message}
		if ce.Suggestion != "" {
			body["suggestion"] = ce.Suggestion
		}
		switch {
		case ce.Status != 0:
			status = ce.Status
		case ce.Code == "not_configured":
			status = http.StatusConflict
		default: // transport errors: the hub could not be reached
			status = http.StatusBadGateway
		}
	}
	writeJSON(w, status, body)
}

func errText(err error) string {
	var ce *client.Error
	if errors.As(err, &ce) {
		return ce.Message
	}
	return err.Error()
}
