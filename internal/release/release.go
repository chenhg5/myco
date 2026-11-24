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
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
	// DefaultTagAPI lists a release's assets (<base>/<tag>); used when a
	// DefaultDownload link fails: each asset's API link redirects straight
	// to the storage host, which works when github.com's links answer 504.
	DefaultTagAPI = "https://api.github.com/repos/chenhg5/myco/releases/tags"
	// DefaultMirror is the Gitee mirror of the same release assets. FetchFrom
	// times both sources and downloads from the faster one.
	DefaultMirror = "https://gitee.com/cg33/myco/releases/download"
	// DefaultMirrorAPI answers the latest-release question from the Gitee
	// mirror, for when GitHub's API refuses or cannot be reached.
	DefaultMirrorAPI = "https://gitee.com/api/v5/repos/cg33/myco/releases/latest"
	// EnvAPI and EnvDownload override them (mirrors, tests). Setting
	// EnvDownload also turns the Gitee mirror off.
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

// DownloadURLs are the places to download release assets from: the single
// override when EnvDownload is set, otherwise GitHub and its Gitee mirror.
// FetchFrom times them and uses the faster one, falling back to the other.
func DownloadURLs(getenv func(string) string) []string {
	if u := strings.TrimSpace(getenv(EnvDownload)); u != "" {
		return []string{strings.TrimRight(u, "/")}
	}
	return []string{DefaultDownload, DefaultMirror}
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

// Latest asks the release API for the latest published tag. When the API
// refuses (it allows 60 unauthenticated requests an hour per address, so a
// shared network soon gets 403) or cannot be reached, the Gitee mirror is
// asked, and then the release page's redirect:
// github.com/<owner>/<repo>/releases/latest answers with the tag's page in
// its Location, without that limit.
func Latest(ctx context.Context, hc *http.Client, api string) (string, error) {
	tag, err := latestFromAPI(ctx, hc, api)
	if err == nil {
		return tag, nil
	}
	if t, merr := latestFromMirror(ctx, hc, api); merr == nil {
		return t, nil
	}
	if page := latestPage(api); page != "" {
		if t, perr := latestFromPage(ctx, hc, page); perr == nil {
			return t, nil
		}
	}
	return "", err
}

// mirrorAPI is DefaultMirrorAPI; tests point it elsewhere.
var mirrorAPI = DefaultMirrorAPI

// latestFromMirror asks the Gitee mirror for the latest tag, only when api
// is GitHub's own (an EnvAPI override names its own sources).
func latestFromMirror(ctx context.Context, hc *http.Client, api string) (string, error) {
	if api != DefaultAPI {
		return "", fmt.Errorf("no mirror for release API %s", api)
	}
	return latestFromAPI(ctx, hc, mirrorAPI)
}

func latestFromAPI(ctx context.Context, hc *http.Client, api string) (string, error) {
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

// latestPage is the release page that redirects to the latest tag, for an
// API URL of the form <scheme>://<host>/repos/<owner>/<repo>/releases/latest
// (api.github.com becomes github.com); "" for anything else.
func latestPage(api string) string {
	u, err := url.Parse(api)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 5 || parts[0] != "repos" || parts[3] != "releases" || parts[4] != "latest" {
		return ""
	}
	host := u.Host
	if host == "api.github.com" {
		host = "github.com"
	}
	return u.Scheme + "://" + host + "/" + parts[1] + "/" + parts[2] + "/releases/latest"
}

func latestFromPage(ctx context.Context, hc *http.Client, page string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, page, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "myco-update-check")
	c := *hc
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if resp.StatusCode/100 != 3 || i < 0 {
		return "", fmt.Errorf("release page %s: HTTP %d", page, resp.StatusCode)
	}
	tag, _ := url.PathUnescape(loc[i+len("/tag/"):])
	if _, ok := Parse(tag); !ok {
		return "", fmt.Errorf("release page pointed at an unexpected tag %q", tag)
	}
	return tag, nil
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
func AssetName(goos, goarch string) string {
	name := "myco-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// OldSuffix marks the binary a Windows upgrade moved aside: a running .exe
// cannot be overwritten or deleted, only renamed. CleanupOld removes it
// once that process has exited.
const OldSuffix = ".old"

// CleanupOld removes target+OldSuffix if a previous upgrade left one.
func CleanupOld(target string) { _ = os.Remove(target + OldSuffix) }

// Fetch downloads tag's asset for goos/goarch and verifies it against the
// release's SHA256SUMS. It returns the verified bytes.
func Fetch(ctx context.Context, hc *http.Client, download, tag, goos, goarch string) ([]byte, error) {
	return FetchAsset(ctx, hc, download, tag, AssetName(goos, goarch), 256<<20)
}

// FetchFrom is Fetch over several download bases (DownloadURLs): the fastest
// ones by a small probe first, the others as a fallback. An asset that one
// base serves badly or not at all comes from the next; each copy is still
// verified against that base's SHA256SUMS.
func FetchFrom(ctx context.Context, hc *http.Client, bases []string, tag, goos, goarch string) ([]byte, error) {
	return FetchAssetFrom(ctx, hc, bases, tag, AssetName(goos, goarch), 256<<20)
}

// FetchAssetFrom downloads one named asset of release tag from the first
// download base that serves it (see FetchFrom).
func FetchAssetFrom(ctx context.Context, hc *http.Client, bases []string, tag, asset string, max int64) ([]byte, error) {
	if len(bases) == 0 {
		return nil, errors.New("no release download source is configured")
	}
	if len(bases) == 1 {
		return FetchAsset(ctx, hc, bases[0], tag, asset, max)
	}
	var errs []error
	for _, base := range orderBySpeed(ctx, hc, bases, tag) {
		b, err := FetchAsset(ctx, hc, base, tag, asset, max)
		if err == nil {
			return b, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", hostOf(base), err))
		if ctx.Err() != nil || strings.Contains(err.Error(), " has no ") {
			break // cancelled, or the release itself lacks the asset
		}
	}
	return nil, errors.Join(errs...)
}

// probeTimeout bounds the speed test of one download base. A base that does
// not serve the probe in time goes last (and is still tried as a fallback).
var probeTimeout = 3 * time.Second

// orderBySpeed fetches the release's SHA256SUMS from each base in parallel
// and returns the bases fastest first. A base that fails the probe keeps its
// place among the failures, after the working ones.
func orderBySpeed(ctx context.Context, hc *http.Client, bases []string, tag string) []string {
	if len(bases) < 2 {
		return bases
	}
	type probe struct {
		i  int
		ok bool
		d  time.Duration
	}
	probes := make([]probe, len(bases))
	ch := make(chan probe, len(bases))
	for i, base := range bases {
		go func(i int, base string) {
			pctx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			req, err := http.NewRequestWithContext(pctx, http.MethodGet, base+"/"+tag+"/SHA256SUMS", nil)
			if err != nil {
				ch <- probe{i: i}
				return
			}
			req.Header.Set("User-Agent", "myco-upgrade")
			start := time.Now()
			resp, err := hc.Do(req)
			if err != nil {
				ch <- probe{i: i}
				return
			}
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
			resp.Body.Close()
			ch <- probe{i: i, ok: resp.StatusCode == http.StatusOK, d: time.Since(start)}
		}(i, base)
	}
	for range bases {
		p := <-ch
		probes[p.i] = p
	}
	sort.SliceStable(probes, func(a, b int) bool {
		if probes[a].ok != probes[b].ok {
			return probes[a].ok
		}
		return probes[a].ok && probes[a].d < probes[b].d
	})
	ordered := make([]string, len(bases))
	for i, p := range probes {
		ordered[i] = bases[p.i]
	}
	return ordered
}

// hostOf names a download base in an error message.
func hostOf(base string) string {
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Host
	}
	return base
}

// FetchAsset downloads one named asset of release tag and verifies it
// against the release's SHA256SUMS.
func FetchAsset(ctx context.Context, hc *http.Client, download, tag, asset string, max int64) ([]byte, error) {
	sums, err := fetchAsset(ctx, hc, download, tag, "SHA256SUMS", 1<<20)
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
	bin, err := fetchAsset(ctx, hc, download, tag, asset, max)
	if err != nil {
		return nil, fmt.Errorf("download %s %s: %w", tag, asset, err)
	}
	sum := sha256.Sum256(bin)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, fmt.Errorf("checksum mismatch for %s %s: got %s, want %s", tag, asset, got, want)
	}
	return bin, nil
}

// tagAPI and fallbackBase are DefaultTagAPI and DefaultDownload; tests
// point them elsewhere.
var (
	tagAPI       = DefaultTagAPI
	fallbackBase = DefaultDownload
)

// fetchAsset downloads <download>/<tag>/<name>. From GitHub (the default
// download base) it falls back to the release API's link for the asset.
func fetchAsset(ctx context.Context, hc *http.Client, download, tag, name string, max int64) ([]byte, error) {
	b, err := get(ctx, hc, download+"/"+tag+"/"+name, max, "")
	if err == nil || download != fallbackBase || ctx.Err() != nil {
		return b, err
	}
	u, aerr := assetAPIURL(ctx, hc, tag, name)
	if aerr != nil {
		return nil, fmt.Errorf("%w; release API fallback: %v", err, aerr)
	}
	b, aerr = get(ctx, hc, u, max, "application/octet-stream")
	if aerr != nil {
		return nil, fmt.Errorf("%w; release API fallback: %v", err, aerr)
	}
	return b, nil
}

// assetAPIURL finds the API link of a release's asset.
func assetAPIURL(ctx context.Context, hc *http.Client, tag, name string) (string, error) {
	raw, err := get(ctx, hc, tagAPI+"/"+tag, 4<<20, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var rel struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &rel); err != nil {
		return "", fmt.Errorf("release %s: %w", tag, err)
	}
	for _, a := range rel.Assets {
		if a.Name == name && a.URL != "" {
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("release %s has no asset %s", tag, name)
}

// getRetries and getBackoff bound retries of transient download failures:
// GitHub's release downloads answer 502/503/504 now and then. An attempt
// that got further than the one before does not count: on a slow, flaky
// connection a large download goes on as long as it keeps moving (up to
// getMaxAttempts in all). stallTimeout ends an attempt whose connection
// stopped sending; the next one resumes where it stopped.
var (
	getRetries     = 5
	getBackoff     = 3 * time.Second
	getMaxAttempts = 40
	stallTimeout   = 60 * time.Second
)

// get downloads url, retrying network errors, stalls and 5xx answers, and
// resuming a download cut off midway (an HTTP range request) instead of
// starting over.
func get(ctx context.Context, hc *http.Client, url string, max int64, accept string) ([]byte, error) {
	var have []byte
	failed := 0
	for attempt := 1; ; attempt++ {
		before := len(have)
		b, retry, err := getOnce(ctx, hc, url, max, accept, have)
		if err == nil {
			return b, nil
		}
		have = b
		if len(have) <= before {
			failed++
		}
		if !retry || failed > getRetries || attempt >= getMaxAttempts || ctx.Err() != nil {
			if attempt > 1 {
				err = fmt.Errorf("%w (after %d attempts)", err, attempt)
			}
			return nil, err
		}
		wait := getBackoff * time.Duration(failed)
		if len(have) > before {
			wait = getBackoff / 3 // it was moving: carry on soon
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(wait):
		}
	}
}

// getOnce makes one attempt. With have (what earlier attempts got), it asks
// for the rest; a server that sends the whole file again starts it over.
// On a failure that is worth retrying it returns what it has so far.
func getOnce(ctx context.Context, hc *http.Client, url string, max int64, accept string, have []byte) (b []byte, retry bool, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "myco-upgrade")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if len(have) > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", len(have)))
	}
	// A connection that stops sending (headers or body) is given up on.
	stall := time.AfterFunc(stallTimeout, cancel)
	defer stall.Stop()
	stalled := func(err error) error {
		if ctx.Err() != nil && !stall.Stop() {
			return fmt.Errorf("GET %s: no data for %s", url, stallTimeout)
		}
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return have, true, stalled(err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusPartialContent && len(have) > 0:
		if start, ok := rangeStart(resp.Header.Get("Content-Range")); !ok || start != int64(len(have)) {
			// Not the part asked for: start over.
			return nil, true, fmt.Errorf("GET %s: unexpected range %q", url, resp.Header.Get("Content-Range"))
		}
	case resp.StatusCode == http.StatusOK:
		have = nil // the whole file again
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && len(have) > 0:
		return nil, true, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	default:
		return have, resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests,
			fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	buf := bytes.NewBuffer(have)
	chunk := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(chunk)
		if n > 0 {
			stall.Reset(stallTimeout)
			buf.Write(chunk[:n])
			if int64(buf.Len()) > max {
				return nil, false, fmt.Errorf("GET %s: response larger than %d bytes", url, max)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return buf.Bytes(), true, stalled(rerr) // cut off mid-download
		}
	}
	if resp.ContentLength >= 0 && resp.StatusCode == http.StatusOK && int64(buf.Len()) != resp.ContentLength {
		return buf.Bytes(), true, fmt.Errorf("GET %s: got %d of %d bytes", url, buf.Len(), resp.ContentLength)
	}
	return buf.Bytes(), false, nil
}

// rangeStart reads the first byte of "bytes 100-199/200".
func rangeStart(cr string) (int64, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(cr), "bytes ")
	if !ok {
		return 0, false
	}
	start, _, ok := strings.Cut(rest, "-")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(start), 10, 64)
	return n, err == nil && n >= 0
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
	if runtime.GOOS == "windows" {
		return swapWindows(tmp, target)
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

// swapWindows replaces target with tmp on Windows, where the running binary
// can be renamed but not overwritten: target moves to target.old (removed by
// the next run), tmp takes its place, and a failure moves target back.
func swapWindows(tmp, target string) error {
	old := target + OldSuffix
	_ = os.Remove(old)
	moved := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, old); err != nil {
			os.Remove(tmp)
			if errors.Is(err, os.ErrPermission) {
				return fmt.Errorf("%w: %v", ErrNotWritable, err)
			}
			return err
		}
		moved = true
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		if moved {
			_ = os.Rename(old, target)
		}
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%w: %v", ErrNotWritable, err)
		}
		return err
	}
	return nil
}
