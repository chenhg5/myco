package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDedicatedProfileIsMachineWideAndDeletionRequiresIdle(t *testing.T) {
	e, err := newEngine(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	call := func(method, path string, in any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(in)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("X-Myco-Scope", "demo/profiles")
		e.handler().ServeHTTP(w, r)
		return w
	}
	var wg sync.WaitGroup
	ids := make(chan string, 24)
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := call("POST", "/profiles", map[string]string{"kind": "agent", "name": "Dedicated agent"})
			var p profile
			json.Unmarshal(w.Body.Bytes(), &p)
			if w.Code != 200 || p.ID == "" {
				t.Error("dedicated resolution failed", w.Code)
				return
			}
			ids <- p.ID
		}()
	}
	wg.Wait()
	close(ids)
	var id string
	for next := range ids {
		if id != "" && id != next {
			t.Fatal("created duplicate dedicated profiles")
		}
		id = next
	}
	if len(e.profiles) != 1 {
		t.Fatal("dedicated resolution was not idempotent")
	}
	for _, parent := range []string{"profiles", "downloads"} {
		path := filepath.Join(e.dir, parent, id)
		os.MkdirAll(path, 0700)
		os.WriteFile(filepath.Join(path, "fixture"), []byte("demo data"), 0600)
	}
	for _, kind := range []string{"window", "opening", "controller"} {
		switch kind {
		case "window":
			e.sessions[id] = &session{closeComplete: true}
		case "opening":
			e.controlOpening = map[string]bool{id: true}
		case "controller":
			e.controls = map[string]*controlSession{"control": {browser: &session{id: id}}}
		}
		w := call("DELETE", "/profiles/"+id, nil)
		delete(e.sessions, id)
		delete(e.controlOpening, id)
		delete(e.controls, "control")
		if w.Code != 409 {
			t.Fatal("deleted a busy profile", kind, w.Code, w.Body.String())
		}
	}
	lease, err := lockProfile(filepath.Join(e.dir, "profiles", id, ".myco-lease"))
	if err != nil {
		t.Fatal(err)
	}
	w := call("DELETE", "/profiles/"+id, nil)
	lease.Close()
	if w.Code != 409 {
		t.Fatal("deleted leased profile", w.Code)
	}
	e.passwords[id] = "fixture-private"
	if w := call("DELETE", "/profiles/"+id, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, ok := e.profiles[id]; ok {
		t.Fatal("profile retained")
	}
	if _, ok := e.passwords[id]; ok {
		t.Fatal("credential retained")
	}
	for _, parent := range []string{"profiles", "downloads"} {
		if _, err := os.Stat(filepath.Join(e.dir, parent, id)); !os.IsNotExist(err) {
			t.Fatal("profile files retained", parent, err)
		}
	}
	reloaded, err := newEngine(context.Background(), e.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.close()
	if len(reloaded.profiles) != 0 {
		t.Fatal("deleted profile returned after restart")
	}
	if w := call("DELETE", "/profiles/"+id, nil); w.Code != 404 {
		t.Fatal("missing profile deletion succeeded")
	}
}
