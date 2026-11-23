package templates

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/release"
)

// The official apps are also published on their own (templates.md), so a
// hub gets new and updated ones without a release of its own: the "apps"
// release on GitHub carries apps.zip (one folder an app, as in official/)
// and SHA256SUMS. A Catalog keeps the newest of what the binary carries
// and what it last fetched, gives it to every workspace's library, and
// fetches again once a day. The archive it fetched is kept beside the
// hub's database, so a restart does not wait for the network.
const (
	// AppsTag is the release that carries the official apps.
	AppsTag = "apps"
	// AppsAsset is the archive in it.
	AppsAsset = "apps.zip"
	maxApps   = 128 << 20
)

// Catalog is the official apps a hub offers.
type Catalog struct {
	// Download is where release assets live (release.DownloadURL).
	Download string
	// StateDir keeps the last archive fetched ("" keeps nothing).
	StateDir string
	// HubVersion is this hub's build: apps needing a newer one are left out.
	HubVersion string
	// Offline: never fetch (the binary's apps, and a kept archive).
	Offline  bool
	Interval time.Duration // default 24h
	// RetryWait: how long to wait before fetching again when the archive
	// and its checksum do not match (default 5s).
	RetryWait time.Duration
	HTTP      *http.Client
	Log       *slog.Logger

	mu      sync.Mutex
	current []app.OfficialTemplate
	subs    []func([]app.OfficialTemplate)
	fetched time.Time
	from    string
	// a Freshen's fetch: when the last began, and whether one is running
	tried    time.Time
	fetching bool
}

// SyncResult says what a sync found.
type SyncResult struct {
	Apps    int       `json:"apps"`
	Updated []string  `json:"updated,omitempty"`
	Source  string    `json:"source"`
	At      time.Time `json:"at,omitempty"`
	Error   string    `json:"error,omitempty"`
}

func (c *Catalog) keptPath() string {
	if c.StateDir == "" {
		return ""
	}
	return filepath.Join(c.StateDir, "official-apps.zip")
}

// Load starts from the binary's apps and the archive kept from before.
func (c *Catalog) Load() error {
	built, err := Official()
	if err != nil {
		return err
	}
	list, from := built, "built in"
	if p := c.keptPath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if got, err := c.read(b); err == nil {
				list, _ = merge(built, got)
				from = "kept"
			} else if c.Log != nil {
				c.Log.Warn("official apps kept from before are unreadable", "err", err)
			}
		}
	}
	c.mu.Lock()
	c.current, c.from = list, from
	c.mu.Unlock()
	return nil
}

// Attach gives a library the apps now and whenever they change.
func (c *Catalog) Attach(set func([]app.OfficialTemplate)) {
	c.mu.Lock()
	c.subs = append(c.subs, set)
	cur := c.current
	c.mu.Unlock()
	if cur == nil {
		if built, err := Official(); err == nil {
			cur = built
		}
	}
	set(cur)
}

// Sync fetches the published apps (at most once a minute) and offers the
// newer ones.
func (c *Catalog) Sync(ctx context.Context) SyncResult {
	c.mu.Lock()
	recent := time.Since(c.fetched) < time.Minute
	c.mu.Unlock()
	if c.Offline || recent {
		return c.result(nil, "")
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	// The release workflow uploads the archive, then its checksum: a fetch
	// between the two (or soon after, while GitHub still hands out the old
	// checksum) finds them unmatched. Try again a little later before
	// saying so.
	wait := c.RetryWait
	if wait <= 0 {
		wait = 5 * time.Second
	}
	var b []byte
	var err error
	for i := 0; ; i++ {
		b, err = release.FetchAsset(ctx, hc, c.Download, AppsTag, AppsAsset, maxApps)
		if err == nil || !strings.Contains(err.Error(), "checksum mismatch") || i >= 2 {
			break
		}
		select {
		case <-time.After(wait * time.Duration(i+1)):
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "checksum mismatch") {
			return c.result(nil, "the official apps are being published right now (their archive and checksum do not match yet): try again in a minute")
		}
		return c.result(nil, err.Error())
	}
	got, err := c.read(b)
	if err != nil {
		return c.result(nil, err.Error())
	}
	built, err := Official()
	if err != nil {
		return c.result(nil, err.Error())
	}
	list, _ := merge(built, got)
	c.mu.Lock()
	before := c.current
	c.current, c.fetched, c.from = list, time.Now(), "release "+AppsTag
	subs := append([]func([]app.OfficialTemplate){}, c.subs...)
	c.mu.Unlock()
	updated := changed(before, list)
	if len(updated) > 0 {
		for _, s := range subs {
			s(list)
		}
		if p := c.keptPath(); p != "" {
			if err := os.WriteFile(p+".tmp", b, 0o600); err == nil {
				_ = os.Rename(p+".tmp", p)
			}
		}
		if c.Log != nil {
			c.Log.Info("official apps updated", "apps", len(list), "updated", strings.Join(updated, ", "))
		}
	}
	return c.result(updated, "")
}

func (c *Catalog) result(updated []string, errText string) SyncResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	src := c.from
	if src == "" {
		src = "built in"
	}
	if c.Offline {
		src += " (fetching is off)"
	}
	return SyncResult{Apps: len(c.current), Updated: updated, Source: src, At: c.fetched, Error: errText}
}

// Freshen fetches the published apps when what the hub has is older than
// maxAge: someone opening a page made from an official app then sees an
// update the same hour it is published, not the next day. One fetch at a
// time; after one (failed or not) it waits maxAge/6 before trying again.
// It waits for the fetch as long as ctx allows.
func (c *Catalog) Freshen(ctx context.Context, maxAge time.Duration) {
	c.mu.Lock()
	stale := time.Since(c.fetched) > maxAge && time.Since(c.tried) > maxAge/6
	if c.Offline || !stale || c.fetching {
		c.mu.Unlock()
		return
	}
	c.fetching, c.tried = true, time.Now()
	c.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { c.mu.Lock(); c.fetching = false; c.mu.Unlock() }()
		// (on its own clock: a page answered early does not cut it short)
		fctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if r := c.Sync(fctx); r.Error != "" && c.Log != nil {
			c.Log.Debug("official apps: fetch on open failed", "err", r.Error)
		}
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// firstRetry and firstRetryMax pace fetching again after a failure while
// nothing was ever fetched or kept: the binary carries only the Builtin
// apps, so the rest should not wait a day for a network that was down at
// start.
// startWait is how long after start the first fetch waits.
var (
	startWait     = 20 * time.Second
	firstRetry    = time.Minute
	firstRetryMax = 30 * time.Minute
)

// Run fetches shortly after start and then every Interval (sooner, after
// a failure, until a first fetch succeeds).
func (c *Catalog) Run(ctx context.Context) {
	if c.Offline {
		return
	}
	interval := c.Interval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	wait := startWait
	retry := firstRetry
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		r := c.Sync(ctx)
		wait = interval
		if r.Error != "" {
			if c.Log != nil {
				c.Log.Debug("official apps: fetch failed (will retry)", "err", r.Error)
			}
			c.mu.Lock()
			never := c.fetched.IsZero() && c.from != "kept"
			c.mu.Unlock()
			if never {
				wait, retry = retry, min(retry*2, firstRetryMax)
			}
		}
	}
}

// read takes the apps out of an archive: one folder an app, each checked
// as a template is; those made for a newer hub are left out.
func (c *Catalog) read(b []byte) ([]app.OfficialTemplate, error) {
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("official apps: not a zip: %w", err)
	}
	dirs := map[string]map[string][]byte{}
	var total int64
	for _, f := range z.File {
		name := strings.TrimPrefix(path.Clean("/"+f.Name), "/")
		if f.FileInfo().IsDir() || name == "" || strings.HasPrefix(path.Base(name), ".") {
			continue
		}
		dir, rest, ok := strings.Cut(name, "/")
		if !ok || rest == "" {
			continue
		}
		total += int64(f.UncompressedSize64)
		if total > 4*maxApps {
			return nil, fmt.Errorf("official apps: the archive is too big")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, domain.MaxPageSize+1))
		rc.Close()
		if err != nil {
			return nil, err
		}
		if dirs[dir] == nil {
			dirs[dir] = map[string][]byte{}
		}
		dirs[dir][rest] = data
	}
	var out []app.OfficialTemplate
	for dir, files := range dirs {
		raw := files["myco.json"]
		if raw == nil {
			continue
		}
		if want := domain.RequiredHub(raw); app.HubTooOld(c.HubVersion, want) {
			if c.Log != nil {
				c.Log.Info("an official app needs a newer hub", "app", dir, "needs", want)
			}
			continue
		}
		m, err := domain.ParseManifest(raw)
		if err != nil || !strings.HasPrefix(m.ID, "myco/") || files[m.Entry] == nil {
			if c.Log != nil {
				c.Log.Warn("an official app is unusable", "app", dir, "err", err)
			}
			continue
		}
		out = append(out, app.OfficialTemplate{Manifest: m, Files: files})
	}
	return out, nil
}

// merge keeps, for each app, the newer of the binary's and the fetched.
func merge(built, got []app.OfficialTemplate) ([]app.OfficialTemplate, []string) {
	by := map[string]int{}
	out := make([]app.OfficialTemplate, 0, len(built)+len(got))
	for _, o := range built {
		if o.Manifest.ID != "myco/poll" {
			out = append(out, o)
		}
	}
	for i, o := range out {
		by[o.Manifest.ID] = i
	}
	var newer []string
	for _, g := range got {
		// Removed apps must not return from a previously fetched archive.
		if g.Manifest.ID == "myco/poll" {
			continue
		}
		if i, ok := by[g.Manifest.ID]; ok {
			if domain.CompareVersions(g.Manifest.Version, out[i].Manifest.Version) > 0 {
				out[i] = g
				newer = append(newer, g.Manifest.ID)
			}
			continue
		}
		by[g.Manifest.ID] = len(out)
		out = append(out, g)
		newer = append(newer, g.Manifest.ID)
	}
	sortOfficial(out)
	return out, newer
}

// changed lists the apps that are new or at another version.
func changed(before, after []app.OfficialTemplate) []string {
	was := map[string]string{}
	for _, o := range before {
		was[o.Manifest.ID] = o.Manifest.Version
	}
	var out []string
	for _, o := range after {
		if was[o.Manifest.ID] != o.Manifest.Version {
			out = append(out, o.Manifest.ID+"@"+o.Manifest.Version)
		}
	}
	return out
}
