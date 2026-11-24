package console

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/client"
)

// The optional Chromium test uses the actual local listeners and recovery
// documents. Playwright is installed separately, as for the web smoke tests.
func TestPageOriginsInBrowser(t *testing.T) {
	if os.Getenv("MYCO_PAGE_ORIGIN_BROWSER") != "1" {
		t.Skip("enable the optional Playwright browser test")
	}
	dir := t.TempDir()
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<!doctype html><title>Old page origin</title>")
	}))
	t.Cleanup(old.Close)
	_, port, _ := strings.Cut(old.URL, "127.0.0.1:")
	if err := os.WriteFile(filepath.Join(dir, "page-port"), []byte(port), 0o600); err != nil {
		t.Fatal(err)
	}
	s := loadPageOrigins(dir)
	t.Cleanup(func() {
		closeTestPageOrigins(s)
		if s.recovery != nil {
			_ = s.recovery.Close()
		}
	})
	vault := os.Getenv("MYCO_TEST_VAULT_DIST")
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vault != "" && strings.HasPrefix(r.URL.Path, "/v1/pages/vault/v/1/") {
			http.StripPrefix("/v1/pages/vault/v/1/", http.FileServer(http.Dir(vault))).ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<!doctype html><title>Isolated page</title><main>Isolated page</main>")
	}))
	t.Cleanup(hub.Close)
	c, err := client.New(hub.URL, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	b := &Backend{Secrets: mapSecrets{"one": "fixture", "two": "fixture"}}
	key := func(k [3]string) pageKey {
		return pageKey{hub: k[0], page: k[2], entry: "index.html", version: 1, client: c, credential: sha256.Sum256([]byte("fixture"))}
	}
	targets := [][3]string{{"one", "owner", "page"}, {"one", "owner", "other"}, {"two", "owner", "page"}, {"one", "member", "page"}, {"one", "owner", "vault"}}
	origins := make([]string, 0, len(targets))
	for _, k := range targets {
		address, _, err := s.open(b, pageScope(k[0], k[1], k[2]), key(k))
		if err != nil {
			t.Fatal(err)
		}
		origins = append(origins, address)
	}
	var afterRestart *pageServer
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/restart":
			first := targets[0]
			s.active[pageScope(first[0], first[1], first[2])].close()
			afterRestart = loadPageOrigins(dir)
			k := targets[0]
			address, _, err := afterRestart.open(b, pageScope(k[0], k[1], k[2]), key(k))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			io.WriteString(w, address)
		case "/recover":
			old.Close()
			address, err := s.openRecovery()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			io.WriteString(w, address)
		case "/dom":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<!doctype html><iframe id="one" src="%s"></iframe><iframe id="two" src="%s"></iframe>`, origins[0], origins[1])
		case "/vault":
			// Equivalent to the panel's ready capability, using the actual
			// injected bridge and matching both the iframe and its origin.
			isolated := strconv.FormatBool(r.URL.Query().Get("old") == "")
			lang := "en"
			if r.URL.Query().Get("lang") == "zh" {
				lang = "zh"
			}
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<!doctype html><iframe id="vault" sandbox="allow-scripts allow-same-origin allow-forms" style="width:100%;height:620px;border:0"></iframe><script>
const frame = document.querySelector("#vault");
frame.src = `+strconv.Quote(origins[4]+"?myco_lang="+lang)+`;
addEventListener("message", event => {
  if (event.source !== frame.contentWindow || event.origin !== new URL(frame.src).origin || event.data.method !== "ready") return;
  frame.contentWindow.postMessage({myco:"result", id:event.data.id, value:{isolation:`+isolated+`,page:{id:"vault"},me:{name:"Fixture"}}},event.origin);
});
</script>`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		control.Close()
		if afterRestart != nil {
			closeTestPageOrigins(afterRestart)
		}
	})
	input, err := json.Marshal(map[string]any{"origins": origins, "old": old.URL, "control": control.URL, "vault": vault != ""})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "../../web/scripts/page-origin.browser.cjs", string(input))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}
