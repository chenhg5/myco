// Command myco-app is the myco console: a menu-bar app (macOS) that manages
// one or more myco hubs with per-device console tokens kept in the system
// keychain. With --serve it runs the same UI in a browser instead (any OS;
// used for development and tests).
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/chenhg5/myco/internal/console"
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
	b := &console.Backend{Store: console.NewStore(storePath), Secrets: console.Keychain{}}
	h := handler(b)
	if *serve != "" {
		if err := runServe(*serve, h); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := runGUI(h); err != nil {
		fmt.Fprintln(os.Stderr, "myco-app:", err)
		os.Exit(1)
	}
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
