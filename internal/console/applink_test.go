package console_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/console"
)

func TestParseAppLink(t *testing.T) {
	for in, want := range map[string][2]string{
		"https://hub.example.com/c/myco_console_abc":                               {"https://hub.example.com", "myco_console_abc"},
		"  https://1-2-3-4.sslip.io/c/myco_console_abc/  ":                         {"https://1-2-3-4.sslip.io", "myco_console_abc"},
		"http://127.0.0.1:7777/c/myco_console_x":                                   {"http://127.0.0.1:7777", "myco_console_x"},
		"https://example.com/myco/c/myco_console_x":                                {"https://example.com/myco", "myco_console_x"},
		"myco://add-hub?link=https%3A%2F%2Fhub.example.com%2Fc%2Fmyco_console_abc": {"https://hub.example.com", "myco_console_abc"},
	} {
		u, tok, err := console.ParseAppLink(in)
		if err != nil || u != want[0] || tok != want[1] {
			t.Errorf("ParseAppLink(%q) = %q %q %v, want %v", in, u, tok, err, want)
		}
	}
	for _, bad := range []string{"", "myco_console_abc", "https://hub.example.com/j/myco_enroll_x",
		"https://hub.example.com/c/myco_abc", "ftp://h/c/myco_console_x", "https://u:p@h/c/myco_console_x"} {
		if _, _, err := console.ParseAppLink(bad); err == nil {
			t.Errorf("ParseAppLink(%q) should fail", bad)
		}
	}
}

func TestAddHubByLinkAndPendingLink(t *testing.T) {
	r := newRig(t)
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets}
	r.app.Config.Handler = b.Handler()

	tok := r.consoleToken("by-link")
	if code, m, _ := r.do("POST", "/app/hubs", map[string]string{"link": "https://nope.example.com/j/x"}); code != 400 || !strings.Contains(m["message"].(string), "App 连接链接") {
		t.Fatalf("bad link: %d %v", code, m)
	}
	code, m, _ := r.do("POST", "/app/hubs", map[string]string{"link": r.hubURL + "/c/" + tok})
	if code != 201 || m["url"] != r.hubURL {
		t.Fatalf("add by link: %d %v", code, m)
	}
	id := m["id"].(string)
	// The same hub again with a new link: one entry, the new token.
	tok2 := r.consoleToken("by-link-2")
	if code, m, _ := r.do("POST", "/app/hubs", map[string]string{"link": r.hubURL + "/c/" + tok2}); code != 200 || m["id"] != id {
		t.Fatalf("re-adding: %d %v", code, m)
	}
	if hubs, _ := console.NewStore(r.store).List(); len(hubs) != 1 || r.secrets.m[id] != tok2 {
		t.Fatalf("hubs %+v, token updated: %v", hubs, r.secrets.m[id] == tok2)
	}

	if code, m, _ := r.do("GET", "/app/pending-link", nil); code != 200 || m["link"] != "" {
		t.Fatalf("no pending link: %v", m)
	}
	b.SetPendingLink("myco://add-hub?link=https%3A%2F%2Fh.example.com%2Fc%2Fmyco_console_z")
	if _, m, _ := r.do("GET", "/app/pending-link", nil); m["link"] != "https://h.example.com/c/myco_console_z" {
		t.Fatalf("pending link: %v", m)
	}
	if _, m, _ := r.do("GET", "/app/pending-link", nil); m["link"] != "" {
		t.Fatalf("a pending link is handed out once: %v", m)
	}
}

func TestEnsureCLIInstallsTheBundledOne(t *testing.T) {
	home := t.TempDir()
	bundled := filepath.Join(t.TempDir(), "myco")
	os.WriteFile(bundled, []byte("#!/bin/sh\necho bundled\n"), 0o755)
	l := &console.Local{Home: home, Bundled: bundled, Dirs: []string{filepath.Join(home, ".local", "bin")},
		LookPath: func(string) (string, error) { return "", os.ErrNotExist }}
	p, err := l.EnsureCLI()
	if err != nil || p == "" {
		t.Fatalf("EnsureCLI: %q %v", p, err)
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "bundled") {
		t.Fatal("installed file is not the bundled CLI")
	}
	if p2, err := l.EnsureCLI(); err != nil || p2 != "" {
		t.Fatalf("an installed CLI must be left alone: %q %v", p2, err)
	}
}

// A one-time link: the app trades its code for this device's own token and
// stores that; the link then stops working.
func TestAddHubByPairingLink(t *testing.T) {
	r := newRig(t)
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Local: &console.Local{Home: t.TempDir(), Device: "glenn-pc"}}
	r.app.Config.Handler = b.Handler()
	pl, err := r.admin.CreatePairing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	code, m, _ := r.do("POST", "/app/hubs", map[string]string{"link": pl.Link})
	if code != 201 {
		t.Fatalf("add by pairing link: %d %v", code, m)
	}
	id := m["id"].(string)
	if tok := r.secrets.m[id]; !strings.HasPrefix(tok, "myco_console_") {
		t.Fatalf("stored %q, want the device's own console token", tok)
	}
	if code, _, _ := r.do("POST", "/app/hubs", map[string]string{"link": pl.Link}); code == 200 || code == 201 {
		t.Fatal("a used link must not work again")
	}
	devs, err := r.admin.Devices(context.Background())
	if err != nil || len(devs) == 0 || devs[len(devs)-1].Name != "glenn-pc" {
		t.Fatalf("devices: %v %+v", err, devs)
	}
}
