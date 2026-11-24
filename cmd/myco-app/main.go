// Command myco-app is the myco console: the App (a window on macOS and
// Windows, the browser on Linux) that manages one or more myco hubs with
// per-device console tokens kept in the system keychain. With --serve it
// runs the same UI in a browser instead (any OS; used for development and
// tests).
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/console"
	"github.com/myco-ai/myco/internal/console/notices"
	"github.com/myco-ai/myco/internal/release"
	"github.com/myco-ai/myco/internal/service"
	"github.com/myco-ai/myco/internal/web"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	serve := flag.String("serve", "", "serve the console in a browser on this loopback address (e.g. 127.0.0.1:0) instead of opening the app window")
	showVersion := flag.Bool("version", false, "print the version")
	quit := flag.Bool("quit", false, "stop the running app (Linux, where it runs in the browser)")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *quit {
		if err := quitRunning(); err != nil {
			fmt.Fprintln(os.Stderr, "myco-app:", err)
			os.Exit(1)
		}
		return
	}
	storePath, err := console.DefaultStorePath()
	if err != nil {
		log.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	local := &console.Local{Home: home, Device: deviceName(), Bundled: bundledCLI(), ShellPath: console.LoginShellPath, ShellEnv: console.LoginShellEnv,
		// No launchd/systemd --user (Windows; Linux in WSL, a container,
		// a su shell): the app runs the local hub itself.
		NoServiceManager: !serviceManager(home)}
	keys := console.Keychain{}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		// No Secret Service on this computer: the tokens go to a 0600 file.
		keys.File = filepath.Join(filepath.Dir(storePath), "console-secrets.json")
	}
	b := &console.Backend{Store: console.NewStore(storePath), Secrets: keys, Version: version, Local: local,
		Update: &console.Updater{Version: version, Bundle: appBundle(), Exe: appExe(), API: release.APIURL(os.Getenv), Downloads: release.DownloadURLs(os.Getenv)}}
	// Pages open with what they showed last time (MYCO_APP_CACHE=off: no).
	if os.Getenv("MYCO_APP_CACHE") != "off" {
		if c, err := openCache(version, keys); err != nil {
			log.Printf("app cache off: %v", err)
		} else {
			b.Cache = c
		}
	}
	// What an update set aside (Windows, Linux) is no longer running.
	console.CleanOldExe(b.Update.Exe)
	if p, err := local.EnsureCLI(); err != nil {
		log.Printf("could not install the bundled myco CLI: %v", err)
	} else if p != "" {
		log.Printf("installed the myco CLI at %s", p)
	}
	if *serve != "" {
		b.Shell = "browser"
		if err := runServe(*serve, handler(b)); err != nil {
			log.Fatal(err)
		}
		return
	}
	b.Shell = guiShell
	b.Listen, b.Dictate = listen, dictate
	if voiceAudio != nil {
		b.Voice = voiceAudio
	}
	if runtime.GOOS == "darwin" {
		b.PickImage = func() (string, error) {
			if pickImage == nil { // not yet: the app is starting
				return "", nil
			}
			return pickImage()
		}
	}
	// A link in a chat opens in the default browser (set by runGUI).
	b.OpenURL = func(u string) error {
		if openURL == nil { // not yet: the app is starting
			return fmt.Errorf("not ready")
		}
		return openURL(u)
	}
	// 专注模式: the window in full screen (set by runGUI where it can).
	b.Fullscreen = func(on bool) error {
		if setFullscreen == nil {
			return fmt.Errorf("not ready")
		}
		setFullscreen(on)
		return nil
	}
	// The pages' language changed: the tray menu (when there is one) follows.
	b.OnUILang = func(l string) {
		if f := uiLangHook; f != nil {
			f(l)
		}
	}
	// Notices: what needs the person, shown while the App runs, window or
	// not (MYCO_APP_NOTICES=off: none).
	if os.Getenv("MYCO_APP_NOTICES") != "off" {
		startNotices = func(p notices.Presenter) { b.StartNotices(p) }
	}
	// An update that started this program waits for it to report in
	// (Windows): it got this far.
	console.ConfirmStarted(b.Update.Exe)
	if err := runGUI(handler(b), b.SetPendingLink, b.Update.SetRelaunch); err != nil {
		fmt.Fprintln(os.Stderr, "myco-app:", err)
		os.Exit(1)
	}
}

// serviceManager says whether myco can install services for this user
// here: launchd on a Mac, systemd --user on Linux when it answers.
func serviceManager(home string) bool {
	switch runtime.GOOS {
	case "darwin":
		return true
	case "windows":
		return false
	}
	m, err := service.Detect(home, service.ExecRunner{})
	return err == nil && service.Reachable(m) == nil
}

// pickImage opens the system's panel for a picture (set by runGUI where
// the web view's own panel cannot keep to pictures).
var pickImage func() (string, error)

// openURL opens a link in the default browser (set by runGUI).
var openURL func(string) error

// setFullscreen puts the window in full screen or takes it out (set by
// runGUI where the window can).
var setFullscreen func(on bool)

// uiLangHook, when the tray menu is up, redraws it in the pages' language.
var uiLangHook func(lang string)

// startNotices follows the workspaces' notices and shows them with the
// system's notifications (set in main; called by runGUI once the app runs).
var startNotices func(p notices.Presenter)

// bundledCLI is the myco CLI inside the app bundle
// (myco.app/Contents/Resources/myco), or "" when not running from one.
func bundledCLI() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	p := filepath.Join(filepath.Dir(exe), "..", "Resources", "myco")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return filepath.Clean(p)
}

// appBundle is the myco.app the app runs from (…/myco.app/Contents/MacOS/
// myco-app), or "" when it does not run from a bundle.
func appBundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	b := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", ".."))
	if !strings.HasSuffix(b, ".app") {
		return ""
	}
	return b
}

// appExe is the running app's program on Windows and Linux ("" on a Mac,
// where the update replaces the bundle), which an update replaces in place.
func appExe() string {
	if runtime.GOOS == "darwin" {
		return ""
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe
}

// deviceName names this computer in console tokens: its host name,
// lower-cased, without ".local", e.g. "glenns-macbook-pro".
func deviceName() string {
	h, err := os.Hostname()
	// On a Mac the host name can be whatever the network handed out (an IP
	// address, "192.168.1.210"): the name set in System Settings is better.
	if runtime.GOOS == "darwin" {
		if out, err2 := exec.Command("scutil", "--get", "LocalHostName").Output(); err2 == nil && strings.TrimSpace(string(out)) != "" {
			h, err = strings.TrimSpace(string(out)), nil
		}
	}
	if err != nil || h == "" {
		return "this-computer"
	}
	h = strings.ToLower(strings.TrimSuffix(h, ".local"))
	var b strings.Builder
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// handler serves the console API under /app/ and the web UI everywhere else.
func handler(b *console.Backend) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/app/", b.Handler())
	mux.Handle("/", web.Handler())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(w, r)
	})
}

const serveCookie = "myco_console"

// runServe serves h on a loopback address, guarded like `myco ui`: only our
// own Host (no DNS rebinding) and a one-time key exchanged for a cookie.
func runServe(addr string, h http.Handler) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok || !tcp.IP.IsLoopback() {
		ln.Close()
		return fmt.Errorf("--serve must be a loopback address such as 127.0.0.1:0, got %s", addr)
	}
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	k := hex.EncodeToString(key)
	fmt.Printf("myco console: http://127.0.0.1:%d/?key=%s\n", tcp.Port, k)
	return http.Serve(ln, guarded(h, tcp.Port, k))
}

// guarded wraps h for --serve: our own Host only, and the one-time key.
func guarded(h http.Handler, port int, k string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != fmt.Sprintf("127.0.0.1:%d", port) && r.Host != fmt.Sprintf("localhost:%d", port) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		// A page origin shares this loopback hostname with the browser
		// console. Reject requests from a different origin even if the
		// browser attaches the console's HttpOnly cookie.
		if strings.HasPrefix(r.URL.Path, "/app/") {
			site := r.Header.Get("Sec-Fetch-Site")
			origin := r.Header.Get("Origin")
			if (site != "" && site != "same-origin" && site != "none") || (origin != "" && origin != "http://"+r.Host) {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		if q := r.URL.Query().Get("key"); q != "" {
			if subtle.ConstantTimeCompare([]byte(q), []byte(k)) != 1 {
				http.Error(w, "invalid key", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: serveCookie, Value: k, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		if c, err := r.Cookie(serveCookie); err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(k)) != 1 {
			http.Error(w, "open the console with the URL printed by myco-app --serve (it contains a one-time key)", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'; frame-src http://127.0.0.1:* http://localhost:*")
		h.ServeHTTP(w, r)
	})
}

// openCache opens the page cache for this version of the app. The keychain
// may ask the person first (a locked keychain): the app does not wait for
// that, it starts without the cache this time.
func openCache(version string, keys console.Keychain) (*console.Cache, error) {
	type result struct {
		c   *console.Cache
		err error
	}
	done := make(chan result, 1)
	go func() {
		path, err := console.DefaultCachePath()
		if err != nil {
			done <- result{nil, err}
			return
		}
		key, err := keys.CacheKey()
		if err != nil {
			done <- result{nil, err}
			return
		}
		c, err := console.OpenCache(path, key)
		if err == nil {
			if err = c.ForVersion(version); err != nil {
				c.Close()
				c = nil
			}
		}
		done <- result{c, err}
	}()
	select {
	case r := <-done:
		return r.c, r.err
	case <-time.After(2 * time.Second):
		go func() { // answered late: not used this time
			if r := <-done; r.c != nil {
				r.c.Close()
			}
		}()
		return nil, errors.New("the keychain did not answer in time")
	}
}
