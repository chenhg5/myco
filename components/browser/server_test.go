package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalHTTPBoundary(t *testing.T) {
	secret := rand.Text() + rand.Text()
	h := secure(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, tc := range []struct {
		host, auth, origin string
		want               int
	}{{"127.0.0.1:1234", "Bearer " + secret, "", 204}, {"localhost:1234", "Bearer " + secret, "", 403}, {"evil.example:1234", "Bearer " + secret, "", 403}, {"127.0.0.1:1234", "Bearer " + secret, "null", 403}, {"127.0.0.1:1234", "Bearer wrong", "", 401}, {"127.0.0.1:1234", secret, "", 401}} {
		req := httptest.NewRequest("GET", "http://127.0.0.1:1234/status", nil)
		req.Host = tc.host
		req.Header.Set("Authorization", tc.auth)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("host/origin boundary: got %d want %d", w.Code, tc.want)
		}
	}
}
func TestPrivateHandshakeAndEOF(t *testing.T) {
	input, send := io.Pipe()
	output, read := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx, input, read) }()
	secret := rand.Text() + rand.Text()
	if err := json.NewEncoder(send).Encode(settings{Secret: secret, DataDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	var hello struct {
		Address  string `json:"address"`
		Protocol int    `json:"protocol"`
	}
	if err := json.NewDecoder(output).Decode(&hello); err != nil {
		t.Fatal(err)
	}
	if hello.Protocol != 1 || !strings.HasPrefix(hello.Address, "127.0.0.1:") {
		t.Fatalf("invalid hello protocol/address")
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+hello.Address+"/status", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("X-Myco-Scope", "fixture/page")
	hc := &http.Client{Transport: &http.Transport{Proxy: nil}}
	defer hc.CloseIdleConnections()
	res, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var status struct {
		Protocol int       `json:"protocol"`
		Profiles []profile `json:"profiles"`
		Actions  []string  `json:"agent_actions"`
	}
	if err = json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Protocol != 1 || status.Profiles == nil || len(status.Actions) != 0 {
		t.Fatal("status contract or unexpected M2 actions")
	}
	send.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("component did not exit on stdin EOF")
	}
}
func TestProfilesStayLocalAndScopeOwnsArtifacts(t *testing.T) {
	e, err := newEngine(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := e.handler()
	call := func(owner, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
		req.Header.Set("X-Myco-Scope", owner)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	w := call("page_a", "POST", "/profiles", `{"name":"Testing"}`)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var p profile
	json.Unmarshal(w.Body.Bytes(), &p)
	if p.ID == "" || p.Agents || p.Running {
		t.Fatal("profile defaults")
	}
	next, err := newEngine(context.Background(), e.dir)
	if err != nil || next.profiles[p.ID].Name != "Testing" {
		t.Fatal("profile persistence")
	}
	id, err := e.storeArtifact("page_a", "image/jpeg", []byte("fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if call("page_b", "GET", "/artifacts/"+id, "").Code != 404 {
		t.Fatal("another page read an artifact")
	}
	if call("page_a", "GET", "/artifacts/"+id, "").Code != 200 {
		t.Fatal("owner cannot read artifact")
	}
	if !e.idle(-time.Second) {
		t.Fatal("idle without a session")
	}
	if call("", "GET", "/status", "").Code != 400 {
		t.Fatal("missing scope accepted")
	}
	if call("page_a", "POST", "/profiles", `{"name":"A"}junk`).Code != 400 {
		t.Fatal("trailing malformed JSON accepted")
	}
}
func TestProfileOSLease(t *testing.T) {
	path := t.TempDir() + "/lease"
	a, err := lockProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := lockProfile(path); err == nil {
		b.Close()
		t.Fatal("second controller acquired profile")
	}
	a.Close()
	b, err := lockProfile(path)
	if err != nil {
		t.Fatal("lease not released", err)
	}
	b.Close()
}

func TestDownloadDirectoryExistsAndUsesNativePrivatePath(t *testing.T) {
	e, err := newEngine(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir, err := e.downloadDir("p_native")
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(e.dir, "downloads", "p_native") {
		t.Fatal("non-native download path", dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatal("Chrome was handed a missing download directory", err)
	}
	if _, err := e.downloadDir("../outside"); err == nil {
		t.Fatal("download path escaped the private store")
	}
}

func TestDownloadDirectoryRejectsSymlinkBeforeCreatingFiles(t *testing.T) {
	e, err := newEngine(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(e.dir, "downloads")); err != nil {
		t.Skip("symlinks unavailable", err)
	}
	if _, err := e.downloadDir("p_native"); err == nil {
		t.Fatal("accepted symlinked download store")
	}
	if _, err := os.Stat(filepath.Join(outside, "p_native")); !os.IsNotExist(err) {
		t.Fatal("wrote outside the private store")
	}
}
