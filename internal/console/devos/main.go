// Command devos serves the App backend's /app/os (what pages use of this
// computer, myco.os) on its own, for the mock App to pass it to:
//
//	go run ./internal/console/devos -addr 127.0.0.1:7790 &
//	cd web && MOCK_OS_URL=http://127.0.0.1:7790 npm run dev:app
//
// Then open the page 这台电脑. It uses this computer for real: your files.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/myco-ai/myco/internal/console"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7790", "loopback address to listen on")
	flag.Parse()
	if !strings.HasPrefix(*addr, "127.0.0.1:") && !strings.HasPrefix(*addr, "localhost:") {
		log.Fatal("devos: listen on 127.0.0.1 only: it runs commands for whoever asks")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	b := &console.Backend{Local: &console.Local{Home: home, ShellPath: console.LoginShellPath}}
	h := b.Handler()
	log.Printf("devos: /app/os on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/app/os") {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})))
}
