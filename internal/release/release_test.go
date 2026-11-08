package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/chenhg5/myco/internal/osutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseAndNewer(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		newer bool
	}{
		{"v0.16.6", "v0.16.5", true},
		{"v0.17.0", "v0.16.9", true},
		{"v1.0.0", "v0.99.99", true},
		{"0.16.6", "v0.16.5", true}, // with or without v
		{"v0.16.5", "v0.16.5", false},
		{"v0.16.5", "v0.16.6", false},
		{"v0.16.5", "v0.16.4-6-gd202d2a", true}, // a build after v0.16.4
		{"v0.16.4", "v0.16.4-6-gd202d2a", false},
		{"v0.16.4-6-gd202d2a", "v0.16.4", true},
		{"v0.16.6", "dev", false}, // unknown versions are never compared
		{"dev", "v0.16.6", false},
		{"", "v0.16.6", false},
		{"v0.16", "v0.15.0", false},
	} {
		if got := Newer(tc.a, tc.b); got != tc.newer {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.newer)
		}
	}
	if v, ok := Parse("v0.16.4-6-gd202d2a-dirty"); !ok || v.Minor != 16 || !v.Ahead {
		t.Errorf("Parse describe: %+v %v", v, ok)
	}
}

func TestDisabledAndOverrides(t *testing.T) {
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	if Disabled(get) || APIURL(get) != DefaultAPI || DownloadURL(get) != DefaultDownload {
		t.Fatal("defaults")
	}
	for _, v := range []string{"1", "true", "YES", "on"} {
		env[EnvNoCheck] = v
		if !Disabled(get) {
			t.Errorf("%s=%s should disable", EnvNoCheck, v)
		}
	}
	env[EnvNoCheck] = "0"
	env[EnvAPI], env[EnvDownload] = "http://x/api", "http://x/dl/"
	if Disabled(get) || APIURL(get) != "http://x/api" || DownloadURL(get) != "http://x/dl" {
		t.Fatal("overrides")
	}
}

func TestLatestAndChecker(t *testing.T) {
	var fail atomic.Bool
	tag := atomic.Value{}
	tag.Store("v0.17.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "rate limited", http.StatusForbidden)
			return
		}
		fmt.Fprintf(w, `{"tag_name":%q,"name":"x"}`, tag.Load())
	}))
	defer srv.Close()
	ctx := context.Background()
	if got, err := Latest(ctx, srv.Client(), srv.URL); err != nil || got != "v0.17.0" {
		t.Fatalf("Latest: %q %v", got, err)
	}
	tag.Store("nightly")
	if _, err := Latest(ctx, srv.Client(), srv.URL); err == nil {
		t.Fatal("a non-version tag must be rejected")
	}
	tag.Store("v0.17.0")

	var c *Checker
	if c.Latest() != "" {
		t.Fatal("nil checker")
	}
	c = &Checker{API: srv.URL, HTTP: srv.Client()}
	if c.Latest() != "" {
		t.Fatal("unknown before the first check")
	}
	if err := c.Check(ctx); err != nil || c.Latest() != "v0.17.0" {
		t.Fatalf("Check: %q %v", c.Latest(), err)
	}
	fail.Store(true)
	if err := c.Check(ctx); err == nil || c.Latest() != "v0.17.0" {
		t.Fatalf("a failed check must keep the last value: %q %v", c.Latest(), err)
	}
	// Run stops with its context.
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { (&Checker{API: srv.URL, Interval: time.Hour}).Run(rctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func releaseServer(t *testing.T, asset string, body []byte, sum string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0.17.0/SHA256SUMS":
			fmt.Fprintf(w, "%s  %s\n%s  myco-other-arch\n", sum, asset, strings.Repeat("0", 64))
		case "/v0.17.0/" + asset:
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchVerifiesChecksums(t *testing.T) {
	ctx := context.Background()
	body := []byte("#!/bin/sh\necho new\n")
	h := sha256.Sum256(body)
	good := hex.EncodeToString(h[:])
	srv := releaseServer(t, "myco-linux-amd64", body, good)
	got, err := Fetch(ctx, srv.Client(), srv.URL, "v0.17.0", "linux", "amd64")
	if err != nil || string(got) != string(body) {
		t.Fatalf("Fetch: %v", err)
	}
	if _, err := Fetch(ctx, srv.Client(), srv.URL, "v0.17.0", "windows", "amd64"); err == nil || !strings.Contains(err.Error(), "no myco-windows-amd64") {
		t.Fatalf("unsupported platform: %v", err)
	}
	if _, err := Fetch(ctx, srv.Client(), srv.URL, "v9.9.9", "linux", "amd64"); err == nil {
		t.Fatal("missing release must fail")
	}
	bad := releaseServer(t, "myco-linux-amd64", body, strings.Repeat("a", 64))
	if _, err := Fetch(ctx, bad.Client(), bad.URL, "v0.17.0", "linux", "amd64"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered binary: %v", err)
	}
}

func TestReplaceIsAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "myco")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(target, []byte("new")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(target)
	st, _ := os.Stat(target)
	if string(b) != "new" || (osutil.UnixPermissions && st.Mode().Perm() != 0o755) {
		t.Fatalf("after Replace: %q %v", b, st.Mode())
	}
	CleanupOld(target) // Windows keeps the running binary as myco.old
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left: %v", entries)
	}
	if os.Geteuid() != 0 { // root can write anywhere
		ro := t.TempDir()
		ot := filepath.Join(ro, "myco")
		os.WriteFile(ot, []byte("old"), 0o755)
		os.Chmod(ro, 0o555)
		defer os.Chmod(ro, 0o755)
		if err := Replace(ot, []byte("new")); !errors.Is(err, ErrNotWritable) {
			t.Fatalf("read-only dir: %v", err)
		}
		if b, _ := os.ReadFile(ot); string(b) != "old" {
			t.Fatal("failed replace must leave the binary untouched")
		}
	}
}

func TestAssetNameWindows(t *testing.T) {
	if got := AssetName("windows", "amd64"); got != "myco-windows-amd64.exe" {
		t.Errorf("AssetName(windows) = %q", got)
	}
	if got := AssetName("darwin", "arm64"); got != "myco-darwin-arm64" {
		t.Errorf("AssetName(darwin) = %q", got)
	}
}

// swapWindows only renames, so its logic can be checked on any OS.
func TestSwapWindows(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "myco.exe")
	tmp := filepath.Join(dir, ".myco-upgrade-1")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+OldSuffix, []byte("older"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := swapWindows(tmp, target); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Errorf("target = %q, want new", b)
	}
	if b, _ := os.ReadFile(target + OldSuffix); string(b) != "old" {
		t.Errorf("old = %q, want the previous binary", b)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temp file left behind")
	}
	CleanupOld(target)
	if _, err := os.Stat(target + OldSuffix); !os.IsNotExist(err) {
		t.Error("CleanupOld left the old binary")
	}

	// A failed swap puts the original back.
	if err := swapWindows(filepath.Join(dir, "missing"), target); err == nil {
		t.Fatal("want an error for a missing temp file")
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Errorf("after a failed swap target = %q, want it restored", b)
	}
}
