// Command myco-app is the myco console: a menu-bar app (macOS) that manages
// one or more myco hubs with per-device console tokens kept in the system
// keychain. With --serve it runs the same UI in a browser instead (any OS;
// used for development and tests).
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
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/console"
	"github.com/chenhg5/myco/internal/release"
	"github.com/chenhg5/myco/internal/web"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	serve := flag.String("serve", "", "serve the console in a browser on this loopback address (e.g. 127.0.0.1:0) instead of opening the app window")
	showVersion := flag.Bool("version", false, "print the version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
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
	local := &console.Local{Home: home, Device: deviceName(), Bundled: bundledCLI(),
		// No launchd/systemd: the app runs the local hub itself.
		NoServiceManager: runtime.GOOS == "windows"}
	b := &console.Backend{Store: console.NewStore(storePath), Secrets: console.Keychain{}, Version: version, Local: local,
		Update: &console.Updater{Version: version, Bundle: appBundle(), Exe: windowsExe(), API: release.APIURL(os.Getenv), Download: release.DownloadURL(os.Getenv)}}
	// Pages open with what they showed last time (MYCO_APP_CACHE=off: no).
	if os.Getenv("MYCO_APP_CACHE") != "off" {
		if c, err := openCache(version); err != nil {
			log.Printf("app cache off: %v", err)
		} else {
			b.Cache = c
		}
	}
	// What a Windows update set aside is no longer running.
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
	b.Dictate = dictate
	if err := runGUI(handler(b), b.SetPendingLink, b.Update.SetRelaunch); err != nil {
		fmt.Fprintln(os.Stderr, "myco-app:", err)
		os.Exit(1)
	}
}

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

// windowsExe is the running app's exe on Windows ("" elsewhere), which an
// update replaces in place.
func windowsExe() string {
	if runtime.GOOS != "windows" {
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'")
		h.ServeHTTP(w, r)
	})
}

// openCache opens the page cache for this version of the app. The keychain
// may ask the person first (a locked keychain): the app does not wait for
// that, it starts without the cache this time.
func openCache(version string) (*console.Cache, error) {
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
		key, err := console.KeychainCacheKey()
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
