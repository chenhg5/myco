package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

func (e *engine) deleteProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e.mu.Lock()
	p, ok := e.profiles[id]
	if !ok || !idPattern.MatchString(id) {
		e.mu.Unlock()
		fail(w, "not_found", 404)
		return
	}
	controlled := false
	for _, c := range e.controls {
		c.mu.Lock()
		live := !c.closed && c.browser != nil && c.browser.id == id
		c.mu.Unlock()
		controlled = controlled || live
	}
	if e.sessions[id] != nil || e.controlOpening[id] || e.profileDeleting[id] || controlled {
		e.mu.Unlock()
		fail(w, "busy", 409)
		return
	}
	if e.profileDeleting == nil {
		e.profileDeleting = map[string]bool{}
	}
	e.profileDeleting[id] = true
	e.mu.Unlock()
	defer func() { e.mu.Lock(); delete(e.profileDeleting, id); e.mu.Unlock() }()
	// Check the process lease as well as the in-memory controller reservation.
	// Windows requires closing this handle before removing its containing tree.
	dir := filepath.Join(e.dir, "profiles", id)
	paths := []string{dir, filepath.Join(e.dir, "downloads", id)}
	for _, path := range paths {
		if info, err := os.Lstat(path); err == nil {
			actual, e2 := filepath.EvalSymlinks(path)
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || e2 != nil || actual != path {
				fail(w, "storage_failed", 500)
				return
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			fail(w, "storage_failed", 500)
			return
		}
	}
	if _, err := os.Stat(dir); err == nil {
		lease, err := lockProfile(filepath.Join(dir, ".myco-lease"))
		if err != nil {
			fail(w, "busy", 409)
			return
		}
		lease.Close()
	}
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			fail(w, "storage_failed", 500)
			return
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	password := e.passwords[id]
	delete(e.profiles, id)
	delete(e.passwords, id)
	if err := e.saveProfilesLocked(); err != nil {
		e.profiles[id] = p
		e.passwords[id] = password
		fail(w, "storage_failed", 500)
		return
	}
	jsonReply(w, true)
}
