// Package release knows about published myco releases: comparing versions,
// asking GitHub for the latest one (in the background, never in a request
// path), and replacing the running binary with a verified download.
package release

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultAPI returns the latest release (GitHub REST API).
	DefaultAPI = "https://api.github.com/repos/chenhg5/myco/releases/latest"
	// DefaultDownload is where release assets live: <base>/<tag>/<asset>.
	DefaultDownload = "https://github.com/chenhg5/myco/releases/download"
	// EnvAPI and EnvDownload override them (mirrors, tests).
	EnvAPI      = "MYCO_RELEASE_API"
	EnvDownload = "MYCO_RELEASE_DOWNLOAD"
	// EnvNoCheck disables every automatic update check when set to 1/true.
	EnvNoCheck = "MYCO_NO_UPDATE_CHECK"
)

// Disabled reports whether automatic update checks are turned off.
func Disabled(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(EnvNoCheck))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// APIURL and DownloadURL honour the environment overrides.
func APIURL(getenv func(string) string) string {
	if u := strings.TrimSpace(getenv(EnvAPI)); u != "" {
		return u
	}
	return DefaultAPI
}

func DownloadURL(getenv func(string) string) string {
	if u := strings.TrimSpace(getenv(EnvDownload)); u != "" {
		return strings.TrimRight(u, "/")
	}
	return DefaultDownload
}

// Version is a parsed release version. Ahead marks a build made after the
// tag (git describe: v0.16.4-6-gabc), i.e. newer than v0.16.4 itself.
type Version struct {
	Major, Minor, Patch int
	Ahead               bool
}

// Parse reads "v1.2.3", "1.2.3" or "v1.2.3-6-gabc[-dirty]". ok is false for
// anything else (e.g. "dev"), which callers must treat as unknown.
func Parse(s string) (Version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	core, suffix, _ := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return Version{}, false
		}
		n[i] = v
	}
	return Version{n[0], n[1], n[2], suffix != ""}, true
}

func (v Version) cmp(o Version) int {
	for _, d := range [3]int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		if d != 0 {
			return d
		}
	}
	switch {
	case v.Ahead == o.Ahead:
		return 0
	case v.Ahead:
		return 1
	}
	return -1
}

// Newer reports whether a is a newer version than b. Unknown versions
// (dev builds) are never newer or older than anything.
func Newer(a, b string) bool {
	va, ok1 := Parse(a)
	vb, ok2 := Parse(b)
	return ok1 && ok2 && va.cmp(vb) > 0
}

// Latest asks the release API for the latest published tag.
func Latest(ctx context.Context, hc *http.Client, api string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "myco-update-check")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release API %s: HTTP %d", api, resp.StatusCode)
	}
	var out struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("release API: %w", err)
	}
	if _, ok := Parse(out.TagName); !ok {
		return "", fmt.Errorf("release API returned an unexpected tag %q", out.TagName)
	}
	return out.TagName, nil
}

// Checker keeps the latest release in memory, refreshed in the background.
// Readers never wait for the network.
type Checker struct {
	API      string
	Interval time.Duration // default 24h
	HTTP     *http.Client  // default: 15s timeout
	Log      *slog.Logger

	mu      sync.RWMutex
	latest  string
	checked time.Time
}

// Latest returns the last known latest release ("" until the first check).
func (c *Checker) Latest() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

// Check refreshes once; on failure the previous value is kept.
func (c *Checker) Check(ctx context.Context) error {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	tag, err := Latest(ctx, hc, c.API)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.latest, c.checked = tag, time.Now()
	c.mu.Unlock()
	return nil
}

// Run checks shortly after start and then every Interval until ctx ends.
func (c *Checker) Run(ctx context.Context) {
	interval := c.Interval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	wait := 5 * time.Second // let the hub finish starting first
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err := c.Check(ctx); err != nil && c.Log != nil {
			c.Log.Debug("update check failed (will retry)", "err", err)
		}
		wait = interval
	}
}

// AssetName is the release asset for an OS and architecture.
func AssetName(goos, goarch string) string { return "myco-" + goos + "-" + goarch }

// Fetch downloads tag's asset for goos/goarch and verifies it against the
// release's SHA256SUMS. It returns the verified bytes.
func Fetch(ctx context.Context, hc *http.Client, download, tag, goos, goarch string) ([]byte, error) {
	asset := AssetName(goos, goarch)
	sums, err := get(ctx, hc, download+"/"+tag+"/SHA256SUMS", 1<<20)
	if err != nil {
		return nil, fmt.Errorf("checksums for %s: %w", tag, err)
	}
	want := ""
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return nil, fmt.Errorf("release %s has no %s (unsupported platform?)", tag, asset)
	}
	bin, err := get(ctx, hc, download+"/"+tag+"/"+asset, 256<<20)
	if err != nil {
		return nil, fmt.Errorf("download %s %s: %w", tag, asset, err)
	}
	sum := sha256.Sum256(bin)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, fmt.Errorf("checksum mismatch for %s %s: got %s, want %s", tag, asset, got, want)
	}
	return bin, nil
}

func get(ctx context.Context, hc *http.Client, url string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "myco-upgrade")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("GET %s: response larger than %d bytes", url, max)
	}
	return b, nil
}

// ErrNotWritable means the binary's directory cannot be written by this user.
var ErrNotWritable = errors.New("cannot replace the myco binary here")

// Replace atomically swaps target for bin (same directory, then rename),
// keeping it executable. A failure leaves target untouched.
func Replace(target string, bin []byte) error {
	dir := filepath.Dir(target)
	f, err := os.CreateTemp(dir, ".myco-upgrade-*")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%w: %v", ErrNotWritable, err)
		}
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(bin); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%w: %v", ErrNotWritable, err)
		}
		return err
	}
	return nil
}
