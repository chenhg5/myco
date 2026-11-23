package templates_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/templates"
)

func appsZip(t *testing.T, apps map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for dir, manifest := range apps {
		for name, body := range map[string]string{"myco.json": manifest, "index.html": "<script src=app.js></script>", "agent.md": "how", "sample.json": "{}"} {
			w, err := z.Create(dir + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			w.Write([]byte(body))
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Official apps published on their own reach a running hub: newer versions
// replace the built-in ones, new apps join, apps needing a newer hub wait;
// what was fetched is kept for the next start.
func TestCatalogFetchesPublishedApps(t *testing.T) {
	archive := appsZip(t, map[string]string{
		"todo":   `{"id": "myco/todo", "name": "待办清单", "version": "9.0.0", "category": "tools", "description": "newer"}`,
		"new":    `{"id": "myco/new", "name": "新应用", "version": "1.0.0", "category": "life"}`,
		"future": `{"id": "myco/future", "name": "未来", "version": "1.0.0", "requires": {"hub": "99.0.0"}, "something": "new"}`,
		"old":    `{"id": "myco/table", "name": "表格", "version": "0.0.1"}`,
	})
	sum := sha256.Sum256(archive)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apps/apps.zip":
			w.Write(archive)
		case "/apps/SHA256SUMS":
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  apps.zip\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := &templates.Catalog{Download: srv.URL, StateDir: dir, HubVersion: "v0.20.7"}
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	var got []app.OfficialTemplate
	c.Attach(func(l []app.OfficialTemplate) { got = l })
	built := len(got)
	r := c.Sync(context.Background())
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	ids := map[string]string{}
	for _, o := range got {
		ids[o.Manifest.ID] = o.Manifest.Version
	}
	if ids["myco/todo"] != "9.0.0" || ids["myco/new"] != "1.0.0" || ids["myco/future"] != "" || ids["myco/table"] == "0.0.1" || len(got) != built+1 {
		t.Fatalf("after a sync: %v (%d built in) %+v", ids, built, r)
	}
	if strings.Join(r.Updated, ",") == "" {
		t.Fatalf("a sync should say what changed: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "official-apps.zip")); err != nil {
		t.Fatalf("the archive should be kept: %v", err)
	}
	// The next start uses what was kept, without the network.
	again := &templates.Catalog{StateDir: dir, HubVersion: "v0.20.7", Offline: true}
	if err := again.Load(); err != nil {
		t.Fatal(err)
	}
	var later []app.OfficialTemplate
	again.Attach(func(l []app.OfficialTemplate) { later = l })
	found := false
	for _, o := range later {
		found = found || o.Manifest.ID == "myco/new"
	}
	if !found {
		t.Fatal("the kept apps should be offered after a restart")
	}
	// A tampered archive is refused.
	tampered := append([]byte{}, archive...)
	tampered[40] ^= 0xff
	archive = tampered
	fresh := &templates.Catalog{Download: srv.URL, HubVersion: "v0.20.7", RetryWait: time.Millisecond}
	_ = fresh.Load()
	if r := fresh.Sync(context.Background()); r.Error == "" || !strings.Contains(r.Error, "being published") {
		t.Fatalf("an archive that does not match SHA256SUMS should be refused, saying why: %+v", r)
	}
}

// Fetched while the release is being published (the new archive, the old
// checksum): the hub waits and fetches again, and gets the new apps.
func TestCatalogRetriesWhilePublishing(t *testing.T) {
	archive := appsZip(t, map[string]string{"new": `{"id": "myco/new", "name": "新应用", "version": "1.0.0", "category": "life"}`})
	sum := sha256.Sum256(archive)
	sums := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apps/apps.zip":
			w.Write(archive)
		case "/apps/SHA256SUMS":
			sums++
			if sums == 1 {
				w.Write([]byte(strings.Repeat("ab", 32) + "  apps.zip\n"))
				return
			}
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  apps.zip\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &templates.Catalog{Download: srv.URL, HubVersion: "v0.20.7", RetryWait: time.Millisecond}
	_ = c.Load()
	if r := c.Sync(context.Background()); r.Error != "" || sums != 2 || len(r.Updated) == 0 {
		t.Fatalf("%d fetches of the checksums: %+v", sums, r)
	}
}

// Opening a page made from an official app has the hub look for newer apps
// when it has not in a while: once, not on every open; never when offline.
func TestCatalogFreshensWhenStale(t *testing.T) {
	archive := appsZip(t, map[string]string{"todo": `{"id": "myco/todo", "name": "待办清单", "version": "9.0.0", "category": "tools"}`})
	sum := sha256.Sum256(archive)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apps/apps.zip":
			hits++
			w.Write(archive)
		case "/apps/SHA256SUMS":
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  apps.zip\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	off := &templates.Catalog{Download: srv.URL, HubVersion: "v0.20.7", Offline: true}
	off.Freshen(context.Background(), time.Hour)
	if hits != 0 {
		t.Fatalf("an offline hub fetched: %d", hits)
	}
	c := &templates.Catalog{Download: srv.URL, HubVersion: "v0.20.7"}
	var got []app.OfficialTemplate
	c.Attach(func(l []app.OfficialTemplate) { got = l })
	c.Freshen(context.Background(), time.Hour)
	if hits != 1 {
		t.Fatalf("a hub that never fetched should fetch on open: %d", hits)
	}
	found := false
	for _, o := range got {
		found = found || (o.Manifest.ID == "myco/todo" && o.Manifest.Version == "9.0.0")
	}
	if !found {
		t.Fatal("the fetched version was not offered")
	}
	c.Freshen(context.Background(), time.Hour)
	if hits != 1 {
		t.Fatalf("fetched again within the hour: %d", hits)
	}
}

// A hub whose first fetch fails (no network at start) tries again soon,
// not a day later: the binary carries only the builtin apps.
func TestCatalogRetriesAFailedFirstFetch(t *testing.T) {
	defer templates.SetRunWaits(10*time.Millisecond, 20*time.Millisecond, 40*time.Millisecond)()
	archive := appsZip(t, map[string]string{"new": `{"id": "myco/new", "name": "新应用", "version": "1.0.0", "category": "life"}`})
	sum := sha256.Sum256(archive)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apps/SHA256SUMS" && r.URL.Path != "/apps/apps.zip" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/apps/SHA256SUMS" {
			calls++
		}
		if calls <= 2 {
			http.Error(w, "down", http.StatusNotFound) // the network is not there yet
			return
		}
		if r.URL.Path == "/apps/apps.zip" {
			w.Write(archive)
		} else {
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  apps.zip\n"))
		}
	}))
	defer srv.Close()
	c := &templates.Catalog{Download: srv.URL, Interval: time.Hour, RetryWait: time.Millisecond}
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	got := make(chan []app.OfficialTemplate, 16)
	c.Attach(func(l []app.OfficialTemplate) { got <- l })
	<-got // the built-in ones
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case l := <-got:
			for _, o := range l {
				if o.Manifest.ID == "myco/new" {
					return
				}
			}
		case <-deadline:
			t.Fatalf("the published apps did not arrive after the first fetches failed (%d tries)", calls)
		}
	}
}
