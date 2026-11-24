package cli

import (
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/testutil"
)

func TestAppLinkURLAndLoopback(t *testing.T) {
	link := "http://192.168.1.20:7777/c/myco_pair_abc?x=1&y=2"
	u, err := url.Parse(appLinkURL(link))
	if err != nil || u.Scheme != "myco" || u.Host != "add-hub" || u.Query().Get("link") != link {
		t.Fatalf("myco:// address: %v %v", u, err)
	}
	for in, want := range map[string]bool{
		"http://127.0.0.1:7777/j/x": true, "http://localhost:7777/j/x": true, "http://[::1]:7777/j/x": true,
		"http://192.168.1.20:7777/j/x": false, "https://hub.example.com/j/x": false,
	} {
		if loopbackLink(in) != want {
			t.Fatalf("loopbackLink(%s) != %v", in, want)
		}
	}
}

func TestConsoleLinkOpenAndLoopbackWarning(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	hub := newHub(t) // 127.0.0.1: the links it makes name this computer only
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(t.TempDir(), "a.yaml")}}

	var opened []string
	defer func(f func(string) error) { openAppLink = f }(openAppLink)
	openAppLink = func(link string) error { opened = append(opened, link); return nil }

	code, out, stderr := admin.run("console", "link", "--open", "--format", "json")
	if code != 0 {
		t.Fatalf("console link --open: %s", stderr)
	}
	var res struct {
		Link     string
		Opened   bool
		Warnings []string
	}
	json.Unmarshal([]byte(out), &res)
	if !res.Opened || len(opened) != 1 || opened[0] != res.Link || !strings.Contains(res.Link, "/c/") {
		t.Fatalf("opened %v for %+v", opened, res)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "MYCO_LAN_ADDR") || !strings.Contains(stderr, "warning:") {
		t.Fatalf("loopback warning: %+v / %s", res.Warnings, stderr)
	}

	// Not opened: said, and the link is still given to paste.
	openAppLink = func(string) error { return errors.New("no app") }
	code, out, _ = admin.run("console", "link", "--open", "--format", "text")
	if code != 0 || !strings.Contains(out, "Not opened: no app") || !strings.Contains(out, "/c/") {
		t.Fatalf("not opened: %d %s", code, out)
	}

	// --dry-run makes no link and opens nothing.
	opened = nil
	if code, out, _ := admin.run("console", "link", "--open", "--dry-run", "--format", "json"); code != 0 || !strings.Contains(out, `"dry_run": true`) || len(opened) != 0 {
		t.Fatalf("dry run: %d %s %v", code, out, opened)
	}

	// Join links say it too.
	if _, out, _ := admin.run("link", "create", "--format", "json"); !strings.Contains(out, "MYCO_LAN_ADDR") {
		t.Fatalf("link create warning: %s", out)
	}
}
