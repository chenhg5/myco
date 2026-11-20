package console_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/console"
)

// The folders agents may start in are the computer's own: the App changes
// them on this computer, or on one it reaches over SSH or WSL (running
// myco machine dirs there); for any other it gives the command.
func TestMachineDirsHereRemoteAndElsewhere(t *testing.T) {
	r := newRig(t)
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".myco"), 0o700)
	os.MkdirAll(filepath.Join(home, "code", "tapnow"), 0o700)
	os.WriteFile(filepath.Join(home, ".myco", "machine.yaml"), []byte("hub: https://h.example\nspawn:\n  dirs: [\"~/code/myco-team\"]\n"), 0o600)
	var script string
	remote := func(ctx context.Context, name string, args []string, stdin string, line func(string)) (string, int, error) {
		script = stdin
		if strings.Contains(stdin, "'~/old'") {
			return `{"error":"invalid_usage","message":"unknown command \"dirs\" for myco machine"}`, 2, nil
		}
		for _, l := range strings.Split("{\n  \"changed\": true,\n  \"dirs\": [\"~/code/myco-team\", \"~/code/tapnow\"]\n}", "\n") {
			line(l)
		}
		return "", 0, nil
	}
	b := &console.Backend{Store: console.NewStore(filepath.Join(t.TempDir(), "hubs.json")), Secrets: r.secrets, Timeout: 3 * time.Second,
		Local: &console.Local{Home: home, Remote: remote}}
	r.app = httptest.NewServer(b.Handler())
	t.Cleanup(r.app.Close)
	_, m, _ := r.do("POST", "/app/hubs", map[string]string{"name": "Main", "url": r.hubURL, "token": r.consoleToken("t")})
	id := m["id"].(string)

	// This computer: machine.yaml here.
	code, m, _ := r.do("POST", "/app/hubs/"+id+"/machines/mach_here/dirs", map[string]any{"add": []string{"~/code/tapnow"}, "here": true})
	if code != 200 || m["via"] != "here" || m["changed"] != true {
		t.Fatalf("here: %d %v", code, m)
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".myco", "machine.yaml")); !strings.Contains(string(b), "~/code/tapnow") {
		t.Fatalf("machine.yaml: %s", b)
	}
	if code, m, _ := r.do("POST", "/app/hubs/"+id+"/machines/mach_here/dirs", map[string]any{"add": []string{"~/nowhere"}, "here": true}); code != 404 {
		t.Fatalf("a folder that is not there: %d %v", code, m)
	}
	// Another computer the App does not reach: the command to run there.
	code, m, _ = r.do("POST", "/app/hubs/"+id+"/machines/mach_far/dirs", map[string]any{"add": []string{"~/code/tapnow"}})
	if code != 409 || m["error"] != "not_reachable" || m["suggestion"] != "myco machine dirs --add '~/code/tapnow'" {
		t.Fatalf("elsewhere: %d %v", code, m)
	}
	// One it reaches into WSL: it runs myco machine dirs there.
	if code, _, _ := r.do("PUT", "/app/hubs/"+id+"/machines/mach_wsl/reach", map[string]any{"via": "wsl", "target": "Ubuntu"}); code != 200 {
		t.Fatalf("reach: %d", code)
	}
	code, m, _ = r.do("POST", "/app/hubs/"+id+"/machines/mach_wsl/dirs", map[string]any{"add": []string{"~/code/tapnow"}})
	if code != 200 || m["via"] != "wsl" || len(m["dirs"].([]any)) != 2 || !strings.Contains(script, "machine dirs --add '~/code/tapnow'") {
		t.Fatalf("wsl: %d %v\n%s", code, m, script)
	}
	// Its myco is too old for that: say to upgrade it.
	if code, m, _ := r.do("POST", "/app/hubs/"+id+"/machines/mach_wsl/dirs", map[string]any{"remove": []string{"~/old"}}); code != 409 || m["error"] != "too_old" {
		t.Fatalf("too old: %d %v", code, m)
	}
}
