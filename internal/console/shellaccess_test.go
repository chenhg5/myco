package console_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/console"
)

// Allowing shell nodes from the app flips spawn.max_access in this
// computer's machine.yaml and keeps everything else, keys it does not know
// included; turning it off goes back to the default.
func TestShellAccess(t *testing.T) {
	r := newRig(t)
	home := t.TempDir()
	l := &console.Local{Home: home, Device: "test-mac", LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Local: l, Shell: "mac", Version: "v1"}
	r.app.Config.Handler = b.Handler()

	if code, m, _ := r.do("POST", "/app/local/shell-access", map[string]bool{"allow": true}); code != 409 || m["error"] != "not_configured" {
		t.Fatalf("before joining: %d %v", code, m)
	}
	path := filepath.Join(home, ".myco", "machine.yaml")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte("hub: https://hub.example.com\nmachine: test-mac\nenroll_token: secret\nname_prefix: test-mac-\nfuture_field: keep me\nspawn:\n  dirs: [~/code]\n"), 0o600)

	_, m, _ := r.do("GET", "/app/local", nil)
	if m["shell_allowed"] != false || len(m["machines"].([]any)) != 1 || m["machines"].([]any)[0] != "test-mac" {
		t.Fatalf("status before: %v", m)
	}
	if code, m, _ := r.do("POST", "/app/local/shell-access", map[string]bool{"allow": true}); code != 200 || m["shell_allowed"] != true {
		t.Fatalf("allow: %d %v", code, m)
	}
	got, _ := os.ReadFile(path)
	for _, want := range []string{"max_access: full", "future_field: keep me", "enroll_token: secret", "~/code"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("machine.yaml lost %q:\n%s", want, got)
		}
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("machine.yaml mode %o, want 600 (it holds a token)", st.Mode().Perm())
	}
	if code, m, _ := r.do("POST", "/app/local/shell-access", map[string]bool{"allow": false}); code != 200 || m["shell_allowed"] != false {
		t.Fatalf("turn off: %d %v", code, m)
	}
	got, _ = os.ReadFile(path)
	if strings.Contains(string(got), "max_access") || !strings.Contains(string(got), "~/code") {
		t.Errorf("turning off must go back to the default and keep the rest:\n%s", got)
	}
}
