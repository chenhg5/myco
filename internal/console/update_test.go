package console

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func appZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o644)
		if filepath.Base(name) == "myco-app" || filepath.Base(name) == "myco" {
			h.SetMode(0o755)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(w, body)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The app checks the latest release, downloads its zip, verifies it
// against SHA256SUMS, swaps the bundle and asks the window to relaunch.
func TestAppUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the app updates itself on macOS only")
	}
	zipped := appZip(t, map[string]string{
		"myco.app/Contents/MacOS/myco-app": "new app",
		"myco.app/Contents/Info.plist":     "plist",
		"__MACOSX/myco.app/._Info.plist":   "junk",
	})
	sum := sha256.Sum256(zipped)
	rel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprint(w, `{"tag_name":"v9.1.0"}`)
		case "/dl/v9.1.0/SHA256SUMS":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), AppAsset("arm64"))
		case "/dl/v9.1.0/" + AppAsset("arm64"):
			w.Write(zipped)
		default:
			http.NotFound(w, r)
		}
	}))
	defer rel.Close()

	dir := t.TempDir()
	bundle := filepath.Join(dir, "myco.app")
	os.MkdirAll(filepath.Join(bundle, "Contents", "MacOS"), 0o755)
	os.WriteFile(filepath.Join(bundle, "Contents", "MacOS", "myco-app"), []byte("old app"), 0o755)

	u := &Updater{Version: "v9.0.0", Bundle: bundle, Arch: "arm64", API: rel.URL + "/latest", Download: rel.URL + "/dl"}
	app := httptest.NewServer((&Backend{Store: NewStore(filepath.Join(dir, "hubs.json")), Update: u}).Handler())
	defer app.Close()

	get := func() UpdateInfo {
		resp, err := http.Get(app.URL + "/app/update")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var info UpdateInfo
		json.NewDecoder(resp.Body).Decode(&info)
		return info
	}
	if info := get(); info.Latest != "v9.1.0" || !info.Available || info.CanInstall {
		t.Fatalf("before the window is up: %+v", info)
	}
	// Without the window (e.g. --serve) the app does not replace itself.
	if resp, _ := http.Post(app.URL+"/app/update", "application/json", nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("update without a window: HTTP %d", resp.StatusCode)
	}

	relaunched := make(chan struct{}, 1)
	u.SetRelaunch(func() { relaunched <- struct{}{} })
	if info := get(); !info.CanInstall {
		t.Fatalf("with the window: %+v", info)
	}
	resp, err := http.Post(app.URL+"/app/update", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %v %v", resp.StatusCode, err)
	}
	if b, _ := os.ReadFile(filepath.Join(bundle, "Contents", "MacOS", "myco-app")); string(b) != "new app" {
		t.Fatalf("bundle not replaced: %q", b)
	}
	if st, _ := os.Stat(filepath.Join(bundle, "Contents", "MacOS", "myco-app")); st.Mode()&0o111 == 0 {
		t.Fatal("the app binary lost its executable bit")
	}
	if _, err := os.Stat(filepath.Join(bundle, "__MACOSX")); err == nil {
		t.Fatal("macOS metadata was unpacked")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".myco-update-*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
	select {
	case <-relaunched:
	case <-time.After(3 * time.Second):
		t.Fatal("the app was not relaunched")
	}
}

func TestUnzipRefusesEscapes(t *testing.T) {
	for _, name := range []string{"../evil", "myco.app/../../evil"} {
		if err := unzipTo(t.TempDir(), appZip(t, map[string]string{name: "x"})); err == nil {
			t.Fatalf("%s was unpacked", name)
		}
	}
	// A bad download leaves the installed app alone.
	dir := t.TempDir()
	bundle := filepath.Join(dir, "myco.app")
	os.MkdirAll(bundle, 0o755)
	os.WriteFile(filepath.Join(bundle, "keep"), []byte("x"), 0o644)
	if err := installBundle(bundle, appZip(t, map[string]string{"other/file": "x"})); err == nil {
		t.Fatal("an incomplete app was installed")
	}
	if _, err := os.Stat(filepath.Join(bundle, "keep")); err != nil {
		t.Fatal("the installed app was touched")
	}
}

// On Windows the app replaces its own exe: the new one takes its path, the
// running one is set aside as .old (removed at the next start), and the
// window relaunches. A download that is not a program is refused.
func TestWindowsAppUpdate(t *testing.T) {
	newExe := append([]byte("MZ"), []byte("new app")...)
	serve := func(body []byte) *httptest.Server {
		sum := sha256.Sum256(body)
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/latest":
				fmt.Fprint(w, `{"tag_name":"v9.1.0"}`)
			case "/dl/v9.1.0/SHA256SUMS":
				fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), WindowsAppAsset("amd64"))
			case "/dl/v9.1.0/" + WindowsAppAsset("amd64"):
				w.Write(body)
			default:
				http.NotFound(w, r)
			}
		}))
	}
	run := func(body []byte) (string, *http.Response) {
		rel := serve(body)
		t.Cleanup(rel.Close)
		dir := t.TempDir()
		exe := filepath.Join(dir, "myco-app-windows-amd64.exe")
		os.WriteFile(exe, []byte("MZold app"), 0o755)
		u := &Updater{Version: "v9.0.0", Exe: exe, Arch: "amd64", API: rel.URL + "/latest", Download: rel.URL + "/dl"}
		relaunched := make(chan struct{}, 1)
		u.SetRelaunch(func() { relaunched <- struct{}{} })
		app := httptest.NewServer((&Backend{Store: NewStore(filepath.Join(dir, "hubs.json")), Update: u}).Handler())
		t.Cleanup(app.Close)
		resp, err := http.Post(app.URL+"/app/update", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode == 200 {
			select {
			case <-relaunched:
			case <-time.After(5 * time.Second):
				t.Fatal("no relaunch")
			}
		}
		return exe, resp
	}

	exe, resp := run(newExe)
	if resp.StatusCode != 200 {
		t.Fatalf("update: %d", resp.StatusCode)
	}
	if b, _ := os.ReadFile(exe); string(b) != string(newExe) {
		t.Fatalf("exe not replaced: %q", b)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "MZold app" {
		t.Fatalf("old exe not kept aside: %q", b)
	}
	CleanOldExe(exe)
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatal("the old exe must be removed at the next start")
	}

	exe, resp = run([]byte("<html>not a program</html>"))
	if resp.StatusCode == 200 {
		t.Fatal("a download that is not a program must be refused")
	}
	if b, _ := os.ReadFile(exe); string(b) != "MZold app" {
		t.Fatalf("a refused update must leave the app alone: %q", b)
	}
}
