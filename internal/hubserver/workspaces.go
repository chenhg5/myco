package hubserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
)

// One hub process can serve several workspaces: the hub itself (the default
// workspace, at the root) and others under /w/<name>. Each is a hub of its
// own, with its own database and files in <data dir>/workspaces/<name>/, so
// nothing is shared between them but the process, the address and the
// server's admin token (which is an admin in every workspace).
//
// The server's owner (the admin token, or the default workspace's owner)
// creates, lists and removes them at /v1/server/workspaces.

// WorkspacesDir is where a hub at dbPath keeps its other workspaces.
func WorkspacesDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "workspaces")
}

var workspaceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// ValidWorkspaceName reports whether name can name a workspace: lowercase
// letters, digits and '-', up to 40, not starting with '-'.
func ValidWorkspaceName(name string) bool {
	return workspaceName.MatchString(name) && name != "default"
}

// workspaces routes requests to the default hub or one of the others.
type workspaces struct {
	ctx  context.Context
	base Options // the default hub's options; each workspace derives its own
	dir  string
	def  *Hub
	mu   sync.RWMutex
	hubs map[string]*workspaceHub
	// root is the hub's public URL ("" = derived from requests), applied to
	// every workspace when it changes (a tunnel).
	root string
}

type workspaceHub struct {
	hub     *Hub
	stop    context.CancelFunc
	created time.Time
}

// Serve wraps hub (the default workspace, built from o) with the other
// workspaces on disk next to it: the handler routes /w/<name> to them and
// serves /v1/server/workspaces. close stops them.
func Serve(ctx context.Context, o Options, hub *Hub) (handler http.Handler, close func(), err error) {
	ws := newWorkspaces(ctx, o, hub)
	if err := ws.load(); err != nil {
		ws.close()
		return nil, nil, err
	}
	return ws, ws.close, nil
}

func newWorkspaces(ctx context.Context, base Options, def *Hub) *workspaces {
	return &workspaces{ctx: ctx, base: base, dir: WorkspacesDir(base.DBPath), def: def, hubs: map[string]*workspaceHub{}, root: base.Dispatch.PublicURL}
}

// load opens every workspace found on disk.
func (ws *workspaces) load() error {
	entries, err := os.ReadDir(ws.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || !ValidWorkspaceName(e.Name()) {
			continue
		}
		if _, err := os.Stat(filepath.Join(ws.dir, e.Name(), "hub.db")); err != nil {
			continue
		}
		if _, err := ws.open(e.Name()); err != nil {
			return fmt.Errorf("workspace %s: %w", e.Name(), err)
		}
	}
	return nil
}

// open builds and starts the workspace in <dir>/<name> (created if new).
func (ws *workspaces) open(name string) (*workspaceHub, error) {
	dir := filepath.Join(ws.dir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	o := ws.base
	o.DBPath = filepath.Join(dir, "hub.db")
	o.FilesDir = filepath.Join(dir, "files")
	o.PathPrefix = "/w/" + name
	o.UpdateCheck = nil // one check for the process; health still reports the build
	if o.Log != nil {
		o.Log = o.Log.With("workspace", name)
	}
	ws.mu.RLock()
	root := ws.root
	ws.mu.RUnlock()
	o.Dispatch.PublicURL = ""
	if root != "" {
		o.Dispatch.PublicURL = strings.TrimRight(root, "/")
	}
	h, err := Build(o)
	if err != nil {
		return nil, err
	}
	if root != "" {
		// Build gave the dispatcher the root; links add the prefix.
		h.SetPublicURL(root)
	}
	if ws.base.UpdateCheck != nil {
		h.Server.SetVersion(ws.base.Version, ws.base.UpdateCheck.Latest)
	}
	ctx, stop := context.WithCancel(ws.ctx)
	go h.Dispatcher.Run(ctx)
	go h.Files.Run(ctx, time.Hour)
	go h.Traces.Run(ctx, time.Hour)
	info, _ := os.Stat(o.DBPath)
	w := &workspaceHub{hub: h, stop: stop, created: time.Now()}
	if info != nil {
		w.created = info.ModTime()
	}
	ws.mu.Lock()
	ws.hubs[name] = w
	ws.mu.Unlock()
	return w, nil
}

// setPublicURL applies the hub's public URL to every workspace.
func (ws *workspaces) setPublicURL(u string) {
	ws.mu.Lock()
	ws.root = u
	hubs := make([]*workspaceHub, 0, len(ws.hubs))
	for _, h := range ws.hubs {
		hubs = append(hubs, h)
	}
	ws.mu.Unlock()
	for _, h := range hubs {
		h.hub.SetPublicURL(u)
	}
}

func (ws *workspaces) setLAN(on bool) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	for _, h := range ws.hubs {
		h.hub.SetLAN(on)
	}
}

func (ws *workspaces) close() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	for name, h := range ws.hubs {
		h.stop()
		_ = h.hub.Close()
		delete(ws.hubs, name)
	}
}

// ServeHTTP sends /w/<name>/... to that workspace (without the prefix),
// /v1/server/workspaces to the server's own routes, and the rest to the
// default workspace.
func (ws *workspaces) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/v1/server/workspaces" || strings.HasPrefix(path, "/v1/server/workspaces/") {
		ws.serveAdmin(w, r)
		return
	}
	if !strings.HasPrefix(path, "/w/") {
		ws.def.Handler.ServeHTTP(w, r)
		return
	}
	rest := path[len("/w/"):]
	name, sub, _ := strings.Cut(rest, "/")
	ws.mu.RLock()
	h := ws.hubs[name]
	ws.mu.RUnlock()
	if h == nil {
		writeErr(w, http.StatusNotFound, "not_found", fmt.Sprintf("no workspace %q on this hub", name), "list them: myco workspace list")
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + sub
	if raw := r.URL.RawPath; raw != "" {
		if _, after, ok := strings.Cut(strings.TrimPrefix(raw, "/w/"), "/"); ok {
			r2.URL.RawPath = "/" + after
		} else {
			r2.URL.RawPath = "/"
		}
	}
	h.hub.Handler.ServeHTTP(w, r2)
}

// serveAdmin: GET and POST /v1/server/workspaces, DELETE .../{name}.
func (ws *workspaces) serveAdmin(w http.ResponseWriter, r *http.Request) {
	if !ws.def.Server.OwnsHub(r) {
		writeErr(w, http.StatusForbidden, "permission_denied", "only the owner of this hub's server can manage its workspaces",
			"use the admin token in hub.env (sudo myco workspace ...), or the app signed in as the owner")
		return
	}
	name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/v1/server/workspaces"), "/")
	switch {
	case name == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, api.WorkspaceList{Workspaces: ws.list(r)})
	case name == "" && r.Method == http.MethodPost:
		var in api.CreateWorkspaceRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_argument", "body must be JSON: {\"name\": \"...\"}", "")
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if !ValidWorkspaceName(in.Name) {
			writeErr(w, http.StatusBadRequest, "invalid_argument", fmt.Sprintf("invalid workspace name %q", in.Name),
				"use 1-40 lowercase letters, digits or '-', starting with a letter or digit (not \"default\")")
			return
		}
		ws.mu.RLock()
		_, exists := ws.hubs[in.Name]
		ws.mu.RUnlock()
		if exists {
			writeErr(w, http.StatusConflict, "conflict", fmt.Sprintf("workspace %q already exists", in.Name), "pick another name, or use it")
			return
		}
		if in.DryRun {
			writeJSON(w, http.StatusOK, ws.info(r, in.Name, time.Now()))
			return
		}
		h, err := ws.open(in.Name)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", "creating the workspace failed: "+err.Error(), "")
			return
		}
		if ws.base.Log != nil {
			ws.base.Log.Info("workspace created", "workspace", in.Name)
		}
		writeJSON(w, http.StatusCreated, ws.info(r, in.Name, h.created))
	case name != "" && r.Method == http.MethodDelete:
		ws.mu.Lock()
		h := ws.hubs[name]
		delete(ws.hubs, name)
		ws.mu.Unlock()
		if h == nil {
			writeErr(w, http.StatusNotFound, "not_found", fmt.Sprintf("no workspace %q on this hub", name), "list them: myco workspace list")
			return
		}
		h.stop()
		_ = h.hub.Close()
		// Kept aside, not deleted: removing a workspace is easy to regret.
		from := filepath.Join(ws.dir, name)
		to := filepath.Join(ws.dir, ".removed", name+"-"+time.Now().UTC().Format("20060102-150405"))
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err == nil {
			err = os.Rename(from, to)
			if err != nil && ws.base.Log != nil {
				ws.base.Log.Error("moving a removed workspace aside failed", "workspace", name, "err", err)
			}
		}
		if ws.base.Log != nil {
			ws.base.Log.Info("workspace removed", "workspace", name, "kept_in", to)
		}
		writeJSON(w, http.StatusOK, map[string]any{"removed": name, "kept_in": to})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "invalid_argument", "use GET or POST /v1/server/workspaces, or DELETE /v1/server/workspaces/{name}", "")
	}
}

func (ws *workspaces) list(r *http.Request) []api.Workspace {
	ws.mu.RLock()
	names := make([]string, 0, len(ws.hubs))
	created := map[string]time.Time{}
	for n, h := range ws.hubs {
		names = append(names, n)
		created[n] = h.created
	}
	ws.mu.RUnlock()
	sort.Strings(names)
	out := make([]api.Workspace, 0, len(names))
	for _, n := range names {
		out = append(out, ws.info(r, n, created[n]))
	}
	return out
}

func (ws *workspaces) info(r *http.Request, name string, created time.Time) api.Workspace {
	return api.Workspace{Name: name, URL: ws.def.Server.RootURL(r) + "/w/" + name, CreatedAt: created.UTC()}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg, suggestion string) {
	writeJSON(w, status, api.Error{Code: code, Message: msg, Suggestion: suggestion, Retryable: false})
}
