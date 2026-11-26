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
// "always" is kept across runs (but never for exec or files.delete);
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

// From hubs that say an app's declarations are all it declares (v0.22.9),
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
	// a page of myco's own apps kept from before declarations: the hub does
	// not say it declares all (httpapi declaresAll), so it is asked
	set("legacy", osDecl{app: true, official: true})
	if !asks("legacy", "fs.scan", map[string]string{"path": "~"}) {
		t.Fatal("an official page from before declarations is asked")
	}
	set("official", osDecl{app: true, declares: true, official: true})
	if !refused("official", "fs.scan", map[string]string{"path": "~"}) {
		t.Fatal("myco's own app is held to what it declares, nothing included")
	}
	set("official2", osDecl{app: true, declares: true, official: true, os: []string{"files"}})
	if !refused("official2", "exec.run", map[string]string{"cmd": "ls"}) {
		t.Fatal("myco's own app that declares is held to it")
	}
}

// An app's new version keeps what the person allowed that its manifest
// still declares: only groups and hosts it adds are asked for, a "no" holds
// for every version, and once-only groups and pages with no manifest are
// asked again. The screen and the browser may be allowed always.
func TestOSGrantsAcrossVersions(t *testing.T) {
	dir := t.TempDir()
	b := &Backend{Local: &Local{Home: filepath.Join(dir, "home")}, Store: &Store{path: filepath.Join(dir, "hubs.json")}}
	g := b.grants()
	set := func(page, ver string, app bool, os, net []string) {
		g.decl[osKey("h1", page)] = osDecl{app: app, os: os, net: net, ver: ver, at: time.Now()}
	}
	call := func(page, name string, args map[string]string) (*osConsent, error) {
		raw, _ := json.Marshal(args)
		return b.osCheck(t.Context(), osCallIn{Name: name, Args: raw, Page: osPage{Hub: "h1", ID: page}})
	}
	file := map[string]string{"path": "~/a", "body": "hi"}
	fetch := func(host string) map[string]string { return map[string]string{"url": "https://" + host + "/v1"} }
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	answer := func(page, group, host, a, want string) {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"hub": "h1", "page": page, "group": group, "host": host, "answer": a})
		r, err := http.Post(srv.URL+"/app/os/grants", "application/json", strings.NewReader(string(raw)))
		if err != nil || r.StatusCode != 200 {
			t.Fatalf("answer %s: %v %v", group, r, err)
		}
		var out struct{ Answer string }
		json.NewDecoder(r.Body).Decode(&out)
		r.Body.Close()
		if out.Answer != want {
			t.Fatalf("answer %s %s: kept as %q, want %q", group, a, out.Answer, want)
		}
	}
	allowed := func(page, name string, args map[string]string, why string) {
		t.Helper()
		if c, err := call(page, name, args); c != nil || err != nil {
			t.Fatalf("%s: %+v %v", why, c, err)
		}
	}
	asked := func(page, name string, args map[string]string, why string) *osConsent {
		t.Helper()
		c, err := call(page, name, args)
		if err != nil || c == nil || c.Error != "consent_required" {
			t.Fatalf("%s: %+v %v", why, c, err)
		}
		return c
	}

	v1 := []string{"files", "notify", "browser", "screen", "exec", "network"}
	set("pg_v", "1/1.0.0", true, v1, []string{"api.example.com"})
	for _, gr := range []string{"files", "browser", "network"} {
		answer("pg_v", gr, "", "always", "always")
	}
	answer("pg_v", "notify", "", "deny", "deny")
	answer("pg_v", "exec", "", "always", "once") // (never always)
	if c := asked("pg_v", "screen.record", nil, "screen is asked"); c.OnceOnly {
		t.Fatal("the screen may be allowed always")
	}
	if c := asked("pg_v", "fs.trash", file, "files.delete is asked"); !c.OnceOnly {
		t.Fatal("deleting files is once only")
	}
	answer("pg_v", "screen", "", "always", "always")
	allowed("pg_v", "fs.read", file, "files allowed")
	allowed("pg_v", "browser.open", nil, "browser allowed always")
	allowed("pg_v", "exec.spawn", nil, "exec allowed this run")
	allowed("pg_v", "net.fetch", fetch("api.example.com"), "declared host")
	raw, _ := os.ReadFile(filepath.Join(dir, "app-grants.json"))
	for _, want := range []string{`"version": "1/1.0.0"`, `"browser"`, `"screen"`, `"net": [`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("grants file lacks %s: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), `"exec"`) {
		t.Fatalf("exec is never kept: %s", raw)
	}

	// The app upgraded and declares a new group and a new host: what was
	// allowed is kept, the new ones are asked for, the "no" holds.
	set("pg_v", "1/1.1.0", true, append(v1, "clipboard"), []string{"api.example.com", "cdn.example.com"})
	allowed("pg_v", "fs.read", file, "files kept on upgrade")
	allowed("pg_v", "browser.open", nil, "browser kept on upgrade")
	allowed("pg_v", "screen.record", nil, "screen kept on upgrade")
	allowed("pg_v", "net.fetch", fetch("api.example.com"), "a host declared before")
	if c := asked("pg_v", "net.fetch", fetch("cdn.example.com"), "a host the new version declares"); c.Host != "cdn.example.com" || !c.Declared {
		t.Fatalf("new host: %+v", c)
	}
	asked("pg_v", "clipboard.read", nil, "a group the new version declares")
	asked("pg_v", "exec.spawn", nil, "exec is asked again for new code")
	if _, err := call("pg_v", "notify", nil); err == nil {
		t.Fatal("a no holds for every version")
	}
	answer("pg_v", "network", "cdn.example.com", "always", "always")
	allowed("pg_v", "net.fetch", fetch("cdn.example.com"), "new host allowed")

	// A version that no longer declares a group drops its allow.
	set("pg_v", "2/1.2.0", true, []string{"files", "network", "screen"}, []string{"api.example.com", "cdn.example.com"})
	allowed("pg_v", "fs.read", file, "files kept on republish")
	asked("pg_v", "browser.open", nil, "browser no longer declared")
	set("pg_v", "2/1.3.0", true, []string{"files", "network", "screen", "browser"}, []string{"api.example.com", "cdn.example.com"})
	asked("pg_v", "browser.open", nil, "a dropped group declared again is asked again")

	// Kept across runs of the App.
	b2 := &Backend{Store: &Store{path: filepath.Join(dir, "hubs.json")}}
	b2.grants().decl[osKey("h1", "pg_v")] = g.decl[osKey("h1", "pg_v")]
	raw2, _ := json.Marshal(file)
	for _, name := range []string{"fs.read", "screen.record"} {
		if c, err := b2.osCheck(t.Context(), osCallIn{Name: name, Args: raw2, Page: osPage{Hub: "h1", ID: "pg_v"}}); c != nil || err != nil {
			t.Fatalf("%s kept across runs: %+v %v", name, c, err)
		}
	}

	// A page with no manifest: nothing says what its new code uses.
	set("pg_plain", "1", false, nil, nil)
	answer("pg_plain", "files", "", "always", "always")
	allowed("pg_plain", "fs.read", file, "plain page allowed")
	set("pg_plain", "2", false, nil, nil)
	asked("pg_plain", "fs.read", file, "a republished plain page is asked again")

	// Grants kept before versions (no version): asked once more.
	b3 := &Backend{Store: &Store{path: filepath.Join(t.TempDir(), "hubs.json")}}
	os.WriteFile(filepath.Join(filepath.Dir(b3.Store.path), "app-grants.json"), []byte(`[{"hub":"h1","page":"pg_v","groups":["files"],"at":1}]`), 0o600)
	b3.grants().decl[osKey("h1", "pg_v")] = osDecl{app: true, os: []string{"files"}, ver: "2/1.1.0", at: time.Now()}
	if c, _ := b3.osCheck(t.Context(), osCallIn{Name: "fs.read", Args: raw2, Page: osPage{Hub: "h1", ID: "pg_v"}}); c == nil {
		t.Fatal("an old grant with no version is asked again")
	}
}
