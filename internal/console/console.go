// Package console is the backend of the myco console app: the hubs the
// user added (names and URLs in a JSON file, console tokens in the system
// keychain) and a same-origin HTTP API (/app/...) that the web UI calls.
// Requests are forwarded to each hub with its console token; the token
// never reaches the web UI.
package console

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/release"
)

// maxAttachUpload caps a file the app uploads to attach (read into memory
// here, to seal it when the hub encrypts): the hub's default limit.
const maxAttachUpload = 50 << 20

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
	// Cache, when set, keeps what the pages fetched across restarts
	// (/app/cache); nil leaves the UI with its in-memory data only.
	Cache *Cache

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
	b.cacheRoutes(m)
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
		if since, _ := strconv.ParseInt(r.URL.Query().Get("updated_since"), 10, 64); since > 0 {
			// Only what changed (the hub says since when it could).
			l, err := c.ThreadSince(r.Context(), r.PathValue("tid"), since)
			if err != nil {
				writeErr(w, err)
				return
			}
			if l.Messages == nil {
				l.Messages = []api.Message{}
			}
			writeJSON(w, http.StatusOK, l)
			return
		}
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
			Avatar      *string `json:"avatar"`
		}
		if !decode(w, r, &in) {
			return
		}
		a, err := c.UpdateProfile(r.Context(), r.PathValue("name"), api.ProfileRequest{DisplayName: in.DisplayName, Description: in.Description, Avatar: in.Avatar})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	// An agent's resume line: the command that brings the same agent back in
	// a new sandbox. Making one revokes the one before. Hubs before v0.19.5
	// answer 404 (and the app hides the row).
	m.HandleFunc("POST /app/hubs/{id}/agents/{name}/resume", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		link, err := c.CreateResume(r.Context(), r.PathValue("name"), false)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": link.URL, "command": link.Command})
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/agents/{name}/resume", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.RevokeResume(r.Context(), r.PathValue("name")); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))
	m.HandleFunc("PUT /app/hubs/{id}/agents/{name}/run-settings", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.RunSettings
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, err)
			return
		}
		a, err := c.SetRunSettings(r.Context(), r.PathValue("name"), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	m.HandleFunc("GET /app/hubs/{id}/aliases", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		list, err := c.ListAliases(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.AliasList{Aliases: list})
	}))
	m.HandleFunc("PUT /app/hubs/{id}/aliases/{alias}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.AliasRequest
		if !decode(w, r, &in) {
			return
		}
		out, err := c.SetAlias(r.Context(), r.PathValue("alias"), in.Target, r.URL.Query().Get("dry_run") == "true")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/aliases/{alias}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		out, err := c.RemoveAlias(r.Context(), r.PathValue("alias"), r.URL.Query().Get("dry_run") == "true")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	// Projects of a hub (proxied as they are).
	m.HandleFunc("GET /app/hubs/{id}/projects", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		list, err := c.ListProjects(r.Context(), r.URL.Query().Get("mine") == "true")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.ProjectList{Projects: list})
	}))
	m.HandleFunc("POST /app/hubs/{id}/projects", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.CreateProjectRequest
		if !decode(w, r, &in) {
			return
		}
		p, err := c.CreateProject(r.Context(), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}))
	m.HandleFunc("GET /app/hubs/{id}/projects/{name}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		p, err := c.GetProject(r.Context(), r.PathValue("name"), r.URL.Query().Get("docs") == "body")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}))
	m.HandleFunc("PATCH /app/hubs/{id}/projects/{name}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.UpdateProjectRequest
		if !decode(w, r, &in) {
			return
		}
		p, err := c.UpdateProject(r.Context(), r.PathValue("name"), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/projects/{name}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.DeleteProject(r.Context(), r.PathValue("name")); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}))
	m.HandleFunc("PUT /app/hubs/{id}/projects/{name}/members/{agent}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.ProjectMemberRequest
		if !decode(w, r, &in) {
			return
		}
		p, err := c.PutProjectMember(r.Context(), r.PathValue("name"), r.PathValue("agent"), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/projects/{name}/members/{agent}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		p, err := c.RemoveProjectMember(r.Context(), r.PathValue("name"), r.PathValue("agent"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}))
	m.HandleFunc("GET /app/hubs/{id}/projects/{name}/docs/{doc}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		d, err := c.GetProjectDoc(r.Context(), r.PathValue("name"), r.PathValue("doc"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}))
	m.HandleFunc("PUT /app/hubs/{id}/projects/{name}/docs/{doc}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.ProjectDocRequest
		if !decode(w, r, &in) {
			return
		}
		d, err := c.PutProjectDoc(r.Context(), r.PathValue("name"), r.PathValue("doc"), in.Body)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}))
	m.HandleFunc("DELETE /app/hubs/{id}/projects/{name}/docs/{doc}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		if err := c.DeleteProjectDoc(r.Context(), r.PathValue("name"), r.PathValue("doc")); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}))
	// (The hub's own limit, 50 MB by default, applies after this.)
	// A file to attach to the next task sent from here (the body is the
	// file; ?name= its name): stored on the hub as this console's, sealed
	// when the hub has end-to-end encryption on.
	m.HandleFunc("POST /app/hubs/{id}/files", b.withHubTimeout(5*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		data, err := io.ReadAll(io.LimitReader(r.Body, maxAttachUpload+1))
		if err != nil {
			writeErr(w, err)
			return
		}
		if len(data) > maxAttachUpload {
			writeErr(w, &client.Error{Code: "invalid_argument", Message: fmt.Sprintf("file larger than %d MB", maxAttachUpload>>20)})
			return
		}
		f, err := c.ConsoleUpload(r.Context(), r.URL.Query().Get("name"), r.Header.Get("Content-Type"), data)
		if err != nil {
			writeErr(w, err)
			return
		}
		// Kept here as well: what you sent shows in your chat without the
		// hub giving it back (hubs too old to, a restart).
		if err := b.Cache.PutSent(r.PathValue("id"), f.ID, f.Name, r.Header.Get("Content-Type"), data); err != nil {
			log.Printf("app: keeping a copy of %s: %v", f.ID, err)
		}
		writeJSON(w, http.StatusCreated, f)
	}))
	// Project files: upload (the body is the file), download, and save to
	// this computer's Downloads folder (shown in Finder / Explorer).
	// A file attached to a message you may read: pictures shown in place
	// (only plain image types; anything else is a download, never rendered
	// in the app's page), or saved to Downloads.
	m.HandleFunc("GET /app/hubs/{id}/files/{fid}", b.withHubTimeout(5*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		f, data, err := b.attachment(r, c)
		if err != nil {
			writeErr(w, err)
			return
		}
		ct, disp := "application/octet-stream", "attachment"
		if inlineImage(f.ContentType) && r.URL.Query().Get("download") == "" {
			ct, disp = f.ContentType, "inline"
		}
		// ?as=text: a text file (Markdown, a log, code) for the app to show
		// as text, never as a page. Anything that is not text says so.
		if r.URL.Query().Get("as") == "text" {
			if len(data) > maxTextPreview || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
				writeErr(w, &client.Error{Code: "not_text", Message: "this file cannot be shown as text", Status: http.StatusUnsupportedMediaType})
				return
			}
			ct, disp = "text/plain; charset=utf-8", "inline"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": f.Name}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; sandbox")
		// A file id never names other content: the app may keep it.
		w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
		_, _ = w.Write(data)
	}))
	// An agent's picture face. The app asks with ?v=<its avatar word>, which
	// changes with the picture, so the answer may be kept.
	m.HandleFunc("GET /app/hubs/{id}/agents/{name}/avatar", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		ct, data, err := c.AgentAvatar(r.Context(), r.PathValue("name"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if !inlineImage(ct) {
			writeErr(w, &client.Error{Code: "not_found", Message: "not a picture", Status: http.StatusNotFound})
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		_, _ = w.Write(data)
	}))
	m.HandleFunc("POST /app/hubs/{id}/files/{fid}/save", b.withHubTimeout(5*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		f, data, err := b.attachment(r, c)
		if err != nil {
			writeErr(w, err)
			return
		}
		path, err := writeToDownloads(f.Name, bytes.NewReader(data))
		if err != nil {
			writeErr(w, err)
			return
		}
		revealFile(path)
		writeJSON(w, http.StatusOK, map[string]string{"path": path})
	}))
	m.HandleFunc("PUT /app/hubs/{id}/projects/{name}/files/{file}", b.withHubTimeout(5*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		d, err := c.PutProjectFile(r.Context(), r.PathValue("name"), r.PathValue("file"), r.Header.Get("Content-Type"), r.Body, r.ContentLength)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}))
	m.HandleFunc("GET /app/hubs/{id}/projects/{name}/files/{file}", b.withHubTimeout(5*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		rc, ct, err := c.OpenProjectFile(r.Context(), r.PathValue("name"), r.PathValue("file"))
		if err != nil {
			writeErr(w, err)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": r.PathValue("file")}))
		io.Copy(w, rc)
	}))
	m.HandleFunc("POST /app/hubs/{id}/projects/{name}/files/{file}/save", b.withHubTimeout(5*time.Minute, func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		path, err := saveToDownloads(r.Context(), c, r.PathValue("name"), r.PathValue("file"))
		if err != nil {
			writeErr(w, err)
			return
		}
		revealFile(path)
		writeJSON(w, http.StatusOK, map[string]string{"path": path})
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
	// The steps of a task's run (what the agent says, thinks and does).
	m.HandleFunc("GET /app/hubs/{id}/messages/{mid}/trace", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		after, _ := strconv.Atoi(r.URL.Query().Get("after"))
		res, err := c.ConsoleTrace(r.Context(), r.PathValue("mid"), after)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	m.HandleFunc("POST /app/hubs/{id}/messages/{mid}/steer", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		var in api.SteerRequest
		if !decode(w, r, &in) {
			return
		}
		res, err := c.ConsoleSteerWith(r.Context(), r.PathValue("mid"), in.Body, in.Attachments)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	}))
	m.HandleFunc("POST /app/hubs/{id}/messages/{mid}/stop", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		res, err := c.ConsoleStop(r.Context(), r.PathValue("mid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
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
		project := q.Get("project")
		res, err := c.RecentGrouped(r.Context(), q.Get("group"), q.Get("side"), project, q.Get("scope"), q.Get("label"), hubQuery, before, 0)
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
		if project != "" {
			// A hub before v0.18.26 ignores the filter: keep only what it marks.
			kept := res.Items[:0]
			for _, it := range res.Items {
				if it.Project == project {
					kept = append(kept, it)
				}
			}
			res.Items = kept
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
		if since, _ := strconv.ParseInt(q.Get("updated_since"), 10, 64); since > 0 {
			ex, err := c.ExchangesSince(r.Context(), q.Get("a"), q.Get("b"), since)
			if err != nil {
				writeErr(w, err)
				return
			}
			if ex.Messages == nil {
				ex.Messages = []api.Message{}
			}
			writeJSON(w, http.StatusOK, ex)
			return
		}
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
	m.HandleFunc("DELETE /app/hubs/{id}/machines/{mid}", b.withHub(func(w http.ResponseWriter, r *http.Request, c *client.Client) {
		out, err := c.RemoveMachine(r.Context(), r.PathValue("mid"), r.URL.Query().Get("dry_run") == "true")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
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
		writeJSON(w, http.StatusOK, map[string]any{"join_url": res.JoinURL, "command": "curl -fsSL " + res.JoinURL + " | bash", "agent_guide": res.AgentGuide})
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
		rec := &statusRecorder{ResponseWriter: w}
		h(rec, r.WithContext(ctx), c)
		if rec.status == http.StatusUnauthorized {
			// The hub no longer takes this app's token: what it showed may
			// no longer be this person's to see.
			b.forgetCache(hub.ID)
		} else if rec.status >= http.StatusOK && rec.status < http.StatusMultipleChoices && b.Cache != nil {
			b.Cache.AllowHub(hub.ID)
		}
	}
}

// statusRecorder notes the status a handler answered with.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(p)
}

// Flush passes streaming through (file downloads).
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		if s.status == 0 {
			s.status = http.StatusOK
		}
		f.Flush()
	}
}

func (b *Backend) forgetCache(hubID string) {
	if b.Cache != nil {
		if err := b.Cache.ForgetHub(hubID); err != nil {
			log.Printf("app cache: forget hub: %v", err)
		}
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
	b.forgetCache(id)
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
			var ce *client.Error
			if errors.As(oer, &ce) && ce.Status == http.StatusUnauthorized {
				b.forgetCache(h.ID) // the token no longer works here
			}
			return
		}
		out[i].OK, out[i].Overview, out[i].Me = true, ov, me
		if b.Cache != nil {
			b.Cache.AllowHub(h.ID)
		}
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

// inlineImage reports whether a file of this type may be shown in the
// app's page as a picture: raster types only (an SVG can carry script).
// maxTextPreview bounds a file the app shows as text (longer ones are
// saved instead).
const maxTextPreview = 2 << 20

func inlineImage(contentType string) bool {
	t, _, _ := mime.ParseMediaType(contentType)
	switch t {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "image/bmp":
		return true
	}
	return false
}

// attachment is a message's file: this app's own copy when it sent it,
// else from the hub (opened when it was sealed).
func (b *Backend) attachment(r *http.Request, c *client.Client) (*api.File, []byte, error) {
	if name, ct, data, ok := b.Cache.Sent(r.PathValue("id"), r.PathValue("fid")); ok {
		return &api.File{ID: r.PathValue("fid"), Name: name, Size: int64(len(data)), ContentType: ct}, data, nil
	}
	return c.ConsoleDownload(r.Context(), r.PathValue("fid"))
}
