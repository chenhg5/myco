package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/release"
)

// Update notices. Ordinary commands never wait for the network: they start
// a short background refresh from the hub (at most once a day) and, when
// they finish, print what an earlier refresh already cached, at most once
// a day, and only in an interactive terminal (agents' JSON stays clean).

const updateCheckEvery = 24 * time.Hour

// updateCache is ~/.myco/update-check.json.
type updateCache struct {
	Hub string `json:"hub,omitempty"`
	// HubVersion is the hub's build; HubKnown is false when the hub did not
	// report one (it predates version reporting).
	HubVersion string    `json:"hub_version,omitempty"`
	HubKnown   bool      `json:"hub_known,omitempty"`
	Latest     string    `json:"latest,omitempty"`
	CheckedAt  time.Time `json:"checked_at,omitempty"`
	NotifiedAt time.Time `json:"notified_at,omitempty"`
}

func updateCachePath() string {
	return filepath.Join(config.Dir(config.DefaultPath()), "update-check.json")
}

func loadUpdateCache() updateCache {
	var c updateCache
	if b, err := os.ReadFile(updateCachePath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func saveUpdateCache(c updateCache) {
	path := updateCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	b, _ := json.Marshal(c)
	f, err := os.CreateTemp(filepath.Dir(path), ".update-check-*")
	if err != nil {
		return
	}
	_, werr := f.Write(b)
	if cerr := f.Close(); werr != nil || cerr != nil || os.Rename(f.Name(), path) != nil {
		os.Remove(f.Name())
	}
}

// knownHub is the hub this machine talks to: MYCO_HUB, the current config,
// or the machine's join credentials.
func knownHub(getenv func(string) string) string {
	if h := strings.TrimSpace(getenv(config.EnvHub)); h != "" {
		return h
	}
	path := getenv(config.EnvConfig)
	if path == "" {
		path = config.DefaultPath()
	}
	if c, err := config.Load(path); err == nil && c.Hub != "" {
		return c.Hub
	}
	if m, err := config.LoadMachine(config.DefaultPath()); err == nil && m != nil {
		return m.Hub
	}
	return ""
}

// hubHealth asks the hub for its version, bounded by timeout.
func hubHealth(ctx context.Context, hub string, timeout time.Duration) (*api.Health, error) {
	c, err := client.New(hub, "")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.Health(ctx)
}

// startUpdateRefresh refreshes the cache in the background when it is older
// than a day. It never blocks: if the process exits first, the next
// command tries again.
func startUpdateRefresh(getenv func(string) string) {
	if release.Disabled(getenv) {
		return
	}
	c := loadUpdateCache()
	hub := knownHub(getenv)
	if hub == "" || (hub == c.Hub && time.Since(c.CheckedAt) < updateCheckEvery) {
		return
	}
	go func() {
		h, err := hubHealth(context.Background(), hub, 3*time.Second)
		if err != nil {
			return
		}
		c := loadUpdateCache()
		c.Hub, c.HubVersion, c.HubKnown, c.Latest, c.CheckedAt = hub, h.ServerVersion, h.ServerVersion != "", h.LatestVersion, time.Now()
		saveUpdateCache(c)
	}()
}

// updateNotices are the lines to show for what the cache knows.
func updateNotices(c updateCache, self string) []string {
	var out []string
	if release.Newer(c.Latest, self) {
		out = append(out, fmt.Sprintf("myco %s is available (this machine has %s): run `myco upgrade`", c.Latest, self))
	}
	switch {
	case c.Hub != "" && !c.HubKnown && !c.CheckedAt.IsZero():
		out = append(out, fmt.Sprintf("the hub at %s runs an older myco (before v0.16.6): upgrade it with `sudo myco upgrade` on that machine, or re-run install-hub.sh", c.Hub))
	case c.HubKnown && release.Newer(self, c.HubVersion):
		out = append(out, fmt.Sprintf("the hub at %s runs myco %s, older than this machine's %s: upgrade it with `sudo myco upgrade` on that machine", c.Hub, c.HubVersion, self))
	}
	return out
}

// printUpdateNotice prints cached notices at most once a day.
func printUpdateNotice(w io.Writer, getenv func(string) string) {
	if release.Disabled(getenv) {
		return
	}
	c := loadUpdateCache()
	if time.Since(c.NotifiedAt) < updateCheckEvery {
		return
	}
	notes := updateNotices(c, Version)
	if len(notes) == 0 {
		return
	}
	for _, n := range notes {
		fmt.Fprintln(w, "notice: "+n)
	}
	c.NotifiedAt = time.Now()
	saveUpdateCache(c)
}

// noticeSkips are commands that never show the notice (they report versions
// themselves, or run for a long time / for machines).
var noticeSkips = []string{"version", "upgrade", "hub", "runner", "ui", "inbox wait"}

func wantsUpdateNotice(path string) bool {
	path = strings.TrimSpace(strings.TrimPrefix(path, "myco"))
	for _, s := range noticeSkips {
		if path == s || strings.HasPrefix(path, s+" ") {
			return false
		}
	}
	return path != ""
}

// latestRelease is the newest release for explicit checks (version, doctor,
// upgrade): the hub's answer when it has one, else GitHub directly.
func latestRelease(ctx context.Context, getenv func(string) string, fromHub string, timeout time.Duration) (string, error) {
	if fromHub != "" {
		return fromHub, nil
	}
	if release.Disabled(getenv) {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return release.Latest(ctx, &http.Client{Timeout: timeout}, release.APIURL(getenv))
}

// watchVersions logs the hub's version and available updates at start-up and
// then daily, in the background (runner start).
func (e *env) watchVersions(log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if release.Disabled(e.in.Getenv) {
		return
	}
	hub := knownHub(e.in.Getenv)
	if hub == "" {
		return
	}
	self := Version // read once, before the goroutine
	go func() {
		for {
			if h, err := hubHealth(e.in.Ctx, hub, 10*time.Second); err == nil {
				c := updateCache{Hub: hub, HubVersion: h.ServerVersion, HubKnown: h.ServerVersion != "", Latest: h.LatestVersion, CheckedAt: time.Now()}
				for _, n := range updateNotices(c, self) {
					log.Warn(n)
				}
				log.Info("versions", "myco", self, "hub", orDefault(h.ServerVersion, "older than v0.16.6"), "latest", orDefault(h.LatestVersion, "unknown"))
			}
			select {
			case <-e.in.Ctx.Done():
				return
			case <-time.After(updateCheckEvery):
			}
		}
	}()
}
