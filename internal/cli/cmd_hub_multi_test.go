package cli

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/testutil"
)

func TestNamedHubPlanListAndPort(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	base := filepath.Join(home, ".myco")
	h := &harnessT{t, map[string]string{}}
	type hubPlan struct {
		Dir, Service string
		Command      []string
	}
	// The plan is JSON: compare fields, not text (Windows paths are escaped).
	code, out, e := h.run("hub", "up", "--tunnel", "none", "--dry-run")
	var def hubPlan
	if code != ExitDryRun || json.Unmarshal([]byte(out), &def) != nil || def.Dir != filepath.Join(base, "hub") ||
		def.Service != "ai.myco.hub" || !strings.Contains(strings.Join(def.Command, " "), ":7777") {
		t.Fatalf("default plan changed: %d %s %s", code, out, e)
	}
	wantPort, err := firstHubPort(base)
	if err != nil {
		t.Fatal(err)
	}
	code, out, e = h.run("hub", "up", "--name", "work", "--tunnel", "none", "--dry-run")
	if code != ExitDryRun {
		t.Fatalf("named plan: %d %s", code, e)
	}
	var plan hubPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Dir != filepath.Join(base, "hubs", "work") || plan.Service != "ai.myco.hub.work" || !strings.Contains(strings.Join(plan.Command, " "), ":"+strconv.Itoa(wantPort)) {
		t.Fatalf("named plan: %s", out)
	}
	code, out, e = h.run("hub", "up", "--name", "work", "--tunnel", "cloudflare", "--hostname", "work.example.test", "--dry-run")
	if code != ExitDryRun || !strings.Contains(out, "myco-work") {
		t.Fatalf("named tunnel plan: %d %s %s", code, out, e)
	}
	if code, _, e := h.run("hub", "up", "--name", "Bad!", "--tunnel", "none", "--dry-run"); code != ExitUsage || !strings.Contains(e, "invalid_argument") {
		t.Fatalf("invalid name: %d %s", code, e)
	}
	if code, _, e := h.run("hub", "up", "--name", "", "--tunnel", "none", "--dry-run"); code != ExitUsage {
		t.Fatalf("empty explicit name: %d %s", code, e)
	}
	if _, err := writeHubEnv(filepath.Join(base, "hub"), "http://127.0.0.1:7777"); err != nil {
		t.Fatal(err)
	}
	if _, err := writeHubEnv(filepath.Join(base, "hubs", "work"), "http://127.0.0.1:7778"); err != nil {
		t.Fatal(err)
	}
	if err := writeHubPublic(filepath.Join(base, "hubs", "work"), "https://work.example.test"); err != nil {
		t.Fatal(err)
	}
	code, out, e = h.run("hub", "list")
	if code != 0 {
		t.Fatalf("list: %d %s", code, e)
	}
	var list struct{ Hubs []localHubItem }
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Hubs) != 2 || list.Hubs[0].Name != "" || list.Hubs[1].Name != "work" || list.Hubs[1].PublicURL != "https://work.example.test" {
		t.Fatalf("hub list: %s", out)
	}
	ids := localHubServiceIDs(base)
	if len(ids) != 3 || ids[2].Unit != "myco-hub-work.service" {
		t.Fatalf("upgrade services: %+v", ids)
	}
	code, out, e = h.run("hub", "up", "--name", "work", "--tunnel", "none", "--dry-run")
	if code != ExitDryRun || !strings.Contains(out, ":7778") {
		t.Fatalf("reconfigure must keep port: %d %s %s", code, out, e)
	}
	if runtime.GOOS != "windows" {
		code, out, e = h.run("hub", "down", "--name", "work", "--dry-run")
		if code != ExitDryRun || !strings.Contains(out, "myco-hub-work.service") {
			t.Fatalf("named down plan: %d %s %s", code, out, e)
		}
	}
}

func TestFirstHubPortSkipsBusyAndReserved(t *testing.T) {
	base := t.TempDir()
	first, err := firstHubPort(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeHubEnv(filepath.Join(base, "hubs", "old"), "http://127.0.0.1:"+strconv.Itoa(first)); err != nil {
		t.Fatal(err)
	}
	next, err := firstHubPort(base)
	if err != nil || next <= first {
		t.Fatalf("reserved port reused: %d -> %d, %v", first, next, err)
	}
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(next)))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	third, err := firstHubPort(base)
	if err != nil || third <= next {
		t.Fatalf("busy port reused: %d -> %d, %v", next, third, err)
	}
}

func TestNamedServerHubEnv(t *testing.T) {
	server := newHub(t)
	home := t.TempDir()
	testutil.SetHome(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".myco"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".myco", "config.yaml"), []byte("hub: http://127.0.0.1:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	old := hubEnvPath
	hubEnvPath = filepath.Join(root, "hub.env")
	defer func() { hubEnvPath = old }()
	path := filepath.Join(root, "hubs", "work")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "hub.env"), []byte("MYCO_ADMIN_TOKEN=admin-token-123456\nMYCO_HUB="+server+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &harnessT{t, map[string]string{"MYCO_HUB_NAME": "work"}}
	if code, out, e := h.run("link", "create"); code != 0 || !strings.Contains(out, server+"/j/") {
		t.Fatalf("named server admin env: %d %s %s", code, out, e)
	}
	h = &harnessT{t, map[string]string{}}
	if code, out, e := h.run("link", "create", "--hub-name", "work"); code != 0 || !strings.Contains(out, server+"/j/") {
		t.Fatalf("named server admin flag: %d %s %s", code, out, e)
	}
}
