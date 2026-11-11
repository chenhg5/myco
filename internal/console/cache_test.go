package console_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chenhg5/myco/internal/console"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestCacheStoresEncryptedAndStartsOverWhenUnreadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-cache.db")
	k := key(t)
	c, err := console.OpenCache(path, k)
	if err != nil {
		t.Fatal(err)
	}
	secret := `{"body":"the launch code is swordfish"}`
	if err := c.Put([]console.CacheEntry{{Key: "thread\x00h1\x00t1", Hub: "h1", Data: json.RawMessage(secret)}}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := c.All(map[string]bool{"h1": true})
	if err != nil || len(got) != 1 || string(got[0].Data) != secret {
		t.Fatalf("%v %+v", err, got)
	}
	c.Close()
	// Nothing readable on disk.
	for _, suffix := range []string{"", "-wal"} {
		raw, _ := os.ReadFile(path + suffix)
		if bytes.Contains(raw, []byte("swordfish")) {
			t.Fatalf("plain text in %s", path+suffix)
		}
	}

	// Another key (the keychain entry was lost): the entry cannot be read
	// and is dropped, without an error.
	c, err = console.OpenCache(path, key(t))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.All(map[string]bool{"h1": true}); err != nil || len(got) != 0 {
		t.Fatalf("another key read it: %v %+v", err, got)
	}
	c.Close()

	// A damaged file: started over.
	if err := os.WriteFile(path, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"-wal", "-shm"} {
		os.Remove(path + s)
	}
	c, err = console.OpenCache(path, k)
	if err != nil {
		t.Fatalf("damaged file: %v", err)
	}
	if n, _, _ := c.Stats(); n != 0 {
		t.Fatalf("entries after starting over: %d", n)
	}
	c.Close()
}

func TestCacheKeepsOnlyHubsInTheAppAndTrims(t *testing.T) {
	c, err := console.OpenCache(filepath.Join(t.TempDir(), "c.db"), key(t))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	put := func(k, hub string, size int) {
		t.Helper()
		data, _ := json.Marshal(strings.Repeat("x", size))
		if err := c.Put([]console.CacheEntry{{Key: k, Hub: hub, Data: data}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	put("a", "h1", 10)
	put("b", "h2", 10)
	put("all", "", 10)
	got, _ := c.All(map[string]bool{"h1": true})
	keys := []string{}
	for _, e := range got {
		keys = append(keys, e.Key)
	}
	if strings.Join(keys, ",") != "all,a" {
		t.Fatalf("a hub no longer in the app keeps its entries: %v", keys)
	}
	if n, _, _ := c.Stats(); n != 2 {
		t.Fatalf("h2's entry not deleted: %d", n)
	}

	// Forgetting a hub also drops what mixes several hubs.
	if err := c.ForgetHub("h1"); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := c.Stats(); n != 0 {
		t.Fatalf("after forgetting h1: %d entries", n)
	}
	c.AllowHub("h1") // a later successful answer permits fresh cache entries

	// Over the size limit: the least recently used go first.
	c.Max = 3000
	put("old", "h1", 1000)
	put("mid", "h1", 1000)
	put("new", "h1", 1000)
	got, _ = c.All(map[string]bool{"h1": true})
	keys = keys[:0]
	for _, e := range got {
		keys = append(keys, e.Key)
	}
	if strings.Join(keys, ",") != "new,mid" {
		t.Fatalf("trim: %v", keys)
	}
}

func TestCacheRoutes(t *testing.T) {
	dir := t.TempDir()
	store := console.NewStore(filepath.Join(dir, "hubs.json"))
	// A hub that no longer takes the app's token.
	var deny atomic.Bool
	deny.Store(true)
	revoked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if deny.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":"unauthorized","message":"invalid token"}`)
			return
		}
		io.WriteString(w, `{"members":[]}`)
	}))
	defer revoked.Close()
	h1, _ := store.Add("one", revoked.URL)
	h2, _ := store.Add("two", "http://127.0.0.1:1")
	secrets := &memSecrets{m: map[string]string{h1.ID: "tok", h2.ID: "tok"}}
	c, err := console.OpenCache(filepath.Join(dir, "c.db"), key(t))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	app := httptest.NewServer((&console.Backend{Store: store, Secrets: secrets, Cache: c}).Handler())
	defer app.Close()
	call := func(method, path, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(method, app.URL+path, strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	put := `{"entries":[{"key":"m1","hub":"` + h1.ID + `","data":{"x":1}},{"key":"m2","hub":"` + h2.ID + `","data":[1,2]},{"key":"local","hub":"","data":{"host":"mac"}}]}`
	if code, _ := call("PUT", "/app/cache", put); code != 204 {
		t.Fatalf("put: %d", code)
	}
	code, body := call("GET", "/app/cache", "")
	if code != 200 || !strings.Contains(body, `"key":"m1"`) || !strings.Contains(body, `"data":{"x":1}`) || !strings.Contains(body, `"enabled":true`) {
		t.Fatalf("get: %d %s", code, body)
	}

	// A 401 from hub one: its entries (and mixed ones) are dropped.
	if code, _ := call("GET", "/app/hubs/"+h1.ID+"/members", ""); code != 401 {
		t.Fatalf("revoked hub answered %d", code)
	}
	_, body = call("GET", "/app/cache", "")
	if strings.Contains(body, `"m1"`) || strings.Contains(body, `"local"`) || !strings.Contains(body, `"m2"`) {
		t.Fatalf("after 401: %s", body)
	}
	// A delayed write from the UI cannot put that hub's old data back.
	call("PUT", "/app/cache", `{"entries":[{"key":"late","hub":"`+h1.ID+`","data":{"old":true}},{"key":"mixed","hub":"","data":1}]}`)
	_, body = call("GET", "/app/cache", "")
	if strings.Contains(body, `"late"`) || !strings.Contains(body, `"mixed"`) {
		t.Fatalf("rejected hub was written back, or mixed entry was blocked: %s", body)
	}
	// A successful answer permits new entries for the hub again.
	deny.Store(false)
	if code, _ := call("GET", "/app/hubs/"+h1.ID+"/members", ""); code != 200 {
		t.Fatalf("restored hub answered %d", code)
	}
	call("PUT", "/app/cache", `{"entries":[{"key":"fresh","hub":"`+h1.ID+`","data":{"new":true}}]}`)
	_, body = call("GET", "/app/cache", "")
	if !strings.Contains(body, `"fresh"`) {
		t.Fatalf("successful hub still blocked: %s", body)
	}
	// The overview's successful hub check also restores writes.
	deny.Store(true)
	if code, _ := call("GET", "/app/hubs/"+h1.ID+"/members", ""); code != 401 {
		t.Fatalf("revoked hub answered %d", code)
	}
	deny.Store(false)
	if code, _ := call("GET", "/app/overview", ""); code != 200 {
		t.Fatalf("overview answered %d", code)
	}
	call("PUT", "/app/cache", `{"entries":[{"key":"after-overview","hub":"`+h1.ID+`","data":2}]}`)
	_, body = call("GET", "/app/cache", "")
	if !strings.Contains(body, `"after-overview"`) {
		t.Fatalf("overview did not restore hub writes: %s", body)
	}

	// Removing hub two drops its entries.
	if code, _ := call("DELETE", "/app/hubs/"+h2.ID, ""); code != 200 {
		t.Fatalf("remove: %d", code)
	}
	if _, body = call("GET", "/app/cache", ""); strings.Contains(body, `"m2"`) {
		t.Fatalf("after removing the hub: %s", body)
	}

	// Clear, and stats.
	call("PUT", "/app/cache", `{"entries":[{"key":"z","hub":"","data":1}]}`)
	if code, _ := call("DELETE", "/app/cache", ""); code != 204 {
		t.Fatalf("clear: %d", code)
	}
	if _, body = call("GET", "/app/cache/stats", ""); !strings.Contains(body, `"entries":0`) {
		t.Fatalf("stats: %s", body)
	}

	// Without a cache: empty answers, writes ignored.
	plain := httptest.NewServer((&console.Backend{Store: store, Secrets: secrets}).Handler())
	defer plain.Close()
	resp, err := http.Get(plain.URL + "/app/cache")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"entries":[]`) || !strings.Contains(string(b), `"enabled":false`) {
		t.Fatalf("no cache: %s", b)
	}
}

func TestCacheStartsOverAfterAnUpdate(t *testing.T) {
	c, err := console.OpenCache(filepath.Join(t.TempDir(), "c.db"), key(t))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.ForVersion("v0.18.34"); err != nil {
		t.Fatal(err)
	}
	if err := c.Put([]console.CacheEntry{{Key: "k", Hub: "", Data: json.RawMessage(`{"shape":1}`)}}, nil); err != nil {
		t.Fatal(err)
	}
	// Same version (a restart): kept.
	if err := c.ForVersion("v0.18.34"); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := c.Stats(); n != 1 {
		t.Fatalf("restart lost the cache: %d", n)
	}
	// Another version (an update, or going back): what the pages saved may
	// have another shape now, so it starts over.
	if err := c.ForVersion("v0.18.35"); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := c.Stats(); n != 0 {
		t.Fatalf("kept %d entries across an update", n)
	}
}
