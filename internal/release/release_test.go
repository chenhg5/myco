package release

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/myco-ai/myco/internal/osutil"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

// mirrorServer serves one release from one download base. delay slows every
// answer (to make its speed obvious); tamper serves a binary that does not
// match its SHA256SUMS.
func mirrorServer(t *testing.T, delay time.Duration, tamper bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	body := []byte("#!/bin/sh\necho new\n")
	served := body
	if tamper {
		served = []byte("#!/bin/sh\necho tampered\n")
	}
	sum := sha256.Sum256(body)
	asset := AssetName("linux", "amd64")
	var assets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		switch r.URL.Path {
		case "/v0.17.0/SHA256SUMS":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), asset)
		case "/v0.17.0/" + asset:
			assets.Add(1)
			w.Write(served)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &assets
}

func TestDownloadURLs(t *testing.T) {
	get := func(k string) string {
		if k == EnvDownload {
			return "http://x/dl/"
		}
		return ""
	}
	if got := DownloadURLs(get); len(got) != 1 || got[0] != "http://x/dl" {
		t.Fatalf("override: %q", got)
	}
	got := DownloadURLs(func(string) string { return "" })
	if len(got) != 2 || got[0] != DefaultDownload || got[1] != DefaultMirror {
		t.Fatalf("defaults: %q", got)
	}
}

// The faster base answers the probe first and is used; the other is not
// asked for the binary at all, whatever the order of the list.
func TestFetchFromUsesTheFasterBase(t *testing.T) {
	slow, slowAssets := mirrorServer(t, 250*time.Millisecond, false)
	fast, fastAssets := mirrorServer(t, 0, false)
	got, err := FetchFrom(context.Background(), fast.Client(), []string{slow.URL, fast.URL}, "v0.17.0", "linux", "amd64")
	if err != nil || string(got) != "#!/bin/sh\necho new\n" {
		t.Fatalf("FetchFrom: %q %v", got, err)
	}
	if fastAssets.Load() != 1 || slowAssets.Load() != 0 {
		t.Fatalf("binaries served: fast %d, slow %d", fastAssets.Load(), slowAssets.Load())
	}
}

// A base that serves a bad copy is skipped and the other one used.
func TestFetchFromFallsBackToTheOtherBase(t *testing.T) {
	bad, badAssets := mirrorServer(t, 0, true)
	good, goodAssets := mirrorServer(t, 200*time.Millisecond, false)
	got, err := FetchFrom(context.Background(), good.Client(), []string{bad.URL, good.URL}, "v0.17.0", "linux", "amd64")
	if err != nil || string(got) != "#!/bin/sh\necho new\n" {
		t.Fatalf("FetchFrom: %q %v", got, err)
	}
	if badAssets.Load() != 1 || goodAssets.Load() != 1 {
		t.Fatalf("binaries served: bad %d, good %d", badAssets.Load(), goodAssets.Load())
	}
	if _, err := FetchAssetFrom(context.Background(), good.Client(), nil, "v0.17.0", "x", 1); err == nil {
		t.Fatal("no download source must fail")
	}
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
	if osutil.UnixPermissions && os.Geteuid() != 0 { // root can write anywhere; Windows has no dir mode bits
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

// GitHub's release downloads answer 504 now and then: get retries 5xx
// (with backoff) but not 4xx.
func TestGetRetriesTransientErrors(t *testing.T) {
	defer func(r int, b time.Duration) { getRetries, getBackoff = r, b }(getRetries, getBackoff)
	getRetries, getBackoff = 3, time.Millisecond
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch {
		case r.URL.Path == "/missing":
			http.NotFound(w, r)
		case calls <= 2:
			w.WriteHeader(http.StatusGatewayTimeout)
		default:
			w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()
	b, err := get(context.Background(), srv.Client(), srv.URL+"/x", 10, "")
	if err != nil || string(b) != "ok" || calls != 3 {
		t.Fatalf("after two 504s: %q %v (calls %d)", b, err, calls)
	}
	calls = 0
	if _, err := get(context.Background(), srv.Client(), srv.URL+"/missing", 10, ""); err == nil || calls != 1 {
		t.Fatalf("a 404 must not be retried: %v (calls %d)", err, calls)
	}
	calls = -100 // always 504
	_, err = get(context.Background(), srv.Client(), srv.URL+"/x", 10, "")
	if err == nil || !strings.Contains(err.Error(), "after 4 attempts") {
		t.Fatalf("giving up: %v", err)
	}
}

// A release download link that keeps answering 504 falls back to the
// release API's asset link (sent with Accept: application/octet-stream).
func TestFetchFallsBackToReleaseAPI(t *testing.T) {
	defer func(r int, b time.Duration, a, f string) { getRetries, getBackoff, tagAPI, fallbackBase = r, b, a, f }(getRetries, getBackoff, tagAPI, fallbackBase)
	getRetries, getBackoff = 1, time.Millisecond
	bin := []byte("new myco")
	sum := sha256.Sum256(bin)
	asset := AssetName("linux", "amd64")
	sums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/download/"):
			w.WriteHeader(http.StatusGatewayTimeout)
		case r.URL.Path == "/tags/v9.0.0":
			fmt.Fprintf(w, `{"assets":[{"name":"SHA256SUMS","url":"%[1]s/assets/1"},{"name":%[2]q,"url":"%[1]s/assets/2"}]}`, srv.URL, asset)
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			if r.Header.Get("Accept") != "application/octet-stream" {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}
			if r.URL.Path == "/assets/1" {
				io.WriteString(w, sums)
			} else {
				w.Write(bin)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	tagAPI, fallbackBase = srv.URL+"/tags", srv.URL+"/download"
	got, err := Fetch(context.Background(), srv.Client(), srv.URL+"/download", "v9.0.0", "linux", "amd64")
	if err != nil || string(got) != string(bin) {
		t.Fatalf("Fetch = %q, %v", got, err)
	}
	// A mirror (not the GitHub base) gets no fallback.
	if _, err := Fetch(context.Background(), srv.Client(), srv.URL+"/download/", "v9.0.0", "linux", "amd64"); err == nil {
		t.Fatal("a mirror should not fall back to the GitHub API")
	}
}

// When GitHub's API refuses or cannot be reached, the Gitee mirror knows the
// same releases; a caller's own API URL keeps to its own sources.
func TestLatestFallsBackToTheGiteeMirror(t *testing.T) {
	defer func(m string) { mirrorAPI = m }(mirrorAPI)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v0.23.0","name":"v0.23.0"}`)
	}))
	defer srv.Close()
	mirrorAPI = srv.URL
	ctx := context.Background()
	tag, err := latestFromMirror(ctx, srv.Client(), DefaultAPI)
	if err != nil || tag != "v0.23.0" {
		t.Fatalf("mirror: %q %v", tag, err)
	}
	if _, err := latestFromMirror(ctx, srv.Client(), "http://example.test/api/latest"); err == nil {
		t.Fatal("a custom API must not ask the mirror")
	}
}

// GitHub allows 60 unauthenticated API requests an hour per address: when
// the API refuses, the release page's redirect gives the latest tag.
func TestLatestFallsBackToTheReleasePage(t *testing.T) {
	var pageHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/myco-ai/myco/releases/latest", "/elsewhere":
			w.WriteHeader(http.StatusForbidden)
		case "/myco-ai/myco/releases/latest":
			pageHits++
			http.Redirect(w, r, "/myco-ai/myco/releases/tag/v0.20.7", http.StatusFound)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	tag, err := Latest(context.Background(), srv.Client(), srv.URL+"/repos/myco-ai/myco/releases/latest")
	if err != nil || tag != "v0.20.7" || pageHits != 1 {
		t.Fatalf("Latest = %q, %v (page asked %d times)", tag, err, pageHits)
	}
	// Not the API's latest-release shape: no page to fall back to, the API's error stands.
	if _, err := Latest(context.Background(), srv.Client(), srv.URL+"/elsewhere"); err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("other URL: %v", err)
	}
}

// A download cut off midway resumes with a range request instead of
// starting over; a server that ignores the range sends it all again.
func TestGetResumesCutOffDownloads(t *testing.T) {
	defer func(r int, b time.Duration) { getRetries, getBackoff = r, b }(getRetries, getBackoff)
	getRetries, getBackoff = 2, time.Millisecond
	body := bytes.Repeat([]byte("0123456789"), 5000) // 50 KB
	for _, honourRange := range []bool{true, false} {
		var ranges []string
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			ranges = append(ranges, r.Header.Get("Range"))
			from := 0
			if rg := r.Header.Get("Range"); rg != "" && honourRange {
				fmt.Sscanf(rg, "bytes=%d-", &from)
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(body)-1, len(body)))
				w.Header().Set("Content-Length", strconv.Itoa(len(body)-from))
				w.WriteHeader(http.StatusPartialContent)
			} else {
				w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			}
			part := body[from:]
			if calls <= 3 { // the first three connections break after 10 KB
				w.Write(part[:10000])
				w.(http.Flusher).Flush()
				hj, _ := w.(http.Hijacker)
				c, _, _ := hj.Hijack()
				c.Close()
				return
			}
			w.Write(part)
		}))
		b, err := get(context.Background(), srv.Client(), srv.URL+"/x", 1<<20, "")
		srv.Close()
		if err != nil || !bytes.Equal(b, body) {
			t.Fatalf("honourRange=%v: %d bytes, %v (ranges %q)", honourRange, len(b), err, ranges)
		}
		if honourRange && (len(ranges) != 4 || ranges[0] != "" || ranges[1] != "bytes=10000-" || ranges[3] != "bytes=30000-") {
			t.Fatalf("ranges asked: %q", ranges)
		}
	}
}

// A connection that stops sending is given up on and resumed.
func TestGetResumesStalledDownloads(t *testing.T) {
	defer func(r int, b, s time.Duration) { getRetries, getBackoff, stallTimeout = r, b, s }(getRetries, getBackoff, stallTimeout)
	getRetries, getBackoff, stallTimeout = 2, time.Millisecond, 200*time.Millisecond
	body := bytes.Repeat([]byte("x"), 30000)
	calls := 0
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Write(body[:1000])
			w.(http.Flusher).Flush()
			select { // hangs, as a stuck connection does
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		var from int
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &from)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(body)-1, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(body[from:])
	}))
	defer srv.Close()
	defer close(release)
	start := time.Now()
	b, err := get(context.Background(), srv.Client(), srv.URL+"/x", 1<<20, "")
	if err != nil || !bytes.Equal(b, body) || calls != 2 {
		t.Fatalf("%d bytes, %v, calls %d", len(b), err, calls)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %s", d)
	}
}

func TestRangeStart(t *testing.T) {
	for in, want := range map[string]int64{"bytes 100-199/200": 100, "bytes 0-9/*": 0} {
		if n, ok := rangeStart(in); !ok || n != want {
			t.Fatalf("%q: %d %v", in, n, ok)
		}
	}
	for _, bad := range []string{"", "bytes */200", "items 1-2/3"} {
		if _, ok := rangeStart(bad); ok {
			t.Fatalf("%q accepted", bad)
		}
	}
}

// Apps live in their own repository; a failed browser download must query that
// release's API, rather than the main binary repository's release.
func TestAppsAssetFallbackKeepsRepository(t *testing.T) {
	body := []byte("verified app archive")
	digest := sha256.Sum256(body)
	sums := hex.EncodeToString(digest[:]) + "  apps.zip\n"
	var calls []string
	client := &http.Client{Transport: appAssetTransport(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.String())
		status, data := http.StatusOK, ""
		switch r.URL.Host + r.URL.Path {
		case "github.com/myco-ai/apps/releases/download/apps/apps.zip", "github.com/myco-ai/apps/releases/download/apps/SHA256SUMS":
			status = http.StatusForbidden
		case "api.github.com/repos/myco-ai/apps/releases/tags/apps":
			data = `{"assets":[{"name":"SHA256SUMS","url":"https://api.github.com/assets/sums"},{"name":"apps.zip","url":"https://api.github.com/assets/zip"}]}`
		case "api.github.com/assets/sums":
			data = sums
		case "api.github.com/assets/zip":
			if r.Header.Get("Accept") != "application/octet-stream" {
				t.Fatal("asset request lacks binary Accept")
			}
			data = string(body)
		default:
			t.Fatalf("unexpected repository or request: %s", r.URL)
		}
		return &http.Response{StatusCode: status, ContentLength: int64(len(data)), Body: io.NopCloser(strings.NewReader(data)), Header: make(http.Header), Request: r}, nil
	})}
	got, err := FetchAsset(context.Background(), client, "https://github.com/myco-ai/apps/releases/download", "apps", "apps.zip", 1024)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("FetchAsset: %q %v (calls %v)", got, err, calls)
	}
}

type appAssetTransport func(*http.Request) (*http.Response, error)

func (f appAssetTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
