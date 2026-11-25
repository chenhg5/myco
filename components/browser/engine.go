package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type engine struct {
	trace      func(string, string, bool) // Optional test diagnostics; never exposed over HTTP.
	ctx        context.Context
	dir        string
	mu         sync.Mutex
	last       time.Time
	profiles   map[string]profile
	passwords  map[string]string
	sessions   map[string]*session
	recordings map[string]*recording
	runs       map[string]*playRun
	artifacts  map[string]artifact
}
type artifact struct {
	Thumbnail string `json:"thumbnail,omitempty"`
	DOM       string `json:"dom,omitempty"`
	Scope     string `json:"scope"`
	Kind      string `json:"kind"`
	Size      int64  `json:"size"`
}

func newEngine(ctx context.Context, dir string) (*engine, error) {
	e := &engine{ctx: ctx, dir: dir, last: time.Now(), profiles: map[string]profile{}, passwords: map[string]string{}, sessions: map[string]*session{}, recordings: map[string]*recording{}, runs: map[string]*playRun{}, artifacts: map[string]artifact{}}
	raw, err := os.ReadFile(filepath.Join(dir, "profiles.json"))
	if err == nil {
		var stored map[string]storedProfile
		if err = json.Unmarshal(raw, &stored); err != nil {
			return nil, errors.New("invalid profile index")
		}
		for id, p := range stored {
			e.profiles[id] = p.profile
			e.passwords[id] = p.ProxyPassword
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for id, p := range e.profiles {
		if id != p.ID || !idPattern.MatchString(id) {
			return nil, errors.New("invalid profile index")
		}
		p.Running = false
		input := profileSettingsInput{UserAgent: p.Settings.UserAgent, Lang: p.Settings.Lang, Args: p.Settings.Args}
		if x := p.Settings.Proxy; x != nil {
			input.Proxy = &proxySettingsInput{Server: x.Server, Bypass: x.Bypass, Username: x.Username}
		}
		settings, password, e2 := validatedSettings(input, p.Settings.Proxy, e.passwords[id])
		if e2 != nil {
			return nil, errors.New("invalid profile index")
		}
		p.Settings = settings
		e.passwords[id] = password
		e.profiles[id] = p
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	e.dir = canonical
	if err = e.loadArtifacts(); err != nil {
		return nil, err
	}
	return e, nil
}
func (e *engine) idle(d time.Duration) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.sessions) == 0 && time.Since(e.last) > d
}
func (e *engine) touch() { e.mu.Lock(); e.last = time.Now(); e.mu.Unlock() }
func (e *engine) close() {
	e.mu.Lock()
	all := []*session{}
	for _, s := range e.sessions {
		all = append(all, s)
	}
	e.mu.Unlock()
	for _, s := range all {
		s.close()
	}
}
func (e *engine) saveProfilesLocked() error {
	stored := map[string]storedProfile{}
	for id, p := range e.profiles {
		stored[id] = storedProfile{profile: p, ProxyPassword: e.passwords[id]}
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	name := filepath.Join(e.dir, "profiles.json.tmp-"+rand.Text())
	if err = os.WriteFile(name, data, 0600); err != nil {
		return err
	}
	defer os.Remove(name)
	return os.Rename(name, filepath.Join(e.dir, "profiles.json"))
}
func jsonReply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "code": code})
}
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(r.Body)
	if d.Decode(v) != nil {
		fail(w, "invalid_argument", 400)
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		fail(w, "invalid_argument", 400)
		return false
	}
	return true
}
func scope(r *http.Request) string { return r.Header.Get("X-Myco-Scope") }
func (e *engine) handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		profiles := []profile{}
		for _, p := range e.profiles {
			p.Running = e.sessions[p.ID] != nil
			profiles = append(profiles, p)
		}
		e.mu.Unlock()
		sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
		browsers := []string{}
		for name := range installedBrowsers() {
			browsers = append(browsers, name)
		}
		sort.Strings(browsers)
		machine, _ := os.Hostname()
		jsonReply(w, map[string]any{"protocol": protocol, "version": version, "browsers": browsers, "machine": machine, "profiles": profiles, "recordings": e.activeRecordings(scope(r)), "agent_actions": []string{}})
	})
	m.HandleFunc("POST /profiles", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || len(in.Name) > 160 || strings.ContainsAny(in.Name, "\r\n\x00") {
			fail(w, "invalid_argument", 400)
			return
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if len(e.profiles) >= 32 {
			fail(w, "profile_limit", 409)
			return
		}
		p := profile{ID: "p_" + rand.Text(), Name: in.Name, Settings: profileSettings{Args: []string{}}}
		e.profiles[p.ID] = p
		if err := e.saveProfilesLocked(); err != nil {
			delete(e.profiles, p.ID)
			fail(w, "storage_failed", 500)
			return
		}
		jsonReply(w, p)
	})
	m.HandleFunc("PATCH /profiles/{id}", e.configureProfile)
	m.HandleFunc("POST /profiles/{id}/agents", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Allowed bool `json:"allowed"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		// This endpoint is only called by the trusted App after local owner confirmation.
		// M0/M1 never register agent actions, irrespective of the saved preference.
		e.mu.Lock()
		defer e.mu.Unlock()
		p, ok := e.profiles[r.PathValue("id")]
		if !ok {
			fail(w, "not_found", 404)
			return
		}
		old := p
		p.Agents = in.Allowed
		e.profiles[p.ID] = p
		if e.saveProfilesLocked() != nil {
			e.profiles[p.ID] = old
			fail(w, "storage_failed", 500)
			return
		}
		p.Running = e.sessions[p.ID] != nil
		jsonReply(w, p)
	})
	m.HandleFunc("POST /sessions", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Profile  string `json:"profile"`
			Headless bool   `json:"headless"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		s, err := e.openSession(r.Context(), scope(r), in.Profile, in.Headless)
		if err != nil {
			fail(w, publicError(err), 409)
			return
		}
		jsonReply(w, map[string]string{"id": s.id})
	})
	m.HandleFunc("DELETE /sessions", func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		owned := []*session{}
		for _, s := range e.sessions {
			if s.scope == scope(r) || (strings.HasSuffix(scope(r), "/") && strings.HasPrefix(s.scope, scope(r))) {
				owned = append(owned, s)
			}
		}
		e.mu.Unlock()
		for _, s := range owned {
			s.close()
		}
		jsonReply(w, true)
	})
	m.HandleFunc("DELETE /sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		s, err := e.session(scope(r), r.PathValue("id"))
		if err != nil {
			fail(w, "not_found", 404)
			return
		}
		s.close()
		jsonReply(w, true)
	})
	e.recordRoutes(m)
	e.runRoutes(m)
	e.debugRoutes(m)
	m.HandleFunc("GET /artifacts/{id}", func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		a, ok := e.artifacts[r.PathValue("id")]
		e.mu.Unlock()
		if !ok || a.Scope != artifactScope(scope(r)) {
			fail(w, "not_found", 404)
			return
		}
		w.Header().Set("Content-Type", a.Kind)
		http.ServeFile(w, r, filepath.Join(e.dir, "artifacts", r.PathValue("id")))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(scope(r)) < 1 || len(scope(r)) > 256 {
			fail(w, "scope_required", 400)
			return
		}
		e.touch()
		m.ServeHTTP(w, r)
	})
}

// Neither errors from CDP (which may contain typed secrets) nor browser stderr
// are returned to a page or written to logs.
func publicError(err error) string {
	for _, s := range []string{"busy", "not_found", "browser_missing", "profile_locked", "invalid_argument", "unsupported_node", "invalid_graph", "storage_failed", "cancelled", "timeout", "target_not_found", "start_not_found", "sensitive_literal", "secret_required", "invalid_param", "unsupported_branch", "cycle", "many_starts"} {
		if strings.HasPrefix(err.Error(), s) {
			return s
		}
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "browser_failed"
}
