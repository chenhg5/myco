package console_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/console"
)

func osPost(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	r, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var m map[string]any
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &m)
	return r.StatusCode, m
}

func TestPagesUseThisComputer(t *testing.T) {
	home := t.TempDir()
	app := httptest.NewServer((&console.Backend{Local: &console.Local{Home: home}}).Handler())
	defer app.Close()

	// what there is
	r, err := http.Get(app.URL + "/app/os")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		OS   string `json:"os"`
		Caps []struct {
			Name      string `json:"name"`
			Job       bool   `json:"job"`
			Available bool   `json:"available"`
		} `json:"caps"`
	}
	_ = json.NewDecoder(r.Body).Decode(&list)
	r.Body.Close()
	if list.OS != runtime.GOOS || len(list.Caps) < 15 {
		t.Fatalf("list: %+v", list)
	}

	page := map[string]string{"hub": "h1", "id": "pg_1", "name": "空间清理"}
	// a call: write a file, read it back ("~" is the person's home)
	code, m := osPost(t, app.URL+"/app/os/call", map[string]any{"name": "fs.write", "args": map[string]string{"path": "~/x/a.txt", "content": "hello"}, "page": page})
	if code != 200 {
		t.Fatalf("write: %d %v", code, m)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "x", "a.txt")); string(b) != "hello" {
		t.Fatalf("not written in the home folder: %q", b)
	}
	code, m = osPost(t, app.URL+"/app/os/call", map[string]any{"name": "fs.read", "args": map[string]string{"path": "~/x/a.txt"}, "page": page})
	if code != 200 || m["value"].(map[string]any)["content"] != "hello" {
		t.Fatalf("read: %d %v", code, m)
	}
	// errors carry the capability's code
	code, m = osPost(t, app.URL+"/app/os/call", map[string]any{"name": "fs.read", "args": map[string]string{"path": "~/nope"}})
	if code != http.StatusNotFound || m["error"] != "not_found" {
		t.Fatalf("missing: %d %v", code, m)
	}
	code, m = osPost(t, app.URL+"/app/os/call", map[string]any{"name": "fs.scan", "args": map[string]string{"path": "~"}})
	if code != http.StatusBadRequest || !strings.Contains(m["message"].(string), "job") {
		t.Fatalf("a job called: %d %v", code, m)
	}

	// a job: scan, read its events to the end
	code, m = osPost(t, app.URL+"/app/os/jobs", map[string]any{"name": "fs.scan", "args": map[string]string{"path": "~"}, "page": page})
	if code != http.StatusCreated {
		t.Fatalf("start: %d %v", code, m)
	}
	id := m["id"].(string)
	var batch struct {
		Done  bool           `json:"done"`
		Next  int            `json:"next"`
		Value map[string]any `json:"value"`
	}
	for i := 0; i < 20 && !batch.Done; i++ {
		r, err := http.Get(app.URL + "/app/os/jobs/" + id + "?after=" + "0" + "&wait=1000")
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewDecoder(r.Body).Decode(&batch)
		r.Body.Close()
	}
	if !batch.Done || batch.Value["files"].(float64) != 2 { // (a.txt and the log)
		t.Fatalf("scan: %+v", batch)
	}

	// each call is in the log, with the page
	log, err := os.ReadFile(filepath.Join(home, ".myco", "logs", "app-os.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) != 5 || !strings.Contains(lines[0], `"name":"fs.write"`) || !strings.Contains(lines[0], "空间清理") || !strings.Contains(lines[2], `"error":"not_found"`) {
		t.Fatalf("log:\n%s", log)
	}

	// not without a local install (a backend that only shows hubs)
	bare := httptest.NewServer((&console.Backend{}).Handler())
	defer bare.Close()
	if code, _ := osPost(t, bare.URL+"/app/os/call", map[string]any{"name": "system.info"}); code == 200 {
		t.Fatal("answered without a local install")
	}
}

// Even short arguments can contain complete credentials. The local OS audit
// records the operation and byte count, never a prefix of the arguments.
func TestOSAuditDoesNotKeepArguments(t *testing.T) {
	home := t.TempDir()
	app := httptest.NewServer((&console.Backend{Local: &console.Local{Home: home}}).Handler())
	defer app.Close()
	secret := "audit-must-not-contain-this"
	raw := `{"name":"system.info","args":{"secret":"` + secret + `","headers":{"Authorization":"Bearer ` + secret + `"}},"page":{"id":"test"}}`
	res, err := http.Post(app.URL+"/app/os/call", "application/json", strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	data, err := os.ReadFile(filepath.Join(home, ".myco", "logs", "app-os.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) || !strings.Contains(string(data), "args_bytes") {
		t.Fatalf("unsafe audit: %s", data)
	}
}

func TestPageProxyProfilesStayLocalAndSeparate(t *testing.T) {
	home := t.TempDir()
	secrets := &memSecrets{m: map[string]string{}}
	app := httptest.NewServer((&console.Backend{Local: &console.Local{Home: home}, Secrets: secrets}).Handler())
	defer app.Close()
	page := map[string]string{"hub": "hub-a", "id": "page-a"}
	code, out := osPost(t, app.URL+"/app/os/call", map[string]any{"name": "proxy.profile", "page": page, "args": map[string]string{"action": "save", "endpoint": "http://127.0.0.1:9090", "engine": "mihomo", "secret": "page-private-secret"}})
	if code != 200 {
		t.Fatal(code, out)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "page-private-secret") {
		t.Fatal("profile exposed credential")
	}
	for _, p := range []map[string]string{{"hub": "hub-a", "id": "page-b"}, {"hub": "hub-b", "id": "page-a"}} {
		code, out = osPost(t, app.URL+"/app/os/call", map[string]any{"name": "proxy.profile", "page": p, "args": map[string]string{"action": "get"}})
		if code != 404 {
			t.Fatal("profile crossed page/workspace boundary", code, out)
		}
	}
	// A new backend (App restart) reuses the local credential store.
	app2 := httptest.NewServer((&console.Backend{Local: &console.Local{Home: home}, Secrets: secrets}).Handler())
	defer app2.Close()
	code, out = osPost(t, app2.URL+"/app/os/call", map[string]any{"name": "proxy.profile", "page": page, "args": map[string]string{"action": "get"}})
	if code != 200 {
		t.Fatal("profile lost on restart", code, out)
	}
	raw, _ = os.ReadFile(filepath.Join(home, ".myco", "logs", "app-os.jsonl"))
	if strings.Contains(string(raw), "page-private-secret") {
		t.Fatal("audit exposed credential")
	}
}
