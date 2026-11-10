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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
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
	// Timeout bounds each call to a hub (default 12s: a hub far away takes
	// a few seconds a request; adding a hub 15s).
	Timeout time.Duration
	// NewClient builds a hub client (tests replace it); default client.New.
	NewClient func(hubURL, token string) (*client.Client, error)
	// Local, when set, enables /app/local (a hub on this computer).
	Local *Local
	// Version and Shell answer GET /app/info; Shell is "mac" for the app
	// window (hidden title bar) and "browser" for --serve.
	Version, Shell string
	// Update, when set, enables /app/update (the app updating itself).
	Update *Updater

	pending pendingLink
	sealers sealers
	steady  steady
}

func (b *Backend) timeout() time.Duration {
	if b.Timeout > 0 {
		return b.Timeout
	}
	return 12 * time.Second
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
	m.HandleFunc("GET /app/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": b.Version, "shell": orStr(b.Shell, "browser")})
	})
	if b.Update != nil {
		m.HandleFunc("GET /app/update", b.getUpdate)
		m.HandleFunc("POST /app/update", b.postUpdate)
	}
	if b.Local != nil {
		m.HandleFunc("GET /app/local/sessions", b.listLocalSessions)
		m.HandleFunc("POST /app/hubs/{id}/local-join", b.withHubAndHubTimeout(6*time.Minute, b.localJoin))
		m.HandleFunc("GET /app/local", b.getLocal)
		m.HandleFunc("POST /app/local/myco", b.installMyco)
		m.HandleFunc("POST /app/local/hub", b.startLocal)
		m.HandleFunc("POST /app/local/hub/stop", b.stopLocal)
		m.HandleFunc("POST /app/local/runner", b.postLocalRunner)
		m.HandleFunc("POST /app/local/shell-access", b.postShellAccess)
		m.HandleFunc("GET /app/local/providers", b.listProviders)
		m.HandleFunc("GET /app/local/agents/{name}/provider", b.getAgentProvider)
		m.HandleFunc("PUT /app/local/agents/{name}/provider", b.putAgentProvider)
	}
	m.HandleFunc("GET /app/pending-link", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"link": b.takePendingLink()})
	})
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
	// Who can find and use a node: team or private.
	m.HandleFunc("PUT /app/hubs/{id}/agents/{name}/visibility", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.VisibilityRequest
		if !decode(w, r, &in) {
			return
		}
		a, err := c.SetVisibility(r.Context(), r.PathValue("name"), in.Visibility)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	// What a node can be asked to do.
	m.HandleFunc("PUT /app/hubs/{id}/agents/{name}/skills", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.SkillsRequest
		if !decode(w, r, &in) {
			return
		}
		a, err := c.SetSkills(r.Context(), r.PathValue("name"), in.Skills)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	// Groups (group=…) and tags (bare labels) of a node.
	m.HandleFunc("PUT /app/hubs/{id}/agents/{name}/labels", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.LabelsRequest
		if !decode(w, r, &in) {
			return
		}
		a, err := c.SetLabels(r.Context(), r.PathValue("name"), in)
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
	m.HandleFunc("POST /app/hubs/{id}/machines/{mid}/spawn", b.withHubTimeout(6*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.SpawnRequest
		if !decode(w, r, &in) {
			return
		}
		res, err := c.SpawnOnMachine(r.Context(), r.PathValue("mid"), in, 5*time.Minute)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	m.HandleFunc("POST /app/hubs/{id}/messages", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.SendRequest
		if !decode(w, r, &in) {
			return
		}
		res, err := c.ConsoleSend(r.Context(), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, res.Message)
	}))
	m.HandleFunc("POST /app/hubs/{id}/messages/{mid}/steer", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.SteerRequest
		if !decode(w, r, &in) {
			return
		}
		res, err := c.ConsoleSteer(r.Context(), r.PathValue("mid"), in.Body)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	}))
	m.HandleFunc("POST /app/hubs/{id}/messages/{mid}/dismiss", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		res, err := c.ConsoleDismiss(r.Context(), r.PathValue("mid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	m.HandleFunc("GET /app/hubs/{id}/conversations", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
		query := q.Get("q")
		// The hub cannot search encrypted bodies: search the decrypted
		// conversations here instead.
		local := query != "" && c.E2EEnabled(r.Context())
		hubQuery := query
		if local {
			hubQuery = ""
		}
		cs, err := c.Conversations(r.Context(), q.Get("agent"), hubQuery, before, 0)
		if err != nil {
			writeErr(w, err)
			return
		}
		if local {
			cs = filterConversations(cs, query)
		}
		if cs == nil {
			cs = []api.Conversation{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"conversations": cs})
	}))
	// The recent list: threads and shell-node exchanges, newest first.
	m.HandleFunc("GET /app/hubs/{id}/recent", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
		query := strings.TrimSpace(q.Get("q"))
		// Encrypted bodies cannot be searched on the hub: search here.
		local := query != "" && c.E2EEnabled(r.Context())
		hubQuery := query
		if local {
			hubQuery = ""
		}
		res, err := c.Recent(r.Context(), q.Get("scope"), q.Get("label"), hubQuery, before, 0)
		var ce *client.Error
		if errors.As(err, &ce) && (ce.Status == http.StatusNotFound || ce.Status == http.StatusMethodNotAllowed) {
			// A hub from before the recent list: the app shows its
			// conversations instead.
			writeJSON(w, http.StatusOK, RecentResponse{RecentList: api.RecentList{Items: []api.RecentItem{}}, Fallback: true})
			return
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		for i := range res.Items {
			refineRecent(&res.Items[i])
		}
		if local {
			res.Items = filterRecent(res.Items, query)
		}
		if res.Items == nil {
			res.Items = []api.RecentItem{}
		}
		writeJSON(w, http.StatusOK, RecentResponse{RecentList: *res})
	}))
	m.HandleFunc("GET /app/hubs/{id}/settings", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		st, err := c.HubSettings(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	}))
	m.HandleFunc("PUT /app/hubs/{id}/settings", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.HubSettings
		if !decode(w, r, &in) {
			return
		}
		st, err := c.SetHubSettings(r.Context(), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		if c.Sealer != nil {
			_ = c.RegisterKeys(r.Context())
		}
		writeJSON(w, http.StatusOK, st)
	}))
	m.HandleFunc("POST /app/hubs/{id}/agents/cleanup", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in struct {
			OfflineDays int  `json:"offline_days"`
			DryRun      bool `json:"dry_run"`
		}
		if !decode(w, r, &in) {
			return
		}
		res, err := c.CleanupAgents(r.Context(), in.OfflineDays, in.DryRun)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	m.HandleFunc("GET /app/hubs/{id}/agents/{name}/peers", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		ps, err := c.Peers(r.Context(), r.PathValue("name"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if ps == nil {
			ps = []api.Peer{}
		}
		writeJSON(w, http.StatusOK, api.PeerList{Peers: ps})
	}))
	m.HandleFunc("GET /app/hubs/{id}/exchanges", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
		ex, err := c.Exchanges(r.Context(), q.Get("a"), q.Get("b"), before, 0)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, ex)
	}))
	m.HandleFunc("PATCH /app/hubs/{id}/machines/{mid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := c.RenameMachine(r.Context(), r.PathValue("mid"), in.Name); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": r.PathValue("mid"), "name": strings.TrimSpace(in.Name)})
	}))
	// This person's devices on a hub, and one-time links to add another.
	m.HandleFunc("GET /app/hubs/{id}/devices", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		list, err := c.Devices(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.DeviceList{Devices: list})
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/devices/{did}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.RemoveDevice(r.Context(), r.PathValue("did")); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))
	m.HandleFunc("POST /app/hubs/{id}/pairings", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		res, err := c.CreatePairing(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	}))
	// Members (people on the hub), owners and the audit log.
	m.HandleFunc("GET /app/hubs/{id}/members", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		ms, err := c.Members(r.Context(), r.URL.Query().Get("all") == "1")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.MemberList{Members: ms})
	}))
	m.HandleFunc("POST /app/hubs/{id}/members", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.InviteRequest
		if !decode(w, r, &in) {
			return
		}
		res, err := c.InviteMember(r.Context(), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	}))
	m.HandleFunc("PATCH /app/hubs/{id}/members/{member}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.MemberUpdateRequest
		if !decode(w, r, &in) {
			return
		}
		res, err := c.UpdateMember(r.Context(), r.PathValue("member"), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/members/{member}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		res, err := c.RemoveMember(r.Context(), r.PathValue("member"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	m.HandleFunc("PUT /app/hubs/{id}/agents/{name}/owner", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.OwnerRequest
		if !decode(w, r, &in) {
			return
		}
		a, err := c.SetAgentOwner(r.Context(), r.PathValue("name"), in.Owner)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	m.HandleFunc("PUT /app/hubs/{id}/machines/{mid}/owner", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.OwnerRequest
		if !decode(w, r, &in) {
			return
		}
		if err := c.SetMachineOwner(r.Context(), r.PathValue("mid"), in.Owner); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))
	m.HandleFunc("GET /app/hubs/{id}/agents/{name}/history", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		list, err := c.AgentHistory(r.Context(), r.PathValue("name"), limit)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.AuditList{Entries: list})
	}))
	m.HandleFunc("GET /app/hubs/{id}/audit", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		q := r.URL.Query()
		f := client.AuditFilter{Actor: q.Get("actor"), Target: q.Get("target")}
		f.Limit, _ = strconv.Atoi(q.Get("limit"))
		if ms, err := strconv.ParseInt(q.Get("before"), 10, 64); err == nil && ms > 0 {
			f.Before = time.UnixMilli(ms)
		}
		list, err := c.Audit(r.Context(), f)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.AuditList{Entries: list})
	}))
	m.HandleFunc("POST /app/hubs/{id}/links", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in struct {
			Uses         int `json:"uses"`
			ExpiresHours int `json:"expires_hours"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Uses <= 0 {
			in.Uses = 1
		}
		if in.ExpiresHours <= 0 {
			in.ExpiresHours = 24
		}
		res, err := c.CreateEnrollment(r.Context(), api.CreateEnrollmentRequest{Kind: "link", MaxUses: in.Uses,
			TTLSeconds: int64(in.ExpiresHours) * 3600, Note: "myco console"}, false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"join_url": res.JoinURL, "command": "curl -fsSL " + res.JoinURL + " | bash"})
	}))
	return m
}

// withHub resolves {id} to a hub client and bounds the call.
func (b *Backend) withHub(h func(http.ResponseWriter, *http.Request, *client.Client)) http.HandlerFunc {
	return b.withHubTimeout(b.timeout(), h)
}

// withHubAndHubTimeout is withHubTimeout for handlers that also need the hub.
func (b *Backend) withHubAndHubTimeout(d time.Duration, h func(http.ResponseWriter, *http.Request, *client.Client, Hub)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hub, err := b.find(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		b.withHubTimeout(d, func(w http.ResponseWriter, r *http.Request, c *client.Client) { h(w, r, c, hub) })(w, r)
	}
}

// withHubTimeout is withHub with its own bound (starting a session takes
// minutes).
func (b *Backend) withHubTimeout(d time.Duration, h func(http.ResponseWriter, *http.Request, *client.Client)) http.HandlerFunc {
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
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		b.withSealer(ctx, hub.ID, c)
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
		var failed error // a failed health check (not a missing token)
		defer func() { out[i] = b.steady.hubStatus(out[i], failed, time.Now()) }()
		c, err := b.client(h)
		if err != nil {
			out[i].Error = errText(err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), b.timeout())
		defer cancel()
		hl, err := c.Health(ctx)
		if err != nil {
			failed = err
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
	var in struct{ Name, URL, Token, Link string }
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Link) != "" {
		u, tok, err := ParseAppLink(in.Link)
		if err != nil {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: err.Error(),
				Suggestion: "在 Hub 服务器上运行 sudo myco console link 生成一个"})
			return
		}
		in.URL, in.Token = u, tok
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
	// A one-time link: trade its code for this device's own token, so the
	// link is worthless afterwards.
	if strings.HasPrefix(in.Token, app.PairingCodePrefix) {
		pc, err := nc(u, "")
		if err != nil {
			writeErr(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*b.timeout())
		res, err := pc.Pair(ctx, in.Token, b.deviceName())
		cancel()
		if err != nil {
			writeErr(w, err)
			return
		}
		in.Token = res.Token
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
	// The same hub again (say, a new link): keep one entry, new token.
	if hubs, err := b.Store.List(); err == nil {
		for _, h := range hubs {
			if h.URL == u {
				if err := b.Secrets.Set(h.ID, in.Token); err != nil {
					writeErr(w, fmt.Errorf("cannot save the token in the keychain: %w", err))
					return
				}
				writeJSON(w, http.StatusOK, HubStatus{Hub: h, OK: true})
				return
			}
		}
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
	b.forgetSealer(id)
	writeJSON(w, http.StatusOK, map[string]any{"removed": id})
}

// HubOverview is one hub's part of GET /app/overview.
type HubOverview struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	OK       bool          `json:"ok"`
	Error    string        `json:"error,omitempty"`
	Overview *api.Overview `json:"overview,omitempty"`
	// Me is who this app is on the hub (nil for hubs without members).
	Me *api.Whoami `json:"me,omitempty"`
	// E2E: the hub has end-to-end encryption on.
	E2E bool `json:"e2e,omitempty"`
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
		var failed error // a failed overview (not a missing token)
		defer func() { out[i] = b.steady.hubOverview(out[i], failed, time.Now()) }()
		c, err := b.client(h)
		if err != nil {
			out[i].Error = errText(err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), b.timeout())
		defer cancel()
		b.withSealer(ctx, h.ID, c)
		// Three calls at once rather than one after another: a hub far away
		// otherwise needs three round trips within the timeout.
		var (
			wg  sync.WaitGroup
			me  *api.Whoami
			st  *api.HubSettings
			ov  *api.Overview
			oer error
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			if v, err := c.Whoami(ctx); err == nil {
				me = v
			}
		}()
		go func() {
			defer wg.Done()
			if v, err := c.HubSettings(ctx); err == nil {
				st = v
			}
		}()
		ov, oer = c.Overview(ctx, 50)
		wg.Wait()
		if oer != nil {
			failed = oer
			out[i].Error = errText(oer)
			return
		}
		out[i].OK, out[i].Overview, out[i].Me = true, ov, me
		if st != nil {
			out[i].E2E = st.E2E
		}
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

// deviceName names this device's console tokens on hubs.
func (b *Backend) deviceName() string {
	if b.Local != nil && b.Local.Device != "" {
		return b.Local.Device
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return strings.TrimSuffix(h, ".local")
	}
	return "myco app"
}

// RecentResponse is GET /app/hubs/{id}/recent. Fallback: the hub has no
// recent list (older version); show its conversations instead.
type RecentResponse struct {
	api.RecentList
	Fallback bool `json:"fallback,omitempty"`
}

// refineRecent marks a shell command that exited non-zero as failed: the
// hub cannot tell (the exit code is in the reply body, maybe encrypted).
func refineRecent(it *api.RecentItem) {
	if it.Kind != "pair" || it.Answer == nil || it.Answer.Type != "reply" || it.Answer.Unreadable != "" {
		return
	}
	var res api.ExecResult
	if json.Unmarshal([]byte(it.Answer.Body), &res) == nil && res.ExitCode != 0 && it.State == "done" {
		it.State = "failed"
	}
}

// filterRecent keeps the rows whose (decrypted) messages or participants
// contain query.
func filterRecent(items []api.RecentItem, query string) []api.RecentItem {
	q := strings.ToLower(query)
	out := items[:0]
	for _, it := range items {
		hay := []string{it.First.Body, it.Last.Body, it.Other, it.Node}
		hay = append(hay, it.Participants...)
		for _, m := range []*api.Message{it.Task, it.Answer} {
			if m != nil {
				hay = append(hay, m.Body)
			}
		}
		for _, h := range hay {
			if strings.Contains(strings.ToLower(h), q) {
				out = append(out, it)
				break
			}
		}
	}
	return out
}

// filterConversations keeps the conversations whose (decrypted) first or
// last message, or other party, contains query (case-insensitive).
func filterConversations(cs []api.Conversation, query string) []api.Conversation {
	q := strings.ToLower(query)
	out := cs[:0]
	for _, c := range cs {
		if strings.Contains(strings.ToLower(c.First.Body), q) || strings.Contains(strings.ToLower(c.Last.Body), q) ||
			strings.Contains(strings.ToLower(c.Agent), q) {
			out = append(out, c)
		}
	}
	return out
}
