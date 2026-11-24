package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOSGroups(t *testing.T) {
	for name, want := range map[string]string{
		"fs.read": "files", "fs.pick": "files", "fs.write": "files.write", "fs.trash": "files.delete",
		"exec.spawn": "exec", "process.kill": "exec", "fs.open": "exec", "screen.record": "screen",
		"audio.record": "microphone", "speech.transcribe": "speech", "clipboard.read": "clipboard",
		"clipboard.write": "", "notify": "notify", "net.fetch": "network", "system.info": "system",
		"ssh.run": "ssh", "db.query": "database", "proxy.control": "proxy", "something.new": "system",
	} {
		if g := osGroupOf(name); g != want {
			t.Errorf("%s: %q, want %q", name, g, want)
		}
	}
}

func TestOSHosts(t *testing.T) {
	list := []string{"api.example.com", "*.example.org", "lan.box:8080"}
	for host, want := range map[string]bool{
		"api.example.com": true, "api.example.com:443": true, "x.example.org": true, "a.b.example.org": true,
		"example.org": false, "lan.box:8080": true, "lan.box:9090": false, "evil.com": false,
	} {
		if hostMatches(list, host) != want {
			t.Errorf("%s: %v", host, !want)
		}
	}
	for host, want := range map[string]bool{
		"localhost:5173": true, "127.0.0.1": true, "192.168.1.9:7777": true, "10.0.0.1": true, "[::1]:80": true,
		"169.254.1.1": true, "printer.local": true, "nas": true, "api.example.com": false, "8.8.8.8": false,
	} {
		if isPrivateHost(host) != want {
			t.Errorf("private %s: %v", host, !want)
		}
	}
}

// A page asks the person before it uses the computer: the first call in a
// group answers consent_required; once allowed it goes; denied it is refused;
// "always" is kept across runs (but never for exec, files.delete, screen);
// files the person picked need no asking.
func TestOSConsent(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	store := &Store{path: filepath.Join(dir, "hubs.json")}
	b := &Backend{Local: &Local{Home: home}, Store: store}
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	post := func(path string, body any) (int, map[string]any) {
		raw, _ := json.Marshal(body)
		r, err := http.Post(srv.URL+path, "application/json", strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		return r.StatusCode, m
	}
	page := map[string]string{"hub": "h1", "id": "pg_1", "name": "空间清理"}
	call := func(name string, args any) (int, map[string]any) {
		return post("/app/os/call", map[string]any{"name": name, "args": args, "page": page})
	}
	answer := func(group, a string) {
		if code, m := post("/app/os/grants", map[string]string{"hub": "h1", "page": "pg_1", "name": "空间清理", "group": group, "answer": a}); code != 200 {
			t.Fatalf("answer %s %s: %d %v", group, a, code, m)
		}
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if code, m := call("fs.paths", nil); code != 403 || m["error"] != "consent_required" || m["group"] != "files" || m["declared"] != true {
		t.Fatalf("first use asks: %d %v", code, m)
	}
	// writing the clipboard needs nobody's say
	if code, m := call("clipboard.write", map[string]string{"text": "x"}); code == 403 {
		t.Fatalf("clipboard.write: %v", m)
	}
	answer("files", "once")
	if code, m := call("fs.paths", nil); code != 200 {
		t.Fatalf("allowed: %d %v", code, m)
	}
	answer("notify", "deny")
	if code, m := call("notify", map[string]string{"title": "x"}); code != 403 || m["error"] != "permission_denied" {
		t.Fatalf("denied: %d %v", code, m)
	}
	// exec can only be allowed this time; files.write always is kept
	if code, m := call("exec.run", map[string]any{"argv": []string{"true"}}); m["once_only"] != true {
		t.Fatalf("exec asks once only: %d %v", code, m)
	}
	answer("exec", "always")
	answer("files.write", "always")
	raw, _ := os.ReadFile(filepath.Join(dir, "app-grants.json"))
	if !strings.Contains(string(raw), `"files.write"`) || strings.Contains(string(raw), `"exec"`) || strings.Contains(string(raw), `"files"`+",") {
		t.Fatalf("kept: %s", raw)
	}
	// a new run of the App: always is still there, this time is not
	b2 := &Backend{Local: &Local{Home: home}, Store: store}
	g := b2.grants()
	if p := g.pages[osKey("h1", "pg_1")]; p == nil || !p.always["files.write"] || p.once["files"] || p.always["exec"] {
		t.Fatalf("after a restart: %+v", p)
	}
	// what the person picked is the page's to read without asking for files
	b2.osPicked(osCallIn{Name: "fs.pick", Page: osPage{Hub: "h1", ID: "pg_2"}}, map[string]any{"paths": []any{"/tmp/a/photo.jpg", "/tmp/b"}})
	for path, ok := range map[string]bool{"/tmp/a/photo.jpg": true, "/tmp/b/c.txt": true, "/tmp/a/other.jpg": false, "/tmp/b/../x": false} {
		args, _ := json.Marshal(map[string]string{"path": path})
		c, err := b2.osCheck(t.Context(), osCallIn{Name: "fs.read", Args: args, Page: osPage{Hub: "h1", ID: "pg_2"}})
		if err != nil || (c == nil) != ok {
			t.Errorf("read %s: consent %v err %v", path, c, err)
		}
	}
	// revoked: asked again
	req, _ := http.NewRequest("DELETE", srv.URL+"/app/os/grants?hub=h1&page=pg_1&group=files", nil)
	if r, err := http.DefaultClient.Do(req); err != nil || r.StatusCode != 204 {
		t.Fatal("revoke", err)
	}
	if code, m := call("fs.paths", nil); code != 403 || m["error"] != "consent_required" {
		t.Fatalf("revoked: %d %v", code, m)
	}
}

// The network: hosts the app declares go with the group's consent; others
// are asked for one by one; a private address an app with a "net" list does
// not name is refused.
func TestOSNetworkConsent(t *testing.T) {
	b := &Backend{}
	g := b.grants()
	key := osKey("h1", "pg_n")
	g.decl[key] = osDecl{app: true, os: []string{"network"}, net: []string{"api.example.com"}, at: time.Now()}
	in := func(url string) osCallIn {
		args, _ := json.Marshal(map[string]string{"url": url})
		return osCallIn{Name: "net.fetch", Args: args, Page: osPage{Hub: "h1", ID: "pg_n"}}
	}
	c, err := b.osCheck(t.Context(), in("https://api.example.com/v1"))
	if err != nil || c == nil || c.Group != "network" || c.Host != "" || !c.Declared {
		t.Fatalf("declared host asks for the network: %+v %v", c, err)
	}
	g.perm("h1", "pg_n").once["network"] = true
	if c, err := b.osCheck(t.Context(), in("https://api.example.com/v1")); c != nil || err != nil {
		t.Fatalf("declared host, network allowed: %+v %v", c, err)
	}
	if c, err := b.osCheck(t.Context(), in("https://other.example.net/")); err != nil || c == nil || c.Host != "other.example.net" {
		t.Fatalf("another host is asked for: %+v %v", c, err)
	}
	if _, err := b.osCheck(t.Context(), in("http://192.168.1.9:7777/")); err == nil {
		t.Fatal("a private address the app does not declare is refused")
	}
}

// From hubs that say an app's declarations are all it declares (v0.22.8),
// an app may not use what it does not declare: refused, not asked. Pages
// with no app, older hubs and myco's own apps that declare nothing yet are
// asked as before; the person's own pick is no way around it.
func TestOSUndeclaredRefused(t *testing.T) {
	b := &Backend{}
	g := b.grants()
	set := func(page string, d osDecl) { d.at = time.Now(); g.decl[osKey("h1", page)] = d }
	call := func(page, name string, args any) osCallIn {
		raw, _ := json.Marshal(args)
		return osCallIn{Name: name, Args: raw, Page: osPage{Hub: "h1", ID: page}}
	}
	refused := func(page, name string, args any) bool {
		c, err := b.osCheck(t.Context(), call(page, name, args))
		re, ok := err.(*osRefusal)
		return c == nil && ok && re.Code == "permission_denied" && strings.Contains(re.Message, `"os"`)
	}
	asks := func(page, name string, args any) bool {
		c, err := b.osCheck(t.Context(), call(page, name, args))
		return err == nil && c != nil && c.Error == "consent_required"
	}
	set("strict", osDecl{app: true, declares: true, os: []string{"files"}})
	if !refused("strict", "exec.run", map[string]string{"cmd": "ls"}) || !refused("strict", "net.fetch", map[string]string{"url": "https://api.example.com"}) {
		t.Fatal("an undeclared group is refused")
	}
	if !asks("strict", "fs.read", map[string]string{"path": "~/a"}) {
		t.Fatal("a declared group is asked for")
	}
	set("nofiles", osDecl{app: true, declares: true, os: []string{"notify"}})
	if !refused("nofiles", "fs.pick", map[string]any{}) {
		t.Fatal("picking is files too: refused when files is not declared")
	}
	g.perm("h1", "nofiles").picked = []string{"/tmp/picked.txt"}
	if !refused("nofiles", "fs.read", map[string]string{"path": "/tmp/picked.txt"}) {
		t.Fatal("a picked file is not a way around the declaration")
	}
	set("old", osDecl{app: true, os: []string{"files"}})
	if !asks("old", "exec.run", map[string]string{"cmd": "ls"}) {
		t.Fatal("an older hub does not say: asked as before")
	}
	set("plain", osDecl{declares: true})
	if !asks("plain", "exec.run", map[string]string{"cmd": "ls"}) {
		t.Fatal("a page with no app is asked")
	}
	set("official", osDecl{app: true, declares: true, official: true})
	if !asks("official", "fs.scan", map[string]string{"path": "~"}) {
		t.Fatal("myco's own app that declares nothing yet is asked for now")
	}
	set("official2", osDecl{app: true, declares: true, official: true, os: []string{"files"}})
	if !refused("official2", "exec.run", map[string]string{"cmd": "ls"}) {
		t.Fatal("myco's own app that declares is held to it")
	}
}
