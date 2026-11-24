package console

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/client"
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
// fakeApp stands in for the new version an update starts on Windows.
type fakeApp struct {
	exited chan error
	killed bool
}

func (f *fakeApp) Wait() error { return <-f.exited }
func (f *fakeApp) Kill() error {
	f.killed = true
	select {
	case f.exited <- errors.New("killed"):
	default:
	}
	return nil
}

func TestWindowsAppUpdate(t *testing.T) {
	defer func(w time.Duration) { handOverWait = w }(handOverWait)
	handOverWait = 300 * time.Millisecond
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
	// How the started new version behaves.
	const (
		reportsIn = iota
		quits
		hangs
		cannotStart
	)
	var started []string
	var last *fakeApp
	run := func(body []byte, behaves int) (string, *http.Response, map[string]any) {
		rel := serve(body)
		t.Cleanup(rel.Close)
		dir := t.TempDir()
		exe := filepath.Join(dir, "myco-app-windows-amd64.exe")
		os.WriteFile(exe, []byte("MZold app"), 0o755)
		u := &Updater{Version: "v9.0.0", Exe: exe, Arch: "amd64", OS: "windows", API: rel.URL + "/latest", Download: rel.URL + "/dl", HTTP: rel.Client()}
		u.launch = func(path string) (waiter, error) {
			started = append(started, path)
			if behaves == cannotStart {
				return nil, errors.New("access is denied")
			}
			if b, _ := os.ReadFile(path); string(b) != string(newExe) {
				t.Errorf("started %q, not the new version", b)
			}
			last = &fakeApp{exited: make(chan error, 1)}
			switch behaves {
			case reportsIn:
				go func() { time.Sleep(50 * time.Millisecond); ConfirmStarted(path) }()
			case quits:
				last.exited <- errors.New("exit status 1")
			}
			return last, nil
		}
		relaunched := make(chan struct{}, 1)
		u.SetRelaunch(func() { relaunched <- struct{}{} })
		app := httptest.NewServer((&Backend{Store: NewStore(filepath.Join(dir, "hubs.json")), Update: u}).Handler())
		t.Cleanup(app.Close)
		resp, err := http.Post(app.URL+"/app/update", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			select {
			case <-relaunched:
			case <-time.After(5 * time.Second):
				t.Fatal("no relaunch")
			}
		}
		if _, err := os.Stat(startedMarker(exe)); !os.IsNotExist(err) {
			t.Fatalf("the start marker was left behind (%v)", err)
		}
		return exe, resp, out
	}

	exe, resp, _ := run(newExe, reportsIn)
	if resp.StatusCode != 200 || len(started) != 1 || started[0] != exe {
		t.Fatalf("update: %d, started %v", resp.StatusCode, started)
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

	// A new version that cannot start, quits at once or never reports in
	// (an antivirus, a crash): the current one is put back and keeps running.
	for _, c := range []struct {
		name    string
		behaves int
		killed  bool
	}{{"cannot start", cannotStart, false}, {"quits", quits, false}, {"hangs", hangs, true}} {
		started, last = nil, nil
		exe, resp, out := run(newExe, c.behaves)
		if resp.StatusCode != http.StatusBadGateway || out["error"] != "start_failed" || out["suggestion"] == nil {
			t.Fatalf("%s: %d %v", c.name, resp.StatusCode, out)
		}
		if b, _ := os.ReadFile(exe); string(b) != "MZold app" {
			t.Fatalf("%s: the current version was not put back: %q", c.name, b)
		}
		if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
			t.Fatalf("%s: .old left behind", c.name)
		}
		if left, _ := filepath.Glob(exe + ".*"); len(left) != 0 {
			t.Fatalf("%s: left behind %v", c.name, left)
		}
		if c.killed && (last == nil || !last.killed) {
			t.Fatalf("%s: the new version was not stopped", c.name)
		}
	}

	started = nil
	exe, resp, _ = run([]byte("<html>not a program</html>"), reportsIn)
	if resp.StatusCode == 200 || len(started) != 0 {
		t.Fatal("a download that is not a program must be refused")
	}
	if b, _ := os.ReadFile(exe); string(b) != "MZold app" {
		t.Fatalf("a refused update must leave the app alone: %q", b)
	}
}

// A failed download says what to do about it.
func TestUpdateDownloadFailureSaysWhy(t *testing.T) {
	rel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			fmt.Fprint(w, `{"tag_name":"v9.1.0"}`)
			return
		}
		http.NotFound(w, r) // the asset cannot be had
	}))
	defer rel.Close()
	dir := t.TempDir()
	exe := filepath.Join(dir, "myco-app.exe")
	os.WriteFile(exe, []byte("MZold app"), 0o755)
	u := &Updater{Version: "v9.0.0", Exe: exe, Arch: "amd64", OS: "windows", API: rel.URL + "/latest", Download: rel.URL + "/dl", HTTP: rel.Client()}
	u.SetRelaunch(func() {})
	app := httptest.NewServer((&Backend{Store: NewStore(filepath.Join(dir, "hubs.json")), Update: u}).Handler())
	defer app.Close()
	resp, err := http.Post(app.URL+"/app/update", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway || out["error"] != "download_failed" || !strings.Contains(fmt.Sprint(out["suggestion"]), "proxy") {
		t.Fatalf("%d %v", resp.StatusCode, out)
	}
	if b, _ := os.ReadFile(exe); string(b) != "MZold app" {
		t.Fatalf("app touched: %q", b)
	}
}

// Through a proxy, the update also tries GitHub directly, so a proxy that
// is set but broken does not block it.
func TestUpdateTriesDirectAfterProxy(t *testing.T) {
	defer client.SetProxy(nil)
	u := &Updater{}
	client.SetProxy(func(*http.Request) (*url.URL, error) { return nil, nil })
	if cs := u.clients(time.Second); len(cs) != 1 {
		t.Fatalf("no proxy: %d ways", len(cs))
	}
	px, _ := url.Parse("http://127.0.0.1:7890")
	client.SetProxy(func(*http.Request) (*url.URL, error) { return px, nil })
	cs := u.clients(time.Second)
	if len(cs) != 2 {
		t.Fatalf("with a proxy: %d ways", len(cs))
	}
	tr, ok := cs[1].Transport.(*http.Transport)
	if !ok || tr.Proxy != nil {
		t.Fatalf("the second way must be direct: %#v", cs[1].Transport)
	}
}

func TestInstallExeRetriesHeldFiles(t *testing.T) {
	defer func(n int, w time.Duration) { renameTries, renameWait = n, w }(renameTries, renameWait)
	renameTries, renameWait = 50, 10*time.Millisecond
	dir := t.TempDir()
	// A rename that fails for a moment (as while an antivirus holds the
	// file; here a directory in the way, removed shortly) is tried again;
	// a missing source is not.
	from, to := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(from, []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(to, "in-the-way"), 0o755)
	go func() { time.Sleep(100 * time.Millisecond); os.RemoveAll(to) }()
	if err := renameRetry(from, to); err != nil {
		t.Fatalf("renameRetry: %v", err)
	}
	if err := renameRetry(filepath.Join(dir, "missing"), to); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source: %v", err)
	}
}

// On Linux the app replaces its own program the same way, from the Linux
// asset, and refuses a download that is not a Linux program.
func TestLinuxAppUpdate(t *testing.T) {
	newExe := append([]byte("\x7fELF"), []byte("new app")...)
	for _, c := range []struct {
		body []byte
		ok   bool
	}{{newExe, true}, {[]byte("MZ windows"), false}} {
		sum := sha256.Sum256(c.body)
		rel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/latest":
				fmt.Fprint(w, `{"tag_name":"v9.1.0"}`)
			case "/dl/v9.1.0/SHA256SUMS":
				fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), LinuxAppAsset("arm64"))
			case "/dl/v9.1.0/" + LinuxAppAsset("arm64"):
				w.Write(c.body)
			default:
				http.NotFound(w, r)
			}
		}))
		dir := t.TempDir()
		exe := filepath.Join(dir, "myco-app")
		os.WriteFile(exe, []byte("\x7fELFold app"), 0o755)
		u := &Updater{Version: "v9.0.0", Exe: exe, Arch: "arm64", OS: "linux", API: rel.URL + "/latest", Download: rel.URL + "/dl"}
		u.SetRelaunch(func() {})
		app := httptest.NewServer((&Backend{Store: NewStore(filepath.Join(dir, "hubs.json")), Update: u}).Handler())
		resp, err := http.Post(app.URL+"/app/update", "application/json", nil)
		app.Close()
		rel.Close()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(exe)
		if c.ok && (resp.StatusCode != 200 || string(b) != string(newExe)) {
			t.Fatalf("update: %d, exe %q", resp.StatusCode, b)
		}
		if !c.ok && (resp.StatusCode == 200 || string(b) != "\x7fELFold app") {
			t.Fatalf("a Windows program must be refused on Linux: %d, exe %q", resp.StatusCode, b)
		}
	}
}
