package console

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A page installed as an app: a launcher bundle in ~/Applications/myco 应用
// that opens the page's link, with its icon, listed and removed again.
func TestInstallAPageAsAnApp(t *testing.T) {
	old := launcherKind
	launcherKind = "mac"
	defer func() { launcherKind = old }()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	store := &Store{path: filepath.Join(dir, "hubs.json")}
	h, err := store.Add("Main", "https://hub.example")
	if err != nil {
		t.Fatal(err)
	}
	b := &Backend{Local: &Local{Home: home}, Store: store, OpenPageWindow: func(string, string, string) error { return nil }}
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 100)...)
	body, _ := json.Marshal(map[string]string{"name": "发布/进度", "icon": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
	res, err := http.Post(srv.URL+"/app/hubs/"+h.ID+"/pages/pg_a/install", "application/json", bytes.NewReader(body))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("install: %v %v", res, err)
	}
	var e installedApp
	_ = json.NewDecoder(res.Body).Decode(&e)
	res.Body.Close()
	want := filepath.Join(home, "Applications", "myco 应用", "发布-进度.app")
	if e.Path != want {
		t.Fatalf("launcher at %q, want %q", e.Path, want)
	}
	script, _ := os.ReadFile(filepath.Join(want, "Contents", "MacOS", "launch"))
	if !strings.Contains(string(script), "exec /usr/bin/open 'myco://app?hub="+h.ID+"&name=") || !strings.Contains(string(script), "&page=pg_a'") {
		t.Fatalf("launch: %s", script)
	}
	if st, err := os.Stat(filepath.Join(want, "Contents", "MacOS", "launch")); err != nil || st.Mode()&0o100 == 0 {
		t.Fatalf("launch is not executable: %v %v", st, err)
	}
	plist, _ := os.ReadFile(filepath.Join(want, "Contents", "Info.plist"))
	if !strings.Contains(string(plist), "<string>发布/进度</string>") || !strings.Contains(string(plist), "dev.myco.app.") {
		t.Fatalf("plist: %s", plist)
	}
	icon, _ := os.ReadFile(filepath.Join(want, "Contents", "Resources", "icon.icns"))
	if string(icon[:4]) != "icns" || int(binary.BigEndian.Uint32(icon[4:])) != len(icon) || string(icon[8:12]) != "ic09" || !bytes.Equal(icon[16:], png) {
		t.Fatalf("icns: % x", icon[:20])
	}
	// listed (and kept next to hubs.json)
	res, _ = http.Get(srv.URL + "/app/installs")
	var list struct {
		Supported bool           `json:"supported"`
		Installs  []installedApp `json:"installs"`
	}
	_ = json.NewDecoder(res.Body).Decode(&list)
	res.Body.Close()
	if !list.Supported || len(list.Installs) != 1 || list.Installs[0].Page != "pg_a" {
		t.Fatalf("list: %+v", list)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "installs.json")); !strings.Contains(string(raw), "pg_a") {
		t.Fatalf("installs.json: %s", raw)
	}
	// not a PNG: refused
	bad, _ := json.Marshal(map[string]string{"name": "x", "icon": "data:image/png;base64,aGVsbG8="})
	if res, _ := http.Post(srv.URL+"/app/hubs/"+h.ID+"/pages/pg_b/install", "application/json", bytes.NewReader(bad)); res.StatusCode != 400 {
		t.Fatalf("bad icon: %d", res.StatusCode)
	}
	// removed: the launcher goes too
	req, _ := http.NewRequest("DELETE", srv.URL+"/app/hubs/"+h.ID+"/pages/pg_a/install", nil)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != 204 {
		t.Fatalf("uninstall: %v %v", res, err)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("launcher still there: %v", err)
	}
	// only a launcher of its own is removed
	removeLauncher(filepath.Join(home, "Applications", "myco 应用"), home)
	if _, err := os.Stat(home); err != nil {
		t.Fatal("removed something that is not its launcher")
	}
}

// Where the App cannot make launchers, it says so.
func TestInstallUnsupported(t *testing.T) {
	old := launcherKind
	launcherKind = ""
	defer func() { launcherKind = old }()
	b := &Backend{Local: &Local{Home: t.TempDir()}, OpenPageWindow: func(string, string, string) error { return nil }}
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	res, _ := http.Post(srv.URL+"/app/hubs/h/pages/p/install", "application/json", strings.NewReader(`{"name":"x"}`))
	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("unsupported: %d", res.StatusCode)
	}
}
